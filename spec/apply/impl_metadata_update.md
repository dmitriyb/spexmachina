# Metadata Update Commands

## Command Construction

```go
func updateBead(ctx context.Context, action Action) error {
    out, err := exec.CommandContext(ctx, "bd", "update", action.BeadID,
        "--metadata", fmt.Sprintf("spec_hash=%s", action.NewHash),
    ).CombinedOutput()
    if err != nil {
        return fmt.Errorf("apply: update bead %s: %w\n%s", action.BeadID, err, out)
    }
    return nil
}
```

## Scope

Only `spec_hash` is updated programmatically. Other metadata fields (`spec_module`, `spec_component`, `spec_impl_section`) should not change — they reflect the bead's identity, not its content version.

If a spec node is renamed, it appears as a remove + add (close old bead, create new bead), not as an update.
