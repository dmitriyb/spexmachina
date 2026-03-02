# Diff Algorithm

## Approach

Map-based comparison of current tree nodes against snapshot nodes.

## Algorithm

```
current_map  = flatten(current_tree)   // path → hash
snapshot_map = flatten(snapshot_tree)   // path → hash

added    = keys(current_map) - keys(snapshot_map)
removed  = keys(snapshot_map) - keys(current_map)
modified = {k | k ∈ both ∧ current_map[k] ≠ snapshot_map[k]}
```

## Leaf-Only Reporting

Only leaf changes are reported in the diff output. Interior node hash changes are implicit — if a leaf changed, all ancestors up to root have changed. The ImpactClassifier uses the leaf path to determine which level was affected.

## First Diff (No Snapshot)

When no snapshot exists (first run), every node is reported as "added". This is the baseline for future diffs.

## Determinism

Changes are sorted by path (lexicographic). Given the same current tree and snapshot, the diff output is always identical. No timestamps, no random ordering.
