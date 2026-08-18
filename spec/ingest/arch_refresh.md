# RefreshHandler

Refresh-mode ingest pathway: [[e68653819f38|it absorbs spec drift that owes no bead work, moving
the journal and the snapshot forward together without any bead lifecycle running]]. Separated from
`Reconciler` so the bead-lifecycle rules and the drift-absorption rules live in distinct
components with distinct test surfaces — and so an external caller can inspect the run-mode
dispatch by looking at `IngestCommand.uses` rather than chasing a flag through Reconciler
internals.

## Purpose

Normal-mode ingest takes the changeset+receipts pair, appends the batch's journal lines per
receipt, and saves the snapshot iff the run is complete. Anything else that touches the snapshot
or the journal breaks the snapshot+journal atomicity invariant (both must always represent the
same point-in-time spec state).

There is a class of legitimate change that rule would otherwise make impossible: drift that owes
no bead work. Two kinds qualify.

**Content edits to any leaf** where the work scope hasn't changed — an author corrects spec prose
to match shipped code, rewrites a stale paragraph, or clarifies a contract. Content modifications
are never gated, whatever leaf they land on.

**Structural additions and removals of node types that produce no bead** — requirements and apis
in either direction, plus component *removals*. Declaring a requirement or adding an api creates
nothing in the tracker, so baselining it costs nothing.

Without refresh the user must either run the normal pipeline (which produces obsolete-then-create
beads for already-shipped work) or skip ingest entirely (which leaves the snapshot stale relative
to current content). `RefreshHandler` absorbs this drift in a single atomic step — and unlike the
retired record-rewrite semantics, the absorption is itself recorded: the run appends what it
absorbed, so a refresh is visible in history instead of amnesiac.

## Inputs

- An empty changeset and an empty receipts file. Both are required so the caller's pipeline
  harness treats refresh as a peer of normal mode; IngestCommand inspects the `--mode refresh`
  flag and routes the call.
- The current spec directory (project.json + module.json files + content leaves).
- The current `spec/.history.jsonl`.
- The current `spec/.snapshot.json` (the diff baseline; required).
- Optionally `--git-head`; the receipt records the value when given and its absence otherwise.

## Outputs

- An extended `spec/.history.jsonl`: one change event per absorbed drift entry — `modified` events
  carrying before/after hashes for content drift, `added`/`removed` events for the absorbable
  structural set (their `node_type` spans the absorbable kinds, `requirement` and `api` included) —
  closed by one `refresh` receipt whose `absorbed` list names exactly those event ids. A
  refresh-born event has no op behind it, so its `eid` derives from `(node, before, after)` — the
  drift itself — rather than from `(git_head, op_id)`. Nothing already in the journal is altered.
- A rewritten `spec/.snapshot.json` matching the current spec state, whenever anything moved at
  all (see the no-op case below).
- A JSON summary on stdout describing how many events were appended.

Both file writes go through a temp-file + rename sequence under one atomic commit boundary: either
both files move to the new state or neither does.

### Recurring drift and eid collisions

`deriveRefreshEID` is a pure function of `(node, before, after)`, so the same drift can legitimately
recur: a node removed and later re-added with byte-identical content derives the same `added` eid
both times, and a leaf whose content flaps between two states derives the same `modified` eid every
time it revisits a transition it has visited before. Before constructing a diff entry's event,
refresh checks whether the node's *latest* journaled change event already records this exact state
— an `added`/`removed` entry against the latest event's kind, a `modified` entry against the latest
event's kind **and** its before/after pair. When it does, there is nothing new to say (the drift was
already absorbed by an earlier, possibly partial, run and only the snapshot is stale) and refresh
constructs nothing for that entry. Every other diff entry is genuinely new information — the node
came back, or the transition recurred after an intervening one — and is always constructed, even
when its derived eid collides with an earlier, non-adjacent occurrence already in the journal. On
collision the id is disambiguated with a `#2`, `#3`, ... suffix (`refresh:<node>:<before>:<after>#2`)
rather than reused, so every `eid` in the journal stays unique while the recurrence itself is still
recorded.

A run that finds nothing to absorb — no diff entry of any kind — writes neither file, appends
nothing (no empty receipt), and reports `snapshot_saved: false`. That is what lets a second
refresh over an unchanged tree leave `git status` clean.

The summary's status is `complete` on every successful run; refresh has no partial state. A
refused or failed run prints no summary at all, only a structured error.

## Refusal contract

