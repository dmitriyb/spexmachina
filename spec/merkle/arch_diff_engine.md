# DiffEngine

Compares two ID-keyed hash trees (current vs snapshot). Same ID + different hash = modified. ID in current but not snapshot = added. ID in snapshot but not current = removed.

## Responsibilities

- Compare current ID-keyed tree against a stored snapshot
- Identify added nodes (ID in current but not in snapshot)
- Identify removed nodes (ID in snapshot but not in current)
- Identify modified nodes (same identity hash, different content hash)
- Report changes with identity hashes and node metadata

## Interface

```go
type Change struct {
    Key      string // identity hash of the spec node, e.g. "a1b2c3d4e5f6";
                    //   or "meta/project" / "meta/<module-hash>" for envelope leaves
    Type     string // "added", "removed", "modified"
    NodeType string // "component", "impl_section", "data_flow", "test_section", "meta", "requirement", "module"
    Module   string // identity hash of the parent module ("" for project-level nodes)
    OldHash  string // empty for "added"
    NewHash  string // empty for "removed"
}

func Diff(current, snapshot *Node) []Change
```

`NodeType` is carried as a separate field because identity hashes do not embed the node type — there is no way to look at `a1b2c3d4e5f6` and tell whether it points at a component or an impl_section. The diff propagates `NodeType` from the merkle tree leaf, where it was set during tree construction.

## Algorithm

1. Flatten both trees into identity-hash → content-hash maps
2. For each identity hash in current but not in snapshot: added
3. For each identity hash in snapshot but not in current: removed
4. For each identity hash in both with different content hashes: modified
5. Sort changes by key for deterministic output

## Rename Stability

Because nodes are keyed by spec ID rather than file path, renaming a module directory or content file does not produce a remove + add. As long as the spec IDs remain the same, the diff correctly identifies the change as a modification.

## Bootstrap behavior

`DiffEngine` is invoked by `spex diff` against two trees: the tree
TreeBuilder just built from the current spec, and the tree
`SnapshotStore.Load` returned. On a fresh project where
`spec/.snapshot.json` does not exist, `Load` returns the empty tree (per
its missing-file contract). DiffEngine compares the populated current tree
against the empty baseline and reports every leaf as `added`. That is the
diff input the rest of the pipeline (impact → emit → adapter → ingest)
consumes to produce the first bead-map and the first snapshot. There is no
"prime the snapshot" step beforehand — bootstrap and steady-state share
the same DiffEngine call.
