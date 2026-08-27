# Schema Validation Tests

Integration and acceptance tests for ProjectSchema (component 1) and ModuleSchema (component 2). These tests verify that the JSON Schema definitions correctly accept valid specs, reject invalid specs, and enforce all constraints. Both documents are composed from the resolved profile at load time; every scenario below runs against the schemas composed from the default profile, which are golden-tested elsewhere to equal the previous static documents, so the assertions here hold byte-for-byte identically to the pre-profile behaviour.

All scenarios below assume a JSON Schema validator is available (e.g., `santhosh-tekuri/jsonschema` or equivalent). The validator is loaded with the embedded schema and then asked to validate JSON documents. "Passes validation" means zero errors; "fails validation" means one or more structured errors with paths.

## Setup

### Fixtures: Valid Minimal Specs

**minimal_project.json** — the smallest valid project.json:
```json
{
  "name": "minimal",
  "modules": [
    { "id": "000000000001", "name": "core", "path": "core" }
  ]
}
```

All ids in these fixtures are 12-character lowercase hex identity-hash strings, written as `000000000001`, `000000000002`, … for readability — an integer id fails the pattern (S10), so a fixture asserting "passes" can never carry one.

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
    { "id": "000000000001", "type": "functional", "title": "Req A", "description": "Details." },
    { "id": "000000000002", "type": "non_functional", "title": "Req B", "depends_on": ["000000000001"] }
  ],
  "modules": [
    { "id": "000000000001", "name": "Alpha", "path": "alpha", "description": "First module." },
    { "id": "000000000002", "name": "Beta", "path": "beta", "requires_module": ["000000000001"] }
  ]
}
```

**valid_module.json** — exercises every optional field and edge type including `test_sections` and `apis`:
```json
{
  "name": "validator",
  "description": "Full module fixture.",
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R1", "preq_id": "000000000001" },
    { "id": "000000000002", "type": "non_functional", "title": "R2", "preq_id": "000000000001", "depends_on": ["000000000001"] }
  ],
  "components": [
    { "id": "000000000001", "name": "C1", "content": "arch_c1.md", "implements": ["000000000001"] },
    { "id": "000000000002", "name": "C2", "content": "arch_c2.md", "uses": ["000000000001"], "implements": ["000000000002"] }
  ],
  "data_flows": [
    { "id": "000000000001", "name": "Flow1", "description": "Data flow.", "content": "flow_main.md", "uses": ["000000000001", "000000000002"] }
  ],
  "test_sections": [
    { "id": "000000000001", "name": "Test coverage for C1 and C2", "content": "test_components.md", "describes": ["000000000001", "000000000002"] }
  ],
  "apis": [
    { "id": "000000000001", "name": "validator run", "description": "Entry point.", "provided_by": ["000000000001"], "group": "cli" }
  ]
}
```

Every module-context requirement fixture below carries a `preq_id` unless the scenario is about its absence — `preq_id` is required (S20), so an input omitting it incidentally would draw an error the scenario does not assert on.

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
**Verifies:** All optional arrays (`requirements`, `components`, `data_flows`, `test_sections`, `apis`) are accepted.

### S5: Project missing required field "name" fails

**Input:**
```json
{ "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }] }
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
{ "components": [{ "id": "000000000001", "name": "C", "content": "arch_c.md" }] }
```
**Expected:** Validation fails. Error references missing `name` at root.

### S9: Requirement missing required fields fails

**Input (in module context):**
```json
{
  "name": "bad-req",
  "requirements": [{ "id": "000000000001", "title": "No type field", "preq_id": "000000000001" }]
}
```
**Expected:** Validation fails. Error references missing `type` in `requirements/0`.

**Input (requirement missing id):**
```json
{
  "name": "bad-req",
  "requirements": [{ "type": "functional", "title": "No id", "preq_id": "000000000001" }]
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
**Expected:** Validation fails. Error references `modules/0/id` — `"one"` does not match the identity-hash pattern `^[0-9a-f]{12}$`. An integer `id` fails the same way, on type: the field is a string.

**Input (float ID in component):**
```json
{
  "name": "m",
  "components": [{ "id": 1.5, "name": "C", "content": "arch_c.md" }]
}
```
**Expected:** Validation fails. Error references `components/0/id` — the field is a string, so any number fails on type; there is no numeric leniency to reach.

### S11: Invalid requirement type enum fails

**Input:**
```json
{
  "name": "m",
  "requirements": [{ "id": "000000000001", "type": "performance", "title": "R", "preq_id": "000000000001" }]
}
```
**Expected:** Validation fails. Error references `requirements/0/type` — value `"performance"` is not in enum `["functional", "non_functional"]`.

### S12: Extra fields rejected by additionalProperties:false

**Input (project level):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "author": "unknown"
}
```
**Expected:** Validation fails. Error references root object, additional property `author` not allowed.

**Input (nested in module declaration):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/", "priority": "high" }]
}
```
**Expected:** Validation fails. Error references `modules/0`, additional property `priority` not allowed.

**Input (nested in component):**
```json
{
  "name": "m",
  "components": [{ "id": "000000000001", "name": "C", "content": "arch_c.md", "status": "done" }]
}
```
**Expected:** Validation fails. Error references `components/0`, additional property `status` not allowed.

**Input (nested in test_section):**
```json
{
  "name": "m",
  "test_sections": [{ "id": "000000000001", "name": "T", "content": "test_t.md", "priority": "P1" }]
}
```
**Expected:** Validation fails. Error references `test_sections/0`, additional property `priority` not allowed.

### S13: ID below minimum (0 or negative) fails

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 0, "name": "C", "content": "arch_c.md" }]
}
```
**Expected:** Validation fails. Error references `components/0/id` — `0` is not a string matching `^[a-f0-9]{12}$` (module.schema.json spells the class in that order; project.schema.json's `$defs/identityHash` spells it `^[0-9a-f]{12}$`).

**Input (negative ID):**
```json
{
  "name": "p",
  "modules": [{ "id": -1, "name": "m", "path": "m/" }]
}
```
**Expected:** Validation fails. Error references `modules/0/id` — `-1` is not a string matching `^[0-9a-f]{12}$`; `project.schema.json` constrains module ids by pattern, not by numeric range.

### S14: Empty string for name fails (minLength: 1)

**Input (project):**
```json
{ "name": "", "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }] }
```
**Expected:** Validation fails. Error references `name` — empty string violates `minLength: 1`.

**Input (module name within project):**
```json
{ "name": "p", "modules": [{ "id": "000000000001", "name": "", "path": "m/" }] }
```
**Expected:** Validation fails. Error references `modules/0/name`.

### S15: depends_on with duplicate items fails (uniqueItems: true)

**Input:**
```json
{
  "name": "m",
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R1", "preq_id": "000000000001" },
    { "id": "000000000002", "type": "functional", "title": "R2", "preq_id": "000000000001", "depends_on": ["000000000001", "000000000001"] }
  ]
}
```
**Expected:** Validation fails. Error references `requirements/1/depends_on` — duplicate items violate `uniqueItems: true`.

### S16: Retired project-level node types are rejected

**Input (a project carrying the retired `milestones` array):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "milestones": []
}
```
**Expected:** Validation fails with an unknown-property error at the root: `milestones` and `test_plan` were removed from the project schema along with their `$defs`, and root-level `additionalProperties: false` rejects either one. Same for `"test_plan": {}`.

