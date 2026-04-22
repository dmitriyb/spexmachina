# Ingest command tests

End-to-end tests for `spex ingest`.

## Setup

- `cmd/spex/testdata/ingest/` holds paired fixtures: changeset, receipts, initial state, expected state.
- Tests invoke the binary via the standard harness.

## Scenarios

### Happy path: complete run

- `spex ingest --changeset testdata/full/changeset.json --receipts testdata/full/receipts.json`
- Expected: exit 0; .bead-map.json matches expected; spec/.snapshot.json rewritten; stdout is a JSON summary (`{"ok": N, "skipped": M, "error": 0, "snapshot_saved": true}`).

### Partial run: exit 0, snapshot untouched

- `spex ingest --changeset partial/changeset.json --receipts partial/receipts.json`
- Expected: exit 0; stdout summary has `snapshot_saved: false`; snapshot file unchanged on disk.

### Missing --changeset flag

- `spex ingest --receipts x.json`
- Expected: exit 1; stderr names the missing flag.

### Missing --receipts flag

- Same, opposite direction.

### Mismatched op_id references

- Receipts reference an op_id not in the changeset.
- Expected: exit 1; error `"ingest: receipt op_id <id> not in changeset"`.

### Missing op receipt

- Changeset has 5 ops; receipts has only 4 op entries (one missing).
- Expected: exit 1; error `"ingest: no receipt for op <id>"`.

### Schema violation in receipts.json

- Malformed JSON or missing required field.
- Expected: exit 1; decoder error with location.

### Invariant failure short-circuits

- Run with a deliberately-crafted pair that produces an orphan record after reconciliation. Inject via fixture that adds a stale record upstream.
- Expected: exit 2; stderr names the invariant and the offending spec_node_id. Mapping file on disk is UNCHANGED (atomic write means partial reconciliation doesn't land).

### Re-run idempotency

- Run ingest; run ingest again with the same inputs.
- Expected: second run exits 0; second run's summary matches first (same counts); .bead-map.json byte-identical; snapshot unchanged after second run.

### Dry-run variant (if supported)

- `spex ingest --changeset x --receipts y --dry-run`: prints the planned changes (JSON) without writing anything.
- Expected: exit 0; no file writes; stdout includes planned upserts and deletes.

## Fixtures

- `cmd/spex/testdata/ingest/full/` — happy path.
- `cmd/spex/testdata/ingest/partial/` — partial receipts.
- `cmd/spex/testdata/ingest/invariant_orphan/` — invariant violation.
