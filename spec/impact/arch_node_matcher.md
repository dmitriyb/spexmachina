# NodeMatcher

Matches changed spec nodes from the ID-based merkle diff to existing beads using spec node IDs from the mapping file.

## Responsibilities

- Take classified changes (from merkle diff) and bead-to-record mappings
- Match each changed spec node to the bead(s) that reference it via mapping records
- Identify unmatched changes (new spec nodes without beads)
- Identify unmatched beads (beads referencing removed spec nodes)
- Skip structural changes — they do not participate in matching

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

### Structural changes are skipped

Changes with `impact: "structural"` (i.e., `project/meta` or `module/X/meta`) are not matched against bead-map records. They produce no matches, no unmatched entries, and no orphans. MatchNodes filters them out before the matching loop.

Structural changes signal that the JSON envelope changed (a requirement was added, a module dependency was modified, etc.). Bead impact comes from leaf-level changes — components, impl_sections, test_sections, data_flows — which merkle detects independently as `arch_impl` or `impl_only` changes. When requirements change, affected components are expected to be updated too, and those component-level changes are the actual triggers for bead obsolete+create.

The consistency between structural and leaf changes is enforced upstream by `spex validate` (state checks) and `spex diff` (change completeness checks). By the time MatchNodes runs, structural consistency is already guaranteed.

## Advantages over Path-Based Matching

The previous approach matched by parsing bead labels (`spec_module:...`, `spec_component:...`) and correlating them with change paths via filename conventions. The ID-based approach eliminates:
- Module name → directory name mapping
- Component name → filename slug conversion
- Case manipulation (PascalCase → snake_case)
- Fragile string concatenation to reconstruct paths
