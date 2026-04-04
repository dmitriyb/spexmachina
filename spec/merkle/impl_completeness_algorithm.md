# Change Completeness Algorithm

## Approach

Individual requirement leaf nodes in the diff tell us exactly which requirements changed. Cross-reference each changed requirement against the current spec graph (module.json `implements` edges) to verify that implementing components' content leaves also changed.

## Algorithm

1. Build a set of all changed paths from the diff for O(1) lookup
2. Collect all requirement leaf changes from the diff (`NodeType == "requirement"`)
3. For each **modified** module-level requirement (`module/X/requirement/Y`):
   - Read the current `module.json` for module X
   - Find all components whose `implements` array contains requirement Y
   - For each component, check whether `module/X/component/Z` is in the changed paths set
   - For each component whose content leaf did NOT change, emit a `DiffError`
4. For each **added** module-level requirement (`module/X/requirement/Y`):
   - Read the current `module.json` for module X
   - Find all components whose `implements` array contains requirement Y
   - If no component implements it, emit a `DiffError`
   - For each implementing component whose content leaf did NOT change, emit a `DiffError`
5. For each **removed** module-level requirement (`module/X/requirement/Y`):
   - Read the current `module.json` for module X
   - For each component whose `implements` array still contains requirement Y, emit a `DiffError`
6. For each **modified** project-level requirement (`project/requirement/Y`):
   - Read the current `project.json` and all `module.json` files
   - Find all module requirements with `preq_id == Y`
   - If none exist, emit a `DiffError`
   - For each such module requirement, find all implementing components
   - For each component whose content leaf did NOT change, emit a `DiffError`
7. For each **added** project-level requirement (`project/requirement/Y`):
   - Find all module requirements with `preq_id == Y`
   - If none exist, emit a `DiffError`
   - For each such module requirement, find all implementing components
   - For each component whose content leaf did NOT change, emit a `DiffError`
8. For each **removed** project-level requirement (`project/requirement/Y`):
   - For each module requirement that still has `preq_id == Y`, emit a `DiffError`
9. For component edge changes: when `module/X/meta` is modified but no requirement leaves in module X changed, the meta change is due to non-requirement modifications. For each component in the current `module.json`, check whether its content leaf also changed. For each component whose content leaf did NOT change, emit a `DiffError`.

## Resolving module paths

The function receives `specDir` and reads the current `project.json` to map module IDs to directory paths. For a structural change at `module/X/meta`, the module ID X is carried in the `ClassifiedChange.Module` field — use `project.json` modules to find the directory path, then read `module.json` from that directory.

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
