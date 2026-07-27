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

## The only tracker caller

The adapter is the sole participant in the pipeline that executes a bead CLI. `spex diff`, `impact`, `emit` and `ingest` read and write files only; every `br` invocation in the whole flow happens inside this script. That is what the boundary buys: the changeset names spec nodes, ops and refs, and the adapter is where those become `br create`, `br update`, `br close` and their flags.

Two consequences follow, and both are contract rather than convention:

- **Every tracker-specific decision belongs here.** Bead-type vocabulary, flag spelling, dep-edge syntax, the `--force` on close, the exit-code quirks — none of it may leak upward into emit. An adapter for a different tracker replaces this file and nothing else.
- **Every non-tracker decision belongs upstream.** The adapter does not reorder ops, choose priorities, allocate record ids, or decide what work exists. It receives those already settled and executes them in file order. An adapter that started making structural decisions would make the run non-deterministic, since nothing upstream could reproduce it.

## Op Translation

The adapter maps changeset op fields to `br` subcommand flags. Two mappings deserve explicit contract documentation because they differ from the obvious component-only path:

### `spec_node_kind` → `br --type`

| `spec_node_kind` | `br --type` |
|------------------|-------------|
| `proposal_epic`  | `epic`      |
| `component`      | `feature`   |
| `data_flow`      | `task`      |
| `test_section`   | `task`      |
| `cleanup`        | `task`      |
| (other / unset)  | `feature` (default) |

The `cleanup → task` mapping carries forward pre-decouple `apply/bead_creator.go::createCleanupBead`'s explicit `Type: "task"` choice; cleanup beads are bookkeeping/maintenance work, not features.

### `op.Labels` → `br create --add-label`

The adapter MUST pass each entry of `op.Labels` as a `--add-label <label>` flag on `br create`, in the order they appear in the changeset. Currently the adapter applies `Labels` only on close ops (and on `update` for label/tag ops); cleanup creates are the first create-op consumer of this pre-existing field. Without this, the cleanup discriminator label `spex:cleanup` never reaches the tracker, and cleanup beads become indistinguishable from ordinary work in any tracker-side query.

The `idempotency.label` is applied separately (the adapter checks for an existing bead with that label before creating, then sets it on the new bead via the standard `--add-label` path or via a follow-up `br update` per the script's existing flow). This is independent of `op.Labels`; cleanup creates carry both `idempotency.label = "spex:cleanup-<spec_node_id>"` AND `Labels = ["spex:cleanup"]`, and both end up on the bead.

## Idempotency

Before `br create`, query `br list --json --label <op.idempotency.label>` for the op's idempotency label. **A match is a bead with the label AND `status == "open"`.** Closed beads carrying the same label are historical artifacts (a previous bead-lifecycle landed and the bead was closed; the label persisted across close); they MUST NOT count as a match for create-idempotency. On a true match (open bead with the label), skip create, record `was_existing=true` with the existing bead_id.

The open-only filter mirrors pre-decouple `apply/bead_creator.go::FindExisting`, which iterated tracker results and returned only beads with `Status == "open"`. The filter matters specifically for modify-pair creates: per the modify-pair record-id-reuse rule (see `spec/emit/arch_idempotency_labeler.md`), the new create's `idempotency.label` deliberately equals the existing record's id. The OLD bead in the tracker (now closed) carries that same label as a historical marker. Without the open filter, the adapter would treat the closed historical bead as a match and skip the create — leaving the bead-map record pointing at the closed bead and the new arch contract unrepresented in the tracker. Reconciler invariant 3 (modified-pair records point to new bead_id) would then fail.

The label format depends on the action class — emit produces `spex:<n>` for fresh and modify-pair creates, `spex:cleanup-<spec_node_id>` for cleanup creates; the adapter just looks up whatever label the op carries. The open-only filter applies uniformly across formats: a closed bead carrying ANY `spex:*` label is historical; only an open bead represents current state. Cleanup beads are unaffected because cleanup-bead labels (`spex:cleanup-<spec_node_id>`) are unique per removed-spec-node identity hash, only one bead ever carries them, and during the only window when re-runs occur (retrying a failed pipeline run against the same proposal) the cleanup bead is still open — actually doing the cleanup work and closing that bead is a separate user activity that does not happen between failing pipeline attempts.

Before `br close`, query `br show <bead_id> --format json` ONCE and read both `labels` and `status` from the returned JSON. Branch on the combined state:

| Pre-state                                  | Action                                                       | Receipt |
|--------------------------------------------|--------------------------------------------------------------|---------|
| `labels` contain `spex:obsolete`           | Skip — already obsoleted in a prior run.                     | `status=skipped`, reason `"already obsoleted"` |
| `status == "closed"` (no `spex:obsolete`)  | Apply labels via `br update --add-label …` only; do NOT call `br close` (it exits 3 on already-closed targets). | `status=ok` |
| `status == "open"` (no `spex:obsolete`)    | Apply labels via `br update --add-label …`, then `br close --force --reason …`. | `status=ok` |

Pre-decouple's binary split this into two phases (`LabelObsoletes` then `CloseBeads` with `cli.Status` check between them). Post-decouple, the adapter does the same status check internally; the contract from emit is unchanged (one close op per obsoleted bead with labels and reason). The status branch is purely adapter-internal.

The label-only branch matters because `br close` exits 3 on already-closed targets even though the labels DO get applied. Treating that exit as `status=error` would push top-level receipts to `partial` and block ingest from saving the snapshot — every modify-pair close on a previously-shipped bead would fail this way.

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
