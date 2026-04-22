# ActionClassifier

Determines the action for each affected bead or unmatched spec node using a simplified state transition table. Only two action types exist: `create` and `obsolete`. Collects `DepSpecNodeIDs` for each create action — identity hashes of spec nodes the created bead will depend on. Bead-ID resolution is NOT done here; it moves to the emit module.

## Responsibilities

- Assign actions based on match results and change types.
- Handle modified nodes by generating both obsolete (old bead) and create (new bead) actions.
- Gate action production by node type and, for test_sections, by `len(describes)`.
- Use the bead's live status (from BeadReader) to choose between "obsolete only" and "obsolete + cleanup create" for removed nodes.
- For each create action, collect `DepSpecNodeIDs` by walking component `uses` (direct) and module `requires_module` (transitive).

## Node-Type Gate

Before applying the state transition table, each change is gated by its node type:

| NodeType | Produces beads? | Notes |
|----------|-----------------|-------|
| `component` | yes (feature) | primary work unit |
| `data_flow` | yes (task) | cross-component contract, always produces a bead |
| `test_section`, `len(describes) >= 2` | yes (task) | cross-component integration test, needs its own bead |
| `test_section`, `len(describes) == 1` | no | bundled into the single described component's feature bead |
| `impl_section` | no | implementation detail; owned by the component bead |
| `meta`, `requirement` | no | filtered upstream by NodeMatcher (`structural` skip) |

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

## DepSpecNodeIDs Collection

After classification, for each create action:

1. **Component `uses` edges** (direct only): read the component's `uses` array; each entry is a sibling component's identity hash. Add each to `DepSpecNodeIDs`.
2. **Module `requires_module` edges** (transitive, with cycle detection): walk the module dependency graph by identity hash. For each reachable required module, add every component's identity hash from that module to `DepSpecNodeIDs`.
3. **Data_flow add-ons** (contract layer): for each create with node_type=component whose spec_node appears in a changed data_flow's `uses` array, add the data_flow's identity hash to the component's `DepSpecNodeIDs`. This ensures emit's topological sort places the data_flow create op first and the component ops gain `ref:op` deps on it.

The output is a set (deduplicated) of identity hashes. No bead lookup, no status filtering — those belong to emit's Resolver.

## Why DepSpecNodeIDs, Not DepBeadIDs

Prior to this proposal, ActionClassifier resolved deps to bead IDs at impact time by querying the mapping store. When a dep was being obsoleted+recreated in the same batch, the resolver picked up the OLD (soon-closed) bead ID, leading to the broken-dep-graph bug (commit `21defea`). The fix is to defer resolution to emit time where the three-shape ref scheme (ref:op / ref:bead / ref:spec_node) can distinguish same-run work from upstream deps.

ActionClassifier now emits spec_node_ids — identity hashes that stay stable across batches. Emit's Resolver classifies each at emit time with full knowledge of the current batch's op_ids.

## Interface

```go
type Action struct {
    Type            string   // "create" or "obsolete"
    BeadID          string   // existing bead ID (for "obsolete"); empty for "create"
    Module          string   // affected module name (carried alongside the identity hash for human-readable output)
    Node            string   // affected spec node name
    NodeType        string   // spec node type (component/data_flow/test_section)
    SpecNodeID      string   // identity hash of the affected node — the lookup key into the mapping store
    SpecHash        string   // current merkle content hash (for "create")
    OldBeadID       string   // predecessor bead ID (for "create" replacing an obsoleted bead)
    DepSpecNodeIDs  []string // identity hashes of spec nodes this action's bead should depend on — resolved to refs by emit
    Reason          string   // human-readable explanation
}

func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action
```

`NodeType` is carried as a separate field on `ClassifiedChange` and `Action` because identity hashes do not embed the node type.

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified node): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
