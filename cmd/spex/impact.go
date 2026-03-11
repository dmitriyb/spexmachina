package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
)

func runImpact(args []string) int {
	fs := flag.NewFlagSet("impact", flag.ContinueOnError)
	diffFlag := fs.String("diff", "", "path to diff JSON file (default: stdin)")
	beadCLI := fs.String("bead-cli", "br", "bead CLI binary name")
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

	ctx := context.Background()
	beads, err := impact.ReadBeads(ctx, *beadCLI)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	absSpec, err := filepath.Abs(specDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: resolve spec path: %v\n", err)
		return 1
	}

	modules, err := buildNodeMaps(absSpec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "spex impact: %v\n", err)
		return 1
	}

	matches, unmatched, orphaned := impact.MatchNodes(changes, beads, modules)
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

// moduleJSON is the subset of module.json we need for building NodeMaps.
type moduleJSON struct {
	Components []struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"components"`
	ImplSections []struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"impl_sections"`
}

// projectJSON is the subset of project.json we need for module name→path mapping.
type projectJSON struct {
	Modules []struct {
		Name string `json:"name"`
		Path string `json:"path"`
	} `json:"modules"`
}

// buildNodeMaps reads project.json and each module's module.json to build
// a map of module name → NodeMap. Module names come from project.json
// (matching the merkle tree path convention).
func buildNodeMaps(specDir string) (map[string]impact.NodeMap, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	var proj projectJSON
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}

	modules := map[string]impact.NodeMap{}
	for _, m := range proj.Modules {
		modPath := filepath.Join(specDir, m.Path, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			continue // module directory may not have module.json yet
		}
		var mod moduleJSON
		if err := json.Unmarshal(data, &mod); err != nil {
			return nil, fmt.Errorf("parse %s: %w", modPath, err)
		}

		nm := impact.NodeMap{}
		for _, c := range mod.Components {
			if c.Content != "" {
				nm["component/"+strconv.Itoa(c.ID)] = c.Name
			}
		}
		for _, s := range mod.ImplSections {
			if s.Content != "" {
				nm["impl_section/"+strconv.Itoa(s.ID)] = s.Name
			}
		}

		// TODO(spexmachina-3ta): add data_flow entries once DataFlows carry bead references.

		modules[m.Name] = nm
	}
	return modules, nil
}
