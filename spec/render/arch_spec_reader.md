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

## Error Handling

If any JSON file fails to parse or any content file is missing, return an error. Run `spex validate` first to catch structural issues.
