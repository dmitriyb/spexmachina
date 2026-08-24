package ingest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
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
// the classifier's bead-producing set. Relative to that negation, the only type
// the explicit list excludes is "meta", the project.json / module.json
// envelope leaf: absorbing an added or removed meta leaf would baseline
// a whole module appearing or vanishing without any gate seeing it.
//
// "component" is removal-only. A removed component's task already
// exists, and absorbing the node's disappearance is safe only because
// the live-pairing gate below still demands the task be closed first.
// An *added* component is a bead that was never created; baselining it
// into the snapshot would remove it from `spex diff` permanently, which
// is precisely the bead lifecycle refresh must not bypass. component is
// on the list at all only because retiring a spec component whose code
// is gone is a structural removal with no bead work left to do.
var refreshAbsorbable = map[string]refreshDirections{
	"requirement": {added: true, removed: true},
	"api":         {added: true, removed: true},
	"component":   {added: false, removed: true},
}

// RefreshRefusal is the typed error for the refresh gates: structural
// diff entries or a live task pairing on a removed node. IngestCommand
// maps it to a non-zero exit with a structured stderr message naming
// the entries.
type RefreshRefusal struct {
	Kind    string   // "added_entries" | "removed_entries" | "live_task_pairing"
	Entries []string
	Hint    string
}

func (e *RefreshRefusal) Error() string {
	return fmt.Sprintf("ingest: refresh refused (%s): %s — %s",
		e.Kind, strings.Join(e.Entries, ", "), e.Hint)
}

// RefreshSummary is the stdout contract of `spex ingest --mode refresh`,
// per flow_ingest.md's "Summary output (mode: refresh)" shape. Field
// order on this struct IS the canonical JSON field order. Status is
// always "complete" on success — refresh has no partial state;
// unsuccessful runs return an error and never print a summary.
type RefreshSummary struct {
	EventsAppended int    `json:"events_appended"`
	SnapshotSaved  bool   `json:"snapshot_saved"`
	Status         string `json:"status"`
}

// RefreshHandler is the refresh-mode ingest pathway: it absorbs drift
// that owes no bead work — content edits to any leaf, plus additions
// and removals of the node types on refreshAbsorbable — by appending
// one change event per absorbed drift entry to the task journal, closed
// by one refresh receipt, and rewriting the snapshot — atomically, with
// no bead lifecycle. See spec/ingest/arch_refresh.md for the refusal
// contract.
type RefreshHandler struct {
	// SnapshotPath is the diff baseline and rewrite target. Defaults to
	// <specDir>/.snapshot.json when empty.
	SnapshotPath string
	// JournalPath is the task journal's resolved location. Defaults to
	// <specDir>/.history.jsonl when empty; the shipped command sets both
	// this and SnapshotPath to the locations inside .spex/ the lifecycle
	// pre-flight resolved, so this component computes no location of its
	// own.
	JournalPath string
	// Changeset and Receipts are the (required, empty) artifacts the
	// caller parsed. Any ops present refuse the run.
	Changeset *plan.Changeset
	Receipts  *adapters.Receipts
	// GitHead is the optional --git-head value. Nil records the value's
	// absence (a JSON null on the refresh receipt); a non-nil value is
	// also stamped onto every change event the run constructs.
	GitHead *string
	// Now is the timestamp source for the rewritten snapshot's
	// created_at. Defaults to time.Now; tests inject a fixed clock.
	Now func() time.Time
}

// liveMetadata is what Apply needs from the current spec graph to name a
// still-live node's added or modified change event: its declared name and
// (for the node types that have one) its content-leaf path.
type liveMetadata struct {
	Name string
	Path string
}

