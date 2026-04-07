# DOTRenderer

Generates a graphviz DOT graph from the spec.

## Responsibilities

- Map spec nodes to DOT nodes with labels and shapes
- Map spec edges to DOT edges with labels
- Produce valid DOT syntax for rendering with `dot`, `neato`, or other graphviz tools

## Interface

```go
func RenderDOT(spec *SpecGraph, w io.Writer) error
```

## Node Mapping

| Spec Node | DOT Shape | Color |
|-----------|-----------|-------|
| Project requirement | box | light blue |
| Module | folder | light gray |
| Module requirement | box | light green |
| Component | component | light yellow |
| Impl section | note | light orange |
| Data flow | ellipse | light purple |

## Edge Mapping

| Spec Edge | DOT Style |
|-----------|-----------|
| depends_on | dashed arrow |
| requires_module | solid arrow |
| implements | solid arrow (blue) |
| uses | dotted arrow |
| describes | solid arrow (green) |
| preq_id | dashed arrow (blue) |

## Sections

Section nodes are project-level nodes rendered outside any module subgraph. Each section gets a distinct shape (e.g., `shape=tab`) to differentiate from other node types.

For coupled sections, a coupling edge connects the section node to its matching module node, showing the structural relationship.

| Spec Node | DOT Shape | Color |
|-----------|-----------|-------|
| Section | tab | light pink |

| Spec Edge | DOT Style |
|-----------|-----------|
| coupled (section → module) | bold solid arrow |

## Subgraphs

Each module is rendered as a DOT subgraph (cluster). This visually groups module contents together.
