package main

import (
	"fmt"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newRegisterCmd() *cobra.Command {
	var gitHead string

	cmd := &cobra.Command{
		Use:   "register <proposal-path>",
		Short: "Register a proposal into spec/proposals/",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegisterE(cmd, args, gitHead)
		},
	}
	cmd.Flags().StringVar(&gitHead, "git-head", "", "git HEAD SHA (caller-supplied, 7-40 hex chars)")
	_ = cmd.MarkFlagRequired("git-head")
	return cmd
}

func runRegisterE(cmd *cobra.Command, args []string, gitHead string) error {
	if err := validateRegisterGitHead(gitHead); err != nil {
		return err
	}

	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	ctx, err := lifecycle.Resolve(resolveProjectRoot(specDir))
	if err != nil {
		return fmt.Errorf("register: %w", err)
	}

	filename, err := proposal.Register(args[0], specDir, ctx.JournalPath, gitHead)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "registered: %s\n", filepath.Join(specDir, "proposals", filename))
	return nil
}

// validateRegisterGitHead applies the same pre-flight `spex plan` uses for
// --git-head: required, matching gitHeadRe. It runs before the proposal
// file is read and before Registrar is reached, so a missing or malformed
// head leaves neither the journal append nor the copy behind.
func validateRegisterGitHead(s string) error {
	if !gitHeadRe.MatchString(s) {
		return fmt.Errorf("register: --git-head must be a hex SHA (7-40 chars), got %q", s)
	}
	return nil
}
