# BeadMapSchema

The `.bead-map.json` JSON Schema definition. Validates the mapping file structure that links spec nodes to beads.

## Scope

Defines the JSON Schema for `.bead-map.json`, covering:

- **Envelope**: `next_id` (integer >= 1), `records` (array)
- **Record fields**: `id`, `spec_node_id`, `bead_id`, `module`, `component`, `content_file`, `spec_hash` (all required), `bead_status` (optional)
- **Format constraints**: `spec_node_id` validated by pattern `^[a-z_]+/(component|impl_section|data_flow|test_section)/[0-9]+$`

## Design Notes

### No uniqueness in schema

The schema does not enforce uniqueness of `bead_id` or `spec_node_id` across records. JSON Schema cannot express cross-record uniqueness constraints. These are enforced programmatically by MappingStore (`bead_id` must be unique; `spec_node_id` may repeat since one spec node can have many beads).

### bead_status is optional

The `bead_status` field is populated at runtime by preflight/apply operations. It is not required on disk — records written by the migration or by `spex apply` may omit it.
