# CompletenessChecker

Validates that requirement leaf changes in the diff are accompanied by corresponding component content leaf changes. Ensures that spec edits are complete before the impact pipeline processes them.

## Responsibilities

- Read the current spec graph (project.json, module.json) to resolve which components implement which requirements
- For each requirement leaf change in the diff, check whether the implementing components' content leaves also changed
- For meta-only changes (no requirement leaf changes), check whether component content leaves changed
- Report errors for incomplete edits

## Checks

All checks operate on identity hashes. The diff change keys, the `implements` arrays in `module.json`, and the keys in the resolved component map are all hex strings — comparison is exact-match.

### Modified requirement → implementing component content must change

When a requirement leaf changed (a change whose `Key` is the requirement's identity hash and whose `NodeType` is `requirement`), resolve which components implement that requirement by scanning every component's `implements` array for that hash. For each such component, its content leaf must also appear as a change in the diff (looked up by the component's identity hash).

For each component whose content leaf did NOT change, report an error:

```json
{
  "type": "incomplete_change",
  "message": "requirement 'Match changed nodes to beads' (impact) description changed but component NodeMatcher content leaf unchanged",
  "path": "7c5e2fa1b3d8",
  "related": ["a1b2c3d4e5f6"]
}
```

The `path` and `related` fields carry identity hashes — the same values used everywhere else in the pipeline.

### Added requirement → must be implemented and component content must change

When a requirement leaf is added, check that the requirement is implemented by at least one component (i.e., its identity hash appears in some `implements` array) and that those components' content leaves also changed. If no component implements it, report an error. For each implementing component whose content leaf did NOT change, report an error.

### Removed requirement → no component may still reference it

When a requirement leaf is removed, check that no component in the current module.json still has the removed requirement's identity hash in its `implements` array. For each component that still references it, report an error.

### Project-level requirement changed → chain must propagate

When a project-level requirement leaf changed, find all module requirements with `preq_id` equal to the project requirement's identity hash. If none exist, report an error. For each such module requirement, find all implementing components. For each component whose content leaf did NOT change, report an error. Same logic applies for added and removed project-level requirements.

### Component edges changed → component content must change

When `meta/<module-hash>` is modified but no requirement leaves in that module changed, the meta change is due to non-requirement modifications (component edges, module description, etc.). For each component in the current module.json, check whether its content leaf also changed (lookup by the component's identity hash). For each component whose content leaf did NOT change, report an error.

## Interface

```go
func CheckCompleteness(changes []ClassifiedChange, specDir string) []DiffError
```

Takes the classified changes and reads the current spec directory to resolve requirement-to-component edges. Returns errors for incomplete edits. Returns nil if all changes are complete.
