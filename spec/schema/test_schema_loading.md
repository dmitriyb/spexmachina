# Schema Loading Tests

Integration and acceptance tests for SchemaLoader and ProfileLoader. The SchemaLoader is the Go package (`schema/schema.go`) that embeds the schema frame, the default profile document `defaultProfile.json`, the journal-line schema `journal-line.schema.json` and the task-state schema `task-state.schema.json` via `go:embed`, composes the effective project and module schemas, and exposes them through `ProjectSchema()`, `ModuleSchema()`, `JournalLineSchema()` and `TaskStateSchema()` functions — the two composed reads take no arguments and always compose from the built-in default profile, consulting no file. It also exposes the `IdentityHash(parts ...string) string` function that defines what spec node IDs look like. ProfileLoader owns the file-backed resolution: it reads `spec/profile.json` when present, falls back to the built-in default profile otherwise, validates the document, and exposes the resolved profile. A caller that needs a project's own profile reflected in the composed schemas takes the resolve-then-compose path — ProfileLoader's resolution handed to the composition entry points directly — which the P-scenarios below exercise; the zero-argument reads are the default-profile convenience over the same composition.

These tests verify that the embedding works correctly, that the composed schemas are structurally sound and reproduce the shipped static documents under the default profile, that profile resolution and fallback behave as declared, that the composed schemas can be used to validate known-good fixtures, and that the `IdentityHash` function is deterministic and matches the schema's hex pattern.

## Setup

### Build Preconditions

- The `schema` package compiles successfully (`go build ./schema/...`).
- The `go:embed` directive references `project.schema.json`, `module.schema.json`, `journal-line.schema.json` and `task-state.schema.json`, all of which exist in the `schema/` directory at build time.
- No external file system access is needed at runtime — schemas are baked into the binary.

### Test Fixtures

The following fixture files live in `schema/testdata/`:

- `valid_project.json` — a full project.json with the optional fields populated (`description`, `version`, requirements carrying `priority` and `depends_on`, modules with inter-module dependencies).
- `minimal_project.json` — the smallest valid project.json (`name` + one module).
- `valid_module.json` — a full module.json with the optional arrays populated (requirements with preq_id, components with implements/uses, data_flows, test_sections, apis).
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

**Verifies:** The composition path behind the zero-argument read works at runtime — the embedded frame and default profile are accessible and yield a well-formed document.

### S2: ModuleSchema() loads without error

**Call:** `data, err := schema.ModuleSchema()`
**Expected:**
- `err` is nil.
- `data` is non-empty (length > 0).
- `data` is valid JSON (unmarshals into `map[string]any` without error).

**Verifies:** The composition path behind the zero-argument read works at runtime — the embedded frame and default profile are accessible and yield a well-formed document.

### S3: ProjectSchema() returns valid JSON Schema document

**Call:** `data, _ := schema.ProjectSchema()` then unmarshal to `map[string]any`.
**Expected assertions on parsed content:**
- `$schema` field equals `"https://json-schema.org/draft/2020-12/schema"`.
- `$id` field equals `"https://spexmachina.dev/schema/project.json"`.
- `title` field equals `"Spex Machina Project"`.
- `type` field equals `"object"`.
- `required` array contains `"name"` and `"modules"`.
- `additionalProperties` is `false`.
- `properties` object contains keys: `name`, `description`, `version`, `spec_version`, `requirements`, `modules`, `sections`.
- `$defs` object contains keys: `identityHash`, `requirement`, `module`, `section` — and nothing else.

**Verifies:** The composed document is the actual project schema (not a stale frame or wrong composition) and carries the `sections` array. The `$defs` assertion is exhaustive on purpose: it is what catches a retired node type (`milestone`, `test_scenario`) coming back through a stale embed.

### S4: ModuleSchema() returns valid JSON Schema document

**Call:** `data, _ := schema.ModuleSchema()` then unmarshal to `map[string]any`.
**Expected assertions on parsed content:**
- `$schema` field equals `"https://json-schema.org/draft/2020-12/schema"`.
- `$id` field equals `"https://spexmachina.dev/schema/module.json"`.
- `title` field equals `"Spex Machina Module"`.
- `type` field equals `"object"`.
- `required` array contains `"name"`.
- `additionalProperties` is `false`.
- `properties` object contains keys: `name`, `description`, `requirements`, `components`, `data_flows`, `test_sections`, `apis`.
- `$defs` object contains keys: `requirement`, `component`, `data_flow`, `test_section`, `api`.

