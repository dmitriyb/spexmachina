# Refresh mode tests

Module integration tests for the `RefreshHandler` component (id
`f9033352c13f`). Refresh mode is the parallel ingest pathway for absorbing
content drift on non-bead-producing leaves without triggering bead lifecycle.
This test_section covers the RefreshHandler's contract end-to-end through
`spex ingest --mode refresh`, exercised against a real fixture spec tree, a
real `.bead-map.json`, and a real `spec/.snapshot.json`. Per-method unit
tests for any internal helpers belong in `ingest/refresh_test.go` and are
bundled with this component's implementation bead.

## Setup

- `tmpdir/` containing a complete fixture spec (project.json + module.json
  files + content leaves) and a pre-seeded `.bead-map.json` with at least
  one record per node-kind that produces beads (component, data_flow,
  multi-component test_section).
- A pre-seeded `spec/.snapshot.json` matching some earlier state of the
  fixture spec.
- An empty changeset and an empty receipts file written to
  `tmpdir/refresh-changeset.json` and `tmpdir/refresh-receipts.json`
  respectively. Both have `version: 1` and an empty `ops` array; receipts
  has top-level `status: "complete"`.
- Helper `runIngest(args ...string) (stdout, stderr string, exitCode int)`
  wraps `IngestCommand.Run`.

The "pre-existing snapshot" is the diff baseline. RefreshHandler reads it,
computes a fresh tree from current files, and uses the diff between them to
decide whether the run is safely a refresh.

## Scenarios

### Refresh-only diff is accepted; spec_hash updates

**Given** the fixture has been edited so that `alpha/impl_widget_logic.md`
and `beta/test_handler.md` are modified relative to the snapshot, and no
other content has changed
**And** the pre-seeded `.bead-map.json` has records for those two nodes with
`spec_hash` matching the snapshot (i.e., stale relative to current content)
**When** `runIngest("--mode", "refresh", "--changeset", "...", "--receipts", "...")` is called
**Then** exit code is 0
**And** the two records' `spec_hash` fields now match the current content
hashes
**And** every other record's fields are unchanged byte-for-byte
**And** `spec/.snapshot.json` is rewritten to match the current spec state

**Rationale**: The headline behavior — drift is absorbed for
non-bead-producing modifications without bead churn.

### Refresh refuses on diff with `added` entries

