# RefreshHandler

Refresh-mode ingest pathway: [[e68653819f38|it absorbs spec drift that owes
no bead work, moving the bead-map's content hashes and the snapshot forward
together without any bead lifecycle running]]. Separated from `Reconciler` so
the bead-lifecycle rules and the drift-absorption rules live in distinct
components with distinct test surfaces — and so an external caller can
inspect the run-mode dispatch by looking at `IngestCommand.uses` rather than
chasing a flag through Reconciler internals.

## Purpose

`spex ingest` currently has one job: take the changeset+receipts pair from a
normal-mode run, reconcile bead-map records per receipt, and save the
snapshot iff the run is complete. Anything else that touches the snapshot or
the bead-map breaks the snapshot+bead-map atomicity invariant (snapshot and
bead-map must always represent the same point-in-time spec state).

There is a class of legitimate change that this rule made impossible: drift
that owes no bead work. Two kinds qualify.

**Content edits to any leaf** where the work scope hasn't changed — an
author corrects spec prose to match shipped code, rewrites an impl note
that was wrong, or cleans up a section that has gone out of date. Content
modifications are never gated, whatever leaf they land on.

**Structural additions and removals of node types that produce no bead** —
requirements and apis in either direction, plus component *removals*.
Declaring a requirement or adding an api creates nothing in the tracker, so
baselining it costs nothing.

Each of these leaves the bead-map's records semantically valid — same
beads, same scope — but with stale `spec_hash` fields. Without refresh the
user must either run the normal pipeline (which produces obsolete-then-create
beads for already-shipped work) or skip ingest entirely (which leaves the
snapshot stale relative to current content).

`RefreshHandler` is the pathway that absorbs this drift in a single
atomic step.

## Inputs

- An empty changeset and an empty receipts file. Both are required so the
  caller's pipeline harness (skill-side) treats refresh as a peer of normal
  mode without needing a separate code path; the IngestCommand inspects the
  `--mode refresh` flag and routes the call.
- The current spec directory (project.json + module.json files + content
  leaves).
- The current `.bead-map.json`.
- The current `spec/.snapshot.json` (the diff baseline; required).

## Outputs

- An updated `.bead-map.json`. Every record except a proposal epic's ends
  carrying the current content hash for its `spec_node_id`, and no record's
  other fields move. Proposal-epic records are passed over whatever their
  `spec_hash` says, because a proposal stem has no content leaf behind it to
  hash — the same exemption the orphan gate below makes.
- A rewritten `spec/.snapshot.json` matching the current spec state, whenever
  anything moved at all (see the no-op case below).
- A JSON summary on stdout describing how many records were touched.

Both file writes go through a temp-file + rename sequence under one
atomic commit boundary: either both files move to the new state or neither
does. This preserves the invariant that snapshot and bead-map always
represent the same point in time.

A run that finds nothing to absorb — no record's hash stale and no diff entry
of any kind — writes neither file and reports `snapshot_saved: false`. That is
what lets a second refresh over an unchanged tree leave `git status` clean
instead of rewriting two files byte-for-byte.

The summary's status is `complete` on every successful run; refresh has no
partial state. A refused or failed run prints no summary at all, only a
structured error.

## Refusal contract

The handler refuses (returns a structured error, leaves both files
unchanged) if any of the following is true:

| Condition | Reason |
|-----------|--------|
| The diff contains an `added` entry whose node type is not absorbable in the added direction | Structural change; requires the normal pipeline so bead lifecycle runs. |
| The diff contains a `removed` entry whose node type is not absorbable in the removed direction | Structural change; requires the normal pipeline. |
| Any bead-map record's `spec_node_id` is not present in the current spec graph | Orphan record. The user must resolve the structural drift via the normal pipeline. Records whose `node_type` is `proposal` are exempt: their `spec_node_id` is a proposal stem rather than a node hash, so it never resolves — the same exemption the Reconciler's orphan invariant makes. |
| `spec/.snapshot.json` does not exist | Refresh's diff baseline is the snapshot; without one, every leaf looks added (the bootstrap case requires the normal pipeline). |
| The changeset or receipts file is non-empty | Configuration error; refresh has no per-op transitions. |

