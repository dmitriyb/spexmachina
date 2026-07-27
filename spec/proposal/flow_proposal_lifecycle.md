# Proposal Lifecycle Flow

## Data Flow

```
proposal file (authored by human or LLM)
     │
     ▼
┌────────────┐
│ Registrar   │── validate sections, copy to spec/proposals/
└──────┬─────┘
       │
       ▼
  spec/proposals/YYYY-MM-DD-name.md
       │
       ▼  (user runs /spec with the proposal)
  spec changes (project.json, module.json, *.md)
       │
       ▼  (spex validate → spex diff → spex impact)
  impact report
       │
       ▼  (spex emit --proposal <ref>)
  changeset.json — proposal-epic create op first, then the ordered bead ops
       │
       ▼  (external adapter, e.g. scripts/apply-br.sh)
  receipts.json — bead ids for every executed op
       │
       ▼  (spex ingest)
  reconciled .bead-map.json + new snapshot
       │
       ▼  (user runs spex log)
┌───────────────┐
│ HistoryViewer  │── show proposal → spec → bead audit trail
└───────────────┘
```

## Key Insight

The proposal module is a bookend: registration happens before the spec change,
history viewing happens after. The middle steps — spec authoring, diff, impact,
emit, adapter execution, ingest — are handled by other modules and by the
adapter outside the binary. The proposal reference threads through the entire
chain: `spex emit --proposal <ref>` stamps it on the changeset, the changeset's
first op creates the proposal epic bead, and every other op in the batch is
parented under that epic. That parentage is what HistoryViewer walks back.

## Traceability Chain

```
Conversation → Proposal → Spec Change → Merkle Diff → Impact Report
            → Changeset → Receipts → Beads + Bead-Map
```

Any point in this chain can be traced forward or backward. The proposal is the
anchor point that explains "why" a change was made; the changeset and receipts
are the durable record of "what was actually executed", so the trail survives
even a partial adapter run.

## Data Shapes

### Proposal file on disk (spec/proposals/YYYY-MM-DD-name.md)

- Markdown document with required H1 heading `# Change Proposal: <title>`
- Required H2 sections (validated by Registrar): `Context`, `Proposed change`,
  `Impact expectation`
- Optional H2 sections: any — treated as freeform

### Registrar → HistoryViewer shared index

- ProposalRecord:
  - path: string — relative path under spec/proposals/
  - date: string, ISO-8601 date (YYYY-MM-DD, extracted from filename prefix)
  - slug: string — filename portion after the date prefix, `.md` stripped
    (used as the proposal reference throughout the rest of the pipeline)
  - title: string — from the H1 heading
  - registered_at: string, ISO-8601 UTC timestamp (optional; set on
    `spex register`)

### Proposal reference (pipeline-wide contract)

- string of the form `YYYY-MM-DD-name` (matches the filename stem)
- Passed as the required `--proposal <ref>` flag to `spex emit`, which copies it
  into the changeset's top-level `proposal` field
- Carried by the proposal-epic create op: `spec_node_kind = "proposal_epic"`,
  `spec_node_id` = the reference itself (not an identity hash), title
  `"Proposal: <ref>"`. Ingest materialises its mapping record with
  `node_type = "proposal"` and no spec-graph lookup.
- Used by HistoryViewer (`spex log`) to group beads under the proposal

### HistoryViewer → stdout

- ProposalLog:
  - proposal: ProposalRecord
  - epic_bead_id: string — bead_id of the proposal epic (empty for
    pre-contract-layer proposals that used labels instead)
  - created_beads: list of string — bead IDs created by this proposal
  - obsoleted_beads: list of string — bead IDs obsoleted by this proposal

The `epic_bead_id` field is empty for historical proposals that ran before the
per-proposal epic mechanism shipped; in that case HistoryViewer falls back to
proposal-label lookups.
