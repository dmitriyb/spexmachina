# Diff and Classification Flow

## Data Flow

```
current tree          stored snapshot
     │                      │
     ▼                      ▼
┌──────────────────────────────┐
│ DiffEngine                    │── compare node-by-node
│ (flatten + set operations)    │
└──────────┬───────────────────┘
           │ changes[]
           ▼
┌──────────────────┐
│ ImpactClassifier  │── classify by node metadata
│ (NodeType switch) │   aggregate by module
└──────────┬───────┘
           │ classified_changes[]
           ▼
    JSON output (stdout)
```

## Input

- Current merkle tree (just built from spec directory)
- Stored snapshot (loaded from `spec/.snapshot.json`)

## Output

A list of classified changes, each with:
- File path
- Change type (added/removed/modified)
- Impact level (impl_only/contract/arch_impl/structural)
- Module name

This output feeds directly into the Impact module for bead matching.

## Data Shapes

### SnapshotStore → DiffEngine

- Snapshot:
  - nodes: map keyed by node.key (identity_hash or `meta/*`) → Node
    (Node shape as defined in flow_hash_computation.md)
  - root_hash: string, 64-character lowercase hex

Both `current` and `previous` Snapshots have the same shape. The diff algorithm
treats missing keys as added/removed and matching keys with different hashes as
modified.

### DiffEngine → ImpactClassifier

- Change:
  - key: string — identity_hash or `meta/*`
  - node_type: string enum — `component` | `requirement` | `impl_section` |
    `data_flow` | `test_section` | `meta`
  - module: string — identity_hash of the parent module, empty for project-level
  - change_type: string enum — `added` | `removed` | `modified`
  - old_hash: string, 64-char hex (empty for added)
  - new_hash: string, 64-char hex (empty for removed)

### ImpactClassifier → downstream

- ClassifiedChange: Change plus
  - impact_level: string enum — `impl_only` | `contract` | `arch_impl` |
    `structural`

Mapping used by the classifier:

| node_type   | impact_level |
|-------------|--------------|
| impl_section, test_section | impl_only |
| data_flow   | contract     |
| component   | arch_impl    |
| meta, requirement | structural |

The `contract` level is new — it is a cross-component shape change within a
module that downstream matchers MUST NOT skip; it must reach the action
classifier so a dedicated data_flow task bead is created.
