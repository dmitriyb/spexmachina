# CompletenessChecker

Validates that requirement leaf changes in the diff are accompanied by corresponding component content leaf changes. Ensures that spec edits are complete before the impact pipeline processes them.

## Responsibilities

- Read the current spec graph (project.json, module.json) to resolve which components implement which requirements
- For each requirement leaf change in the diff, check whether the implementing components' content leaves also changed
- For meta-only changes (no requirement leaf changes), check whether component content leaves changed
- Report errors for incomplete edits

## Checks

### Modified requirement → implementing component content must change

When a requirement leaf changed (`module/X/requirement/Y` modified), resolve which components implement that requirement via `implements` edges. For each such component, its content leaf must also appear as a change in the diff.

For each component whose content leaf did NOT change, report an error:

```json
{
  "type": "incomplete_change",
  "message": "requirement 2 (impact) description changed but component NodeMatcher content leaf unchanged",
  "path": "module/4/requirement/2",
  "related": ["module/4/component/2"]
}
```

### Added requirement → must be implemented and component content must change

When a requirement leaf is added (`module/X/requirement/Y` added), check that the requirement is implemented by components and that those components' content leaves also changed. If no component implements it, report an error. For each implementing component whose content leaf did NOT change, report an error.

### Removed requirement → no component may still reference it

When a requirement leaf is removed (`module/X/requirement/Y` removed), check that no component in the current module.json still has the removed requirement ID in its `implements` array. For each component that still references it, report an error.

### Project-level requirement changed → chain must propagate

When a project-level requirement leaf changed (`project/requirement/Y`), find all module requirements with `preq_id == Y`. If none exist, report an error. For each such module requirement, find all implementing components. For each component whose content leaf did NOT change, report an error. Same logic applies for added and removed project-level requirements.

### Component edges changed → component content must change

When `module/X/meta` is modified but no requirement leaves in module X changed, the meta change is due to non-requirement modifications (component edges, module description, etc.). For each component in the current module.json, check whether its content leaf also changed. For each component whose content leaf did NOT change, report an error.

## Interface

```go
func CheckCompleteness(changes []ClassifiedChange, specDir string) []DiffError
```

Takes the classified changes and reads the current spec directory to resolve requirement-to-component edges. Returns errors for incomplete edits. Returns nil if all changes are complete.
