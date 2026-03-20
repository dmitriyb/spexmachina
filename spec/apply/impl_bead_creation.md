# Bead Creation Commands

## Type Assignment

```go
func beadType(nodeType string) string {
    switch nodeType {
    case "module":       return "epic"
    case "component":    return "feature"
    case "test_section": return "task"
    default:             return "" // impl_section, data_flow do not get beads
    }
}
```

## Command Construction

```go
func createBead(ctx context.Context, bin string, store map.Store, action Action) (string, error) {
    bt := beadType(action.NodeType)
    if bt == "" {
        return "", fmt.Errorf("apply: node type %q does not get a bead", action.NodeType)
    }

    args := []string{
        "create",
        "--title", fmt.Sprintf("%s: %s", action.Module, action.Node),
        "--type", bt,
        "--silent",
    }

    // Parent hierarchy
    if parentID := resolveParent(store, action); parentID != "" {
        args = append(args, "--parent", parentID)
    }

    // Lineage via deps
    if action.OldBeadID != "" {
        args = append(args, "--deps", fmt.Sprintf("blocks:%s", action.OldBeadID))
    }

    // Priority propagation
    if action.Priority >= 0 {
        args = append(args, "--priority", strconv.Itoa(action.Priority))
    }

    out, err := exec.CommandContext(ctx, bin, args...).Output()
    if err != nil {
        return "", fmt.Errorf("apply: create bead for %s/%s: %w", action.Module, action.Node, err)
    }
    beadID := strings.TrimRight(string(out), "\n")

    // Create or update mapping record
    record := map.Record{
        SpecNodeID:  action.SpecNodeID,
        BeadID:      beadID,
        BeadType:    bt,
        Module:      action.Module,
        Component:   action.Node,
        ContentFile: action.ContentFile,
        SpecHash:    action.SpecHash,
    }
    recordID, err := store.CreateOrUpdate(record)
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

## Cleanup Bead Creation

```go
func createCleanupBead(ctx context.Context, bin string, action Action) (string, error) {
    args := []string{
        "create",
        "--title", fmt.Sprintf("Code cleanup: %s", action.Node),
        "--type", "task",
        "--silent",
    }

    if action.OldBeadID != "" {
        args = append(args, "--deps", fmt.Sprintf("blocks:%s", action.OldBeadID))
    }

    out, err := exec.CommandContext(ctx, bin, args...).Output()
    if err != nil {
        return "", fmt.Errorf("apply: create cleanup bead for %s/%s: %w", action.Module, action.Node, err)
    }
    beadID := strings.TrimRight(string(out), "\n")

    // Label with spex:cleanup (no mapping record — component no longer in spec)
    labelArgs := []string{"update", beadID, "--add-label", "spex:cleanup"}
    if _, err := exec.CommandContext(ctx, bin, labelArgs...).Output(); err != nil {
        return "", fmt.Errorf("apply: set cleanup label on %s: %w", beadID, err)
    }

    return beadID, nil
}
```

## Parent Resolution

```go
func resolveParent(store map.Store, action Action) string {
    switch action.NodeType {
    case "component":
        // Parent is the module's epic bead
        rec, err := store.GetBySpecNode(action.Module + "/module")
        if err == nil { return rec.BeadID }
    case "test_section":
        // Parent is the component's feature bead
        // Component ID is resolved from the test_section's describes array
        rec, err := store.GetBySpecNode(action.ParentSpecNodeID)
        if err == nil { return rec.BeadID }
    }
    return ""
}
```

## Batch Processing

Create actions are processed sequentially in hierarchy order: epics first, then features, then tasks. This ensures parent bead IDs are available in the mapping file when child beads are created.
