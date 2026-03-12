package main

import (
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		wrapCmd("hash", "Build merkle tree and save snapshot", runHash),
		wrapCmd("diff", "Compute changes between snapshot and current spec", runDiff),
		wrapCmd("validate", "Validate spec directory structure", runValidate),
		wrapCmd("impact", "Compute impact of spec changes on beads", runImpact),
		wrapCmd("apply", "Execute bead actions from impact report", runApply),
		wrapMapCmd(),
		wrapCmd("check", "Validate mapping status for a bead", runCheck),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// wrapCmd creates a cobra command that delegates to an existing run* handler.
// The handler receives all arguments after the subcommand name, preserving
// the original flag.FlagSet parsing within each handler.
func wrapCmd(name, short string, fn func([]string) int) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              short,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code := fn(args); code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

// wrapMapCmd creates the map command with its get/list subcommands.
func wrapMapCmd() *cobra.Command {
	mapCmd := &cobra.Command{
		Use:   "map",
		Short: "Manage bead mapping records",
	}
	mapCmd.AddCommand(
		wrapCmd("get", "Get a mapping record by ID", runMapGet),
		wrapCmd("list", "List all mapping records", runMapList),
	)
	return mapCmd
}
