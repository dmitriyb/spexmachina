// Package impact maps merkle diff changes to affected beads, classifying
// actions (create/close/review) for each impacted spec node.
package impact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path"
	"sort"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
)

// BeadSpec holds the spec-related metadata extracted from a bead.
type BeadSpec struct {
	ID          string `json:"id"`
	Module      string `json:"module"`
	Component   string `json:"component"`
	ImplSection string `json:"impl_section"`
	SpecHash    string `json:"spec_hash"`
}

// Match pairs a classified change with the beads that reference its spec node.
type Match struct {
	Change merkle.ClassifiedChange
	Beads  []BeadSpec
}

// Action describes a single impact action derived from change analysis.
type Action struct {
	Type   string `json:"type"`             // "create", "close", "review"
	BeadID string `json:"bead_id,omitempty"` // existing bead ID (empty for "create")
	Module string `json:"module"`
	Node   string `json:"node"`             // component or impl_section name
	Impact string `json:"impact,omitempty"` // impact level from merkle classification
	Reason string `json:"reason"`
}

// ImpactReport is the structured output of impact analysis.
type ImpactReport struct {
	Creates []Action `json:"creates"`
	Closes  []Action `json:"closes"`
	Reviews []Action `json:"reviews"`
	Summary Summary  `json:"summary"`
}

// Summary counts actions by type.
type Summary struct {
	CreateCount int `json:"create_count"`
	CloseCount  int `json:"close_count"`
	ReviewCount int `json:"review_count"`
}

// ReadBeads calls the bead CLI to list all beads with spec metadata.
// Beads without spec metadata fields are ignored.
func ReadBeads(ctx context.Context, beadCLI string) ([]BeadSpec, error) {
	out, err := exec.CommandContext(ctx, beadCLI, "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("impact: read beads: %w", err)
	}

	var raw []struct {
		ID     string            `json:"id"`
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("impact: parse bead list: %w", err)
	}

	var beads []BeadSpec
	for _, b := range raw {
		mod := b.Labels["spec_module"]
		if mod == "" {
			continue
		}
		beads = append(beads, BeadSpec{
			ID:          b.ID,
			Module:      mod,
			Component:   b.Labels["spec_component"],
			ImplSection: b.Labels["spec_impl_section"],
			SpecHash:    b.Labels["spec_hash"],
		})
	}
	return beads, nil
}

// MatchNodes correlates classified changes with beads.
// Returns matched changes, unmatched changes (no bead), and orphaned beads
// (bead references a removed node).
func MatchNodes(changes []merkle.ClassifiedChange, beads []BeadSpec) (matched []Match, unmatched []merkle.ClassifiedChange, orphaned []BeadSpec) {
	// Index beads by (module, component) and (module, impl_section).
	type key struct{ module, node string }
	compIdx := make(map[key][]BeadSpec)
	implIdx := make(map[key][]BeadSpec)
	modIdx := make(map[string][]BeadSpec)

	for _, b := range beads {
		if b.Component != "" {
			compIdx[key{b.Module, b.Component}] = append(compIdx[key{b.Module, b.Component}], b)
		}
		if b.ImplSection != "" {
			implIdx[key{b.Module, b.ImplSection}] = append(implIdx[key{b.Module, b.ImplSection}], b)
		}
		modIdx[b.Module] = append(modIdx[b.Module], b)
	}

	// Track which beads were matched (for orphan detection on removals).
	matchedBeadIDs := make(map[string]bool)

	for _, cc := range changes {
		nodeName, nodeType := resolveNode(cc.Path)
		var found []BeadSpec

		if nodeType == "structural" {
			// Structural changes affect all beads in the module.
			found = modIdx[cc.Module]
		} else if nodeType == "component" {
			found = compIdx[key{cc.Module, nodeName}]
		} else if nodeType == "impl_section" || nodeType == "flow" {
			found = implIdx[key{cc.Module, nodeName}]
		}

		if len(found) > 0 {
			matched = append(matched, Match{Change: cc, Beads: found})
			for _, b := range found {
				matchedBeadIDs[b.ID] = true
			}
		} else {
			unmatched = append(unmatched, cc)
		}
	}

	// Orphaned: beads referencing removed nodes that weren't matched above.
	// Only relevant for "removed" changes — find beads that reference nodes
	// which were removed but didn't appear in matched results.
	for _, cc := range changes {
		if cc.Type != merkle.Removed {
			continue
		}
		nodeName, nodeType := resolveNode(cc.Path)
		var candidates []BeadSpec
		if nodeType == "component" {
			candidates = compIdx[key{cc.Module, nodeName}]
		} else if nodeType == "impl_section" || nodeType == "flow" {
			candidates = implIdx[key{cc.Module, nodeName}]
		}
		for _, b := range candidates {
			if !matchedBeadIDs[b.ID] {
				orphaned = append(orphaned, b)
				matchedBeadIDs[b.ID] = true
			}
		}
	}

	return matched, unmatched, orphaned
}

