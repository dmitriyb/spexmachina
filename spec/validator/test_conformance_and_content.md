# Conformance and Content Tests

Integration and acceptance test scenarios for SchemaChecker (component 1) and ContentResolver (component 2).

## Setup

Each scenario reads one checked-in fixture directory under `validator/testdata/`. A fixture is a complete spec — `project.json`, its module directories, its content files — already carrying the mutation or mutations the scenarios that read it name.

### Shared Assertions


- Every scenario asserts the `check` field is `"schema"` or `"content"` as appropriate.
- Scenarios whose fixture has a closed error set assert the exact count; the rest assert that a matching error is present among those returned.
- `path` on a content error is the declaring node — `<module>/module.json:/<kind>s/<node name>/content` — and on a schema error is `<file>` or `<file>:<JSON pointer>`. Scenarios that assert it name the expected value.

---

## Scenarios

### S15: Schema and content checks run on multiple modules

**Given** a project with two modules `alpha` and `beta`. `alpha` is fully valid. `beta/module.json` has a schema violation and a missing content file.
**When** `CheckSchema` and `CheckContentPaths` are both called.
**Then** `CheckSchema` returns one error referencing `beta/module.json`. `CheckContentPaths` returns one error referencing the missing file in `beta/`. No errors reference `alpha`.

### S16: Conformance runs against the profile-composed schemas

**Given** the baseline fixture plus a `spec/profile.json` declaring an additional module-scoped `endpoint` type, and `alpha/module.json` carrying an `endpoints` array conforming to the envelope.
**When** `CheckSchema(specDir)` is called.
**Then** zero errors — the schema validated against is composed from the resolved profile, so the declared array is accepted; removing the profile file and re-running yields one `additionalProperties` error at `alpha/module.json`, because the default profile does not declare the type. `schema_path` values in the error point into the generated document, so interpreting them requires the profile the run resolved.

### S17: Malformed profile fails before any conformance check

**Given** the baseline fixture plus a `spec/profile.json` that is not valid JSON.
**When** the validation pipeline runs.
**Then** one error naming `spec/profile.json` and the parse defect, and zero schema-conformance errors — composition happens once per run before any check, so a broken profile never surfaces as a cascade of conformance failures against a half-composed schema.

### S18: Content resolution walks the declared content-bearing types

**Given** a `spec/profile.json` declaring an `endpoint` type that requires a content leaf, and `alpha/module.json` with an endpoint whose `content` names a missing file.
**When** `CheckContentPaths(specDir)` is called.
**Then** one `content` error for the endpoint's declared path — ContentResolver walks whichever types the resolved profile marks content-bearing, not a fixed three; under the default profile the walked set is exactly components, data_flows and test_sections.

---


## Edge Cases

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
