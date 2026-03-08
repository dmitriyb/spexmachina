# Metadata Update Commands

## Command Construction

```go
func (c *execCLI) Update(ctx context.Context, id string, metadata map[string]string) error {
    for k, v := range metadata {
        args := []string{"update", id, "--add-label", fmt.Sprintf("%s:%s", k, v)}
        out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
        if err != nil {
            return fmt.Errorf("apply: %s update %s: %w\n%s", c.bin, id, err, out)
        }
    }
    return nil
}
```

## Scope

Only `spec_hash` is updated programmatically. Other metadata fields (`spec_module`, `spec_component`, `spec_impl_section`) should not change — they reflect the bead's identity, not its content version.

If a spec node is renamed, it appears as a remove + add (close old bead, create new bead), not as an update.
