# RootCommand

The top-level `spex` cobra command. All other subcommands are children of this command.

## Responsibilities

- Define the root `cobra.Command` with the binary name `spex`, a short description, and long usage text.
- Provide a `Register(cmd *cobra.Command)` function (or equivalent) that each module calls to add its subcommand.
- Set global persistent flags (e.g., `--spec-dir` to override the default `spec/` directory).
- When invoked with no args, print help.
- When invoked with an unknown subcommand, cobra's built-in "did you mean?" suggestion fires automatically.
- Add the built-in `completion` subcommand for bash, zsh, and fish shell completions.

## Subcommand Registration Pattern

Each functional module defines its own `cobra.Command` in a `cmd.go` file within its package (e.g., `merkle/cmd.go` exports `NewCmd() *cobra.Command`). The root command does not import module internals — it only calls each module's command constructor and adds the result via `rootCmd.AddCommand(...)`.

Wiring happens in `main.go`:

```
rootCmd := cli.NewRootCmd()
rootCmd.AddCommand(
    validate.NewCmd(),
    merkle.NewCmd(),
    impact.NewCmd(),
    apply.NewCmd(),
    proposal.NewCmd(),
    render.NewCmd(),
    cli.NewVersionCmd(),
)
rootCmd.Execute()
```

This keeps command definitions close to the code they invoke and avoids circular imports.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--spec-dir` | string | `spec/` | Path to the spec directory |

Global flags are defined as persistent flags on the root command so they are available to all subcommands.

## Dependency Boundary

cobra is the only third-party CLI framework in the binary. No second command library and no hand-rolled parser stands beside it, and every capability the CLI framework owes the user is taken from a cobra built-in rather than added alongside it:

| Capability | Source |
|------------|--------|
| Argument and flag parsing | cobra, via pflag |
| Help and usage text | cobra's generated help |

cobra is not confined to a single import site. Under the Subcommand Registration Pattern above, every package that defines a subcommand constructs its own `cobra.Command` and therefore imports cobra; the root command is only the first of them. What this boundary fixes is *which* third-party module may be reached for and *how far* it reaches:

- `cli/root.go` imports cobra and nothing else — no standard library, no internal package.
- cobra stops at command construction. The packages that do the work — validator, merkle, impact, emit, ingest, mapping, render, schema — import no CLI framework at all; a subcommand's `RunE` reads flags into plain Go values and calls into them.
- pflag and mousetrap arrive transitively through cobra and are never imported directly.

A subcommand needing something cobra does not provide uses the standard library; anything else amends the project's `Declared stack` requirement first, which enumerates the permitted modules against which `go.mod`'s direct requires are read.

## Design Rationale

cobra is the de facto standard for Go CLIs (kubectl, docker, hugo). It provides declarative subcommand registration, auto-generated help, POSIX flag parsing via pflag, and shell completions with zero custom code. This is the one sanctioned exception to the standard-library-first rule of the declared stack — reimplementing this infrastructure would add complexity without value — and the Dependency Boundary above is what keeps the exception from widening.
