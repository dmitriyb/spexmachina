# Ingest command tests

End-to-end tests for `spex ingest`.

## Setup

- In-code fixtures, no on-disk testdata: `cmd/spex/ingest_test.go` writes each changeset/receipts
  pair and the initial journal (at the fixture project's resolved location) into a `t.TempDir()`.
- Tests invoke the binary via the standard harness.

## Scenarios

### Happy path: complete run

**Given** a `t.TempDir()` holding a complete changeset/receipts pair and the initial journal at the fixture project's resolved location.

- `spex ingest --changeset changeset.json --receipts receipts.json`
- Expected: exit 0; the journal gains the batch's change events and receipts; the snapshot
  rewritten; stdout is a JSON summary (`{"ok": N, "skipped": M, "errors": 0,
  "events_appended": …, "receipts_appended": …, "snapshot_saved": true, "status": "complete"}`).

### Partial run: exit 0, snapshot untouched

**Given** a `t.TempDir()` holding the initial journal, a snapshot, and the partial-status pair `partial/changeset.json` and `partial/receipts.json`.

- `spex ingest --changeset partial/changeset.json --receipts partial/receipts.json`
- Expected: exit 0; stdout summary has `snapshot_saved: false`; snapshot file unchanged on disk;
  the ok ops' events are appended (the journal may legitimately run ahead of the snapshot until
  the completing re-run).

### Missing --changeset flag

**Given** a `t.TempDir()` project holding the receipts file `x.json` and no `--changeset` value on the command line.

- `spex ingest --receipts x.json`
- Expected: exit 1; stderr names the missing flag.

### Missing --receipts flag

**Given** the same `t.TempDir()` project holding a changeset file and no `--receipts` value on the command line.

- Same, opposite direction.

### Mismatched op_id references

**Given** a receipts document referencing an op_id not in the changeset.

- Expected: exit 1; error `"ingest: receipt op_id <id> not in changeset"`.

### Missing op receipt

**Given** a changeset with 5 ops and a receipts document holding only 4 op entries, one missing.

- Expected: exit 1; error `"ingest: no receipt for op <id>"`.

### Schema violation in receipts.json

**Given** a `receipts.json` that is malformed JSON or missing a required field.

- Expected: exit 1; decoder error with location.

### Invariant failure short-circuits

**Given** a deliberately-crafted changeset/receipts pair whose ok create pairs with no referent — no change event, no prior removed event, no proposal stem.

- Expected: exit 2; stderr names the invariant and the offending op. The journal on disk is
  UNCHANGED (the append is a whole-file write-and-rename; a refused batch lands nothing).

### Re-run idempotency

**Given** a `t.TempDir()` project on which ingest has already run once, and the same inputs again.

- Run ingest; run ingest again with the same inputs.
- Expected: second run exits 0 with the same summary counts; the journal byte-identical
  after the second run; snapshot unchanged.

### Pre-flight refusal in both modes

**Given** two starting states — a resolved location with no project state, and a project whose journal carries a line violating the journal-line schema.

- With no project state at the resolved location: exit is the not-a-spex-project code, stderr
  names `spex init`, neither pathway runs.
- With a journal line that violates the journal-line schema: same code in normal and refresh
  mode alike, stderr names `spex doctor`, the journal is untouched — the lifecycle pre-flight
  reads both state files before `--mode` forks, so neither mode's own reader is reached.

### A v3 pair is refused at the version envelope

**Given** a changeset carrying `"version": 3` with a dep in the v3 task-ref spelling and a receipts file carrying `"version": 1`.

- `spex ingest --changeset <v3.json> --receipts <v1.json>` — a changeset carrying `"version": 3`
  with a dep in the v3 task-ref spelling, receipts carrying `"version": 1`.
- Expected: exit 1 from the command's pre-flight naming the version, before either mode runs;
  nothing appended, snapshot untouched. The retired shapes are refused, never adapted — the
  pre-flight requires `version: 4` on the changeset and `version: 2` on the receipts.

### Unknown --mode value

**Given** a valid changeset and a valid receipts file in the `t.TempDir()` project, invoked with `--mode bogus`.

- `spex ingest --changeset <valid.json> --receipts <valid.json> --mode bogus`.
- Expected: exit 1; stderr is `ingest: --mode must be normal or refresh, got "bogus"`. The mode
  check runs after both artifacts load, so unreadable paths surface their own error first. There
  is no `--dry-run` flag.

## Fixtures

In-code fixtures, no on-disk testdata. `cmd/spex/ingest_test.go` writes each changeset/receipts
pair and the initial journal into a `t.TempDir()` per scenario.
