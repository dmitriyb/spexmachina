# JSON Serialization Implementation

## Approach

Build flat node and edge arrays, then serialize as JSON.

## Algorithm

1. Create empty `nodes` and `edges` slices
2. Add project node
3. For each module:
   - Add module node
   - Add `requires_module` edges
   - For each requirement: add node, add `depends_on` and `preq_id` edges
   - For each component: add node, add `implements` and `uses` edges
   - For each impl_section: add node, add `describes` edges
   - For each data_flow: add node, add `uses` edges
4. Serialize `{"nodes": [...], "edges": [...]}` with 2-space indentation

## Node Structure

```go
type GraphNode struct {
    ID          string `json:"id"`
    Type        string `json:"type"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    Content     string `json:"content,omitempty"`
    Module      string `json:"module,omitempty"`
}
```

## Edge Structure

```go
type GraphEdge struct {
    From string `json:"from"`
    To   string `json:"to"`
    Type string `json:"type"`
}
```

## Sections

Section nodes are added after the project node and before module nodes:

```go
for _, section := range project.Sections {
    node := GraphNode{
        ID:   fmt.Sprintf("section:%s", section.Name),
        Type: "section",
        Name: section.Name,
    }
    // Include freeform content from raw JSON
    nodes = append(nodes, node)

    if section.Type == "coupled" {
        edges = append(edges, GraphEdge{
            From: node.ID,
            To:   fmt.Sprintf("module:%s", section.Name),
            Type: "coupled",
        })
    }
}
```

Section nodes include all freeform content fields from the raw section JSON, making the JSON output self-contained for section content as well.

## Content Inclusion

The `content` field on nodes contains the full markdown content (inlined, not the file path). This makes the JSON self-contained — consumers don't need filesystem access.
