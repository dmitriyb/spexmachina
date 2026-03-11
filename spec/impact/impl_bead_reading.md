# Bead Metadata Reading Implementation

## Bead CLI Interface

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool.

```go
func ReadBeads(ctx context.Context, bin string) ([]BeadSpec, error) {
    out, err := exec.CommandContext(ctx, bin, "list", "--json").Output()
    if err != nil {
        return nil, fmt.Errorf("impact: read beads: %w", err)
    }
    // Parse JSON output, extract spex label
}
```

## JSON Parsing

The `<bin> list --json` output is a JSON array of bead objects. Each bead has a `labels` array containing `key:value` strings. Parse using `encoding/json` into a typed structure, then find the `spex:` label.

## Label Extraction

Each spec-managed bead has a single label with the `spex:` prefix:

```go
func extractRecordID(labels []string) (int, bool) {
    for _, label := range labels {
        if strings.HasPrefix(label, "spex:") {
            id, err := strconv.Atoi(strings.TrimPrefix(label, "spex:"))
            if err == nil {
                return id, true
            }
        }
    }
    return 0, false
}
```

Beads without a `spex:` label are ignored — they are not spec-managed beads.

This replaces the previous multi-label approach that required parsing `spec_module`, `spec_component`, `spec_impl_section`, and `spec_hash` labels separately.
