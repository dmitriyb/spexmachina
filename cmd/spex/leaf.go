package main

import (
	"github.com/dmitriyb/spexmachina/author"
	"github.com/spf13/cobra"
)

// newLeafCmd is the `spex leaf` grouping: scaffold alone today, no RunE of
// its own, so a bare `spex leaf` prints this grouping's help and exits 0.
func newLeafCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leaf",
		Short: "Scaffold spec content leaves",
	}
	cmd.AddCommand(newLeafScaffoldCmd())
	return cmd
}

// newLeafScaffoldCmd is `spex leaf scaffold`, LeafScaffolder's surface: an
// existing content-bearing node named by id.
func newLeafScaffoldCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scaffold <id>",
		Short: "Write the content leaf skeleton of an existing node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLeafScaffoldE(cmd, args[0])
		},
	}
}

func runLeafScaffoldE(cmd *cobra.Command, id string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	report, refusals, err := author.Scaffold(specDir, author.ScaffoldInput{ID: id})
	return finishAuthorResult("leaf scaffold", report, refusals, err)
}
