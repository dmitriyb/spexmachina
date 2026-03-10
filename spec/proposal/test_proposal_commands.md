# Proposal Command Tests

Tests for the ProposalCommands component (component 4): CLI entry points `spex register`, `spex log`, `spex template` and their wiring to Registrar, HistoryViewer, and TemplateProvider.

## Setup

Build the `spex` binary from source. Create a temporary directory with a valid spec structure for end-to-end command execution:

```
tmpdir/
  spec/
    project.json
    proposals/
      2026-02-23-spex-machina.md    # existing registered proposal
  input/
    new-change.md                   # valid change proposal (Context, Proposed change, Impact expectation)
    invalid-proposal.md             # missing required sections
```

Mock or set up a `br` binary that responds to `br list --json` with deterministic bead data (same as HistoryViewer setup). Set `$PATH` so that the mock `br` is found before any real installation.

For exit code tests, run `spex` via `exec.Command` and inspect `cmd.Run()` error / `ExitError.ExitCode()`.

## Scenarios

### spex register

#### S1: Register a valid proposal via CLI

**Given** `new-change.md` is a valid change proposal with all required sections.
**When** `spex register input/new-change.md` is executed in `tmpdir/`.
**Then:**
- Exit code is 0.
- `spec/proposals/` contains a new file with today's date and a slug derived from the proposal's H1 heading.
- Stdout reports the registered filename (e.g., `registered: spec/proposals/2026-03-10-new-change.md`).

#### S2: Register with explicit spec directory

**Given** `new-change.md` is valid. Spec directory is at a non-default location `/tmp/myspec/`.
**When** `spex register --spec-dir /tmp/myspec input/new-change.md` is executed.
**Then:**
- The proposal is copied to `/tmp/myspec/proposals/`.
- Exit code is 0.

#### S3: Register with validation failure

**Given** `invalid-proposal.md` has headings `## Background` and `## Plan` (neither project nor change type detectable).
**When** `spex register input/invalid-proposal.md` is executed.
**Then:**
- Exit code is 1.
- Stderr contains the validation error message: "proposal: cannot detect type from headings".
- No file is created in `spec/proposals/`.

#### S4: Register with missing file argument

**Given** no arguments passed after `register`.
**When** `spex register` is executed (no proposal path).
**Then:**
- Exit code is 1.
- Stderr contains a usage message indicating that a proposal file path is required.

#### S5: Register with nonexistent file

**Given** the path `input/ghost.md` does not exist.
**When** `spex register input/ghost.md` is executed.
**Then:**
- Exit code is 1.
- Stderr contains a file-not-found error.

### spex log

#### S6: Show proposal history in human-readable format

**Given** `spec/proposals/` contains `2026-02-23-spex-machina.md` and beads are tagged with this proposal.
**When** `spex log` is executed.
**Then:**
- Exit code is 0.
- Stdout contains the proposal filename and its linked bead actions, formatted as described in the HistoryViewer architecture:
  ```
  2026-02-23-spex-machina.md (project proposal)
    Created: spexmachina-abc (schema: ProjectSchema)
  ```

#### S7: Show proposal history in JSON format

**Given** same as S6.
**When** `spex log --json` is executed.
**Then:**
- Exit code is 0.
- Stdout is valid JSON parseable by `jq` or `json.Unmarshal`.
- JSON structure matches the HistoryViewer JSON output spec (array of proposal records with `proposal`, `type`, `date`, `beads` fields).

#### S8: Log with empty proposals directory

**Given** `spec/proposals/` exists but is empty.
**When** `spex log` is executed.
**Then:**
- Exit code is 0.
- Stdout is empty (human-readable) or `[]` (JSON mode).
- No error output.

#### S9: Log with explicit spec directory

**Given** proposals exist at `/tmp/otherspec/proposals/`.
**When** `spex log --spec-dir /tmp/otherspec` is executed.
**Then:**
- History is read from `/tmp/otherspec/proposals/`.
- Exit code is 0.

#### S10: Log when bead CLI is unavailable

**Given** `br` is not on `$PATH`.
**When** `spex log` is executed.
**Then:**
- Exit code is 1.
- Stderr contains an error about the bead CLI being unavailable.

