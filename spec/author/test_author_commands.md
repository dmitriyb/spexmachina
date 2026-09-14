# Author command tests

Acceptance scenarios for AuthorCommands: the surface the eight apis present — exit codes, stdout shape, flag handling — as opposed to what each worker does with the values, which the other test leaves in this module cover.

## Setup

The fixture of the node editing tests — `tmp/spec/` with module `alpha`, components Comp1 and Comp2, test section T1 and api `demo run` — and the compiled `spex` binary. Every scenario invokes one or more of `spex node add`, `spex node remove`, `spex node rename`, `spex edge add`, `spex edge remove`, `spex leaf scaffold`, `spex profile show` and `spex migrate` with `--spec-dir tmp/spec/`, and asserts on the exit code first.

## Scenarios

### A1: Every surface is registered and has help

**Given** the compiled binary.
**When** `spex --help`, `spex node --help`, `spex edge --help` and each of the eight surfaces with `--help` are run.
**Then** every run exits 0; `spex --help` lists `node`, `edge`, `leaf`, `profile` and `migrate` among the subcommands; `spex node --help` lists `add`, `remove` and `rename`; `spex edge --help` lists `add` and `remove`; `spex leaf --help` lists `scaffold` and `spex profile --help` lists `show`; and a bare `spex node`, `spex edge`, `spex leaf` or `spex profile` prints that grouping's help and exits 0.

### A2: Exit codes are the documented set

**Given** the fixture.
**When** four runs are made: a successful `spex node add` (N1's), a refused one (N4's undeclared type), one with a missing required flag, and one pointed at a directory holding no `project.json`.
**Then** the codes are 0, 2, 1 and 1 respectively — the last two the same code, because a missing flag and a missing spec are both input errors and nothing about the spec was judged; a malformed `spec/profile.json` is a fifth run and exits 1 for the same reason. Every non-zero run puts one error line on stderr with no usage block beside it, and the refusal's structured error document, with its `fix`, on stdout.

### A3: Output is machine-readable when piped

**Given** the fixture.
**When** `spex profile show | cat` and a successful `spex node add | cat` are run.
**Then** each stdout is one compact JSON document — the profile, and the write report with its `obligations` key — parseable by `jq` with no prose around it; the same runs on a terminal pretty-print the same documents.

### A4: A run is deterministic

**Given** two byte-identical copies of the fixture.
**When** N8's `spex node rename` is run over each.
**Then** the two trees are byte-identical to each other afterwards, and the two stdouts are byte-identical.

## Edge cases

### A5: The retired name of a rename reaches stdout as data

**Given** the fixture.
**When** N8's `spex node rename` is run with stdout piped.
**Then** the write report carries a `retired_name` key holding `Comp1`, so a caller feeding the vocabulary sweep reads it from the document rather than from prose.

### A6: --spec-dir is honoured on every surface

**Given** two fixtures at different paths.
**When** each of the eight surfaces is run with `--spec-dir` naming the second.
**Then** the first fixture is byte-identical before and after every run.
