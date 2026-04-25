// Package impact maps merkle diff to affected beads.
package impact

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// BeadSpec holds the spec-related metadata extracted from a tracker bead.
type BeadSpec struct {
	ID       string   // tracker bead ID, e.g. "spexmachina-abc"
	RecordID int      // integer parsed from the "spex:<n>" label
	Status   string   // live status: open | in_progress | closed | blocked | deferred
	Labels   []string // all labels, retained for downstream filters
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

// ReadBeads decodes tracker list output from r into []BeadSpec. It is a pure
// parser: it performs no subprocess invocation and makes no live tracker
// calls. Callers feed it the bytes of `br list --json` (or any tracker whose
// output conforms to the same shape) via --beads file input or stdin.
//
// Accepted input shapes:
//   - wrapped: {"issues": [...]}
//   - bare array: [...]
//
// Only beads carrying a spex:<n> label are returned; others are silently
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
		recID, ok := extractRecordID(b.Labels)
		if !ok {
			continue
		}
		out = append(out, BeadSpec{
			ID:       b.ID,
			RecordID: recID,
			Status:   b.Status,
			Labels:   b.Labels,
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

// extractRecordID returns the integer from the first well-formed spex:<n>
// label. Non-numeric spex: values are skipped, matching defensive intent —
// validator-level rules should prevent them, but the reader does not error.
func extractRecordID(labels []string) (int, bool) {
	for _, lbl := range labels {
		if !strings.HasPrefix(lbl, "spex:") {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(lbl, "spex:")); err == nil && n >= 0 {
			return n, true
		}
	}
	return 0, false
}
