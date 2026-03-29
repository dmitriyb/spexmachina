package main

import (
	"os"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newTemplateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "template <project|change>",
		Short: "Output a proposal template to stdout",
		Args:  cobra.ExactArgs(1),
		RunE:  runTemplateE,
	}
}

func runTemplateE(cmd *cobra.Command, args []string) error {
	return proposal.Template(args[0], os.Stdout)
}
