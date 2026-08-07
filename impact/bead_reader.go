// Package impact maps merkle diff to affected beads.
package impact

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// BeadSpec holds the spec-related metadata extracted from a tracker bead.
// It carries exactly the four things arch_bead_reader.md's Interface
// section names: the tracker's own bead id, the spec node identity read out
// of the bead's label, its live status, and its full label list.
type BeadSpec struct {
	ID         string   // tracker bead ID, e.g. "spexmachina-abc"
	SpecNodeID string   // identity hash or proposal slug from the spex:<spec_node_id> label
	Status     string   // live status, exactly as the input reported it
	Labels     []string // all labels, retained for downstream filters
}

// NodeMap maps node identifiers to their canonical spec node names.
type NodeMap map[string]string

// trackerBead mirrors the input JSON shape.
type trackerBead struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

// wrappedInput is the {"issues": [...]} envelope produced by br list --json.
type wrappedInput struct {
	Issues []trackerBead `json:"issues"`
}

// identityHashPattern matches a spec node identity hash: 12 lowercase hex
// characters, the same shape the merkle tree and the task journal key on.
var identityHashPattern = regexp.MustCompile(`^[a-f0-9]{12}$`)

// proposalSlugPattern matches the dated-stem convention a proposal's own
// spec node id follows, e.g. "2026-04-18-decouple-spex-from-br". Only the
// date prefix is anchored; the rest of the slug is free text.
var proposalSlugPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-`)

// ReadBeads decodes tracker list output from r into []BeadSpec. It is a pure
// parser: it performs no subprocess invocation and makes no live tracker
// calls. Callers feed it the bytes of `br list --json` (or any tracker whose
// output conforms to the same shape) via --beads file input or stdin.
//
// Accepted input shapes:
//   - wrapped: {"issues": [...]}
//   - bare array: [...]
//
// Only beads carrying a spex:<spec_node_id> label whose suffix reads as an
// identity hash or a proposal slug are returned; others are silently
// dropped. An empty valid array returns an empty slice, not an error.
func ReadBeads(r io.Reader) ([]BeadSpec, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("impact: read beads: %w", err)
	}
	return ReadBeadsBytes(data)
}

// ReadBeadsBytes is ReadBeads for callers that already have the payload in
// memory.
func ReadBeadsBytes(data []byte) ([]BeadSpec, error) {
	beads, err := decodeBeads(data)
	if err != nil {
		return nil, err
	}

	out := make([]BeadSpec, 0, len(beads))
	for i, b := range beads {
		if b.ID == "" {
			return nil, fmt.Errorf("impact: read beads: missing bead id at index %d", i)
		}
		nodeID, ok := extractSpecNodeID(b.Labels)
		if !ok {
			continue
		}
		out = append(out, BeadSpec{
			ID:         b.ID,
			SpecNodeID: nodeID,
			Status:     b.Status,
			Labels:     b.Labels,
		})
	}
	return out, nil
}

// decodeBeads tries the wrapped envelope first and falls back to a bare
// array. A single parse error from the bare-array path is what gets surfaced
// to the caller — the wrapped attempt is best-effort.
func decodeBeads(data []byte) ([]trackerBead, error) {
	var wrap wrappedInput
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Issues != nil {
		return wrap.Issues, nil
	}
	var beads []trackerBead
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, fmt.Errorf("impact: read beads: parse: %w", err)
	}
	return beads, nil
}

// extractSpecNodeID returns the spec node id carried by the first
// spex:<...> label whose suffix reads as an identity hash or a proposal
// slug. Labels in the legacy integer form, the spex:cleanup-<hash> form,
// and inert markers like spex:obsolete or bare spex:cleanup match neither
// grammar and are passed over as inert. If a bead carries more than one
// live-form label, the rest are ignored — validator-level rules should
// prevent that, but extraction is defensive rather than reliant on them.
func extractSpecNodeID(labels []string) (string, bool) {
	for _, lbl := range labels {
		suffix, ok := strings.CutPrefix(lbl, "spex:")
		if !ok {
			continue
		}
		if identityHashPattern.MatchString(suffix) || proposalSlugPattern.MatchString(suffix) {
			return suffix, true
		}
	}
	return "", false
}
