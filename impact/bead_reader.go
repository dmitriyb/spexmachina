// Package impact maps merkle diff to affected beads.
package impact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// BeadSpec holds the spec-related metadata extracted from a bead's labels.
type BeadSpec struct {
	ID          string
	Status      string
	Module      string
	Component   string
	ImplSection string
	SpecHash    string
}

// rawBead is the JSON shape returned by `<bin> list --json`.
type rawBead struct {
	ID     string   `json:"id"`
	Status string   `json:"status"`
	Labels []string `json:"labels"`
}

// ReadBeads calls `<bin> list --json` and extracts beads that have
// spec-related labels (spec_module, spec_component, spec_impl_section, spec_hash).
// Beads without a spec_module label are ignored.
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
		bs := parseLabels(r)
		if bs.Module == "" {
			continue
		}
		beads = append(beads, bs)
	}
	return beads, nil
}

// parseLabels extracts spec metadata from a raw bead's labels.
func parseLabels(r rawBead) BeadSpec {
	bs := BeadSpec{
		ID:     r.ID,
		Status: r.Status,
	}
	for _, l := range r.Labels {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		switch k {
		case "spec_module":
			bs.Module = v
		case "spec_component":
			bs.Component = v
		case "spec_impl_section":
			bs.ImplSection = v
		case "spec_hash":
			bs.SpecHash = v
		}
	}
	return bs
}
