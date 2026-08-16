package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

// gitHeadRe enforces the --git-head pre-flight: 7-40 hex chars, lowercase.
var gitHeadRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// resolveSpecDir reads the --spec-dir persistent flag from the root command
// and returns its absolute path.
func resolveSpecDir(cmd *cobra.Command) (string, error) {
	specDir, err := cmd.Root().PersistentFlags().GetString("spec-dir")
	if err != nil {
		return "", fmt.Errorf("resolve spec-dir: %w", err)
	}
	abs, err := filepath.Abs(specDir)
	if err != nil {
		return "", fmt.Errorf("resolve spec-dir: %w", err)
	}
	return abs, nil
}

// parseDiffJSON converts the JSON output of `spex diff --json` into
// []merkle.ClassifiedChange and []merkle.DiffError for the plan pipeline.
func parseDiffJSON(data []byte) ([]merkle.ClassifiedChange, []merkle.DiffError, error) {
	var raw struct {
		Changes []struct {
			Path     string `json:"path"`
			Type     string `json:"type"`
			Impact   string `json:"impact"`
			Module   string `json:"module"`
			NodeType string `json:"node_type"`
			OldHash  string `json:"old_hash"`
			NewHash  string `json:"new_hash"`
		} `json:"changes"`
		Errors []merkle.DiffError `json:"errors"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("parse diff JSON: %w", err)
	}

	changes := make([]merkle.ClassifiedChange, len(raw.Changes))
	for i, c := range raw.Changes {
		ct, err := parseChangeType(c.Type)
		if err != nil {
			return nil, nil, err
		}
		il, err := parseImpactLevel(c.Impact)
		if err != nil {
			return nil, nil, err
		}
		changes[i] = merkle.ClassifiedChange{
			Change: merkle.Change{
				Key:      c.Path,
				Type:     ct,
				NodeType: c.NodeType,
				OldHash:  c.OldHash,
				NewHash:  c.NewHash,
			},
			Impact: il,
			Module: c.Module,
		}
	}
	return changes, raw.Errors, nil
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
	case "contract":
		return merkle.Contract, nil
	case "arch_impl":
		return merkle.ArchImpl, nil
	case "structural":
		return merkle.Structural, nil
	default:
		return 0, fmt.Errorf("unknown impact level: %q", s)
	}
}
