# BeadReader

Reads bead metadata from the bd CLI to extract spec-related fields.

## Responsibilities

- Call `bd list --json` to get all beads with metadata
- Extract spec-related metadata fields: `module`, `component`, `impl_section`, `spec_hash`
- Build a lookup structure for the NodeMatcher

## Interface

```go
type BeadSpec struct {
    ID          string
    Module      string
    Component   string
    ImplSection string
    SpecHash    string
}

func ReadBeads(ctx context.Context) ([]BeadSpec, error)
```

## bd CLI Interaction

Uses `exec.CommandContext(ctx, "bd", "list", "--json")` to get bead data. The `--json` flag outputs machine-readable JSON. Parse the output and extract `metadata.*` fields.

## Error Handling

- If `bd` is not installed or not in PATH, return a clear error
- If no beads have spec metadata, return an empty slice (not an error)
- Wrap all errors with `"impact: read beads: ..."` context
