# Mapping Store Tests

## Setup

- Create a temporary directory with a valid spec structure (project.json + one module)
- Initialize an empty `.bead-map.json` file
- Construct a MappingStore instance pointing at the temp directory

## Scenarios

### Create mapping record

- **Input**: spec_node_id="schema/component/1", bead_id="abc-123", module="schema", component="ProjectSchema", content_file="spec/schema/arch_project_schema.md", spec_hash="e3b0c44..."
- **Expected**: Record is written to `.bead-map.json` with a sequential integer ID. File is valid JSON. Record contains all supplied fields.

### Read mapping record by ID

- **Input**: Create a record, then read it back by its integer ID
- **Expected**: Returned record matches all fields that were written

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

- Load from a file containing `[]`
- All operations work on the empty list

### Missing mapping file

- MappingStore is constructed with a path where `.bead-map.json` does not exist
- First write creates the file
- Read before any write returns an empty list

### Duplicate bead ID

- Attempt to create two records with the same bead_id
- Expected: error — each bead maps to exactly one spec node

### Duplicate spec node ID

- Attempt to create two records with the same spec_node_id
- Expected: error — each spec node maps to at most one bead
