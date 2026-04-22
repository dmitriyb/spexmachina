# IngestCommand

CLI entry point for `spex ingest`. Loads changeset and receipts, wires Reconciler and SnapshotSaver, emits a summary.

## Usage

```
spex ingest --changeset <file> --receipts <file>
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--changeset` | yes | Path to changeset.json (the one produced by `spex emit`). |
| `--receipts`  | yes | Path to receipts.json (the one produced by the adapter). |

## Output

Writes a JSON summary to stdout:

```json
{
  "ok": 12,
  "skipped": 0,
  "errors": 0,
  "records_added": 8,
  "records_updated": 2,
  "records_deleted": 2,
  "snapshot_saved": true,
  "status": "complete"
}
```

Exit code reflects the outcome:

- `0` — success (complete OR partial with no reconciler errors).
- `1` — input error (bad flags, malformed JSON, op_id mismatch).
- `2` — invariant failure.

## Wiring

```
IngestCommand
  ├─ load changeset.json (validate version == 1)
  ├─ load receipts.json (validate version == 1)
  ├─ pair ops with receipts by op_id
  ├─ Reconciler.Apply(changeset, receipts)
  │   └─ commits .bead-map.json (atomic)
  └─ SnapshotSaver.Save(receipts.Status)
      └─ writes spec/.snapshot.json iff complete
```

## Pre-flight

- Both files must parse successfully as JSON with `version: 1`.
- Op IDs in receipts must be a subset of op IDs in changeset (every receipt's op must exist in the changeset).
- Every op in changeset must have a receipt (no partial receipts allowed — the ADAPTER writes one receipt per op; a missing receipt means the adapter crashed before that op and wrote `partial` in the top-level status).

If any pre-flight check fails, exit 1 without touching the mapping store or snapshot.

## Transaction Order

1. Reconciler commits the mapping store.
2. SnapshotSaver writes the snapshot.

If Reconciler fails, snapshot is not written (invariants failed — we don't want a snapshot against inconsistent mapping state).

If SnapshotSaver fails (unlikely: FS error), the mapping store has already been committed. This is acceptable because:
- The mapping store reflects the adapter's actual work.
- The snapshot is regenerable from the spec tree alone — next run can recompute.
- The caller sees exit code 1 and the stderr message indicates snapshot write failure; they can re-run `spex ingest` or `spex emit`.

## Composability

- `spex emit ... > changeset.json && adapter changeset.json > receipts.json && spex ingest --changeset changeset.json --receipts receipts.json` is the full local pipeline.
- No stdin on ingest because it needs two distinct files; stdin is a single stream.

## Non-Responsibilities

- Does NOT run emit or the adapter — ingest assumes both have run.
- Does NOT retry failed ops — that's the user's job via re-running emit→adapter→ingest.
- Does NOT invoke git or any tracker CLI.
