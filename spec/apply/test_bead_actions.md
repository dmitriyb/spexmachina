# Bead Action Tests

Integration and acceptance tests for BeadCreator (component 1) and BeadCloser (component 2). These tests verify that the apply module correctly translates impact report actions into bead CLI commands with deterministic type assignment, parent hierarchy, lineage tracking, priority propagation, and obsolescence labeling.

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
- **Multi-node create**: three create actions spanning two modules (`validator/ContentResolver`, `validator/DagChecker`, `merkle/Hasher`)
- **Single obsolete**: one obsolete action for bead `spexmachina-42` in module `validator`, node `LegacyChecker`
- **Mixed batch**: two creates, two obsoletes — exercises both components in a single apply

## Scenarios

### S1: BeadCreator creates bead with correct type for a component node

Given a create action with `module=validator`, `node=ContentResolver`, `node_type=component`, `spec_hash=abc123`.

When `CreateBeads` is called with this action:

Then the fake receives exactly one `Create` call with:
- Title: `"validator: ContentResolver"`
- Type: `"feature"` (component nodes get `feature` type)
- `--silent` flag set

And the returned bead ID list contains the single ID from the fake's response.

### S2: BeadCreator creates bead with correct type for a test_section node

Given a create action with `module=validator`, `node=SchemaTests`, `node_type=test_section`, `spec_hash=fff000`.

When `CreateBeads` is called:

Then the `Create` call has `--type task` (test_section nodes get `task` type).

### S3: BeadCreator creates bead with correct type for a module node

Given a create action with `module=validator`, `node_type=module`.

When `CreateBeads` is called:

Then the `Create` call has `--type epic` (module nodes get `epic` type).

### S4: BeadCreator sets --parent for component beads

Given a create action for a component where the module's epic bead ID is `epic-001` (resolved from the mapping file).

When `CreateBeads` is called:

Then the `Create` call includes `--parent epic-001`. Component (feature) beads are parented under the module's epic bead.

### S5: BeadCreator sets --parent for test_section beads

Given a create action for a test_section where the component's feature bead ID is `feature-002` (resolved from the mapping file).

When `CreateBeads` is called:

Then the `Create` call includes `--parent feature-002`. Test (task) beads are parented under the component's feature bead.

### S6: BeadCreator sets --deps blocks for lineage on modified nodes

Given a create action where `old_bead_id=spexmachina-77` (the bead being replaced after obsolescence).

When `CreateBeads` is called:

Then the `Create` call includes `--deps blocks:spexmachina-77`. This creates a lineage chain: new bead blocks old bead.

### S7: BeadCreator does not set --deps for genuinely new nodes

Given a create action for a new node with no `old_bead_id`.

When `CreateBeads` is called:

Then the `Create` call has no `--deps` flag.

### S8: BeadCreator sets --priority from project requirement chain

Given a create action for a component that implements requirements tracing to P0 and P2 project requirements.

When `CreateBeads` is called:

Then the `Create` call includes `--priority 0`. The lowest priority number (highest urgency) wins.

### S9: BeadCreator skips creation when matching bead already exists (idempotency)

Given a create action for `validator/ContentResolver` and the fake's `FindExisting` returns bead ID `spexmachina-99` for the matching `spex:` label.

When `CreateBeads` is called:

Then `Create` is never called on the fake. The returned bead ID list contains `spexmachina-99`. This verifies the idempotency guarantee.

### S10: BeadCreator processes multiple creates sequentially and accumulates IDs

Given three create actions across two modules.

When `CreateBeads` is called:

Then the fake receives exactly three `Create` calls in the order the actions appear. The returned bead ID list has three entries.

### S11: BeadCreator propagates creation errors and stops the batch

Given two create actions where the fake returns an error on the second create.

When `CreateBeads` is called:

Then the first create succeeds, the second returns an error, and `CreateBeads` returns that error. No third action is attempted.

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

## Edge Cases

### E1: Create action with empty spec_hash

Given a create action where `spec_hash` is an empty string.

When `CreateBeads` is called:

Then the bead is created. The empty hash signals "not yet hashed."

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

Then `CreateBeads` is called with an empty slice and returns immediately. `CloseBeads` processes the single obsolete. Empty sublists do not cause nil pointer dereferences.

### E5: Large batch ordering verification

Given 50 create actions with distinct modules and nodes.

When `CreateBeads` is called:

Then the fake records exactly 50 `Create` calls in the same order as the input slice.

### S15: BeadCreator creates cleanup bead with correct attributes

Given a create action with reason `"Code cleanup: BeadUpdater"`, `old_bead_id=spexmachina-lvf`, `module=apply`, `node=BeadUpdater`.

When `CreateBeads` is called:

Then the fake receives one `Create` call with:
- Title: `"Code cleanup: BeadUpdater"`
- Type: `"task"`
- `--deps blocks:spexmachina-lvf`
- `--silent` flag set

And the bead is labeled `spex:cleanup` (not `spex:<record-id>`).

And no mapping record is created in `.bead-map.json` (the component no longer exists in the spec).

### E6: Type table is exhaustive

Attempt to create a bead for each spec node type that produces beads (module, component, test_section). Assert each maps to the correct bead type (epic, feature, task). Attempt to create for node types that do NOT produce beads (impl_section, data_flow). Assert these are rejected or skipped.
