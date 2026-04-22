# Reconciliation tests

Integration tests for `Reconciler.Apply` against fixture changesets and receipts.

## Setup

- `testdata/` contains paired `changeset_*.json` + `receipts_*.json` + `expected_bead_map_*.json` fixtures.
- Tests load an initial `.bead-map.json`, run reconciliation, assert the resulting file matches expected.

## Scenarios

### Ok create → record inserted

- Changeset: one create op, spec_node_id `abc123def456`, idempotency label `spex:42`.
- Receipts: corresponding op receipt with `status: "ok"`, `bead_id: "br-new"`, `was_existing: false`.
- Expected: `.bead-map.json` gains record `{id: 42, spec_node_id: "abc123def456", bead_id: "br-new", ...}`.

### Ok close on removed → record deleted

- Initial map has record for spec_node `xyz` → bead `br-old`.
- Changeset: one close op targeting bead `br-old`, `reason` starting with "Spec node removed".
- Receipts: op receipt `status: "ok"`.
- Expected: record for `xyz` is gone.

### Modified node: close+create → record updated

- Initial map has record `{id: 10, spec_node_id: "mod1", bead_id: "br-old"}`.
- Changeset: close op for `br-old`, then create op for `mod1` with label `spex:10` (reused — same record-id).
- Receipts: both ok; create's `bead_id: "br-new"`, `was_existing: false`.
- Expected: record with id 10 points to `br-new`, spec_node_id unchanged.

### Was_existing=true → idempotent no-op

- Changeset: create op for spec_node `A`, label `spex:7`.
- Initial map has record `{id: 7, spec_node_id: "A", bead_id: "br-7"}`.
- Receipts: create op `status: "ok"`, `bead_id: "br-7"`, `was_existing: true`.
- Expected: record unchanged (same bead_id, same spec_hash, etc.). No error.

### Error status → op skipped, no mapping change

- Changeset: one create op.
- Receipts: that op with `status: "error"`, `bead_id: ""`.
- Expected: no record inserted; no error from reconciler (the adapter's failure is the user's problem, ingest records truth).

### Skipped status → op no-op

- Changeset: create op.
- Receipts: op with `status: "skipped"` (adapter chose not to run it; typical for a label-add op in an edge case).
- Expected: no mapping change.

### Mixed ops: careful ordering

- Changeset: [close br-A (modified lineage), create for spec_node X with label `spex:5` replacing A, close br-B (removed), create for new spec_node Y].
- Receipts: all ok.
- Expected final map: record for X has bead_id from the new-create receipt; no record for B's spec_node; new record for Y.

## Counter Advance

After each successful create op, the mapping store's `next_record_id` counter advances to max(existing + 1).

- Initial counter 100. Three ok creates with labels `spex:100`, `spex:101`, `spex:102`. Expected counter after ingest: 103.
- Two creates ok, one error. Counter: 102 (only committed labels count).

## Fixtures

Under `ingest/testdata/reconciliation/`:

- `changeset_*.json`, `receipts_*.json`, `initial_bead_map_*.json`, `expected_bead_map_*.json` — one triple per scenario above.
