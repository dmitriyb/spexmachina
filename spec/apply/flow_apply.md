# Apply Flow

## Data Flow

```
impact report (JSON, stdin)
     │
     ▼
┌─────────────┐
│ Parse report │── deserialize creates, closes, reviews
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ BeadCreator  │── bead create for each new spec node
│              │   returns new bead IDs
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ BeadUpdater  │── bead update metadata for modified nodes
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ BeadCloser   │── bead close for removed spec nodes
└──────┬──────┘
       │
       ▼
┌────────────────┐
│ ProposalTagger  │── bead update metadata with proposal ref
│                 │   for all affected beads
└──────┬─────────┘
       │
       ▼
┌───────────────┐
│ SnapshotSaver  │── compute tree, save .snapshot.json
└──────┬────────┘
       │
       ▼
  apply complete
  (exit 0)
```

## Execution Order

1. Creates first — new beads exist before tagging
2. Updates second — existing beads get new metadata
3. Closes third — obsolete beads closed after everything else succeeds
4. Tag all affected beads with proposal reference
5. Save snapshot last — marks apply as complete

## Error Handling

If any step fails, subsequent steps do not run. The snapshot is not saved, so the next `spex apply` will retry all actions. Already-created beads are detected via idempotency checks (no duplicates).

## Input

The impact report is read from stdin (for piping from `spex impact`) or from a file path argument.
