# Schema Definitions

## Implementation Approach

Schemas are authored as standard JSON Schema (draft 2020-12) files stored in `schema/project.schema.json` and `schema/module.schema.json`.

## Key Decisions

### Shared requirement definition

The requirement `$def` is duplicated between project and module schemas rather than extracted to a shared schema file. This keeps each schema self-contained and avoids `$ref` resolution across files. The duplication is small (one definition) and documented with a `$comment` noting the need to keep both in sync.

The two definitions differ in two ways:
- **Module requirements** have `preq_id` in their `required` array — every module requirement must trace to a project requirement.
- **Project requirements** have an optional `priority` field (integer 0-4) — the validator enforces its presence, but the schema allows omission for migration flexibility.

### No `additionalProperties` in nested objects

Both schemas use `additionalProperties: false` at all levels. This prevents silent acceptance of misspelled or unknown fields. Any new field must be added to the schema before use.

### Array vs map for collections

Requirements, components, impl_sections, and data_flows are arrays (not maps) in JSON. This preserves ordering, which matters for rendering and consistent output. The `id` field within each item provides lookup by identifier.

### Numeric IDs

IDs are integers starting from 1 (`minimum: 1`). The schema enforces the minimum but not uniqueness within an array — uniqueness is a structural constraint enforced by the validator module.

### Sections array design

The `sections` array in project.schema.json uses a different `additionalProperties` strategy than the rest of the schema. Each section item requires envelope fields (`id`, `name`, `type`) but sets `additionalProperties: true` (the default) to allow freeform content. This is deliberate:

- The envelope validates structurally (spex core knows about id/name/type)
- The content validates semantically (the coupled module's `section.schema.json` knows about the domain-specific fields)

The section `$def` defines only the envelope:
```json
"section": {
  "type": "object",
  "required": ["id", "name", "type"],
  "properties": {
    "id": { "type": "integer", "minimum": 1 },
    "name": { "type": "string", "minLength": 1 },
    "type": { "type": "string", "minLength": 1 }
  }
}
```

### section.schema.json convention

The convention that coupled modules provide `section.schema.json` is a file-system contract, not a JSON Schema constraint. The project schema cannot express "if type == coupled, then a file must exist at a certain path." This cross-cutting validation is handled by the validator module's CoupledSectionChecker, not by JSON Schema itself.