`modified` entries are never gated. Only additions and removals reach the
structural gate.

### The absorbable set

| Node type | `added` | `removed` |
|-----------|---------|-----------|
| `requirement`   | absorbed | absorbed |
| `api`           | absorbed | absorbed |
| `component`     | **refused** | absorbed |
| `data_flow`, `test_section`, `meta`, `module` | refused | refused |

The set is written out explicitly rather than derived by negating impact's
bead-producing types. That negation would also admit `meta` — the
`project.json` / `module.json` envelope leaf — and refresh runs neither
`spex validate` nor the completeness checker. It would happily baseline a
project-requirement addition that `spex diff` rejects with exit 2 and a
module-requirement removal that `spex validate` rejects with exit 1, hiding
both from every downstream tool.

`component` is removal-only, and the asymmetry is the point. A removed
component's bead already exists, and absorbing the node's disappearance is
safe only because the orphan gate below still demands the bead-map record be
retired first. An *added* component is a bead that was never created;
baselining it into the snapshot would remove it from `spex diff` permanently,
which is precisely the bead lifecycle refresh must not bypass. `component`
is on the list at all only because retiring a spec component whose code is
gone is a structural removal with no bead work left to do.

The diff comes from the merkle module's existing diff path, which RefreshHandler does not reimplement: every entry already arrives carrying the node type of the leaf that moved, so the filter reads that field, matches it against the absorbable set in the entry's own direction, and refuses on whatever survives. No impact level is consulted — absorbability is decided by node type and direction alone, not by how the impact classifier would tier the change.

## What refresh does NOT change

- Record `bead_id` fields stay identical.
- Record `status` fields stay identical (open/closed transitions are bead lifecycle, which refresh does not touch).
- Mapping store's monotonic record-id counter does not advance (no new records).
- Tracker state (the bead system itself) is untouched. `spex` does not
  invoke a tracker subprocess in refresh mode any more than it does in
  normal mode.

## Wiring

`IngestCommand` loads both artifacts, reads `--mode`, and on `refresh` wires
this handler over the current spec directory, the current `.bead-map.json`
and the current `spec/.snapshot.json` — the last of which is both the diff
baseline and the rewrite target. Normal mode wires Reconciler and
SnapshotSaver instead and is untouched by any of this.

Once wired, the handler runs in one order:

1. Rebuild the merkle tree for the spec directory as it stands now.
2. Diff that tree against the pre-refresh snapshot.
3. Refuse on any added or removed entry the absorbable set does not cover.
4. Refuse on any orphan mapping record.
5. Update each stale `spec_hash` in memory, leaving every other field alone.
6. Commit the bead-map and the snapshot together, atomically.
7. Write the summary to stdout.

## Relationship to existing components

- Reads its content hashes straight off the merkle tree it just built, so
  every hash it writes into a record is the same digest `spex diff` would
  compute for that leaf. It introduces no hashing primitive of its own.
- Uses the same atomic-write helper SnapshotSaver uses. The atomicity
  guarantee is the same primitive; refresh just commits both writes
  together.
- Does not extend `Reconciler`. Reconciler's surface remains
  per-op-transition + invariant-assertion; the refresh-mode rules are
  fundamentally different (no ops, no invariants beyond the snapshot+map
  pairing) and would only complicate Reconciler's tests.

## Failure modes

- IO error reading current files → exit 1, both files unchanged.
- Diff computation fails → exit 1, both files unchanged.
- Refusal (non-absorbable added/removed, or orphan) → exit 2 with a
  structured stderr message naming the specific entries; both files unchanged.
- Atomic-write failure mid-commit → exit 1, the temp file is removed and
  both target files are unchanged. The handler MUST NOT leave one file
  updated and the other stale.

## Composability

`spex ingest --mode refresh` reads file paths from flags and writes a JSON
summary to stdout. Callers compose it the same way they compose the
normal-mode call — including the not-yet-shipped skill that would read
`mode: refresh` off a proposal's frontmatter and turn it into this flag.
Nothing inside `spex` reads that frontmatter; the flag is the only activation
path.
