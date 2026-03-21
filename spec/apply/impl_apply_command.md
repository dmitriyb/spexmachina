# Apply command implementation

## Structure

`cmd/spex/apply.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, read impact report from stdin or file
2. For each obsolete action: `BeadCloser.Label(action)` — add `spex:obsolete` + `commit:<HEAD>` labels, delete mapping records for removed nodes. Beads stay open.
3. For each create action (in hierarchy order: epics → features → tasks): `BeadCreator.Create(action)` with type, parent, deps, priority. Old beads are still open, so `--deps blocks:<old-bead-id>` references valid beads.
4. For each obsolete action: `BeadCloser.Close(action)` — close the labeled beads. Replacements already exist.
5. Call `ProposalTagger.Tag(allAffected, proposalRef)`
6. Call `SnapshotSaver.Save(currentTree)`
7. In dry-run mode, print actions without executing

## Creation Ordering

Creates are sorted by spec node type before execution:
1. Module beads (epics) — no parent dependency
2. Component beads (features) — parent is the module's epic (must exist)
3. Test_section beads (tasks) — parent is the component's feature (must exist)

This ensures `--parent` references are resolvable from the mapping file.

## Topological Sort Within Type Levels

Within each type level (e.g., all features), actions are topologically sorted by `DepBeadIDs` to ensure dependency beads are created before their dependents. This handles the case where both bead A and bead B are features being created in the same run, and A depends on B.

```go
func topoSortActions(actions []Action) ([]Action, error) {
    // Build adjacency: action index -> indices it must come after
    // Only consider DepBeadIDs that reference other actions in this batch
    // (identified by matching spec node IDs to action entries)

    inDegree := make([]int, len(actions))
    adj := make([][]int, len(actions))
    // ... build graph from DepBeadIDs cross-references ...

    // Kahn's algorithm
    var queue []int
    for i, d := range inDegree {
        if d == 0 {
            queue = append(queue, i)
        }
    }

    var sorted []Action
    for len(queue) > 0 {
        idx := queue[0]
        queue = queue[1:]
        sorted = append(sorted, actions[idx])
        for _, next := range adj[idx] {
            inDegree[next]--
            if inDegree[next] == 0 {
                queue = append(queue, next)
            }
        }
    }

    if len(sorted) != len(actions) {
        return nil, fmt.Errorf("apply: circular dependency among %d actions", len(actions)-len(sorted))
    }
    return sorted, nil
}
```

When no dependency edges exist between actions in the same batch, the topological sort is stable and preserves the original input order.
