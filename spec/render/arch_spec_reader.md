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

## Nodes Without Content

Not every node the reader surfaces has a content file. An api has none — it has no `content` field at all, and hashes from its JSON fields alone — so it never appears in the `Content` map and a renderer that reaches for content by path finds nothing to reach for.

Such a node is still a node, and the reader must hand it on. The parsed `apis` array travels with the rest of the module's typed fields, and each renderer projects an api from those fields directly — but each projects a different subset:

| Renderer | Projects | Omits |
|----------|----------|-------|
| JSON | name, description, group, and one `provided_by` edge per component hash | — |
| Markdown | name, group and description, one line per api | `provided_by` |
| DOT | name, as the node label, and one `provided_by` edge per component hash | description, group |

The JSON renderer's slim mode is the documented exception: it reduces every node type, api included, to id/type/name/module.

The markdown omission is worth naming plainly rather than leaving to be discovered. `provided_by` is the only place in the graph the surface-to-worker pairing is recorded, so a markdown rendering shows a module's external surface without showing which components stand behind it — a reader of that output alone cannot recover the pairing from anywhere else. It is a narrower form of the same failure as dropping the surface entirely: an api whose `provided_by` is invisible reads as work belonging to nobody.

A renderer that walks only the `Content` map drops the module's entire external surface silently, which is the failure this section exists to prevent — the reader cannot signal the omission, because there is no missing file to report.

## Sections Support

When `project.json` contains a `sections` array, SpecReader preserves both the typed envelope and the raw freeform content:

- The `Project` struct includes a `Sections` field with envelope data (id, name, type)
- Raw section content is preserved via `json.RawMessage` or equivalent, so renderers can access freeform fields without knowing the section's content schema
- For coupled sections, SpecReader does NOT load or validate `section.schema.json` — that's the validator's job. SpecReader just reads and preserves what's there.

## Error Handling

If any JSON file fails to parse or any content file is missing, return an error. Run `spex validate` first to catch structural issues.
