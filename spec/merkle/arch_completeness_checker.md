# CompletenessChecker

Validates that structural meta changes in the diff are accompanied by corresponding leaf-level changes. Ensures that spec edits are complete before the impact pipeline processes them.

## Responsibilities

- Read the spec graph (project.json, module.json) to resolve which components implement which requirements
- For each structural meta change in the diff, check whether the affected leaf nodes also changed
- Report errors for incomplete edits (structural change without corresponding leaf changes)

## Checks

### Requirement description changed → implementing component content must change

When `module/X/meta` changed and a requirement's description text changed (detected by comparing current vs snapshot module.json), resolve which components implement that requirement via `implements` edges. At least one of those components' content leaves must also appear as `arch_impl` or `impl_only` changes in the same diff.

If none do, report an error:

```json
{
  "type": "incomplete_change",
  "message": "Requirement 2 (impact) description changed but implementing component NodeMatcher content leaf unchanged",
  "path": "module/4/meta",
  "related": ["module/4/component/2"]
}
```

### Component edges changed → component content must change

When a component's `implements` or `uses` array changed in module.json (detected via meta change), the component's content leaf should also have changed. The component's architecture description should reflect the new edges.

### Project-level requirement changed → module requirement must exist

When `project/meta` changed and a project requirement's description changed, check that at least one module requirement with matching `preq_id` exists AND that the chain continues to a leaf change. If the module requirement exists but no component content changed, report an error.

## Interface

```go
func CheckCompleteness(changes []ClassifiedChange, specDir string, snapshotPath string) []DiffError
```

Takes the classified changes, reads current and snapshot spec state to detect what specifically changed in the meta nodes, and returns errors for incomplete edits. Returns nil if all structural changes are covered.
