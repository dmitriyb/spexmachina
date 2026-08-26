# Change Proposal: Reconciler decomposition

## Context

`spec/ingest/module.json` declares `Reconciler` (`2b5158af774b`) as one component. The Go behind that single name is about 35KB across 23 functions, and its own arch leaf gives the split away: `arch_reconciler.md` carries separate sections for the per-op construction table, retarget ops, absorbed entries, the modified-node pair, proposal-epic ops and cleanup-create ops — then, quite separately, invariant enforcement, and separately again the single write path. The spec already describes several things. It just declares one.

That is the argument, and it is why this cannot be a quiet refactor. In a self-hosted repository the component list is the spec, so splitting a component means new components, new arch leaves and new test coverage — a spec change by construction, not a cleanup somebody performs and mentions afterwards.

Two things are worth saying plainly in the other direction, because the code is better than a first pass suggests.

**The invariant numbering is correct, not arbitrary.** `checkInvariant1`, `checkInvariant2` and `checkInvariant5` look like a numbering with gaps. They are not. `spec/ingest/test_consistency_invariants.md` has a section titled for the five invariants, and 3 and 4 are there — re-run appends nothing, and snapshot saved only on complete. They have no check function because they are enforced by construction rather than by a per-line predicate. So the names trace to a numbered spec section, which in this repository is the right way to name them. What is genuinely missing is smaller: a reader inside the Go file cannot tell what invariant 2 *is* without leaving the file. A doc comment per check naming what it enforces closes that, and no rename is needed or wanted.

**The doc comments and tests are good.** The test file beside `reconciler.go` is larger than the source. This is not neglected code; it is code whose structure has outgrown its declaration.

The one genuine structural smell is parameter threading. `Apply` builds a `hasEID` closure over the journal's existing eids plus the in-flight batch, and then hands it, along with the journal fold, the same-batch removals, the registered-by-stem map and the modified-handled set, down through builder after builder by hand. Several builders take eight parameters for this reason. That is per-run state being passed as arguments because there is nowhere to put it.

**Priority, stated honestly.** This is maintenance debt. It has no user-visible value, and it ranks below every adoption-facing proposal in this series. The argument that survives is narrower: `ingest` is where a correctness bug is most expensive — it is what writes the journal, the one artifact that cannot be rebuilt — and it is the module an outside contributor is most likely to have to touch. It should also land before any future change that adds a mode to this module, because restructuring underneath a new mode is harder than restructuring before it.

## Proposed change

### Reconciler stays, and stays an orchestrator

`Reconciler` is **not** renamed and **not** removed. That is a deliberate constraint rather than an accident of convenience: a rename is delete-plus-create, and `Reconciler` has closed tasks, so a removal would mint cleanup beads on top of the new-node cost.

It is also honest against the code. `Apply` pairs receipts to ops, opens the journal, folds it, builds the eid closure, loops the ops dispatching each to the right construction path, then runs the invariant checks and writes once. That is an orchestrator, and it remains one. The three components below are carved out from beneath it and reached through `uses` edges.

### EventBuilder

Owns construction of journal events per action class — the arch leaf's per-op construction table, the retarget and absorbed paths, the modified-node pair, and the epic and cleanup-create paths. This is the largest of the three and the one the parameter threading exists to serve.

The threaded arguments become the builder's own per-run state: the eid predicate that answers whether an eid is already in the journal or already in this batch, the journal fold, the same-batch removals, the registered-by-stem map and the modified-handled set. `Reconciler` constructs it once per run and calls it per op. No behaviour changes; what changes is that the state has a place to live instead of a path to travel.

### InvariantChecker

Owns the consistency checks that run over the existing journal and the batch before anything is written. Each check gains a doc comment naming the invariant it enforces, so the numbering resolves without leaving the file, and the spec section it traces to is named.

### JournalEncoder

Owns turning an event into a journal line and validating that line against the bead-map schema before it is written — invariant 5's guarantee, expressed as a component rather than as two helpers at the bottom of a large file.

