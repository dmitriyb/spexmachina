# Mapping File Format

## File Structure

The mapping file at `spec/.bead-map.json` is a JSON object with a records array:

```json
{
  "next_id": 4,
  "records": [
    {
      "id": 1,
      "spec_node_id": "schema/component/1",
      "bead_id": "abc-001",
      "module": "schema",
      "component": "ProjectSchema",
      "content_file": "spec/schema/arch_project_schema.md",
      "spec_hash": "a1b2c3..."
    },
    {
      "id": 2,
      "spec_node_id": "schema/component/2",
      "bead_id": "abc-002",
      "module": "schema",
      "component": "ModuleSchema",
      "content_file": "spec/schema/arch_module_schema.md",
      "spec_hash": "d4e5f6..."
    },
    {
      "id": 3,
      "spec_node_id": "validator/component/1",
      "bead_id": "abc-003",
      "module": "validator",
      "component": "SchemaChecker",
      "content_file": "spec/validator/arch_schema_checker.md",
      "spec_hash": "g7h8i9..."
    }
  ]
}
```

## Fields

### Envelope

| Field | Type | Description |
|-------|------|-------------|
| `next_id` | int | Next auto-increment ID. Monotonically increasing, never reused. |
| `records` | array | All mapping records, sorted by ID. |

### Record

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Unique record ID, assigned by MappingStore |
| `spec_node_id` | string | `<module>/<node_type>/<node_id>` — e.g., `"impact/component/3"` |
| `bead_id` | string | Bead ID from `br` or `bd` |
| `module` | string | Module name (matches `module.json` name) |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Relative path to the spec content file |
| `spec_hash` | string | Merkle hash of the spec node when the record was created or last updated |

### spec_node_id format

The composite key `<module>/<node_type>/<node_id>` uniquely identifies a spec node:
- `node_type` is one of: `component`, `impl_section`, `data_flow`, `test_section`
- `node_id` is the integer ID from module.json

Examples: `"merkle/component/2"`, `"impact/impl_section/1"`, `"apply/data_flow/1"`

## Design Decisions

### next_id in envelope

Storing the next ID in the envelope (rather than computing max(ids)+1) ensures IDs are never reused even after deletions. If record 5 is deleted and next_id is 6, the next record gets ID 6, not 5.

### Sorted records

Records are always written sorted by ID. This makes the file diff-friendly in git — additions append to the array, modifications change a single entry in place.

### No nested structure

Records are stored in a flat array, not nested by module. This keeps the format simple and makes lookups by any field equally efficient with a linear scan. The file is small enough (one record per bead) that indexing is unnecessary.
