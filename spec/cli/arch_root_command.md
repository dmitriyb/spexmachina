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

## Design Rationale

cobra is the de facto standard for Go CLIs (kubectl, docker, hugo). It provides declarative subcommand registration, auto-generated help, POSIX flag parsing via pflag, and shell completions with zero custom code. This is an intentional exception to the "Go standard library first" constraint — reimplementing this infrastructure would add complexity without value.
