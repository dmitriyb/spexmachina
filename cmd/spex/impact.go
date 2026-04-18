package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

func newImpactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "impact",
		Short: "Compute impact of spec changes on beads",
		RunE:  runImpactE,
	}
	cmd.Flags().String("diff", "", "path to diff JSON file (default: stdin)")
	cmd.Flags().String("map", ".bead-map.json", "path to bead mapping file")
	return cmd
}

func runImpactE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	diffFlag, _ := cmd.Flags().GetString("diff")

	// Read diff JSON input.
	var diffData []byte
	if diffFlag != "" {
		diffData, err = os.ReadFile(diffFlag)
		if err != nil {
			return fmt.Errorf("impact: read diff: %w", err)
		}
	} else {
		diffData, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("impact: read stdin: %w", err)
		}
	}

	changes, diffErrors, err := parseDiffJSON(diffData)
	if err != nil {
		return fmt.Errorf("impact: %w", err)
	}

	if len(diffErrors) > 0 {
		for _, de := range diffErrors {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: [%s] %s\n", de.Type, de.Message)
		}
		return fmt.Errorf("impact: diff contains %d error(s), refusing to proceed", len(diffErrors))
	}

	// Resolve map path relative to spec dir if not absolute.
	mapFlag, _ := cmd.Flags().GetString("map")
	mapPath := mapFlag
	if !filepath.IsAbs(mapPath) {
		mapPath = filepath.Join(filepath.Dir(specDir), mapPath)
	}

	store := mapping.NewFileStore(mapPath)
	records, err := store.List()
	if err != nil {
		return fmt.Errorf("impact: read mapping records: %w", err)
	}

	// Changes and records both key on identity hashes; NodeMatcher joins them
	// directly without any path-format translation.
	matches, unmatched, orphaned := impact.MatchNodes(changes, records)
	actions := impact.ClassifyActions(matches, unmatched, orphaned)

	// Post-processing: resolve spec-graph dependencies for create actions.
	specGraph, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		return fmt.Errorf("impact: load spec graph: %w", err)
	}
	for i := range actions {
		if actions[i].Type == "create" {
			actions[i].DepBeadIDs = impact.ResolveDeps(specGraph, records, actions[i])
		}
	}

	if err := impact.GenerateReport(actions, os.Stdout); err != nil {
		return fmt.Errorf("impact: %w", err)
	}
	return nil
}

// parseDiffJSON converts the JSON output of `spex diff --json` into
// []merkle.ClassifiedChange and []merkle.DiffError for the impact pipeline.
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
				Path:     c.Path,
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
		ID      string `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
	} `json:"components"`
	ImplSections []struct {
		ID      string `json:"id"`
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

// ContentMap maps node keys (e.g., "component/1") to content file paths
// (e.g., "spec/render/arch_spec_reader.md") within a module.
type ContentMap map[string]string

// buildNodeMaps reads project.json and each module's module.json to build
// a map of module name → NodeMap for resolving spec-ID keys to human-readable names.
// Also returns a map of module name → ContentMap for resolving content file paths.
func buildNodeMaps(specDir string) (map[string]impact.NodeMap, map[string]ContentMap, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read project.json: %w", err)
	}
	var proj projectJSON
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, nil, fmt.Errorf("parse project.json: %w", err)
	}

	modules := map[string]impact.NodeMap{}
	contents := map[string]ContentMap{}
	for _, m := range proj.Modules {
		modPath := filepath.Join(specDir, m.Path, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			continue // module directory may not have module.json yet
		}
		var mod moduleJSON
		if err := json.Unmarshal(data, &mod); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", modPath, err)
		}

		nm := impact.NodeMap{}
		cm := ContentMap{}
		for _, c := range mod.Components {
			if c.Content != "" && c.ID != "" {
				nm[c.ID] = c.Name
				cm[c.ID] = filepath.Join("spec", m.Path, c.Content)
			}
		}
		for _, s := range mod.ImplSections {
			if s.Content != "" && s.ID != "" {
				nm[s.ID] = s.Name
				cm[s.ID] = filepath.Join("spec", m.Path, s.Content)
			}
		}

		modules[m.Name] = nm
		contents[m.Name] = cm
	}
	return modules, contents, nil
}
