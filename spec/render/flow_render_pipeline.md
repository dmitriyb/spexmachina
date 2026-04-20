# Render Pipeline

## Data Flow

```
spec directory
     │
     ▼
┌────────────┐
│ SpecReader  │── parse JSON + read markdown content
└──────┬─────┘
       │ SpecGraph
       ▼
┌──────────────────────────────┐
│ Format Selection              │
│ (--format markdown|dot|json)  │
└──────┬──────┬──────┬─────────┘
       │      │      │
       ▼      ▼      ▼
   Markdown  DOT   JSON
   Renderer  Rend.  Rend.
       │      │      │
       ▼      ▼      ▼
     stdout  stdout  stdout
```

## Subcommand Design

The render module is exposed as `spex render` with a `--format` flag:

```
spex render --format markdown    # collated spec document
spex render --format dot         # graphviz graph
spex render --format json        # machine-readable graph
```

Default format is `markdown`.

## Piping Examples

```bash
spex render --format dot | dot -Tpng > spec.png
spex render --format json | jq '.nodes[] | select(.type == "component")'
spex render --format markdown > spec.md
```

## Data Shapes

### SpecReader → renderers (in-memory SpecGraph)

- SpecGraph:
  - project: ProjectNode
  - modules: list of ModuleNode
  - nodes_by_id: map keyed by identity_hash → any node

- ProjectNode: shape of spec/project.json after JSON parse, plus
  - content_files: list of string — markdown file paths read from disk (absolute)

- ModuleNode: shape of spec/<module>/module.json plus
  - dir: string — absolute directory path
  - content_files: list of string — resolved content markdown paths

Every node carries its 12-char hex identity hash in its `id` field. Edges are
kept as identity-hash references (no pointer fix-up; consumers resolve via
nodes_by_id).

### Renderers → stdout (format envelope)

- MarkdownRender: single UTF-8 document. Section ordering is project → modules →
  per-module requirements → components → impl_sections → data_flows →
  test_sections. Heading levels mirror graph depth.
- DotRender: single `digraph` block. Node IDs are identity hashes; labels are
  `<name>\\n<id[:8]>`. Edge kinds (`implements`, `uses`, `describes`,
  `requires_module`, `preq_id`, `depends_on`) become DOT edge attributes.
- JsonRender:
  - nodes: list of {id, type, name, module, content_hash}
  - edges: list of {from, to, kind}

### Shape contract

Field renames, additions, or removals in SpecGraph require updates to all
renderers. A new spec node type (e.g., `section`) requires matching handling
in all three render formats.
