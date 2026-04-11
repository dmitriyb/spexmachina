# Hash ID Implementation

## Structure

`cmd/spex/hashid.go` — registered as a subcommand of the root `spex` command.

## Implementation

```go
func newHashIDCmd() *cobra.Command {
    var module, nodeType, name string

    cmd := &cobra.Command{
        Use:   "hash-id",
        Short: "Compute identity hash for a spec node",
        RunE: func(cmd *cobra.Command, args []string) error {
            parts, err := buildParts(module, nodeType, name)
            if err != nil {
                return err
            }
            fmt.Println(schema.IdentityHash(parts...))
            return nil
        },
    }

    cmd.Flags().StringVar(&module, "module", "", "Module name")
    cmd.Flags().StringVar(&nodeType, "type", "", "Node type")
    cmd.Flags().StringVar(&name, "name", "", "Node name or title")
    cmd.MarkFlagRequired("type")
    cmd.MarkFlagRequired("name")

    return cmd
}
```

## Part Construction

`buildParts` maps the `--type` flag to the correct identity string parts:

```go
func buildParts(module, nodeType, name string) ([]string, error) {
    switch nodeType {
    case "module":
        return []string{"module", name}, nil
    case "milestone":
        return []string{"milestone", name}, nil
    case "scenario":
        return []string{"test_plan", "scenario", name}, nil
    case "requirement":
        if module == "" {
            return []string{"project", "requirement", name}, nil
        }
        return []string{module, "requirement", name}, nil
    case "component", "impl_section", "data_flow", "test_section":
        if module == "" {
            return nil, fmt.Errorf("--module is required for type %q", nodeType)
        }
        return []string{module, nodeType, name}, nil
    default:
        return nil, fmt.Errorf("unknown type %q; valid types: requirement, component, impl_section, data_flow, test_section, module, milestone, scenario", nodeType)
    }
}
```

The switch is exhaustive. No fallthrough, no default passthrough. Unknown types fail explicitly.
