# Markdown Rendering Implementation

## Approach

Walk the spec graph in a defined order and write markdown to the output writer.

## Algorithm

1. Write project heading and description
2. Write project requirements (grouped by type: functional, then non-functional)
3. For each module (in declaration order from project.json):
   - Write module heading and description
   - Write module requirements
   - Write architecture section: for each component, write heading + inlined content
   - Write implementation section: for each impl_section, write heading + inlined content
   - Write data flows section: for each data_flow, write heading + inlined content

## Heading Level Adjustment

Content markdown files use `#` for their top heading. When inlining, this needs to be adjusted to fit the document hierarchy. For example, a component's content `# Hasher` becomes `#### Hasher` when nested under `## Module: Merkle` → `### Architecture`.

```go
func adjustHeadings(content string, baseLevel int) string {
    // Replace "# " with "#### ", "## " with "##### ", etc.
}
```

## Sections Rendering

After project requirements and before modules, if `sections` is non-empty:

1. Write `## Sections` heading
2. For each section in declaration order:
   - Write `### <name>` heading with type annotation (e.g., `### delivery (coupled)`)
   - Walk the raw JSON content (excluding envelope fields) and render:
     - Objects as nested bullet lists with bold keys
     - Arrays as ordered lists
     - Scalars as inline values
3. No heading adjustment needed — section content is rendered from structured data, not from markdown files

```go
func renderSectionContent(w io.Writer, raw map[string]interface{}, indent int) {
    for key, val := range raw {
        if key == "id" || key == "name" || key == "type" {
            continue // skip envelope
        }
        // render key-value pair based on val's type
    }
}
```

## Output

Pure markdown, no front matter or metadata. The output is suitable for rendering with any markdown viewer or converting to HTML/PDF with pandoc.
