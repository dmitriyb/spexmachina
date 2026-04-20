# SnapshotSaver

Saves the new merkle snapshot after all bead actions are complete.

## Responsibilities

- Compute a fresh merkle tree from the current spec
- Write it to `spec/.snapshot.json`
- This becomes the baseline for the next diff

## Interface

```go
func SaveSnapshot(ctx context.Context, specDir string, createdAt time.Time) error
```

The `createdAt` parameter controls the snapshot timestamp passed to `merkle.Save`, ensuring deterministic output.

## Timing

The snapshot is saved last, after all bead actions succeed (label → create → close). If any bead action fails, the snapshot is not updated — this ensures the next diff will re-detect the same changes and retry the failed actions.

The step-4 proposal-tagging phase no longer exists; SnapshotSaver is the immediate successor of the close-obsoletes phase in the apply flow. The "Save snapshot" requirement's `depends_on` now lists Create-beads and Obsolete-beads directly instead of transiting through the removed "Tag with proposal" requirement.

## Dependency on Merkle Module

Uses the merkle package's `BuildTree` and `Save` functions. The apply module depends on the merkle module for tree computation and snapshot serialization.
