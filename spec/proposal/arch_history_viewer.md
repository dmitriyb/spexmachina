# HistoryViewer

Shows proposal history — which proposals led to which spec changes and bead actions.

## Responsibilities

- List all proposals in `spec/proposals/` with dates
- For each proposal, find beads tagged with `spec_proposal=<filename>`
- Show the link chain: proposal → beads created/closed/modified

## Interface

```go
func ShowHistory(ctx context.Context, specDir string, w io.Writer) error
```

## Output Format

```
2026-02-23-spex-machina.md (project proposal)
  Created: spexmachina-abc (schema: ProjectSchema)
  Created: spexmachina-def (validator: SchemaChecker)
  ...

2026-03-01-add-caching.md (change proposal)
  Modified: spexmachina-ghi (merkle: SnapshotStore)
  Created: spexmachina-jkl (merkle: CacheLayer)
```

## Bead Query

Uses `<bin> list --json` (where `<bin>` is `br` or `bd`) and filters beads by `spec_proposal` metadata field. Groups results by proposal filename.

## Composability

Output is written to a writer (stdout by default). In JSON mode (`--json` flag), output is machine-readable for piping.