The handler refuses (returns a structured error, leaves both files unchanged) if any of the
following is true:

| Condition | Reason |
|-----------|--------|
| The diff contains an `added` entry whose node type is not absorbable in the added direction | Structural change; requires the normal pipeline so bead lifecycle runs. |
| The diff contains a `removed` entry whose node type is not absorbable in the removed direction | Structural change; requires the normal pipeline. |
| A `removed` entry's node still has a live task pairing in the journal fold — a `task_created` with no matching `task_closed` | Open work points at the vanishing node. The normal pipeline owes a close or a cleanup; absorbing the removal would leave the tracker lying about live work. |
| `spec/.snapshot.json` does not exist | Refresh's diff baseline is the snapshot; without one, every leaf looks added (the bootstrap case requires the normal pipeline). |
| The changeset or receipts file is non-empty | Configuration error; refresh has no per-op transitions. |

`modified` entries are never gated. Only additions and removals reach the structural gate.

### The absorbable set

| Node type | `added` | `removed` |
|-----------|---------|-----------|
| `requirement`   | absorbed | absorbed |
| `api`           | absorbed | absorbed |
| `component`     | **refused** | absorbed |
| `data_flow`, `test_section`, `meta`, `module` | refused | refused |

The set is written out explicitly rather than derived by negating plan's bead-producing types.
That negation would also admit `meta` — the `project.json` / `module.json` envelope leaf — and
refresh runs neither `spex validate` nor the completeness checker: baselining a meta addition or
removal would hide a whole module appearing or vanishing from every downstream tool.

`component` is removal-only, and the asymmetry is the point. A removed component's task already
exists, and absorbing the node's disappearance is safe only because the live-pairing gate above
still demands the task be closed first. An *added* component is a bead that was never created;
baselining it into the snapshot would remove it from `spex diff` permanently, which is precisely
the bead lifecycle refresh must not bypass. `component` is on the list at all only because
retiring a spec component whose code is gone is a structural removal with no bead work left to do.

The diff comes from the merkle module's existing diff path, which RefreshHandler does not
reimplement: every entry already arrives carrying the node type of the leaf that moved, so the
filter reads that field, matches it against the absorbable set in the entry's own direction, and
refuses on whatever survives. No impact level is consulted — absorbability is decided by node type
and direction alone.

## What refresh does NOT change

- Existing journal lines: appends only, never edits or deletes.
- Task pairings: no receipt of kind `task_created` or `task_closed` is ever born from a refresh.
- Tracker state: `spex` does not invoke a tracker subprocess in refresh mode any more than in
  normal mode.

## Wiring

`IngestCommand` loads both artifacts, reads `--mode`, and on `refresh` wires this handler over the
current spec directory, journal and snapshot — the last of which is both the diff baseline and the
rewrite target. Normal mode wires Reconciler and SnapshotSaver instead and is untouched by any of
this.

Once wired, the handler runs in one order:

1. Rebuild the merkle tree for the spec directory as it stands now.
2. Diff that tree against the pre-refresh snapshot.
3. Refuse on any added or removed entry the absorbable set does not cover.
4. Refuse on any removed node whose journal pairing is still live.
5. Construct one change event per absorbed entry, plus the refresh receipt naming them.
6. Commit the journal and the snapshot together, atomically.
7. Write the summary to stdout.

## Relationship to existing components

- Reads its content hashes straight off the merkle tree it just built, so every hash it writes
  into an event is the same digest `spex diff` would compute for that leaf. It introduces no
  hashing primitive of its own.
- Uses the same atomic-write helper SnapshotSaver uses; refresh just commits both writes together.
- Does not extend `Reconciler`. Reconciler's surface remains per-op construction plus invariant
  assertion; the refresh-mode rules are fundamentally different (no ops, no receipts from a
  tracker) and would only complicate Reconciler's tests.

## Failure modes

- IO error reading current files → exit 1, both files unchanged.
- Diff computation fails → exit 1, both files unchanged.
- Refusal (non-absorbable added/removed, or live pairing on a removed node) → exit 2 with a
  structured stderr message naming the specific entries; both files unchanged.
- Atomic-write failure mid-commit → exit 1, the temp file is removed and both target files are
  unchanged. The handler MUST NOT leave one file updated and the other stale.

## Composability

`spex ingest --mode refresh` reads file paths from flags and writes a JSON summary to stdout.
Callers compose it the same way they compose the normal-mode call. Nothing inside `spex` reads
proposal frontmatter or any other side channel; the flag is the only activation path.
