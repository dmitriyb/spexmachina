# RefreshHandler

Refresh-mode ingest pathway. Implements the `Refresh mode for impl_only
drift` requirement (id `e68653819f38`). Separated from `Reconciler` so the
bead-lifecycle rules and the drift-absorption rules live in distinct
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
requirements, impl_sections and apis in either direction, plus component
*removals*. Declaring a requirement, splitting an impl section, or adding
an api creates nothing in the tracker, so baselining it costs nothing.

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

- An updated `.bead-map.json` where every record's `spec_hash` matches the
  current content hash for its `spec_node_id`. Other record fields untouched.
- A rewritten `spec/.snapshot.json` matching the current spec state.
- A JSON summary on stdout describing how many records were touched.

Both file writes go through a temp-file + rename sequence under one
atomic commit boundary: either both files move to the new state or neither
does. This preserves the invariant that snapshot and bead-map always
represent the same point in time.

## Refusal contract

The handler refuses (returns a structured error, leaves both files
unchanged) if any of the following is true:

| Condition | Reason |
|-----------|--------|
| The diff contains an `added` entry whose node type is not absorbable in the added direction | Structural change; requires the normal pipeline so bead lifecycle runs. |
| The diff contains a `removed` entry whose node type is not absorbable in the removed direction | Structural change; requires the normal pipeline. |
| Any bead-map record's `spec_node_id` is not present in the current spec graph | Orphan record. The user must resolve the structural drift via the normal pipeline. |
| `spec/.snapshot.json` does not exist | Refresh's diff baseline is the snapshot; without one, every leaf looks added (the bootstrap case requires the normal pipeline). |
| The changeset or receipts file is non-empty | Configuration error; refresh has no per-op transitions. |

`modified` entries are never gated. Only additions and removals reach the
structural gate.

### The absorbable set

| Node type | `added` | `removed` |
|-----------|---------|-----------|
| `requirement`   | absorbed | absorbed |
| `impl_section`  | absorbed | absorbed |
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

The diff is computed via the merkle module's existing DiffEngine + ImpactClassifier path — RefreshHandler does not reimplement classification. It reads the `added` and `removed` entries, filters each against the absorbable set by node type and direction, and refuses on whatever survives the filter.

## What refresh does NOT change

- Record `bead_id` fields stay identical.
- Record `status` fields stay identical (open/closed transitions are bead lifecycle, which refresh does not touch).
- Mapping store's monotonic record-id counter does not advance (no new records).
- Tracker state (the bead system itself) is untouched. `spex` does not
  invoke a tracker subprocess in refresh mode any more than it does in
  normal mode.

## Wiring

```
IngestCommand
  ├─ load changeset.json
  ├─ load receipts.json
  ├─ inspect --mode flag
  │
  ├─ if mode == normal:
  │     wire Reconciler + SnapshotSaver (existing path, unchanged)
  │
  └─ if mode == refresh:
        wire RefreshHandler
        RefreshHandler reads:
          ├─ current spec directory
          ├─ current .bead-map.json
          └─ current spec/.snapshot.json (diff baseline)
        RefreshHandler:
          ├─ rebuild current merkle tree (Hasher + TreeBuilder)
          ├─ DiffEngine vs pre-refresh snapshot
          ├─ refuse on non-absorbable added/removed entries
          ├─ refuse on orphan records
          ├─ for each record with stale spec_hash: update in memory
          ├─ atomically commit: write new .bead-map.json AND new snapshot
          └─ emit summary
```

## Relationship to existing components

- Uses `merkle.HashFile` (or the equivalent leaf-hash helper) to compute
  current content hashes. Not a new hashing primitive.
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
- Refusal (non-absorbable added/removed, or orphan) → exit non-zero with a
  structured stderr message naming the specific entries; both files unchanged.
- Atomic-write failure mid-commit → exit 1, the temp file is removed and
  both target files are unchanged. The handler MUST NOT leave one file
  updated and the other stale.

## Composability

`spex ingest --mode refresh` reads file paths from flags and writes a JSON
summary to stdout. Callers (the future skill that consumes
`mode: refresh` proposal frontmatter) compose it the same way they compose
the normal-mode call.
