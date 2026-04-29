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

### Proposal-epic create → record materialised without spec-graph lookup

- Changeset: one create op with `spec_node_kind: "proposal_epic"`, `spec_node_id: "2026-04-29-decouple-contract-gaps"` (the proposal stem), `idempotency.label: "spex:77"`, `title: "Proposal: 2026-04-29-decouple-contract-gaps"`.
- Receipts: `status: "ok"`, `bead_id: "br-epic"`, `was_existing: false`.
- Spec graph: empty (the proposal stem is NOT a spec-graph node by design).
- Expected: record `{id: 77, spec_node_id: "2026-04-29-decouple-contract-gaps", bead_id: "br-epic", bead_type: "epic", node_type: "proposal", component: "2026-04-29-decouple-contract-gaps", module: "", content_file: "", spec_hash: ""}`. The reconciler MUST NOT call `SpecGraph.NodeMetadata` for this op — that lookup would fail because the proposal stem is not in the identity-hash keyspace; the bug it prevents is the "spec graph: no node <stem>" error that aborts ingest with exit 2.

### Cleanup create → no mapping record

- Changeset: one create op with `spec_node_kind: "cleanup"`, `spec_node_id: "abc123def456"` (the identity hash of the now-removed spec node), `idempotency.label: "spex:cleanup-abc123def456"`, `title: "Code cleanup: m/X"`, `labels: ["spex:cleanup"]`, `deps: [{ref:bead, bead_id:"spexmachina-old", type:"blocks"}]`, `priority: 3`.
- Receipts: `status: "ok"`, `bead_id: "br-cleanup"`, `was_existing: false`.
- Initial counter: 50.
- Expected: `.bead-map.json` UNCHANGED — no record materialised for the cleanup bead. The counter stays at 50 (the cleanup label uses the spec-node-id form, not the cursor; no allocation happened). `RecordsAdded` in the summary is 0; `OkCreates` is 1 (the op was processed successfully — it just didn't produce a record by design). Invariant 1 ("every ok create has a record") MUST be amended to exempt cleanup creates and MUST NOT fire on this op.

### Invariant exemptions: proposal and cleanup records vs. invariant 4 (orphans)

- Initial map: `{id: 60, spec_node_id: "2026-04-18-decouple-spex-from-br", node_type: "proposal", ...}` (a pre-existing proposal-epic record; its spec_node_id is a proposal stem, NOT in `SpecGraph`).
- Changeset: any benign change (e.g., a no-op).
- Expected: `Reconciler.Apply` does NOT report invariant 4 (orphan) for this record. The check short-circuits on `record.NodeType == "proposal"` because proposal stems will never resolve through `SpecGraph.HasNode`. Without this exemption every run would falsely flag the proposal record as orphan and abort.
- Cleanup records (per the Cleanup-create scenario above) do NOT exist by construction, so invariant 4 trivially passes for them — the exemption is built into the fact that no record was materialised.

## Counter Advance

After each successful create op, the mapping store's `next_record_id` counter advances to max(existing + 1).

- Initial counter 100. Three ok creates with labels `spex:100`, `spex:101`, `spex:102`. Expected counter after ingest: 103.
- Two creates ok, one error. Counter: 102 (only committed labels count).

## Fixtures

Under `ingest/testdata/reconciliation/`:

- `changeset_*.json`, `receipts_*.json`, `initial_bead_map_*.json`, `expected_bead_map_*.json` — one triple per scenario above.
