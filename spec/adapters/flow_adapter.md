# Adapter flow

[[7f2e76cecab3|The reference adapter]] touches a run twice: once before `spex plan`, to tell it what the tracker holds, and once after, to do what the changeset says.

## Export, before plan

`tasks.json` leaves on stdout or as `$1`, produced in one step: run `br list --json --all --limit 0`, keep every task whose status is `open` or `in_progress`, project each to `task_id` and `status`, and wrap the result as `{"version": 1, "tasks": [...]}`. Nothing else is read and nothing is written to the tracker. A listing with nothing in flight yields an empty `tasks` array. A failure of the listing exits 1 and writes no document — `spex plan` then refuses to run for want of its required `--tasks` input, which is the intended failure: a plan run without a fresh export would read every task as finished.

## Apply, after plan

`changeset.json` arrives on stdin or as `$1`, `receipts.json` leaves on stdout or as `$2`, and in between the script runs three phases:

1. **Pre-flight.** Parse the changeset as v4, confirm `br` answers `--version`, and start with an empty substitution table. A failure in this phase exits 1 and writes no receipts at all. The top-level `absorbed` array, when present, is not read past the parse — it is ingest's input.
2. **Iterate the ops, in the order the changeset lists them.** A create op asks the idempotency question before it resolves anything — `br list --json --all --limit 0 --label <op.idempotency.label>`, status-unfiltered because a match is exact-label in any status — and a match ends the op there with an `ok` receipt carrying `was_existing=true` and the existing task id, so its `parent` and `deps` refs are never resolved at all; a create carrying an unresolvable `deps` ref therefore returns `ok`, not `error`, when its label already matches a task. The receipt must be `ok`, not `skipped`: ingest constructs journal lines only for `ok` receipts, and the `was_existing=true` pairing is exactly what the crashed-adapter recovery path needs to land. Only on the no-match path does it resolve those refs and call `br create`. A close op runs the two the other way round: it resolves `target` first, then reads that task's current state with `br show` and branches on it. A retarget op resolves `target` and its `deps`, reads the task's current deps with `br show`, then runs `br update` once per event label and `br dep add` once per missing dep — the dep half leaves the update surface because `br update` has no dep flag — with no idempotency query at all, because both calls converge when applied twice. Any other `type` is an `error` receipt without a tracker call. Every op ends by appending its receipt entry, and a create op additionally records the task it reached in the substitution table, so a later op can name it by `op_id`; close and retarget ops never write to the table.
3. **Emit `receipts.json`.** Determine the top-level status from the entries collected, assemble the v2 wrapper around them, and write it in one atomic step.

Phase 2 is the only phase that can partially succeed, and the failures it checks for are recorded rather than raised — a malformed op, an unresolvable ref, a `br` invocation that exits non-zero all end that op with an `error` receipt and the loop carries on to phase 3. A narrow class escapes that, because it is never checked for: a few op shapes break one of the unguarded field reads the loop opens each iteration with, and the script runs under `set -euo pipefail`, so the run stops mid-phase-2 and phase 3 never happens. Nothing is written, and the caller reads the absent `receipts.json` the same way it reads a pre-flight refusal — `spex ingest` fails with `ingest: read receipts: ... no such file or directory` and exit 1, so the run must be re-run rather than ingested. Not every wrong shape does this: a malformed *ref* is read inside a resolver that goes on to return a sentinel, so the failed read never becomes the run's exit status — the op ends with an `error` receipt and the loop carries on. `arch_br_reference_adapter.md` names the shapes that do it and the ones that only look like they do.

## Subprocess Shape

The export forks `br list --json --all --limit 0` exactly once. The apply loop forks:

- `br list --json` once per create op (idempotency check). Could be optimized by caching a single initial listing, but the reference impl takes the simple path.
- `br show <id> --format json` once per close op (idempotency check) and once per retarget op (current-deps read).
- `br create` once per non-existing create.
- `br update --add-label` once per label attached: one per entry of a retarget's `labels` — creates and closes carry no `labels` to attach.
- `br dep add` once per dep a retarget's target does not already carry.
- `br close` once per open close target.

For a changeset with N creates (M fresh), K closes (J open) and R retargets (D missing deps across them), the adapter makes approximately `N + K + M + J + 2R + D` br invocations. On a 10-op changeset, ~10-20 invocations. Closes are fewer than they were: with the modify pair retired, a close appears only for live work being cancelled, never as a no-op against a finished task.

## Data Shapes

### Output: tasks.json (export)

The version-1 task-state document the schema module declares — `version`, `tasks[].{task_id, status}` — and nothing else; see `spec/schema/arch_task_state_schema.md`.

### Input: changeset.json (apply half)

See `spec/plan/arch_changeset_builder.md` for the full shape. Adapter consumes: `version`, `git_head`, `proposal`, `ops[].{op_id, type, spec_node_kind, spec_node_id, idempotency.label, parent, deps, priority, title, body, target, labels, reason}`. The top-level `absorbed` array is deliberately not in that list. Every ref is `{"ref":"op","op_id":…}` or `{"ref":"task","task_id":…}`, with no further key.

### Output: receipts.json (apply half)

```json
{
  "version": 2,
  "status": "complete",
  "ops": [
    {"op_id":"op-component-a1b2c3d4e5f6","status":"ok","task_id":"spexmachina-abc","was_existing":false},
    {"op_id":"op-component-b2c3d4e5f6a1","status":"ok","task_id":"spexmachina-def","was_existing":false},
    {"op_id":"op-component-c3d4e5f6a1b2","status":"ok","task_id":"spexmachina-ghi","was_existing":true},
    {"op_id":"op-component-d4e5f6a1b2c3","status":"error","task_id":"","was_existing":false,"error":"br create exited 1: invalid priority -1"}
  ]
}
```

## Error Paths

- Listing failure in the export → exit 1, no `tasks.json` written.
- Changeset parse error → exit 1, no receipts file written.
- br pre-flight fails → exit 1, no receipts file written.
- br operation errors mid-run → receipts.json gets top-level `partial` status; adapter continues processing remaining ops.
- Unresolvable ref (op_id not in SUB_TABLE) on an op that gets as far as resolving it → that op's receipt is `error`; subsequent ops still processed; top-level → `partial`.

## Success Paths

- Export over a tracker with work in flight → `tasks.json` listing it; with nothing in flight → `{"version": 1, "tasks": []}`; exit 0 either way.
- All ops ok or intentional-skipped → top-level `complete`; receipts.json written; exit 0.
- Empty changeset (no ops) → top-level `complete`, empty `ops` array, exit 0. (Well-formed edge case.)

## Integration with Ingest

The export half's output is plan's input, and the apply half's output is ingest's input. Both contracts are tight:

- `tasks.json` validates against the task-state schema; plan refuses it otherwise, exit 1.
- Every op in the changeset has exactly one receipt entry.
- Op ordering in receipts.ops mirrors changeset.ops.
- Status values align with ingest's expected transition table.

Ingest trusts these properties; violations cause ingest to exit 1 with pre-flight errors.
