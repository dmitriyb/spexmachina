#!/usr/bin/env bash
#
# apply-br.sh — Reference adapter consuming spex changeset.json v1 and invoking br.
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
#   SPEX_MAPPING_FILE   override .bead-map.json path for ref:spec_node lookup
#                       (default: .bead-map.json in CWD).
#   SPEX_ADAPTER_DEBUG  set to 1 to dump SUB_TABLE to stderr after each op.

set -euo pipefail

BR_BIN="${BR_BIN:-br}"
SPEX_MAPPING_FILE="${SPEX_MAPPING_FILE:-.bead-map.json}"

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
if [[ "$VERSION" != "1" ]]; then
    echo "error: unsupported changeset version: $VERSION (expected 1)" >&2
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

declare -A SUB_TABLE   # op_id → bead_id (created or matched)
declare -A OP_STATUS   # op_id → ok|skipped|error (for ref:op resolution diagnostics)
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
    local op_id="$1" bead_id="$2" was_existing="$3"
    OP_STATUS["$op_id"]="ok"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg bid "$bead_id" --argjson we "$was_existing" \
        '{op_id: $op, status: "ok", bead_id: $bid, was_existing: $we}')")
}

append_receipt_skipped() {
    local op_id="$1" bead_id="$2" was_existing="$3" reason="$4"
    OP_STATUS["$op_id"]="skipped"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg bid "$bead_id" --argjson we "$was_existing" --arg r "$reason" \
        '{op_id: $op, status: "skipped", bead_id: $bid, was_existing: $we, reason: $r}')")
}

append_receipt_error() {
    local op_id="$1" bead_id="$2" err="$3"
    OP_STATUS["$op_id"]="error"
    RECEIPTS+=("$(jq -cn \
        --arg op "$op_id" --arg bid "$bead_id" --arg e "$err" \
        '{op_id: $op, status: "error", bead_id: $bid, was_existing: false, error: $e}')")
}

# resolve_ref echoes the resolved bead_id (or sentinel) for a ref JSON object.
# Sentinels:
#   __UNRESOLVED_OP__<op_id>     ref:op pointed at an op_id with no SUB_TABLE entry.
#   __ERRORED_OP__<op_id>        ref:op pointed at an op that errored earlier.
#   __UNKNOWN_REF__<kind>        unknown ref kind discriminator.
#   __NO_MAPPING__<spec_node_id> ref:spec_node had no record in the mapping store.
#   __MAPPING_UNAVAILABLE__      SPEX_MAPPING_FILE missing while a ref:spec_node was used.
resolve_ref() {
    local ref_json="$1"
    local kind
    kind=$(jq -r '.ref // empty' <<< "$ref_json")
    case "$kind" in
        bead)
            jq -r '.bead_id' <<< "$ref_json"
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
        spec_node)
            local snid
            snid=$(jq -r '.spec_node_id' <<< "$ref_json")
            if [[ ! -f "$SPEX_MAPPING_FILE" ]]; then
                echo "__MAPPING_UNAVAILABLE__"
                return
            fi
            local hit
            hit=$(jq -r --arg snid "$snid" \
                '(.records // []) | map(select(.spec_node_id == $snid)) |
                 sort_by(.id // 0) | last | (.bead_id // empty)' \
                "$SPEX_MAPPING_FILE")
            if [[ -z "$hit" || "$hit" == "null" ]]; then
                echo "__NO_MAPPING__$snid"
            else
                echo "$hit"
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

# ref_error_for inspects a sentinel resolved bead_id and echoes a human-readable
# diagnostic, or empty if the value is a real bead_id.
ref_error_for() {
    local val="$1"
    case "$val" in
        __UNRESOLVED_OP__*)    echo "dependency ${val#__UNRESOLVED_OP__} not yet resolved" ;;
        __ERRORED_OP__*)       echo "dependency ${val#__ERRORED_OP__} errored; cannot resolve op ref" ;;
        __NO_MAPPING__*)       echo "spec_node_id ${val#__NO_MAPPING__} has no mapping record" ;;
        __MAPPING_UNAVAILABLE__) echo "mapping file unavailable for ref:spec_node lookup" ;;
        __UNKNOWN_REF__*)      echo "unknown ref kind: ${val#__UNKNOWN_REF__}" ;;
        *)                     echo "" ;;
    esac
}

# spec_kind_to_bead_type maps changeset spec_node_kind onto br --type values.
spec_kind_to_bead_type() {
    case "$1" in
        proposal_epic) echo epic ;;
        component)     echo feature ;;
        data_flow)     echo task ;;
        test_section)  echo task ;;
        cleanup)       echo task ;;
        *)             echo feature ;;
    esac
}

