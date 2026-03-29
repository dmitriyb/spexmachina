package main

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newRegisterCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "register <proposal-path>",
		Short: "Register a proposal into spec/proposals/",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegisterE,
	}
}

func runRegisterE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	filename, err := proposal.Register(args[0], specDir)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "registered: spec/proposals/%s\n", filename)
	return nil
}
