# IngestCommand

CLI entry point for `spex ingest`. Loads changeset and receipts, inspects
the run mode, and dispatches to either the normal-mode pathway (Reconciler
+ SnapshotSaver) or the refresh-mode pathway (RefreshHandler). Emits a
summary.

## Usage

```
spex ingest --changeset <file> --receipts <file> [--mode normal|refresh]
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--changeset` | yes | Path to changeset.json (the one produced by `spex emit`). For refresh mode, this is an empty changeset (`"ops": []`). |
| `--receipts`  | yes | Path to receipts.json (the one produced by the adapter). For refresh mode, this is an empty receipts file. |
| `--mode`      | no  | `normal` (default) or `refresh`. Selects the dispatch pathway. |

The module declares one external surface, the api `spex ingest`, with `provided_by` naming IngestCommand. `--mode refresh` is a flag on that surface, not a second one: the declared name is the invocation string alone, so both pathways answer to the same identity hash and a rename of the subcommand would move both together. This is the deliberate reading — refresh is an alternative reconciliation strategy for the same job, not a separate entry point, and splitting it into its own api node would let the two halves of one command drift apart in the graph.

## Output (mode: normal)

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

## Output (mode: refresh)

Writes a JSON summary to stdout:

```json
{
  "records_updated": 2,
  "records_unchanged": 14,
  "snapshot_saved": true,
  "status": "complete"
}
```

Refresh has no per-op accounting; it reports record counts only.

## Exit codes

Exit code reflects the outcome:

- `0` — success.
- `1` — input error (bad flags, malformed JSON, op_id mismatch in normal
  mode, missing pre-refresh snapshot in refresh mode, non-empty
  changeset/receipts in refresh mode, atomic-write failure).
- `2` — invariant failure (normal mode only) or refresh refusal (a
  *non-absorbable* added or removed entry, orphan record). Not every
  structural entry refuses: refresh absorbs `requirement`, `impl_section`
  and `api` entries in either direction, plus `component` removals. See
  RefreshHandler's absorbable set for the full table — declaring an api
  is the common case that would otherwise refuse for nothing.

## Wiring

```
IngestCommand
  ├─ load changeset.json (validate version == 1)
  ├─ load receipts.json (validate version == 1)
  ├─ inspect --mode flag
  │
  ├─ if mode == normal (default):
  │     ├─ pair ops with receipts by op_id
  │     ├─ Reconciler.Apply(changeset, receipts)
  │     │   └─ commits .bead-map.json (atomic)
  │     └─ SnapshotSaver.Save(receipts.Status)
  │         └─ writes spec/.snapshot.json iff complete
  │
  └─ if mode == refresh:
        └─ RefreshHandler.Apply(specDir)
            ├─ refusal gates (added/removed/orphan)
            ├─ update stale record.spec_hash in memory
            └─ atomic commit: .bead-map.json AND spec/.snapshot.json
```

## Pre-flight

### Both modes

- Both files must parse successfully as JSON with `version: 1`.

### Mode: normal

- Op IDs in receipts must be a subset of op IDs in changeset (every
  receipt's op must exist in the changeset).
- Every op in changeset must have a receipt (no partial receipts allowed —
  the ADAPTER writes one receipt per op; a missing receipt means the
  adapter crashed before that op and wrote `partial` in the top-level
  status).

### Mode: refresh

- Both changeset and receipts must have an empty `ops` array.
- `spec/.snapshot.json` must exist (refresh's diff baseline).

If any pre-flight check fails, exit 1 without touching the mapping store
or snapshot.

## Transaction Order (mode: normal)

1. Reconciler commits the mapping store.
2. SnapshotSaver writes the snapshot.

If Reconciler fails, snapshot is not written (invariants failed — we don't
want a snapshot against inconsistent mapping state).

If SnapshotSaver fails (unlikely: FS error), the mapping store has already
been committed. This is acceptable because:
- The mapping store reflects the adapter's actual work.
- The snapshot is regenerable from the spec tree alone — next run can
  recompute.
- The caller sees exit code 1 and the stderr message indicates snapshot
  write failure; they can re-run `spex ingest` or `spex emit`.

## Transaction Order (mode: refresh)

The mapping store and snapshot must move together — both writes are part
of one atomic commit boundary. If the snapshot write fails after the
mapping write, the mapping write is rolled back so the pair stays
consistent. This is stricter than the normal-mode contract because the
refreshed snapshot IS the next run's diff baseline; a half-committed
refresh would mean the next normal-mode run computes against a stale
baseline and the bead-map already moved forward.

## Composability

- Normal mode: `spex emit ... > changeset.json && adapter changeset.json
  > receipts.json && spex ingest --changeset changeset.json --receipts
  receipts.json` is the full local pipeline.
- Refresh mode: `spex ingest --mode refresh --changeset
  empty-changeset.json --receipts empty-receipts.json` is invoked by a
  follow-up skill (not yet shipped) that consumes proposal frontmatter
  `mode: refresh`.
- No stdin on ingest because it needs two distinct files; stdin is a
  single stream.

## Non-Responsibilities

- Does NOT run emit or the adapter — ingest assumes both have run (normal
  mode) or that the proposal author has chosen refresh mode (no adapter
  needed).
- Does NOT retry failed ops — that's the user's job via re-running
  emit→adapter→ingest (normal mode) or fixing the underlying drift and
  re-running refresh.
- Does NOT invoke git or any tracker CLI in either mode.
