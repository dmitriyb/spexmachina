# Bead Action Tests

Integration and acceptance tests for BeadCreator (component 1) and BeadCloser (component 2). These tests verify that the apply module correctly translates impact report actions into bead CLI commands with deterministic type assignment, proposal-epic hierarchy, lineage tracking, priority propagation, and obsolescence labeling.

## Setup

### Test Fixture: Fake BeadCLI

All bead action tests use a fake implementation of `BeadCLI` that records every call and returns configurable responses. This avoids shelling out to real `br`/`bd` binaries during tests.

```go
type fakeBeadCLI struct {
    createCalls   []CreateOpts
    closeCalls    []closeCall
    findResults   map[string]string // labels key -> existing bead ID
    createResults []string          // bead IDs returned in order
    closeErrors   map[string]error  // bead ID -> error to return
}
```

The fake tracks call order across Create and Close to verify execution sequencing.

### Test Fixture: Impact Report Actions

Standard action sets used across scenarios:

- **Single component create**: one create action for `validator/ContentResolver` with `spec_node_type=component`, `spec_hash=abc123`
- **Single data_flow create**: one create action for `merkle/Hash computation flow` with `spec_node_type=data_flow`, `spec_hash=def456`, `uses=[Hasher, TreeBuilder, SnapshotStore]`
- **Multi-component test_section create**: one create action for `apply/Bead action tests` with `spec_node_type=test_section`, `describes=[BeadCreator, BeadCloser]` (len >= 2)
- **Single-component test_section** (reference fixture for describes-length gate): used to assert ActionClassifier filters it out and it never reaches BeadCreator
- **Multi-node create**: four create actions in one apply run — two components across two modules, one data_flow, one multi-component test_section
- **Single obsolete**: one obsolete action for bead `spexmachina-42` in module `validator`, node `LegacyChecker`
- **Mixed batch**: two creates, two obsoletes — exercises both components in a single apply

### Test Fixture: Proposal Reference

All create-producing scenarios set `ImpactReport.proposal = "2026-04-12-data-flow-contract-layer"` (or a stub equivalent). The expected epic title is the verbatim proposal reference.

## Proposal Epic Scenarios

### S0a: BeadCreator creates the proposal epic first on every apply run with creates

Given an impact report with `proposal="test-proposal-2026-04-20"` and any non-empty `creates[]`.

When `CreateBeads` is called:

Then the fake's first `Create` call has:
- Title: `"test-proposal-2026-04-20"`
- Type: `"epic"`
- No `--parent` flag
- No `--deps blocks:` or `--deps depends:` flags

The returned epic bead ID is stored on the BeadCreator and used as `--parent` for every subsequent create in this run.

A bead-map record is written with `node_type="proposal"`, `bead_type="epic"`, `spec_node_id="test-proposal-2026-04-20"`, empty `spec_hash` and `module`.

### S0b: BeadCreator skips epic creation when the apply run has zero creates

Given an impact report with empty `creates[]` and one or more `obsoletes[]`.

When `CreateBeads` is called:

Then the fake receives zero `Create` calls. No proposal epic is created for a pure-obsolete run.

### S0c: BeadCreator creates one epic per apply run (not one per module)

Given an impact report with four create actions spanning three modules.

When `CreateBeads` is called:

Then exactly one epic `Create` call is made. All subsequent creates reference that single epic's bead ID as `--parent`. Module identity does not multiply epics.

## Create Scenarios

### S1: BeadCreator creates bead with correct type for a component node

Given a create action with `module=validator`, `node=ContentResolver`, `node_type=component`, `spec_hash=abc123`.

When `CreateBeads` is called with this action (preceded by the epic create per S0a):

Then the second `Create` call has:
- Title: `"validator: ContentResolver"`
- Type: `"feature"` (component nodes get `feature` type)
- `--parent <proposal-epic-bead-id>`
- `--silent` flag set

### S2: BeadCreator creates bead with correct type for a data_flow node

Given a create action with `module=merkle`, `node="Hash computation flow"`, `node_type=data_flow`, `spec_hash=def456`.

When `CreateBeads` is called:

Then the `Create` call has:
- Title: `"merkle: Hash computation flow"`
- Type: `"task"` (data_flow nodes get `task` type)
- `--parent <proposal-epic-bead-id>`

Data_flow beads are tasks, not features; they represent coordination work, not a capability to build.

### S3: BeadCreator creates bead with correct type for a multi-component test_section

Given a create action with `module=apply`, `node="Bead action tests"`, `node_type=test_section`, and the module spec shows `describes=["BeadCreator", "BeadCloser"]` (len == 2).

When `CreateBeads` is called:

Then the `Create` call has:
- Type: `"task"` (multi-component test_section nodes get `task` type)
- `--parent <proposal-epic-bead-id>`

### S4 (REMOVED): module→epic

