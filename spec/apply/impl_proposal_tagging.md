# Proposal Tagging Implementation

## Command Construction

```go
func tagBead(ctx context.Context, bin, beadID, proposalRef string) error {
    out, err := exec.CommandContext(ctx, bin, "update", beadID,
        "--add-label", fmt.Sprintf("spec_proposal:%s", proposalRef),
    ).CombinedOutput()
    if err != nil {
        return fmt.Errorf("apply: tag bead %s with proposal: %w\n%s", beadID, err, out)
    }
    return nil
}
```

## Batch Processing

All affected bead IDs (from creates, closes, and updates) are collected and tagged in a single pass. The proposal reference is the same for all beads in one apply run.

## Proposal Reference Format

The `.md` extension is stripped before storing as a label since `br` labels do not allow dots. The stored reference is the stem only, e.g., `2026-02-23-spex-machina`. When reading back, append `.md` and resolve relative to `spec/proposals/`. If the file does not exist, warn the user.
