# Map Command Implementation

## Command Registration

```go
func newMapCmd() *cobra.Command {
    mapCmd := &cobra.Command{
        Use:   "map",
        Short: "Manage bead mapping records",
    }
    // ... getCmd, listCmd, contextCmd setup ...
    mapCmd.AddCommand(getCmd, listCmd, contextCmd)
    return mapCmd
}

```

The map command is registered on the root `spex` command in `cmd/spex/main.go` via the CLI module's subcommand registration framework.

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

## spex map context

```go
func runMapContextE(cmd *cobra.Command, args []string) error {
    specDir, err := resolveSpecDir(cmd)
    if err != nil {
        return err
    }

    mapFile, _ := cmd.Flags().GetString("map-file")

    id, err := strconv.Atoi(args[0])
    if err != nil {
        return fmt.Errorf("map context: invalid record ID: %s", args[0])
    }

    store := mapping.NewFileStore(mapFile)
    record, err := store.Get(id)
    if err != nil {
        return fmt.Errorf("map context: %w", err)
    }

    result, err := mapping.ResolveContext(specDir, record)
    if err != nil {
        return fmt.Errorf("map context: %w", err)
    }

    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    if err := enc.Encode(result); err != nil {
        return fmt.Errorf("map context: %w", err)
    }
    return nil
}
```

Resolves the full spec context for a component by looking up the mapping record, then calling `mapping.ResolveContext` which reads `spec/<module>/module.json` and collects all arch, impl, test, and flow files relevant to that component.

## Error Output

All errors are written to stderr as plain text. Structured output goes to stdout only. This allows skills to reliably parse stdout as JSON while still seeing diagnostic errors.
