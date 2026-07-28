# IngestCommand

CLI entry point for `spex ingest`. Loads changeset and receipts, inspects
the run mode, and dispatches to either the normal-mode pathway (Reconciler
+ SnapshotSaver) or the refresh-mode pathway (RefreshHandler). Emits a
summary.

## Usage

```
spex ingest --changeset <file> --receipts <file> [--map <file>] [--mode normal|refresh]
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--changeset` | yes | Path to changeset.json (the one produced by `spex emit`). For refresh mode, this is an empty changeset (`"ops": []`). |
| `--receipts`  | yes | Path to receipts.json (the one produced by the adapter). For refresh mode, this is an empty receipts file. |
| `--map`       | no (`.bead-map.json`) | Path to the bead mapping store. A relative path resolves against the spec directory's parent, so the default finds the repository's own `.bead-map.json`. |
| `--mode`      | no  | `normal` (default) or `refresh`. Selects the dispatch pathway. Any other value is an input error. |

The root's persistent `--spec-dir` is read too: it is the tree the snapshot is computed from, and what a relative `--map` is resolved against.

The module puts one external surface in the graph: [[3589714e50f8|the api `spex ingest`]], whose `provided_by` names IngestCommand and whose declared name is the bare invocation string. That is what makes `--mode refresh` a flag on the surface rather than a second surface — both pathways answer to the same identity hash, and a rename of the subcommand moves them together. This is the deliberate reading: refresh is an alternative reconciliation strategy for the same job, not a separate entry point, and splitting it into its own api node would let the two halves of one command drift apart in the graph.

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

Every run starts the same way and forks exactly once, on `--mode`:

1. Read and parse `changeset.json`, then `receipts.json`. Neither file's version envelope is checked until both are in hand, so a changeset carrying the wrong version alongside an unreadable receipts path reports the receipts read failure, not the version.
2. Run the pre-flight checks below over the loaded pair.
3. Construct a mapping-store handle over the resolved `--map` path — nothing is read from `.bead-map.json` yet — then read `--mode` and take one of two paths. Whichever path runs performs the first read itself, which is why an unreadable bead-map surfaces differently in the two modes.

**Normal.** Hand the changeset and receipts together to [[2b5158af774b|Reconciler]], which applies the per-op transitions and commits `.bead-map.json` atomically; then hand the receipts' top-level status to [[f85bd2f94aeb|SnapshotSaver]], which writes `spec/.snapshot.json` only when that status is `complete`.

**Refresh.** Hand the same store and the two (empty) artifacts to [[f9033352c13f|RefreshHandler]] and set it running against the spec directory: it runs its refusal gates, updates stale `spec_hash` fields in memory, and commits the bead-map and the snapshot together.

The command owns none of the three. It parses flags, loads two files, picks a path, and turns what comes back into one JSON summary and one exit code.

## Pre-flight

### Both modes

- Both files must parse successfully as JSON with `version: 1`.
- The changeset and the receipts must cover exactly the same set of op ids:
  every receipt's op present in the changeset, and every op receipted. In
  refresh mode both files are empty, so the check passes trivially.

### Mode: normal

- The op-id check above is what rejects a truncated receipts file. No
  partial receipts are allowed: the ADAPTER writes one receipt per op, so a
  missing receipt means it crashed before that op and wrote `partial` in the
  top-level status. That is an input error, not a partial run to reconcile —
  a genuine partial run still carries one receipt per op, most of them
  `skipped` or `error`.

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
- Does NOT repair a `.bead-map.json` it cannot read. The store is checked
  against the bead-map schema on the way in as well as on the way out, so a
  file that violates the schema ends the run with the file untouched. Which
  exit code that is depends on where the read sits: in normal mode the first
  read happens inside reconciliation, so the failure takes the invariant code
  `2`; in refresh mode it is an input error and exits `1`. An absent file is
  not an error — the store simply starts empty.
- [[20589ccf7072|Does NOT invoke git, a tracker, or any other subprocess in
  either mode]]. Both inputs and both file outputs are local files; besides
  them the run writes its JSON summary to stdout and, on failure, an error
  line to stderr.
