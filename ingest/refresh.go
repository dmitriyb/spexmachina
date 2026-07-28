package ingest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Sentinel pre-flight errors for refresh mode. IngestCommand maps these
// to ExitInputError; the gate refusals below map to ExitInvariant.
var (
	// ErrRefreshRequiresSnapshot rejects a refresh run with no
	// pre-existing snapshot: the snapshot is the diff baseline, and
	// without one every leaf looks added — the bootstrap case that
	// requires the normal pipeline.
	ErrRefreshRequiresSnapshot = errors.New("ingest: refresh: no pre-existing snapshot; refresh requires a snapshot baseline (bootstrap via a normal-mode run)")
	// ErrRefreshNonEmptyArtifacts rejects a refresh run whose changeset
	// or receipts carry ops. Refresh has no per-op transitions; a
	// non-empty artifact is a configuration error.
	ErrRefreshNonEmptyArtifacts = errors.New("ingest: refresh: changeset and receipts must be empty in refresh mode")
)

// refreshDirections records, for one node type, which structural diff
// directions refresh may absorb.
type refreshDirections struct {
	added   bool
	removed bool
}

// refreshAbsorbable is the allow-list of node types whose structural
// addition or removal refresh absorbs; every type absent from it — and
// in particular "meta", "module", "data_flow" and "test_section" — is
// refused in both directions.
//
// The list is deliberately written out rather than derived by negating
// impact's bead-producing set. That negation also admits "meta", the
// project.json / module.json envelope leaf, and refresh runs neither
// `spex validate` nor the completeness checker: it would happily
// baseline a project-requirement addition `spex diff` rejects with exit
// 2 and a module-requirement removal `spex validate` rejects with exit
// 1, hiding both from every downstream tool.
//
// "component" is removal-only. A removed component's bead already
// exists, and absorbing the node's disappearance is safe only because
// the orphan gate below still demands the bead-map record be retired
// first. An *added* component is a bead that was never created;
// baselining it into the snapshot would remove it from `spex diff`
// permanently, which is precisely the bead lifecycle refresh must not
// bypass. component is on the list at all only because retiring a spec
// component whose code is gone is a structural removal with no bead
// work left to do.
var refreshAbsorbable = map[string]refreshDirections{
	"requirement":  {added: true, removed: true},
	"api":          {added: true, removed: true},
	"component":    {added: false, removed: true},
}

// RefreshRefusal is the typed error for the refresh gates: structural
// diff entries or orphan mapping records. IngestCommand maps it to a
// non-zero exit with a structured stderr message naming the entries.
type RefreshRefusal struct {
	Kind    string   // "added_entries" | "removed_entries" | "orphan_record"
	Entries []string // identity-hash keys (or "spec_node_id (bead_id)" for orphans)
	Hint    string
}

func (e *RefreshRefusal) Error() string {
	return fmt.Sprintf("ingest: refresh refused (%s): %s — %s",
		e.Kind, strings.Join(e.Entries, ", "), e.Hint)
}

// RefreshSummary is the stdout contract of `spex ingest --mode refresh`.
// Field order on this struct IS the canonical JSON field order. Status
// is always "complete" on success — refresh has no partial state;
// unsuccessful runs return an error and never print a summary.
type RefreshSummary struct {
	RecordsUpdated   int    `json:"records_updated"`
	RecordsUnchanged int    `json:"records_unchanged"`
	SnapshotSaved    bool   `json:"snapshot_saved"`
	Status           string `json:"status"`
}

// RefreshHandler is the refresh-mode ingest pathway: it absorbs drift
// that owes no bead work — content edits to any leaf, plus additions
// and removals of the node types on refreshAbsorbable — by re-hashing
// every content leaf, updating stale mapping-record spec_hash fields,
// and rewriting the snapshot — atomically, with no bead lifecycle. See
// spec/ingest/arch_refresh.md for the refusal contract.
type RefreshHandler struct {
	// Store is the bead-map the handler updates in place. Only
	// spec_hash fields ever change; bead ids, statuses, and the
	// monotonic counter are untouched.
	Store mapping.Store
	// SnapshotPath is the diff baseline and rewrite target. Defaults
	// to <specDir>/.snapshot.json when empty.
	SnapshotPath string
	// Changeset and Receipts are the (required, empty) artifacts the
	// caller parsed. Any ops present refuse the run.
	Changeset *emit.Changeset
	Receipts  *adapters.Receipts
	// Now is the timestamp source for the rewritten snapshot's
	// created_at. Defaults to time.Now; tests inject a fixed clock.
	Now func() time.Time
}

