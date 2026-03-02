# Impact Analysis Flow

## Data Flow

```
merkle diff                bead metadata
(classified changes)       (from bd list --json)
     │                          │
     ▼                          ▼
┌──────────────────────────────────┐
│ NodeMatcher                       │
│ index beads by spec coords        │
│ look up each changed node         │
└──────────┬───────────────────────┘
           │ matched[], unmatched[], orphaned[]
           ▼
┌──────────────────┐
│ ActionClassifier  │
│ apply decision    │
│ table per match   │
└──────────┬───────┘
           │ actions[]
           ▼
┌──────────────────┐
│ ReportGenerator   │
│ format JSON       │
│ write to stdout   │
└──────────┬───────┘
           │
           ▼
    impact report (JSON, stdout)
```

## Pipeline Position

Impact sits between merkle diff and apply:

```
spex validate → spex hash → spex diff → spex impact → spex apply
```

The impact report is the decision document — it shows what will happen before `apply` executes it. This supports the supervised spec change workflow: review the impact report, then approve apply.
