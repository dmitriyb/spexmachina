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
