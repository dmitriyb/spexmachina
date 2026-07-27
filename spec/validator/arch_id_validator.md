# IDValidator

Validates identity-hash uniqueness, cross-reference integrity, mandatory `preq_id` on module requirements, and `priority` presence on project requirements.

## Responsibilities

### ID Uniqueness
- Check that every identity hash ID is unique within the array that contains it. In project.json the checked arrays are `requirements`, `modules`, `milestones`, `test_plan.scenarios` and `sections`; in each module.json they are `requirements`, `components`, `impl_sections`, `data_flows` and `test_sections`
- One array is not yet covered: module.json `apis`. The schema declares an api's `id` unique within the apis array, but nothing enforces it — duplicate api IDs pass validation today
- Uniqueness is checked by tallying each hash in a per-array set of strings — any hash counted more than once is reported with its array location and the offending hash
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

Given the path to a spec directory, the checker loads project.json and every module.json and returns a flat list of validation entries — empty when the spec is clean. If the spec cannot be loaded, the load failures are returned and no further checks run. The checker never mutates the spec and never writes output: aggregation, sorting and formatting belong to ErrorReporter.

Uniqueness runs before cross-reference resolution, and the two do not mix in a single run. When any duplicate is found, the checker returns the duplicate entries alone and skips reference resolution — a reference cannot be unambiguously resolved while two nodes share a hash, so resolving anyway would produce misleading errors.

Modules are visited in sorted name order, so the same spec always produces the same entries in the same sequence.

Every ID and cross-reference field is a string end to end — the loaded spec carries identity hashes as text, so the checker compares them directly with no conversion, parsing or decomposition.

## Error Format

Each error includes the source node (type, identity hash, module) and the dangling reference hash or missing field. Identity hashes are short (12 characters) so error messages remain readable even when several appear in one report.
