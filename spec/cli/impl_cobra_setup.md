# Cobra Setup and Registration

How the root command is constructed and how subcommands are registered.

## Package Layout

The CLI module lives in `internal/cli/` (or `cmd/spex/cli/` depending on project convention). It exports:

- `NewRootCmd() *cobra.Command` — constructs and returns the root command.
- `NewVersionCmd() *cobra.Command` — constructs and returns the version subcommand.

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

The existing `main.go` switch statement:

```go
switch os.Args[1] {
case "validate":
    // ...
case "merkle":
    // ...
}
```

is replaced by:

```go
func main() {
    rootCmd := cli.NewRootCmd()
    rootCmd.AddCommand(
        validate.NewCmd(),
        merkle.NewCmd(),
        // ...
    )
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

Each module's `NewCmd()` function defines its flags and run function locally. The migration is mechanical: move the existing handler code into a `RunE` function on the cobra command.
