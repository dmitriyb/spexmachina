# IDValidator

Validates identity-hash uniqueness, cross-reference integrity, mandatory `preq_id` on module requirements, and `priority` presence on project requirements.

## Responsibilities

### ID Uniqueness
- Check that all identity hash IDs within each array are unique (requirements, components, impl_sections, data_flows, test_sections, modules, milestones, sections, test_plan scenarios)
- Uniqueness is checked by inserting into a `map[string]bool` per array — duplicates fail the insert and are reported with their array location and the offending hash
- Collisions across distinct logical nodes are mathematically improbable in the 48-bit hash space, but the validator still checks them so hand-edited or hand-merged files cannot smuggle a stale ID into a new node

### Cross-Reference Integrity
All references are identity hash strings, validated by string set membership against the appropriate per-array set:

- `implements`: component → requirement identity hashes within the same module
- `uses` (component): component → component identity hashes within the same module
- `describes`: impl_section → component identity hashes within the same module
- `uses` (data_flow): data_flow → component identity hashes within the same module
- `depends_on`: requirement → requirement identity hashes within the same scope
- `requires_module`: module → module identity hashes in project.json
- `groups`: milestone → module identity hashes in project.json
- `preq_id`: module requirement → project requirement identity hash (must exist)
- `describes` (test_section): test_section → component identity hashes within the same module
- `modules` (test_plan): test_plan scenario → module identity hashes in project.json

There is no integer parsing, no path decomposition (`module/N/component/M`), and no comparison across types — every check is a single `set[hash]` lookup.

### Mandatory preq_id
- Every module requirement must have a non-empty `preq_id` field whose value is the identity hash of an existing project requirement
- Missing or empty `preq_id` is reported as an error
- A `preq_id` that does not match any project requirement is reported as a dangling reference

### Priority Presence
- Every project requirement must have a `priority` field (integer 0-4)
- Missing or out-of-range priority is reported as an error

## Interface

```go
func CheckIDs(project *schema.Project, modules map[string]*schema.Module) []ValidationError
```

The `Project` and `Module` Go structs use `string` rather than `int` for every ID and cross-reference field, so the validator can compare them directly without conversion.

## Error Format

Each error includes the source node (type, identity hash, module) and the dangling reference hash or missing field. Identity hashes are short (12 characters) so error messages remain readable even when several appear in one report.
