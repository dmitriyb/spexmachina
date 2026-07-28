package main

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

func newHashIDCmd() *cobra.Command {
	var module, nodeType, name string

	cmd := &cobra.Command{
		Use:   "hash-id",
		Short: "Compute identity hash for a spec node",
		RunE: func(cmd *cobra.Command, args []string) error {
			parts, err := buildIdentityParts(module, nodeType, name)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), schema.IdentityHash(parts...))
			return nil
		},
	}

	cmd.Flags().StringVar(&module, "module", "", "Module name (required for module-scoped node types)")
	cmd.Flags().StringVar(&nodeType, "type", "", "Node type: requirement, component, data_flow, test_section, api, module")
	cmd.Flags().StringVar(&name, "name", "", "Node name or title")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func buildIdentityParts(module, nodeType, name string) ([]string, error) {
	switch nodeType {
	case "module":
		return []string{"module", name}, nil
	case "requirement":
		if module == "" {
			return []string{"project", "requirement", name}, nil
		}
		return []string{module, "requirement", name}, nil
	case "component", "data_flow", "test_section", "api":
		if module == "" {
			return nil, fmt.Errorf("hash-id: --module is required for type %q", nodeType)
		}
		return []string{module, nodeType, name}, nil
	default:
		return nil, fmt.Errorf("hash-id: unknown type %q; valid types: requirement, component, data_flow, test_section, api, module", nodeType)
	}
}
