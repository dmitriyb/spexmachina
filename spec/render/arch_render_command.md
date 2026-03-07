# RenderCommand

CLI entry point for `spex render`. Generates human-readable or machine-readable output from the spec.

## Responsibilities

- Parse CLI flags: spec directory, output format (markdown/dot/json)
- Wire SpecReader to parse the spec into an in-memory graph
- Wire the selected renderer (MarkdownRenderer, DOTRenderer, or JSONRenderer)
- Output to stdout

## Interface

```
spex render [dir] [--format markdown|dot|json]
```
