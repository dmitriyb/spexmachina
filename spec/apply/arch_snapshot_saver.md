# SnapshotSaver

Saves the new merkle snapshot after all bead actions are complete.

## Responsibilities

- Compute a fresh merkle tree from the current spec
- Write it to `spec/.snapshot.json`
- This becomes the baseline for the next diff

## Interface

```go
func SaveSnapshot(ctx context.Context, specDir string) error
```

## Timing

The snapshot is saved last, after all bead actions succeed. If any bead action fails, the snapshot is not updated — this ensures the next diff will re-detect the same changes and retry the failed actions.

## Dependency on Merkle Module

Uses the merkle package's `BuildTree` and `Save` functions. The apply module depends on the merkle module for tree computation and snapshot serialization.
