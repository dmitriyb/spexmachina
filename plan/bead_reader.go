package plan

import (
	"encoding/json"
	"fmt"
	"io"
)

// TODO(bead:spexmachina-swvx.7): BeadReader (Bead, ReadBeads, ReadBeadsBytes
// below) is the pre-task-lifecycle --beads input spec/plan/flow_plan.md step
// 2 replaces with TaskReader/--tasks. Remove this file and its callers once
// spexmachina-swvx.14 lands TaskReader and cmd/spex/plan.go's --beads flag
// retires.

// Bead is BeadReader's per-entry output: the tracker's own bead id and its
// live status, exactly as the input reported it. Nothing else survives the
// parse — BeadReader reads no labels, since the label is an adapter-facing
// idempotency key spex reads nothing from (see arch_bead_reader.md, "No
// Label Parsing").
type Bead struct {
	ID     string
	Status string
}

// trackerBead mirrors the input JSON shape. Fields beyond id and status —
// labels included — are present in real tracker output but are not decoded
// here: BeadReader reads nothing from them.
type trackerBead struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// trackerListing is the {"issues": [...]} envelope br list --json produces.
type trackerListing struct {
	Issues []trackerBead `json:"issues"`
}

// ReadBeads decodes a tracker listing from r into one Bead per entry, in
// input order. It is a pure parser: no subprocess invocation, no live
// tracker calls (arch_bead_reader.md, "No Subprocess"). Callers supply the
// bytes of `br list --json`, or any tracker whose output conforms to the
// same shape, via --beads file input.
//
// Both the wrapped ({"issues": [...]}) and bare ([...]) shapes are
// accepted, the latter for adapter-produced JSON that may have unwrapped
// it. An empty valid array returns an empty slice, not an error.
func ReadBeads(r io.Reader) ([]Bead, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("plan: read beads: %w", err)
	}
	return ReadBeadsBytes(data)
}

// ReadBeadsBytes is ReadBeads for callers that already hold the payload in
// memory.
func ReadBeadsBytes(data []byte) ([]Bead, error) {
	beads, err := decodeBeads(data)
	if err != nil {
		return nil, err
	}

	out := make([]Bead, 0, len(beads))
	for i, b := range beads {
		if b.ID == "" {
			return nil, fmt.Errorf("plan: read beads: missing bead id at index %d", i)
		}
		out = append(out, Bead{ID: b.ID, Status: b.Status})
	}
	return out, nil
}

// decodeBeads tries the wrapped envelope first and falls back to a bare
// array. The bare-array parse error is what reaches the caller — the
// wrapped attempt is best-effort.
func decodeBeads(data []byte) ([]trackerBead, error) {
	var wrap trackerListing
	if err := json.Unmarshal(data, &wrap); err == nil && wrap.Issues != nil {
		return wrap.Issues, nil
	}
	var beads []trackerBead
	if err := json.Unmarshal(data, &beads); err != nil {
		return nil, fmt.Errorf("plan: read beads: parse: %w", err)
	}
	return beads, nil
}
