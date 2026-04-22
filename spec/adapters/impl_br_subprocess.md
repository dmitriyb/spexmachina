# Br subprocess invocation

How the adapter calls `br` subcommands and maps changeset op fields to br flags.

## Commands Used

- `br --version` — pre-flight.
- `br list --json` — idempotency pre-check for create ops.
- `br show <bead_id> --json` — idempotency pre-check for close ops; post-create verification.
- `br create --title <t> [--body <b>] [--parent <id>] [--deps <rel>:<id>] [--priority <n>] [--label <lbl>] [--type <type>]`.
- `br update <bead_id> --add-label <lbl>`.
- `br close <bead_id>`.

## Create Op → br create

```bash
process_create() {
    local op="$1"
    local op_id=$(jq -r '.op_id' <<< "$op")
    local label=$(jq -r '.idempotency.label' <<< "$op")

    # Idempotency pre-check
    local existing=$(br list --json | jq -r --arg L "$label" '.issues[] | select(.labels | any(. == $L)) | .id')
    if [[ -n "$existing" ]]; then
        SUB_TABLE["$op_id"]="$existing"
        append_receipt "$op_id" ok "$existing" true ""
        return
    fi

    # Build flags
    local title=$(jq -r '.title' <<< "$op")
    local body=$(jq -r '.body // empty' <<< "$op")
    local kind=$(jq -r '.spec_node_kind' <<< "$op")
    local priority=$(jq -r '.priority // empty' <<< "$op")
    local bead_type=$(spec_kind_to_bead_type "$kind") # e.g., component → feature

    local flags=(--title "$title" --label "$label" --type "$bead_type")
    [[ -n "$body" ]] && flags+=(--body "$body")
    [[ -n "$priority" ]] && flags+=(--priority "$priority")

    # Parent
    local parent_ref=$(jq -c '.parent // empty' <<< "$op")
    if [[ -n "$parent_ref" ]]; then
        local parent_bead=$(resolve_ref "$parent_ref")
        flags+=(--parent "$parent_bead")
    fi

    # Deps — multiple --deps flags, each depends:<bead>
    while read -r dep_ref; do
        [[ -z "$dep_ref" ]] && continue
        local dep_bead=$(resolve_ref "$dep_ref")
        flags+=(--deps "depends:$dep_bead")
    done < <(jq -c '.deps[]? // empty' <<< "$op")

    # Execute
    local new_bead
    if new_bead=$(br create "${flags[@]}" | jq -r '.id'); then
        SUB_TABLE["$op_id"]="$new_bead"
        append_receipt "$op_id" ok "$new_bead" false ""
    else
        append_receipt "$op_id" error "" false "br create failed"
    fi
}
```

## Close Op → br update + br close

```bash
process_close() {
    local op="$1"
    local op_id=$(jq -r '.op_id' <<< "$op")
    local target=$(jq -c '.target' <<< "$op")
    local bead_id=$(resolve_ref "$target")

    # Idempotency: already obsoleted?
    if br show "$bead_id" --json | jq -e '.labels | any(. == "spex:obsolete")' >/dev/null; then
        append_receipt "$op_id" skipped "$bead_id" false "already obsoleted"
        return
    fi

    # Add labels from op.labels
    while read -r lbl; do
        [[ -z "$lbl" ]] && continue
        br update "$bead_id" --add-label "$lbl" || {
            append_receipt "$op_id" error "$bead_id" false "add-label $lbl failed"
            return
        }
    done < <(jq -r '.labels[]' <<< "$op")

    # Close
    if br close "$bead_id"; then
        append_receipt "$op_id" ok "$bead_id" false ""
    else
        append_receipt "$op_id" error "$bead_id" false "br close failed"
    fi
}
```

## Spec-Kind → Bead-Type Mapping

```bash
spec_kind_to_bead_type() {
    case "$1" in
        proposal_epic) echo epic ;;
        component)     echo feature ;;
        data_flow)     echo task ;;
        test_section)  echo task ;;
        *) echo feature ;; # safe fallback
    esac
}
```

This mirrors the mapping that existed in the legacy BeadCreator — spex emit passes spec_node_kind; the adapter translates to br's bead type.

## Label Passthrough

The adapter adds the `spex:<record-id>` label from `idempotency.label` on create, plus `spex:obsolete` and `commit:<HEAD>` from `op.labels` on close. No other labels are synthesized in the reference implementation.

## br Version Check

Pre-flight:

```bash
if ! br --version >/dev/null 2>&1; then
    echo "error: br not on PATH or unresponsive" >&2
    exit 1
fi
```

Minimum version is pinned by a regex check if the adapter's `require_version` const is set. The reference impl does not pin a minimum — users who need version enforcement should fork and customize.

## Error Surface

Each br invocation's exit status is checked; non-zero → record error receipt with the invocation's stderr captured in the error field. The adapter continues processing remaining ops (does not abort on first error) so receipts.json reflects every attempted op.
