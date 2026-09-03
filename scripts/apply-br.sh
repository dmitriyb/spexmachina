#!/usr/bin/env bash
#
# apply-br.sh — Reference adapter consuming spex changeset.json v4 and invoking br.
#
# REFERENCE IMPLEMENTATION. Vet before production use. See spec/adapters/ for the
# adapter contract that any implementation (this one or your own) must satisfy.
#
# Usage: apply-br.sh [<changeset.json>] [<receipts.json>]
#
#   With no args: changeset on stdin, receipts on stdout.
#   With one arg: that file is the changeset; receipts on stdout.
#   With two args: receipts written atomically to <receipts.json>.
#
# Test hooks (env vars):
#   BR_BIN              override the br binary path (default: br on PATH).
#   SPEX_ADAPTER_DEBUG  set to 1 to dump SUB_TABLE to stderr after each op.

set -euo pipefail

BR_BIN="${BR_BIN:-br}"

# ---- Pre-flight ------------------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
    echo "error: jq not on PATH" >&2
    exit 1
fi
if ! command -v "$BR_BIN" >/dev/null 2>&1; then
    echo "error: br ('$BR_BIN') not on PATH" >&2
    exit 1
fi
if ! "$BR_BIN" --version >/dev/null 2>&1; then
    echo "error: br ('$BR_BIN') unresponsive to --version" >&2
    exit 1
fi

# Resolve I/O.
CHANGESET_SRC=""
RECEIPTS_OUT=""
case "$#" in
    0) ;;
    1) CHANGESET_SRC="$1" ;;
    2) CHANGESET_SRC="$1"; RECEIPTS_OUT="$2" ;;
    *) echo "usage: apply-br.sh [<changeset.json>] [<receipts.json>]" >&2; exit 1 ;;
esac

if [[ -n "$CHANGESET_SRC" ]]; then
    if [[ ! -f "$CHANGESET_SRC" ]]; then
        echo "error: changeset file not found: $CHANGESET_SRC" >&2
        exit 1
    fi
    CHANGESET_JSON=$(cat "$CHANGESET_SRC")
else
    CHANGESET_JSON=$(cat)
fi

# ---- Changeset parse + validation -----------------------------------------

if ! echo "$CHANGESET_JSON" | jq -e . >/dev/null 2>&1; then
    echo "error: changeset is not valid JSON" >&2
    exit 1
fi

VERSION=$(jq -r '.version // empty' <<< "$CHANGESET_JSON")
if [[ -z "$VERSION" ]]; then
    echo "error: changeset missing required field: version" >&2
    exit 1
fi
if [[ "$VERSION" != "4" ]]; then
    echo "error: unsupported changeset version: $VERSION (expected 4)" >&2
    exit 1
fi

GIT_HEAD=$(jq -r '.git_head // empty' <<< "$CHANGESET_JSON")
if [[ -z "$GIT_HEAD" ]]; then
    echo "error: changeset missing required field: git_head" >&2
    exit 1
fi

PROPOSAL=$(jq -r '.proposal // empty' <<< "$CHANGESET_JSON")
if [[ -z "$PROPOSAL" ]]; then
    echo "error: changeset missing required field: proposal" >&2
    exit 1
fi

if ! jq -e '.ops | type == "array"' <<< "$CHANGESET_JSON" >/dev/null 2>&1; then
    echo "error: changeset missing or malformed required field: ops" >&2
    exit 1
fi

# ---- State -----------------------------------------------------------------

declare -A SUB_TABLE   # op_id → task_id (created or matched)
declare -A OP_STATUS   # op_id → ok|error (for ref:op resolution diagnostics)
RECEIPTS=()

# ---- Helpers ---------------------------------------------------------------

debug_sub_table() {
    if [[ "${SPEX_ADAPTER_DEBUG:-0}" == "1" ]]; then
        local after="$1"
        echo "DEBUG: SUB_TABLE after $after:" >&2
        for k in "${!SUB_TABLE[@]}"; do
            echo "  $k → ${SUB_TABLE[$k]}" >&2
        done
    fi
}

