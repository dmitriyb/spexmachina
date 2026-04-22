# ProposalCommands

CLI entry points for proposal management: `spex register`, `spex log`, `spex template`. None of these commands invoke external subprocesses; inputs come from files or stdin as data artifacts.

## Responsibilities

- `spex register`: parse proposal path, wire Registrar to validate and register
- `spex log`: read tracker bead data from stdin, parse into `[]BeadRecord`, wire HistoryViewer to group by `spec_proposal:<ref>` label and render grouped output
- `spex template`: wire TemplateProvider to output a project or change proposal template

## `spex log` stdin contract

`spex log` reads the bead tracker's JSON output from stdin. The tracker is whichever tool the user has on PATH (`br`, `bd`, or a GitHub/Jira wrapper); `spex log` never shells out to a tracker binary. Typical usage:

```
br list --json | spex log --proposal 2026-04-18-decouple-spex-from-br
```

Or for all proposals:

```
br list --json | spex log
```

The input shape is the same as impact's `--beads` input: a JSON array (or `{"issues": [...]}` wrapper) of bead objects with `id`, `status`, `labels`, and `title`. ProposalCommands parses it into `[]BeadRecord` and hands the slice to HistoryViewer.

If stdin is empty or not JSON, ProposalCommands exits non-zero with a clear error: `"spex log: no bead data on stdin; pipe 'br list --json' or equivalent"`.

## `spex register` interface

Unchanged from the existing implementation. Registrar reads proposal file content, validates required sections, writes the file into `spec/proposals/`.

## `spex template` interface

Unchanged. TemplateProvider emits project or change proposal templates to stdout.

## Interface

```
spex register <proposal-path>
spex log [--proposal <ref>] [--json]   # reads bead data on stdin
spex template [project|change]
```

## No subprocess invocation

Implements the project-level non-functional requirement `No runtime subprocesses` (id `58ea35f52b86`) for the proposal surface area:

- `spex log` reads stdin; caller pipes tracker output.
- `spex register` reads a file path; no tracker interaction.
- `spex template` writes stdout; no tracker interaction.

The previous `proposal/exec.go` and `proposal/history.go`'s `CLIBeadLister` type — both of which ran `br list --json` as subprocesses — are deleted as part of this proposal's implementation.
