# IDValidator

Validates ID uniqueness and cross-reference integrity across the spec.

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
- `preq_id`: module requirement → project requirement IDs

## Interface

```go
func CheckIDs(project *schema.Project, modules map[string]*schema.Module) []ValidationError
```

## Error Format

Each error includes the source node (type, ID, module) and the dangling reference ID.
