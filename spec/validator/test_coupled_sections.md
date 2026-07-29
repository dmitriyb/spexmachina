# Coupled Section Tests

Integration and acceptance tests for CoupledSectionChecker (component 10). These tests verify that the validator correctly handles the `sections` array in project.json: envelope validation, coupled module enforcement, and schema delegation.

## Setup

All scenarios use a temporary spec directory with a `project.json` and module directories. The `section.schema.json` files are created in the coupled module directories as needed.

**Fixture: valid coupled section setup**

```
spec/
  project.json    # has sections: [{ id: "000000000001", name: "delivery", type: "coupled", ... }]
  delivery/
    module.json   # name: "delivery"
    section.schema.json  # validates the delivery section content
```

The `section.schema.json` for the fixture accepts a simple structure:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["versioning"],
  "properties": {
    "versioning": {
      "type": "object",
      "required": ["scheme"],
      "properties": {
        "scheme": { "type": "string" }
      }
    }
  }
}
```

### Preconditions

- CoupledSectionChecker receives the spec directory path only (`CheckCoupledSections(specDir string)`) and parses `project.json` itself through the shared `loadSpec`; a project.json that will not parse ends the check with one load error.
- The project schema already allows the `sections` array with `additionalProperties` on each section entry (envelope fields validated by schema, content fields are freeform).

## Scenarios

### S1: Valid coupled section passes all checks

**Given** a project with one section (`id: "000000000001", name: "delivery", type: "coupled"`) and a `delivery` module exists with a valid `section.schema.json` that accepts the section content.

**When** CoupledSectionChecker runs.

**Then:** Zero errors. The section envelope is valid, the coupled module exists, and the section content passes the module's schema validation.

### S2: Section with non-coupled type is skipped

**Given** a project with one section (`id: "000000000001", name: "notes", type: "informational"`). No module named "notes" exists.

**When** CoupledSectionChecker runs.

**Then:** Zero errors. Non-coupled sections do not require a matching module or `section.schema.json`. The checker only validates the envelope (id, name, type present).

### S3: Coupled section with no matching module fails

**Given** a project with section (`id: "000000000001", name: "delivery", type: "coupled"`) but no module named "delivery" in the modules array.

**When** CoupledSectionChecker runs.

**Then:** Error reported: coupled section "delivery" has no matching module. Error includes the section name and suggests adding a module with `name: "delivery"`.

### S4: Coupled section with missing section.schema.json fails

**Given** a project with a coupled "delivery" section, a module named "delivery" exists at path `delivery/`, but `spec/delivery/section.schema.json` does not exist on disk.

**When** CoupledSectionChecker runs.

**Then:** Error reported: `coupled module "delivery" is missing section.schema.json at delivery/section.schema.json`. The path is relative to the spec directory; the spec directory itself is not part of the message.

### S5: Section content fails module schema validation

**Given** a project with a coupled "delivery" section whose content includes `versioning: { scheme: 123 }` (number instead of string). The `section.schema.json` requires `scheme` to be a string.

**When** CoupledSectionChecker runs.

**Then:** Error reported: section "delivery" content failed validation against `section.schema.json`. Error includes the JSON Schema validation details (path and constraint violated).

### S6: Duplicate section IDs fail

**Given** a project with two sections both having `id: "000000000001"`.

**When** CoupledSectionChecker runs.

**Then:** Error reported: `duplicate section id 000000000001 (also at sections/0)`. The message carries the id and the index of the first section that claimed it; section names are not in it.

### S7: Section missing required envelope fields fails

**Given** a project with a section entry that has `id: "000000000001"` and `type: "coupled"` but is missing the `name` field.

**When** CoupledSectionChecker runs.

**Then:** Error reported: section at index 0 is missing required field `name`. (Note: this may be caught by JSON Schema validation of the envelope before CoupledSectionChecker runs, depending on implementation order. If so, the schema check catches it first.)

### S8: Multiple coupled sections validated independently

**Given** a project with two coupled sections: `delivery` (valid) and `performance` (missing module).

**When** CoupledSectionChecker runs.

**Then:** One error for `performance` (no matching module). No errors for `delivery`. Both sections are checked — the checker does not short-circuit on the first error.

### S9: Empty sections array passes

**Given** a project with `sections: []`.

**When** CoupledSectionChecker runs.

**Then:** Zero errors. An empty sections array is valid.

### S10: Project with no sections array passes

**Given** a project with no `sections` field at all.

**When** CoupledSectionChecker runs.

**Then:** Zero errors. The sections array is optional.

## Edge Cases

### E1: Section name matches module name case-sensitively

**Given** a project with section `name: "Delivery"` (capital D) and a module named `"delivery"` (lowercase).

**When** CoupledSectionChecker runs.

**Then:** Error reported: no matching module for coupled section "Delivery". The match is case-sensitive — section name must exactly match the module name. Error suggests the near-match "delivery".

### E2: section.schema.json is invalid JSON Schema

**Given** a project with a coupled "delivery" section, a module named "delivery" exists, and `section.schema.json` exists but contains `{ "type": "invalid_type" }` (not a valid JSON Schema type).

**When** CoupledSectionChecker runs.

**Then:** Error reported: failed to load or compile `section.schema.json` for module "delivery". The error includes the schema compilation failure details.

### E3: section.schema.json is not valid JSON

**Given** `section.schema.json` contains `{broken json`.

**When** CoupledSectionChecker runs.

**Then:** Error reported: failed to parse `section.schema.json` as JSON. Includes file path and parse error.

### E4: Coupled section with empty content (only envelope fields)

**Given** a section with `id: "000000000001", name: "delivery", type: "coupled"` and no additional fields beyond the envelope. The `section.schema.json` requires a `versioning` object.

**When** CoupledSectionChecker runs.

**Then:** Error reported: section content fails validation — missing required `versioning` field. The envelope fields (id, name, type) are stripped before validating content against the module schema.

### E5: Multiple sections referencing the same module

**Given** two sections with different IDs but both `name: "delivery"` and `type: "coupled"`.

**When** CoupledSectionChecker runs.

**Then:** Error reported: duplicate section name "delivery". Each coupled module can have at most one section.
