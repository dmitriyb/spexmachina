# Bead Closure Commands

## Command Construction

```go
func (c *execCLI) Close(ctx context.Context, id string, reason string) error {
    args := []string{"close", id, "--reason", reason}
    out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
    if err != nil {
        return fmt.Errorf("apply: %s close %s: %w\n%s", c.bin, id, err, out)
    }
    return nil
}
```

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool.

## Mapping Record Removal

After closing the bead, remove the corresponding mapping record:

```go
func closeBead(ctx context.Context, cli BeadCLI, store map.Store, action Action) error {
    if err := cli.Close(ctx, action.BeadID, reason); err != nil {
        return err
    }
    record, err := store.GetByBead(action.BeadID)
    if err != nil {
        // Log warning — orphaned record will be cleaned up later
        return nil
    }
    return store.Delete(record.ID)
}
```

## Batch Processing

Close actions are processed sequentially. Each failure is logged as a warning and accumulated. The batch continues even if individual closes fail.

## Error Tolerance

Any non-zero exit code from the close command is treated as a warning, not a fatal error. This covers already-closed beads, missing bead IDs, and other transient issues. The warning is logged with the bead ID and command output for debuggability.

If the bead close succeeds but the mapping record deletion fails, the orphaned record is logged as a warning and will be cleaned up on the next `spex apply` run.