// Apply runs the refresh-mode pathway end-to-end: pre-flight, diff
// gates, orphan gate, spec_hash updates, and the atomic paired commit
// of bead-map + snapshot. On any error both files are left unchanged
// (the bead-map is rolled back if the snapshot write fails after it).
func (h *RefreshHandler) Apply(specDir string) (RefreshSummary, error) {
	var summary RefreshSummary

	if (h.Changeset != nil && len(h.Changeset.Ops) > 0) ||
		(h.Receipts != nil && len(h.Receipts.Ops) > 0) {
		return summary, ErrRefreshNonEmptyArtifacts
	}

	snapPath := h.SnapshotPath
	if snapPath == "" {
		snapPath = filepath.Join(specDir, ".snapshot.json")
	}
	if _, err := os.Stat(snapPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return summary, ErrRefreshRequiresSnapshot
		}
		return summary, fmt.Errorf("ingest: refresh: stat snapshot: %w", err)
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: %w", err)
	}
	snapshot, err := merkle.Load(snapPath)
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: %w", err)
	}

	// Structural gate. Only the node types on refreshAbsorbable may
	// appear as an addition or a removal, and only in the direction
	// that type allows; everything else refuses the run.
	changes := merkle.Diff(tree, snapshot)
	var added, removed []string
	for _, c := range changes {
		switch c.Type {
		case merkle.Added:
			if !refreshAbsorbable[c.NodeType].added {
				added = append(added, c.Key)
			}
		case merkle.Removed:
			if !refreshAbsorbable[c.NodeType].removed {
				removed = append(removed, c.Key)
			}
		}
	}
	if len(added) > 0 {
		return summary, &RefreshRefusal{
			Kind:    "added_entries",
			Entries: added,
			Hint:    "refresh mode does not absorb structural changes; use the normal pipeline",
		}
	}
	if len(removed) > 0 {
		return summary, &RefreshRefusal{
			Kind:    "removed_entries",
			Entries: removed,
			Hint:    "refresh mode does not absorb structural changes; use the normal pipeline",
		}
	}

	leafHashes := map[string]string{}
	collectLeafHashes(leafHashes, tree)

	records, err := h.Store.List()
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: read bead-map: %w", err)
	}
	nextID, err := h.Store.NextRecordID()
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: read bead-map counter: %w", err)
	}

	// Orphan gate. Proposal-epic records reference the proposal ref,
	// not a spec node hash, so they are exempt — same rule as the
	// Reconciler's invariant 4.
	for _, rec := range records {
		if rec.NodeType == "proposal" {
			continue
		}
		if _, ok := leafHashes[rec.SpecNodeID]; !ok {
			return summary, &RefreshRefusal{
				Kind:    "orphan_record",
				Entries: []string{fmt.Sprintf("%s (%s)", rec.SpecNodeID, rec.BeadID)},
				Hint:    "orphan mapping record; structural drift requires the normal pipeline",
			}
		}
	}

	// Update stale spec_hash fields in the in-memory copy. No other
	// record field changes; the counter does not advance.
	before := make([]mapping.Record, len(records))
	copy(before, records)
	for i := range records {
		if records[i].NodeType == "proposal" {
			summary.RecordsUnchanged++
			continue
		}
		current := leafHashes[records[i].SpecNodeID]
		if records[i].SpecHash != current {
			records[i].SpecHash = current
			summary.RecordsUpdated++
		} else {
			summary.RecordsUnchanged++
		}
	}

	summary.Status = adapters.StatusComplete

	// Clean no-op: nothing drifted, so neither file is rewritten —
	// re-running refresh on the same state must not perturb git status.
	if summary.RecordsUpdated == 0 && len(changes) == 0 {
		return summary, nil
	}

	// Atomic paired commit: bead-map first, snapshot second; roll the
	// bead-map back if the snapshot write fails so the two files never
	// represent different points in time.
	now := h.Now
	if now == nil {
		now = time.Now
	}
	if err := h.Store.Replace(records, nextID); err != nil {
		return summary, fmt.Errorf("ingest: refresh: write bead-map: %w", err)
	}
	if err := writeAtomic(snapPath, tree, now()); err != nil {
		if rbErr := h.Store.Replace(before, nextID); rbErr != nil {
			return summary, fmt.Errorf("ingest: refresh: write snapshot: %w (bead-map rollback also failed: %v)", err, rbErr)
		}
		return summary, fmt.Errorf("ingest: refresh: write snapshot: %w (bead-map rolled back)", err)
	}
	summary.SnapshotSaved = true
	return summary, nil
}

// collectLeafHashes flattens the tree's leaves into a key → content-hash
// map, the lookup table for both the orphan gate and the spec_hash
// updates.
func collectLeafHashes(out map[string]string, n *merkle.Node) {
	if n.Type == "leaf" {
		out[n.Key] = n.Hash
		return
	}
	for _, child := range n.Children {
		collectLeafHashes(out, child)
	}
}
