# Mapping Store Tests

## Setup

- Create a temporary directory with a valid spec structure (project.json + one module)
- Leave `.bead-map.json` absent, or write the bead-map document `{"next_id": 1, "records": []}`. A
  zero-byte file is neither, and the store refuses it before any operation runs
- Construct a MappingStore instance pointing at the temp directory

## Scenarios

### Create mapping record

- **Input**: spec_node_id="a1b2c3d4e5f6" (the identity hash of schema/component/ProjectSchema), bead_id="abc-123", bead_type="feature", module="schema", component="ProjectSchema", content_file="spec/schema/arch_project_schema.md", spec_hash="e3b0c44..."
- **Expected**: Record is written to `.bead-map.json` with a sequential integer record `id` (the internal auto-increment, used for the `spex:<id>` bead label). The record's `spec_node_id` round-trips as the supplied identity hash. File is valid JSON. Record contains all supplied fields including `bead_type`.

### Read mapping record by ID

- **Input**: Create a record, then read it back by its integer record `id`
- **Expected**: Returned record matches all fields that were written, including the identity-hash `spec_node_id`

### Update mapping record

- **Input**: Create a record, then update its spec_hash field
- **Expected**: Record's spec_hash is updated. Other fields are unchanged. The record ID is unchanged.

### Delete mapping record

- **Input**: Create a record, then delete it by ID
- **Expected**: Record is removed from `.bead-map.json`. File is valid JSON. Reading by the deleted ID returns not-found.

### List all mapping records

- **Input**: Create three records for different spec nodes
- **Expected**: List returns all three records. Order is deterministic (sorted by ID).

### Lookup by bead ID

- **Input**: Create a record with bead_id="abc-123", then look up by bead_id
- **Expected**: Returns the record matching that bead ID

### Lookup by spec node ID

- **Input**: Create a record with spec_node_id="impact/component/3", then look up by spec_node_id
- **Expected**: Returns the record matching that spec node ID

### Concurrent-safe file access

- **Input**: Two goroutines simultaneously call Create
- **Expected**: Both records are written correctly. No data corruption. File is valid JSON after both operations complete.

## Edge Cases

### Empty mapping file

- Load from a file containing `{"next_id": 1, "records": []}`. A bare `[]` will not do: the store
  schema-validates the file first and refuses it with `map: schema validation …` and exit 1
- All operations work on the empty list

### Missing mapping file

- MappingStore is constructed with a path where `.bead-map.json` does not exist
- First write creates the file
- Read before any write returns an empty list

### Duplicate bead ID

- Attempt to create two records with the same bead_id
- Expected: error — each bead maps to exactly one spec node

### Multiple beads per spec node

- Attempt to create two records with the same spec_node_id but different bead_ids
- Expected: error — with the obsolete+create model, the mapping file is current state only (one record per active spec node). The old record is updated with the new bead_id, not duplicated.
- Exception: a create whose `node_type` is `proposal` skips this check — a second proposal record with the same spec_node_id and a different bead_id is created rather than refused.

### bead_type field is preserved

- Create a record with bead_type="feature", read it back
- Expected: bead_type field is present and matches "feature"

### Update bead_id and spec_hash on existing record

- Create a record, then update its bead_id and spec_hash (used when obsolete+create replaces a bead)
- Expected: Record's bead_id and spec_hash are updated. Other fields (spec_node_id, module, component, content_file, bead_type) are unchanged.
