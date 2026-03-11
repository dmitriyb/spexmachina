# Root Command Tests

Integration and acceptance tests for the RootCommand component.

## Setup

- Build the `spex` binary with `go build -o bin/spex ./cmd/spex/`.
- Tests can also use `cmd.Execute()` programmatically by constructing the root command in-process.

## Scenarios

### 1. No arguments prints help

**Input**: `spex` (no args)
**Expected**: Exit 0. Stdout contains "Usage:" and lists all registered subcommands (validate, merkle, impact, apply, proposal, render, version, completion).

### 2. --help flag prints help

**Input**: `spex --help`
**Expected**: Exit 0. Same output as no-args case.

### 3. Unknown subcommand suggests alternatives

**Input**: `spex valdate` (typo)
**Expected**: Exit 1. Stderr contains "unknown command" and "Did you mean" with "validate" as a suggestion.

### 4. Global --spec-dir flag is inherited

**Input**: `spex validate --spec-dir ./testdata/valid-spec`
**Expected**: The validate subcommand receives `./testdata/valid-spec` as the spec directory, not the default `spec/`.

### 5. All subcommands registered

**Input**: `spex --help`
**Expected**: Output lists all expected subcommands. Verify by checking that each of `validate`, `merkle`, `impact`, `apply`, `proposal`, `render`, `version`, `completion` appears in the output.

### 6. Subcommand --help works

**Input**: `spex validate --help`
**Expected**: Exit 0. Stdout contains validate-specific usage text and flags.

## Edge Cases

- **Empty argv**: Should not panic. Cobra handles this gracefully by printing help.
- **Flags before subcommand**: `spex --spec-dir ./foo validate` — cobra parses persistent flags regardless of position relative to the subcommand.
- **Double-dash**: `spex -- validate` — cobra treats "validate" as an argument to the root command, not a subcommand. Root command should handle this gracefully (print help or error).
