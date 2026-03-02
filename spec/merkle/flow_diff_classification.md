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
│ ImpactClassifier  │── classify by filename pattern
│ (path analysis)   │   aggregate by module
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
- Impact level (impl_only/arch_impl/structural)
- Module name

This output feeds directly into the Impact module for bead matching.
