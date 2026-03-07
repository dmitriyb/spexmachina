# History Tracking Implementation

## Approach

Combine filesystem listing of proposals with bead metadata queries.

## Algorithm

1. List files in `spec/proposals/` matching `*.md`
2. Sort by filename (which sorts by date due to the naming convention)
3. Call `<bin> list --json` (where `<bin>` is `br` or `bd`) to get all beads
4. For each proposal filename, filter beads where `metadata.spec_proposal == filename`
5. Group matched beads by proposal
6. Format and output

## Performance

Bead listing is done once. Filtering is done in-memory. For projects with hundreds of proposals and thousands of beads, this is still fast — both datasets fit in memory easily.

## JSON Output Mode

When `--json` flag is set, output a JSON array of proposal records:

```json
[
  {
    "proposal": "2026-02-23-spex-machina.md",
    "type": "project",
    "date": "2026-02-23",
    "beads": [
      {"id": "spexmachina-abc", "action": "created", "module": "schema", "node": "ProjectSchema"}
    ]
  }
]
```
