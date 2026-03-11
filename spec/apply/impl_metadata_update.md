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

## Mapping Record Update

After updating the bead's `spec_hash` label, update the mapping record:

```go
func updateBead(ctx context.Context, cli BeadCLI, store map.Store, action Action) error {
    if err := cli.Update(ctx, action.BeadID, map[string]string{"spec_hash": action.SpecHash}); err != nil {
        return err
    }
    record, err := store.GetByBead(action.BeadID)
    if err != nil {
        return fmt.Errorf("apply: mapping record not found for bead %s: %w", action.BeadID, err)
    }
    return store.Update(record.ID, map[string]string{"spec_hash": action.SpecHash})
}
```

## Scope

Only `spec_hash` is updated programmatically — both on the bead label and in the mapping record. The `spex:<record-id>` label is immutable and must never change.

If a spec node is renamed, it appears as a remove + add (close old bead + mapping, create new bead + mapping), not as an update.
