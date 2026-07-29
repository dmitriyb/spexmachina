# JSONRenderer

Generates a machine-readable JSON graph from the spec.

## Responsibilities

- Flatten the spec into a single JSON document with explicit nodes and edges
- Include all metadata and content
- Write to stdout for piping

## Interface

The renderer is handed the graph [[7d1150c19724|SpecReader]] parsed and the stream to write to; `spex render --format json` supplies stdout. It has two output shapes, and the caller picks one: the full graph, and the slim node list `--slim` selects.

## Output Format

```json
{
  "nodes": [
    {"id": "project", "type": "project", "name": "spex-machina", "description": "..."},
    {"id": "project:req:<hash>", "type": "requirement", "name": "<title>", "description": "..."},
    {"id": "module:schema", "type": "module", "name": "schema", "description": "..."},
    {"id": "module:schema:req:<hash>", "type": "requirement", "name": "<title>", "description": "...", "module": "schema"},
    {"id": "module:schema:comp:<hash>", "type": "component", "name": "...", "description": "...", "content": "<inlined markdown>", "module": "schema"},
    {"id": "module:schema:flow:<hash>", "type": "data_flow", "name": "...", "description": "...", "content": "<inlined markdown>", "module": "schema"},
    {"id": "module:schema:test:<hash>", "type": "test_section", "name": "...", "content": "<inlined markdown>", "module": "schema"},
    {"id": "module:render:api:<hash>", "type": "api", "name": "spex render", "description": "...", "module": "render", "group": "cli"}
  ],
  "edges": [
    {"from": "module:validator", "to": "module:schema", "type": "requires_module"},
    {"from": "module:schema:comp:<hash>", "to": "module:schema:req:<hash>", "type": "implements"},
    {"from": "module:schema:test:<hash>", "to": "module:schema:comp:<hash>", "type": "describes"},
    {"from": "module:render:api:<hash>", "to": "module:render:comp:<hash>", "type": "provided_by"}
  ]
}
```

Every node carries `id`, `type` and `name`; `description`, `content`, `module` and `group` appear only where the declaration has them, so an api carries no `content` key at all and a test section carries no `description`. The document is indented two spaces, and both arrays follow one declaration walk — the project node, the project's requirements and sections, then each module in turn contributing its own node, its requirements, its components, its data flows, its test sections and its apis. Declaration order is the only order there is, which is what makes two renderings of one unchanged spec byte-identical.

`--slim` replaces that envelope with a nodes-only one. There is no `edges` key at all — callers read edges from `module.json` — and each node carries exactly `id`, `type`, `name` and `module`, dropping the inlined content and descriptions that make up most of the full document's bytes. `module` names the owning module and is absent on the project's own requirements and sections and on module nodes themselves. The whole thing is one compact line rather than an indented document, because it is a lookup table and not something to read. Every node type the full graph emits is there, test sections and apis included; the one node the full graph has and the slim view does not is the project root, which has no identity hash to be looked up by.

## Node IDs

Synthetic IDs are constructed from the path: `module:<name>:<type>:<id>`, where `<type>` is the abbreviated node kind — `req`, `comp`, `flow`, `test`, `api`. A module is `module:<name>`, a project requirement is `project:req:<id>` and the project itself is the bare string `project`. This creates globally unique identifiers for the flat graph representation.

`--slim` uses no synthetic prefix: a slim node's `id` is the declared identity hash itself, copied out of the declaration and never recomputed from the node's name. That distinction is load-bearing, not incidental — the identity hashes that predate the current identity string, still keyed on by snapshots and by the bead map, would not survive a renderer that recomputed them.

## Sections

[[d1a61942bc21|Render sections]] reaches this format as ordinary graph nodes. Section nodes use the ID format `section:<name>` (e.g., `section:delivery`). The node includes:

- `type: "section"`
- `name`: the section name
- `section_type`: the section type (e.g., "coupled")
- All freeform content fields from the section entry, except where one collides with an envelope key, which the envelope wins

For coupled sections, an edge `{"from": "section:<name>", "to": "module:<name>", "type": "coupled"}` represents the coupling relationship.

## Composability

[[1078c088e0c6|Render JSON]] exists to be read by another program: the graph is designed for piping to tools like `jq` for querying, or to visualization tools that consume graph JSON. That is why a node's `content` holds the markdown text itself rather than the path it was read from — the document answers questions about the spec without its consumer touching the spec directory at all.
