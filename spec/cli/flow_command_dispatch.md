# Command Dispatch

How user input flows from argv through cobra to the correct subcommand handler.

## Flow

```
User types: spex validate --spec-dir ./myspec
                │
                ▼
        ┌───────────────┐
        │   os.Args      │
        │ ["spex",       │
        │  "validate",   │
        │  "--spec-dir", │
        │  "./myspec"]   │
        └───────┬───────┘
                │
                ▼
        ┌───────────────┐
        │  RootCommand   │
        │  Execute()     │
        │                │
        │ 1. Parse       │
        │    persistent  │
        │    flags       │
        │ 2. Match       │
        │    subcommand  │
        │    "validate"  │
        └───────┬───────┘
                │
                ▼
        ┌───────────────┐
        │  validate.Cmd  │
        │  RunE()        │
        │                │
        │ 1. Parse local │
        │    flags       │
        │ 2. Read        │
        │    --spec-dir  │
        │    from parent │
        │ 3. Execute     │
        │    validation  │
        └───────────────┘
```

## Key Behaviors

1. **No args**: `spex` with no arguments prints help text listing all subcommands.
2. **Unknown subcommand**: `spex foobar` prints "unknown command 'foobar'" with "Did you mean...?" suggestions from cobra.
3. **Global flags**: `--spec-dir` is a persistent flag on the root, inherited by all subcommands. Each subcommand reads it via `cmd.Root().PersistentFlags().GetString("spec-dir")` or through cobra's flag inheritance.
4. **Subcommand help**: `spex validate --help` prints validate-specific usage, flags, and examples.
5. **Error handling**: Subcommand `RunE` functions return errors. The root `Execute()` propagates the error to `main()`, which prints it to stderr and exits with code 1.

## Completions

cobra auto-generates a `completion` subcommand. Users run:

```sh
spex completion bash > /etc/bash_completion.d/spex
spex completion zsh > "${fpath[1]}/_spex"
spex completion fish > ~/.config/fish/completions/spex.fish
```

No custom code is needed — cobra generates completions from the registered command tree.

## Data Shapes

### argv → RootCommand

- os.Args: list of string (as provided by the Go runtime)
- Cobra parses this into:
  - PersistentFlags: map[string]string (e.g., `spec-dir`)
  - Subcommand path: list of string (always length 1 in spex today)
  - LocalFlags: map[string]string (per-subcommand)
  - PositionalArgs: list of string

### RootCommand → Subcommand handler (RunE signature)

- cobra.Command receiver with:
  - Flags() / PersistentFlags() accessors
  - Root() for persistent flag lookup
- Positional args: `args []string`
- Return: `error` — nil for success; non-nil propagates to main()

### Subcommand → stderr / stdout (exit contract)

- stdout: subcommand-specific payload (JSON, text, DOT, etc.)
- stderr: error messages + optional progress logs
- exit code: 0 on RunE returning nil, 1 otherwise

Any new persistent flag on RootCommand must be documented here. Any new
subcommand must be registered via the CommandRegistrar pattern — drive-by
additions outside that pattern break shell completion and help output.
