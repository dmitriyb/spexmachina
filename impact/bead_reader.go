// Package impact maps merkle diff to affected beads.
package impact

import (
	"encoding/json"
	"fmt"
	"io"
)

// BeadSpec holds the two facts arch_bead_reader.md's Interface section
// promises: the tracker's own bead id and the bead's live status, exactly as
// the input reported it. BeadReader parses no labels — the label is an
// adapter-facing idempotency key spex reads nothing from — so BeadSpec
// carries nothing derived from one.
type BeadSpec struct {
	ID     string // tracker bead ID, e.g. "spexmachina-abc"
	Status string // live status, exactly as the input reported it
}

// trackerBead mirrors the input JSON shape. Fields beyond id and status
// (labels and the rest) are present in real tracker output but are not
// decoded here: BeadReader reads nothing from them.
type trackerBead struct {
	ID     string `json:"id"`
	Status string `json:"status"`
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
// One entry is returned per bead, in input order. An empty valid array
// returns an empty slice, not an error.
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
		out = append(out, BeadSpec{ID: b.ID, Status: b.Status})
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
