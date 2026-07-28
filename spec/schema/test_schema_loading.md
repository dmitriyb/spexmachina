# Schema Loading Tests

Integration and acceptance tests for SchemaLoader (component 3). The SchemaLoader is the Go package (`schema/schema.go`) that embeds `project.schema.json`, `module.schema.json`, and `bead-map.schema.json` via `go:embed` and exposes them through `ProjectSchema()`, `ModuleSchema()`, and `BeadMapSchema()` functions. It also exposes the `IdentityHash(parts ...string) string` function that defines what spec node IDs look like.

These tests verify that the embedding works correctly, that the loaded schemas are structurally sound, that they can be used to validate known-good fixtures, and that the `IdentityHash` function is deterministic and matches the schema's hex pattern.

## Setup

### Build Preconditions

- The `schema` package compiles successfully (`go build ./schema/...`).
- The `go:embed` directive references `project.schema.json`, `module.schema.json` and `bead-map.schema.json`, all of which exist in the `schema/` directory at build time.
- No external file system access is needed at runtime — schemas are baked into the binary.

### Test Fixtures

The following fixture files live in `schema/testdata/`:

- `valid_project.json` — a full project.json with the optional fields populated (`description`, `version`, requirements carrying `priority` and `depends_on`, modules with inter-module dependencies).
- `minimal_project.json` — the smallest valid project.json (`name` + one module).
- `valid_module.json` — a full module.json with the optional arrays populated (requirements with preq_id, components with implements/uses, impl_sections, data_flows, test_sections, apis).
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
- `properties` object contains keys: `name`, `description`, `version`, `requirements`, `modules`, `sections`.
- `$defs` object contains keys: `identityHash`, `requirement`, `module`, `section` — and nothing else.

**Verifies:** The embedded file is the actual project schema (not a stale copy or wrong file) and carries the `sections` array. The `$defs` assertion is exhaustive on purpose: it is what catches a retired node type (`milestone`, `test_scenario`) coming back through a stale embed.

### S4: ModuleSchema() returns valid JSON Schema document

**Call:** `data, _ := schema.ModuleSchema()` then unmarshal to `map[string]any`.
**Expected assertions on parsed content:**
- `$schema` field equals `"https://json-schema.org/draft/2020-12/schema"`.
- `$id` field equals `"https://spexmachina.dev/schema/module.json"`.
- `title` field equals `"Spex Machina Module"`.
- `type` field equals `"object"`.
- `required` array contains `"name"`.
- `additionalProperties` is `false`.
- `properties` object contains keys: `name`, `description`, `requirements`, `components`, `impl_sections`, `data_flows`, `test_sections`, `apis`.
- `$defs` object contains keys: `requirement`, `component`, `impl_section`, `data_flow`, `test_section`, `api`.

**Verifies:** The embedded file is the actual module schema and includes the `test_sections`/`test_section` and `apis`/`api` additions.

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

**Verifies:** The embed FS only contains the three expected schema files and does not silently serve other content.

### E6: Schema files reference correct $defs internally

**Steps:**
1. Load module schema, parse to `map[string]any`.
2. Walk `properties.components.items` and extract the `$ref` value.
3. Confirm it equals `"#/$defs/component"`.
4. Confirm `$defs.component` exists in the same document.

**Expected:** All `$ref` pointers resolve to definitions within the same schema file.

**Verifies:** The schemas are self-contained — no external `$ref` URIs that would fail when loaded from an embed FS (which cannot resolve external references).

## IdentityHash Scenarios

These scenarios cover the `schema.IdentityHash(parts ...string) string` function. The function is small but load-bearing — every other module imports it to compute or compare spec node IDs.

### IH1: IdentityHash is deterministic

**Call:** `IdentityHash("impact", "component", "NodeMatcher")` ten times.

**Expected:** All ten calls return the same 12-character hex string.

**Verifies:** The function has no hidden state, no time dependence, no randomness.

### IH2: IdentityHash output matches the schema pattern

**Call:** Compute hashes for ~30 representative inputs (varying part counts and lengths).

**Expected:** Every output matches the regex `^[a-f0-9]{12}$` — exactly 12 characters, lowercase hex only.

**Verifies:** Every value the function produces is a legal `id` field value per `project.schema.json` and `module.schema.json`. If this test fails, every other module will start emitting schema-invalid records.

### IH3: Different parts produce different hashes

**Call:** Hash `("a", "b", "c")` and `("a", "b", "d")` and `("a", "c", "b")` and `("d", "b", "c")`.

**Expected:** All four results are distinct.

**Verifies:** The function is not collapsing parts via concatenation in a way that loses positional information.

### IH4: Same node identity across modules produces different hashes

**Call:** `IdentityHash("validator", "component", "Foo")` and `IdentityHash("merkle", "component", "Foo")`.

**Expected:** The two hashes differ.

**Verifies:** Two components with the same name in different modules are correctly disambiguated by the module part. This is the property that lets the validator skip cross-module collision checks.

### IH5: Joining is on `/` exactly

**Call:** `IdentityHash("a", "b")` versus a manual `sha256.Sum256([]byte("a/b"))[:6]` hex-encoded.

**Expected:** The two values are equal.

**Verifies:** The join separator is `/`, not `.`, `,`, or `_`. This is the contract every other module relies on when re-deriving an ID by hand.

### IH6: Empty parts and empty input

**Call:** `IdentityHash()`, `IdentityHash("")`, `IdentityHash("a", "", "b")`.

**Expected:** Each call returns *some* 12-char hex string, all three are distinct, and none panic. The values are not asserted against fixed strings — only that the function is total and produces schema-valid output for any input.

**Verifies:** The function does not crash on degenerate input. It also does not silently treat empty strings as identical to absent parts.

## BeadMapSchema Scenarios

### BM1: BeadMapSchema() loads without error

**Call:** `data, err := schema.BeadMapSchema()`

**Expected:** `err` is nil; `data` is non-empty; `data` is valid JSON.

### BM2: BeadMapSchema() enforces the identity hash spec_node_id pattern

**Steps:**
1. Load the bead-map schema and compile a validator.
2. Validate a record where `spec_node_id` is `"a1b2c3d4e5f6"` (legal identity hash).
3. Validate a record where `spec_node_id` is `"impact/component/3"` (legacy format).

**Expected:** The first passes; the second fails with a pattern violation referencing `^[a-f0-9]{12}$`.

**Verifies:** The schema enforces the new identity hash format. This is the regression guard that catches any code path which tries to write a legacy-format `spec_node_id` into the bead-map.
