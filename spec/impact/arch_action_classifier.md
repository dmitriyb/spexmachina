# ActionClassifier

Determines the action for each affected bead or unmatched spec node using a simplified state transition table. Only two action types exist: `create` and `obsolete`.

## Responsibilities

- Assign actions based on match results and change types
- Handle modified nodes by generating both obsolete (old bead) and create (new bead) actions
- Handle edge cases (multiple beads per node, unexpected state combinations)

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

## Interface

```go
type Action struct {
    Type      string   // "create" or "obsolete"
    BeadID    string   // existing bead ID (for "obsolete"); empty for "create"
    Module    string   // affected module
    Node      string   // affected spec node (component/test_section name)
    NodeType  string   // spec node type (module/component/test_section)
    SpecHash  string   // current merkle hash (for "create")
    OldBeadID string   // predecessor bead ID (for "create" replacing an obsoleted bead)
    Reason    string   // human-readable explanation
}

func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action
```

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified node): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
