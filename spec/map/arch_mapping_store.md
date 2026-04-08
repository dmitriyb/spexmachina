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
| `module` | string | Module name (human-readable, for context-resolver output and debug) |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Path to the spec content markdown file |
| `spec_hash` | string | Merkle content hash of the spec node at time of mapping |

`spec_node_id` is the bead-map's primary key into the spec graph. It is identical, byte for byte, to the merkle tree key for the same node, so the impact command can look up changed merkle nodes in the bead-map with no translation step.

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

The mapping file lives at `spec/.bead-map.json`, adjacent to `.snapshot.json`. It is committed to git alongside the spec but is NOT hashed into the merkle tree — it is metadata about the relationship between spec and beads, not spec content.

## Design Rationale

### Why a separate file?

Embedding mapping data in module.json would make spec content depend on bead state, breaking the separation of concerns. The mapping file is maintained by `spex apply`, not by spec authoring.

### Why not bead labels?

Bead labels are limited in capacity and format. Complex metadata in labels couples spex to the bead CLI's label format. The mapping file gives spex full control over the data structure.

### Why auto-incrementing record IDs?

The record `id` field is what gets stored in the bead label (`spex:42`). Integer record IDs are compact, predictable, and easy to type, and they are assigned by MappingStore so the caller never has to coordinate. This is distinct from the spec graph's identity hashes: the record `id` is internal to the bead-map and is not referenced from any spec node, so it does not need to be content-addressable. The `spec_node_id` field on the same record holds the identity hash, which is the actual link into the spec graph.
