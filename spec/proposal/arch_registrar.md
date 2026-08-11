# Registrar

Registers proposals: it implements [[2b62ad5e8ef2|Register proposal]], reading a proposal file,
checking that the sections its type requires are present, appending the `registered` journal
event that opens the proposal's lifecycle, and copying the file into `spec/proposals/`.
That copy plus that one journal line are the whole of the record, because
[[924cf30cbd58|proposals are git-native]] — plain markdown and a JSONL line, both under version
control, so a registration adds to the working tree and touches no
database and no state living outside git. The module that starts a lifecycle records its start:
the epic's `task_created` later references the registered event, which is what lets every task
pairing in the journal reference an event.

## Responsibilities

- Accept a proposal file path as input, plus a caller-supplied git head — spex never calls git
- Validate required sections are present based on proposal type
- Append the `registered` event — `{"event":"registered","eid":"<git_head>:<slug>",
  "proposal":"<slug>","git_head":"<head>"}` — through the map module's MappingStore, the
  journal's writer-owner
- Copy the file to `spec/proposals/` with the naming convention `YYYY-MM-DD-<name>.md`
- Report every missing section, not only the first one found, so one run tells the author
  everything that has to be fixed

## Interface

[[ac3c6ca0b337|`spex register`]] takes one argument, the path to the source proposal file, and
registers it under the spec directory the command was given: the file is read, its type is detected
from its headings, the sections that type requires are checked, and only then is a copy written into
`proposals/` beneath that directory. A proposal is refused when the file cannot be read, when its
type cannot be detected, when a required section is missing, when a proposal is already registered
under the filename this run would choose — reported as `proposal: already registered: <filename>` —
when the `registered` event cannot be appended to the task journal, and when `proposals/` cannot
be created or the copy cannot be written into it. In every refusal an
error goes to the caller, the exit code is non-zero, and no file is added to the proposals
directory.

What it reports back is the **basename** the proposal was written under: a bare
`YYYY-MM-DD-<name>.md`, never a path. No part of the directory it wrote into comes back with it, so
a caller that names a directory names one of its own.

Reporting the filename is what makes the caller's next step possible: its stem *is* the proposal
reference threaded through the rest of the pipeline (`spex emit --proposal <ref>`), and the
registrar is the component that decides it, since it may rename the file to satisfy the
`YYYY-MM-DD-<name>.md` convention. A caller that only learned success or failure would have to
re-derive the reference by guessing at the same naming rules.

Registration is a local file read, a section check, a journal append and a file write — no
subprocess, no network, nothing long-running, and so nothing to cancel part-way through.

## Ordering and recovery

A successful registration leaves two marks: the `registered` journal event and the copied file.
The order is validation, then the journal append, then the copy — every refusal happens before
either mark lands, so a refused proposal leaves neither. The append is idempotent by eid
(`<git_head>:<slug>` is deterministic), so the one partial state a crash can leave — event
appended, file not yet copied — is repaired by re-running: the already-registered check finds no
file, the append finds its eid already present and adds nothing, and the copy lands. No ordering
leaves a registered file without its event.

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

Proposal type is detected by section headings. If the file contains a `## Vision` heading, it's a project proposal. If it contains `## Proposed change`, it's a change proposal; a file carrying both reads as a project proposal, because Vision is looked for first. Heading matching is case-insensitive, both here and in the required-section check, so `## VISION` and `## vision` count exactly as `## Vision` does. A file with neither heading is refused before anything is copied, with an error saying the proposal type could not be detected from the headings.

## Naming

If the source file doesn't follow the `YYYY-MM-DD-<name>.md` convention, the registrar renames it during copy, using today's date and a slug derived from the first heading. The copy is written with mode 0644 — readable, never executable.
