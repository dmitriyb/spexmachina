package main

import (
	"github.com/dmitriyb/spexmachina/author"
	"github.com/spf13/cobra"
)

// newEdgeCmd is the `spex edge` grouping: add and remove, no RunE of its
// own, so a bare `spex edge` prints this grouping's help and exits 0.
func newEdgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "Add or remove one entry of one reference field",
	}
	cmd.AddCommand(newEdgeAddCmd(), newEdgeRemoveCmd())
	return cmd
}

// newEdgeAddCmd is `spex edge add`: a source id, a field name and a target
// id, all positional — flow_authoring.md's "Into a worker": "for edges, a
// source id, a field name and a target id."
func newEdgeAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <source-id> <field> <target-id>",
		Short: "Add one entry to one reference field of one node",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdgeAddE(cmd, args[0], args[1], args[2])
		},
	}
}

func runEdgeAddE(cmd *cobra.Command, sourceID, field, targetID string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.AddEdge(specDir, author.EdgeInput{
		SourceID: sourceID,
		Field:    field,
		TargetID: targetID,
	})
	return finishAuthorResult("edge add", report, refusals, err)
}

// newEdgeRemoveCmd is `spex edge remove`, the mirror of add.
func newEdgeRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <source-id> <field> <target-id>",
		Short: "Remove one entry from one reference field of one node",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdgeRemoveE(cmd, args[0], args[1], args[2])
		},
	}
}

func runEdgeRemoveE(cmd *cobra.Command, sourceID, field, targetID string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.RemoveEdge(specDir, author.EdgeInput{
		SourceID: sourceID,
		Field:    field,
		TargetID: targetID,
	})
	return finishAuthorResult("edge remove", report, refusals, err)
}
