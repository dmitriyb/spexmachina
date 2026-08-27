# SpecReader

Reads and parses the spec directory into an in-memory graph structure.

## Responsibilities

- Read `project.json` and every `module.json` it declares
- Parse each file into the typed spec nodes the renderers read — the arrays walked are the ones the resolved profile declares, so a profile-declared type's nodes are read on the same terms as the built-in types', and under the default profile the walked set is exactly today's arrays
- Read all referenced markdown content files
- Build an in-memory graph with nodes, edges, and inline content

## Interface

The reader has a single entry point. It takes the path of a spec directory and yields the parsed graph: the project envelope, and then each declared module — in `project.json` declaration order — carrying that module's own declarations together with the text of every content file it named.

## Shared Foundation

One reader serves three outputs. [[8828685278e9|Render markdown]], [[a596d8caefb1|Render DOT]] and [[1078c088e0c6|Render JSON]] are three transformations of one parsed representation rather than three readers of the same directory, so whatever the reader fails to surface is missing from all three at once. Availability is not delivery, though: a node the reader hands on reaches a given output only if that format draws it, and § Nodes Without Content below records where the three differ. Test sections are the plainest case — the reader reads their content like any other, and only the JSON output emits them.

## Content Inlining

Markdown content files are read into memory and stored under their relative path — the same string the declaring node's `content` field holds. The declarations that carry a `content` field are the resolved profile's content-bearing types — under the default profile: components, data flows and test sections — and every one of them that fills it has that file read. Renderers then ask for a node's content by the path that node declared.

## Nodes Without Content

Not every node the reader surfaces has a content file. An api has none — it has no `content` field at all, and hashes from its JSON fields alone — so no file is ever read on its behalf and a renderer that reaches for content by path finds nothing to reach for.

Such a node is still a node, and the reader must hand it on. The parsed `apis` array travels with the rest of the module's typed fields, and each renderer projects an api from those fields directly — but each projects a different subset:

| Renderer | Projects | Omits |
|----------|----------|-------|
| JSON | name, description, group, and one `provided_by` edge per component hash | — |
| Markdown | name, group and description, one line per api | `provided_by` |
| DOT | name, as the node label, and one `provided_by` edge per component hash | description, group |

The JSON renderer's slim mode is the documented exception: it reduces every node type, api included, to id/type/name/module.

The markdown omission is worth naming plainly rather than leaving to be discovered. `provided_by` is the only place in the graph the surface-to-worker pairing is recorded, so a markdown rendering shows a module's external surface without showing which components stand behind it — a reader of that output alone cannot recover the pairing from anywhere else. It is a narrower form of the same failure as dropping the surface entirely: an api whose `provided_by` is invisible reads as work belonging to nobody.

[[8d441659a190|Composable output]] is what makes handing an api on a contract rather than a courtesy. A renderer that walks only the content read from disk drops the module's entire external surface silently, which is the failure this section exists to prevent — the reader cannot signal the omission, because there is no missing file to report.

The same requirement bounds what this reader may open: authored spec only — `project.json`, each `module.json`, and the content leaves they declare. SpecReader never reads the snapshot or the task journal, which is what keeps the render surface runnable in a directory `spex init` has never touched: every failure it can produce is a read or parse failure of an authored file, surfacing as the command's documented input-error exit, never as a project-state error.

## Sections Support

[[d1a61942bc21|Render sections]] is served generically here: when `project.json` contains a `sections` array, SpecReader preserves both the typed envelope and the raw freeform content, and it does so without knowing any section type by name.

- The parsed project carries a sections list holding each section's envelope: id, name and type
- Each section's body is preserved verbatim as raw JSON, so renderers can reach freeform fields without knowing the section's content schema
- For coupled sections, SpecReader does NOT load or validate `section.schema.json` — that's the validator's job. SpecReader just reads and preserves what's there.

## Error Handling

If any JSON file fails to parse or any content file is missing, the whole read fails and no partial graph comes back. The error names the file it was reading: a module that will not load is reported by module name as well as by path, and a missing content file is reported by path together with the module that referenced it. Run `spex validate` first to catch structural issues.
