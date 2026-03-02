# Snapshot Saving Implementation

## Sequence

1. All bead actions (create, close, update) complete successfully
2. Compute fresh merkle tree from current spec directory
3. Write tree to `spec/.snapshot.json`

```go
func SaveSnapshot(ctx context.Context, specDir string) error {
    tree, err := merkle.BuildTree(specDir)
    if err != nil {
        return fmt.Errorf("apply: build tree for snapshot: %w", err)
    }
    snapshotPath := filepath.Join(specDir, ".snapshot.json")
    if err := merkle.Save(tree, snapshotPath); err != nil {
        return fmt.Errorf("apply: save snapshot: %w", err)
    }
    return nil
}
```

## Atomicity

The snapshot is saved only after all bead actions succeed. If any bead action fails, the apply function returns an error and does not save the snapshot. This ensures the failed changes will be retried on the next run.

## Git Integration

The snapshot file is a regular file in the working tree. The user commits it to git alongside the spec changes. This is consistent with the git-native design — no automatic git operations.
