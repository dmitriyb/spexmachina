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
