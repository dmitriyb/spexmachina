# Change Completeness Algorithm

## Approach

Compare current and snapshot versions of changed JSON files to identify what specifically changed (which requirement, which edge). Then cross-reference against the leaf-level changes in the same diff.

## Algorithm

1. Collect all structural changes from the diff (`impact == Structural`)
2. For each structural change, load the current and snapshot versions of the JSON file:
   - `project/meta` → compare current and snapshot `project.json`
   - `module/X/meta` → compare current and snapshot `spec/<module>/module.json`
3. Diff the JSON to identify what changed:
   - Requirements: added, removed, or description text changed
   - Components: `implements` or `uses` arrays changed
   - Module declarations: `requires_module` changed
4. For each detected JSON change, resolve the expected leaf changes:
   - Requirement changed → find components with `implements` containing that req ID → expect their content leaves in the diff
   - Component edges changed → expect that component's content leaf in the diff
5. Check if the expected leaf changes are present in the diff's `arch_impl`/`impl_only` changes
6. Report errors for any missing expected changes

## JSON diffing

Compare field-by-field rather than full JSON diff. For requirements, compare `description` text. For components, compare `implements` and `uses` arrays. Ignore non-semantic changes (whitespace, field ordering).

```go
type metaDiff struct {
    ChangedReqs  []reqChange      // requirements with changed description
    ChangedEdges []edgeChange     // components with changed implements/uses
    AddedReqs    []int            // new requirement IDs (covered by validate, not an error here)
    RemovedReqs  []int            // removed requirement IDs (covered by validate)
}
```

Added and removed requirements are state issues caught by `spex validate`. The completeness checker only flags **modified** requirements that lack corresponding leaf changes.

## Error output

Errors are returned as `[]DiffError` and included in the diff JSON output:

```go
type DiffError struct {
    Type    string   `json:"type"`
    Message string   `json:"message"`
    Path    string   `json:"path"`
    Related []string `json:"related"`
}
```