// ClassifyActions determines the action for each matched, unmatched, and
// orphaned result according to the decision table in the spec.
func ClassifyActions(matched []Match, unmatched []merkle.ClassifiedChange, orphaned []BeadSpec) []Action {
	var actions []Action

	// Matched changes: action depends on change type.
	for _, m := range matched {
		cc := m.Change
		nodeName, _ := resolveNode(cc.Path)
		switch cc.Type {
		case merkle.Removed:
			for _, b := range m.Beads {
				actions = append(actions, Action{
					Type:   "close",
					BeadID: b.ID,
					Module: cc.Module,
					Node:   nodeName,
					Reason: fmt.Sprintf("Spec node removed: %s/%s", cc.Module, nodeName),
				})
			}
		case merkle.Added, merkle.Modified:
			for _, b := range m.Beads {
				actions = append(actions, Action{
					Type:   "review",
					BeadID: b.ID,
					Module: cc.Module,
					Node:   nodeName,
					Impact: cc.Impact.String(),
					Reason: fmt.Sprintf("Spec node modified (%s): %s/%s", cc.Impact, cc.Module, nodeName),
				})
			}
		}
	}

	// Unmatched changes: create for added/modified, skip for removed.
	for _, cc := range unmatched {
		if cc.Type == merkle.Removed {
			continue
		}
		nodeName, _ := resolveNode(cc.Path)
		actions = append(actions, Action{
			Type:   "create",
			Module: cc.Module,
			Node:   nodeName,
			Impact: cc.Impact.String(),
			Reason: fmt.Sprintf("New spec node: %s/%s", cc.Module, nodeName),
		})
	}

	// Orphaned beads: close.
	for _, b := range orphaned {
		node := b.Component
		if node == "" {
			node = b.ImplSection
		}
		actions = append(actions, Action{
			Type:   "close",
			BeadID: b.ID,
			Module: b.Module,
			Node:   node,
			Reason: fmt.Sprintf("Spec node removed: %s/%s", b.Module, node),
		})
	}

	// Sort for determinism: by type (close, create, review), then module, then node.
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Type != actions[j].Type {
			return actions[i].Type < actions[j].Type
		}
		if actions[i].Module != actions[j].Module {
			return actions[i].Module < actions[j].Module
		}
		return actions[i].Node < actions[j].Node
	})

	return actions
}

// GenerateReport builds an ImpactReport from actions and writes it as JSON.
func GenerateReport(actions []Action, w io.Writer) error {
	report := ImpactReport{
		Creates: []Action{},
		Closes:  []Action{},
		Reviews: []Action{},
	}

	for _, a := range actions {
		switch a.Type {
		case "create":
			report.Creates = append(report.Creates, a)
		case "close":
			report.Closes = append(report.Closes, a)
		case "review":
			report.Reviews = append(report.Reviews, a)
		}
	}

	report.Summary = Summary{
		CreateCount: len(report.Creates),
		CloseCount:  len(report.Closes),
		ReviewCount: len(report.Reviews),
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&report); err != nil {
		return fmt.Errorf("impact: encode report: %w", err)
	}
	return nil
}

// resolveNode extracts the node name and type from a merkle tree path.
// Returns the filename-derived name and the node type ("component",
// "impl_section", "flow", or "structural").
func resolveNode(p string) (name, nodeType string) {
	base := path.Base(p)

	if base == "project.json" || base == "module.json" {
		return base, "structural"
	}

	// Strip .md extension.
	base = strings.TrimSuffix(base, ".md")

	if strings.HasPrefix(base, "arch_") {
		return snakeToPascal(strings.TrimPrefix(base, "arch_")), "component"
	}
	if strings.HasPrefix(base, "impl_") {
		return snakeToPascal(strings.TrimPrefix(base, "impl_")), "impl_section"
	}
	if strings.HasPrefix(base, "flow_") {
		return snakeToPascal(strings.TrimPrefix(base, "flow_")), "flow"
	}

	return base, ""
}

// snakeToPascal converts snake_case to PascalCase.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}
