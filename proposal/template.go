package proposal

import (
	"fmt"
	"io"
)

const projectTemplate = `# Project Proposal: <Project Name>

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
`

const changeTemplate = `# Change Proposal: <Title>

## Context

<What exists today and why it needs to change>

## Proposed change

<What specifically will change in the spec>

## Impact expectation

<Which modules/components are expected to be affected>
`

// Template writes the requested proposal template to w.
// Valid types are "project" and "change".
func Template(templateType string, w io.Writer) error {
	switch templateType {
	case "project":
		_, err := io.WriteString(w, projectTemplate)
		return err
	case "change":
		_, err := io.WriteString(w, changeTemplate)
		return err
	default:
		return fmt.Errorf("proposal: unknown template type: %q", templateType)
	}
}