### spex template

#### S11: Output project template

**Given** no preconditions.
**When** `spex template project` is executed.
**Then:**
- Exit code is 0.
- Stdout contains the full project proposal template starting with `# Project Proposal: <Project Name>`.
- Template includes `## Vision`, `## Modules`, `## Key requirements`, `## Design decisions` headings.

#### S12: Output change template

**Given** no preconditions.
**When** `spex template change` is executed.
**Then:**
- Exit code is 0.
- Stdout contains the full change proposal template starting with `# Change Proposal: <Title>`.
- Template includes `## Context`, `## Proposed change`, `## Impact expectation` headings.

#### S13: Invalid template type argument

**Given** no preconditions.
**When** `spex template unknown` is executed.
**Then:**
- Exit code is 1.
- Stderr contains: `proposal: unknown template type: "unknown"`.

#### S14: Missing template type argument

**Given** no preconditions.
**When** `spex template` is executed (no type argument).
**Then:**
- Exit code is 1.
- Stderr contains a usage message indicating that a template type (project or change) is required.

#### S15: Template output is pipeable

**Given** no preconditions.
**When** `spex template project > /tmp/my-proposal.md` is executed (stdout redirected to file).
**Then:**
- `/tmp/my-proposal.md` contains the full project template.
- No extra output (no progress messages, no prompts) is mixed into stdout. Informational messages, if any, go to stderr.

### End-to-end pipeline

#### S16: Register then log round-trip

**Given** empty `spec/proposals/` directory.
**When:**
1. `spex template change > /tmp/new-proposal.md` (generate template).
2. Fill in required sections in `/tmp/new-proposal.md` with real content.
3. `spex register /tmp/new-proposal.md` (register the filled-in proposal).
4. `spex log --json` (view history).
**Then:**
- Step 3 succeeds (exit code 0).
- Step 4 output includes the newly registered proposal in the JSON array.
- The registered proposal has an empty `beads` array (no beads tagged with it yet).

#### S17: Register preserves composability contract

**Given** a valid change proposal file.
**When** `spex register input/new-change.md` is executed.
**Then:**
- The command reads a file (input), writes a file (output to `spec/proposals/`), and exits 0 or 1.
- No interactive prompts. No network calls. No side effects outside the spec directory.
- Suitable for use in shell scripts and CI pipelines.

## Edge Cases

### E1: Register when spec/proposals/ is a symlink

**Given** `spec/proposals` is a symlink to `/tmp/shared-proposals/`.
**When** `spex register input/new-change.md` is executed.
**Then:**
- File is written through the symlink to the target directory.
- Exit code is 0.

### E2: Register with read-only proposals directory

**Given** `spec/proposals/` exists but has permissions 0555 (read + execute only).
**When** `spex register input/new-change.md` is executed.
**Then:**
- Exit code is 1.
- Stderr contains a permission-denied error.
- The original proposal file is not modified.

### E3: Log output does not include ANSI color codes when piped

**Given** stdout is not a terminal (piped to another process).
**When** `spex log | cat` is executed.
**Then:**
- Output contains no ANSI escape sequences.
- Human-readable format remains parseable as plain text.

### E4: Multiple subcommands share the same spec directory default

**Given** the working directory contains `spec/proposals/` with existing proposals.
**When** `spex register`, `spex log`, and `spex template` are each run without `--spec-dir`.
**Then:**
- All three commands default to `spec/` as the spec directory relative to the current working directory.
- Behavior is consistent: a file registered with `spex register` is visible in `spex log` output.

### E5: Concurrent register calls for different proposals

**Given** two valid proposal files `a.md` and `b.md`.
**When** `spex register a.md` and `spex register b.md` are executed concurrently.
**Then:**
- Both succeed (exit code 0 for each).
- `spec/proposals/` contains both registered files.
- No race condition on directory creation or file writes (files have distinct names).

### E6: Register idempotency check

**Given** a valid proposal that has already been registered.
**When** `spex register input/new-change.md` is executed a second time on the same day.
**Then:**
- Exit code is 1 (target file already exists).
- Stderr contains a message indicating the proposal is already registered.
- The existing registered file is not modified or overwritten.
