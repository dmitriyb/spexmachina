# Bead Creation Mapping Flow

## Data Flow

```
spex apply (reads impact report)
     │
     ▼
┌──────────────┐
│ BeadCreator   │── creates bead via br/bd
│               │   captures new bead ID
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ MappingStore  │── Create(record) with:
│               │   - spec_node_id from impact report
│               │   - bead_id from BeadCreator
│               │   - metadata from spec graph
│               │   returns record ID
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ BeadCreator   │── sets bead label to "spex:<record-id>"
│               │   via br update <bead-id> --add-label spex:<id>
└──────┬───────┘
       │
       ▼
  bead + mapping record created
```

## Lifecycle

### Create (new spec node)

1. `spex apply` processes a "create" action from the impact report
2. BeadCreator creates the bead via `br create`
3. BeadCreator calls MappingStore.Create with the bead ID and spec node metadata
4. MappingStore assigns a record ID and writes to `.bead-map.json`
5. BeadCreator sets the bead label to `spex:<record-id>`

### Update (modified spec node)

1. `spex apply` processes a "review" action
2. BeadUpdater updates `spec_hash` on the bead label
3. BeadUpdater calls MappingStore.Update to update the spec_hash in the mapping record

### Delete (removed spec node)

1. `spex apply` processes a "close" action
2. BeadCloser closes the bead via `br close`
3. BeadCloser calls MappingStore.Delete to remove the mapping record

## Invariants

- Every open, spec-managed bead has exactly one mapping record
- Every mapping record's bead_id points to a bead that has a `spex:<record-id>` label
- The label value matches the mapping record's ID
- No orphaned records (record without a bead) or orphaned labels (label without a record)

## Data Shapes

### BeadCreator → MappingStore.Create (request)

- CreateRequest:
  - bead_id: string — newly created bead ID from bead CLI (e.g., `spexmachina-abc`)
  - bead_type: string enum — `epic` | `feature` | `task`
  - node_type: string enum — `proposal` | `component` | `data_flow` | `test_section`
  - spec_node_id: string — 12-char hex identity hash, OR a proposal reference
    (e.g., `2026-04-12-data-flow-contract-layer`) when node_type = `proposal`
  - spec_hash: string — 64-char hex content hash (empty for epic records)
  - module: string — 12-char hex identity hash of parent module
    (empty for proposal epic records)

### MappingStore → on-disk format (.bead-map.json)

- MapFile:
  - version: integer — schema version (currently 1)
  - next_id: integer — monotonic counter for new record IDs
  - records: list of MapRecord

- MapRecord (same shape as CreateRequest plus):
  - id: integer — assigned by MappingStore from next_id

### MappingStore → BeadCreator (response)

- record_id: integer — freshly assigned id; used by BeadCreator as the
  `spex:<record-id>` label value

### MappingStore query interface (CLI `spex map get`)

- QueryResponse:
  - records: list of MapRecord matched by the query predicate
  - not_found: list of input identifiers that had no matching record

No field renames, additions, or removals of these shapes without updating
every consumer in BeadCreator, BeadCloser, and the `spex map` CLI command.
