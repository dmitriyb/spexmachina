# BeadMapSchema

The `.bead-map.json` JSON Schema definition. Validates the mapping file structure that links spec nodes to beads.

## Scope

Defines the JSON Schema for `.bead-map.json`, covering:

- **Envelope**: `next_id` (integer >= 1), `records` (array)
- **Record fields**: `id`, `spec_node_id`, `bead_id`, `bead_type`, `module`, `component`, `content_file`, `spec_hash` (all required), `node_type` and `bead_status` (optional)
- **Mappable node types**: `node_type` is a closed enum — `proposal`, `component`, `data_flow`, `test_section` — naming what the record points at
- **Format constraints**: `spec_node_id` validated by pattern `^[a-f0-9]{12}$` — the same identity hash format used by node IDs in `project.json` and `module.json`. The mapping store and the merkle tree use the same key format, so no rekeying is needed when impact analysis matches changed merkle nodes against existing mapping records.

## Design Notes

### Single key format across the pipeline

`spec_node_id` is the identity hash of the spec node the record points at. The merkle tree keys its leaves by the same identity hash. This means impact analysis can look up a changed merkle node directly in the mapping store with no key translation. Earlier versions of the schema used the format `<module>/<node_type>/<integer_id>` (e.g. `impact/component/2`) which required rekeying between merkle and mapping; that translation layer was deleted when identity hashes were introduced.

### `node_type` is the closed set of things a record may point at

The enum is the schema's own statement of what can be mapped: a record names a component, a data_flow, a test_section, or the epic standing in for a proposal, and nothing else. A kind of spec content that exists only as part of a component's contract has no entry, because it never owns a bead of its own — it reaches the tracker inside the bead of the component it belongs to.

Stating that in the schema rather than leaving it implied is what makes the boundary fail closed. `Store.Replace` re-validates the entire candidate file against this schema before the atomic rename, so an ingest that somehow materialised a record for an unmappable kind is rejected at the write boundary instead of landing a record that no later run could resolve.

The guarantee lives on that one method, and it is worth knowing which one. `Replace` is the store's whole production write path — ingest's reconcile and refresh commit through it and nothing else writes — so on the pipeline the check is unconditional. The store's other mutators (`Create`, `Update`, `Delete`) write straight through with no schema check; a bad record placed through one of them would reach disk and surface only on the next load, which validates the file it read. The corollary is that retiring a kind of spec content that was never in the enum is not a bead-map migration: no record named one, so no record has to be rewritten.

### Internal record `id` stays integer

The record `id` field is still an integer auto-incremented by MappingStore. It is internal to the bead-map file (used to generate the `spex:<record-id>` bead label) and is not referenced from the spec graph, so it does not need to be a hash.

### No uniqueness in schema

The schema does not enforce uniqueness of `bead_id` or `spec_node_id` across records. JSON Schema cannot express cross-record uniqueness constraints. These are enforced programmatically by MappingStore (`bead_id` must be unique; `spec_node_id` may repeat since one spec node can have many beads).

### bead_status is optional

The `bead_status` field carries a live tracker status and is populated only by callers that have queried the tracker. It is not required on disk — `spex ingest` writes records from changeset ops and receipts, neither of which carries a status, so reconciled records normally omit it. Consumers must treat its absence as "status unknown", never as a value.