### S17: test_sections validates correctly in module.json

**Input (valid test_sections with full fields):**
```json
{
  "name": "m",
  "test_sections": [
    { "id": "000000000001", "name": "Unit tests", "content": "test_unit.md", "describes": ["000000000001", "000000000002"] }
  ]
}
```
**Expected:** Validation passes. All fields on test_section are correctly typed.

**Input (test_section missing required name):**
```json
{
  "name": "m",
  "test_sections": [{ "id": "000000000001", "content": "test_t.md" }]
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
- `mod.TestSections[0].Describes` matches
- `mod.DataFlows[0].Uses` matches

## Edge Cases

### E1: Empty optional arrays are valid

**Input (project):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
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
  "data_flows": [],
  "test_sections": []
}
```
**Expected:** Validation passes.

### E2: Boundary ID value (identity-hash length)

**Input:** Any node with `"id": "aabbccddeeff"` — passes. Any node with `"id": "aabbccddeef"` (11 chars) — fails.
This is the boundary test for the 12-hex-character identity pattern every ID field carries — `^[0-9a-f]{12}$` at `project.schema.json`'s `$defs/identityHash`, spelled `^[a-f0-9]{12}$` at each of the 12 pattern sites in `module.schema.json`.

### E3: Numeric ID of any magnitude fails on type

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 2147483647, "name": "MaxInt", "content": "arch_maxint.md" }]
}
```
**Expected:** Validation fails. Error references `components/0/id` — the field is a string matching the identity-hash pattern, so a number fails on type whatever its magnitude. There is no integer id anywhere in the format for a bound to apply to.

### E4: Whole-number float in ID field fails the same way

**Input:**
```json
{
  "name": "m",
  "components": [{ "id": 1.0, "name": "C", "content": "arch_c.md" }]
}
```
**Expected:** Validation fails on `components/0/id`. Draft 2020-12's rule that `1.0` counts as an integer is unreachable here: the field's type is string, so the number never gets as far as an integer-vs-float judgement.

### E5: Null values for optional fields

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
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
    { "id": "000000000001", "type": "functional", "title": "R1", "preq_id": "000000000001", "depends_on": ["deadbeefdead"] }
  ]
}
```
**Expected:** Validation passes at the schema level: `deadbeefdead` is a well-formed identity hash that references nothing, and JSON Schema does not enforce referential integrity — that is the validator module's responsibility. This edge case documents the boundary between schema validation and structural validation.

