# Refresh mode tests

Module integration tests for the `RefreshHandler` component (id `f9033352c13f`). Refresh mode is
the parallel ingest pathway for absorbing spec drift that owes no task work, without triggering
task lifecycle. This test_section covers the RefreshHandler's contract end-to-end through
`spex ingest --mode refresh`, exercised against a real fixture spec tree, with a real journal
and a real snapshot at the fixture project's resolved locations. Per-method unit tests for internal
helpers belong in `ingest/refresh_test.go` and are bundled with this component's implementation
task.

## Setup

- `tmpdir/` containing a complete fixture spec (project.json + module.json files + content
  leaves), a pre-seeded journal with pairings for the fixture's task-producing nodes, and a
  pre-seeded snapshot matching some earlier state of the fixture spec.
- An empty changeset (`version: 4`, empty `ops`) and an empty receipts file (`version: 2`,
  empty `ops`, `status: "complete"`).
- Helper `runIngest(args ...string) (stdout, stderr string, exitCode int)` wraps
  `IngestCommand.Run`.

The "pre-existing snapshot" is the diff baseline. RefreshHandler reads it, computes a fresh tree
from current files, and uses the diff between them to decide whether the run is safely a refresh.

## Scenarios

### Refresh-only diff is absorbed; events appended

**Given** the fixture has been edited so that `beta/arch_handler.md` and
`alpha/test_widget_logic.md` are modified relative to the snapshot, and no other content changed
**When** `runIngest("--mode", "refresh", "--changeset", "...", "--receipts", "...")` is called
**Then** exit code is 0
**And** the journal gains one `modified` change event per drifted leaf, each carrying the before
and after hashes, and one `refresh` receipt whose `absorbed` list names exactly those event ids
**And** the receipt records the `--git-head` value when one was given, and its absence otherwise
**And** no pre-existing journal line is altered
**And** the snapshot is rewritten to match the current spec state

**Rationale**: the headline behavior — drift is absorbed without task churn, and unlike the
retired record-rewrite semantics, the absorption itself is now on the record: a refresh run is
visible in history, not amnesiac.

### Refresh refuses on diff with `added` entries

