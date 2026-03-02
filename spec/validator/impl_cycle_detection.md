# Cycle Detection Implementation

## Algorithm: DFS with Three-Color Marking

Use depth-first search with node coloring to detect cycles in directed graphs.

```
WHITE = unvisited
GRAY  = in current DFS stack (being explored)
BLACK = fully explored (all descendants visited)
```

### Procedure

1. Build adjacency list from the edges (e.g., `requires_module` → module ID → module ID)
2. Initialize all nodes as WHITE
3. For each WHITE node, start DFS:
   - Mark node GRAY
   - For each neighbor:
     - If WHITE: recurse
     - If GRAY: cycle detected — record the cycle path
     - If BLACK: skip (already fully explored)
   - Mark node BLACK

### Cycle Path Reconstruction

When a GRAY node is encountered, walk the DFS stack from the encountered node back to itself to reconstruct the full cycle path. Include this path in the error message for debugging.

## Graphs

Three separate graphs are checked:
1. Module dependency graph (project-wide)
2. Requirement dependency graph (per module)
3. Component `uses` graph (per module)

Each graph is built and checked independently. Cycles in different graphs produce separate errors.

## Complexity

O(V + E) per graph, where V is the number of nodes and E is the number of edges. For 100 modules with 5 dependencies each, this is negligible.
