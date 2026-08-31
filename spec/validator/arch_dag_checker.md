# DAGChecker

Verifies that dependency graphs within the spec are acyclic — the property [[d0451520f7be|DAG acyclicity]] states.

## Responsibilities

- Build directed graphs from module dependencies (`requires_module`), requirement dependencies (`depends_on`), and component dependencies (`uses`)
- Detect cycles in each graph
- Report cycles with the full cycle path for debugging

## Interface

Given the path to a spec directory, the checker loads `project.json` and the `module.json` files it names, and returns a flat list of validation entries — empty when the spec is acyclic. If the spec cannot be loaded, the load failures are returned under this checker's own name and no graph is built. Modules are visited in sorted name order and, within a graph, the walk is started from each node in sorted identity-hash order — a node reached first through an edge is walked where that edge led, in the order its predecessor declares its neighbours — so the same spec produces the same entries in the same sequence.

## Graphs Checked

The graphs are the edge kinds the resolved profile declares, minus those marked `cyclic: true` — that optional flag on an edge declaration marks a descriptive edge exempt from the cycle check, and an omitted flag means cycle-checked. One graph is built per non-exempt edge kind and source occurrence; the checker holds no fixed edge list of its own. The default profile omits the flag on all seven of its edge kinds, so every one is checked — four of the seven vacuously: `implements`, `provided_by` and `describes` because an edge kind whose source and target types differ cannot close a loop, and `preq_id` because its source and target are both requirements but the target scope's requirements carry no such edge, so no loop can close there either — while `requires_module`, `depends_on` and `uses` can. `depends_on` is walked at both scopes its `requirement` source is declared at, so the default yields four graphs that can actually cycle:

1. **Module dependency graph**: nodes are modules, edges are `requires_module` references
2. **Requirement dependency graph** (per module): nodes are a module's requirements, edges are `depends_on` references
3. **Component dependency graph** (per module): nodes are components, edges are `uses` references
4. **Project requirement dependency graph** (project-wide): nodes are `project.json`'s own requirements, edges are the same `depends_on` kind at project scope — walked by the generic profile-edge machinery, not a dedicated built-in graph

A profile-declared edge is built and walked by the same machinery. Each graph is built and walked on its own, and one cycle produces one entry. The entry's message names which graph it came from — the three built-in graphs name it in prose (module, requirement or component dependency), and a graph the generic machinery walks names the edge kind (`<edge kind> cycle: …`), located by the entry's path — and spells the cycle out as the chain of labels it runs through: module names, requirement titles, component names. Two cycles in different graphs therefore arrive as two report lines.

## Cycle Detection

A depth-first walk marks each node with one of three states: unvisited, on the current search stack, fully explored. Reaching a node that is still on the current stack is a cycle. The walk then retraces the stack from that node back to itself to recover the full path, and that path is what the entry's message carries.

## Dependencies

The graph structure must parse before it can be walked, and that is [[651d5315eebf|SchemaChecker]]'s subject, so the command runs the schema check first. It is an ordering, not a gate: this checker runs whatever the schema check found, and a `module.json` that will not parse is reported here as well, under this checker's own name, rather than passed over.
