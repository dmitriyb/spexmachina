# BrReferenceAdapter

Bash + jq script at `scripts/apply-br.sh`. Reads `changeset.json` on stdin (or `$1`), executes operations against the `br` CLI, writes `receipts.json` to stdout (or `$2`).

## Responsibilities

- Parse changeset v1.
- For each op in order:
  - Resolve ref fields (parent, deps, target) into concrete bead IDs via the substitution table + mapping store + literal passthrough.
  - Check idempotency before create/close.
  - Invoke the appropriate `br` subcommand with correct flags.
  - Write a receipt entry.
- Emit a top-level status field: `complete` if no op errored, `partial` otherwise.

## Interface

```
scripts/apply-br.sh [<changeset.json>] [<receipts.json>]

# Defaults: changeset on stdin, receipts on stdout.
```

## Dependencies

- `br` — any version that supports `create`, `list --json`, `show <id> --json`, `update`, `close`. Minimum version is enforced by a `br --version` check at script start.
- `jq` — 1.6+.
- Bash 4.0+ (for associative arrays used by the substitution table).

## Op Processing Loop

```bash
declare -A SUB_TABLE  # op_id → bead_id
RECEIPTS=()

jq -c '.ops[]' "$CHANGESET" | while read -r op; do
    op_id=$(jq -r '.op_id' <<< "$op")
    op_type=$(jq -r '.type' <<< "$op")

    case "$op_type" in
        create) process_create "$op" ;;
        close)  process_close  "$op" ;;
        label)  process_label  "$op" ;;
        tag)    process_tag    "$op" ;;
        *) fail_receipt "$op_id" "unknown op type $op_type" ;;
    esac
done

emit_receipts_json
```

## Substitution Table

A bash associative array keyed by op_id, populated after each successful create (and updated with `was_existing` lookups). See `impl_substitution_table.md`.

## Idempotency

Before `br create`, query `br list --json | jq ... 'label == $label'` for the op's `idempotency.label`. On match, skip create, record `was_existing=true` with the existing bead_id.

Before `br close`, query `br show $bead_id --json | jq '.labels | any(. == "spex:obsolete")'`. On match, skip close, record `status=skipped`.

## Receipt Atomicity

Each op's receipt is appended to an in-memory array. After the last op, the full receipts.json is written atomically via temp file + mv (analogous to emit's --out).

If the adapter crashes mid-run, partial receipts are lost unless a caller runs it with `--checkpoint`. (The reference adapter does NOT implement checkpointing — it's a reference, not a crash-safe production adapter. Users building production adapters add checkpointing as an enhancement.)

## Reference-Adapter Limitations

Explicitly documented:

- Single-threaded sequential processing. Concurrent invocations will trample receipts.
- No retry on transient `br` failures — user's responsibility to fix and re-run.
- No dry-run mode.
- No pre-flight check that the tracker state matches what emit assumed (e.g., existing bead statuses). Production adapters should add this.

## Header Notice

The script begins with:

```
#!/usr/bin/env bash
#
# apply-br.sh — Reference adapter consuming spex changeset.json v1 and invoking br.
#
# REFERENCE IMPLEMENTATION. Vet before production use. See spec/adapters/ for the
# adapter contract that any implementation (this one or your own) must satisfy.
#
# Usage: apply-br.sh [<changeset.json>] [<receipts.json>]
#
set -euo pipefail
```

## Test Hooks

- `BR_BIN` env var — overrides `br` binary path (for tests with a mock).
- `SPEX_MAPPING_FILE` env var — overrides `.bead-map.json` location (for tests with a synthetic mapping).

Both default to sensible real-world values.
