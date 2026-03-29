package main

import (
	"context"
	"os"

	"github.com/dmitriyb/spexmachina/proposal"
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show proposal history and linked bead actions",
		RunE:  runLogE,
	}
	cmd.Flags().Bool("json", false, "output in JSON format")
	return cmd
}

func runLogE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	jsonMode, _ := cmd.Flags().GetBool("json")
	lister := &proposal.CLIBeadLister{Bin: "br"}

	return proposal.ShowHistory(context.Background(), specDir, lister, os.Stdout, jsonMode)
}