**Verifies:** The composed document is the actual module schema and includes the `test_sections`/`test_section` and `apis`/`api` additions.

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

**Verifies:** The two compositions yield two distinct documents. No cross-contamination between the project and module paths.

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

**Verifies:** The embed FS only contains the four expected schema files and does not silently serve other content.

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

**Verifies:** Part order and part content both reach the hash — reordering parts or changing one changes the result. No general injectivity is claimed: the algorithm's deliberate collision cases are stated in SchemaLoader's leaf and exercised by IH6.

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

**Expected:** Each call returns *some* 12-char hex string and none panic. No distinctness is asserted between the calls: under the `join(parts, "/")` algorithm declared in SchemaLoader's leaf, no parts and one empty part both produce the identity string `""` and therefore the same hash. That collision is accepted — every real part is a fixed type literal or comes from a `name` field the schemas require to be non-empty (`minLength: 1`), so no reachable node hashes a degenerate identity string. The values are not asserted against fixed strings — only that the function is total and produces schema-valid output for any input.

**Verifies:** The function does not crash on degenerate input.

## ProfileLoader Scenarios

These scenarios cover profile resolution and the composition acceptance criterion. The spec directory used is a fixture; "the default profile" means the built-in declaration of today's ontology.

### P1: Absent profile file resolves to the default profile

**Steps:** Resolve the profile over a spec directory containing no `spec/profile.json`.
**Expected:** Resolution succeeds; the resolved profile declares exactly the five built-in node types (requirement, component, data_flow, test_section, api) with today's per-type role flags — the completeness trigger on requirement, the name-declarable role on exactly component and api — each type's field declarations, reference kinds included, with the `cyclic` flag omitted on every one and the hash-participation flag reproducing the retired allowlists exactly, plus the three coverage links, the plan-relevant set, the per-type impact-level mapping, and refresh's absorbable directions. The resolved form is interned: each node type and field is resolved once, consumers compare resolved references rather than strings, and iteration order is declaration order.
**Verifies:** Absence of the file is the supported default, not an error — an existing project adopts the profile mechanism by doing nothing.

### P2: Composed schemas equal the shipped static documents (golden test)

**Steps:** Resolve the default profile, compose the project and module schemas from it, and compare each against the shipped static schema document as a golden record — equal as JSON values, independent of formatting and key order, since the shipped copies are hand-formatted and the composer emits compact key-sorted JSON.
**Expected:** Both composed documents reproduce the shipped documents' content; a composition that does not is a test failure, not a tolerated drift.
**Verifies:** The acceptance criterion that the default profile reproduces the shipped documents exactly. The one deliberate on-disk change for existing project.json and module.json files is the requirement type's title-to-name rename — spec format version 1's break, exercised by P8's sibling scenarios, not by this comparison.

### P3: Malformed profile is a distinct early failure

