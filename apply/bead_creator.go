// Package apply executes bead actions derived from spec impact analysis.
package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/dmitriyb/spexmachina/mapping"
)

// Action describes a bead action derived from impact analysis.
type Action struct {
	Module           string   // spec module name, e.g. "validator"
	Node             string   // node name, e.g. "SchemaChecker"
	NodeType         string   // "module", "component", "test_section"
	SpecHash         string   // merkle hash of the spec node
	BeadID           string   // existing bead ID (for close/obsolete actions)
	OldBeadID        string   // predecessor bead ID (for creates replacing obsoleted beads)
	DepBeadIDs       []string // spec-graph dependency bead IDs
	Priority         int      // derived priority; -1 means unset
	SpecNodeID       string   // e.g. "validator/component/1"
	ContentFile      string   // spec content file path
	ParentSpecNodeID string   // parent spec node ID (for test_sections)
	Reason           string   // human-readable reason
	ChangeType       string   // "removed" or "modified" (for obsolete actions)
}

// CreateOpts holds parameters for creating a single bead.
type CreateOpts struct {
	Title    string
	Type     string
	Parent   string   // --parent flag value; empty means unset
	Deps     []string // --deps flag values (e.g. "blocks:xxx", "depends:yyy")
	Priority int      // --priority flag; -1 means unset
}

// BeadCLI abstracts bead creation, lookup, closure, and metadata updates
// so callers are not coupled to a specific binary (br or bd).
type BeadCLI interface {
	Create(ctx context.Context, opts CreateOpts) (string, error)
	FindExisting(ctx context.Context, labels []string) (string, error)
	Close(ctx context.Context, id string, labels []string) error
	Update(ctx context.Context, id string, metadata map[string]string) error
	// Status returns the current status of a bead (e.g. "open", "in_progress", "closed").
	Status(ctx context.Context, id string) (string, error)
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
		"--silent",
	}
	if opts.Parent != "" {
		args = append(args, "--parent", opts.Parent)
	}
	for _, dep := range opts.Deps {
		args = append(args, "--deps", dep)
	}
	if opts.Priority >= 0 {
		args = append(args, "--priority", strconv.Itoa(opts.Priority))
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

	var result struct {
		Issues []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("apply: parse %s list output: %w", c.bin, err)
	}
	for _, b := range result.Issues {
		if b.Status == "open" {
			return b.ID, nil
		}
	}
	return "", nil
}

// Close closes a bead, first adding the given labels via update, then closing.
// Labels are applied before close so the bead is marked while still open,
// giving br auto-flush a clean state transition.
func (c *execCLI) Close(ctx context.Context, id string, labels []string) error {
	// Add labels via update (br close does not support --add-label).
	for _, label := range labels {
		args := []string{"update", id, "--add-label", label}
		out, err := exec.CommandContext(ctx, c.bin, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("apply: %s update %s (label %s): %w\n%s", c.bin, id, label, err, out)
		}
	}

	out, err := exec.CommandContext(ctx, c.bin, "close", id).CombinedOutput()
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

// Status returns the current status of a bead by calling br show --json.
func (c *execCLI) Status(ctx context.Context, id string) (string, error) {
	out, err := exec.CommandContext(ctx, c.bin, "show", id, "--json").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("apply: %s show %s: %w\n%s", c.bin, id, err, out)
	}
	var items []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return "", fmt.Errorf("apply: parse %s show output: %w", c.bin, err)
	}
	if len(items) == 0 {
		return "", fmt.Errorf("apply: %s show %s: empty result", c.bin, id)
	}
	return items[0].Status, nil
}

// beadType maps spec node types to bead types.
// Returns empty string for node types that do not get beads.
func beadType(nodeType string) string {
	switch nodeType {
	case "module":
		return "epic"
	case "component":
		return "feature"
	case "test_section":
		return "task"
	default:
		return ""
	}
}

// resolveParent resolves the parent bead ID from the mapping store.
// Components are parented under the module's epic bead.
// Test sections are parented under the component's feature bead.
func resolveParent(store mapping.Store, action Action) string {
	switch action.NodeType {
	case "component":
		recs, err := store.GetBySpecNode(action.Module + "/module")
		if err == nil && len(recs) > 0 {
			return recs[0].BeadID
		}
	case "test_section":
		if action.ParentSpecNodeID != "" {
			recs, err := store.GetBySpecNode(action.ParentSpecNodeID)
			if err == nil && len(recs) > 0 {
				return recs[0].BeadID
			}
		}
	}
	return ""
}

