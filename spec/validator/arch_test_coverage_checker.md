# TestCoverageChecker

Validates that every component in each module is described by at least one `test_section`.

## Responsibilities

- Walk all modules and their components
- For each component, check if any `test_section` in the same module has the component's ID in its `describes` array
- Report uncovered components as validation errors

## Interface

```go
func CheckTestCoverage(specDir string, project *schema.Project) []ValidationError
```

## Behavior

1. For each module in `project.json`, read its `module.json`
2. Collect all component IDs in the module
3. Collect all component IDs referenced by `test_sections[].describes`
4. Any component ID not in the describes set is uncovered — emit a validation error with the module name, component ID, and component name

## Error Format

Each error includes:
- Module path and name
- Component ID and name
- Message: `"component <name> (id:<id>) has no test_section coverage"`

## Edge Cases

- Module with no components: no errors (nothing to cover)
- Module with no `test_sections` array: every component is uncovered
- `test_section` with empty `describes`: valid but covers no components
- Component covered by multiple test_sections: valid (no uniqueness constraint)
