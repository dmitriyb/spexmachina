package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// TODO(bead:spexmachina-y0wc.21): MapCommand's get/list/context RunE bodies
// below wired mapping.NewFileStore + mapping.ResolveContext, both retired
// by spexmachina-y0wc.19's migration of MappingStore onto the journal
// (spec/.history.jsonl). Rewrite against mapping.NewMappingStore's
// Get/List and the (not-yet-migrated) ContextResolver per
// spec/map/arch_map_command.md — including dropping the retired
// --map-file flag, since the journal's location is a function of
// --spec-dir alone (see spec/map/arch_mapping_store.md "File Location").

func newMapCmd() *cobra.Command {
	mapCmd := &cobra.Command{
		Use:   "map",
		Short: "Manage bead mapping records",
	}

	getCmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get one node's journal linkage by identity hash or task id",
		Args:  cobra.ExactArgs(1),
		RunE:  runMapGetE,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List the folded node-to-task linkage",
		RunE:  runMapListE,
	}

	contextCmd := &cobra.Command{
		Use:   "context <key>",
		Short: "Resolve full spec context for a spec node, live or removed",
		Args:  cobra.ExactArgs(1),
		RunE:  runMapContextE,
	}

	mapCmd.AddCommand(getCmd, listCmd, contextCmd)
	return mapCmd
}

func runMapGetE(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("map get: not yet migrated onto the journal-backed MappingStore (spexmachina-y0wc.21)")
}

func runMapListE(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("map list: not yet migrated onto the journal-backed MappingStore (spexmachina-y0wc.21)")
}

func runMapContextE(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("map context: not yet migrated onto the journal-backed MappingStore (spexmachina-y0wc.21)")
}