Carving it out makes one thing visible that is currently invisible: `ingest` encodes journal lines while the `map` module owns the journal. Whether that responsibility should eventually move is a real question and explicitly not this proposal's — naming the component is what makes the question askable at all.

### Behaviour-preserving, and the tests say so

Nothing about what ingest does changes. The existing test files are the acceptance evidence, and any behavioural difference they surface is a defect introduced by this work, not a feature of it.

## Impact expectation

### New components — ingest module

| Component | Implements | Bead |
|---|---|---|
| `EventBuilder` | [[539030e8c5a4|reconcile per-op receipts]], [[7191a50f7447|reconcile retarget receipts]], [[7900dcd38c4a|absorb entries reach the journal]] | yes |
| `InvariantChecker` | [[ee28b5d190ae|mapping consistency invariants]], [[fd6f08ef34fa|re-run idempotency]] | yes |
| `JournalEncoder` | [[ee28b5d190ae|mapping consistency invariants]] | yes |

No new requirement is needed. Every responsibility being carved out is already required by one of the module's existing requirements — which is itself evidence that the spec under-declared its components rather than under-specifying its behaviour. `Reconciler` keeps [[16dbbee94e88|partial receipts tolerance]] and co-implements `539030e8c5a4` as the orchestrator, the same way `ValidateCommand` and `ErrorReporter` co-implement one requirement in the validator module.

All three are reached from `Reconciler` by `uses` edges, which stay acyclic.

### Modified nodes

| Node | Hash | Bead | Why |
|---|---|---|---|
| `Reconciler` | `2b5158af774b` | yes | narrowed to orchestration; gains three `uses` edges; leaf rewritten around what it still owns |
| Reconciliation tests | `425f743382a7` | yes | gains `EventBuilder`, reaching two components |
| Consistency invariants | `2cd902e10873` | yes | gains `InvariantChecker` and `JournalEncoder` |
| Partial run recovery tests | `2515c91337a5` | yes | the recovery path now runs through the carved-out builders |

Coverage is satisfied without a single new test section: each new component joins a `describes` list that already covers the behaviour it took over. `Reconciliation tests` crosses the two-component threshold and becomes a bead; the other two were beads already.

### Modified requirements, and the leaves they oblige

| Requirement | Hash | Obliges |
|---|---|---|
| Absorb entries reach the journal | `7900dcd38c4a` | Reconciler, EventBuilder |
| Reconcile retarget receipts | `7191a50f7447` | Reconciler, EventBuilder |

Both are **forced**, not incidental: each description names `Reconciler` as the thing that constructs the events in question, and after the split `EventBuilder` constructs them. Leaving the wording alone would leave two requirements pointing at the wrong component.

### Meta-leaf accounting

`ingest/module.json` changes substantially — three components added, `implements` and `uses` edges rewritten — which moves the module's `meta` leaf. `ingest` has four existing components, so an unsuppressed meta rule would oblige a changed leaf from `SnapshotSaver` (`f85bd2f94aeb`), `IngestCommand` (`db90eb607bcb`) and `RefreshHandler` (`f9033352c13f`) as well, none of which this change touches.

It is suppressed, because `7900dcd38c4a` and `7191a50f7447` both change in the same module. That is not luck — the two requirements that name `Reconciler` are exactly the ones the split forces — but it is worth checking rather than assuming, because it is the difference between four leaves and seven.

### The checks that can actually fail

The whole existing suite is the check, and it is a real one: this change is a defect if a single test in `ingest` changes its expected value. Two additions earn their place beyond that.

A test asserting that the eid predicate sees both the journal's existing eids and the in-flight batch when reached through `EventBuilder`'s own state rather than through a threaded closure — that is the one thing the refactor could plausibly get wrong, because the closure captures a map that the loop mutates as it goes.

And a `JournalEncoder` test asserting a deliberately schema-invalid line is refused before the write, which is invariant 5 exercised against the component that now owns it rather than against the file that used to.

### Scope

Seven beads under one epic: four component leaves — three new, one modified — and three test sections. Nothing is renamed, nothing is removed, and no cleanup beads are minted.
