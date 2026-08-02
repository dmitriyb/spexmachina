# DAGChecker

Verifies that dependency graphs within the spec are acyclic — the property [[d0451520f7be|DAG acyclicity]] states.

## Responsibilities

- Build directed graphs from module dependencies (`requires_module`), requirement dependencies (`depends_on`), and component dependencies (`uses`)
- Detect cycles in each graph
- Report cycles with the full cycle path for debugging

## Interface

Given the path to a spec directory, the checker loads `project.json` and the `module.json` files it names, and returns a flat list of validation entries — empty when the spec is acyclic. If the spec cannot be loaded, the load failures are returned under this checker's own name and no graph is built. Modules are visited in sorted name order and, within a graph, the walk is started from each node in sorted identity-hash order — a node reached first through an edge is walked where that edge led, in the order its predecessor declares its neighbours — so the same spec produces the same entries in the same sequence.

## Graphs Checked

1. **Module dependency graph**: nodes are modules, edges are `requires_module` references
2. **Requirement dependency graph** (per module): nodes are requirements, edges are `depends_on` references
3. **Component dependency graph** (per module): nodes are components, edges are `uses` references

Each graph is built and walked on its own, and one cycle produces one entry. The entry's message names which of the three it came from — module, requirement or component dependency — and spells the cycle out as the chain of labels it runs through: module names, requirement titles, component names. Two cycles in different graphs therefore arrive as two report lines.

## Cycle Detection

A depth-first walk marks each node with one of three states: unvisited, on the current search stack, fully explored. Reaching a node that is still on the current stack is a cycle. The walk then retraces the stack from that node back to itself to recover the full path, and that path is what the entry's message carries.

## Dependencies

The graph structure must parse before it can be walked, and that is [[651d5315eebf|SchemaChecker]]'s subject, so the command runs the schema check first. It is an ordering, not a gate: this checker runs whatever the schema check found, and a `module.json` that will not parse is reported here as well, under this checker's own name, rather than passed over.
