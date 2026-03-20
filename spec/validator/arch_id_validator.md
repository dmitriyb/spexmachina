# IDValidator

Validates ID uniqueness, cross-reference integrity, mandatory `preq_id` on module requirements, and `priority` presence on project requirements.

## Responsibilities

### ID Uniqueness
- Check that all IDs within each array are unique (requirements, components, impl_sections, data_flows, modules, milestones)
- Report duplicate IDs with their location

### Cross-Reference Integrity
- `implements`: component references → requirement IDs within the same module
- `uses` (component): component references → component IDs within the same module
- `describes`: impl_section references → component IDs within the same module
- `uses` (data_flow): data_flow references → component IDs within the same module
- `depends_on`: requirement references → requirement IDs within the same scope
- `requires_module`: module references → module IDs in project.json
- `groups`: milestone references → module IDs in project.json
- `preq_id`: module requirement → project requirement IDs (must exist)
- `describes` (test_section): test_section references → component IDs within the same module

### Mandatory preq_id
- Every module requirement must have a `preq_id` that references a valid project requirement ID
- Missing `preq_id` is reported as an error (not a warning)

### Priority Presence
- Every project requirement must have a `priority` field (integer 0-4)
- Missing or out-of-range priority is reported as an error

## Interface

```go
func CheckIDs(project *schema.Project, modules map[string]*schema.Module) []ValidationError
```

## Error Format

Each error includes the source node (type, ID, module) and the dangling reference ID or missing field.
