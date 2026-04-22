# Ingest flow

```
  changeset.json          receipts.json
       │                        │
       └────────┬───────────────┘
                ▼
   ┌──────────────────────────┐
   │ spex ingest               │
   │   --changeset             │
   │   --receipts              │
   └──────────┬───────────────┘
              │
              ▼
   ┌──────────────────────────┐
   │ 1. Preflight              │
   │   - parse both files      │
   │   - version == 1 check    │
   │   - op_id set equality    │
   └──────────┬───────────────┘
              │
              ▼
   ┌──────────────────────────┐
   │ 2. Reconciler.Apply       │
   │   - clone mapping store   │
   │   - apply per-op transitions│
   │     (in-memory)           │
   │   - AssertInvariants      │
   │   - commit atomically     │
   └──────────┬───────────────┘
              │
              ▼
   ┌──────────────────────────┐
   │ 3. SnapshotSaver.Save     │
   │   - if status != complete │
   │     → skip, return false  │
   │   - else: build merkle    │
   │     tree and write atomically│
   └──────────┬───────────────┘
              │
              ▼
       JSON summary (stdout)
```

## Data Shapes

### receipts.json input

```json
{
  "version": 1,
  "status": "complete",
  "ops": [
    {
      "op_id": "op-0001",
      "status": "ok",
      "bead_id": "spexmachina-abc",
      "was_existing": false
    },
    {
      "op_id": "op-0002",
      "status": "skipped",
      "bead_id": "",
      "was_existing": false,
      "reason": "already labeled"
    },
    {
      "op_id": "op-0003",
      "status": "error",
      "bead_id": "",
      "error": "bead_cli exited 1: invalid priority"
    }
  ]
}
```

### Summary output

```json
{
  "ok": 10,
  "skipped": 1,
  "errors": 0,
  "records_added": 7,
  "records_updated": 2,
  "records_deleted": 1,
  "snapshot_saved": true,
  "status": "complete"
}
```

## Per-Op Transitions

For the full transition table, see `arch_reconciler.md`. Summary:

- ok create / was_existing=false → insert record.
- ok create / was_existing=true → verify and no-op.
- ok close / reason="Spec node removed" → delete record.
- ok close / reason starts "Spec node modified" → no-op (paired with create).
- error / skipped → no-op.

## Invariant Check Placement

Invariants are checked AFTER all ops are applied (to the in-memory copy), BEFORE the atomic commit. This means a single bad op can block the whole run from committing — all-or-nothing semantics at the commit level.

## Error Paths

- Malformed changeset → exit 1.
- Malformed receipts → exit 1.
- Missing or extra op_ids → exit 1.
- Invariant failure → exit 2, .bead-map.json on disk unchanged.
- Snapshot write failure → exit 1, .bead-map.json DID commit (snapshot is regenerable, mapping is not).

## Success Paths

- Complete run → exit 0, .bead-map.json updated, snapshot rewritten.
- Partial run → exit 0, .bead-map.json updated (only ok ops), snapshot unchanged.
- Re-run against same inputs → exit 0, idempotent (no changes).
