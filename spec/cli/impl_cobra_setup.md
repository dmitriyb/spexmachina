# Cobra Setup and Registration

How the root command is constructed and how subcommands are registered.

## Package Layout

Package `cli` lives at `cli/` in the repository root and holds one file, `cli/root.go`. It exports exactly one symbol:

- `NewRootCmd() *cobra.Command` — constructs and returns the root command, with global persistent flags set and **no** subcommands attached.

Subcommands do not live here. Each is an unexported `newXxxCmd() *cobra.Command` constructor in a file of its own under `cmd/spex/` — `cmd/spex/validate.go`, `cmd/spex/diff.go`, `cmd/spex/version.go` and so on. See `arch_root_command.md`, "Subcommand Registration Pattern".

## Root Command Construction

```go
func NewRootCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "spex",
        Short: "The spec state machine",
        Long:  "spex owns the structural half of spec-driven development...",
        SilenceUsage:  true,
        SilenceErrors: true,
    }

    cmd.PersistentFlags().StringP("spec-dir", "s", "spec/", "path to the spec directory")

    return cmd
}
```

### SilenceUsage and SilenceErrors

Both are set to `true` so that cobra does not print usage on every error. Errors are handled explicitly — the root command's `Execute()` return value is checked in `main.go`, and errors are printed to stderr with `fmt.Fprintln(os.Stderr, err)` before calling `os.Exit(1)`.

## Dependency: cobra

Add `github.com/spf13/cobra` to `go.mod`. This pulls in `github.com/spf13/pflag` transitively. No other external dependencies are introduced.

## Migration Path

This migration is done; the shape it replaced is recorded here because the reasoning still applies to any future subcommand. The pre-cobra `main.go` dispatched on argv directly:

```go
switch os.Args[1] {
case "validate":
    // ...
case "merkle":
    // ...
}
```

is replaced by a `main` that builds the root, attaches every `cmd/spex` constructor to it in one `AddCommand` call, and maps the returned error onto an exit code:

```
func main() {
    rootCmd := cli.NewRootCmd()
    rootCmd.AddCommand(
        newHashIDCmd(), newDiffCmd(), newValidateCmd(), newImpactCmd(),
        newMapCmd(), newRegisterCmd(), newLogCmd(), newTemplateCmd(),
        newVersionCmd(), newRenderCmd(), newEmitCmd(), newIngestCmd(),
    )
    if err := rootCmd.Execute(); err != nil { ... }
}
```

Each constructor declares its own flags and `RunE` in its own `cmd/spex` file; the worker packages (validator, merkle, impact, emit, ingest, mapping, proposal, render, schema) define no commands and import no CLI framework. The migration is mechanical: move the existing handler code into a `RunE` function on the cobra command, and let it call into the worker package rather than live there.
