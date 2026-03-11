package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/merkle"
)

func runDiff(args []string) int {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	snapshotFlag := fs.String("snapshot", "", "path to snapshot file (default: <dir>/.snapshot.json)")
	jsonOut := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex diff: %v\n", err)
		return 1
	}

	specDir := "spec"
	if fs.NArg() > 0 {
		specDir = fs.Arg(0)
	}

	specDir, err := filepath.Abs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex diff: resolve path: %v\n", err)
		return 1
	}

	current, err := merkle.BuildTree(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex diff: %v\n", err)
		return 1
	}

	snapshotPath := *snapshotFlag
	if snapshotPath == "" {
		snapshotPath = filepath.Join(specDir, ".snapshot.json")
	}

	var snapshot *merkle.Node
	if _, statErr := os.Stat(snapshotPath); statErr == nil {
		snapshot, err = merkle.Load(snapshotPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex diff: %v\n", err)
			return 1
		}
	}

	changes := merkle.Diff(current, snapshot)
	moduleNames := merkle.ModuleNames(current)
	classified := merkle.Classify(changes, moduleNames)

	if *jsonOut {
		return printDiffJSON(classified)
	}
	printDiffSummary(classified)
	return 0
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
	Total      int            `json:"total"`
	ByType     map[string]int `json:"by_type"`
	ByImpact   map[string]int `json:"by_impact"`
}

func printDiffJSON(classified []merkle.ClassifiedChange) int {
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
		fmt.Fprintf(os.Stderr, "spex diff: marshal json: %v\n", err)
		return 1
	}
	fmt.Println(string(data))
	return 0
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