This scenario is intentionally removed. Module nodes no longer produce beads via BeadCreator. Epics are now per-proposal, created once per apply run (see S0a). Module identity is carried via the `module` field on each bead-map record and via labels, not via an epic-per-module.

### S5: BeadCreator parents every created bead under the proposal epic

Given three create actions (one component, one data_flow, one multi-component test_section) in a single run where the proposal epic `Create` returns bead ID `spexmachina-epic1`.

When `CreateBeads` is called:

Then the fake records exactly four `Create` calls in this order:
1. Epic creation (no `--parent`)
2. Component feature with `--parent spexmachina-epic1`
3. Data_flow task with `--parent spexmachina-epic1`
4. Multi-component test_section task with `--parent spexmachina-epic1`

No calls use module-epic or component-feature as a parent.

### S6: BeadCreator sets --deps blocks for lineage on modified nodes

Given a create action where `old_bead_id=spexmachina-77` (the bead being replaced after obsolescence).

When `CreateBeads` is called:

Then the `Create` call includes `--deps blocks:spexmachina-77`. This creates a lineage chain: new bead blocks old bead.

### S7: BeadCreator does not set --deps blocks for genuinely new nodes

Given a create action for a new node with no `old_bead_id`.

When `CreateBeads` is called:

Then the `Create` call has no `--deps blocks:` flag. (It may still have `--deps depends:` flags if `DepBeadIDs` is non-empty.)

### S8: BeadCreator sets --priority from project requirement chain

Given a create action for a component that implements requirements tracing to P0 and P2 project requirements.

When `CreateBeads` is called:

Then the `Create` call includes `--priority 0`. The lowest priority number (highest urgency) wins.

### S9: BeadCreator skips creation when matching bead already exists (idempotency)

Given a create action for `validator/ContentResolver` and the fake's `FindExisting` returns bead ID `spexmachina-99` for the matching `spex:` label.

When `CreateBeads` is called:

Then the follow-up `Create` for that component is not issued. The returned bead ID list contains `spexmachina-99` in the component slot. The proposal epic create still happens (S0a) — idempotency applies per-node, not per-run.

### S10: BeadCreator processes multiple creates sequentially and accumulates IDs

Given three create actions across two modules (on top of the epic create).

When `CreateBeads` is called:

Then the fake receives exactly four `Create` calls (epic + three) in the order: epic first, then the three actions in input order. The returned bead ID list has four entries.

### S11: BeadCreator propagates creation errors and stops the batch

Given two create actions where the fake returns an error on the second non-epic create.

When `CreateBeads` is called:

Then the epic succeeds, the first action succeeds, the second returns an error, and `CreateBeads` returns that error. No subsequent action is attempted.

### S12: BeadCloser obsoletes bead with correct labels

Given an obsolete action with `bead_id=spexmachina-42`, `module=validator`, `node=LegacyChecker`.

When `CloseBeads` is called:

Then the fake receives one `Close` call with:
- ID: `spexmachina-42`
- Labels: `spex:obsolete`, `commit:<HEAD>` (where `<HEAD>` is the current git HEAD)

### S13: BeadCloser treats individual close errors as warnings and continues

Given three obsolete actions where the second one fails (e.g., bead already closed).

When `CloseBeads` is called:

Then all three `Close` calls are made. The returned error is a summary error aggregating the warning from the second close.

### S14: BeadCloser returns nil when all closes succeed

Given two obsolete actions that both succeed.

When `CloseBeads` is called:

Then the returned error is nil. Both `Close` calls were made with `spex:obsolete` and `commit:<HEAD>` labels.

### S15: BeadCreator creates cleanup bead with correct attributes

Given a create action with reason `"Code cleanup: BeadUpdater"`, `old_bead_id=spexmachina-lvf`, `module=apply`, `node=BeadUpdater`.

When `CreateBeads` is called:

