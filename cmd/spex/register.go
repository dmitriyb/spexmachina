package main

import (
	"fmt"
	"path/filepath"

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
	cmd.Flags().StringVar(&gitHead, "git-head", "", "git HEAD SHA (7-40 hex chars); caller-supplied, spex never calls git")
	_ = cmd.MarkFlagRequired("git-head")
	return cmd
}

// runRegisterE applies the --git-head pre-flight before anything else: the
// proposal file is not read and Registrar is not reached until the value is
// confirmed to look like a git SHA. gitHeadRe is the shared regex declared
// in emit.go, which enforces the identical flag for `spex emit`.
func runRegisterE(cmd *cobra.Command, args []string, gitHead string) error {
	if !gitHeadRe.MatchString(gitHead) {
		return fmt.Errorf("register: --git-head must be a hex SHA (7-40 chars), got %q", gitHead)
	}

	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	filename, err := proposal.Register(args[0], specDir, gitHead)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "registered: %s\n", filepath.Join(specDir, "proposals", filename))
	return nil
}
