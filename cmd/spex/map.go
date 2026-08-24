package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dmitriyb/spexmachina/lifecycle"
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

// resolveMapProject reads --spec-dir and runs the lifecycle pre-flight
// before any child reads the journal, per spec/map/arch_map_command.md
// "The journal's location comes from the lifecycle pre-flight" and
// spec/map/test_map_command.md "No journal exists": a query surface fails
// loudly rather than folding the ambiguity between "never initialised"
// and "broken" into one silent empty answer. The MappingStore library
// layer is untouched by this check and keeps folding an absent journal to
// empty for any caller that bypasses this pre-flight.
func resolveMapProject(cmd *cobra.Command) (specDir string, ctx *lifecycle.ProjectContext, err error) {
	specDir, err = resolveSpecDir(cmd)
	if err != nil {
		return "", nil, err
	}
	ctx, err = lifecycle.Resolve(resolveProjectRoot(specDir))
	if err != nil {
		return "", nil, fmt.Errorf("map: %w", err)
	}
	return specDir, ctx, nil
}

func runMapGetE(cmd *cobra.Command, args []string) error {
	_, ctx, err := resolveMapProject(cmd)
	if err != nil {
		return err
	}
	store := mapping.NewMappingStore(ctx.JournalPath)
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
	_, ctx, err := resolveMapProject(cmd)
	if err != nil {
		return err
	}
	store := mapping.NewMappingStore(ctx.JournalPath)
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
	specDir, ctx, err := resolveMapProject(cmd)
	if err != nil {
		return err
	}
	result, err := mapping.ResolveContext(specDir, ctx.JournalPath, args[0])
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
