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
	completenessErrors := merkle.CheckCompleteness(classified, specDir)

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		if err := printDiffJSON(classified, completenessErrors); err != nil {
			return err
		}
	} else {
		printDiffSummary(classified, completenessErrors)
	}

	// A non-empty errors array is a pipeline halt signal: the full diff is
	// already on stdout, but the non-zero exit tells callers not to pipe this
	// into `spex impact`. See arch_diff_command.md "Exit codes".
	if len(completenessErrors) > 0 {
		return &diffError{
			code: 2,
			err:  fmt.Errorf("diff: %d completeness error(s) found", len(completenessErrors)),
		}
	}
	return nil
}

// diffError carries a process exit code alongside the wrapped error. main
// inspects the ExitCode interface to honour the codes documented in
// arch_diff_command.md (1 for IO/parse failure, 2 for a non-empty errors
// array).
type diffError struct {
	code int
	err  error
}

func (e *diffError) Error() string { return e.err.Error() }
func (e *diffError) Unwrap() error { return e.err }
func (e *diffError) ExitCode() int { return e.code }

// diffOutput is the JSON representation of the diff command result.
type diffOutput struct {
	Changes []diffChange    `json:"changes"`
	Errors  []merkle.DiffError `json:"errors"`
	Summary diffSummary     `json:"summary"`
}

type diffChange struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Impact   string `json:"impact"`
	Module   string `json:"module"`
	NodeType string `json:"node_type,omitempty"`
	OldHash  string `json:"old_hash,omitempty"`
	NewHash  string `json:"new_hash,omitempty"`
}

type diffSummary struct {
	Total    int            `json:"total"`
	ByType   map[string]int `json:"by_type"`
	ByImpact map[string]int `json:"by_impact"`
}

func printDiffJSON(classified []merkle.ClassifiedChange, errors []merkle.DiffError) error {
	out := diffOutput{
		Changes: make([]diffChange, len(classified)),
		Errors:  errors,
		Summary: diffSummary{
			Total:    len(classified),
			ByType:   make(map[string]int),
			ByImpact: make(map[string]int),
		},
	}
	if out.Errors == nil {
		out.Errors = []merkle.DiffError{}
	}

	for i, cc := range classified {
		out.Changes[i] = diffChange{
			Path:     cc.Key,
			Type:     cc.Type.String(),
			Impact:   cc.Impact.String(),
			Module:   cc.Module,
			NodeType: cc.NodeType,
			OldHash:  cc.OldHash,
			NewHash:  cc.NewHash,
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

func printDiffSummary(classified []merkle.ClassifiedChange, errors []merkle.DiffError) {
	if len(classified) == 0 && len(errors) == 0 {
		fmt.Println("no changes")
		return
	}

	for _, cc := range classified {
		fmt.Printf("%-10s %-12s %-10s %s\n", cc.Type, cc.Impact, cc.Module, cc.Key)
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
	for _, imp := range []string{"impl_only", "contract", "arch_impl", "structural"} {
		if c, ok := byImpact[imp]; ok {
			fmt.Printf("  %d %s\n", c, imp)
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\n%d error(s):\n", len(errors))
		for _, e := range errors {
			fmt.Printf("  error: [%s] %s\n", e.Type, e.Message)
			if e.Path != "" {
				fmt.Printf("    path: %s\n", e.Path)
			}
			for _, r := range e.Related {
				fmt.Printf("    related: %s\n", r)
			}
		}
	}
}