append_receipt_ok() {
    local op_id="$1" task_id="$2" was_existing="$3"
    OP_STATUS["$op_id"]="ok"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg tid "$task_id" --argjson we "$was_existing" \
        '{op_id: $op, status: "ok", task_id: $tid, was_existing: $we}')")
}

append_receipt_ok_no_existing() {
    local op_id="$1" task_id="$2"
    OP_STATUS["$op_id"]="ok"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg tid "$task_id" \
        '{op_id: $op, status: "ok", task_id: $tid}')")
}

append_receipt_error() {
    local op_id="$1" task_id="$2" err="$3"
    OP_STATUS["$op_id"]="error"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg tid "$task_id" --arg e "$err" \
        '{op_id: $op, status: "error", task_id: $tid, was_existing: false, error: $e}')")
}

# resolve_ref echoes the resolved task_id (or sentinel) for a ref JSON object.
# Changeset v4 admits exactly two ref shapes, neither carrying an edge-type
# field — plan resolves spec-node references in-process before the adapter
# ever runs, so the adapter reads no spex-owned file. Sentinels:
#   __UNRESOLVED_OP__<op_id>     ref:op pointed at an op_id with no SUB_TABLE entry.
#   __ERRORED_OP__<op_id>        ref:op pointed at an op that errored earlier.
#   __UNKNOWN_REF__<kind>        unknown ref kind discriminator (the retired
#                                 v3 "bead" spelling included).
resolve_ref() {
    local ref_json="$1"
    local kind
    kind=$(jq -r '.ref // empty' <<< "$ref_json")
    case "$kind" in
        task)
            jq -r '.task_id' <<< "$ref_json"
            ;;
        op)
            local op_id
            op_id=$(jq -r '.op_id' <<< "$ref_json")
            if [[ -n "${SUB_TABLE[$op_id]:-}" ]]; then
                echo "${SUB_TABLE[$op_id]}"
            elif [[ "${OP_STATUS[$op_id]:-}" == "error" ]]; then
                echo "__ERRORED_OP__$op_id"
            else
                echo "__UNRESOLVED_OP__$op_id"
            fi
            ;;
        "")
            echo "__UNKNOWN_REF__missing"
            ;;
        *)
            echo "__UNKNOWN_REF__$kind"
            ;;
    esac
}

# ref_error_for inspects a sentinel resolved task_id and echoes a human-readable
# diagnostic, or empty if the value is a real task_id.
ref_error_for() {
    local val="$1"
    case "$val" in
        __UNRESOLVED_OP__*)    echo "dependency ${val#__UNRESOLVED_OP__} not yet resolved" ;;
        __ERRORED_OP__*)       echo "dependency ${val#__ERRORED_OP__} errored; cannot resolve op ref" ;;
        __UNKNOWN_REF__*)      echo "unknown ref kind: ${val#__UNKNOWN_REF__}" ;;
        *)                     echo "" ;;
    esac
}

# spec_kind_to_task_type maps changeset spec_node_kind onto br --type values.
spec_kind_to_task_type() {
    case "$1" in
        proposal_epic) echo epic ;;
        component)     echo feature ;;
        data_flow)     echo task ;;
        test_section)  echo task ;;
        cleanup)       echo task ;;
        *)             echo feature ;;
    esac
}

# ---- Op processing ---------------------------------------------------------

