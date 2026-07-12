# Partial run recovery tests

Exercises the two-run sequence where the first ingest runs with partial receipts and the second run completes the work.

## Setup

- Full pipeline fixture: a spec change that produces 3 new creates (A, B, C) and 1 close.
- Synthetic receipts for Run 1: A, B ok; C error. Top-level status: partial. Close ok.
- Synthetic receipts for Run 2: C ok (fresh label reservation). Top-level status: complete.

## Run 1 Scenario

1. `spex ingest --changeset changeset1.json --receipts receipts1.json`.
2. Expected:
   - Mapping store has records for A and B (bead_ids from receipts).
   - No record for C.
   - Removed node's record is deleted (close ok).
   - `spec/.snapshot.json` is unchanged (partial → not saved).
   - Exit code 0 (partial is not a failure, it's just a status to record).

## Run 2 Scenario

1. User re-runs the pipeline: impact → emit → adapter → ingest.
2. The new emit sees A and B in the mapping store (already reconciled) and only C as still needing creation. Emit reserves label `spex:<N>` where N is C's label from run 1's reserved range (since the counter never advanced past A,B in run 1).

   Wait: let's be precise. In run 1, the counter was reserved A=42, B=43, C=44. A and B committed → counter persisted at 44. In run 2, emit sees C still missing → new create reserved at `spex:44`.

3. Adapter creates C fresh, receipt ok, was_existing=false.
4. Ingest Run 2: record for C inserted at id 44 → bead_id from receipt. Snapshot now saved.

### Assertions

- After Run 1: records `{42: A→brA}, {43: B→brB}`; counter = 44; no snapshot write.
- After Run 2: records add `{44: C→brC}`; counter = 45; snapshot rewritten.

## Partial with Adapter-Side Duplicates

Edge case: adapter, mid-run, died AFTER creating C in the tracker but BEFORE writing C's receipt. Run 2's emit still sees C as "not yet created" (no mapping record). Emit reserves `spex:44` for C again. Adapter runs, sees a bead with label `spex:44` already exists in the tracker (from the dead run 1), responds with `was_existing: true` — correct idempotent match. Ingest Run 2 commits C.

### Assertion

Tested by mocking the adapter to return `was_existing: true` for C's receipt in Run 2. Expected: record created normally; no error.

## Snapshot Correctness

Invariant: after Run 2 (complete), `spec/.snapshot.json` corresponds EXACTLY to the spec tree state at that moment. Nothing stale from Run 1.

Assertion: parse the written snapshot, compare merkle root hash to an independently-computed value from the spec tree.

## Fixtures

Inline Go fixtures, no on-disk testdata. The shipped tests in
`ingest/partial_run_recovery_test.go` build the run1/run2 changesets and
receipts (including the adapter-duplicate variant) directly from the
`emit` and `adapters` types, and reuse the package's shared helpers:
`newFakeSpecGraph` and `newTestStore` from `reconciler_test.go`, `idem`,
and `setupSpecDir` from `snapshot_saver_test.go`.
