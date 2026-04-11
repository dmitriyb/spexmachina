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

### Identity hash IDs

IDs are 12-character lowercase hex strings, validated by the JSON Schema pattern `^[a-f0-9]{12}$`. The schema enforces the format but not uniqueness within an array — uniqueness is a structural constraint enforced by the validator module. IDs are not assigned manually; they are computed deterministically by `schema.IdentityHash` from the node's identity string (module + type + name/title). The same pattern applies to every cross-reference field (`depends_on`, `requires_module`, `implements`, `uses`, `describes`, `groups`, `preq_id`, `modules`).

The reason IDs are hashes rather than integers: integers are order-dependent, so two branches that independently add nodes assign the same next integer to different things and collide on merge. Identity hashes are derived from each node's name and position, so different nodes always produce different hashes and merging is collision-free without coordination. See `impl_identity_hash.md` for the algorithm and identity-string table.

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
    "id": { "type": "string", "pattern": "^[a-f0-9]{12}$" },
    "name": { "type": "string", "minLength": 1 },
    "type": { "type": "string", "minLength": 1 }
  }
}
```

### section.schema.json convention

The convention that coupled modules provide `section.schema.json` is a file-system contract, not a JSON Schema constraint. The project schema cannot express "if type == coupled, then a file must exist at a certain path." This cross-cutting validation is handled by the validator module's CoupledSectionChecker, not by JSON Schema itself.
