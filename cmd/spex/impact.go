package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

func runImpact(args []string) int {
	fs := flag.NewFlagSet("impact", flag.ContinueOnError)
	diffFlag := fs.String("diff", "", "path to diff JSON file (default: stdin)")
	mapFlag := fs.String("map", ".bead-map.json", "path to bead mapping file")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	specDir := "spec"
	if fs.NArg() > 0 {
		specDir = fs.Arg(0)
	}

	// Read diff JSON input.
	var diffData []byte
	var err error
	if *diffFlag != "" {
		diffData, err = os.ReadFile(*diffFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex impact: read diff: %v\n", err)
			return 1
		}
	} else {
		diffData, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex impact: read stdin: %v\n", err)
			return 1
		}
	}

	changes, err := parseDiffJSON(diffData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	// Resolve map path relative to spec dir if not absolute.
	mapPath := *mapFlag
	if !filepath.IsAbs(mapPath) {
		absSpec, err := filepath.Abs(specDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "spex impact: resolve spec path: %v\n", err)
			return 1
		}
		mapPath = filepath.Join(filepath.Dir(absSpec), mapPath)
	}

	store := mapping.NewFileStore(mapPath)
	records, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: read mapping records: %v\n", err)
		return 1
	}

	matches, unmatched, orphaned := impact.MatchNodes(changes, records)
	actions := impact.ClassifyActions(matches, unmatched, orphaned)

	if err := impact.GenerateReport(actions, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}
	return 0
}

// parseDiffJSON converts the JSON output of `spex diff --json` into
// []merkle.ClassifiedChange for the impact pipeline.
func parseDiffJSON(data []byte) ([]merkle.ClassifiedChange, error) {
	var raw struct {
		Changes []struct {
			Path    string `json:"path"`
			Type    string `json:"type"`
			Impact  string `json:"impact"`
			Module  string `json:"module"`
			OldHash string `json:"old_hash"`
			NewHash string `json:"new_hash"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse diff JSON: %w", err)
	}

	changes := make([]merkle.ClassifiedChange, len(raw.Changes))
	for i, c := range raw.Changes {
		ct, err := parseChangeType(c.Type)
		if err != nil {
			return nil, err
		}
		il, err := parseImpactLevel(c.Impact)
		if err != nil {
			return nil, err
		}
		changes[i] = merkle.ClassifiedChange{
			Change: merkle.Change{
				Path:    c.Path,
				Type:    ct,
				OldHash: c.OldHash,
				NewHash: c.NewHash,
			},
			Impact: il,
			Module: c.Module,
		}
	}
	return changes, nil
}

func parseChangeType(s string) (merkle.ChangeType, error) {
	switch s {
	case "added":
		return merkle.Added, nil
	case "removed":
		return merkle.Removed, nil
	case "modified":
		return merkle.Modified, nil
	default:
		return 0, fmt.Errorf("unknown change type: %q", s)
	}
}

func parseImpactLevel(s string) (merkle.ImpactLevel, error) {
	switch s {
	case "impl_only":
		return merkle.ImplOnly, nil
	case "arch_impl":
		return merkle.ArchImpl, nil
	case "structural":
		return merkle.Structural, nil
	default:
		return 0, fmt.Errorf("unknown impact level: %q", s)
	}
}
