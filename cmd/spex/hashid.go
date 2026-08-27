package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

func newHashIDCmd() *cobra.Command {
	var module, nodeType, name string

	cmd := &cobra.Command{
		Use:   "hash-id",
		Short: "Compute identity hash for a spec node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			specDir, err := resolveSpecDir(cmd)
			if err != nil {
				return err
			}
			profile, err := schema.ResolveProfile(specDir)
			if err != nil {
				return fmt.Errorf("hash-id: %w", err)
			}
			parts, err := buildIdentityParts(profile, module, nodeType, name)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), schema.IdentityHash(parts...))
			return nil
		},
	}

	cmd.Flags().StringVar(&module, "module", "", "Module name (required for module-scoped node types)")
	cmd.Flags().StringVar(&nodeType, "type", "", `Node type the resolved profile declares, plus the fixed "module" type`)
	cmd.Flags().StringVar(&name, "name", "", "Node name or title")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// buildIdentityParts maps --module/--type/--name onto the identity-string
// parts schema.IdentityHash hashes, validating --type against the node
// types the resolved profile declares rather than a fixed switch. "module"
// is fixed — it is not the profile's to declare, since a module is an
// interior node of the merkle tree rather than vocabulary. A type declared
// for both scopes (requirement, in the default profile) hashes module-scoped
// when --module is given and project-scoped otherwise; a type declared for
// module scope alone requires --module.
func buildIdentityParts(profile *schema.Profile, module, nodeType, name string) ([]string, error) {
	if nodeType == "module" {
		return []string{"module", name}, nil
	}

	var declaredProject, declaredModule bool
	for _, t := range profile.NodeTypes {
		if t.Name != nodeType {
			continue
		}
		switch t.Scope {
		case "project":
			declaredProject = true
		case "module":
			declaredModule = true
		}
	}

	if !declaredProject && !declaredModule {
		return nil, fmt.Errorf("hash-id: unknown type %q; valid types: %s", nodeType, validTypeList(profile))
	}

	if declaredModule && module != "" {
		return []string{module, nodeType, name}, nil
	}
	if declaredProject {
		return []string{"project", nodeType, name}, nil
	}
	return nil, fmt.Errorf("hash-id: --module is required for type %q", nodeType)
}

// validTypeList lists the resolved profile's declared node type names, each
// once even when a name is declared for both scopes, plus the fixed
// "module" type — the same set an unknown --type is rejected against.
func validTypeList(profile *schema.Profile) string {
	seen := make(map[string]bool)
	var names []string
	for _, t := range profile.NodeTypes {
		if seen[t.Name] {
			continue
		}
		seen[t.Name] = true
		names = append(names, t.Name)
	}
	sort.Strings(names)
	names = append(names, "module")
	return strings.Join(names, ", ")
}
