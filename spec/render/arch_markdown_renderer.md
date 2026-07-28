# MarkdownRenderer

Generates a collated markdown document from the spec.

## Responsibilities

- Produce a single markdown document with all spec content inlined
- Structure: project overview → per-module (requirements → external surface → architecture → implementation → data flows)
- Write to stdout, or to whatever stream the caller supplies
- Add nothing of its own around the markdown: no front matter, no metadata header, no wrapper — the first line of output is the project heading

## Interface

The renderer is handed the graph [[7d1150c19724|SpecReader]] parsed and the stream to write to; `spex render` supplies stdout. It opens no files of its own: every byte it inlines was already read into that graph.

## Document Structure

The shape [[8828685278e9|Render markdown]] asks for is one collated document, requirements before architecture, with each node's own markdown inlined where the node sits:

```markdown
# <Project Name>

<Project description>

## Requirements

### Functional
- FR<n>: <title> — <description>
...

### Non-functional
- NFR<n>: <title> — <description>
...

## Module: <Name>

<Module description>

### Requirements
- FR<n>: <title> — <description>
- NFR<n>: <title> — <description>

### APIs
- `<api name>` (<group>) — <description>

### Architecture

#### <the arch_*.md file's own `#` heading>
<the rest of that file, inlined>

### Implementation

#### <the impl_*.md file's own `#` heading>
<the rest of that file, inlined>

### Data Flows

#### <the flow_*.md file's own `#` heading>
<the rest of that file, inlined>
```

Where a node declares a content file, that file's own `#` heading is what lands at `####` and the node's declared `name` appears as no heading at all — a component called `Parser` whose `arch_parser.md` opens `# Totally Different Title` renders under that title. A node declaring no content file is the other case, and there the `name` is written as a bare `#### <name>` with nothing beneath it.

Modules appear in the order `project.json` declares them, and within a module the apis, components, impl sections and data flows follow their own declaration order; requirements are grouped functional first and non-functional second, each group in declaration order. The `FR`/`NFR` prefixes are positional labels, not identities: within a module each group restarts at `FR1` and `NFR1`, while the project-level non-functional list continues the numbering its functional list ended on. A module that declares no apis emits no `### APIs` heading at all, and the same holds for `### Requirements`, `### Architecture`, `### Implementation` and `### Data Flows`; the `## Module:` heading itself is always written, so a module declaring nothing renders as that heading and nothing beneath it.

## Sections Rendering

[[d1a61942bc21|Render sections]] reaches markdown as a `## Sections` heading placed after the project requirements and before the first module. Each section is rendered as:

```markdown
## Sections

### <section name> (type)

<Freeform content fields rendered as key-value pairs or structured lists>
```

Sections keep their declaration order, and the renderer iterates them generically — no hardcoded logic for specific section types. Freeform content is rendered by walking the raw JSON structure:
- Objects render as nested lists or subsections
- Arrays render as ordered lists
- Scalars render inline

Nothing is heading-adjusted here, because none of it came from a markdown file. For coupled sections, the module's own components and impl sections appear in the module's `## Module:` section, not duplicated under the sections heading.

## Content Inlining

Markdown content is included verbatim from the content files, with only its heading levels shifted to fit the document hierarchy. A content file's own `#` heading lands three levels down, as `####`, which is one level under the `### Architecture`, `### Implementation` or `### Data Flows` heading it sits beneath — the shift is the same three levels under all three. Deeper headings take that same shift until it would carry them past `######`, the deepest markdown has, and there they stop: `##` becomes `#####` and `###` becomes `######`, but `####` shifts by only two and anything deeper by less still, all of them landing on `######`. So `# Hasher` in a component's content file reads as `#### Hasher` under `## Module: merkle` → `### Architecture`, and a `## Algorithm` inside that same file reads as `##### Algorithm`, while a `#### Edge case` and a `##### Edge case` in that file are both flattened onto `######` and stop being distinguishable in the collated document.
