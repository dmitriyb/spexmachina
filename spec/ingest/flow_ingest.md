# Ingest flow

`spex ingest` runs in one of two modes selected by the `--mode` flag (or
the proposal frontmatter the calling skill consumes). The dispatch happens
inside `IngestCommand`. Both modes terminate with the same atomicity
guarantee: snapshot and `.bead-map.json` move together or neither moves.

```
  changeset.json          receipts.json          --mode {normal|refresh}
       │                        │                        │
       └────────┬───────────────┴───────────────┬────────┘
                ▼                               ▼
          ┌──────────────────────────┐
          │ spex ingest               │
          │   --changeset             │
          │   --receipts              │
          │   [--mode refresh]        │
          └──────────┬───────────────┘
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
   mode: normal              mode: refresh
        │                         │
        ▼                         ▼
```

## Mode: normal (default)

```
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

## Mode: refresh

The refresh-mode pathway is for absorbing content drift on
non-bead-producing leaves without bead lifecycle. Typical caller: a
follow-up skill that reads a proposal with `mode: refresh` frontmatter and
runs `spex ingest --mode refresh` against an empty changeset/receipts pair.
See `arch_refresh.md` for the contract and `impl_refresh.md` for the steps.

```
   ┌──────────────────────────────────┐
   │ 1. Pre-flight                     │
   │   - confirm changeset, receipts    │
   │     are empty, version == 1        │
   │   - confirm spec/.snapshot.json    │
   │     exists                         │
   └──────────┬───────────────────────┘
              │
              ▼
   ┌──────────────────────────────────┐
   │ 2. Compute diff                   │
   │   - rebuild current tree (merkle  │
   │     Hasher + TreeBuilder)         │
   │   - load pre-refresh snapshot     │
   │   - run DiffEngine.Compare        │
   └──────────┬───────────────────────┘
              │
              ▼
   ┌──────────────────────────────────┐
   │ 3. Refusal gates                  │
   │   - any added entries  → refuse   │
   │   - any removed entries → refuse  │
   │   - any orphan record   → refuse  │
   │   (each refusal: structured error,│
   │    files unchanged)               │
   └──────────┬───────────────────────┘
              │
              ▼
   ┌──────────────────────────────────┐
   │ 4. Update stale spec_hash fields  │
   │   - for each bead-map record,     │
   │     compare recorded spec_hash to │
   │     current content hash; update  │
   │     in memory only.               │
   │   - other record fields untouched.│
   └──────────┬───────────────────────┘
              │
              ▼
   ┌──────────────────────────────────┐
   │ 5. Atomic commit                  │
   │   - write new .bead-map.json      │
   │   - write new spec/.snapshot.json │
   │   - both move together; failure   │
   │     of either rolls back the other│
   └──────────┬───────────────────────┘
              │
              ▼
       JSON summary (stdout)
```

## Data Shapes

### receipts.json input (mode: normal)

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

### receipts.json input (mode: refresh)

Always empty:

```json
{
  "version": 1,
  "status": "complete",
  "ops": []
}
```

The changeset.json passed to refresh mode is similarly empty (`"ops": []`).
A non-empty changeset or receipts file is a configuration error — see
`arch_refresh.md`.

### Summary output (mode: normal)

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

### Summary output (mode: refresh)

```json
{
  "records_updated": 2,
  "records_unchanged": 14,
  "snapshot_saved": true,
  "status": "complete"
}
```

Refresh has no per-op accounting (no ops). The `status` field is always
`complete` on success; failures return a structured error and no summary.

## Per-Op Transitions (mode: normal)

For the full transition table, see `arch_reconciler.md`. Summary:

- ok create / was_existing=false → insert record.
- ok create / was_existing=true → verify and no-op.
- ok close / reason="Spec node removed" → delete record.
- ok close / reason starts "Spec node modified" → no-op (paired with create).
- error / skipped → no-op.

## Per-Record Transitions (mode: refresh)

- record.spec_hash == current content hash → no change.
- record.spec_hash != current content hash → update record.spec_hash to
  current; all other fields unchanged.

## Invariant Check Placement (mode: normal)

Invariants are checked AFTER all ops are applied (to the in-memory copy),
BEFORE the atomic commit. This means a single bad op can block the whole
run from committing — all-or-nothing semantics at the commit level.

## Refusal Gates (mode: refresh)

Refusals are checked AFTER the diff is computed, BEFORE any record-level
update is attempted. This means a refusal returns the bead-map and snapshot
byte-identical to their pre-call state.

## Error Paths

### Both modes

- Malformed changeset → exit 1.
- Malformed receipts → exit 1.

### Mode: normal

- Missing or extra op_ids → exit 1.
- Invariant failure → exit 2, `.bead-map.json` on disk unchanged.
- Snapshot write failure → exit 1, `.bead-map.json` DID commit (snapshot
  is regenerable, mapping is not).

### Mode: refresh

- Missing pre-refresh snapshot → exit 1, files unchanged.
- Non-empty changeset or receipts → exit 1, files unchanged.
- Diff contains added or removed entries → exit non-zero (structured
  error), files unchanged.
- Orphan bead-map record → exit non-zero (structured error), files
  unchanged.
- Atomic-write failure of either file → exit 1, both files rolled back to
  pre-call state.

## Success Paths

### Mode: normal

- Complete run → exit 0, `.bead-map.json` updated, snapshot rewritten.
- Partial run → exit 0, `.bead-map.json` updated (only ok ops), snapshot
  unchanged.
- Re-run against same inputs → exit 0, idempotent (no changes).

### Mode: refresh

- Stale records present → exit 0, `.bead-map.json` updated for stale
  records, snapshot rewritten.
- No drift → exit 0, files unchanged (or rewritten byte-identically).
- Re-run after a successful refresh → exit 0, no records updated.
