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

Impact sits between merkle diff and the emit → adapter → ingest tail:

```
spex validate → spex diff → spex impact → spex emit → adapter → spex ingest
```

The impact report is the decision document — it shows what will happen before `spex emit` turns it into a changeset and the adapter executes it. This supports the supervised spec change workflow: review the impact report, then approve the emit → adapter run.

## Data Shapes

### merkle diff → ActionClassifier input

- ClassifiedChange (as defined in merkle/flow_diff_classification.md)

### BeadReader → NodeMatcher

- BeadRecord:
  - bead_id: string (e.g., `spexmachina-abc`)
  - spec_record_id: integer — internal `.bead-map.json` record id, extracted
    from the bead's `spex:<id>` label
  - spec_node_id: string, 12-char hex identity hash (from mapping file lookup
    using spec_record_id)
  - status: string enum — `open` | `in_progress` | `blocked` | `closed`

### NodeMatcher → ActionClassifier

- MatchedPair:
  - change: ClassifiedChange
  - bead: BeadRecord | null (null means unmatched — no existing bead)
- structural_skipped: list of ClassifiedChange (skipped per impact_level)

NodeMatcher forwards `impl_only`, `contract`, and `arch_impl` changes to the
ActionClassifier. It skips `structural` (module.json, project.json, requirement
leaves). Contract-level changes (data_flow) are forwarded — not skipped — so
that a dedicated data_flow task bead is produced.

### ActionClassifier → ReportGenerator

- Action:
  - kind: string enum — `create` | `obsolete`
  - node_type: string enum — `component` | `data_flow` | `test_section`
    (impl_section is never an action target)
  - spec_node_id: string, 12-char hex
  - bead_id: string — target bead (create: empty until the adapter run; obsolete: existing)
  - dep_bead_ids: list of string — resolved spec-graph dependencies
    (uses and requires_module → current open bead IDs); populated on `create`
    actions only
  - lineage_bead_id: string — for modified nodes, the obsoleted predecessor
    (used as `--deps blocks:<id>`); empty for purely added nodes

Gating rules applied by ActionClassifier:

| node_type | produce bead? |
|-----------|---------------|
| module | yes (via proposal epic creation, handled by emit) |
| component | yes (feature) |
| data_flow | yes (task) |
| test_section, len(describes) >= 2 | yes (task) |
| test_section, len(describes) == 1 | no (bundled with that component's feature bead) |
| impl_section | no |

### ReportGenerator → downstream (emit)

- ImpactReport:
  - proposal: string — proposal reference (from the register/emit flag)
  - actions: list of Action
  - generated_at: string, ISO-8601 UTC timestamp

Emit consumes this shape as its input.
