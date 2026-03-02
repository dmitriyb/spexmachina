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
