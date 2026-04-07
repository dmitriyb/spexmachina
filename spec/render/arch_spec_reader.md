# SpecReader

Reads and parses the spec directory into an in-memory graph structure.

## Responsibilities

- Read `project.json` and all `module.json` files
- Parse into typed Go structs
- Read all referenced markdown content files
- Build an in-memory graph with nodes, edges, and inline content

## Interface

```go
type SpecGraph struct {
    Project    Project
    Modules    []ModuleGraph
}

type ModuleGraph struct {
    Module     Module
    Content    map[string]string  // path → markdown content
}

func ReadSpec(specDir string) (*SpecGraph, error)
```

## Shared Foundation

SpecReader is used by all three renderers. It provides the common parsed representation that each renderer transforms into its output format.

## Content Inlining

Markdown content files are read into memory and stored in the `Content` map keyed by their relative path. Renderers access content by the `content` field of components, impl_sections, and data_flows.

## Sections Support

When `project.json` contains a `sections` array, SpecReader preserves both the typed envelope and the raw freeform content:

- The `Project` struct includes a `Sections` field with envelope data (id, name, type)
- Raw section content is preserved via `json.RawMessage` or equivalent, so renderers can access freeform fields without knowing the section's content schema
- For coupled sections, SpecReader does NOT load or validate `section.schema.json` — that's the validator's job. SpecReader just reads and preserves what's there.

## Error Handling

If any JSON file fails to parse or any content file is missing, return an error. Run `spex validate` first to catch structural issues.
