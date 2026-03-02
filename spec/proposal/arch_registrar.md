# Registrar

Registers proposals by copying them to `spec/proposals/` and validating their structure.

## Responsibilities

- Accept a proposal file path as input
- Validate required sections are present based on proposal type
- Copy the file to `spec/proposals/` with the naming convention `YYYY-MM-DD-<name>.md`
- Report validation errors if sections are missing

## Interface

```go
func Register(ctx context.Context, proposalPath, specDir string) error
```

## Section Validation

### Project proposal (required sections)
- Vision
- Modules
- Key requirements
- Design decisions

### Change proposal (required sections)
- Context
- Proposed change
- Impact expectation

## Detection

Proposal type is detected by section headings. If the file contains a `## Vision` heading, it's a project proposal. If it contains `## Proposed change`, it's a change proposal.

## Naming

If the source file doesn't follow the `YYYY-MM-DD-<name>.md` convention, the registrar renames it during copy, using today's date and a slug derived from the first heading.
