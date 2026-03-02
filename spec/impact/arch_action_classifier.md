# ActionClassifier

Determines the action for each affected bead or unmatched spec node.

## Responsibilities

- Assign actions based on match results and change types
- Handle edge cases (multiple changes affecting the same bead)

## Action Types

| Condition | Action | Description |
|-----------|--------|-------------|
| Changed spec node has no matching bead (Unmatched) | `create` | New spec content needs a new bead |
| Removed spec node has matching bead (Orphaned) | `close` | Spec content removed, bead is obsolete |
| Modified spec node has matching bead (Match + modified) | `review` | Spec content changed, bead needs review |
| Added spec node has matching bead (shouldn't happen) | `review` | Bead exists for new content — review for consistency |

## Interface

```go
type Action struct {
    Type    string   // "create", "close", "review"
    BeadID  string   // existing bead ID (empty for "create")
    Module  string   // affected module
    Node    string   // affected spec node (component/impl_section name)
    Impact  string   // impact level from merkle classification
    Reason  string   // human-readable explanation
}

func ClassifyActions(matches []Match, unmatched []Unmatched, orphaned []Orphaned) []Action
```
