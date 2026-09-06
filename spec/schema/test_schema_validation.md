# Schema Validation Tests

Integration and acceptance tests for ProjectSchema (component 1) and ModuleSchema (component 2). These tests verify that the JSON Schema definitions correctly accept valid specs, reject invalid specs, and enforce all constraints. Both documents are composed from the resolved profile at load time; every scenario below runs against the schemas composed from the default profile, which are golden-tested elsewhere to equal the shipped static documents, so the assertions here hold identically to the pre-profile behaviour — up to the requirement type's title-to-name rename, which every requirement fixture below already speaks.

All scenarios below assume a JSON Schema validator is available (e.g., `santhosh-tekuri/jsonschema` or equivalent). The validator is loaded with the embedded schema and then asked to validate JSON documents. "Passes validation" means zero errors; "fails validation" means one or more structured errors with paths.

## Setup

### Preconditions


- The JSON Schema validator is initialized with the schema loaded from `ProjectSchema()` or `ModuleSchema()` respectively.
- Validation is performed against the 2020-12 draft vocabulary (matching the `$schema` declaration in both schema files).

## Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.

## Edge Cases

### S27: Profile-declared arrays and fields accepted, undeclared ones still rejected

**Given** a profile declaring an additional module-scoped `endpoint` type with plural key `endpoints` and a required content leaf, and the module schema composed from it.

**Steps:** Compose the module schema from a profile that declares an additional module-scoped `endpoint` type with plural key `endpoints` and a required content leaf. Validate documents against the composed schema:

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

**Input (declared reference field):** compose instead from the same profile extended with a `serves` field on `endpoint` — kind reference, target `component`, cardinality many; the same document with `"serves": ["000000000002"]` added to the endpoint entry.
**Expected:** Validation passes — a reference-kind field composes as an array-of-identity-hash property on its declaring type's entry definition, so a node of the type can carry the field the profile says it may. Declared with cardinality one instead, the property composes as a single identity-hash scalar — the shape `preq_id` has always had — and the array form then fails on type.

**Input (declared text field with an enumeration):** the profile extended with a `protocol` text field on `endpoint`, enum `["http", "grpc"]`; the endpoint entry carrying `"protocol": "http"` and, in a second document, `"protocol": "ftp"`.
**Expected:** The first passes; the second fails at `endpoints/0/protocol` — the value is not in the declared enumeration.

**Input (declared integer field with bounds):** the profile extended with a `port` integer field on `endpoint`, bounds 1–65535; the endpoint entry carrying `"port": 8080` and, in a second document, `"port": 0`.
**Expected:** The first passes; the second fails at `endpoints/0/port` on the declared minimum.

**Input (declared required field absent):** the profile's `protocol` field marked required; the endpoint entry without it.
**Expected:** Validation fails at `endpoints/0`, missing required `protocol` — and a required text field is composed non-empty, so `"protocol": ""` fails too.

**Input (undeclared field):** the `"serves"`-carrying document validated against the schema composed from the field-less profile of the first step.
**Expected:** Validation fails at the endpoint entry — the entry-level `additionalProperties: false` rejects a field that is neither envelope nor declared.

**Input (undeclared array):** the first document with `"widgets": []` added.
**Expected:** Validation fails at the root — `additionalProperties: false` is part of the fixed frame, so an array no profile declares is rejected exactly as it always was.

**Verifies:** The frame-plus-vocabulary split — the profile supplies array properties and each declared type's fields, the frame supplies everything else; declaring a type is the only way to open an array, and declaring a field is the only way to open a property on a declared type. The built-in types compose through this same path from the default profile's field declarations, holding exactly as they did before the composition — the golden comparison itself is by JSON value, never by bytes.

