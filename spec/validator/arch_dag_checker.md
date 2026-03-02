# DAGChecker

Verifies that dependency graphs within the spec are acyclic.

## Responsibilities

- Build directed graphs from module dependencies (`requires_module`), requirement dependencies (`depends_on`), and component dependencies (`uses`)
- Detect cycles in each graph
- Report cycles with the full cycle path for debugging

## Interface

```go
func CheckDAG(project *schema.Project, modules map[string]*schema.Module) []ValidationError
```

## Graphs Checked

1. **Module dependency graph**: nodes are modules, edges are `requires_module` references
2. **Requirement dependency graph** (per module): nodes are requirements, edges are `depends_on` references
3. **Component dependency graph** (per module): nodes are components, edges are `uses` references

## Cycle Detection

Uses depth-first search with a visited/in-stack coloring scheme. When a back edge is found (node already in the current DFS stack), a cycle is reported.

## Dependencies

Depends on SchemaChecker (component 1) — DAG checks only run after schema validation passes, since the graph structure must be parseable first.
