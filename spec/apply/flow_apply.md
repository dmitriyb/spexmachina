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
│ BeadCreator      │── <bin> create proposal epic first (if any create actions)
│                  │   all subsequent creates use --parent <proposal-epic-id>
│                  │   creation order within run: features + data_flow tasks
│                  │     (topologically sorted by DepBeadIDs), then test tasks
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
2. Create proposal epic — first create action in any run with at least one `create`. Epic is named after the proposal (`--title "<proposal>"`), typed `--type epic`, parent unset. Its bead ID is reused as `--parent` for every subsequent create in this run.
3. Creates in hierarchy order with topological sort — features (components) and task-type creates (data_flow, multi-component test_section) in a single pool sorted by `DepBeadIDs` so dependency beads are created before dependents. Each create passes `--parent <proposal-epic-id>` for hierarchy, `--deps depends:<id>` for spec-graph dependencies, `--deps blocks:<id>` for lineage. Old beads are still open at this point.
4. Close obsoletes — `<bin> close` all beads labeled in step 1. Replacements already exist.
5. Save snapshot last — marks apply as complete.

## Mapping File Maintenance

Each bead operation stage also maintains the corresponding mapping record in `.bead-map.json`:
- **Obsolete (removed)**: Removes the record from the mapping file
- **Obsolete (modified)**: Leaves the record intact for BeadCreator to update
- **Create (new)**: Adds a record with the new bead ID, bead type, and spec metadata, then labels the bead with `spex:<record-id>`
- **Create (modified)**: Updates the existing record with the new bead ID and spec hash
- **Create (cleanup)**: No mapping record (component removed from spec); bead labeled `spex:cleanup`
- **Proposal epic**: A bead-map record is written with `node_type = proposal`, `bead_type = epic`, `spec_node_id = <proposal-reference>`. This lets subsequent runs look up the correct parent if needed for reporting, though each run normally creates a fresh epic.

The mapping file is committed to git alongside `.snapshot.json` after apply completes.

## Error Handling

If any step fails, subsequent steps do not run. The snapshot is not saved, so the next `spex apply` will retry all actions. Already-created beads are detected via idempotency checks (no duplicates). Orphaned mapping records (from partial failures) are cleaned up on retry.

## Input

The impact report is read from stdin (for piping from `spex impact`) or from a file path argument.

## Data Shapes

### ImpactReport → BeadCreator / BeadCloser (input)

- ImpactReport (as defined in impact/flow_impact_analysis.md)

### BeadCreator → bead CLI (outbound invocation)

Epic creation (once per run, first):
- `<bin> create --type epic --title "<proposal>" --priority <p>`
- Returns: bead_id (string, `<project-slug>-<suffix>`)

Feature/task creation:
- `<bin> create --type feature|task --title "<node-name>" --parent <epic-bead-id>
   --deps depends:<id1>,depends:<id2>,blocks:<old-id> --priority <p>`
- Returns: bead_id

### BeadCreator → .bead-map.json

- MapRecord:
  - id: integer — internal record id (monotonic within file)
  - bead_id: string
  - bead_type: string enum — `epic` | `feature` | `task`
  - node_type: string enum — `proposal` | `component` | `data_flow` | `test_section`
  - spec_node_id: string — 12-char hex identity hash, OR proposal reference
    when node_type = `proposal`
  - spec_hash: string — 64-char hex content hash at creation time (empty for epics)
  - module: string — 12-char hex identity hash of parent module (empty for proposal epics)

### BeadCreator / BeadCloser → bead label

- Label format: `spex:<record-id>` (record id from MapRecord.id)
- For cleanup beads (component removed): label is `spex:cleanup`, no MapRecord

### SnapshotSaver → spec/.snapshot.json

- Snapshot (as defined in merkle/flow_diff_classification.md)
