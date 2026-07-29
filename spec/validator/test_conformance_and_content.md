# Conformance and Content Tests

Integration and acceptance test scenarios for SchemaChecker (component 1) and ContentResolver (component 2).

## Setup

Each scenario reads one checked-in fixture directory under `validator/testdata/`. A fixture is a complete spec — `project.json`, its module directories, its content files — already carrying the mutation or mutations the scenarios that read it name. Fixtures are shared where scenarios agree (`content_valid` serves S7, S14 and the test_section variant).

### Fixture Structure

```
tmp/spec/
  project.json                  # valid project referencing module "alpha"
  alpha/
    module.json                 # valid module with 1 requirement, 1 component, 1 data_flow, 1 test_section
    arch_widget.md              # component content
    flow_widget_data.md         # data_flow content
    test_widget_behavior.md     # test_section content
```

### Shared Assertions

- Every scenario asserts the `check` field is `"schema"` or `"content"` as appropriate.
- Scenarios whose fixture has a closed error set assert the exact count; the rest assert that a matching error is present among those returned.
- `path` on a content error is the declaring node — `<module>/module.json:/<kind>s/<node name>/content` — and on a schema error is `<file>` or `<file>:<JSON pointer>`. Scenarios that assert it name the expected value.

---

## Scenarios

### S1: Valid project and module pass schema check

**Given** the baseline fixture with a conformant `project.json` and `module.json`.
**When** `CheckSchema(specDir)` is called.
**Then** it returns an empty error slice.
**Acceptance** Exit code 0 when this is the only checker.

### S2: project.json missing required `name` field

**Given** `project.json` with the `name` field removed.
**When** `CheckSchema(specDir)` is called.
**Then** it returns one error with:
- `check`: `"schema"`
- `path`: `"project.json"`
- `message` containing `"name"` and `"required"`

### S3: module.json component missing required `id` field

**Given** `alpha/module.json` where `components[0]` has `id` removed.
**When** `CheckSchema(specDir)` is called.
**Then** it returns one error referencing `alpha/module.json` and `components[0].id`.

### S4: Multiple violations reported, not just the first

**Given** `project.json` missing `name` AND `alpha/module.json` missing `requirements[0].title`.
**When** `CheckSchema(specDir)` is called.
**Then** it returns at least two errors, one for each file.
**Rationale** Validates the "report all violations" requirement (requirement 1).

### S5: module.json with invalid field type (string where number expected)

**Given** `alpha/module.json` where `requirements[0].id` is the string `"one"` instead of a number.
**When** `CheckSchema(specDir)` is called.
**Then** it returns one error whose `message` references a type mismatch.

### S6: project.json with unknown extra field (strict mode)

**Given** `project.json` with an additional property `"flavor": "vanilla"` not defined in the schema.
**When** `CheckSchema(specDir)` is called.
**Then** behavior depends on schema's `additionalProperties` setting. If the schema forbids additional properties, one error is returned. If it allows them, zero errors. This scenario documents the expected policy.

### S7: All content paths resolve successfully

**Given** the baseline fixture where every `content` field in `module.json` points to an existing `.md` file.
**When** `CheckContentPaths(specDir)` is called.
**Then** it returns an empty error slice.

### S8: Missing component content file

**Given** `alpha/module.json` references `arch_widget.md` but the file is deleted from disk.
**When** `CheckContentPaths(specDir)` is called.
**Then** it returns one error with:
- `check`: `"content"`
- `path`: `"alpha/module.json:/components/<component name>/content"` — the declaring node, not the missing file
- `message`: `"content file not found: arch_widget.md"`

### S9: Missing data_flow content file

**Given** `flow_widget_data.md` is deleted.
**When** `CheckContentPaths(specDir)` is called.
**Then** one error referencing the data_flow's content path.

### S10: Missing test_section content file (test_*.md)

**Given** `alpha/module.json` has a `test_sections` entry with `"content": "test_widget_behavior.md"` and that file is deleted.
**When** `CheckContentPaths(specDir)` is called.
**Then** one error referencing the test_section content path `"alpha/test_widget_behavior.md"`.
**Rationale** Validates that ContentResolver was updated to walk `test_sections` (requirement 10).

### S11: Content path with path traversal (`..`)

**Given** `alpha/module.json` has a component with `"content": "../escape.md"`.
**When** `CheckContentPaths(specDir)` is called.
**Then** one error flagging the path traversal as invalid, regardless of whether `../escape.md` exists.

### S12: Content path with absolute path

**Given** `alpha/module.json` has a component with `"content": "/etc/passwd"`.
**When** `CheckContentPaths(specDir)` is called.
**Then** one error flagging the absolute path as invalid.

### S13: Multiple missing content paths across sections

**Given** `alpha/module.json` references three content files: one in `components`, one in `data_flows`, and one in `test_sections`. All three files are missing.
**When** `CheckContentPaths(specDir)` is called.
**Then** exactly three errors, one per missing file, each with the correct `path` identifying which section referenced it.

### S14: Empty content field is not an error

**Given** `alpha/module.json` has a component with `"content": ""`.
**When** `CheckContentPaths(specDir)` is called.
**Then** zero errors for that component (empty content is optional per schema).

### S15: Schema and content checks run on multiple modules

**Given** a project with two modules `alpha` and `beta`. `alpha` is fully valid. `beta/module.json` has a schema violation and a missing content file.
**When** `CheckSchema` and `CheckContentPaths` are both called.
**Then** `CheckSchema` returns one error referencing `beta/module.json`. `CheckContentPaths` returns one error referencing the missing file in `beta/`. No errors reference `alpha`.

---

## Edge Cases

### E1: Spec directory does not exist

**Given** `specDir` points to a non-existent directory.
**When** `CheckSchema(specDir)` is called.
**Then** it returns one error indicating `project.json` could not be read, not a panic.

### E2: project.json is not valid JSON (parse failure)

**Given** `project.json` contains `{invalid json`.
**When** `CheckSchema(specDir)` is called.
**Then** it returns one error with a parse failure message. Subsequent checkers that depend on parsed JSON should handle the absence gracefully.

### E3: module.json missing entirely

**Given** `project.json` references module `alpha` at path `alpha/`, but `alpha/module.json` does not exist.
**When** `CheckSchema(specDir)` is called.
**Then** one error indicating the module.json file is missing.

### E4: Content file exists but is empty

**Given** `arch_widget.md` exists but has zero bytes.
**When** `CheckContentPaths(specDir)` is called.
**Then** zero content-resolution errors. ContentResolver only checks existence, not content.

### E5: data_flow content path also checked

**Given** `alpha/module.json` has a `data_flows` entry with `"content": "flow_missing.md"` and that file does not exist.
**When** `CheckContentPaths(specDir)` is called.
**Then** one error referencing `"alpha/flow_missing.md"`. ContentResolver must walk `data_flows` content paths in addition to components and test_sections.

### E6: Unicode in content file names

**Given** `alpha/module.json` has a component with `"content": "arch_widget_\u00fc.md"` and the file exists on disk with that name.
**When** `CheckContentPaths(specDir)` is called.
**Then** zero errors. Path resolution handles UTF-8 file names correctly.
