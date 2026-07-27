# RootCommand

The top-level `spex` cobra command. All other subcommands are children of this command.

## Responsibilities

- Define the root `cobra.Command` with the binary name `spex`, a short description, and long usage text.
- Export a single constructor, `NewRootCmd() *cobra.Command`, that returns the configured root with no subcommands attached. Attaching them is the binary's job, not the root's.
- Set global persistent flags (e.g., `--spec-dir` to override the default `spec/` directory).
- When invoked with no args, print help.
- When invoked with an unknown subcommand, cobra's built-in "did you mean?" suggestion fires automatically.
- Add the built-in `completion` subcommand for bash, zsh, and fish shell completions.

## Subcommand Registration Pattern

Every subcommand is defined in the `cmd/spex` package — one file per command (`cmd/spex/diff.go`, `cmd/spex/emit.go`, `cmd/spex/ingest.go`, and so on), each exporting nothing and providing an unexported `newXxxCmd() *cobra.Command` constructor. The worker packages hold no command definitions at all.

Wiring happens in `cmd/spex/main.go`:

```
rootCmd := cli.NewRootCmd()
rootCmd.AddCommand(
    newHashIDCmd(),
    newDiffCmd(),
    newValidateCmd(),
    newImpactCmd(),
    newMapCmd(),
    newRegisterCmd(),
    newLogCmd(),
    newTemplateCmd(),
    newVersionCmd(),
    newRenderCmd(),
    newEmitCmd(),
    newIngestCmd(),
)
rootCmd.Execute()
```

The root command imports no worker package; `cmd/spex` imports both the root and the workers, and is the only place the two meet. Keeping the constructors unexported in a `main` package is what makes that boundary unforgeable — nothing outside the binary can reach a subcommand constructor, so no worker package can grow a CLI dependency by accident.

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

cobra is confined to exactly two packages — `cli` and `cmd/spex`. Under the Subcommand Registration Pattern above, no other package imports it. What this boundary fixes is *which* third-party module may be reached for and *how far* it reaches:

- `cli/root.go` imports cobra and nothing else — no standard library, no internal package.
- cobra stops at command construction, all of which happens under `cmd/spex`. The packages that do the work — validator, merkle, impact, emit, ingest, mapping, proposal, render, schema — import no CLI framework at all; a subcommand's `RunE` reads flags into plain Go values and calls into them.
- pflag and mousetrap arrive transitively through cobra and are never imported directly.

A subcommand needing something cobra does not provide uses the standard library; anything else amends the project's `Declared stack` requirement first, which enumerates the permitted modules against which `go.mod`'s direct requires are read.

## Design Rationale

cobra is the de facto standard for Go CLIs (kubectl, docker, hugo). It provides declarative subcommand registration, auto-generated help, POSIX flag parsing via pflag, and shell completions with zero custom code. This is the one sanctioned exception to the standard-library-first rule of the declared stack — reimplementing this infrastructure would add complexity without value — and the Dependency Boundary above is what keeps the exception from widening.
