# Test Schema Definitions

## Implementation Approach

Test-related schema additions follow the same patterns as the existing `impl_sections` and `data_flows` definitions.

### test_sections (module.schema.json)

Added as a top-level optional array in module.json, parallel to `impl_sections`:

```json
"test_sections": {
  "type": "array",
  "items": { "$ref": "#/$defs/test_section" }
}
```

The `test_section` definition mirrors `impl_section`:
- `id` (integer >= 1, required) — unique within the test_sections array
- `name` (string, required) — test section name
- `content` (string, optional) — path to `test_*.md` file relative to module directory
- `describes` (integer[], optional) — component IDs this test section covers

The `describes` edge reuses the same semantics as `impl_sections.describes`: it points to component IDs within the same module. This enables the validator to check test coverage (every component should be described by at least one test_section).

### test_plan (project.schema.json)

Added as a top-level optional object in project.json:

```json
"test_plan": {
  "type": "object",
  "properties": {
    "scenarios": {
      "type": "array",
      "items": { "$ref": "#/$defs/test_scenario" }
    }
  }
}
```

The `test_scenario` definition:
- `id` (integer >= 1, required) — unique within the scenarios array
- `name` (string, required) — scenario name
- `description` (string, optional) — scenario description
- `content` (string, optional) — path to `test_*.md` file relative to spec root
- `modules` (integer[], optional) — module IDs involved in this cross-module scenario

The `modules` edge is analogous to `milestones.groups` — it references module IDs from the project's modules array.

## Key Decisions

### test_plan as object, not array

`test_plan` is an object with a `scenarios` array rather than a bare array. This leaves room for future fields (e.g., `strategy`, `coverage_threshold`) without a breaking schema change.

### Content path conventions

Test content files follow the pattern `test_<snake_name>.md`, consistent with `arch_*.md`, `impl_*.md`, and `flow_*.md`. Project-level test scenario content lives in the spec root directory (alongside `project.json`), while module-level test content lives in the module directory.
