# IngestCommand

CLI entry point for `spex ingest`. Loads changeset and receipts, inspects the run mode, and
dispatches to either the normal-mode pathway (Reconciler + SnapshotSaver) or the refresh-mode
pathway (RefreshHandler). Emits a summary.

## Usage

```
spex ingest --changeset <file> --receipts <file> [--mode normal|refresh] [--git-head <sha>]
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--changeset` | yes | Path to changeset.json (the one produced by `spex plan`). For refresh mode, this is an empty changeset (`"ops": []`). |
| `--receipts`  | yes | Path to receipts.json (the one produced by the adapter). For refresh mode, this is an empty receipts file. |
| `--mode`      | no  | `normal` (default) or `refresh`. Selects the dispatch pathway. Any other value is an input error. |
| `--git-head`  | no  | Refresh mode only: the commit to stamp on the refresh receipt. Normal mode ignores it — the changeset already carries its own `git_head`. |

The root's persistent `--spec-dir` is read too: it is the tree the snapshot is computed from, and
where both `spec/.snapshot.json` and the journal `spec/.history.jsonl` live. The retired `--map`
flag is gone with the file it pointed at.

`--git-head` exists because refresh mode has no other source of provenance: a refresh runs on an
empty changeset, so no `git_head` field rides in for the refresh receipt to record. The command is
the only place that knows the operator's answer, and it passes the value through to the refresh
pathway untouched — it never invokes git to discover a commit itself, which keeps the
no-subprocess contract intact across both modes.

The module puts one external surface in the graph: [[3589714e50f8|the api `spex ingest`]], whose
`provided_by` names IngestCommand and whose declared name is the bare invocation string. That is
what makes `--mode refresh` a flag on the surface rather than a second surface — both pathways
answer to the same identity hash, and a rename of the subcommand moves them together. This is the
deliberate reading: refresh is an alternative reconciliation strategy for the same job, not a
separate entry point, and splitting it into its own api node would let the two halves of one
command drift apart in the graph.

## Output (mode: normal)

Writes a JSON summary to stdout:

```json
{
  "ok": 12,
  "skipped": 0,
  "errors": 0,
  "events_appended": 10,
  "receipts_appended": 12,
  "snapshot_saved": true,
  "status": "complete"
}
```

## Output (mode: refresh)

Writes a JSON summary to stdout:

```json
{
  "events_appended": 2,
  "snapshot_saved": true,
  "status": "complete"
}
```

Refresh has no per-op accounting; it reports what it appended.

## Exit codes

Exit code reflects the outcome:

- `0` — success.
- `1` — input error (bad flags, malformed JSON, op_id mismatch in normal mode, missing
  pre-refresh snapshot in refresh mode, non-empty changeset/receipts in refresh mode,
  atomic-write failure).
- `2` — invariant failure (normal mode) or refresh refusal (a *non-absorbable* added or removed
  entry, or a removed node whose journal pairing is still live). Not every structural entry
  refuses: refresh absorbs `requirement` and `api` entries in either direction, plus `component`
  removals. See RefreshHandler's absorbable set for the full table — declaring an api is the
  common case that would otherwise refuse for nothing.

## Wiring

Every run starts the same way and forks exactly once, on `--mode`:

1. Read and parse `changeset.json`, then `receipts.json`. Neither file's version envelope is
   checked until both are in hand, so a changeset carrying the wrong version alongside an
   unreadable receipts path reports the receipts read failure, not the version.
2. Run the pre-flight checks below over the loaded pair.
3. Open the journal at `<spec-dir>/.history.jsonl` — nothing is read from it yet — then read
   `--mode` and take one of two paths. Whichever path runs performs the first read itself.

**Normal.** Hand the changeset and receipts together to [[2b5158af774b|Reconciler]], which
constructs and appends the batch's journal lines atomically; then hand the receipts' top-level
status to [[f85bd2f94aeb|SnapshotSaver]], which writes `spec/.snapshot.json` only when that
status is `complete`.

**Refresh.** Hand the same journal and the two (empty) artifacts to
[[f9033352c13f|RefreshHandler]] and set it running against the spec directory: it runs its refusal
gates, constructs the absorbed drift's events and receipt, and commits the journal and the
snapshot together.

The command owns none of the three. It parses flags, loads two files, picks a path, and turns what
comes back into one JSON summary and one exit code.

## Pre-flight

### Both modes

- Both files must parse successfully as JSON with their declared versions — `version: 3` on the changeset, `version: 1` on the receipts.
- The changeset and the receipts must cover exactly the same set of op ids: every receipt's op
  present in the changeset, and every op receipted. In refresh mode both files are empty, so the
  check passes trivially.

### Mode: normal

- The op-id check above is what rejects a truncated receipts file. No partial receipts are
  allowed: the ADAPTER writes one receipt per op, so a missing receipt means it crashed before
  that op and wrote `partial` in the top-level status. That is an input error, not a partial run
  to reconcile — a genuine partial run still carries one receipt per op, the failed ones
  `error`.

### Mode: refresh

- Both changeset and receipts must have an empty `ops` array.
- `spec/.snapshot.json` must exist (refresh's diff baseline).

If any pre-flight check fails, exit 1 without touching the journal or snapshot.

## Transaction Order (mode: normal)

1. Reconciler commits the journal append.
2. SnapshotSaver writes the snapshot.

If Reconciler fails, the snapshot is not written (invariants failed — we don't want a snapshot
against an inconsistent journal).

If SnapshotSaver fails (unlikely: FS error), the journal has already been appended. This is
acceptable because:
- The journal reflects the adapter's actual work.
- The snapshot is regenerable from the spec tree alone — the next run recomputes it.
- The caller sees exit code 1 and the stderr message indicates snapshot write failure; they can
  re-run `spex ingest` — the re-run appends nothing (derived event ids) and completes the
  snapshot.

## Transaction Order (mode: refresh)

The journal and snapshot must move together — both writes are part of one atomic commit boundary.
If the snapshot write fails after the journal write, the journal write is rolled back so the pair
stays consistent. This is stricter than the normal-mode contract because the refreshed snapshot IS
the next run's diff baseline; a half-committed refresh would mean the next normal-mode run
computes against a stale baseline while the journal already moved forward.

## Composability

- Normal mode: `spex plan ... > changeset.json && adapter changeset.json > receipts.json &&
  spex ingest --changeset changeset.json --receipts receipts.json` is the full local pipeline.
- Refresh mode: `spex ingest --mode refresh --changeset empty-changeset.json --receipts
  empty-receipts.json` — the flag is the only activation path; nothing inside `spex` reads
  proposal frontmatter or any other side channel.
- No stdin on ingest because it needs two distinct files; stdin is a single stream.

## Non-Responsibilities

- Does NOT run plan or the adapter — ingest assumes both have run (normal mode) or that the
  author has chosen refresh mode (no adapter needed).
- Does NOT retry failed ops — that's the user's job via re-running plan→adapter→ingest (normal
  mode) or fixing the underlying drift and re-running refresh.
- Does NOT repair a journal it cannot read. Each line is checked against the journal-line schema
  on the way in as well as before every append, so a file that violates the schema ends the run
  with the file untouched. Which exit code that is depends on where the read sits: in normal mode
  the first read happens inside reconciliation, so the failure takes the invariant code `2`; in
  refresh mode it is an input error and exits `1`. An absent journal is not an error — it simply
  starts empty.
- [[20589ccf7072|Does NOT invoke git, a tracker, or any other subprocess in either mode]]. Both
  inputs and both file outputs are local files; besides them the run writes its JSON summary to
  stdout and, on failure, an error line to stderr.
