# Adapter flow

`changeset.json` arrives on stdin or as `$1`, `receipts.json` leaves on stdout or as `$2`, and in between [[7f2e76cecab3|the reference adapter]] runs three phases:

1. **Pre-flight.** Parse the changeset as v3, confirm `br` answers `--version`, and start with an empty substitution table. A failure in this phase exits 1 and writes no receipts at all. The top-level `absorbed` array, when present, is not read past the parse — it is ingest's input.
2. **Iterate the ops, in the order the changeset lists them.** A create op asks the idempotency question before it resolves anything — `br list --json --all --limit 0 --label <op.idempotency.label>`, status-unfiltered because a match is exact-label in any status — and a match ends the op there with an `ok` receipt carrying `was_existing=true` and the existing bead id, so its `parent` and `deps` refs are never resolved at all; a create carrying an unresolvable `deps` ref therefore returns `ok`, not `error`, when its label already matches a bead. The receipt must be `ok`, not `skipped`: ingest constructs journal lines only for `ok` receipts, and the `was_existing=true` pairing is exactly what the crashed-adapter recovery path needs to land. Only on the no-match path does it resolve those refs and call `br create`. A close op runs the two the other way round: it resolves `target` first, then reads that bead's current state with `br show` and branches on it. A retarget op resolves `target` and its `deps`, reads the bead's current deps with `br show`, then runs `br update` once per event label and `br dep add` once per missing dep — the dep half leaves the update surface because `br update` has no dep flag — with no idempotency query at all, because both calls converge when applied twice. A `label` or `tag` op resolves `target` and calls `br update`, likewise unprobed. Every op ends by appending its receipt entry, and a create op additionally records the bead it reached in the substitution table, so a later op can name it by `op_id`; close, retarget and label ops never write to the table.
3. **Emit `receipts.json`.** Determine the top-level status from the entries collected, assemble the v1 wrapper around them, and write it in one atomic step.

Phase 2 is the only phase that can partially succeed, and the failures it checks for are recorded rather than raised — a malformed op, an unresolvable ref, a `br` invocation that exits non-zero all end that op with an `error` receipt and the loop carries on to phase 3. A narrow class escapes that, because it is never checked for: a few op shapes break one of the unguarded field reads the loop opens each iteration with, and the script runs under `set -euo pipefail`, so the run stops mid-phase-2 and phase 3 never happens. Nothing is written, and the caller reads the absent `receipts.json` the same way it reads a pre-flight refusal — `spex ingest` fails with `ingest: read receipts: ... no such file or directory` and exit 1, so the run must be re-run rather than ingested. Not every wrong shape does this: a malformed *ref* is read inside a resolver that goes on to return a sentinel, so the failed read never becomes the run's exit status — the op ends with an `error` receipt and the loop carries on. `arch_br_reference_adapter.md` names the shapes that do it and the ones that only look like they do.

## Subprocess Shape

The adapter forks:

- `br list --json` once per create op (idempotency check). Could be optimized by caching a single initial listing, but the reference impl takes the simple path.
- `br show <id> --format json` once per close op (idempotency check) and once per retarget op (current-deps read).
- `br create` once per non-existing create.
- `br update --add-label` once per label attached: one per entry of a create op's `labels`, one per label on a close or retarget, and one per label on a `label` or `tag` op.
- `br dep add` once per dep a retarget's target does not already carry.
- `br close` once per non-existing close.

For a changeset with N creates (M fresh), K closes (J fresh) and R retargets (D missing deps across them), the adapter makes approximately `N + K + M + 2J + 2R + D` br invocations. On a 10-op changeset, ~10-20 invocations.

## Data Shapes

### Input: changeset.json

See `spec/plan/arch_changeset_builder.md` for the full shape. Adapter consumes: `version`, `git_head`, `proposal`, `ops[].{op_id, type, spec_node_kind, spec_node_id, idempotency.label, parent, deps, priority, title, body, target, labels, reason}`. The top-level `absorbed` array is deliberately not in that list.

### Output: receipts.json

```json
{
  "version": 1,
  "status": "complete",
  "ops": [
    {"op_id":"op-0001","status":"ok","bead_id":"spexmachina-abc","was_existing":false},
    {"op_id":"op-0002","status":"ok","bead_id":"spexmachina-def","was_existing":false},
    {"op_id":"op-0003","status":"ok","bead_id":"spexmachina-ghi","was_existing":true},
    {"op_id":"op-0004","status":"error","bead_id":"","was_existing":false,"error":"br create exited 1: invalid priority -1"}
  ]
}
```

## Error Paths

- Changeset parse error → exit 1, no receipts file written.
- br pre-flight fails → exit 1, no receipts file written.
- br operation errors mid-run → receipts.json gets top-level `partial` status; adapter continues processing remaining ops.
- Unresolvable ref (op_id not in SUB_TABLE) on an op that gets as far as resolving it → that op's receipt is `error`; subsequent ops still processed; top-level → `partial`.

## Success Paths

- All ops ok or intentional-skipped → top-level `complete`; receipts.json written; exit 0.
- Empty changeset (no ops) → top-level `complete`, empty `ops` array, exit 0. (Well-formed edge case.)

## Integration with Ingest

The adapter's output is ingest's input. The contract is tight:

- Every op in the changeset has exactly one receipt entry.
- Op ordering in receipts.ops mirrors changeset.ops.
- Status values align with ingest's expected transition table.

Ingest trusts these properties; violations cause ingest to exit 1 with pre-flight errors.
