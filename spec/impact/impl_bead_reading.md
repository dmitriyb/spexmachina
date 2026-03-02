# Bead Metadata Reading Implementation

## bd CLI Interface

```go
func ReadBeads(ctx context.Context) ([]BeadSpec, error) {
    out, err := exec.CommandContext(ctx, "bd", "list", "--json").Output()
    if err != nil {
        return nil, fmt.Errorf("impact: read beads: %w", err)
    }
    // Parse JSON output, extract spec metadata
}
```

## JSON Parsing

The `bd list --json` output is a JSON array of bead objects. Each bead may have a `metadata` object with spec-related fields. Parse using `encoding/json` into a generic structure, then extract the relevant fields.

## Metadata Fields

Expected metadata keys set by `spex apply`:
- `spec_module`: module name (e.g., "validator")
- `spec_component`: component name (e.g., "SchemaChecker")
- `spec_impl_section`: impl_section name (e.g., "Schema validation implementation")
- `spec_hash`: hash of the spec node at the time the bead was created

Beads without these metadata fields are ignored — they are not spec-managed beads.
