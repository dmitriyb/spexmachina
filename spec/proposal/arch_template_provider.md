# TemplateProvider

Generates proposal templates and writes them to stdout.

## Responsibilities

- Output a project proposal template or a change proposal template
- Templates include all required sections with placeholder content
- Templates are embedded in the binary (no external files)

## Interface

```go
func Template(templateType string, w io.Writer) error
```

## Template Types

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
