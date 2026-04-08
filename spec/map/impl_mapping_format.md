# Mapping File Format

## File Structure

The mapping file at `spec/.bead-map.json` is a JSON object with a records array:

```json
{
  "next_id": 4,
  "records": [
    {
      "id": 1,
      "spec_node_id": "a1b2c3d4e5f6",
      "bead_id": "abc-001",
      "bead_type": "feature",
      "module": "schema",
      "component": "ProjectSchema",
      "content_file": "spec/schema/arch_project_schema.md",
      "spec_hash": "a1b2c3..."
    },
    {
      "id": 2,
      "spec_node_id": "0f1e2d3c4b5a",
      "bead_id": "abc-002",
      "bead_type": "feature",
      "module": "schema",
      "component": "ModuleSchema",
      "content_file": "spec/schema/arch_module_schema.md",
      "spec_hash": "d4e5f6..."
    },
    {
      "id": 3,
      "spec_node_id": "9988776655ee",
      "bead_id": "abc-003",
      "bead_type": "feature",
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
| `next_id` | int | Next auto-increment record ID. Monotonically increasing, never reused. Internal to the bead-map only — not part of the spec graph. |
| `records` | array | All mapping records, sorted by ID. |

### Record

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Unique record ID, assigned by MappingStore. Used as the bead label `spex:<id>`. Stays integer because it is internal to the bead-map. |
| `spec_node_id` | string | The spec node's identity hash (12-char lowercase hex, pattern `^[a-f0-9]{12}$`). Identical to the merkle tree key for the same node. |
| `bead_id` | string | Bead ID from `br` or `bd` |
| `bead_type` | string | Bead issue type (`epic`, `feature`, or `task`) — carried as a separate field because identity hashes do not embed type information |
| `module` | string | Module name (matches `module.json` name) — human-readable, for context resolution and debug output |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Relative path to the spec content file |
| `spec_hash` | string | Merkle content hash of the spec node when the record was created or last updated |

### spec_node_id format

The `spec_node_id` is the spec node's identity hash, computed once at spec-author time by `schema.IdentityHash(module, type, name)`. It is a 12-character lowercase hex string and matches the pattern `^[a-f0-9]{12}$`.

The merkle tree uses this same hash as its leaf key for the corresponding node, so the impact command can look up changed merkle nodes in the bead-map by the change key directly — no parsing, no rekeying. The `module`, `component`, and `content_file` fields exist alongside `spec_node_id` purely as denormalized human-readable context for tooling output; they are not used as join keys.

## Design Decisions

### next_id in envelope

Storing the next ID in the envelope (rather than computing max(ids)+1) ensures IDs are never reused even after deletions. If record 5 is deleted and next_id is 6, the next record gets ID 6, not 5.

### Sorted records

Records are always written sorted by ID. This makes the file diff-friendly in git — additions append to the array, modifications change a single entry in place.

### No nested structure

Records are stored in a flat array, not nested by module. This keeps the format simple and makes lookups by any field equally efficient with a linear scan. The file is small enough (one record per bead) that indexing is unnecessary.