// Apply runs the refresh-mode pathway end-to-end: pre-flight, the diff
// against the pre-refresh snapshot, the structural and live-pairing
// gates, event construction, and the atomic paired commit of the
// journal and the snapshot. On any error both files are left unchanged
// (the journal is rolled back if the snapshot write fails after it).
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

	changes := merkle.Diff(tree, snapshot)
	if len(changes) == 0 {
		summary.Status = adapters.StatusComplete
		return summary, nil
	}

	// Structural gate: any added or removed entry the absorbable set
	// does not cover refuses the whole run before anything is
	// constructed. Modified entries never reach this gate.
	var addedRefused, removedRefused []string
	for _, c := range changes {
		switch c.Type {
		case merkle.Added:
			if !refreshAbsorbable[c.NodeType].added {
				addedRefused = append(addedRefused, c.Key)
			}
		case merkle.Removed:
			if !refreshAbsorbable[c.NodeType].removed {
				removedRefused = append(removedRefused, c.Key)
			}
		}
	}
	if len(addedRefused) > 0 {
		return summary, &RefreshRefusal{
			Kind:    "added_entries",
			Entries: addedRefused,
			Hint:    "refresh mode does not absorb structural changes; use the normal pipeline",
		}
	}
	if len(removedRefused) > 0 {
		return summary, &RefreshRefusal{
			Kind:    "removed_entries",
			Entries: removedRefused,
			Hint:    "refresh mode does not absorb structural changes; use the normal pipeline",
		}
	}

	journalPath := h.JournalPath
	if journalPath == "" {
		journalPath = filepath.Join(specDir, ".history.jsonl")
	}
	store := mapping.NewMappingStore(journalPath)
	existing, err := store.Parse()
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: read journal: %w", err)
	}
	fold, err := store.List()
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: read journal: %w", err)
	}

	foldByKey := make(map[string]mapping.FoldEntry, len(fold.Entries))
	for _, e := range fold.Entries {
		foldByKey[e.Key] = e
	}
	closedTaskIDs := map[string]bool{}
	lastChangeByNode := map[string]mapping.Event{}
	seenEIDs := make(map[string]bool, len(existing))
	for _, ev := range existing {
		switch ev.Event {
		case "task_closed":
			closedTaskIDs[ev.TaskID] = true
		case "added", "modified", "removed":
			lastChangeByNode[ev.Node] = ev
		}
		if ev.EID != "" {
			seenEIDs[ev.EID] = true
		}
	}

	// Live-pairing gate: a removed entry whose node's current fold
	// linkage is still a live, unclosed task_created refuses the run —
	// bead work is owed and the normal pipeline must close or clean it
	// up first. Liveness is decided from TaskID/closedTaskIDs alone: the
	// fold's Removed flag says nothing about whether that task is open —
	// a cleanup's task_created folds to Removed:true too, since it
	// inherits Removed from the removal event it pairs with — so it must
	// not gate this check.
	var livePairing []string
	for _, c := range changes {
		if c.Type != merkle.Removed {
			continue
		}
		entry, ok := foldByKey[c.Key]
		if ok && entry.TaskID != "" && !closedTaskIDs[entry.TaskID] {
			livePairing = append(livePairing, fmt.Sprintf("%s (%s)", c.Key, entry.TaskID))
		}
	}
	if len(livePairing) > 0 {
		return summary, &RefreshRefusal{
			Kind:    "live_task_pairing",
			Entries: livePairing,
			Hint:    "live task for removed node; structural drift requires the normal pipeline",
		}
	}

	liveIndex, err := buildLiveIndex(specDir)
	if err != nil {
		return summary, fmt.Errorf("ingest: refresh: %w", err)
	}

	gitHead := ""
	if h.GitHead != nil {
		gitHead = *h.GitHead
	}

	var batch []mapping.Event
	var absorbed []string
	for _, c := range changes {
		switch c.Type {
		case merkle.Added:
			if last, hasLast := lastChangeByNode[c.Key]; hasLast && last.Event == "added" {
				// Already the journal's current state for this node —
				// its latest change event is itself an addition, from a
				// prior (possibly partial) run. Only the snapshot is
				// stale. Checked off the node's latest change event
				// rather than a raw eid-seen check: deriveRefreshEID is
				// a pure function of (node, before, after), so a node
				// re-added with byte-identical content after a removal
				// derives the same eid as its first addition — that is
				// genuinely new information (the node came back) and
				// must still be constructed, just under a disambiguated
				// eid (see uniqueRefreshEID below).
				continue
			}
			eid := uniqueRefreshEID(seenEIDs, deriveRefreshEID(c.Key, nil, strPtr(c.NewHash)))
			md := liveIndex[c.Key]
			ev := mapping.Event{
				Event: "added", EID: eid,
				Node: c.Key, Name: md.Name, NodeType: c.NodeType, Module: c.Module,
				Before: nil, After: strPtr(c.NewHash),
				GitHead: gitHead, Path: md.Path,
			}
			batch = append(batch, ev)
			absorbed = append(absorbed, ev.EID)

		case merkle.Modified:
			// meta leaves (the project.json / module.json envelope)
			// carry no node_type the journal-line schema accepts —
			// their drift is absorbed into the snapshot rewrite alone,
			// never as a change event.
			if c.NodeType == "meta" {
				continue
			}
			if last, hasLast := lastChangeByNode[c.Key]; hasLast && last.Event == "modified" &&
				derefOrEmpty(last.Before) == c.OldHash && derefOrEmpty(last.After) == c.NewHash {
				// Already the journal's current state for this node —
				// its latest change event already records this exact
				// before->after transition, from a prior (possibly
				// partial) run that journaled it but left the snapshot
				// stale. Nothing new to say, so construct nothing,
				// mirroring the Added/Removed branches above. A
				// transition that recurs after an intervening one
				// (a flap back to an earlier state) is NOT this case —
				// its before/after pair won't match the latest event —
				// and falls through to construct below, same as a
				// content-identical re-add falls through on Added.
				continue
			}
			eid := uniqueRefreshEID(seenEIDs, deriveRefreshEID(c.Key, strPtr(c.OldHash), strPtr(c.NewHash)))
			md := liveIndex[c.Key]
			ev := mapping.Event{
				Event: "modified", EID: eid,
				Node: c.Key, Name: md.Name, NodeType: c.NodeType, Module: c.Module,
				Before: strPtr(c.OldHash), After: strPtr(c.NewHash),
				GitHead: gitHead, Path: md.Path,
			}
			batch = append(batch, ev)
			absorbed = append(absorbed, ev.EID)

		case merkle.Removed:
			last, hasLast := lastChangeByNode[c.Key]
			if hasLast && last.Event == "removed" {
				// Already the journal's current state for this node —
				// its latest change event is itself a removal, from a
				// prior (possibly partial) run. Only the snapshot is
				// stale; there is no new information here, so construct
				// nothing rather than manufacture a redundant line under
				// a disambiguated eid. Checked off the node's latest
				// change event rather than the fold's Removed flag:
				// fold() never updates a requirement/api entry on an
				// "added" event (they get no task_created to carry the
				// update), so after a re-add that flag would stay stuck
				// on the earlier removal.
				continue
			}
			entry, hasFold := foldByKey[c.Key]
			name, path := "", ""
			if hasFold {
				name, path = entry.Source.Name, entry.Source.Path
			} else if hasLast {
				name, path = last.Name, last.Path
			}
			// Composes with the lastChangeByNode skip above rather than
			// replacing it: that check only catches *consecutive*
			// removals of the same node. A node restored verbatim
			// between two refresh-absorbed removals derives the same
			// eid both times too — same node, same before-hash, no
			// after-hash — but its latest event is "added" in between,
			// so the check above does not fire. Disambiguate instead of
			// dropping: the removal is still real information the
			// journal must carry, just under an eid distinct from the
			// earlier occurrence's.
			eid := uniqueRefreshEID(seenEIDs, deriveRefreshEID(c.Key, strPtr(c.OldHash), nil))
			ev := mapping.Event{
				Event: "removed", EID: eid,
				Node: c.Key, Name: name, NodeType: c.NodeType, Module: c.Module,
				Before: strPtr(c.OldHash), After: nil,
				GitHead: gitHead, Path: path,
			}
			batch = append(batch, ev)
			absorbed = append(absorbed, ev.EID)
		}
	}

	if absorbed == nil {
		absorbed = []string{}
	}
	batch = append(batch, mapping.Event{Event: "refresh", GitHead: gitHead, Absorbed: absorbed})

	if err := checkInvariant5(batch); err != nil {
		return summary, err
	}

	originalJournal, readErr := os.ReadFile(journalPath)
	journalExisted := true
	if readErr != nil {
		if !errors.Is(readErr, os.ErrNotExist) {
			return summary, fmt.Errorf("ingest: refresh: read journal: %w", readErr)
		}
		journalExisted = false
	}

	if err := store.Append(batch); err != nil {
		return summary, fmt.Errorf("ingest: refresh: write journal: %w", err)
	}

	now := h.Now
	if now == nil {
		now = time.Now
	}
	if err := writeAtomic(snapPath, tree, now()); err != nil {
		var rbErr error
		if journalExisted {
			rbErr = restoreJournalBytes(journalPath, originalJournal)
		} else {
			rbErr = os.Remove(journalPath)
		}
		if rbErr != nil {
			return summary, fmt.Errorf("ingest: refresh: write snapshot: %w (journal rollback also failed: %v)", err, rbErr)
		}
		return summary, fmt.Errorf("ingest: refresh: write snapshot: %w (journal rolled back)", err)
	}

	summary.EventsAppended = len(absorbed)
	summary.SnapshotSaved = true
	summary.Status = adapters.StatusComplete
	return summary, nil
}

