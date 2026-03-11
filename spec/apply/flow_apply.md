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
┌─────────────────┐
│ BeadCreator      │── bead create for each new spec node
│                  │   create mapping record in .bead-map.json
│                  │   set bead label to spex:<record-id>
│                  │   returns new bead IDs
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ BeadUpdater      │── bead update spec_hash for modified nodes
│                  │   update mapping record spec_hash
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ BeadCloser       │── bead close for removed spec nodes
│                  │   remove mapping record from .bead-map.json
└──────┬──────────┘
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

1. Creates first — new beads and mapping records exist before tagging
2. Updates second — existing beads and mapping records get new spec_hash
3. Closes third — obsolete beads and mapping records removed after everything else succeeds
4. Tag all affected beads with proposal reference
5. Save snapshot last — marks apply as complete

## Mapping File Maintenance

Each bead operation stage (create/update/close) also maintains the corresponding mapping record in `.bead-map.json`:
- **Create**: Adds a record with the new bead ID and spec metadata, then labels the bead with `spex:<record-id>`
- **Update**: Updates the record's `spec_hash` to match the new spec content hash
- **Close**: Removes the record from the mapping file

The mapping file is committed to git alongside `.snapshot.json` after apply completes.

## Error Handling

If any step fails, subsequent steps do not run. The snapshot is not saved, so the next `spex apply` will retry all actions. Already-created beads are detected via idempotency checks (no duplicates). Orphaned mapping records (from partial failures) are cleaned up on retry.

## Input

The impact report is read from stdin (for piping from `spex impact`) or from a file path argument.
