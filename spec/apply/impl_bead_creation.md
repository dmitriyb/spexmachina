# Bead Creation Commands

## Command Construction

```go
func createBead(ctx context.Context, bin string, store map.Store, action Action) (string, error) {
    args := []string{
        "create",
        "--title", fmt.Sprintf("%s: %s", action.Module, action.Node),
        "--type", "task",
        "--silent",
    }

    out, err := exec.CommandContext(ctx, bin, args...).Output()
    if err != nil {
        return "", fmt.Errorf("apply: create bead for %s/%s: %w", action.Module, action.Node, err)
    }
    beadID := strings.TrimRight(string(out), "\n")

    // Create mapping record
    recordID, err := store.Create(map.Record{
        SpecNodeID:  action.SpecNodeID,
        BeadID:      beadID,
        Module:      action.Module,
        Component:   action.Node,
        ContentFile: action.ContentFile,
        SpecHash:    action.SpecHash,
    })
    if err != nil {
        return "", fmt.Errorf("apply: create mapping for %s: %w", beadID, err)
    }

    // Set the bead label to the mapping record ID
    labelArgs := []string{"update", beadID, "--add-label", fmt.Sprintf("spex:%d", recordID)}
    if _, err := exec.CommandContext(ctx, bin, labelArgs...).Output(); err != nil {
        return "", fmt.Errorf("apply: set label on %s: %w", beadID, err)
    }

    return beadID, nil
}
```

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool since they share compatible flags.

## Batch Processing

Create actions are processed sequentially. Parallel creation could cause race conditions in the bead store and the mapping file. Each creation returns the new bead ID, which is accumulated for proposal tagging.
