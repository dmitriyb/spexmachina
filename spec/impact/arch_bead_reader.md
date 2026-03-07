# BeadReader

Reads bead metadata from the bead CLI (`br` or `bd`) to extract spec-related fields.

## Responsibilities

- Call `<bin> list --json` to get all beads with labels
- Extract spec-related labels (`key:value` strings): `spec_module`, `spec_component`, `spec_impl_section`, `spec_hash`
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

## Bead CLI Interaction

Uses `exec.CommandContext(ctx, bin, "list", "--json")` to get bead data, where `bin` is `"br"` or `"bd"`. The `--json` flag outputs machine-readable JSON with a `labels` array of `key:value` strings. Parse the output and extract spec-related labels (e.g., `spec_module:validator`).

## Error Handling

- If the bead CLI (`br` or `bd`) is not installed or not in PATH, return a clear error
- If no beads have spec metadata, return an empty slice (not an error)
- Wrap all errors with `"impact: read beads: ..."` context