// isCleanup returns true if the action represents a cleanup bead.
func isCleanup(a Action) bool {
	return strings.HasPrefix(a.Reason, "Code cleanup:")
}

// CreateBeads processes a batch of create actions sequentially.
// For each action, it checks for an existing open bead (idempotency)
// before creating a new one. After creation, it creates or updates
// the mapping record in the store and labels the bead with the record ID.
// Returns the list of bead IDs (existing or new).
func CreateBeads(ctx context.Context, cli BeadCLI, store mapping.Store, creates []Action) ([]string, error) {
	ids := make([]string, 0, len(creates))

	for _, a := range creates {
		if isCleanup(a) {
			id, err := createCleanupBead(ctx, cli, a)
			if err != nil {
				return ids, err
			}
			ids = append(ids, id)
			continue
		}

		bt := beadType(a.NodeType)
		if bt == "" {
			return ids, fmt.Errorf("apply: node type %q does not get a bead", a.NodeType)
		}

		// Idempotency: check for existing record and open bead with matching label.
		recs, _ := store.GetBySpecNode(a.SpecNodeID)
		if len(recs) > 0 {
			rec := recs[0]
			existing, err := cli.FindExisting(ctx, []string{fmt.Sprintf("spex:%d", rec.ID)})
			if err != nil {
				return ids, fmt.Errorf("apply: check existing bead for %s/%s: %w", a.Module, a.Node, err)
			}
			if existing != "" {
				ids = append(ids, existing)
				continue
			}
		}

		// Build create opts.
		opts := CreateOpts{
			Title:    fmt.Sprintf("%s: %s", a.Module, a.Node),
			Type:     bt,
			Priority: a.Priority,
		}

		if parentID := resolveParent(store, a); parentID != "" {
			opts.Parent = parentID
		}

		if a.OldBeadID != "" {
			opts.Deps = append(opts.Deps, fmt.Sprintf("blocks:%s", a.OldBeadID))
		}

		for _, depID := range a.DepBeadIDs {
			opts.Deps = append(opts.Deps, fmt.Sprintf("blocked-by:%s", depID))
		}

		beadID, err := cli.Create(ctx, opts)
		if err != nil {
			return ids, fmt.Errorf("apply: create bead for %s/%s: %w", a.Module, a.Node, err)
		}

		// Create or update mapping record.
		var recordID int
		if len(recs) > 0 {
			recordID = recs[0].ID
			err = store.Update(recordID, map[string]string{
				"bead_id":   beadID,
				"spec_hash": a.SpecHash,
			})
		} else {
			record := mapping.Record{
				SpecNodeID:  a.SpecNodeID,
				BeadID:      beadID,
				BeadType:    bt,
				Module:      a.Module,
				Component:   a.Node,
				ContentFile: a.ContentFile,
				SpecHash:    a.SpecHash,
			}
			recordID, err = store.Create(record)
		}
		if err != nil {
			return ids, fmt.Errorf("apply: mapping record for %s: %w", beadID, err)
		}

		// Label the bead with the mapping record ID.
		if err := cli.Update(ctx, beadID, map[string]string{
			"spex": strconv.Itoa(recordID),
		}); err != nil {
			return ids, fmt.Errorf("apply: set label on %s: %w", beadID, err)
		}

		ids = append(ids, beadID)
	}

	return ids, nil
}

// createCleanupBead creates a cleanup bead for a removed component.
// Cleanup beads have no mapping record and are labeled spex:cleanup.
func createCleanupBead(ctx context.Context, cli BeadCLI, a Action) (string, error) {
	opts := CreateOpts{
		Title:    fmt.Sprintf("Code cleanup: %s", a.Node),
		Type:     "task",
		Priority: -1,
	}
	if a.OldBeadID != "" {
		opts.Deps = append(opts.Deps, fmt.Sprintf("blocks:%s", a.OldBeadID))
	}

	beadID, err := cli.Create(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("apply: create cleanup bead for %s/%s: %w", a.Module, a.Node, err)
	}

	if err := cli.Update(ctx, beadID, map[string]string{
		"spex": "cleanup",
	}); err != nil {
		return "", fmt.Errorf("apply: set cleanup label on %s: %w", beadID, err)
	}

	return beadID, nil
}

// cliVersion returns the version string of the bead CLI, or "unknown" on error.
func cliVersion(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimRight(string(out), "\n")
}
