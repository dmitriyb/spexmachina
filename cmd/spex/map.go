package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/spf13/cobra"
)

// mapEntryView flattens a mapping.FoldEntry into the JSON shape
// spec/map/arch_map_command.md documents for `spex map get`/`spex map
// list` — the node or proposal-epic key, the current task id, and the
// sourcing event's descriptive fields, all at the top level.
type mapEntryView struct {
	Node     string `json:"node,omitempty"`
	Proposal string `json:"proposal,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Name     string `json:"name,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	Module   string `json:"module,omitempty"`
	GitHead  string `json:"git_head,omitempty"`
	Removed  bool   `json:"removed,omitempty"`
}

func newMapEntryView(e mapping.FoldEntry) mapEntryView {
	v := mapEntryView{
		TaskID:   e.TaskID,
		Name:     e.Source.Name,
		NodeType: e.Source.NodeType,
		Module:   e.Source.Module,
		GitHead:  e.Source.GitHead,
		Proposal: e.Source.Proposal,
		Removed:  e.Removed,
	}
	// A proposal-epic entry's Source is the task_created receipt itself
	// (no Node); a node entry's Source is its change event.
	if e.Source.Node != "" {
		v.Node = e.Key
	}
	return v
}

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
		Short: "Resolve full spec context for a node, live or removed, by identity hash or task id",
		Args:  cobra.ExactArgs(1),
		RunE:  runMapContextE,
	}

	mapCmd.AddCommand(getCmd, listCmd, contextCmd)
	return mapCmd
}

func runMapGetE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	// TODO(bead:spexmachina-uiei.8): resolve the journal location through
	// ProjectResolver instead of joining specDir here, once it lands.
	store := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl"))
	entry, err := store.Get(args[0])
	if err != nil {
		return fmt.Errorf("map: %w", err)
	}

	if err := json.NewEncoder(os.Stdout).Encode(newMapEntryView(entry)); err != nil {
		return fmt.Errorf("map: %w", err)
	}
	return nil
}

func runMapListE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	// TODO(bead:spexmachina-uiei.8): resolve the journal location through
	// ProjectResolver instead of joining specDir here, once it lands.
	store := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl"))
	fold, err := store.List()
	if err != nil {
		return fmt.Errorf("map: %w", err)
	}

	views := make([]mapEntryView, len(fold.Entries))
	for i, e := range fold.Entries {
		views[i] = newMapEntryView(e)
	}
	if err := json.NewEncoder(os.Stdout).Encode(views); err != nil {
		return fmt.Errorf("map: %w", err)
	}
	return nil
}

func runMapContextE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	result, err := mapping.ResolveContext(specDir, args[0])
	if err != nil {
		return fmt.Errorf("map context: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("map context: %w", err)
	}
	return nil
}
