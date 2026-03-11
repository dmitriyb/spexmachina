# MappingStore

Owns the `.bead-map.json` file — the single source of truth for spec-to-bead correlation.

## Responsibilities

- CRUD operations on mapping records
- Ensure referential uniqueness (one spec node → one bead, one bead → one spec node)
- Provide lookup by record ID, bead ID, or spec node ID
- Atomic file writes to prevent corruption

## Record Format

Each mapping record contains:

```json
{
  "id": 42,
  "spec_node_id": "impact/component/3",
  "bead_id": "abc-123",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Auto-incrementing record ID, unique within the mapping file |
| `spec_node_id` | string | Composite key: `<module>/<node_type>/<node_id>` |
| `bead_id` | string | Bead ID from `br` or `bd` |
| `module` | string | Module name |
| `component` | string | Component or section name (human-readable) |
| `content_file` | string | Path to the spec content markdown file |
| `spec_hash` | string | Merkle hash of the spec node at time of mapping |

## Interface

```go
type Record struct {
    ID          int    `json:"id"`
    SpecNodeID  string `json:"spec_node_id"`
    BeadID      string `json:"bead_id"`
    Module      string `json:"module"`
    Component   string `json:"component"`
    ContentFile string `json:"content_file"`
    SpecHash    string `json:"spec_hash"`
}

type Store interface {
    Create(r Record) (int, error)
    Get(id int) (Record, error)
    GetByBead(beadID string) (Record, error)
    GetBySpecNode(specNodeID string) (Record, error)
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

### Why auto-incrementing IDs?

The record ID is what gets stored in the bead label (`spex:42`). Integer IDs are compact, predictable, and easy to type. The ID is assigned by MappingStore, not by the caller.
