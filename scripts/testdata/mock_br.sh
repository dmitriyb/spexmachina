#!/usr/bin/env bash
#
# mock_br.sh — minimal br stand-in for adapter substitution-table tests.
#
# Records every invocation (one line per call, args quoted-and-printed via
# printf %q) to "$BR_MOCK_LOG". Reads canned state from "$BR_MOCK_STATE"
# (a JSON file the test harness sets up and mutates between calls).
#
# Supported subcommands (only what the adapter needs):
#   --version                          → prints a fake version line, exits 0.
#   list --json [...]                  → echoes .issues from BR_MOCK_STATE as
#                                        {"issues": [...]}; supports --label X
#                                        for an in-memory filter.
#   create --json [...]                → mints a new id from a sequence in
#                                        BR_MOCK_STATE (.create_ids[]); records
#                                        the new bead onto .issues; prints
#                                        {"id":"<minted>"}.
#   show <id> --format json            → echoes [issue] for matching .issues,
#                                        else exits 1 with "not found".
#   update <id> --add-label <l>        → mutates .issues[<id>].labels
#                                        (deduped, matching real br).
#   close <id> [...]                   → marks status=closed.
#   dep add <id> <dep> --type <t>      → appends {id,dependency_type} to
#                                        .issues[<id>].dependencies (deduped
#                                        by dep id, matching real br).
#
# The mock is intentionally NOT a faithful br emulator — it covers the calls
# the adapter makes and lets tests assert exact flag sequences via the log.

set -euo pipefail

LOG="${BR_MOCK_LOG:-/dev/null}"
STATE="${BR_MOCK_STATE:-}"

# Log the invocation as a single line.
{
    line=""
    for a in "$@"; do
        line+=" $(printf '%q' "$a")"
    done
    echo "br${line}"
} >> "$LOG"

# Helper: read state into a variable, mutate, write back atomically.
_mutate_state() {
    if [[ -z "$STATE" || ! -f "$STATE" ]]; then
        echo "mock_br: BR_MOCK_STATE not set or missing" >&2
        exit 2
    fi
    local tmp="${STATE}.tmp"
    jq "$1" "$STATE" > "$tmp"
    mv "$tmp" "$STATE"
}

_state() {
    if [[ -z "$STATE" || ! -f "$STATE" ]]; then
        echo '{"issues":[],"create_ids":[]}'
    else
        cat "$STATE"
    fi
}

# Top-level dispatch.
if [[ "${1:-}" == "--version" ]]; then
    echo "br 0.0.0-mock"
    exit 0
fi

sub="${1:-}"
shift || true

case "$sub" in
    list)
        # Filter: --label <X> (AND); ignore other flags.
        labels=()
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --label|-l) labels+=("$2"); shift 2 ;;
                --json|--format) shift ;;
                json) shift ;;
                *) shift ;;
            esac
        done
        s=$(_state)
        if [[ "${#labels[@]}" -eq 0 ]]; then
            echo "$s" | jq '{issues: (.issues // [])}'
        else
            jqargs=(--argjson req "$(printf '%s\n' "${labels[@]}" | jq -R . | jq -s .)")
            echo "$s" | jq "${jqargs[@]}" \
                '{issues: ((.issues // []) | map(select(.labels as $L | $req | all(. as $r | $L | index($r)))))}'
        fi
        ;;
    create)
        # Parse flags we care about.
        title=""; body=""; type=""; priority=""
        labels_csv=""; parent=""; deps=()
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --title)       title="$2"; shift 2 ;;
                --description|--body|-d) body="$2"; shift 2 ;;
                --type|-t)     type="$2"; shift 2 ;;
                --priority|-p) priority="$2"; shift 2 ;;
                --labels|-l)   labels_csv="$2"; shift 2 ;;
                --parent)      parent="$2"; shift 2 ;;
                --deps)        deps+=("$2"); shift 2 ;;
                --json)        shift ;;
                *)             shift ;;
            esac
        done

        if [[ -z "$STATE" || ! -f "$STATE" ]]; then
            echo "mock_br: BR_MOCK_STATE not set" >&2
            exit 2
        fi

        new_id=$(jq -r '(.create_ids // [])[0] // empty' "$STATE")
        if [[ -z "$new_id" ]]; then
            echo "mock_br: create_ids exhausted" >&2
            exit 1
        fi

        # Build labels array from CSV.
        labels_json='[]'
        if [[ -n "$labels_csv" ]]; then
            labels_json=$(printf '%s' "$labels_csv" | jq -R 'split(",") | map(select(length > 0))')
        fi

        new_issue=$(jq -n \
            --arg id "$new_id" --arg title "$title" --arg type "$type" \
            --arg parent "$parent" --argjson labels "$labels_json" \
            '{id:$id, title:$title, status:"open", issue_type:$type, parent:$parent, labels:$labels}')

        _mutate_state "(.issues += [$new_issue]) | .create_ids = ((.create_ids // [])[1:])"

        jq -n --arg id "$new_id" '{id:$id}'
        ;;
    show)
        id="$1"; shift || true
        # Skip --format json or other flags.
        s=$(_state)
        out=$(jq --arg id "$id" '(.issues // []) | map(select(.id == $id))' <<< "$s")
        if [[ "$(jq 'length' <<< "$out")" == "0" ]]; then
            echo "mock_br: not found: $id" >&2
            exit 1
        fi
        echo "$out"
        ;;
    update)
        id="$1"; shift || true
        add_labels=()
        while [[ $# -gt 0 ]]; do
            case "$1" in
                --add-label) add_labels+=("$2"); shift 2 ;;
                *) shift ;;
            esac
        done
        # Build a jq filter that appends each label, preserving insertion
        # order. Deduped, same as real br: adding a label the issue already
        # carries is a no-op.
        for l in "${add_labels[@]}"; do
            _mutate_state "(.issues |= map(if .id == \"$id\" then .labels = (if ((.labels // []) | index(\"$l\")) then (.labels // []) else ((.labels // []) + [\"$l\"]) end) else . end))"
        done
        echo "ok"
        ;;
    dep)
        subsub="${1:-}"; shift || true
        case "$subsub" in
            add)
                id="$1"; shift || true
                dep_id="$1"; shift || true
                dtype="blocks"
                while [[ $# -gt 0 ]]; do
                    case "$1" in
                        --type|-t) dtype="$2"; shift 2 ;;
                        --metadata) shift 2 ;;
                        --json) shift ;;
                        *) shift ;;
                    esac
                done
                # Appended, preserving insertion order. Deduped by dep id,
                # same as real br: re-adding an edge the issue already
                # carries is a no-op.
                _mutate_state "(.issues |= map(if .id == \"$id\" then .dependencies = (if ((.dependencies // []) | any(.id == \"$dep_id\")) then (.dependencies // []) else ((.dependencies // []) + [{id: \"$dep_id\", dependency_type: \"$dtype\"}]) end) else . end))"
                echo "ok"
                ;;
            *)
                echo "mock_br: unsupported dep subcommand: $subsub" >&2
                exit 2
                ;;
        esac
        ;;
    close)
        id="$1"; shift || true
        # Verify the bead exists; required for the "missing target" error path.
        s=$(_state)
        if [[ "$(jq --arg id "$id" '(.issues // []) | map(select(.id == $id)) | length' <<< "$s")" == "0" ]]; then
            echo "mock_br: not found: $id" >&2
            exit 1
        fi
        _mutate_state "(.issues |= map(if .id == \"$id\" then .status = \"closed\" else . end))"
        echo "closed"
        ;;
    *)
        echo "mock_br: unsupported subcommand: $sub" >&2
        exit 2
        ;;
esac
