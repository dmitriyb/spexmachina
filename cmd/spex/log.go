package main

import (
	"fmt"

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

// TODO(bead:spexmachina-0lk.21): rewire after spexmachina-0lk.20 retired
// CLIBeadLister and changed proposal.HistoryViewer to a struct that consumes a
// pre-parsed []BeadRecord. The new wiring (read stdin → parse → hand to
// HistoryViewer) is owned by the ProposalCommands bead.
func runLogE(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("spex log: pending ProposalCommands rewire (spexmachina-0lk.21)")
}
