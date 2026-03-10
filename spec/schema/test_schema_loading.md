# Schema Loading Tests

Integration and acceptance tests for SchemaLoader (component 3). The SchemaLoader is the Go package (`schema/schema.go`) that embeds `project.schema.json` and `module.schema.json` via `go:embed` and exposes them through `ProjectSchema()` and `ModuleSchema()` functions.

These tests verify that the embedding works correctly, that the loaded schemas are structurally sound, and that they can be used to validate known-good fixtures.

## Setup

### Build Preconditions

- The `schema` package compiles successfully (`go build ./schema/...`).
- The `go:embed` directive references `project.schema.json` and `module.schema.json`, both of which exist in the `schema/` directory at build time.
- No external file system access is needed at runtime — schemas are baked into the binary.

### Test Fixtures

The following fixture files live in `schema/testdata/`:

- `valid_project.json` — a full project.json with all optional fields populated (requirements, modules with dependencies, milestones, test_plan with scenarios).
- `minimal_project.json` — the smallest valid project.json (`name` + one module).
- `valid_module.json` — a full module.json with all optional arrays populated (requirements with preq_id, components with implements/uses, impl_sections, data_flows, test_sections).
- `minimal_module.json` — the smallest valid module.json (just `name`).

### Dependencies

- `encoding/json` from the Go standard library for JSON parsing.
- A JSON Schema validation library (e.g., `santhosh-tekuri/jsonschema/v6`) for full schema-against-document validation scenarios. If no validation library is available, structural assertions on the parsed schema JSON serve as a fallback.

## Scenarios

### S1: ProjectSchema() loads without error

**Call:** `data, err := schema.ProjectSchema()`
**Expected:**
- `err` is nil.
- `data` is non-empty (length > 0).
- `data` is valid JSON (unmarshals into `map[string]any` without error).

**Verifies:** The `go:embed` directive for `project.schema.json` works and the file content is accessible at runtime.

### S2: ModuleSchema() loads without error

**Call:** `data, err := schema.ModuleSchema()`
**Expected:**
- `err` is nil.
- `data` is non-empty (length > 0).
- `data` is valid JSON (unmarshals into `map[string]any` without error).

**Verifies:** The `go:embed` directive for `module.schema.json` works and the file content is accessible at runtime.

### S3: ProjectSchema() returns valid JSON Schema document

**Call:** `data, _ := schema.ProjectSchema()` then unmarshal to `map[string]any`.
**Expected assertions on parsed content:**
- `$schema` field equals `"https://json-schema.org/draft/2020-12/schema"`.
- `$id` field equals `"https://spexmachina.dev/schema/project.json"`.
- `title` field equals `"Spex Machina Project"`.
- `type` field equals `"object"`.
- `required` array contains `"name"` and `"modules"`.
- `additionalProperties` is `false`.
- `properties` object contains keys: `name`, `description`, `version`, `requirements`, `modules`, `milestones`, `test_plan`.
- `$defs` object contains keys: `requirement`, `module`, `milestone`, `test_scenario`.

**Verifies:** The embedded file is the actual project schema (not a stale copy or wrong file) and includes the `test_plan`/`test_scenario` additions.

### S4: ModuleSchema() returns valid JSON Schema document

**Call:** `data, _ := schema.ModuleSchema()` then unmarshal to `map[string]any`.
**Expected assertions on parsed content:**
- `$schema` field equals `"https://json-schema.org/draft/2020-12/schema"`.
- `$id` field equals `"https://spexmachina.dev/schema/module.json"`.
- `title` field equals `"Spex Machina Module"`.
- `type` field equals `"object"`.
- `required` array contains `"name"`.
- `additionalProperties` is `false`.
- `properties` object contains keys: `name`, `description`, `requirements`, `components`, `impl_sections`, `data_flows`, `test_sections`.
- `$defs` object contains keys: `requirement`, `component`, `impl_section`, `data_flow`, `test_section`.

**Verifies:** The embedded file is the actual module schema and includes the `test_sections`/`test_section` additions.

### S5: Both schemas are independently loadable

**Call:** Load both schemas in sequence:
```go
proj, err1 := schema.ProjectSchema()
mod, err2 := schema.ModuleSchema()
```
**Expected:**
- Both `err1` and `err2` are nil.
- `proj` and `mod` are different byte slices (not the same content).
- The `$id` values differ between the two documents.

**Verifies:** The embed FS correctly serves two distinct files. No cross-contamination between the two schema files.

### S6: Loaded project schema validates the valid_project.json fixture

**Steps:**
1. Load the project schema via `ProjectSchema()`.
2. Compile it into a JSON Schema validator.
3. Read `testdata/valid_project.json`.
4. Validate the fixture against the compiled schema.

**Expected:** Validation passes with zero errors.

**Verifies:** The embedded schema is not only parseable but functionally correct — it accepts a known-good document. This is the key integration point: the loader produces a schema that actually works for validation.

### S7: Loaded module schema validates the valid_module.json fixture

