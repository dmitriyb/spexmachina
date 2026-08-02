# Ingest command tests

End-to-end tests for `spex ingest`.

## Setup

- In-code fixtures, no on-disk testdata: `cmd/spex/ingest_test.go` writes each changeset/receipts pair and the initial `.bead-map.json` into a `t.TempDir()`.
- Tests invoke the binary via the standard harness.

## Scenarios

### Happy path: complete run

- `spex ingest --changeset testdata/full/changeset.json --receipts testdata/full/receipts.json`
- Expected: exit 0; .bead-map.json matches expected; spec/.snapshot.json rewritten; stdout is a JSON summary (`{"ok": N, "skipped": M, "errors": 0, "records_added": …, "records_updated": …, "records_deleted": …, "snapshot_saved": true, "status": "complete"}`).

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

### Unknown --mode value

- `spex ingest --changeset <valid.json> --receipts <valid.json> --mode bogus`.
- Expected: exit 1; stderr is `ingest: --mode must be normal or refresh, got "bogus"`. The mode check runs after both artifacts load, so unreadable paths surface their own error first. There is no `--dry-run` flag.

## Fixtures

In-code fixtures, no on-disk testdata. `cmd/spex/ingest_test.go` writes each changeset/receipts pair and the initial `.bead-map.json` into a `t.TempDir()` per scenario: the happy path at `:146`, the partial run at `:196`, and the invariant violation at `:383`.
