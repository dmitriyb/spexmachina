package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

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

	// Build a reverse index: bead-map spec_node_id → records, keyed by
	// the merkle path format so NodeMatcher can match directly.
	// This avoids mutating Change.Path (which apply's deriveSpecNodeID needs
	// in original merkle format).
	merkleIndex := buildMerkleIndex(records, changes)

	matches, unmatched, orphaned := impact.MatchNodes(changes, merkleIndex)
	actions := impact.ClassifyActions(matches, unmatched, orphaned)

	if err := impact.GenerateReport(actions, os.Stdout); err != nil {
		return fmt.Errorf("impact: %w", err)
	}
	return nil
}

// toSpecNodeID translates a merkle key (module/<moduleID>/<nodeType>/<nodeID>)
// to a bead-map spec_node_id (moduleName/<nodeType>/<nodeID>), using the
// module name already carried in the ClassifiedChange.
func toSpecNodeID(c merkle.ClassifiedChange) string {
	parts := splitKey(c.Change.Path)
	if len(parts) >= 4 && parts[0] == "module" {
		return c.Module + "/" + parts[2] + "/" + parts[3]
	}
	if len(parts) >= 2 && parts[0] == "module" {
		return c.Module + "/module"
	}
	if len(parts) >= 2 && parts[0] == "project" {
		return "project/" + parts[1]
	}
	return c.Change.Path
}

// buildMerkleIndex re-keys bead-map records from their spec_node_id format
// (moduleName/nodeType/nodeID) to the merkle path format (module/moduleID/nodeType/nodeID)
// so that NodeMatcher's direct string comparison works against Change.Path.
func buildMerkleIndex(records []mapping.Record, changes []merkle.ClassifiedChange) []mapping.Record {
	// Build module name → merkle ID prefix mapping from the changes.
	moduleIDs := map[string]string{}
	for _, c := range changes {
		parts := splitKey(c.Change.Path)
		if len(parts) >= 2 && parts[0] == "module" && c.Module != "" {
			moduleIDs[c.Module] = parts[1] // e.g., "render" → "7"
		}
	}

	// Re-key each record's SpecNodeID to merkle format.
	rekeyed := make([]mapping.Record, len(records))
	copy(rekeyed, records)
	for i, r := range rekeyed {
		parts := splitKey(r.SpecNodeID)
		if len(parts) == 3 {
			// "schema/component/1" → "module/<moduleID>/component/1"
			if modID, ok := moduleIDs[parts[0]]; ok {
				rekeyed[i].SpecNodeID = "module/" + modID + "/" + parts[1] + "/" + parts[2]
			}
		}
	}
	return rekeyed
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
// a map of module name → NodeMap for resolving spec-ID keys to human-readable names.
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

		modules[m.Name] = nm
	}
	return modules, nil
}