**Given** the fixture has been edited to introduce a new **task-producing** content leaf (a new
component referenced from `alpha/module.json`) so the diff contains a non-absorbable `added` entry
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr contains a structured error naming the added entries and the reason ("refresh mode
does not absorb structural changes; use the normal pipeline")
**And** the journal and the snapshot are unchanged byte-for-byte

**Rationale**: structural changes that owe task work must go through the normal Reconciler path.
The fixture must add a `component`, `data_flow` or `test_section`; an added `requirement` or `api`
is absorbed and would not refuse.

### Refresh refuses on diff with `removed` entries

**Given** the fixture has been edited to delete a **non-absorbable** leaf (a data_flow removed
from `beta/module.json`) so the diff contains a non-absorbable `removed` entry
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero, stderr names the removed entries with the same "use the normal
pipeline" reason, and both files are unchanged

**Rationale**: the removal direction is narrower than the addition direction: `component` removal
IS absorbed, so the fixture must remove a `data_flow` or `test_section` to exercise the gate.

### Refresh absorbs the absorbable structural set

**Given** the fixture has been edited to add a requirement, add and remove an api, and remove a
component whose journal pairing names a task the normal pipeline already closed
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is 0
**And** the journal gains `added`/`removed` change events for each absorbed entry plus the
`refresh` receipt naming them
**And** the snapshot is rewritten to match the current spec state

**Rationale**: the refusal gate is a filter on node type and direction, not a blanket ban. A test
that only exercises the refusal side would pass against an implementation that refused everything.
The component removal is paired with a journaled `task_closed` deliberately: the orphan gate, not
the structural gate, is what keeps a component removal honest.

### Refresh refuses while an unclosed pairing points at a removed node

**Given** the fixture's current spec graph does NOT contain a node whose journal fold still shows
a task pairing with no `task_closed` — the only signal refresh has, since it reads no task-state
artifact and cannot tell an open task from a finished one
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero
**And** stderr contains a structured error naming the node's identity hash and the task id,
with the reason ("live task for removed node; structural drift requires the normal pipeline") — "live" in the message meaning unclosed in the journal's sense, the only sense refresh has
**And** both files are unchanged

**Rationale**: a task pointing at a deleted node signals task work the normal pipeline owes
(close or cleanup) — and only the normal pipeline, reading the task-state artifact, can tell
which. Refresh absorbing the removal would leave the tracker lying about live work — a state we
want surfaced, not baselined. A finished task the journal never saw closed refuses here too, and
that is the intended cost: the normal pipeline's cleanup path is where a finished task's removal
is recorded.

### Refresh on no-change spec is a clean no-op

**Given** the fixture is byte-identical to the snapshot
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is 0, the journal is unchanged byte-for-byte (no events, and no empty refresh
receipt either), the snapshot is unchanged, and stdout indicates nothing was absorbed

**Rationale**: refresh is safe to run on a clean spec — a no-op rather than an error, a snapshot
rewrite, or a noise line in the journal.

### Re-running refresh on the same state is a no-op

**Given** a previous refresh run completed successfully and appended events
**When** `runIngest("--mode", "refresh", ...)` is called again immediately, without spec edits
between runs
**Then** exit code is 0 and both files end byte-identical to the state after the first run

**Rationale**: idempotency. Refresh is deterministic over the same inputs and converges in one
run; the second run finds no drift against the freshly written snapshot and appends nothing.

### Interaction with `mode: normal` runs

**Given** a fixture where one leaf drifted content-only AND one new component has been added
**When** a normal-mode `runIngest` is run with a complete-status receipts file containing the
added component's create receipt
**Then** the journal gains the added component's change event and `task_created` receipt
(Reconciler's behavior), and the snapshot rebuild absorbs the content-only drift on the snapshot
side — but no journal event is appended for the receipt-less drifted leaf, because the Reconciler
path only appends for ops and marked absorb entries; recording unmarked receipt-less drift is
exactly what refresh mode exists for
**And in a separate scenario**, the same fixture with the drift reverted and no new component
refreshes as a no-op

**Rationale**: refresh and normal runs are complementary, not redundant. Normal runs baseline
whatever task lifecycle they process; refresh is the pathway when there is *only* driftwork, and
its receipt is what keeps that absorption accountable.

### The absorbable table is the resolved profile's declaration

**Given** the absorbable-set fixture above, run once with no profile file and once with a
`spec/profile.json` byte-identical to the default declaration
**When** `runIngest("--mode", "refresh", ...)` is called on each
**Then** both runs absorb and refuse identically — requirement and api in either direction,
component on removal only — because the table RefreshHandler consults is read from the resolved
profile's per-type, per-direction absorbable declarations, and the default profile declares
exactly the previous built-in set
**And** with a profile declaring an `endpoint` type absorbable in neither direction, an added
endpoint refuses the run with the same "use the normal pipeline" error the non-absorbable
built-ins draw

**Rationale**: the gate's policy moved from code to the profile without moving the gate; a
default-profile run must be indistinguishable from the pre-profile binary.

## Edge cases

### Refresh refuses on an empty journal — the bootstrap guard

**Given** an initialised project whose snapshot is present — the empty tree init seeded, or any
later state — but whose journal contains zero lines
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero and stderr indicates refresh requires a completed cycle (run the
normal pipeline first)
**And** both files are unchanged

**Rationale**: this is the exact state the guard's re-key exists for. The guard used to ask
"does a snapshot file exist?" as a proxy for "has a cycle ever completed?" — a proxy that
`spex init` writing a snapshot at birth makes permanently true. It now keys on the fact it
always wanted: the journal being empty. The stakes are modest and disclosed as such — refresh
only absorbs added `requirement` and `api` leaves, so a real new project's added components
would refuse the run at the structural gate regardless; the re-key turns that late refusal
listing every added entry back into one early, clear refusal naming the real problem.

### Refresh with non-empty changeset or non-empty receipts

**Given** the changeset or receipts file contains any ops
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero and stderr indicates that refresh mode requires both files to be
empty

### Atomic write failure mid-refresh

**Given** the snapshot write step fails (FS error simulated by an unwritable target)
**When** `runIngest("--mode", "refresh", ...)` is called
**Then** exit code is non-zero, the journal is rolled back to its pre-refresh content
(both writes are part of one atomic commit boundary), and stderr names which step failed

**Rationale**: the snapshot+journal atomicity invariant must hold for refresh just as it does for
normal-mode complete runs. Either both move together or neither does.

## Fixtures

In-code Go fixtures, no on-disk testdata (the package convention). The handler-level tests in
`ingest/refresh_test.go` build a two-module spec tree, seed its snapshot and journal via a fixture
helper, then introduce per-scenario drift by editing content files (headline), adding or removing
module.json entries (refusal gates), or seeding an unclosed pairing for a node about to be removed
(orphan gate). The command-level tests in `cmd/spex/ingest_test.go` drive
`spex ingest --mode refresh` end-to-end, seeding the baseline by running a complete normal-mode
ingest first.