**Given** the fixture has been edited to introduce a new **bead-producing**
content leaf (e.g., a new component `alpha/arch_new_thing.md` referenced from
`alpha/module.json`) so the diff contains a non-absorbable `added` entry
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr contains a structured error naming the added entries and the
reason ("refresh mode does not absorb structural changes; use the normal
pipeline")
**And** `.bead-map.json` is unchanged byte-for-byte
**And** `spec/.snapshot.json` is unchanged byte-for-byte

**Rationale**: Structural changes that owe bead work must go through the
normal Reconciler path so bead lifecycle runs. Refresh mode's job is to NOT
trigger bead lifecycle — it must refuse anything that would normally produce
beads. The fixture must add a `component`, `data_flow` or `test_section`; an
added `requirement` or `api` is absorbed and would not refuse.

### Refresh refuses on diff with `removed` entries

**Given** the fixture has been edited to delete an existing **non-absorbable**
content leaf (e.g., a data_flow `beta/flow_service.md` removed from
`beta/module.json`) so the diff contains a non-absorbable `removed` entry
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr contains a structured error naming the removed entries and
the same "use the normal pipeline" reason
**And** `.bead-map.json` and `spec/.snapshot.json` are unchanged

**Rationale**: Same as the added-entry case. The removal direction is
narrower than the addition direction: `component` removal IS absorbed, so the
fixture must remove a `data_flow` or a `test_section` to exercise the gate.

### Refresh absorbs the absorbable structural set

**Given** the fixture has been edited to add a requirement, add and remove an
api, and remove a component whose bead-map record has already been retired
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is 0
**And** the summary reports `status: "complete"`
**And** `spec/.snapshot.json` is rewritten to match the current spec state

**Rationale**: The refusal gate is a filter on node type and direction, not a
blanket ban on `added`/`removed`. `requirement` and `api` are
absorbed in both directions and `component` in the removed direction, because
none of those transitions owes any bead work. A test that only exercises the
refusal side would pass against an implementation that refused everything —
which is the contract this spec previously described and the code never had.
The component removal is paired with a retired record deliberately: the orphan
gate, not the structural gate, is what keeps a component removal honest.

### Refresh refuses on bead-map record with no live spec node

**Given** the fixture's current spec graph does NOT contain a node for the
identity hash referenced by some `.bead-map.json` record (`spec_node_id`
does not resolve)
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr contains a structured error naming the orphan record's
`spec_node_id` and `bead_id` and the reason ("orphan mapping record;
structural drift requires the normal pipeline")
**And** `.bead-map.json` and `spec/.snapshot.json` are unchanged

**Rationale**: An orphan bead-map record signals structural drift the user
must reconcile via the normal pipeline. Refresh mode would silently leave
the record in place pointing at a deleted node — a state we want surfaced,
not absorbed.

### Refresh on no-change spec is a clean no-op

**Given** the fixture is byte-identical to the snapshot (no content has
changed since the snapshot was written)
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is 0
**And** `.bead-map.json` is unchanged byte-for-byte (no record's `spec_hash`
needed updating)
**And** `spec/.snapshot.json` is unchanged byte-for-byte (or, equivalently,
rewritten with byte-identical content)
**And** stdout indicates zero records updated

**Rationale**: Refresh is safe to run on a clean spec — it should be a no-op
rather than an error or a snapshot rewrite that perturbs git status.

### Re-running refresh on the same state is a no-op

**Given** a previous refresh run completed successfully and updated some
records
**When** `runIngest("--mode", "refresh", ...)` is called again immediately,
without any spec edits between runs
**Then** exit code is 0
**And** zero records' `spec_hash` fields change (every record now matches
the current content hash)
**And** `.bead-map.json` and `spec/.snapshot.json` end byte-identical to the
state after the first run

**Rationale**: Idempotency. Refresh is deterministic over the same inputs
and converges in one run.

### Interaction with `mode: normal` runs

**Given** a fixture where one record's `spec_hash` is stale (impl-only
content edit) AND one new component has been added (so the diff contains
an `added` entry)
**When** a normal-mode `runIngest("--changeset", "...", "--receipts", "...")`
is run with a complete-status receipts file containing the `added`
component's create receipt
**Then** the new component's record is inserted with the receipt's
`bead_id` and the current `spec_hash` (Reconciler's existing behavior)
**And** the SNAPSHOT-side drift of the stale leaf is absorbed because
`SnapshotSaver` rebuilds the snapshot from the current spec state on
every complete-status ingest (per the impact-expectation note in the
proposal) — the stale record's `spec_hash` field itself is NOT rewritten
by a normal run, because the Reconciler only touches records that
receive receipts; aligning receipt-less records' `spec_hash` is exactly
what refresh mode exists for
**And** all other records are unchanged byte-for-byte
**And** the snapshot is rewritten to current

**Then in a separate scenario**, **Given** the same fixture but with the
stale record's content reverted to match the snapshot AND no new
component (clean spec)
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** the run is a no-op (per the no-change scenario above)

**Rationale**: Refresh and normal runs are complementary, not redundant.
Normal runs already rebuild the snapshot from current state, so
non-bead-producing drift is absorbed alongside whatever bead lifecycle the
run is doing. Refresh is the pathway when there is *only* non-bead-producing
drift and the user wants to absorb it without authoring a bead-producing
proposal.

## Edge cases

### Refresh against a missing pre-refresh snapshot

**Given** `spec/.snapshot.json` does not exist
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr indicates a missing-snapshot error and that refresh requires
a pre-existing snapshot (use a normal-mode bootstrap run instead)

**Rationale**: Refresh's diff baseline is the snapshot. Without one, every
leaf looks added — including every component — so the structural gate would
refuse anyway, but with an error naming the whole spec instead of the real
problem. This is exactly the bootstrap case that requires the normal pipeline.

### Refresh with non-empty changeset or non-empty receipts

**Given** the changeset or receipts file contains any ops
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr indicates that refresh mode requires both files to be empty

**Rationale**: Refresh is a parallel pathway with no per-op transitions; a
non-empty changeset is a configuration error.

### Atomic write failure mid-refresh

**Given** the snapshot write step fails (e.g., FS error simulated by an
unwritable target)
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** `.bead-map.json` is rolled back to its pre-refresh content (the
in-memory copy is committed only after the snapshot writes successfully —
or, equivalently, both writes are part of one atomic commit boundary)
**And** stderr names which step failed

**Rationale**: The snapshot+bead-map atomicity invariant must hold for
refresh just as it does for normal-mode complete runs. Either both move
together or neither does.

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The
handler-level tests in `ingest/refresh_test.go` build a two-module spec
tree, seed its snapshot and a `.bead-map.json` via `setupRefreshFixture`,
then introduce per-scenario drift by editing content files (headline),
adding or removing module.json entries (refusal gates), or appending an
orphan record. The command-level tests in `cmd/spex/ingest_test.go`
drive `spex ingest --mode refresh` through `setupRefreshedFixture`,
which seeds the baseline by running a complete normal-mode ingest first.
