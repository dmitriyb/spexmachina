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
       ▼  (user runs spex diff → spex impact)
  impact report
       │
       ▼  (user runs spex apply with proposal ref)
  bead actions + proposal tagging
       │
       ▼  (user runs spex log)
┌───────────────┐
│ HistoryViewer  │── show proposal → spec → bead audit trail
└───────────────┘
```

## Key Insight

The proposal module is a bookend: registration happens before the spec change, history viewing happens after. The middle steps (spec authoring, diff, impact, apply) are handled by other modules. The proposal reference threads through the entire chain via bead metadata.

## Traceability Chain

```
Conversation → Proposal → Spec Change → Merkle Diff → Impact Report → Bead Actions
```

Any point in this chain can be traced forward or backward. The proposal is the anchor point that explains "why" a change was made.

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
- Passed as a flag to `spex apply --proposal <ref>`
- Used by BeadCreator as the proposal epic `--title` value
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
