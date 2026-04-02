# MarkdownRenderer

Generates a collated markdown document from the spec.

## Responsibilities

- Produce a single markdown document with all spec content inlined
- Structure: project overview → per-module (requirements → architecture → implementation)
- Write to stdout or a specified writer

## Interface

```go
func RenderMarkdown(spec *SpecGraph, w io.Writer) error
```

## Document Structure

```markdown
# <Project Name>

<Project description>

## Requirements

### Functional
- FR1: <title> — <description>
...

### Non-functional
- NFR1: <title> — <description>
...

## Module: <Name>

<Module description>

### Requirements
...

### Architecture

#### <Component Name>
<Inlined content from arch_*.md>

### Implementation

#### <Impl Section Name>
<Inlined content from impl_*.md>

### Data Flows

#### <Flow Name>
<Inlined content from flow_*.md>
```

## Sections Rendering

If the spec contains a `sections` array, a `## Sections` heading appears after project requirements and before modules. Each section is rendered as:

```markdown
## Sections

### <section name> (type)

<Freeform content fields rendered as key-value pairs or structured lists>
```

The renderer iterates sections generically — no hardcoded logic for specific section types. Freeform content is rendered by walking the raw JSON structure:
- Objects render as nested lists or subsections
- Arrays render as ordered lists
- Scalars render inline

For coupled sections, the module's own components/impl_sections appear in the module's `## Module:` section, not duplicated under the sections heading.

## Content Inlining

Markdown content is included verbatim from the content files. Headings within content files are adjusted (indented by the appropriate level) to fit the document hierarchy.
