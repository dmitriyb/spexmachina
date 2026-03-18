# Bead Obsolescence Commands

## Command Construction

```go
func (c *execCLI) Close(ctx context.Context, id string, labels []string) error {
    args := []string{"close", id}
    for _, label := range labels {
        args = append(args, "--add-label", label)
    }
    out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
    if err != nil {
        return fmt.Errorf("apply: %s close %s: %w\n%s", c.bin, id, err, out)
    }
    return nil
}
```

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool.

## Label Assignment

Every obsoleted bead receives exactly two labels:
- `spex:obsolete` — lifecycle marker identifying the bead as superseded
- `commit:<HEAD>` — the git HEAD at the moment of obsolescence, stamping the last commit where the bead's spec was valid

```go
func obsoleteBead(ctx context.Context, cli BeadCLI, store map.Store, action Action) error {
    head, err := gitHEAD()
    if err != nil {
        return fmt.Errorf("apply: resolve HEAD: %w", err)
    }
    labels := []string{"spex:obsolete", fmt.Sprintf("commit:%s", head)}
    if err := cli.Close(ctx, action.BeadID, labels); err != nil {
        return err
    }

    // Only delete mapping record for removed nodes
    if action.ChangeType == "removed" {
        record, err := store.GetByBead(action.BeadID)
        if err != nil {
            return nil // Log warning — orphaned record will be cleaned up later
        }
        return store.Delete(record.ID)
    }
    return nil
}
```

## Batch Processing

Obsolete actions are processed sequentially. Each failure is logged as a warning and accumulated. The batch continues even if individual closes fail.

## Error Tolerance

Any non-zero exit code from the close command is treated as a warning, not a fatal error. This covers already-closed beads, missing bead IDs, and other transient issues.

`CloseBeads` returns a summary error aggregating all warnings, or nil if all succeeded.
