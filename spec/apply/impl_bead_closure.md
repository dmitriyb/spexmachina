# Bead Closure Commands

## Command Construction

```go
func closeBead(ctx context.Context, action Action) error {
    reason := fmt.Sprintf("Spec node removed: %s/%s", action.Module, action.Node)
    out, err := exec.CommandContext(ctx, "bd", "close", action.BeadID, "--reason", reason).CombinedOutput()
    if err != nil {
        return fmt.Errorf("apply: close bead %s: %w\n%s", action.BeadID, err, out)
    }
    return nil
}
```

## Error Tolerance

If bd returns an error indicating the bead is already closed, treat it as success (idempotency). Only fail on unexpected errors (bd not found, storage corruption, etc.).