Then the fake receives one `Create` call with:
- Title: `"Code cleanup: BeadUpdater"`
- Type: `"task"`
- `--parent <proposal-epic-bead-id>` (cleanup beads are children of the current proposal epic — the proposal that's cleaning up the removed code)
- `--deps blocks:spexmachina-lvf`
- `--silent` flag set

And the bead is labeled `spex:cleanup` (not `spex:<record-id>`).

And no mapping record is created in `.bead-map.json` (the component no longer exists in the spec).

## Describes-Length Gate Scenarios

These scenarios test that single-component test_sections are filtered out by ActionClassifier and never reach BeadCreator — complementary to the ActionClassifier tests (covered in `impact/test_classification_reporting.md`).

### G1: BeadCreator asserts and rejects a single-component test_section action

Given a malformed create action for a test_section where the module spec shows `describes=["OnlyOneComponent"]` (len == 1).

When `CreateBeads` is called:

Then it returns an error `"single-component test_section reached BeadCreator"` without issuing a bead CLI call. This is defense-in-depth; the authoritative filter is in ActionClassifier.

### G2: BeadCreator accepts a multi-component test_section action

Given a create action for a test_section where `describes` has length 2 or more.

When `CreateBeads` is called:

Then the `Create` call proceeds normally with type `task` and `--parent <proposal-epic-bead-id>`.

## Edge Cases

### E1: Create action with empty spec_hash

Given a create action where `spec_hash` is an empty string.

When `CreateBeads` is called:

Then the bead is created. The empty hash signals "not yet hashed." The proposal epic itself always has empty `spec_hash`.

### E2: Obsolete action with bead_id that no longer exists

Given an obsolete action for bead `spexmachina-gone` where the fake returns `"bead not found"` error.

When `CloseBeads` is called:

Then the error is logged as a warning (not fatal). `CloseBeads` returns a summary error but does not abort.

### E3: Create action where FindExisting errors

Given a create action where `FindExisting` returns an error (e.g., bead CLI timeout).

When `CreateBeads` is called:

Then the error propagates immediately. A failed existence check is not the same as "does not exist."

### E4: Mixed batch with zero-length sublists

Given an impact report with `creates=[]`, `obsoletes=[one action]`.

When the full apply flow processes this:

Then `CreateBeads` is called with an empty slice and returns immediately (no epic created — S0b). `CloseBeads` processes the single obsolete. Empty sublists do not cause nil pointer dereferences.

### E5: Large batch ordering verification

Given 50 create actions with distinct modules and nodes.

When `CreateBeads` is called:

Then the fake records exactly 51 `Create` calls (one epic + 50 actions) with the 50 actions in the same order as the input slice.

### E6: Type table is exhaustive

For each spec node type that produces beads, assert the correct mapping:
- `proposal` → `epic` (implicit, one per run)
- `component` → `feature`
- `data_flow` → `task`
- `test_section` with `len(describes) >= 2` → `task`

For node types that do NOT produce beads, assert ActionClassifier filters them and BeadCreator is never invoked:
- `impl_section` — always skipped
- `test_section` with `len(describes) == 1` — filtered by the describes-length gate
- `meta`, `requirement` — filtered by NodeMatcher as structural

### E7: Epic bead ID reuse across a long run

Given 20 create actions.

When `CreateBeads` is called:

Then the epic bead ID returned by the first `Create` call is used as `--parent` for all 20 subsequent calls. No other bead ID appears in the `--parent` flag during this run.

## Spec-Graph Dependency Scenarios

These scenarios test that BeadCreator passes `--deps depends:<bead-id>` for spec-graph dependencies carried in the action's `DepBeadIDs` field. The `depends` relationship type is separate from `blocks` (lineage).

### D1: BeadCreator passes --deps depends for each DepBeadID

Given a create action with `DepBeadIDs: ["spex-200", "spex-201"]` and no `OldBeadID`.

When `CreateBeads` is called:

Then the fake receives one `Create` call (beyond the epic) with:
- `--deps depends:spex-200`
- `--deps depends:spex-201`
- No `--deps blocks:` flag (no lineage)
- `--parent <proposal-epic-bead-id>`

Multiple `--deps` flags are passed — one per dependency.

### D2: BeadCreator passes both blocks and depends deps

Given a create action with `OldBeadID: "spex-100"` and `DepBeadIDs: ["spex-200"]`.

When `CreateBeads` is called:

Then the fake receives one `Create` call with:
- `--deps blocks:spex-100` (lineage)
- `--deps depends:spex-200` (spec-graph dependency)
- `--parent <proposal-epic-bead-id>`

All three flag categories coexist on the same bead.

### D3: BeadCreator skips --deps depends when DepBeadIDs is empty

Given a create action with `DepBeadIDs: []` (empty) or nil.

When `CreateBeads` is called:

Then no `--deps depends:` flags are passed. Only `--deps blocks:` is passed if `OldBeadID` is set.

### D4: BeadCreator passes multiple depends deps for large dependency sets

Given a create action with `DepBeadIDs: ["a", "b", "c", "d", "e"]` (5 dependencies).

When `CreateBeads` is called:

Then the fake receives five `--deps depends:` flags in the order they appear in `DepBeadIDs`. Order is deterministic.

### D5: Data_flow bead ID flows into component bead DepBeadIDs within the same run

Given a run with a data_flow create (returns bead ID `spex-flow-1`) and a component create where the component is in the data_flow's `uses` array. The ActionClassifier has already populated `DepBeadIDs: ["spex-flow-1"]` on the component's create action via topological resolution.

When `CreateBeads` is called:

Then the data_flow task is created first (topologically before its dependents), its bead ID is captured, and the component's `Create` call includes `--deps depends:spex-flow-1`. This establishes the contract-first ordering: the data_flow bead must complete before any participating component bead can start.
