# Bead Creation Commands

## Command Construction

```go
func createBead(ctx context.Context, action Action) (string, error) {
    args := []string{
        "create",
        "--title", fmt.Sprintf("%s: %s", action.Module, action.Node),
        "--type", "task",
        "--metadata", fmt.Sprintf("spec_module=%s", action.Module),
        "--metadata", fmt.Sprintf("spec_hash=%s", action.SpecHash),
    }

    // Add component or impl_section metadata based on node type
    if action.NodeType == "component" {
        args = append(args, "--metadata", fmt.Sprintf("spec_component=%s", action.Node))
    } else if action.NodeType == "impl_section" {
        args = append(args, "--metadata", fmt.Sprintf("spec_impl_section=%s", action.Node))
    }

    out, err := exec.CommandContext(ctx, "bd", args...).Output()
    if err != nil {
        return "", fmt.Errorf("apply: create bead for %s/%s: %w", action.Module, action.Node, err)
    }
    return strings.TrimRight(string(out), "\n"), nil
}
```

## Batch Processing

Create actions are processed sequentially. Parallel creation could cause race conditions in bd's storage. Each creation returns the new bead ID, which is accumulated for proposal tagging.
