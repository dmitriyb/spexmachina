# Schema Validation Tests

Integration and acceptance tests for ProjectSchema (component 1) and ModuleSchema (component 2). These tests verify that the JSON Schema definitions correctly accept valid specs, reject invalid specs, and enforce all constraints defined in `project.schema.json` and `module.schema.json`.

All scenarios below assume a JSON Schema validator is available (e.g., `santhosh-tekuri/jsonschema` or equivalent). The validator is loaded with the embedded schema and then asked to validate JSON documents. "Passes validation" means zero errors; "fails validation" means one or more structured errors with paths.

## Setup

### Fixtures: Valid Minimal Specs

**minimal_project.json** — the smallest valid project.json:
```json
{
  "name": "minimal",
  "modules": [
    { "id": 1, "name": "core", "path": "core" }
  ]
}
```

**minimal_module.json** — the smallest valid module.json:
```json
{
  "name": "core"
}
```

### Fixtures: Valid Full Specs

**valid_project.json** — exercises every optional field and edge type:
```json
{
  "name": "full-project",
  "description": "A project with all fields populated.",
  "version": "1.0.0",
  "requirements": [
    { "id": 1, "type": "functional", "title": "Req A", "description": "Details." },
    { "id": 2, "type": "non_functional", "title": "Req B", "depends_on": [1] }
  ],
  "modules": [
    { "id": 1, "name": "Alpha", "path": "alpha", "description": "First module." },
    { "id": 2, "name": "Beta", "path": "beta", "requires_module": [1] }
  ]
}
```

**valid_module.json** — exercises every optional field and edge type including `test_sections`:
```json
{
  "name": "validator",
  "description": "Full module fixture.",
  "requirements": [
    { "id": 1, "type": "functional", "title": "R1", "preq_id": 1 },
    { "id": 2, "type": "non_functional", "title": "R2", "depends_on": [1] }
  ],
  "components": [
    { "id": 1, "name": "C1", "content": "arch_c1.md", "implements": [1] },
    { "id": 2, "name": "C2", "uses": [1], "implements": [2] }
  ],
  "impl_sections": [
    { "id": 1, "name": "Impl1", "content": "impl_c1.md", "describes": [1] }
  ],
  "data_flows": [
    { "id": 1, "name": "Flow1", "description": "Data flow.", "content": "flow_main.md", "uses": [1, 2] }
  ],
  "test_sections": [
    { "id": 1, "name": "Test coverage for C1 and C2", "content": "test_components.md", "describes": [1, 2] }
  ]
}
```

### Preconditions

- The JSON Schema validator is initialized with the schema loaded from `ProjectSchema()` or `ModuleSchema()` respectively.
- Validation is performed against the 2020-12 draft vocabulary (matching the `$schema` declaration in both schema files).

## Scenarios

### S1: Minimal project.json passes validation

**Input:** `minimal_project.json` (see Setup)
**Expected:** Validation passes. Zero errors.
**Verifies:** Only `name` and `modules` (with at least one entry) are required. All other fields are optional.

### S2: Full project.json passes validation

**Input:** `valid_project.json` (see Setup)
**Expected:** Validation passes. Zero errors.
**Verifies:** All optional fields (`description`, `version`, `requirements`) are accepted when present and correctly typed.

### S3: Minimal module.json passes validation

**Input:** `minimal_module.json` (see Setup)
**Expected:** Validation passes. Zero errors.
**Verifies:** Only `name` is required for a module. Empty modules with no requirements, components, or sections are valid.

### S4: Full module.json passes validation

**Input:** `valid_module.json` (see Setup)
**Expected:** Validation passes. Zero errors.
**Verifies:** All optional arrays (`requirements`, `components`, `impl_sections`, `data_flows`, `test_sections`) are accepted.

### S5: Project missing required field "name" fails

**Input:**
```json
{ "modules": [{ "id": 1, "name": "m", "path": "m/" }] }
```
**Expected:** Validation fails. Error path points to root object, message references missing `name`.

### S6: Project missing required field "modules" fails

**Input:**
```json
{ "name": "orphan" }
```
**Expected:** Validation fails. Error references missing `modules`.

### S7: Project with empty modules array fails

**Input:**
```json
{ "name": "empty-modules", "modules": [] }
```
**Expected:** Validation fails. Error references `modules` violating `minItems: 1`.

### S8: Module missing required field "name" fails

**Input:**
```json
{ "components": [{ "id": 1, "name": "C" }] }
```
**Expected:** Validation fails. Error references missing `name` at root.

### S9: Requirement missing required fields fails

**Input (in module context):**
```json
{
  "name": "bad-req",
  "requirements": [{ "id": 1, "title": "No type field" }]
}
```
**Expected:** Validation fails. Error references missing `type` in `requirements/0`.

**Input (requirement missing id):**
```json
{
  "name": "bad-req",
  "requirements": [{ "type": "functional", "title": "No id" }]
}
```
**Expected:** Validation fails. Error references missing `id` in `requirements/0`.

