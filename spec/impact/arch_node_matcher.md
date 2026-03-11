# NodeMatcher

Matches changed spec nodes from the ID-based merkle diff to existing beads using spec node IDs from the mapping file.

## Responsibilities

- Take classified changes (from merkle diff) and bead-to-record mappings
- Match each changed spec node to the bead(s) that reference it via mapping records
- Identify unmatched changes (new spec nodes without beads)
- Identify unmatched beads (beads referencing removed spec nodes)

## Interface

```go
type Match struct {
    Change  ClassifiedChange
    Records []Record // mapping records linking this spec node to beads
}

type Unmatched struct {
    Change ClassifiedChange // new spec node, no mapping record
}

type Orphaned struct {
    Record Record // mapping record references removed spec node
}

func MatchNodes(changes []ClassifiedChange, records []Record) ([]Match, []Unmatched, []Orphaned)
```

## Matching Logic

A mapping record matches a changed spec node when:
- `record.SpecNodeID` matches the change's spec ID key (e.g., `"module/3/component/2"`)

This is a direct ID comparison — no string manipulation, no case conversion, no naming convention coupling.

For structural changes (module.json), all mapping records in the affected module are considered impacted.

## Advantages over Path-Based Matching

The previous approach matched by parsing bead labels (`spec_module:...`, `spec_component:...`) and correlating them with change paths via filename conventions. The ID-based approach eliminates:
- Module name → directory name mapping
- Component name → filename slug conversion
- Case manipulation (PascalCase → snake_case)
- Fragile string concatenation to reconstruct paths
