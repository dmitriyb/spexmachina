package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
)

func newImpactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "impact",
		Short: "Compute impact of spec changes on beads",
		Args:  cobra.NoArgs,
		RunE:  runImpactE,
	}
	cmd.Flags().String("diff", "", "path to diff JSON file (default: stdin)")
	cmd.Flags().String("beads", "", "path to tracker list JSON (e.g. `br list --json` output) — supplies live bead status for cleanup-bead classification")
	// --bead-cli is retained for backward compatibility during the
	// decouple-spex-from-br transition (proposal 2026-04-18). It is currently
	// a no-op; callers should migrate to --beads.
	cmd.Flags().String("bead-cli", "br", "deprecated; use --beads instead")
	cmd.Flags().Bool("json", true, "emit JSON output (default and currently the only supported format)")
	return cmd
}

func runImpactE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}

	diffFlag, _ := cmd.Flags().GetString("diff")

	// Read diff JSON input. "-" is the explicit stdin convention; an empty
	// flag also means stdin (the flag was not supplied).
	var diffData []byte
	if diffFlag != "" && diffFlag != "-" {
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

	// Refuse an incomplete or inconsistent spec edit before any of
	// matching, classification or report generation runs.
	if len(diffErrors) > 0 {
		for _, de := range diffErrors {
			fmt.Fprintf(cmd.ErrOrStderr(), "error: [%s] %s\n", de.Type, de.Message)
		}
		return fmt.Errorf("impact: diff contains %d error(s), refusing to proceed", len(diffErrors))
	}

	// The task journal is the sole source of node-to-task pairings. There is
	// no separate --map flag: its location is a function of --spec-dir alone.
	fold, err := mapping.NewMappingStore(specDir).List()
	if err != nil {
		return fmt.Errorf("impact: read journal: map: %w", err)
	}
	pairings := pairingsFromFold(fold)

	beadsFlag, _ := cmd.Flags().GetString("beads")
	if beadsFlag != "" {
		beadsData, err := os.ReadFile(beadsFlag)
		if err != nil {
			return fmt.Errorf("impact: read beads: %w", err)
		}
		beads, err := impact.ReadBeadsBytes(beadsData)
		if err != nil {
			return fmt.Errorf("impact: %w", err)
		}
		pairings = enrichPairingsWithBeadStatus(beads, pairings)
	}

	// Spec graph is used for test_section describes-length gating inside
	// ClassifyActions and for dependency resolution on create actions.
	specGraph, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		return fmt.Errorf("impact: load spec graph: %w", err)
	}

	// Changes and pairings both key on identity hashes; NodeMatcher joins
	// them directly without any path-format translation. DepSpecNodeIDs is
	// filled inline by ClassifyActions — bead-ID resolution is deferred to
	// emit.
	matches, unmatched, orphaned := impact.MatchNodes(changes, pairings)
	actions := impact.ClassifyActions(specGraph, matches, unmatched, orphaned)

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

// pairingsFromFold adapts the task journal's folded linkage
// (mapping.Fold, from spec/.history.jsonl) into impact.Pairing,
// NodeMatcher's own input shape. Both key on identity hashes, so this is a
// field copy — no rekeying. Proposal-epic entries (keyed by a proposal slug
// rather than a spec node identity hash) carry no Source.Node and are
// dropped: they never match a diff change, which always keys on a node's
// identity hash. Removed entries are tombstones — biographies of
// already-ingested removals with no live TaskID — and are dropped too: if
// the same identity hash reappears in a diff (a component re-added under
// its old name), a tombstone must not be mistaken for a live pairing.
func pairingsFromFold(fold mapping.Fold) []impact.Pairing {
	var out []impact.Pairing
	for _, e := range fold.Entries {
		if e.Source.Node == "" || e.Removed {
			continue
		}
		var after string
		if e.Source.After != nil {
			after = *e.Source.After
		}
		out = append(out, impact.Pairing{
			SpecNodeID: e.Key,
			TaskID:     e.TaskID,
			NodeType:   e.Source.NodeType,
			Module:     e.Source.Module,
			Name:       e.Source.Name,
			After:      after,
		})
	}
	return out
}

// enrichPairingsWithBeadStatus populates each pairing's BeadStatus field by
// matching on task id. spexmachina-hdkq.13 retired BeadSpec.SpecNodeID and
// BeadSpec.Labels — BeadReader no longer parses labels (see
// spec/impact/arch_bead_reader.md, "No Label Parsing") — so the join moved
// from spec node id to task id, per flow_impact_analysis.md's "BeadReader →
// NodeMatcher" data shape: "each entry's status is copied onto the journal
// fold's pairing whose task id matches". When the same task id appears on
// more than one bead, the last one in the input wins — decodeBeads/
// ReadBeadsBytes preserve input order, so the join is deterministic.
// Pairings without a matching bead are returned unchanged so the
// cleanup-bead gate at action_classifier.go defaults closed (no cleanup
// actions emitted) for safety.
func enrichPairingsWithBeadStatus(beads []impact.BeadSpec, pairings []impact.Pairing) []impact.Pairing {
	if pairings == nil {
		return nil
	}
	statusByTaskID := make(map[string]string, len(beads))
	for _, b := range beads {
		statusByTaskID[b.ID] = b.Status
	}
	enriched := make([]impact.Pairing, len(pairings))
	for i, p := range pairings {
		if status, ok := statusByTaskID[p.TaskID]; ok {
			p.BeadStatus = status
		}
		enriched[i] = p
	}
	return enriched
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
