# ProposalCommands

CLI entry points for proposal management: `spex register`, `spex log`, `spex template`. None of these commands invoke external subprocesses; inputs come from files or stdin as data artifacts.

## Responsibilities

- `spex register`: take the proposal path and the caller-supplied git head off the command line
  and hand them to
  [[24180f55c0b4|Registrar]], which decides both whether the file is accepted and what it ends up
  being called, and appends the `registered` journal event that opens the proposal's lifecycle
- `spex log`: read the tracker's bead data from stdin and parse it, then hand the records to
  [[97f73ced5a02|HistoryViewer]] — the reading and parsing are this component's half of that
  surface, the grouping and rendering are the viewer's
- `spex template`: hand the requested type through to [[cc8adc823719|TemplateProvider]] without
  inspecting it, so an unrecognised type is refused by the provider rather than here

## Declared surfaces

Each entry point is a declared api node, and each of the three — [[ac3c6ca0b337|`spex register`]], [[44a9d44c7eec|`spex log`]] and [[2fcadf69c5c3|`spex template`]] — is declared with two providers rather than one: ProposalCommands, which is the surface a user types at, and the worker that realises it (Registrar, HistoryViewer and TemplateProvider respectively). Whichever of the three is typed, this component is what runs first.

Listing both is not redundancy. ProposalCommands is the only component in this module that cobra reaches, so its `uses` edges are the same set no matter which of the three a caller typed; the per-surface pairing exists nowhere else in the graph, and `provided_by` is where it is now recorded. A reader asking "what does `spex template` actually run" gets an answer from the spec instead of from `cmd/spex/template.go`.

The declared names are the invocation strings alone. `--proposal`, `--json` and the positional `project|change` argument sit behind them, so a flag change moves no name and no hash; renaming a subcommand changes the api's identity and reads as a removal plus an addition, with the removal-time name check reporting every corpus mention that still uses the old string.

## `spex log` stdin contract

`spex log` reads the bead tracker's JSON output from stdin — [[fd540b407fb4|the declared stdin contract for bead data]], and the reason no tracker binary is ever executed. The tracker is whichever tool the user has on PATH (`br`, `bd`, or a GitHub/Jira wrapper). Typical usage:

```
br list --json | spex log --proposal 2026-04-18-decouple-spex-from-br
```

Or for all proposals:

```
br list --json | spex log
```

The input shape is the same as impact's `--beads` input: a JSON array (or `{"issues": [...]}` wrapper) of bead objects with `id`, `status`, `labels`, and `title`. ProposalCommands parses it into bead records and hands them to HistoryViewer.

`--proposal <ref>` narrows the set before the hand-off, keeping only beads whose `spec_proposal:`
label matches the reference given. A trailing `.md` is tolerated on either side of that comparison,
so `--proposal 2026-04-18-decouple-spex-from-br` and the same reference written with `.md` select
the same beads. What survives the filter is grouped and rendered as
[[2e38135487be|the proposal history]] — each proposal beside the bead actions it produced.

If stdin is empty — or carries nothing but whitespace — ProposalCommands exits non-zero with
`spex log: no bead data on stdin; pipe 'br list --json' or equivalent`. Input that arrives but is
not bead JSON is a different failure with a different message: the exit is still non-zero, and the
error reads `spex log: parse bead JSON: <detail>`, naming the decode failure rather than an empty
pipe.

## `spex register` interface

[[2b62ad5e8ef2|Register proposal]] at the command layer. The command carries a `--git-head <sha>`
flag — the caller supplies `$(git rev-parse HEAD)` from their shell, exactly as for `spex emit`,
because spex never calls git; the head feeds the registered event's `<git_head>:<slug>` eid.

The flag is **required**, and required in the same shape `spex emit` requires it: a pre-flight checks
that the value matches `^[0-9a-f]{7,40}$`, and a head that is absent or malformed is refused there —
before the proposal file is read, before Registrar is reached, so neither the journal append nor the
copy can happen. The exit is non-zero and the error names the flag. There is no headless path: an
empty head would key the registered event `":<slug>"`, an eid no later `task_created --for` could
address, so the run is refused rather than threaded through. A bare `spex register <path>` is
therefore an error, not a shorthand that defaults the head to empty.

Two refusals meet on the barest invocation of all, `spex register` with neither a path nor a head,
and the order between them is fixed: the missing positional argument is reported, not the missing
flag. The argument count is checked before any flag is, so the pre-flight above is first only among
the things this command does itself. That ordering is what keeps the missing-path message the answer
to a bare invocation, whatever the flag is doing.

On success the command writes one line to
stdout, `registered: <spec-dir>/proposals/<filename>`. The filename half is the basename Registrar
chose rather than one the command derived, and the directory half is the spec directory this run
resolved — so a run pointed elsewhere by `--spec-dir` names the path it actually wrote. On a refused proposal the command
writes nothing to stdout, reports Registrar's error and exits non-zero.

## `spex template` interface

[[e8c48d1b4cde|Provide templates]] at the command layer. The chosen template reaches stdout with
nothing wrapped around it — no banner, no trailing summary, nothing but the template's own bytes —
so `spex template change > my-proposal.md` is the intended way to start one.

## Interface

```
spex register <proposal-path> --git-head <sha>
spex log [--proposal <ref>] [--json]   # reads bead data on stdin
spex template <project|change>
```

## No subprocess invocation

Implements the project-level non-functional requirement `No runtime subprocesses` (id `58ea35f52b86`) for the proposal surface area — a surface with no share in that requirement's sole sanctioned exception (the cli upgrade command's embedded-installer drive):

- `spex log` reads stdin; caller pipes tracker output.
- `spex register` reads a file path; no tracker interaction.
- `spex template` writes stdout; no tracker interaction.

The previous `proposal/exec.go` and `proposal/history.go`'s `CLIBeadLister` type — both of which ran `br list --json` as subprocesses — are deleted as part of this proposal's implementation.
