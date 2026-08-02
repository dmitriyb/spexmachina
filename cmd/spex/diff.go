package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/validator"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compute changes between snapshot and current spec",
		Args:  cobra.NoArgs,
		RunE:  runDiffE,
	}
	cmd.Flags().String("snapshot", "", "path to snapshot file (default: <dir>/.snapshot.json)")
	cmd.Flags().String("map", ".bead-map.json", "path to bead mapping file, read as a hash -> name source for removal checks")
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

	// Removal-time name checking. It belongs here rather than in `spex
	// validate` because it is the only check in the system that is
	// relative to history: it needs the snapshot to know what was removed,
	// and every validator checker takes a spec directory and nothing else.
	// This is the same shape as CheckCompleteness above — a corpus-wide
	// gate keyed off the classified diff, reported through .errors, and
	// halting the pipeline with exit 2.
	//
	// The bead map is passed as a second hash -> name source: when a whole
	// module is retired, its component records still carry the module name
	// the diff can only report as a hash. It is read, never written, and an
	// absent file is not an error — `spex diff` runs in trees that have
	// never been ingested.
	mapPath, _ := cmd.Flags().GetString("map")
	removed, err := validator.CheckRemovedNames(specDir, mapPath, classified)
	if err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	for _, s := range removed.Survivors {
		completenessErrors = append(completenessErrors, merkle.DiffError{
			Type: "surviving_name",
			Message: fmt.Sprintf("removed %s %q (%s) is still named in the spec corpus at %d site(s); sweep the mentions or restore the node",
				s.NodeType, s.Name, s.Key, len(s.Sites)),
			Path:    s.Sites[0],
			Related: s.Sites,
		})
	}

	// Notes are the sweep's disclosures, not violations: a removal it could
	// not check, or hits it discarded because a live node covers them. They
	// never gate the exit code — a suppressed hit is a correct answer, and
	// after the bead-map fallback an unverifiable one is a state no author
	// action can clear (see unverifiableModuleNote) — but every one of them
	// is a place where "no errors" means less than it looks, so they are
	// printed rather than dropped.
	notes := make([]diffNote, 0, len(removed.Notes))
	for _, n := range removed.Notes {
		notes = append(notes, diffNote{Type: n.Kind, Message: n.Message, Related: n.Keys})
	}

	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		if err := printDiffJSON(classified, completenessErrors, notes); err != nil {
			return err
		}
	} else {
		printDiffSummary(classified, completenessErrors, notes)
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
//
// Notes is omitted when empty so the documented three-key shape (changes,
// errors, summary) is what every clean run emits. It is never a place a
// violation can hide: `errors` remains the only field the exit code reads.
type diffOutput struct {
	Changes []diffChange       `json:"changes"`
	Errors  []merkle.DiffError `json:"errors"`
	Notes   []diffNote         `json:"notes,omitempty"`
	Summary diffSummary        `json:"summary"`
}

// diffNote mirrors merkle.DiffError's shape for a finding that is not a
// failure, so both render the same way in text and in JSON.
type diffNote struct {
	Type    string   `json:"type"`
	Message string   `json:"message"`
	Related []string `json:"related,omitempty"`
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

func printDiffJSON(classified []merkle.ClassifiedChange, errors []merkle.DiffError, notes []diffNote) error {
	out := diffOutput{
		Changes: make([]diffChange, len(classified)),
		Errors:  errors,
		Notes:   notes,
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

func printDiffSummary(classified []merkle.ClassifiedChange, errors []merkle.DiffError, notes []diffNote) {
	if len(classified) == 0 && len(errors) == 0 && len(notes) == 0 {
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

	if len(notes) > 0 {
		fmt.Printf("\n%d note(s):\n", len(notes))
		for _, n := range notes {
			fmt.Printf("  note: [%s] %s\n", n.Type, n.Message)
			for _, r := range n.Related {
				fmt.Printf("    related: %s\n", r)
			}
		}
	}
}
