# Partial run recovery tests

Exercises the two-run sequence where the first ingest runs with partial receipts and the second
run completes the work.

## Setup

- Full pipeline fixture: a spec change that produces 3 new creates (A, B, C) and 1 close.
- Synthetic receipts for Run 1: A, B ok; C error. Top-level status: partial. Close ok.
- Synthetic receipts for Run 2: C ok. Top-level status: complete.

## Run 1 Scenario

1. `spex ingest --changeset changeset1.json --receipts receipts1.json`.
2. Expected:
   - The journal holds change events and `task_created` receipts for A and B, and the removed
     node's `removed` event with its `task_closed`.
   - No event or receipt for C — an error-status op appends nothing.
   - `spec/.snapshot.json` is unchanged (partial → not saved).
   - Exit code 0 (partial is not a failure, it's just a status to record).

## Run 2 Scenario

1. User re-runs the pipeline: diff → plan → adapter → ingest.
2. The new plan run folds the journal and drops A and B as already tracked — their open
   pairings' events record the same after hashes the unchanged leaves still carry — so the
   changeset carries only C's create.
   Its label is `spex:<git_head>:<C's op_id in this batch>` — a fresh derivation, and nothing
   requires it to match Run 1's: C's Run-1 create errored, so no task and no pairing exist for
   the old label to find. No counter, no reserved range, nothing to recompute.
3. Adapter creates C fresh, receipt ok, was_existing=false.
4. Ingest Run 2 appends C's change event and `task_created` receipt. Snapshot now saved.

### Assertions

- After Run 1: the journal pairs A and B with their tasks, records the close, and carries nothing
  for C; no snapshot write.
- After Run 2: the journal additionally pairs C; the snapshot is rewritten. The A and B lines are
  byte-identical to their Run-1 form — Run 2 re-derives their event ids, finds them present, and
  appends nothing for them.

## Partial with Adapter-Side Duplicates

Edge case: adapter, mid-run, died AFTER creating C in the tracker but BEFORE writing receipts —
which means no `receipts.json` at all, since receipts are written once after the last op, so
ingest refuses the run and the discipline is **re-run the adapter with the same changeset**,
never re-emit. The re-run derives nothing: the changeset's labels are byte-identical to the dead
run's, so the adapter's exact-match probe (any status) finds the task the dead run made and
responds `was_existing: true` — the correct idempotent match — for C, and for A and B alike.
Receipts land complete; ingest appends every pairing, C's carrying the pre-existing task id.

The same-changeset discipline is what the label guards; a *fresh* plan run would mint fresh labels
(different op ids), miss the orphaned task, and duplicate it — which is why an aborted adapter
run is re-run, not re-planned, exactly as the adapters module's receipts contract states.

### Assertion

Tested by mocking the adapter to return `was_existing: true` for C's receipt in the same-changeset
re-run. Expected: the pairing lands normally; no error; no duplicate task in the tracker.

## Snapshot Correctness

Invariant: after Run 2 (complete), `spec/.snapshot.json` corresponds EXACTLY to the spec tree
state at that moment. Nothing stale from Run 1.

Assertion: parse the written snapshot, compare merkle root hash to an independently-computed value
from the spec tree.

## Fixtures

Inline Go fixtures, no on-disk testdata. The shipped tests in
`ingest/partial_run_recovery_test.go` build the run1/run2 changesets and receipts (including the
adapter-duplicate variant) directly from the `plan` and `adapters` types, and reuse the package's
shared helpers: the fake spec graph and journal seeder from `reconciler_test.go`, and
`setupSpecDir` from `snapshot_saver_test.go`.