# dep_edge_type returns the edge-type prefix for a dep ref. Honors the ref's
# optional "type" field (e.g., "blocks" on lineage refs) and defaults to
# blocked-by since the dep semantics are "this bead depends on these".
dep_edge_type() {
    local ref_json="$1"
    local t
    t=$(jq -r '.type // empty' <<< "$ref_json")
    if [[ -n "$t" ]]; then
        echo "$t"
    else
        echo "blocked-by"
    fi
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

    # Idempotency pre-check: any existing bead carrying this label?
    local existing
    if ! existing=$("$BR_BIN" list --json --label "$label" 2>/dev/null \
            | jq -r --arg L "$label" \
                '(.issues // []) | map(select((.labels // []) | any(. == $L))) | .[0].id // empty'); then
        append_receipt_error "$op_id" "" "br list failed during idempotency check"
        return
    fi
    if [[ -n "$existing" && "$existing" != "null" ]]; then
        SUB_TABLE["$op_id"]="$existing"
        append_receipt_skipped "$op_id" "$existing" true "idempotent re-match"
        return
    fi

    local title body kind priority bead_type
    title=$(jq -r '.title // empty' <<< "$op")
    body=$(jq -r '.body // empty' <<< "$op")
    kind=$(jq -r '.spec_node_kind // empty' <<< "$op")
    priority=$(jq -r '.priority // empty' <<< "$op")
    bead_type=$(spec_kind_to_bead_type "$kind")

    if [[ -z "$title" ]]; then
        append_receipt_error "$op_id" "" "create op missing title"
        return
    fi

    local -a flags
    flags=(--title "$title" --labels "$label" --type "$bead_type" --json)
    [[ -n "$body" ]] && flags+=(--description "$body")
    [[ -n "$priority" ]] && flags+=(--priority "$priority")

    # Parent ref.
    local parent_ref
    parent_ref=$(jq -c '.parent // empty' <<< "$op")
    if [[ -n "$parent_ref" && "$parent_ref" != "null" ]]; then
        local parent_bead parent_err
        parent_bead=$(resolve_ref "$parent_ref")
        parent_err=$(ref_error_for "$parent_bead")
        if [[ -n "$parent_err" ]]; then
            append_receipt_error "$op_id" "" "parent ref: $parent_err"
            return
        fi
        flags+=(--parent "$parent_bead")
    fi

    # Dep refs — emit one --deps flag per ref.
    local dep_ref dep_bead dep_err edge
    while IFS= read -r dep_ref; do
        [[ -z "$dep_ref" || "$dep_ref" == "null" ]] && continue
        dep_bead=$(resolve_ref "$dep_ref")
        dep_err=$(ref_error_for "$dep_bead")
        if [[ -n "$dep_err" ]]; then
            append_receipt_error "$op_id" "" "dep ref: $dep_err"
            return
        fi
        edge=$(dep_edge_type "$dep_ref")
        flags+=(--deps "$edge:$dep_bead")
    done < <(jq -c '(.deps // [])[]' <<< "$op")

    # Execute.
    local out new_bead rc
    if out=$("$BR_BIN" create "${flags[@]}" 2>&1); then
        new_bead=$(jq -r '.id // empty' <<< "$out" 2>/dev/null || true)
        if [[ -z "$new_bead" || "$new_bead" == "null" ]]; then
            append_receipt_error "$op_id" "" "br create returned no id: $out"
            return
        fi
        SUB_TABLE["$op_id"]="$new_bead"
    else
        rc=$?
        append_receipt_error "$op_id" "" "br create exited $rc: $out"
        return
    fi

    # op.Labels → post-create `br update --add-label` calls. `br create`
    # has no --add-label flag (only the comma-joined --labels for the
    # idempotency label); the spex:cleanup discriminator and any other
    # op.Labels entry attaches via update, mirroring pre-decouple
    # createCleanupBead's pattern (create + cli.Update). See
    # arch_br_reference_adapter.md "Op Translation".
    local extra_label upd_out
    while IFS= read -r extra_label; do
        [[ -z "$extra_label" ]] && continue
        if ! upd_out=$("$BR_BIN" update "$new_bead" --add-label "$extra_label" 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$new_bead" "br update --add-label $extra_label exited $rc: $upd_out"
            return
        fi
    done < <(jq -r '(.labels // [])[]' <<< "$op")

    append_receipt_ok "$op_id" "$new_bead" false
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

    local bead_id ref_err
    bead_id=$(resolve_ref "$target")
    ref_err=$(ref_error_for "$bead_id")
    if [[ -n "$ref_err" ]]; then
        append_receipt_error "$op_id" "" "target ref: $ref_err"
        return
    fi

    # Idempotency + already-closed branching per spec/adapters/arch_br_reference_adapter.md
    # "Idempotency". Read both labels AND status from the same `br show`
    # JSON; pick branch by combined state:
    #   - labels contain spex:obsolete → skip, status=skipped
    #   - status="closed" no spex:obsolete → label-only path, status=ok
    #   - status="open" no spex:obsolete → label + close, status=ok
    local show_out
    if ! show_out=$("$BR_BIN" show "$bead_id" --format json 2>&1); then
        append_receipt_error "$op_id" "$bead_id" "br show failed: $show_out"
        return
    fi
    if jq -e '(.[0].labels // .labels // []) | any(. == "spex:obsolete")' <<< "$show_out" >/dev/null 2>&1; then
        append_receipt_skipped "$op_id" "$bead_id" false "already obsoleted"
        return
    fi
    local current_status
    current_status=$(jq -r '(.[0].status // .status // "")' <<< "$show_out")

    # Apply labels regardless of branch (typically spex:obsolete + commit:<HEAD>).
    local lbl out rc
    while IFS= read -r lbl; do
        [[ -z "$lbl" ]] && continue
        if ! out=$("$BR_BIN" update "$bead_id" --add-label "$lbl" 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$bead_id" "br update --add-label $lbl exited $rc: $out"
            return
        fi
    done < <(jq -r '(.labels // [])[]' <<< "$op")

    if [[ "$current_status" == "closed" ]]; then
        # Label-only branch: bead is already closed (e.g. from an
        # earlier bead-lifecycle run). Skip the close call — `br close`
        # exits 3 on already-closed targets even though the labels
        # apply. Receipt is ok; downstream Reconciler treats this
        # identically to the close+label branch in applyClose.
        append_receipt_ok "$op_id" "$bead_id" false
        return
    fi

    # Close+label branch: bead is open.
    local reason
    reason=$(jq -r '.reason // empty' <<< "$op")
    local -a close_flags
    close_flags=("$bead_id" --force)
    [[ -n "$reason" ]] && close_flags+=(--reason "$reason")
    if out=$("$BR_BIN" close "${close_flags[@]}" 2>&1); then
        append_receipt_ok "$op_id" "$bead_id" false
    else
        rc=$?
        append_receipt_error "$op_id" "$bead_id" "br close exited $rc: $out"
    fi
}

process_label() {
    local op="$1"
    local op_id="$2"
    local target
    target=$(jq -c '.target // empty' <<< "$op")
    if [[ -z "$target" || "$target" == "null" ]]; then
        append_receipt_error "$op_id" "" "label op missing target"
        return
    fi

    local bead_id ref_err
    bead_id=$(resolve_ref "$target")
    ref_err=$(ref_error_for "$bead_id")
    if [[ -n "$ref_err" ]]; then
        append_receipt_error "$op_id" "" "target ref: $ref_err"
        return
    fi

    local lbl out rc
    while IFS= read -r lbl; do
        [[ -z "$lbl" ]] && continue
        if ! out=$("$BR_BIN" update "$bead_id" --add-label "$lbl" 2>&1); then
            rc=$?
            append_receipt_error "$op_id" "$bead_id" "br update --add-label $lbl exited $rc: $out"
            return
        fi
    done < <(jq -r '(.labels // [])[]' <<< "$op")
    append_receipt_ok "$op_id" "$bead_id" false
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
            create) process_create "$op" "$op_id" ;;
            close)  process_close  "$op" "$op_id" ;;
            label)  process_label  "$op" "$op_id" ;;
            tag)    process_label  "$op" "$op_id" ;;  # tag and label are structurally identical for br.
            *)      append_receipt_error "$op_id" "" "unknown op type: $op_type" ;;
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

# Build the final v1 wrapper. Avoid relying on shell expansion for the ops
# array — feed each receipt to jq via --slurpfile-equivalent stdin so we never
# trip over IFS or empty-arrays.
ops_json="[]"
if [[ "${#RECEIPTS[@]}" -gt 0 ]]; then
    ops_json=$(printf '%s\n' "${RECEIPTS[@]}" | jq -s '.')
fi

out=$(jq -n \
    --arg st "$top_status" \
    --argjson ops "$ops_json" \
    '{version: 1, status: $st, ops: $ops}')

if [[ -n "$RECEIPTS_OUT" ]]; then
    tmp="${RECEIPTS_OUT}.tmp"
    jq . <<< "$out" > "$tmp"
    mv "$tmp" "$RECEIPTS_OUT"
else
    jq . <<< "$out"
fi
