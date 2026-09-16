package main

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/author"
	"github.com/spf13/cobra"
)

// newNodeCmd is the `spex node` grouping: it registers add, set, remove and
// rename and carries no RunE of its own, so a bare `spex node` prints this
// grouping's help and exits 0 — the same shape `spex profile` already uses
// — per spec/author/arch_author_commands.md, "The nine surfaces": "a
// grouping here owns nothing a child does not."
func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Declare, set, remove or rename spec nodes",
	}
	cmd.AddCommand(newNodeAddCmd(), newNodeSetCmd(), newNodeRemoveCmd(), newNodeRenameCmd())
	return cmd
}

// newNodeAddCmd is `spex node add`, NodeEditor's declare surface. --type is
// the one flag the spec fixes for this surface (a profile-declared type
// name, or the frame value "module"); --module and --field are this
// implementation's own vocabulary choice for the module name and the
// per-field values flow_authoring.md leaves open
// (spec/author/arch_author_commands.md, "Three flags are fixed by the
// spec").
func newNodeAddCmd() *cobra.Command {
	var typeName, module string
	var fields []string

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Declare a node of a profile-declared type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeAddE(cmd, args[0], typeName, module, fields)
		},
	}
	cmd.Flags().StringVar(&typeName, "type", "", "profile-declared node type, or \"module\"")
	cmd.Flags().StringVar(&module, "module", "", "module name, for a module-scoped type")
	cmd.Flags().StringArrayVar(&fields, "field", nil, "a declared field value as key=value; repeatable")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func runNodeAddE(cmd *cobra.Command, name, typeName, module string, rawFields []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}
	fields, err := parseFieldFlags(rawFields)
	if err != nil {
		return fmt.Errorf("node add: %w", err)
	}

	report, refusals, err := author.Add(specDir, author.NodeAddInput{
		TypeName: typeName,
		Name:     name,
		Module:   module,
		Fields:   fields,
	})
	return finishAuthorResult("node add", report, refusals, err)
}

// newNodeSetCmd is `spex node set`, NodeEditor's field-edit surface.
// --field and --unset are the two flags the spec fixes for this surface
// (spec/author/arch_author_commands.md, "Five flags are fixed by the
// spec"): --field name=value, repeatable, replaces a declared
// non-reference field's value; --unset name, repeatable, removes an
// optional one; the two may appear together in one invocation, with a
// name given to both refused by the worker.
func newNodeSetCmd() *cobra.Command {
	var fields []string
	var unset []string

	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Set declared field values on an existing node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeSetE(cmd, args[0], fields, unset)
		},
	}
	cmd.Flags().StringArrayVar(&fields, "field", nil, "a declared field value as key=value; repeatable")
	cmd.Flags().StringArrayVar(&unset, "unset", nil, "a declared optional field name to remove; repeatable")
	return cmd
}

func runNodeSetE(cmd *cobra.Command, id string, rawFields, unset []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}
	fields, err := parseFieldFlags(rawFields)
	if err != nil {
		return fmt.Errorf("node set: %w", err)
	}

	report, refusals, err := author.Set(specDir, author.NodeSetInput{
		ID:     id,
		Fields: fields,
		Unset:  unset,
	})
	return finishAuthorResult("node set", report, refusals, err)
}

// newNodeRemoveCmd is `spex node remove`. --force is the one flag the spec
// fixes for this surface; the node is named by id, per flow_authoring.md's
// "Into a worker": "every id is a 12-character identity hash."
func newNodeRemoveCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a node by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeRemoveE(cmd, args[0], force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "remove over inbound references, listing them as dangling")
	return cmd
}

func runNodeRemoveE(cmd *cobra.Command, id string, force bool) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.Remove(specDir, author.NodeRemoveInput{ID: id, Force: force})
	return finishAuthorResult("node remove", report, refusals, err)
}

// newNodeRenameCmd is `spex node rename`, NodeRenamer's single-transaction
// surface: an id and the new name.
func newNodeRenameCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <id> <new-name>",
		Short: "Rename a node as one transaction",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeRenameE(cmd, args[0], args[1])
		},
	}
	return cmd
}

func runNodeRenameE(cmd *cobra.Command, id, newName string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.Rename(specDir, author.RenameInput{ID: id, NewName: newName})
	return finishAuthorResult("node rename", report, refusals, err)
}
