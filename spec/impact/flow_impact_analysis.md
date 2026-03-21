# Impact Analysis Flow

## Data Flow

```
merkle diff                bead metadata        mapping file
(classified changes)       (from bead list)     (.bead-map.json)
     │                          │                     │
     ▼                          ▼                     │
┌──────────────────────────────────┐                  │
│ NodeMatcher                       │                  │
│ index beads by spec coords        │                  │
│ look up each changed node         │                  │
└──────────┬───────────────────────┘                  │
           │ matched[], unmatched[], orphaned[]        │
           ▼                                           │
┌──────────────────────────┐                           │
│ ActionClassifier          │                           │
│ state transition table    │◄──────────────────────────┘
│ + dependency resolution   │  resolve uses + requires_module
│ modified → obsolete+create│  to bead IDs via mapping file
│ create → attach DepBeadIDs│
└──────────┬───────────────┘
           │ actions[] (create w/ DepBeadIDs, obsolete)
           ▼
┌──────────────────┐
│ ReportGenerator   │
│ format JSON       │
│ include dep_bead_ │
│ ids on creates    │
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
