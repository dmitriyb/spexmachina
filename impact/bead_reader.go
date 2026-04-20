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

// ReadBeads calls the bead CLI to list every bead (regardless of status) and
// extracts those that carry a `spex:<record-id>` label. Beads without that
// label are ignored.
//
// Passes explicit status filters because br list defaults to excluding closed
// beads, which would hide exactly the records the cleanup classifier needs.
// Also passes --limit 0 to bypass the default 50-bead cap. bd ignores these
// flags when they are unrecognised, so the same command works for both CLIs.
func ReadBeads(ctx context.Context, bin string) ([]BeadSpec, error) {
	out, err := exec.CommandContext(ctx, bin, "list",
		"-s", "open",
		"-s", "in_progress",
		"-s", "blocked",
		"-s", "closed",
		"-s", "deferred",
		"--limit", "0",
		"--json",
	).Output()
	if err != nil {
		msg := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("impact: read beads: %s list --json: %w\n%s", bin, err, msg)
	}

	// br list --json returns {"issues": [...]}; bd list --json returns a bare
	// array. Accept both shapes so the reader works across bead CLIs.
	var envelope struct {
		Issues []rawBead `json:"issues"`
	}
	var raw []rawBead
	if err := json.Unmarshal(out, &envelope); err == nil && envelope.Issues != nil {
		raw = envelope.Issues
	} else if err := json.Unmarshal(out, &raw); err != nil {
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
