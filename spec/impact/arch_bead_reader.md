# BeadReader

Reads bead metadata from the bead CLI (`br` or `bd`). Extracts the mapping record ID from the simplified bead label (`spex:<record-id>`) for correlation with spec nodes.

## Responsibilities

- Call `<bin> list --json` to get all beads with labels
- Extract the `spex:<record-id>` label from each bead
- Build a lookup structure mapping bead IDs to record IDs for the NodeMatcher

## Interface

```go
type BeadSpec struct {
    ID       string // bead ID
    RecordID int    // mapping record ID from "spex:<id>" label
}

func ReadBeads(ctx context.Context) ([]BeadSpec, error)
```

## Bead CLI Interaction

Uses `exec.CommandContext(ctx, bin, "list", "--json")` to get bead data, where `bin` is `"br"` or `"bd"`. The `--json` flag outputs machine-readable JSON with a `labels` array of `key:value` strings. Parse the output and extract the `spex` label (e.g., `spex:42`).

## Label Format

Each spec-managed bead has a single label: `spex:<record-id>`. The record ID is an integer that indexes into `.bead-map.json`. This replaces the previous multi-label format (`spec_module:...`, `spec_component:...`, `spec_hash:...`).

## Error Handling

- If the bead CLI (`br` or `bd`) is not installed or not in PATH, return a clear error
- If no beads have a `spex:` label, return an empty slice (not an error)
- Wrap all errors with `"impact: read beads: ..."` context
