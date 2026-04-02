# DOT Generation Implementation

## Approach

Build DOT syntax incrementally using `fmt.Fprintf` to a writer.

## Algorithm

1. Write `digraph spec {` preamble with global settings
2. For each module, write a `subgraph cluster_<name> {` block
3. Within each subgraph, write nodes for requirements, components, impl_sections, data_flows
4. After all subgraphs, write edges (edges cross subgraph boundaries)
5. Write closing `}`

## Node ID Generation

DOT node IDs must be valid identifiers. Use `<module>_<type>_<id>` format:
- `schema_req_1` for schema requirement 1
- `validator_comp_2` for validator component 2

## Edge Generation

For each edge type, iterate the source nodes and their reference arrays:

```go
for _, comp := range module.Components {
    for _, reqID := range comp.Implements {
        fmt.Fprintf(w, "  %s -> %s [label=\"implements\"];\n",
            nodeID(module, "comp", comp.ID),
            nodeID(module, "req", reqID))
    }
}
```

## Sections

Section nodes are emitted before module subgraphs, at the project level:

```go
for _, section := range project.Sections {
    fmt.Fprintf(w, "  section_%d [label=%q, shape=tab, style=filled, fillcolor=mistyrose];\n",
        section.ID, section.Name)
    if section.Type == "coupled" {
        module := findModuleByName(project.Modules, section.Name)
        if module != nil {
            fmt.Fprintf(w, "  section_%d -> %s [style=bold, label=\"coupled\"];\n",
                section.ID, moduleNodeID(module))
        }
    }
}
```

The renderer doesn't need to know about specific section content — it just renders the node and the coupling edge. Module-internal details appear in the module's subgraph as usual.

## Layout

Use `rankdir=LR` for left-to-right layout, which reads more naturally for dependency graphs. Modules are clustered visually using subgraphs.
