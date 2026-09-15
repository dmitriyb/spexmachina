package main

import (
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/author"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Inspect the resolved spec profile",
	}
	cmd.AddCommand(newProfileShowCmd())
	return cmd
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved profile as JSON",
		Args:  cobra.NoArgs,
		RunE:  runProfileShowE,
	}
}

// runProfileShowE is ProfileInspector's CLI entry point: it hands the
// resolved spec directory to author.ShowProfile and nothing else — no
// pre-flight against .spex/, no obligations, no write — per
// spec/author/arch_profile_inspector.md "What it does not do".
func runProfileShowE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	if _, err := author.ShowProfile(specDir, os.Stdout, isTTY); err != nil {
		return fmt.Errorf("profile show: %w", err)
	}
	return nil
}
