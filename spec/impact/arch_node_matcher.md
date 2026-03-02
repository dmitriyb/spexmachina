# NodeMatcher

Matches changed spec nodes from the merkle diff to existing beads.

## Responsibilities

- Take classified changes (from merkle diff) and bead spec metadata
- Match each changed spec node to the bead(s) that reference it
- Identify unmatched changes (new spec nodes without beads)
- Identify unmatched beads (beads referencing removed spec nodes)

## Interface

```go
type Match struct {
    Change ClassifiedChange
    Beads  []BeadSpec // beads that reference this spec node
}

type Unmatched struct {
    Change ClassifiedChange // new spec node, no bead
}

type Orphaned struct {
    Bead BeadSpec // bead references removed spec node
}

func MatchNodes(changes []ClassifiedChange, beads []BeadSpec) ([]Match, []Unmatched, []Orphaned)
```

## Matching Logic

A bead matches a changed spec node when:
- `bead.Module` matches the change's module name, AND
- `bead.Component` or `bead.ImplSection` matches the specific node that changed

For structural changes (module.json/project.json), all beads in the affected module are considered impacted.
