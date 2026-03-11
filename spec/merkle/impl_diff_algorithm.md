# Diff Algorithm

## Approach

Map-based comparison of current tree nodes against snapshot nodes, keyed by spec ID.

## Algorithm

```
current_map  = flatten(current_tree)   // spec_id → {hash, node_type, module}
snapshot_map = flatten(snapshot_tree)   // spec_id → {hash, node_type, module}

added    = keys(current_map) - keys(snapshot_map)
removed  = keys(snapshot_map) - keys(current_map)
modified = {k | k ∈ both ∧ current_map[k].hash ≠ snapshot_map[k].hash}
```

Each Change carries the spec ID key, node_type, and module from the node metadata.

## Leaf-Only Reporting

Only leaf changes are reported in the diff output. Interior node hash changes are implicit — if a leaf changed, its module interior node hash has also changed. The ImpactClassifier uses the node_type metadata to determine which level was affected.

## First Diff (No Snapshot)

When no snapshot exists (first run), every node is reported as "added". This is the baseline for future diffs.

## Determinism

Changes are sorted by key (lexicographic). Given the same current tree and snapshot, the diff output is always identical. No timestamps, no random ordering.

## Rename Stability

Because keys are spec IDs (e.g., `module/3/component/2`), renaming a module directory from `impact/` to `impact_analysis/` does not affect the diff — the spec ID remains `module/4/component/1` regardless of filesystem path. Only content changes produce diff entries.