process_create() {
    local op="$1"
    local op_id="$2"
    local label
    label=$(jq -r '.idempotency.label // empty' <<< "$op")

    if [[ -z "$label" ]]; then
        append_receipt_error "$op_id" "" "create op missing idempotency.label"
        return
    fi

    # Idempotency pre-check: any task carrying this exact label, in any
    # status? Status-unfiltered and unbounded (--all --limit 0) because
    # br list's defaults hide closed tasks and cap the row count, and
    # either default would silently reintroduce the retired open-only
    # semantics. Labels are spex:<eid> — unique per change — so a task
    # carrying this op's exact label, whatever its status, can only be
    # this same op's own earlier product; no status filtering is needed.
    # See spec/adapters/arch_br_reference_adapter.md "Idempotency".
    local existing
    if ! existing=$("$BR_BIN" list --json --all --limit 0 --label "$label" 2>/dev/null \
            | jq -r --arg L "$label" \
                '(.issues // []) | map(select((.labels // []) | any(. == $L))) | .[0].id // empty'); then
        append_receipt_error "$op_id" "" "br list failed during idempotency check"
        return
    fi
    if [[ -n "$existing" && "$existing" != "null" ]]; then
        # Per flow_adapter.md: a match ends the op with an ok receipt, not
        # skipped — ingest constructs journal lines only for ok receipts,
        # and this was_existing=true pairing is what the crashed-adapter
        # recovery path needs to land.
        SUB_TABLE["$op_id"]="$existing"
        append_receipt_ok "$op_id" "$existing" true
        return
    fi

    local title body kind priority task_type
    title=$(jq -r '.title // empty' <<< "$op")
    body=$(jq -r '.body // empty' <<< "$op")
    kind=$(jq -r '.spec_node_kind // empty' <<< "$op")
    priority=$(jq -r '.priority // empty' <<< "$op")
    task_type=$(spec_kind_to_task_type "$kind")

    if [[ -z "$title" ]]; then
        append_receipt_error "$op_id" "" "create op missing title"
        return
    fi

    local -a flags
    flags=(--title "$title" --labels "$label" --type "$task_type" --json)
    [[ -n "$body" ]] && flags+=(--description "$body")
    [[ -n "$priority" ]] && flags+=(--priority "$priority")

    # Parent ref.
    local parent_ref
    parent_ref=$(jq -c '.parent // empty' <<< "$op")
    if [[ -n "$parent_ref" && "$parent_ref" != "null" ]]; then
        local parent_task parent_err
        parent_task=$(resolve_ref "$parent_ref")
        parent_err=$(ref_error_for "$parent_task")
        if [[ -n "$parent_err" ]]; then
            append_receipt_error "$op_id" "" "parent ref: $parent_err"
            return
        fi
        flags+=(--parent "$parent_task")
    fi

    # Dep refs — emit one --deps flag per ref. The edge is always
    # blocked-by: v4 refs carry no edge-type field, and the only typed dep
    # (the lineage edge) is gone.
    local dep_ref dep_task dep_err
    while IFS= read -r dep_ref; do
        [[ -z "$dep_ref" || "$dep_ref" == "null" ]] && continue
        dep_task=$(resolve_ref "$dep_ref")
        dep_err=$(ref_error_for "$dep_task")
        if [[ -n "$dep_err" ]]; then
            append_receipt_error "$op_id" "" "dep ref: $dep_err"
            return
        fi
        flags+=(--deps "blocked-by:$dep_task")
    done < <(jq -c '(.deps // [])[]' <<< "$op")

    # Execute.
    local out new_task rc
    if out=$("$BR_BIN" create "${flags[@]}" 2>&1); then
        new_task=$(jq -r '.id // empty' <<< "$out" 2>/dev/null || true)
        if [[ -z "$new_task" || "$new_task" == "null" ]]; then
            append_receipt_error "$op_id" "" "br create returned no id: $out"
            return
        fi
        SUB_TABLE["$op_id"]="$new_task"
    else
        rc=$?
        append_receipt_error "$op_id" "" "br create exited $rc: $out"
        return
    fi

    # op.Labels → post-create `br update --add-label` calls. `br create`
    # has no --add-label flag (only the comma-joined --labels for the
    # idempotency label). In current plan output only retarget ops
    # populate op.Labels — creates carry none — but the application path
    # stays generic. See arch_br_reference_adapter.md "Op Translation".
    local extra_label upd_out
    while IFS= read -r extra_label; do
        [[ -z "$extra_label" ]] && continue
        if ! upd_out=$("$BR_BIN" update "$new_task" --add-label "$extra_label" 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$new_task" "br update --add-label $extra_label exited $rc: $upd_out"
            return
        fi
    done < <(jq -r '(.labels // [])[]' <<< "$op")

    append_receipt_ok "$op_id" "$new_task" false
}

process_close() {
    local op="$1"
    local op_id="$2"
    local target
    target=$(jq -c '.target // empty' <<< "$op")
    if [[ -z "$target" || "$target" == "null" ]]; then
        append_receipt_error "$op_id" "" "close op missing target"
        return
    fi

    local task_id ref_err
    task_id=$(resolve_ref "$target")
    ref_err=$(ref_error_for "$task_id")
    if [[ -n "$ref_err" ]]; then
        append_receipt_error "$op_id" "" "target ref: $ref_err"
        return
    fi

    # Idempotency per spec/adapters/arch_br_reference_adapter.md
    # "Idempotency". Close ops carry no labels, so this keys purely on the
    # tracker's own status, read with a single `br show`. Nothing is
    # re-queried between the decision and the action.
    local show_out
    if ! show_out=$("$BR_BIN" show "$task_id" --format json 2>&1); then
        append_receipt_error "$op_id" "$task_id" "br show failed: $show_out"
        return
    fi
    local current_status
    current_status=$(jq -r '(.[0].status // .status // "")' <<< "$show_out")

    if [[ "$current_status" == "closed" ]]; then
        # Skip branch: whichever run — or the task's own lifecycle —
        # closed the target first, a close op against a closed task is
        # complete. `br close` exits 3 on already-closed targets, so the
        # call is skipped rather than attempted. Converges on status=ok
        # (not skipped) deliberately: the journal's eid-deduped receipts
        # absorb a re-run without a second task_closed line.
        append_receipt_ok "$op_id" "$task_id" false
        return
    fi

    # Open branch: close it.
    local reason out rc
    reason=$(jq -r '.reason // empty' <<< "$op")
    local -a close_flags
    close_flags=("$task_id" --force)
    [[ -n "$reason" ]] && close_flags+=(--reason "$reason")
    if out=$("$BR_BIN" close "${close_flags[@]}" 2>&1); then
        append_receipt_ok "$op_id" "$task_id" false
    else
        rc=$?
        append_receipt_error "$op_id" "$task_id" "br close exited $rc: $out"
    fi
}

process_retarget() {
    local op="$1"
    local op_id="$2"
    local target
    target=$(jq -c '.target // empty' <<< "$op")
    if [[ -z "$target" || "$target" == "null" ]]; then
        append_receipt_error "$op_id" "" "retarget op missing target"
        return
    fi

    local task_id ref_err
    task_id=$(resolve_ref "$target")
    ref_err=$(ref_error_for "$task_id")
    if [[ -n "$ref_err" ]]; then
        append_receipt_error "$op_id" "" "target ref: $ref_err"
        return
    fi

    # Resolve dep refs up front — a ref that fails to resolve stops the op
    # before any br call that would change tracker state, same as create.
    local -a dep_tasks=()
    local dep_ref dep_task dep_err
    while IFS= read -r dep_ref; do
        [[ -z "$dep_ref" || "$dep_ref" == "null" ]] && continue
        dep_task=$(resolve_ref "$dep_ref")
        dep_err=$(ref_error_for "$dep_task")
        if [[ -n "$dep_err" ]]; then
            append_receipt_error "$op_id" "$task_id" "dep ref: $dep_err"
            return
        fi
        dep_tasks+=("$dep_task")
    done < <(jq -c '(.deps // [])[]' <<< "$op")

    # No idempotency probe — br update and br dep add both converge when
    # applied twice. Read current deps once, up front, per
    # arch_br_reference_adapter.md "retarget op → br update + br dep add".
    local show_out
    if ! show_out=$("$BR_BIN" show "$task_id" --format json 2>&1); then
        append_receipt_error "$op_id" "$task_id" "br show failed: $show_out"
        return
    fi

    # Event label(s) — br update carries no dep flag of any name, so the
    # label half stays on the update surface.
    local lbl out rc
    while IFS= read -r lbl; do
        [[ -z "$lbl" ]] && continue
        if ! out=$("$BR_BIN" update "$task_id" --add-label "$lbl" 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$task_id" "br update --add-label $lbl exited $rc: $out"
            return
        fi
    done < <(jq -r '(.labels // [])[]' <<< "$op")

    # Missing deps only — add-only by contract, nothing removed. The edge
    # is always "blocks": the tracker's own dep-type vocabulary, the same
    # edge the create path spells "blocked-by" via `br create --deps`.
    local db
    for db in "${dep_tasks[@]:-}"; do
        [[ -z "$db" ]] && continue
        if jq -e --arg id "$db" '(.[0].dependencies // .dependencies // []) | any(.id == $id)' <<< "$show_out" >/dev/null 2>&1; then
            continue
        fi
        if ! out=$("$BR_BIN" dep add "$task_id" "$db" --type blocks 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$task_id" "br dep add $db --type blocks exited $rc: $out"
            return
        fi
    done

    append_receipt_ok_no_existing "$op_id" "$task_id"
}

# ---- Main loop -------------------------------------------------------------

OP_COUNT=$(jq '.ops | length' <<< "$CHANGESET_JSON")
i=0
while [[ "$i" -lt "$OP_COUNT" ]]; do
    op=$(jq -c ".ops[$i]" <<< "$CHANGESET_JSON")
    op_id=$(jq -r '.op_id // empty' <<< "$op")
    op_type=$(jq -r '.type // empty' <<< "$op")

    if [[ -z "$op_id" || -z "$op_type" ]]; then
        # Use a stable synthetic id so receipts still align with input order.
        synth="op-index-$i"
        append_receipt_error "${op_id:-$synth}" "" "malformed op: missing op_id or type"
    else
        case "$op_type" in
            create)   process_create   "$op" "$op_id" ;;
            close)    process_close    "$op" "$op_id" ;;
            retarget) process_retarget "$op" "$op_id" ;;
            # The v3 "label" and "tag" kinds land here: nothing ever
            # emitted them, and v4 retired them from the vocabulary.
            *)        append_receipt_error "$op_id" "" "unknown op type: $op_type" ;;
        esac
    fi
    debug_sub_table "$op_id"
    i=$((i + 1))
done

# ---- Emit receipts ---------------------------------------------------------

top_status="complete"
for r in "${RECEIPTS[@]:-}"; do
    [[ -z "$r" ]] && continue
    s=$(jq -r '.status' <<< "$r")
    if [[ "$s" == "error" ]]; then
        top_status="partial"
        break
    fi
done

# Build the final v2 wrapper. Avoid relying on shell expansion for the ops
# array — feed each receipt to jq via --slurpfile-equivalent stdin so we never
# trip over IFS or empty-arrays.
ops_json="[]"
if [[ "${#RECEIPTS[@]}" -gt 0 ]]; then
    ops_json=$(printf '%s\n' "${RECEIPTS[@]}" | jq -s '.')
fi

out=$(jq -n \
    --arg st "$top_status" \
    --argjson ops "$ops_json" \
    '{version: 2, status: $st, ops: $ops}')

if [[ -n "$RECEIPTS_OUT" ]]; then
    tmp="${RECEIPTS_OUT}.tmp"
    jq . <<< "$out" > "$tmp"
    mv "$tmp" "$RECEIPTS_OUT"
else
    jq . <<< "$out"
fi
