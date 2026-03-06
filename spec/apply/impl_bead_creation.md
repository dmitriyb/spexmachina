# Bead Creation Commands

## Command Construction

```go
func createBead(ctx context.Context, bin string, action Action) (string, error) {
    labels := []string{
        fmt.Sprintf("spec_module:%s", action.Module),
        fmt.Sprintf("spec_hash:%s", action.SpecHash),
    }

    // Add component or impl_section label based on node type
    if action.NodeType == "component" {
        labels = append(labels, fmt.Sprintf("spec_component:%s", action.Node))
    } else if action.NodeType == "impl_section" {
        labels = append(labels, fmt.Sprintf("spec_impl_section:%s", action.Node))
    }

    args := []string{
        "create",
        "--title", fmt.Sprintf("%s: %s", action.Module, action.Node),
        "--type", "task",
        "--labels", strings.Join(labels, ","),
        "--silent",
    }

    out, err := exec.CommandContext(ctx, bin, args...).Output()
    if err != nil {
        return "", fmt.Errorf("apply: create bead for %s/%s: %w", action.Module, action.Node, err)
    }
    return strings.TrimRight(string(out), "\n"), nil
}
```

The `bin` parameter is the bead CLI binary name (`"br"` or `"bd"`), allowing the same logic to work with either tool since they share compatible flags.

## Batch Processing

Create actions are processed sequentially. Parallel creation could cause race conditions in the bead store. Each creation returns the new bead ID, which is accumulated for proposal tagging.
