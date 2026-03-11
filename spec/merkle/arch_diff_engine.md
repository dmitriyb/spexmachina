# DiffEngine

Compares two ID-keyed hash trees (current vs snapshot). Same ID + different hash = modified. ID in current but not snapshot = added. ID in snapshot but not current = removed.

## Responsibilities

- Compare current ID-keyed tree against a stored snapshot
- Identify added nodes (ID in current but not in snapshot)
- Identify removed nodes (ID in snapshot but not in current)
- Identify modified nodes (same ID, different hash)
- Report changes with spec IDs and node metadata

## Interface

```go
type Change struct {
    Key      string // spec ID, e.g. "module/3/component/2"
    Type     string // "added", "removed", "modified"
    NodeType string // "component", "impl_section", "data_flow", "test_section", "meta"
    Module   int    // module ID
    OldHash  string // empty for "added"
    NewHash  string // empty for "removed"
}

func Diff(current, snapshot *Node) []Change
```

## Algorithm

1. Flatten both trees into ID → hash maps
2. For each ID in current but not in snapshot: added
3. For each ID in snapshot but not in current: removed
4. For each ID in both with different hashes: modified
5. Sort changes by key for deterministic output

## Rename Stability

Because nodes are keyed by spec ID rather than file path, renaming a module directory or content file does not produce a remove + add. As long as the spec IDs remain the same, the diff correctly identifies the change as a modification.