**Steps:**
1. Load the module schema via `ModuleSchema()`.
2. Compile it into a JSON Schema validator.
3. Read `testdata/valid_module.json`.
4. Validate the fixture against the compiled schema.

**Expected:** Validation passes with zero errors.

**Verifies:** Same as S6 but for the module schema path.

### S8: Loaded project schema rejects an invalid document

**Steps:**
1. Load the project schema via `ProjectSchema()`.
2. Compile it into a JSON Schema validator.
3. Validate `{"name": "p"}` (missing required `modules`).

**Expected:** Validation fails with an error referencing the missing `modules` field.

**Verifies:** The schema loaded from embedding actually enforces constraints, not just accepts everything. This is a sanity check that the validator is using the correct schema (not a permissive fallback).

### S9: Loaded module schema rejects an invalid document

**Steps:**
1. Load the module schema via `ModuleSchema()`.
2. Compile it into a JSON Schema validator.
3. Validate `{"components": [{"id": 1, "name": "C"}]}` (missing required `name`).

**Expected:** Validation fails with an error referencing the missing `name` field at root.

**Verifies:** Same as S8 but for the module schema path.

### S10: Loaded schemas validate minimal fixtures

**Steps:**
1. Load both schemas.
2. Validate `testdata/minimal_project.json` against the project schema.
3. Validate `testdata/minimal_module.json` against the module schema.

**Expected:** Both pass validation.

**Verifies:** Minimal documents (only required fields) are accepted. This ensures the schema does not accidentally require optional fields.

### S11: ProjectSchema() is idempotent

**Call:** Invoke `ProjectSchema()` twice in succession.
**Expected:**
- Both calls return nil error.
- Both calls return byte-identical content (`bytes.Equal(data1, data2)` is true).

**Verifies:** The embed FS returns consistent content across multiple reads. No state mutation between calls.

### S12: ModuleSchema() is idempotent

**Call:** Invoke `ModuleSchema()` twice in succession.
**Expected:** Same as S11 — both calls return byte-identical content.

### S13: Go types unmarshal from fixtures validated by loaded schemas

**Steps:**
1. Load module schema, compile validator, validate `testdata/valid_module.json` — confirm it passes.
2. Unmarshal the same fixture into `schema.ModuleSpec`.
3. Assert that `mod.Name` is non-empty, `mod.Components` has entries, `mod.TestSections` has entries (once the Go type is updated for test_sections).

**Expected:** The document that passes schema validation also unmarshals cleanly into the Go types. No field is silently dropped.

**Verifies:** The Go struct types and JSON Schema definitions are in agreement. If a field exists in the schema but not in the Go type (or vice versa), this test will catch the discrepancy.

## Edge Cases

### E1: Schema files are non-trivially sized

**Call:** `data, _ := schema.ProjectSchema()`
**Expected:** `len(data) > 100` (the schema is a substantial JSON document, not a stub or empty object).

Same for `ModuleSchema()`.

**Verifies:** The embed did not silently include a truncated or placeholder file.

### E2: Schema content starts with valid JSON object opening

**Call:** `data, _ := schema.ProjectSchema()`
**Expected:** `data[0] == '{'` after trimming any leading whitespace.

**Verifies:** Basic structural sanity — the embedded content is a JSON object, not an array, string, or binary garbage.

### E3: Schema content is deterministic across builds

**Steps:**
1. Build the binary, call `ProjectSchema()`, store the SHA-256 hash.
2. Build again (same source), call `ProjectSchema()`, compute hash.

**Expected:** Hashes are identical.

**Verifies:** The embed process is deterministic. This matters because the merkle module will hash schema files — non-deterministic embedding would break snapshot reproducibility.

### E4: Concurrent access to schema loading

**Steps:**
1. Launch 10 goroutines, each calling `ProjectSchema()` and `ModuleSchema()`.
2. Collect all results.

**Expected:** All 20 calls succeed (nil error). All project results are byte-identical. All module results are byte-identical.

**Verifies:** `embed.FS.ReadFile` is safe for concurrent use (it is, per Go documentation, but this test confirms no wrapper state breaks that guarantee).

### E5: Attempting to load a non-existent schema name

The current API uses fixed function names (`ProjectSchema`, `ModuleSchema`) rather than a generic `LoadSchema(name)`. If the API were extended to accept a name parameter:

**Call:** `data, err := schemaFS.ReadFile("nonexistent.schema.json")`
**Expected:** `err` is non-nil (file not found in embed FS). `data` is nil or empty.

**Verifies:** The embed FS only contains the two expected schema files and does not silently serve other content.

### E6: Schema files reference correct $defs internally

**Steps:**
1. Load module schema, parse to `map[string]any`.
2. Walk `properties.components.items` and extract the `$ref` value.
3. Confirm it equals `"#/$defs/component"`.
4. Confirm `$defs.component` exists in the same document.

**Expected:** All `$ref` pointers resolve to definitions within the same schema file.

**Verifies:** The schemas are self-contained — no external `$ref` URIs that would fail when loaded from an embed FS (which cannot resolve external references).
