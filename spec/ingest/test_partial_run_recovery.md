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

1. User re-runs the pipeline: impact → emit → adapter → ingest.
2. The new emit folds the journal, sees A and B already paired, and produces only C's create.
   The label is `spex:<C's spec_node_id>` — the same label both runs, by construction, because
   the label is the node's own hash: no counter, no reserved range, nothing to recompute.
3. Adapter creates C fresh, receipt ok, was_existing=false.
4. Ingest Run 2 appends C's change event and `task_created` receipt. Snapshot now saved.

### Assertions

- After Run 1: the journal pairs A and B with their tasks, records the close, and carries nothing
  for C; no snapshot write.
- After Run 2: the journal additionally pairs C; the snapshot is rewritten. The A and B lines are
  byte-identical to their Run-1 form — Run 2 re-derives their event ids, finds them present, and
  appends nothing for them.

## Partial with Adapter-Side Duplicates

Edge case: adapter, mid-run, died AFTER creating C in the tracker but BEFORE writing C's receipt.
Run 2's emit still sees C as unpaired (no task-bearing event in the fold). Emit labels C's create
`spex:<C's hash>` — necessarily the same label as the dead run used. The adapter finds a task with
that label already in the tracker, responds `was_existing: true` — the correct idempotent match.
Ingest Run 2 appends C's event and pairs it with the pre-existing task id.

### Assertion

Tested by mocking the adapter to return `was_existing: true` for C's receipt in Run 2. Expected:
the pairing lands normally; no error; no duplicate task in the tracker.

## Snapshot Correctness

Invariant: after Run 2 (complete), `spec/.snapshot.json` corresponds EXACTLY to the spec tree
state at that moment. Nothing stale from Run 1.

Assertion: parse the written snapshot, compare merkle root hash to an independently-computed value
from the spec tree.

## Fixtures

Inline Go fixtures, no on-disk testdata. The shipped tests in
`ingest/partial_run_recovery_test.go` build the run1/run2 changesets and receipts (including the
adapter-duplicate variant) directly from the `emit` and `adapters` types, and reuse the package's
shared helpers: the fake spec graph and journal seeder from `reconciler_test.go`, and
`setupSpecDir` from `snapshot_saver_test.go`.
