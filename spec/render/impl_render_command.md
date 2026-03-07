# Render command implementation

## Structure

`cmd/spex/render.go` — registered as a subcommand of the root `spex` command.

## Flow

1. Parse flags, resolve spec directory and format
2. Call `SpecReader.Read(dir)` to parse spec into graph
3. Based on format flag, call the appropriate renderer:
   - `MarkdownRenderer.Render(graph)` for markdown
   - `DOTRenderer.Render(graph)` for graphviz DOT
   - `JSONRenderer.Render(graph)` for JSON
4. Output to stdout
