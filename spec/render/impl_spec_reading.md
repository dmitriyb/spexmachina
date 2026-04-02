# Spec Reading Implementation

## Algorithm

1. Read and parse `spec/project.json` into a `Project` struct
2. For each module in `project.modules`:
   - Construct path: `spec/<module.path>/module.json`
   - Read and parse into a `Module` struct
   - For each component, impl_section, and data_flow with a `content` field:
     - Read the content file from `spec/<module.path>/<content>`
     - Store in the `Content` map
3. Return the assembled `SpecGraph`

## Struct Reuse

The same Go structs used by the validator can be reused here. Both modules parse the same JSON files into the same structures. The render module imports the schema package for struct definitions.

## Sections Parsing

If `project.json` includes a `sections` array, the reader preserves it in the `SpecGraph`:

1. During `project.json` unmarshal, sections are parsed into both a typed slice (envelope fields) and raw `json.RawMessage` entries (full content)
2. The raw entries are stored so renderers can access freeform content without knowing the schema
3. No `section.schema.json` loading occurs during reading — validation is the validator's responsibility

```go
type Section struct {
    ID   int             `json:"id"`
    Name string          `json:"name"`
    Type string          `json:"type"`
    Raw  json.RawMessage `json:"-"` // full section entry for renderer access
}
```

## Error Handling

- Missing `project.json`: return error
- Missing `module.json`: return error with module name
- Missing content file: return error with file path (content is required for rendering)
- JSON parse error: return error with file path and parse details
