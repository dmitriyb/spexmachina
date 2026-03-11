# Map Command Implementation

## Command Registration

```go
func NewMapCmd(store Store) *cobra.Command {
    cmd := &cobra.Command{
        Use:   "map",
        Short: "Query spec-to-bead mapping records",
    }
    cmd.AddCommand(newMapGetCmd(store))
    cmd.AddCommand(newMapListCmd(store))
    return cmd
}

func NewCheckCmd(store Store, spec SpecGraph) *cobra.Command {
    return &cobra.Command{
        Use:   "check <bead-id>",
        Short: "Run preflight check for a bead",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            result, err := Check(cmd.Context(), store, spec, args[0])
            // ...
        },
    }
}
```

Both commands are registered on the root `spex` command in `cmd/spex/main.go` via the CLI module's subcommand registration framework.

## spex map get

```go
func newMapGetCmd(store Store) *cobra.Command {
    return &cobra.Command{
        Use:  "get <record-id>",
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            id, err := strconv.Atoi(args[0])
            if err != nil {
                return fmt.Errorf("invalid record ID: %s", args[0])
            }
            record, err := store.Get(id)
            if err != nil {
                return err
            }
            return json.NewEncoder(cmd.OutOrStdout()).Encode(record)
        },
    }
}
```

## spex map list

```go
func newMapListCmd(store Store) *cobra.Command {
    return &cobra.Command{
        Use: "list",
        RunE: func(cmd *cobra.Command, args []string) error {
            records, err := store.List()
            if err != nil {
                return err
            }
            return json.NewEncoder(cmd.OutOrStdout()).Encode(records)
        },
    }
}
```

## spex check

Parses the bead ID from the first argument, calls `PreflightChecker.Check`, and encodes the result as JSON. Exit code is set based on the result status:
- "ready" → exit 0
- "blocked" or "stale" → exit 1

## Error Output

All errors are written to stderr as plain text. Structured output goes to stdout only. This allows skills to reliably parse stdout as JSON while still seeing diagnostic errors.
