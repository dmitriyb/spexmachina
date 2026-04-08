# Diff Algorithm

## Approach

Map-based comparison of current tree nodes against snapshot nodes, keyed by identity hash.

## Algorithm

```
current_map  = flatten(current_tree)   // identity_hash → {content_hash, node_type, module_hash}
snapshot_map = flatten(snapshot_tree)  // identity_hash → {content_hash, node_type, module_hash}

added    = keys(current_map) - keys(snapshot_map)
removed  = keys(snapshot_map) - keys(current_map)
modified = {k | k ∈ both ∧ current_map[k].content_hash ≠ snapshot_map[k].content_hash}
```

Each Change carries the identity hash key, `node_type`, and parent module identity hash from the node metadata.

## Leaf-Only Reporting

Only leaf changes are reported in the diff output. Interior node hash changes are implicit — if a leaf changed, its module interior node hash has also changed. The ImpactClassifier uses the `node_type` metadata to determine which level was affected.

## First Diff (No Snapshot)

When no snapshot exists (first run), every node is reported as "added". This is the baseline for future diffs.

## Determinism

Changes are sorted by key (lexicographic). Given the same current tree and snapshot, the diff output is always identical. No timestamps, no random ordering. Identity hashes sort consistently because they are pure lowercase hex.

## Rename Stability

Because keys are identity hashes (e.g., `a1b2c3d4e5f6`) — and the identity hash is computed from module name + node type + node name, not from filesystem path — renaming a module directory from `impact/` to `impact_analysis/` does not affect the diff. The `module/<name>` identity string only changes if the module's `name` field in `module.json` changes, which is treated as a real rename, not a filesystem move. Pure file moves produce no diff entries.

A node rename (changing the `name` field of a component, for instance) *is* a content-affecting change: the identity hash changes, so the old identity hash appears as `removed` and a new identity hash appears as `added`. This is the documented "rename is destructive" behavior — a rename in the spec graph is equivalent to delete + create at the bead layer.
