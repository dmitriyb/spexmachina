# Change Proposal: CLI Infrastructure

## Context

The CLI is a bare `switch` statement in `main.go` with no `--help` flags, no per-subcommand usage text, and no `spex version`. As new subcommands are added (`spex map`, `spex check`, and future commands), this approach becomes harder to maintain and worse to use. There is no consistent pattern for registering subcommands, and users get no guidance when they mistype a command or forget flags.

Well-established Go CLI libraries solve all of this out of the box: subcommand registration, auto-generated help, flag parsing, shell completions. This proposal introduces `cobra` as the CLI framework and migrates existing subcommands to it.

## Proposed change

### New module: CLI

Cross-cutting CLI infrastructure. Every functional module registers its subcommands through this module.

**Library choice: `cobra`**

`cobra` is the de facto standard for Go CLIs (used by kubectl, docker, hugo). It provides:
- Declarative subcommand registration
- Auto-generated `--help` for every subcommand
- POSIX-style flag parsing via `pflag`
- Shell completions (bash, zsh, fish) for free
- Consistent error handling patterns

This is an exception to the "Go standard library first" constraint. The alternative — reimplementing subcommand registration, help formatting, and flag parsing — adds complexity without value.

**Components:**

| Component | Purpose |
|-----------|---------|
| RootCommand | Top-level `spex` command. Registers all subcommands, sets global flags, prints help when invoked with no args. |
| VersionCommand | `spex version` — prints version and build info (injected via `ldflags` at build time). |

**Migration:**
- Each existing subcommand (validate, merkle, impact, apply, proposal, render) becomes a `cobra.Command` registered on the root.
- Future subcommands (map, check) follow the same pattern.
- The `switch` statement in `main.go` is replaced by `rootCmd.Execute()`.

**Subcommand registration pattern:**
Each module owns its command definition (e.g. `merkle/cmd.go` defines `merkleCmd`) and exposes a function to register it. The root command imports and wires them in `main.go`. This keeps command definitions close to the code they invoke.

**What users get:**
- `spex --help` — lists all subcommands with descriptions
- `spex validate --help` — shows validate-specific flags and usage
- `spex version` — prints version string
- Misspelled commands get "did you mean?" suggestions
- Shell completions via `spex completion bash|zsh|fish`

## Impact expectation

**New beads:**
- CLI module: 2 components (RootCommand, VersionCommand) + requirements, impl sections, tests.

**Modified beads:**
- Every existing command module (validate, merkle, impact, apply, proposal, render) — migrate from switch-case to cobra subcommand registration. These are small, mechanical changes per module.
