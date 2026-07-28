# TemplateProvider

Generates proposal templates and writes them to stdout, which is the whole of
[[e8c48d1b4cde|Provide templates]]: whatever it emits already carries every section the registrar
will later insist on, with placeholder content under each one.

## Responsibilities

- Output a project proposal template or a change proposal template
- Templates include all required sections with placeholder content
- Templates are embedded in the binary (no external files)

## Interface

[[2fcadf69c5c3|`spex template`]] takes exactly one argument, `project` or `change`, and writes the
matching template to stdout. Any other value is refused: nothing is written, and the error names the template type
it did not recognise. The argument is required and only one is accepted, so both `spex template`
alone and `spex template project change` are refused before a template is chosen.

## Template Types

The two templates are static text. There is no variable substitution and no template engine, so the
same invocation emits the same bytes every time and the placeholders are filled in afterwards — by
the author, or with LLM assistance. A template language was the obvious alternative and buys a
proposal author nothing that placeholder text does not, at the cost of the determinism the rest of
the pipeline depends on.

### Project Proposal Template

```markdown
# Project Proposal: <Project Name>

## Vision

<Describe the project vision and motivation>

## Modules

### 1. <Module Name>

<Module description>

## Key requirements

### Functional

1. **<Requirement>** — <description>

### Non-functional

1. **<Requirement>** — <description>

## Design decisions

### <Decision title>

<Rationale and alternatives considered>
```

### Change Proposal Template

```markdown
# Change Proposal: <Title>

## Context

<What exists today and why it needs to change>

## Proposed change

<What specifically will change in the spec>

## Impact expectation

<Which modules/components are expected to be affected>
```