### S19: Priority field accepted on project requirements

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R", "priority": 1 }
  ]
}
```
**Expected:** Validation passes. The `priority` field (integer 0-4) is accepted on project requirements.

**Input (priority out of range):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R", "priority": 5 }
  ]
}
```
**Expected:** Validation fails. `priority` value 5 exceeds `maximum: 4`.

**Input (negative priority):**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R", "priority": -1 }
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
    { "id": "000000000001", "type": "functional", "title": "R" }
  ]
}
```
**Expected:** Validation fails. Error references `requirements/0`, missing required `preq_id`.

**Input (module requirement with preq_id):**
```json
{
  "name": "m",
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R", "preq_id": "000000000001" }
  ]
}
```
**Expected:** Validation passes. `preq_id` satisfies the required constraint.

### E8: preq_id present in project-level requirement

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "requirements": [
    { "id": "000000000001", "type": "functional", "title": "R", "preq_id": "000000000005" }
  ]
}
```
**Expected:** Validation fails. The project-level requirement definition does not include `preq_id` (it is only in the module-level definition), and `additionalProperties: false` rejects it. This verifies that the two requirement definitions are correctly separated.

### S21: Sections array accepted in project.json

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "sections": [
    { "id": "000000000001", "name": "delivery", "type": "coupled", "versioning": { "scheme": "semver" } }
  ]
}
```
**Expected:** Validation passes. The `sections` array is accepted. Each section requires `id`, `name`, `type` in the envelope. Additional properties (`versioning`) are allowed — they are freeform content validated by the coupled module's `section.schema.json`, not by the project schema.

### S22: Section missing required envelope field fails

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "sections": [
    { "id": "000000000001", "type": "coupled" }
  ]
}
```
**Expected:** Validation fails. Error references `sections/0`, missing required `name`.

### S23: Section with invalid ID type fails

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "sections": [
    { "id": "one", "name": "delivery", "type": "coupled" }
  ]
}
```
**Expected:** Validation fails. Error references `sections/0/id` — `"one"` does not match the identity-hash pattern `^[0-9a-f]{12}$`.

### S24: Empty sections array passes

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "sections": []
}
```
**Expected:** Validation passes. Empty sections array is valid.

### S25: Derivation field accepted on project requirements, pending only

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "aabbccddeeff", "name": "m", "path": "m/" }],
  "requirements": [
    { "id": "aabbccddee00", "type": "functional", "title": "R", "priority": 1, "derivation": "pending" }
  ]
}
```
**Expected:** Validation passes. `derivation` is an optional property on a project requirement, and `"pending"` is its only permitted value.

**Input (unknown value):** the same document with `"derivation": "done"`.
**Expected:** Validation fails. Error references `requirements/0/derivation` — the value is not in the single-entry enum. Same for `"derivation": ""` and any other string.

**Input (wrong type):** the same document with `"derivation": true`.
**Expected:** Validation fails on `requirements/0/derivation`.

### S26: Derivation field rejected on module requirements

**Input:**
```json
{
  "name": "m",
  "requirements": [
    { "id": "aabbccddee01", "type": "functional", "title": "R", "preq_id": "aabbccddee00", "derivation": "pending" }
  ]
}
```
**Expected:** Validation fails. The module-level requirement definition does not include `derivation` — a module requirement derives by construction through its required `preq_id` — and `additionalProperties: false` rejects it. Like E8 in the opposite direction, this verifies the two requirement definitions stay correctly separated.

### S27: Profile-declared array accepted, undeclared array still rejected

**Steps:** Compose the module schema from a profile that declares an additional module-scoped `endpoint` type with plural key `endpoints` and a required content leaf. Validate two documents against the composed schema:

**Input (declared array):**
```json
{
  "name": "m",
  "endpoints": [
    { "id": "000000000001", "name": "GET /things", "content": "endpoint_things.md" }
  ]
}
```
**Expected:** Validation passes — the profile-supplied array gets the same envelope constraints (identity-hash id, non-empty name, required content) the built-in types get.

**Input (undeclared array):** the same document with `"widgets": []` added.
**Expected:** Validation fails at the root — `additionalProperties: false` is part of the fixed frame, so an array no profile declares is rejected exactly as it always was.

**Verifies:** The frame-plus-vocabulary split — the profile supplies array properties, the frame supplies everything else, and declaring a type is the only way to open an array.

### E11: Section with arbitrary content properties passes schema validation

**Input:**
```json
{
  "name": "p",
  "modules": [{ "id": "000000000001", "name": "m", "path": "m/" }],
  "sections": [
    {
      "id": "000000000001",
      "name": "performance",
      "type": "coupled",
      "budgets": [{ "metric": "p99_latency", "threshold_ms": 200 }],
      "monitoring": { "dashboard": "grafana.internal/perf" }
    }
  ]
}
```
**Expected:** Validation passes. The project schema allows additional properties on section entries beyond the envelope. Content validation is delegated to the coupled module's `section.schema.json`.
