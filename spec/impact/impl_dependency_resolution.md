# Dependency Resolution Algorithm

Resolves spec-graph dependencies for create actions by walking `uses` (intra-module) and `requires_module` (inter-module) edges, then looking up current bead IDs from the mapping file. The resolved IDs are attached to the Action as `DepBeadIDs` for downstream propagation.

## Algorithm

For each create action:

1. **Identify the spec node's module** from the action's module field
2. **Resolve component `uses` edges** (direct, not transitive):
   - Read the component's `uses` array from the spec graph
   - For each referenced component ID, look up the current bead from the mapping file
   - If the bead is open, add its ID to `DepBeadIDs`
   - If the bead is closed, skip it (dependency already satisfied)
3. **Resolve `requires_module` edges** (transitive, with cycle detection):
   - Read the module's `requires_module` array from the spec graph
   - For each required module, collect all component beads from the mapping file
   - Add open component bead IDs to `DepBeadIDs`
   - Recurse: if the required module itself has `requires_module`, resolve those too
   - Track visited modules to detect and break cycles
4. **Attach `DepBeadIDs`** to the action

## Transitivity Rules

These rules match PreflightChecker's existing semantics:

- **`requires_module`**: Transitive. If A requires B and B requires C, A depends on open beads in both B and C.
- **`uses`**: NOT transitive. If X uses Y and Y uses Z, X depends on Y only.

## Cycle Detection

The `requires_module` traversal uses a visited-set:

```go
func resolveModuleDeps(specGraph *SpecGraph, store map.Store, moduleID int, visited map[int]bool) []string {
    if visited[moduleID] {
        return nil // cycle detected, break
    }
    visited[moduleID] = true

    var depIDs []string
    mod := specGraph.Module(moduleID)

    // Collect open component beads in this module
    for _, comp := range mod.Components {
        rec, err := store.GetBySpecNode(comp.SpecNodeID)
        if err != nil {
            continue // no mapping record
        }
        if rec.Status != "closed" {
            depIDs = append(depIDs, rec.BeadID)
        }
    }

    // Recurse into transitive dependencies
    for _, reqID := range mod.RequiresModule {
        depIDs = append(depIDs, resolveModuleDeps(specGraph, store, reqID, visited)...)
    }

    return depIDs
}
```

## Closed Bead Filtering

A bead is considered "closed" when its status in the mapping file (or bead store) indicates the work is complete. Closed beads represent satisfied dependencies — no edge is needed.

The status check reads from the mapping file first (cheaper), falling back to `BeadReader` for live status if the mapping file entry lacks status information.

## Integration with ActionClassifier

The dependency resolution runs as a post-processing step after action classification. The classifier produces actions with the standard fields, then the resolver enriches create actions with `DepBeadIDs`:

```go
actions := ClassifyActions(matches, unmatched, orphaned)
for i := range actions {
    if actions[i].Type == "create" {
        actions[i].DepBeadIDs = resolveDeps(specGraph, store, actions[i])
    }
}
```

This keeps the state transition table clean and separates the two concerns.

## Limitations

- Dependencies can only be resolved against existing mapping file entries. If two beads are being created in the same apply run and one depends on the other, the impact module cannot resolve this — it has no bead ID yet. The actual ordering is handled by ApplyCommand's topological sort.
- The spec graph must be loaded for dependency resolution. The ImpactCommand wires this by passing the parsed spec to the ActionClassifier.
