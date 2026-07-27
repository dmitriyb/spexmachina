# Registrar

Registers proposals by copying them to `spec/proposals/` and validating their structure.

## Responsibilities

- Accept a proposal file path as input
- Validate required sections are present based on proposal type
- Copy the file to `spec/proposals/` with the naming convention `YYYY-MM-DD-<name>.md`
- Report validation errors if sections are missing

## Interface

`Register` takes two strings — the path to the source proposal file and the spec
directory to register it into — and returns two values: the **basename** the
proposal was written under, and an error. It is a bare `YYYY-MM-DD-<name>.md`,
not a path: the caller holds the spec directory it passed in and joins the two
itself, which is why `cmd/spex/register.go` prints `registered:
spec/proposals/<filename>` rather than echoing the return value directly.

Returning the filename is what makes the caller's next step possible: its stem
*is* the proposal reference threaded through the rest of the pipeline
(`spex emit --proposal <ref>`), and the registrar is the component that decides
it, since it may rename the file to satisfy the `YYYY-MM-DD-<name>.md`
convention. A caller that only learned success or failure would have to
re-derive the reference by guessing at the same naming rules.

There is no context parameter. Registration is a local file read, a structural
check and a file write — no subprocess, no network, nothing to cancel.

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
