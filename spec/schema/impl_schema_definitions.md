# Schema Definitions

## Implementation Approach

Schemas are authored as standard JSON Schema (draft 2020-12) files stored in `schema/project.schema.json` and `schema/module.schema.json`.

## Key Decisions

### Shared requirement definition

The requirement `$def` is duplicated between project and module schemas rather than extracted to a shared schema file. This keeps each schema self-contained and avoidable `$ref` resolution across files. The duplication is small (one definition) and documented with a `$comment` noting the need to keep both in sync.

### No `additionalProperties` in nested objects

Both schemas use `additionalProperties: false` at all levels. This prevents silent acceptance of misspelled or unknown fields. Any new field must be added to the schema before use.

### Array vs map for collections

Requirements, components, impl_sections, and data_flows are arrays (not maps) in JSON. This preserves ordering, which matters for rendering and consistent output. The `id` field within each item provides lookup by identifier.

### Numeric IDs

IDs are integers starting from 1 (`minimum: 1`). The schema enforces the minimum but not uniqueness within an array — uniqueness is a structural constraint enforced by the validator module.