### S10: Wrong type for ID field fails

**Input (string ID in module declaration):**
```json
{
  "name": "p",
  "modules": [{ "id": "one", "name": "m", "path": "m/" }]
}
```
**Expected:** Validation fails. Error references `modules/0/id` with type mismatch (expected integer, got string).

**Input (float ID in component):**
```json
{
  "name": "m",
  "components": [{ "id": 1.5, "name": "C" }]
}
```
**Expected:** Validation fails. Error references `components/0/id` — JSON Schema integer type rejects non-whole numbers.

### S11: Invalid requirement type enum fails

**Input:**
```json
{
  "name": "m",
  "requirements": [{ "id": 1, "type": "performance", "title": "R" }]
}
```
**Expected:** Validation fails. Error references `requirements/0/type` — value `"performance"` is not in enum `["functional", "non_functional"]`.

### S12: Extra fields rejected by additionalProperties:false

**Input (project level):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "author": "unknown"
}
```
**Expected:** Validation fails. Error references root object, additional property `author` not allowed.

**Input (nested in module declaration):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/", "priority": "high" }]
}
```
**Expected:** Validation fails. Error references `modules/0`, additional property `priority` not allowed.

**Input (nested in component):**
```json
{
  "name": "m",
  "components": [{ "id": 1, "name": "C", "status": "done" }]
}
```
**Expected:** Validation fails. Error references `components/0`, additional property `status` not allowed.

**Input (nested in test_section):**
```json
{
  "name": "m",
  "test_sections": [{ "id": 1, "name": "T", "priority": "P1" }]
}
```
**Expected:** Validation fails. Error references `test_sections/0`, additional property `priority` not allowed.

### S13: ID below minimum (0 or negative) fails

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 0, "name": "C" }]
}
```
**Expected:** Validation fails. Error references `components/0/id` — value 0 is below `minimum: 1`.

**Input (negative ID):**
```json
{
  "name": "p",
  "modules": [{ "id": -1, "name": "m", "path": "m/" }]
}
```
**Expected:** Validation fails. Error references `modules/0/id` — value -1 is below `minimum: 1`.

### S14: Empty string for name fails (minLength: 1)

**Input (project):**
```json
{ "name": "", "modules": [{ "id": 1, "name": "m", "path": "m/" }] }
```
**Expected:** Validation fails. Error references `name` — empty string violates `minLength: 1`.

**Input (module name within project):**
```json
{ "name": "p", "modules": [{ "id": 1, "name": "", "path": "m/" }] }
```
**Expected:** Validation fails. Error references `modules/0/name`.

### S15: depends_on with duplicate items fails (uniqueItems: true)

**Input:**
```json
{
  "name": "m",
  "requirements": [
    { "id": 1, "type": "functional", "title": "R1" },
    { "id": 2, "type": "functional", "title": "R2", "depends_on": [1, 1] }
  ]
}
```
**Expected:** Validation fails. Error references `requirements/1/depends_on` — duplicate items violate `uniqueItems: true`.

### S16: Retired project-level node types are rejected

**Input (a project carrying the retired `milestones` array):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "milestones": []
}
```
**Expected:** Validation fails with an unknown-property error at the root: `milestones` and `test_plan` were removed from the project schema along with their `$defs`, and root-level `additionalProperties: false` rejects either one. Same for `"test_plan": {}`. This fixture draws a second, independent error from its `"id": 1`, since a module id is an identity hash string — so the assertion is on the unknown-property error, not on the error count.

### S17: test_sections validates correctly in module.json

**Input (valid test_sections with full fields):**
```json
{
  "name": "m",
  "test_sections": [
    { "id": 1, "name": "Unit tests", "content": "test_unit.md", "describes": [1, 2] }
  ]
}
```
**Expected:** Validation passes. All fields on test_section are correctly typed.

**Input (test_section missing required name):**
```json
{
  "name": "m",
  "test_sections": [{ "id": 1 }]
}
```
**Expected:** Validation fails. Error references `test_sections/0`, missing required `name`.

### S18: Go type round-trip preserves all fields

**Input:** Unmarshal `valid_project.json` into `schema.Project`, then marshal back to JSON, then unmarshal again.
**Expected:** All fields are identical across the round-trip. Specifically:
- `proj.Name == proj2.Name`
- `len(proj.Modules) == len(proj2.Modules)`
- `proj.Modules[1].RequiresModule` matches
- `proj.Requirements[1].DependsOn` matches

Same pattern for `schema.ModuleSpec` with `valid_module.json`:
- `mod.Components[0].Implements` matches
- `mod.ImplSections[0].Describes` matches
- `mod.DataFlows[0].Uses` matches

## Edge Cases

### E1: Empty optional arrays are valid

