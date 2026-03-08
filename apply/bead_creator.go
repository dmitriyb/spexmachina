// Package apply executes bead actions derived from spec impact analysis.
package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Action describes a bead action derived from impact analysis.
type Action struct {
	Module   string // spec module name, e.g. "validator"
	Node     string // node name, e.g. "SchemaChecker"
	NodeType string // "component" or "impl_section"
	SpecHash string // merkle hash of the spec node
	BeadID   string // existing bead ID (for close actions)
}

// CreateOpts holds parameters for creating a single bead.
type CreateOpts struct {
	Title  string
	Type   string
	Labels []string
}

// BeadCLI abstracts bead creation, lookup, closure, and metadata updates
// so callers are not coupled to a specific binary (br or bd).
type BeadCLI interface {
	Create(ctx context.Context, opts CreateOpts) (string, error)
	FindExisting(ctx context.Context, labels []string) (string, error)
	Close(ctx context.Context, id string, reason string) error
	Update(ctx context.Context, id string, metadata map[string]string) error
}

// execCLI implements BeadCLI by shelling out to br or bd.
type execCLI struct {
	bin string // "br" or "bd"
}

// NewBeadCLI constructs a BeadCLI backed by the given binary name.
// It verifies the binary exists on PATH and probes that the create,
// close, and update subcommands are available.
func NewBeadCLI(ctx context.Context, bin string) (BeadCLI, error) {
	if _, err := exec.LookPath(bin); err != nil {
		return nil, fmt.Errorf("apply: bead CLI not found: %s: %w", bin, err)
	}

	// Probe: verify the flags we depend on are accepted.
	probe := exec.CommandContext(ctx, bin,
		"create", "--dry-run",
		"--title", "probe",
		"--type", "task",
		"--labels", "probe",
		"--silent",
	)
	if out, err := probe.CombinedOutput(); err != nil {
		version := cliVersion(ctx, bin)
		return nil, fmt.Errorf("apply: %s create probe failed (version %s): %w\n%s", bin, version, err, out)
	}

	// Probe: verify the close subcommand exists.
	closeProbe := exec.CommandContext(ctx, bin, "close", "--help")
	if out, err := closeProbe.CombinedOutput(); err != nil {
		version := cliVersion(ctx, bin)
		return nil, fmt.Errorf("apply: %s close probe failed (version %s): %w\n%s", bin, version, err, out)
	}

	// Probe: verify the update subcommand exists.
	updateProbe := exec.CommandContext(ctx, bin, "update", "--help")
	if out, err := updateProbe.CombinedOutput(); err != nil {
		version := cliVersion(ctx, bin)
		return nil, fmt.Errorf("apply: %s update probe failed (version %s): %w\n%s", bin, version, err, out)
	}

	return &execCLI{bin: bin}, nil
}

// Create creates a new bead and returns its ID.
func (c *execCLI) Create(ctx context.Context, opts CreateOpts) (string, error) {
	args := []string{
		"create",
		"--title", opts.Title,
		"--type", opts.Type,
		"--labels", strings.Join(opts.Labels, ","),
		"--silent",
	}

	cmd := exec.CommandContext(ctx, c.bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("apply: %s create %q: %w\n%s", c.bin, opts.Title, err, ee.Stderr)
		}
		return "", fmt.Errorf("apply: %s create %q: %w", c.bin, opts.Title, err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// FindExisting searches for an open bead matching all given labels.
// Returns the bead ID if found, or empty string if none exists.
//
// Note: --status and --label filters cannot be combined in br (br bug),
// so we filter by label only and check status in Go.
func (c *execCLI) FindExisting(ctx context.Context, labels []string) (string, error) {
	args := []string{"list", "--json"}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	cmd := exec.CommandContext(ctx, c.bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("apply: %s list: %w\n%s", c.bin, err, ee.Stderr)
		}
		return "", fmt.Errorf("apply: %s list: %w", c.bin, err)
	}

	var beads []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &beads); err != nil {
		return "", fmt.Errorf("apply: parse %s list output: %w", c.bin, err)
	}
	for _, b := range beads {
		if b.Status == "open" {
			return b.ID, nil
		}
	}
	return "", nil
}

// Close closes a bead with the given reason.
func (c *execCLI) Close(ctx context.Context, id string, reason string) error {
	args := []string{"close", id, "--reason", reason}
	out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("apply: %s close %s: %w\n%s", c.bin, id, err, out)
	}
	return nil
}

// Update sets metadata key-value pairs on an existing bead.
// Keys are applied in sorted order for deterministic behavior.
func (c *execCLI) Update(ctx context.Context, id string, metadata map[string]string) error {
	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := metadata[k]
		args := []string{"update", id, "--add-label", fmt.Sprintf("%s:%s", k, v)}
		out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("apply: %s update %s: %w\n%s", c.bin, id, err, out)
		}
	}
	return nil
}

// specLabels builds the label set for an action's spec metadata.
func specLabels(a Action) []string {
	labels := []string{
		fmt.Sprintf("spec_module:%s", a.Module),
		fmt.Sprintf("spec_hash:%s", a.SpecHash),
	}
	switch a.NodeType {
	case "component":
		labels = append(labels, fmt.Sprintf("spec_component:%s", a.Node))
	case "impl_section":
		labels = append(labels, fmt.Sprintf("spec_impl_section:%s", a.Node))
	}
	return labels
}

// idempotencyLabels returns the labels used to check for existing beads.
// Excludes spec_hash since a hash change should not create a duplicate.
func idempotencyLabels(a Action) []string {
	labels := []string{
		fmt.Sprintf("spec_module:%s", a.Module),
	}
	switch a.NodeType {
	case "component":
		labels = append(labels, fmt.Sprintf("spec_component:%s", a.Node))
	case "impl_section":
		labels = append(labels, fmt.Sprintf("spec_impl_section:%s", a.Node))
	}
	return labels
}

// CreateBeads processes a batch of create actions sequentially.
// For each action, it checks for an existing open bead (idempotency)
// before creating a new one. Returns the list of bead IDs (existing or new).
func CreateBeads(ctx context.Context, cli BeadCLI, actions []Action) ([]string, error) {
	ids := make([]string, 0, len(actions))

	for _, a := range actions {
		existing, err := cli.FindExisting(ctx, idempotencyLabels(a))
		if err != nil {
			return ids, fmt.Errorf("apply: check existing bead for %s/%s: %w", a.Module, a.Node, err)
		}
		if existing != "" {
			ids = append(ids, existing)
			continue
		}

		id, err := cli.Create(ctx, CreateOpts{
			Title:  fmt.Sprintf("%s: %s", a.Module, a.Node),
			Type:   "task",
			Labels: specLabels(a),
		})
		if err != nil {
			return ids, fmt.Errorf("apply: create bead for %s/%s: %w", a.Module, a.Node, err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// cliVersion returns the version string of the bead CLI, or "unknown" on error.
func cliVersion(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimRight(string(out), "\n")
}
