# Apply Flow

## Data Flow

```
impact report (JSON, stdin)
     │
     ▼
┌─────────────┐
│ Parse report │── deserialize creates, obsoletes
└──────┬──────┘
       │
       ▼
┌─────────────────┐
│ BeadCloser       │── LABEL PHASE: <bin> update with spex:obsolete + commit:<HEAD>
│ (label phase)    │   beads stay open — marks intent
│                  │   for removed nodes: delete mapping record
│                  │   for modified nodes: leave record (BeadCreator updates it)
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ BeadCreator      │── <bin> create with --type, --parent, --deps, --priority
│                  │   hierarchy order: epics → features → tasks
│                  │   topological sort within each type level (DepBeadIDs)
│                  │   --deps blocks:<old> (lineage) + --deps depends:<dep> (spec-graph)
│                  │   old beads still open — lineage refs are valid
│                  │   create/update mapping record in .bead-map.json
│                  │   set bead label to spex:<record-id>
│                  │   cleanup beads: spex:cleanup label, no mapping record
└──────┬──────────┘
       │
       ▼
┌─────────────────┐
│ BeadCloser       │── CLOSE PHASE: <bin> close <bead_id>
│ (close phase)    │   replacements exist — safe to close
└──────┬──────────┘
       │
       ▼
┌────────────────┐
│ ProposalTagger  │── <bin> update metadata with proposal ref
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

Where `<bin>` is the configured bead CLI binary (`br` or `bd`).

## Execution Order

1. Label obsoletes — mark beads with `spex:obsolete` + `commit:<HEAD>` labels via `<bin> update`, keep open. Delete mapping records for removed nodes.
2. Creates in hierarchy order with topological sort — epics (modules) first, then features (components), then tasks (test_sections). Within each type level, topological sort by `DepBeadIDs` ensures dependency beads are created before dependents. Each level's parent IDs are resolved from the mapping file. Each create passes `--deps depends:<id>` for spec-graph dependencies in addition to `--deps blocks:<id>` for lineage. Old beads are still open at this point.
3. Close obsoletes — `<bin> close` all beads labeled in step 1. Replacements already exist.
4. Tag all affected beads with proposal reference
5. Save snapshot last — marks apply as complete

## Mapping File Maintenance

Each bead operation stage also maintains the corresponding mapping record in `.bead-map.json`:
- **Obsolete (removed)**: Removes the record from the mapping file
- **Obsolete (modified)**: Leaves the record intact for BeadCreator to update
- **Create (new)**: Adds a record with the new bead ID, bead type, and spec metadata, then labels the bead with `spex:<record-id>`
- **Create (modified)**: Updates the existing record with the new bead ID and spec hash
- **Create (cleanup)**: No mapping record (component removed from spec); bead labeled `spex:cleanup`

The mapping file is committed to git alongside `.snapshot.json` after apply completes.

## Error Handling

If any step fails, subsequent steps do not run. The snapshot is not saved, so the next `spex apply` will retry all actions. Already-created beads are detected via idempotency checks (no duplicates). Orphaned mapping records (from partial failures) are cleaned up on retry.

## Input

The impact report is read from stdin (for piping from `spex impact`) or from a file path argument.
