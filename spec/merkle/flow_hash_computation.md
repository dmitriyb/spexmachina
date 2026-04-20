# Hash Computation Flow

## Data Flow

```
spec directory
     │
     ▼
┌────────────┐
│ Read        │── project.json → module paths
│ project.json│   module.json  → content file paths
└──────┬─────┘
       │ file paths
       ▼
┌────────────┐
│ Hasher      │── SHA-256 each file
│ (leaves)    │
└──────┬─────┘
       │ leaf hashes
       ▼
┌────────────┐
│ TreeBuilder │── group by type, compute interior hashes
│ (interior)  │   bottom-up: leaves → groups → modules → root
└──────┬─────┘
       │ complete tree
       ▼
┌──────────────┐
│SnapshotStore │── serialize tree to spec/.snapshot.json
└──────────────┘
```

## Input

The spec directory must be valid (pass `spex validate`). Tree building reads:
- `spec/project.json`
- `spec/<module>/module.json` for each module
- All content files referenced by `content` fields

## Output

A merkle tree data structure with hashes at every level, serializable to a snapshot file.

## Data Shapes

Shapes flowing between the three participating components. A change to any field
here is a contract change: all three components must be updated in lockstep.

### Hasher → TreeBuilder

- file_hash: string, 64-character lowercase hex SHA-256 digest of file bytes
- identity_hash: string, 12-character lowercase hex (spec node ID the leaf represents)

### TreeBuilder → SnapshotStore

- Node:
  - key: string — identity_hash for spec-node leaves; `meta/<module-identity-hash>`
    for module.json envelopes; `meta/project` for project.json envelope
  - hash: string, 64-character lowercase hex SHA-256 digest
  - type: string enum — `leaf` | `module` | `project`
  - node_type: string enum — `component` | `requirement` | `impl_section` |
    `data_flow` | `test_section` | `meta` (for envelope leaves)
  - module: string — identity_hash of the parent module; empty string for
    project-level nodes
  - children: list of Node — empty for leaves; sorted by `key` ascending for
    interior nodes (sort order is part of the hash contract)

### Tree root

The root Node has key `root`, type `project`, and children = [project envelope
leaf, each module's subtree]. The root hash is the SHA-256 of the sorted
concatenation of children hashes.