**Steps:** Place a `spec/profile.json` containing `{invalid json` (and, in a second case, a well-formed JSON document that violates the profile's own constraints, e.g. a node type with no plural array key). Resolve the profile.
**Expected:** Resolution fails with one error naming the profile file and the defect. No composed schema is produced and no downstream conformance check runs, so the failure is never reported as a cascade of confusing schema-conformance errors.

### P4: A declared custom type reaches the composed schema

**Steps:** Place a valid `spec/profile.json` declaring an `endpoint` type, module-scoped, plural key `endpoints`, content leaf required, with a text field carrying an enumeration and a reference field targeting `component`. Resolve and compose the module schema.
**Expected:** The composed module schema's `properties` contains an `endpoints` array validated with the same envelope constraints (identity-hash id, name, required content) the built-in types get, and its entry definition carries one property per declared field — enum-constrained for the text field, an array of identity hashes for the reference field; `additionalProperties: false` still rejects any array the profile does not declare and any field the type does not.

### P5: Resolution is deterministic

**Steps:** Resolve the same profile (default and file-backed) twice.
**Expected:** Byte-identical resolved profiles and byte-identical composed schemas across runs.

### P6: A new reference field on a built-in type composes like any other

**Steps:** Place a `spec/profile.json` declaring the default ontology plus an `audits` reference field — one the built-in shape never carried — on the built-in `component` type, targeting `component`. Resolve the profile and compose the module schema.
**Expected:** Resolution succeeds and the composed component definition carries an `audits` array-of-identity-hash property beside the built-in fields.
**Verifies:** The interim rule refusing a new edge kind sourced at a built-in type is retired: built-in types compose from field declarations through the same path declared types take, so there is no frame-fixed definition left for a new field to be unable to reach. The v1 scenario asserting that refusal is superseded by this one.

### P7: Profile validation names each defective field declaration

**Steps:** Resolve, one at a time, profiles carrying: a field with an unknown kind; a reference field naming an undeclared target type; an enumeration on a non-text field; bounds on a non-integer field; a duplicate field name within one type; a field name colliding with an envelope field (`id`).
**Expected:** Each resolution fails with one distinct early error naming the defective declaration. No composed schema is produced and no downstream check runs. The v1 rule stands unchanged: a profile attempting to declare a fixed point fails validation the same way.

### P8: profile_version outside the supported range fails early

**Steps:** Place a `spec/profile.json` identical to the default declaration but carrying `"profile_version": 99`. Resolve the profile. In a second case, omit `profile_version` entirely.
**Expected:** The first resolution fails with one message naming the file, the version it declares (99) and the supported range — the migrate-before-using-this-spex signal, produced before any conformance check. The second succeeds: an absent `profile_version` means version 1, which is the version this binary supports, so adoption is doing nothing. A pre-versioning document in the earlier `edges`/`hashed_fields` format draws no version message at all — it fails ordinary profile validation as a malformed document, the deliberate breaking change the format-version requirement records.

### P9: The embedded default profile is an ordinary profile document

**Steps:** Read the embedded `defaultProfile.json` bytes, parse them, and resolve them through the same validation the file-backed path uses. Separately, copy the document to `spec/profile.json` in a fixture directory and resolve over it.
**Expected:** Both resolutions succeed and yield identical resolved profiles — the embedded default is not a privileged code path but a document in the source tree, identical in format to what a project may commit, declaring `profile_version` 1. Composition from either yields the same composed schemas, so P2's golden comparison holds over both.

## JournalLineSchema Scenarios

### JL1: JournalLineSchema() loads without error

**Call:** `data, err := schema.JournalLineSchema()`

**Expected:** `err` is nil; `data` is non-empty; `data` is valid JSON.

### JL2: JournalLineSchema() accepts both identities a journal line carries

**Steps:**
1. Load the journal-line schema and compile a validator.
2. Validate a change event whose `node` is `"a1b2c3d4e5f6"` (identity hash) — passes.
3. Validate an epic receipt whose `proposal` is `"2026-04-12-data-flow-contract-layer"` — passes.
4. Validate a change event whose `node` is `""` — fails the pattern.

**Expected:** The first two pass; the empty string fails the identity-hash pattern.

**Verifies:** node keys and proposal slugs live in different fields with different constraints — the pattern lives where the hash lives, and slugs never share its field.

## TaskStateSchema Scenarios

### TS1: TaskStateSchema() loads without error

**Call:** `data, err := schema.TaskStateSchema()`

**Expected:** `err` is nil; `data` is non-empty; `data` is valid JSON and compiles as a JSON Schema document.

### TS2: The embedded task-state schema is the one plan validates against

**Steps:**
1. Load the task-state schema and compile a validator.
2. Validate `{"version": 1, "tasks": [{"task_id": "spexmachina-abc", "status": "open"}]}` — passes.
3. Validate `{"version": 1, "tasks": [{"task_id": "spexmachina-abc", "status": "closed"}]}` — fails the enum.

**Expected:** the first passes and the second fails naming the `status` constraint.

**Verifies:** the loader serves the same document `TaskReader` refuses a `closed` status against, so the two cannot drift: one embedded file, read by one function, compiled by its one consumer.
