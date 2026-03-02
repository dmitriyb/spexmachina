# DiffEngine

Compares two merkle trees and identifies all changed nodes.

## Responsibilities

- Compare current tree against a stored snapshot tree
- Identify added nodes (in current but not in snapshot)
- Identify removed nodes (in snapshot but not in current)
- Identify modified nodes (same path, different hash)
- Report changed paths from leaf to root

## Interface

```go
type Change struct {
    Path   string // e.g., "validator/arch/arch_schema_checker.md"
    Type   string // "added", "removed", "modified"
    OldHash string // empty for "added"
    NewHash string // empty for "removed"
}

func Diff(current, snapshot *Node) []Change
```

## Algorithm

1. Flatten both trees into path → hash maps
2. For each path in current but not in snapshot: added
3. For each path in snapshot but not in current: removed
4. For each path in both with different hashes: modified
5. Sort changes by path for deterministic output

## Changed Path Propagation

When a leaf changes, all ancestor interior nodes also have changed hashes. The diff reports leaf-level changes. The ImpactClassifier uses the ancestor path to determine the impact level.