// restoreJournalBytes rewrites the journal at path back to original via
// the same temp-file + rename sequence MappingStore.Append uses, so a
// rollback carries the same crash-safety guarantee as the write it undoes.
func restoreJournalBytes(path string, original []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, original, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// deriveRefreshEID derives a refresh-born change event's id from the
// drift it records — the node identity plus its before/after hashes —
// rather than from (git_head, op_id): a refresh-born event has no op
// behind it. See arch_refresh.md.
func deriveRefreshEID(node string, before, after *string) string {
	return "refresh:" + node + ":" + derefOrEmpty(before) + ":" + derefOrEmpty(after)
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// uniqueRefreshEID returns base, or — if base already names a journal
// line (this run's or an earlier one's, per seenEIDs) — a suffixed
// variant that does not. deriveRefreshEID is a pure function of (node,
// before, after), so an added or removed event can legitimately recur
// with the exact same derived id across non-adjacent add/remove cycles
// once the drift's content repeats verbatim; the caller has already
// decided the event carries real information (via lastChangeByNode) and
// must not be dropped, so collision is resolved by disambiguating the id
// rather than skipping construction. Registers the chosen id in
// seenEIDs before returning it.
func uniqueRefreshEID(seenEIDs map[string]bool, base string) string {
	eid := base
	for n := 2; seenEIDs[eid]; n++ {
		eid = fmt.Sprintf("%s#%d", base, n)
	}
	seenEIDs[eid] = true
	return eid
}

// buildLiveIndex reads the current spec directory's project.json and
// every module.json to resolve the name and content-leaf path (when the
// node type has one) of every requirement, api, component, data_flow and
// test_section the spec currently declares. Apply uses it to name added
// and modified events — both always describe a node the current spec
// still carries.
func buildLiveIndex(specDir string) (map[string]liveMetadata, error) {
	projData, err := os.ReadFile(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil, fmt.Errorf("read project.json: %w", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}

	idx := map[string]liveMetadata{}
	for _, req := range proj.Requirements {
		idx[req.ID] = liveMetadata{Name: req.Title}
	}

	for _, mod := range proj.Modules {
		modDir := filepath.Join(specDir, mod.Path)
		modPath := filepath.Join(modDir, "module.json")
		data, err := os.ReadFile(modPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", modPath, err)
		}
		var ms schema.ModuleSpec
		if err := json.Unmarshal(data, &ms); err != nil {
			return nil, fmt.Errorf("parse %s: %w", modPath, err)
		}

		for _, req := range ms.Requirements {
			idx[req.ID] = liveMetadata{Name: req.Title}
		}
		for _, api := range ms.APIs {
			idx[api.ID] = liveMetadata{Name: api.Name}
		}
		for _, c := range ms.Components {
			idx[c.ID] = liveMetadata{Name: c.Name, Path: contentPath(mod.Path, c.Content)}
		}
		for _, df := range ms.DataFlows {
			idx[df.ID] = liveMetadata{Name: df.Name, Path: contentPath(mod.Path, df.Content)}
		}
		for _, ts := range ms.TestSections {
			idx[ts.ID] = liveMetadata{Name: ts.Name, Path: contentPath(mod.Path, ts.Content)}
		}
	}
	return idx, nil
}

// contentPath renders a node's content-leaf path in the journal's
// recorded convention: "spec/<module-path>/<content-file>", regardless
// of what the spec directory is actually called on disk. Empty when the
// node has no content file (requirement and api events never call this).
func contentPath(modPath, content string) string {
	if content == "" {
		return ""
	}
	return filepath.Join("spec", modPath, content)
}
