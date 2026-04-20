# ActionClassifier

Determines the action for each affected bead or unmatched spec node using a simplified state transition table. Only two action types exist: `create` and `obsolete`.

## Responsibilities

- Assign actions based on match results and change types
- Handle modified nodes by generating both obsolete (old bead) and create (new bead) actions
- Gate action production by node type and, for test_sections, by `len(describes)`
- Handle edge cases (multiple beads per node, unexpected state combinations)

## Node-Type Gate

Before applying the state transition table, each change is gated by its node type:

| NodeType | Produces beads? | Notes |
|----------|-----------------|-------|
| `component` | yes (feature) | primary work unit |
| `data_flow` | yes (task) | cross-component contract, always produces a bead |
| `test_section`, `len(describes) >= 2` | yes (task) | cross-component integration test, needs its own bead |
| `test_section`, `len(describes) == 1` | no | bundled into the single described component's feature bead; implement skill reads the test_section content as part of the component's TDD workflow |
| `impl_section` | no | implementation detail; owned by the component bead |
| `meta`, `requirement` | no | filtered upstream by NodeMatcher (`structural` skip) |

`describes` array length is read from the current module.json for the test_section's node. If the array is empty (orphan test_section), validator would already have rejected it — ActionClassifier may assert and skip defensively.

## State Transition Table

| Spec Change | Existing Bead? | Action |
|-------------|---------------|--------|
| added | no | **create** new bead |
| added | yes (unexpected) | **obsolete** old bead + **create** new bead |
| modified | no | **create** new bead |
| modified | yes | **obsolete** old bead + **create** new bead |
| removed | no | no action |
| removed | yes, open/in_progress | **obsolete** old bead |
| removed | yes, closed | **obsolete** old bead + **create** cleanup bead |

The "review" action is eliminated. Any spec change to a node with an existing bead always obsoletes the old bead. If the node still exists (added or modified), a fresh bead is created.

## Spec-Graph Dependency Resolution

After classification, the ActionClassifier resolves spec-graph dependencies for each create action:

1. **Component `uses` edges** (direct only): For each component the created node uses, the `uses` array contains identity hashes. Look each hash up directly in the mapping file. If the matched bead is open, add to `DepBeadIDs`.
2. **Module `requires_module` edges** (transitive): Walk the module dependency graph by identity hash, collecting open component beads from each required module. Uses cycle detection to handle invalid graphs.
3. **Closed beads are skipped**: If a dependency's bead is closed, the work is done — no edge needed.

This resolves structural dependencies into concrete bead IDs that flow through the impact report to BeadCreator, which passes them as `--deps depends:<bead-id>`. There is no `fmt.Sprintf("%s/component/%d", ...)` reconstruction — the spec-graph edges are already in the form the mapping store accepts.

## Interface

```go
type Action struct {
    Type       string   // "create" or "obsolete"
    BeadID     string   // existing bead ID (for "obsolete"); empty for "create"
    Module     string   // affected module name (carried alongside the identity hash for human-readable output)
    Node       string   // affected spec node name (carried alongside the identity hash)
    NodeType   string   // spec node type (component/data_flow/test_section)
    SpecNodeID string   // identity hash of the affected node — the lookup key into the mapping store
    SpecHash   string   // current merkle content hash (for "create")
    OldBeadID  string   // predecessor bead ID (for "create" replacing an obsoleted bead)
    DepBeadIDs []string // bead IDs this action's bead should depend on (from spec graph)
    Reason     string   // human-readable explanation
}

func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action
```

`NodeType` is carried as a separate field on `ClassifiedChange` and `Action` because identity hashes do not embed the node type — there is no way to tell from `abc123def456` alone whether the node is a component, impl_section, or test_section. The merkle tree records `NodeType` on every leaf when it builds the tree, and that field flows through the diff into impact and apply.

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified node): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