**Input (project):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "requirements": []
}
```
**Expected:** Validation passes. Empty arrays satisfy the schema — only `modules` has `minItems: 1`.

**Input (module):**
```json
{
  "name": "m",
  "requirements": [],
  "components": [],
  "impl_sections": [],
  "data_flows": [],
  "test_sections": []
}
```
**Expected:** Validation passes.

### E2: Boundary ID value (minimum = 1)

**Input:** Any node with `"id": 1` — passes. Any node with `"id": 0` — fails.
This is the boundary test for the `minimum: 1` constraint on all ID fields.

### E3: Large ID values

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 2147483647, "name": "MaxInt" }]
}
```
**Expected:** Validation passes. JSON Schema integer has no upper bound by default. The Go `int` type on the struct will also accept this value.

### E4: Non-integer number in ID field

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 1.0, "name": "C" }]
}
```
**Expected:** Behavior depends on validator. In JSON Schema draft 2020-12, `1.0` is a valid integer (it has no fractional part). Most validators accept this. Go's `json.Unmarshal` into `int` also accepts `1.0`. This is an edge case to be aware of, not necessarily a failure.

### E5: Null values for optional fields

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "description": null
}
```
**Expected:** Validation fails. The schema defines `description` as `"type": "string"`, which does not include null. To allow null, the schema would need `"type": ["string", "null"]`.

### E6: Wrong top-level type

**Input:** `"just a string"` validated against project schema.
**Expected:** Validation fails. Schema requires `"type": "object"`.

**Input:** `[]` validated against module schema.
**Expected:** Validation fails. Schema requires `"type": "object"`.

### E7: depends_on referencing IDs that do not exist in the array

**Input:**
```json
{
  "name": "m",
  "requirements": [
    { "id": 1, "type": "functional", "title": "R1", "depends_on": [999] }
  ]
}
```
**Expected:** Validation passes at the schema level. JSON Schema does not enforce referential integrity — that is the validator module's responsibility. This edge case documents the boundary between schema validation and structural validation.

### S19: Priority field accepted on project requirements

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "requirements": [
    { "id": 1, "type": "functional", "title": "R", "priority": 1 }
  ]
}
```
**Expected:** Validation passes. The `priority` field (integer 0-4) is accepted on project requirements.

**Input (priority out of range):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "requirements": [
    { "id": 1, "type": "functional", "title": "R", "priority": 5 }
  ]
}
```
**Expected:** Validation fails. `priority` value 5 exceeds `maximum: 4`.

**Input (negative priority):**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "requirements": [
    { "id": 1, "type": "functional", "title": "R", "priority": -1 }
  ]
}
```
**Expected:** Validation fails. `priority` value -1 is below `minimum: 0`.

### S20: preq_id required on module requirements

**Input (module requirement without preq_id):**
```json
{
  "name": "m",
  "requirements": [
    { "id": 1, "type": "functional", "title": "R" }
  ]
}
```
**Expected:** Validation fails. Error references `requirements/0`, missing required `preq_id`.

**Input (module requirement with preq_id):**
```json
{
  "name": "m",
  "requirements": [
    { "id": 1, "type": "functional", "title": "R", "preq_id": 1 }
  ]
}
```
**Expected:** Validation passes. `preq_id` satisfies the required constraint.

### E8: preq_id present in project-level requirement

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "requirements": [
    { "id": 1, "type": "functional", "title": "R", "preq_id": 5 }
  ]
}
```
**Expected:** Validation fails. The project-level requirement definition does not include `preq_id` (it is only in the module-level definition), and `additionalProperties: false` rejects it. This verifies that the two requirement definitions are correctly separated.

### S21: Sections array accepted in project.json

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "sections": [
    { "id": 1, "name": "delivery", "type": "coupled", "versioning": { "scheme": "semver" } }
  ]
}
```
**Expected:** Validation passes. The `sections` array is accepted. Each section requires `id`, `name`, `type` in the envelope. Additional properties (`versioning`) are allowed — they are freeform content validated by the coupled module's `section.schema.json`, not by the project schema.

### S22: Section missing required envelope field fails

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "sections": [
    { "id": 1, "type": "coupled" }
  ]
}
```
**Expected:** Validation fails. Error references `sections/0`, missing required `name`.

### S23: Section with invalid ID type fails

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "sections": [
    { "id": "one", "name": "delivery", "type": "coupled" }
  ]
}
```
**Expected:** Validation fails. Error references `sections/0/id` — expected integer, got string.

### S24: Empty sections array passes

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "sections": []
}
```
**Expected:** Validation passes. Empty sections array is valid.

### E11: Section with arbitrary content properties passes schema validation

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": 1, "name": "m", "path": "m/" }],
  "sections": [
    {
      "id": 1,
      "name": "performance",
      "type": "coupled",
      "budgets": [{ "metric": "p99_latency", "threshold_ms": 200 }],
      "monitoring": { "dashboard": "grafana.internal/perf" }
    }
  ]
}
```
**Expected:** Validation passes. The project schema allows additional properties on section entries beyond the envelope. Content validation is delegated to the coupled module's `section.schema.json`.
