# Bead Metadata Reading Implementation

## Bead CLI Interface

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool.

```go
func ReadBeads(ctx context.Context, bin string) ([]BeadSpec, error) {
    out, err := exec.CommandContext(ctx, bin, "list", "--json").Output()
    if err != nil {
        return nil, fmt.Errorf("impact: read beads: %w", err)
    }
    // Parse JSON output, extract spec metadata
}
```

## JSON Parsing

The `<bin> list --json` output is a JSON array of bead objects. Each bead has a `labels` array containing `key:value` strings. Parse using `encoding/json` into a typed structure, then extract spec-related labels by splitting on the first `:`.

## Spec Labels

Expected label keys set by `spex apply` (stored as `key:value` strings in the `labels` array):
- `spec_module`: module name (e.g., "validator")
- `spec_component`: component name (e.g., "SchemaChecker")
- `spec_impl_section`: impl_section name (e.g., "Schema validation implementation")
- `spec_hash`: hash of the spec node at the time the bead was created

Beads without these labels are ignored — they are not spec-managed beads.
