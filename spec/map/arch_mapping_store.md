# MappingStore

Owns the `.bead-map.json` file — the single source of truth for spec-to-bead correlation.

## Responsibilities

- CRUD operations on mapping records
- Ensure bead uniqueness (one bead → one spec node; one spec node may have many beads)
- Provide lookup by record ID, bead ID, or spec node ID
- Atomic file writes to prevent corruption

## Record Format

Each mapping record contains:

```json
{
  "id": 42,
  "spec_node_id": "a1b2c3d4e5f6",
  "bead_id": "abc-123",
  "bead_type": "feature",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Auto-incrementing record ID, unique within the mapping file. Used as the bead label `spex:<id>`. Stays integer because it is internal to the bead-map and never referenced from the spec graph. |
| `spec_node_id` | string | Identity hash of the spec node (12-char lowercase hex, pattern `^[a-f0-9]{12}$`). Identical to the merkle tree key for the same node. |
| `bead_id` | string | Bead ID from `br` or `bd` |
| `bead_type` | string | Bead issue type (`epic`, `feature`, or `task`) — determined by spec node type. Carried as a separate field because identity hashes do not embed type information. |
| `node_type` | string | The kind of spec node the record points at: `component`, `data_flow`, `test_section`, or `proposal`. Optional — it is present on records that need to distinguish a proposal epic from a spec-graph node, and the bead-map schema constrains it to that closed set. |
| `module` | string | Module name (human-readable, for context-resolver output and debug) |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Path to the spec content markdown file |
| `spec_hash` | string | Merkle content hash of the spec node at time of mapping |

`spec_node_id` is the bead-map's primary key into the spec graph. It is identical, byte for byte, to the merkle tree key for the same node, so the impact command can look up changed merkle nodes in the bead-map with no translation step.

A record points at a whole node, never at a part of one. The values `node_type` may take are exactly the kinds of node that can own a bead: a section of a component's contract is content belonging to that component's leaf and reaches the tracker inside the component's bead, so it never acquires a record, an identity in this file, or a `spex:<id>` label of its own. That is what makes the mapping store indifferent to how a component's contract is laid out across files — the store keys on the component, and a change to which files carry that contract is a content change to the component's leaf, not a change to the set of records.

## Interface

```go
type Record struct {
    ID          int    `json:"id"`
    SpecNodeID  string `json:"spec_node_id"`
    BeadID      string `json:"bead_id"`
    BeadType    string `json:"bead_type"`
    Module      string `json:"module"`
    Component   string `json:"component"`
    ContentFile string `json:"content_file"`
    SpecHash    string `json:"spec_hash"`
}

type Store interface {
    Create(r Record) (int, error)
    Get(id int) (Record, error)
    GetByBead(beadID string) (Record, error)
    GetBySpecNode(specNodeID string) ([]Record, error)
    Update(id int, updates map[string]string) error
    Delete(id int) error
    List() ([]Record, error)
}
```

## File Location

The mapping file is `.bead-map.json` in the repository root, not inside the spec directory. Every command that touches it — `diff`, `impact`, `emit`, `ingest` via `--map`, and `map get/list/context` via `--map-file` — defaults to that bare relative path, resolved against the working directory, and `scripts/apply-br.sh` reads the same default through `SPEX_MAPPING_FILE`. It is never derived from `--spec-dir`.

That separation is deliberate. `spec/.snapshot.json` belongs to the spec tree; `.bead-map.json` does not. The file is committed to git but is NOT hashed into the merkle tree — it is metadata about the relationship between spec and beads, not spec content, and keeping it outside `--spec-dir` means pointing spex at a different spec directory does not silently point it at a different mapping store.

## Design Rationale

### Why a separate file?

Embedding mapping data in module.json would make spec content depend on bead state, breaking the separation of concerns. The mapping file is maintained by `spex ingest`, which reconciles it from a changeset and the adapter's receipts — never by spec authoring, and never by the tracker directly. Emit only reads the store (record lookup and the `next_id` counter); ingest is the sole writer.

### Why not bead labels?

Bead labels are limited in capacity and format. Complex metadata in labels couples spex to the bead CLI's label format. The mapping file gives spex full control over the data structure.

### Why auto-incrementing record IDs?

The record `id` field is what gets stored in the bead label (`spex:42`). Integer record IDs are compact, predictable, and easy to type, and they are assigned by MappingStore so the caller never has to coordinate. This is distinct from the spec graph's identity hashes: the record `id` is internal to the bead-map and is not referenced from any spec node, so it does not need to be content-addressable. The `spec_node_id` field on the same record holds the identity hash, which is the actual link into the spec graph.
