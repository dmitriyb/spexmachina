// Package impact maps merkle diff to affected beads.
package impact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// BeadSpec holds the spec-related metadata extracted from a bead's labels.
type BeadSpec struct {
	ID       string // bead ID
	Status   string // bead status
	RecordID int    // mapping record ID from "spex:<id>" label
}

// NodeMap maps node identifiers to their canonical spec node names.
// Used by the apply command for resolving spec-ID keys to human-readable names.
type NodeMap map[string]string

// rawBead is the JSON shape returned by `<bin> list --json`.
type rawBead struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

// ReadBeads calls `<bin> list --json` and extracts beads that carry a
// `spex:<record-id>` label. Beads without that label are ignored.
func ReadBeads(ctx context.Context, bin string) ([]BeadSpec, error) {
	out, err := exec.CommandContext(ctx, bin, "list", "--json").Output()
	if err != nil {
		msg := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("impact: read beads: %s list --json: %w\n%s", bin, err, msg)
	}

	var raw []rawBead
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("impact: read beads: parse JSON: %w", err)
	}

	var beads []BeadSpec
	for _, r := range raw {
		recID, ok := extractRecordID(r.Labels)
		if !ok {
			continue
		}
		beads = append(beads, BeadSpec{
			ID:       r.ID,
			Status:   r.Status,
			RecordID: recID,
		})
	}
	return beads, nil
}

// extractRecordID finds the spex:<record-id> label and returns the integer ID.
func extractRecordID(labels []string) (int, bool) {
	for _, label := range labels {
		if strings.HasPrefix(label, "spex:") {
			id, err := strconv.Atoi(strings.TrimPrefix(label, "spex:"))
			if err == nil && id >= 0 {
				return id, true
			}
		}
	}
	return 0, false
}
