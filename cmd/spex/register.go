package main

import (
	"fmt"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register <proposal-path>",
		Short: "Register a proposal into spec/proposals/",
		Args:  cobra.ExactArgs(1),
		RunE:  runRegisterE,
	}
	cmd.Flags().String("git-head", "", "git HEAD SHA (caller-supplied, e.g. $(git rev-parse HEAD))")
	return cmd
}

func runRegisterE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	gitHead, err := cmd.Flags().GetString("git-head")
	if err != nil {
		return fmt.Errorf("resolve git-head: %w", err)
	}

	filename, err := proposal.Register(args[0], specDir, gitHead)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "registered: %s\n", filepath.Join(specDir, "proposals", filename))
	return nil
}
