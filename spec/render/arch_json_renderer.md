# JSONRenderer

Generates a machine-readable JSON graph from the spec.

## Responsibilities

- Flatten the spec into a single JSON document with explicit nodes and edges
- Include all metadata and content
- Write to stdout for piping

## Interface

```go
func RenderJSON(spec *SpecGraph, w io.Writer) error
```

## Output Format

```json
{
  "nodes": [
    {"id": "project", "type": "project", "name": "spex-machina", "description": "..."},
    {"id": "module:schema", "type": "module", "name": "schema", "description": "..."},
    {"id": "module:schema:req:1", "type": "requirement", "title": "...", "description": "..."},
    {"id": "module:schema:comp:1", "type": "component", "name": "...", "content": "..."}
  ],
  "edges": [
    {"from": "module:validator", "to": "module:schema", "type": "requires_module"},
    {"from": "module:schema:comp:1", "to": "module:schema:req:1", "type": "implements"}
  ]
}
```

## Node IDs

Synthetic IDs are constructed from the path: `module:<name>:<type>:<id>`. This creates globally unique identifiers for the flat graph representation.

## Composability

The JSON graph is designed for piping to tools like `jq` for querying, or to visualization tools that consume graph JSON.
