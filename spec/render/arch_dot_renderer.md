# DOTRenderer

Generates a graphviz DOT graph from the spec.

## Responsibilities

- Map spec nodes to DOT nodes with labels and shapes
- Map spec edges to DOT edges with labels
- Produce valid DOT syntax for rendering with `dot`, `neato`, or other graphviz tools
- Lay the graph out left to right (`rankdir=LR`), which reads more naturally for a dependency graph than top to bottom

## Interface

The renderer is handed the graph [[7d1150c19724|SpecReader]] parsed and the stream to write to; `spex render --format dot` supplies stdout. It emits one `digraph` and nothing else — no wrapper, no trailing commentary — so the output is a complete graphviz document that pipes straight into `dot`.

## Node Mapping

[[a596d8caefb1|Render DOT]] fixes how each of these declarations is drawn:

| Spec Node | DOT Shape | Fill |
|-----------|-----------|------|
| Project requirement | box | lightblue |
| Module | folder | lightgray |
| Module requirement | box | lightgreen |
| Component | component | lightyellow |
| Data flow | ellipse | plum1 |
| Api | cds | paleturquoise |

Section nodes are drawn too and are covered by § Sections below, because they are placed differently rather than shaped differently. Two declared things are left out of the graph altogether: test sections, which are given no shape here and reach only the JSON output, and the project root itself, which gets no node of its own — its requirements and its sections stand for it at the top level.

Every node's label is the name a reader would recognise — a requirement's title, everything else's name. An api's description and group are not drawn; the graph is a shape, and the prose belongs to the other two formats.

## Edge Mapping

| Spec Edge | DOT Style |
|-----------|-----------|
| depends_on | dashed arrow |
| requires_module | solid arrow |
| implements | solid arrow (blue) |
| uses | dotted arrow |
| provided_by | solid arrow (purple) |
| preq_id | dashed arrow (blue) |

Every edge also carries a label naming its kind, so the relationship survives in a rendering that drops colour. `describes` is declared only by test sections, and they are not nodes here, so no `describes` edge is drawn in this format at all — that edge survives only in the JSON output.

## Sections

[[d1a61942bc21|Render sections]] reaches this format as project-level nodes: section nodes are declared before the first module cluster opens, which is what keeps them outside every module subgraph. Each section gets a distinct shape to differentiate it from other node types.

For coupled sections, a coupling edge connects the section node to the module node sharing its name, showing the structural relationship. A section of any other type, or one whose name matches no module, is drawn with no coupling edge rather than with a dangling one.

| Spec Node | DOT Shape | Fill |
|-----------|-----------|------|
| Section | tab | mistyrose |

| Spec Edge | DOT Style |
|-----------|-----------|
| coupled (section → module) | bold solid arrow |

## Subgraphs

Each module is rendered as a DOT subgraph (cluster). This visually groups module contents together. The cluster is named for the module — `cluster_<module>`, with hyphens replaced by underscores, since a cluster name is an identifier — and labelled with the module name as declared.

The document is written in one pass: the project-level nodes first, then one cluster per module holding that module's own nodes, then every edge in the graph. Edges come last because they are the only statements that join two clusters, and a `uses` or `requires_module` arrow crossing from one module to another is exactly what the picture is for.
