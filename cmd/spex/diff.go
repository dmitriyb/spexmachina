package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compute changes between snapshot and current spec",
		RunE:  runDiffE,
	}
	cmd.Flags().String("snapshot", "", "path to snapshot file (default: <dir>/.snapshot.json)")
	cmd.Flags().Bool("json", false, "output as JSON")
	return cmd
}

func runDiffE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	current, err := merkle.BuildTree(specDir)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}

	snapshotPath, _ := cmd.Flags().GetString("snapshot")
	if snapshotPath == "" {
		snapshotPath = filepath.Join(specDir, ".snapshot.json")
	}

	var snapshot *merkle.Node
	if _, statErr := os.Stat(snapshotPath); statErr == nil {
		snapshot, err = merkle.Load(snapshotPath)
		if err != nil {
			return fmt.Errorf("diff: %w", err)
		}
	}

	changes := merkle.Diff(current, snapshot)
	moduleNames := merkle.ModuleNames(current)
	classified := merkle.Classify(changes, moduleNames)

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		return printDiffJSON(classified)
	}
	printDiffSummary(classified)
	return nil
}

// diffOutput is the JSON representation of the diff command result.
type diffOutput struct {
	Changes []diffChange `json:"changes"`
	Summary diffSummary  `json:"summary"`
}

type diffChange struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Impact  string `json:"impact"`
	Module  string `json:"module"`
	OldHash string `json:"old_hash,omitempty"`
	NewHash string `json:"new_hash,omitempty"`
}

type diffSummary struct {
	Total    int            `json:"total"`
	ByType   map[string]int `json:"by_type"`
	ByImpact map[string]int `json:"by_impact"`
}

func printDiffJSON(classified []merkle.ClassifiedChange) error {
	out := diffOutput{
		Changes: make([]diffChange, len(classified)),
		Summary: diffSummary{
			Total:    len(classified),
			ByType:   make(map[string]int),
			ByImpact: make(map[string]int),
		},
	}

	for i, cc := range classified {
		out.Changes[i] = diffChange{
			Path:    cc.Path,
			Type:    cc.Type.String(),
			Impact:  cc.Impact.String(),
			Module:  cc.Module,
			OldHash: cc.OldHash,
			NewHash: cc.NewHash,
		}
		out.Summary.ByType[cc.Type.String()]++
		out.Summary.ByImpact[cc.Impact.String()]++
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("diff: marshal json: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printDiffSummary(classified []merkle.ClassifiedChange) {
	if len(classified) == 0 {
		fmt.Println("no changes")
		return
	}

	for _, cc := range classified {
		fmt.Printf("%-10s %-12s %-10s %s\n", cc.Type, cc.Impact, cc.Module, cc.Path)
	}

	byType := make(map[string]int)
	byImpact := make(map[string]int)
	for _, cc := range classified {
		byType[cc.Type.String()]++
		byImpact[cc.Impact.String()]++
	}

	fmt.Printf("\n%d change(s)", len(classified))
	for _, t := range []string{"added", "modified", "removed"} {
		if c, ok := byType[t]; ok {
			fmt.Printf(", %d %s", c, t)
		}
	}
	fmt.Println()
	for _, imp := range []string{"impl_only", "arch_impl", "structural"} {
		if c, ok := byImpact[imp]; ok {
			fmt.Printf("  %d %s\n", c, imp)
		}
	}
}
