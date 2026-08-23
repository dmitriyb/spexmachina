package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
)

// SpecGraph supplies the spec-side metadata the Reconciler needs to build a
// fresh or modified node's change event: its name, kind and module.
// Cleanup and proposal-epic creates never reach it — see
// spec/ingest/arch_reconciler.md "Proposal-Epic Ops" and "Cleanup-Create
// Ops".
type SpecGraph interface {
	NodeMetadata(specNodeID string) (NodeMetadata, error)
}

// NodeMetadata is the subset of spec-graph data a change event needs.
// ContentFile is the node's content-leaf path (the event's "path" field);
// SpecHash is the merkle leaf hash (the event's "after" field on add/modify).
type NodeMetadata struct {
	Module      string
	Component   string
	ContentFile string
	SpecHash    string
	NodeType    string
}

// ReconcileSummary is the per-op tally Reconciler.Apply returns. The CLI
// folds OkCreates+OkCloses into Summary.Ok before serialising to stdout.
// EventsAppended and ReceiptsAppended count only lines that actually
// landed on this call — an idempotent re-run reports zero of each even
// though every op still counts as ok.
type ReconcileSummary struct {
	OkCreates        int
	OkCloses         int
	Skipped          int
	Errors           int
	EventsAppended   int
	ReceiptsAppended int
}

// Reconciler consumes a paired changeset+receipts and appends the journal
// lines each op implies to spec/.history.jsonl: change events (added,
// modified, removed) and task receipts (task_created, task_closed).
// Event ids derive from (git_head, op_id), so a line whose eid the journal
// already carries is dropped rather than re-appended — the batch is
// idempotent by construction. Invariants 1, 2 and 5 are asserted against
// the whole batch before anything reaches disk; invariant 4 (snapshot
// saved iff complete) is SnapshotSaver's gate, not this component's.
type Reconciler struct {
	// SpecDir is the spec root; the journal lives at
	// <SpecDir>/.history.jsonl.
	SpecDir string
	// SpecGraph resolves a fresh or modified node's current metadata.
	SpecGraph SpecGraph
}

// Apply pairs each changeset op with its receipt, constructs the journal
// lines each ok op implies against an in-memory copy of the journal,
// asserts the invariants over journal-plus-batch, then commits the whole
// batch atomically. On any error the on-disk journal is untouched.
func (r *Reconciler) Apply(cs plan.Changeset, rc adapters.Receipts) (ReconcileSummary, error) {
	receiptsByOp, err := pairReceipts(cs, rc)
	if err != nil {
		return ReconcileSummary{}, err
	}

	// TODO(bead:spexmachina-uiei.8): resolve the journal location through
	// ProjectResolver instead of joining SpecDir here, once it lands.
	store := mapping.NewMappingStore(filepath.Join(r.SpecDir, ".history.jsonl"))
	existing, err := store.Parse()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}
	fold, err := store.List()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}

	existingEIDs := make(map[string]bool, len(existing))
	registeredByStem := map[string]string{}
	for _, ev := range existing {
		if ev.EID != "" {
			existingEIDs[ev.EID] = true
		}
		if ev.Event == "registered" {
			registeredByStem[ev.Proposal] = ev.EID
		}
	}
	batchEIDs := map[string]bool{}
	hasEID := func(eid string) bool { return existingEIDs[eid] || batchEIDs[eid] }

	removals := sameBatchRemovals(cs, receiptsByOp, fold)
	claimedByCreate := modifiedPairClaims(cs)

	var (
		sum             ReconcileSummary
		batch           []mapping.Event
		modifiedHandled = map[string]bool{}
		pendingModified = map[string]plan.Op{}
	)
	appendBatch := func(lines []mapping.Event) {
		for _, ev := range lines {
			batch = append(batch, ev)
			if ev.EID != "" {
				batchEIDs[ev.EID] = true
			}
		}
	}

	for _, op := range cs.Ops {
		opRC := receiptsByOp[op.OpID]

		switch op.Type {
		case plan.OpLabel, plan.OpTag:
			// Labels and tags carry no journal consequence and no tally.

		case plan.OpClose:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkCloses++
				switch {
				case strings.HasPrefix(op.Reason, ReasonRemovedPrefix):
					lines, err := r.buildRemoved(cs, op, opRC, fold, hasEID)
					if err != nil {
						return ReconcileSummary{}, err
					}
					appendBatch(lines)
				case strings.HasPrefix(op.Reason, ReasonModifiedPrefix):
					if op.Target == nil || op.Target.Kind != plan.RefBead || op.Target.BeadID == "" {
						return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:bead", op.OpID)
					}
					// Plan orders the paired create before this close (see
					// arch_reconciler.md "Ordering"), so the create has
					// already built the modified event and both receipts
					// by the time this close is reached — unless no such
					// create exists in the batch, which is checked once
					// the whole batch is processed. See "The Modified-Node
					// Pair".
					if !modifiedHandled[op.Target.BeadID] {
						pendingModified[op.Target.BeadID] = op
					}
				}
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		case plan.OpCreate:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkCreates++
				lines, err := r.buildCreate(cs, op, opRC, fold, modifiedHandled, removals, registeredByStem, hasEID)
				if err != nil {
					return ReconcileSummary{}, err
				}
				appendBatch(lines)
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		case plan.OpRetarget:
			switch opRC.Status {
			case adapters.OpStatusOk:
				// Ok retargets are not folded into OkCreates/OkCloses — the
				// summary's ok count aggregates only creates and closes, per
				// arch_reconciler.md "Interface".
				lines, err := r.buildRetarget(cs, op, fold, hasEID)
				if err != nil {
					return ReconcileSummary{}, err
				}
				appendBatch(lines)
			case adapters.OpStatusError:
				sum.Errors++
			case adapters.OpStatusSkipped:
				sum.Skipped++
			default:
				return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, opRC.Status)
			}

		default:
			return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: unknown type %q", op.OpID, op.Type)
		}
	}

	if len(pendingModified) > 0 {
		beadIDs := make([]string, 0, len(pendingModified))
		for beadID := range pendingModified {
			beadIDs = append(beadIDs, beadID)
		}
		sort.Strings(beadIDs)
		for _, beadID := range beadIDs {
			op := pendingModified[beadID]
			if claimedByCreate[beadID] {
				// The paired create exists in this batch but its receipt
				// was error/skipped — a partial run. Construct nothing for
				// the pair and let the rest of the batch land; see
				// arch_reconciler.md "The Modified-Node Pair".
				continue
			}
			// No create in the batch ever claimed this bead: the shape
			// ActionClassifier emits for a coupled test_section edit (an
			// obsolete with no replacement create). The node still exists,
			// so build the modified event straight from the close, off the
			// fold's live entry for the bead.
			lines, err := r.buildModifiedFromClose(cs, op, receiptsByOp[op.OpID], fold, hasEID)
			if err != nil {
				return ReconcileSummary{}, err
			}
			appendBatch(lines)
		}
	}

	// The changeset's top-level absorbed array is not receipt-gated — it
	// describes spec state rather than tracker work, so it is processed
	// regardless of what the ops loop above found. See arch_reconciler.md
	// "Absorbed Entries".
	absorbedLines, err := r.buildAbsorbed(cs, hasEID)
	if err != nil {
		return ReconcileSummary{}, err
	}
	appendBatch(absorbedLines)

	if len(batch) > 0 {
		if err := checkInvariant1(existing, batch); err != nil {
			return ReconcileSummary{}, err
		}
		if err := checkInvariant2(existing, batch); err != nil {
			return ReconcileSummary{}, err
		}
		if err := checkInvariant5(batch); err != nil {
			return ReconcileSummary{}, err
		}
		if err := store.Append(batch); err != nil {
			return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: commit: %w", err)
		}
	}

	for _, ev := range batch {
		switch ev.Event {
		case "added", "modified", "removed":
			sum.EventsAppended++
		case "task_created", "task_closed", "task_retargeted", "refresh":
			sum.ReceiptsAppended++
		}
	}
	return sum, nil
}

// buildCreate discriminates a create op's spec_node_kind before anything
// else, per arch_reconciler.md — a proposal-epic's spec_node_id is a
// proposal stem, not an identity hash, and a cleanup's spec_node_id names
// an already-removed node; neither may reach the spec graph.
func (r *Reconciler) buildCreate(cs plan.Changeset, op plan.Op, opRC adapters.OpReceipt, fold mapping.Fold, modifiedHandled map[string]bool, removals map[string]string, registeredByStem map[string]string, hasEID func(string) bool) ([]mapping.Event, error) {
	switch op.SpecNodeKind {
	case "proposal_epic":
		return buildEpicCreate(op, opRC, fold, registeredByStem)
	case "cleanup":
		return buildCleanupCreate(op, opRC, fold, removals)
	default:
		if oldBeadID, ok := blocksDepBeadID(op); ok {
			// Plan orders this create before the close that retires
			// oldBeadID (see arch_reconciler.md "Ordering"), so the pair
			// is built here, off the create's own blocks dep, rather
			// than waiting for the close. Marking it handled lets the
			// later close recognise the pairing already happened instead
			// of parking itself as unconsumed.
			modifiedHandled[oldBeadID] = true
			return r.buildModifiedPair(cs, op, opRC, oldBeadID, fold, hasEID)
		}
		return r.buildFreshCreate(cs, op, opRC, hasEID)
	}
}

// sameBatchRemovals maps every node identity hash this batch's ok
// "Spec node removed" closes will retire to the eid their removed event
// will carry — computed once, before any op is processed, so a cleanup
// create for that same hash resolves its referent regardless of whether
// plan placed the cleanup's create before or after its own removal close
// (real changesets always put it before — see arch_reconciler.md
// "Ordering"). Node hash comes from the fold's live entry for the close's
// target bead, exactly as buildRemoved resolves it.
func sameBatchRemovals(cs plan.Changeset, receiptsByOp map[string]adapters.OpReceipt, fold mapping.Fold) map[string]string {
	out := map[string]string{}
	for _, op := range cs.Ops {
		if op.Type != plan.OpClose || !strings.HasPrefix(op.Reason, ReasonRemovedPrefix) {
			continue
		}
		if receiptsByOp[op.OpID].Status != adapters.OpStatusOk {
			continue
		}
		if op.Target == nil || op.Target.Kind != plan.RefBead || op.Target.BeadID == "" {
			continue
		}
		for _, e := range fold.Entries {
			if e.TaskID == op.Target.BeadID {
				out[e.Key] = deriveEID(cs.GitHead, op.OpID)
				break
			}
		}
	}
	return out
}

// buildEpicCreate builds the one-line receipt a proposal-epic create
// implies: a task_created whose for names the proposal's registered event.
// Dedup is fold-based, not eid-based: an epic's task_created has no change
// event of its own to key an eid off, so a re-run recognises an existing
// epic by its slug already appearing in the fold, keyed there off the
// registered event's referent. A stem with no registered event in the
// journal is a malformed changeset — plan refuses to build such an op, so
// its arrival here is an invariant failure, not a fallback. See
// arch_reconciler.md "Proposal-Epic Ops".
func buildEpicCreate(op plan.Op, opRC adapters.OpReceipt, fold mapping.Fold, registeredByStem map[string]string) ([]mapping.Event, error) {
	stem := op.SpecNodeID
	for _, e := range fold.Entries {
		if e.Key == stem {
			return nil, nil // idempotent no-op: the epic already exists
		}
	}
	eid, ok := registeredByStem[stem]
	if !ok {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: proposal-epic %s: no registered event in journal", op.OpID, stem)
	}
	return []mapping.Event{{Event: "task_created", TaskID: opRC.BeadID, For: eid}}, nil
}

// buildCleanupCreate builds the one-line receipt a cleanup create implies:
// a task_created whose for names the removed node's own removal event.
// The journal is checked first — a live (not yet removed) fold entry for
// the hash means the removal is still pending in this same batch, so it
// falls through to removals, the precomputed same-batch removal map (see
// sameBatchRemovals); a hash that matches neither is a malformed
// changeset, not a fallback. See arch_reconciler.md "Cleanup-Create Ops".
func buildCleanupCreate(op plan.Op, opRC adapters.OpReceipt, fold mapping.Fold, removals map[string]string) ([]mapping.Event, error) {
	hash := op.SpecNodeID
	for _, e := range fold.Entries {
		if e.Key != hash || !e.Removed {
			continue
		}
		if e.TaskID != "" {
			return nil, nil // idempotent no-op: cleanup already landed for this removal
		}
		return []mapping.Event{{Event: "task_created", TaskID: opRC.BeadID, For: e.Source.EID}}, nil
	}
	if eid, ok := removals[hash]; ok {
		return []mapping.Event{{Event: "task_created", TaskID: opRC.BeadID, For: eid}}, nil
	}
	return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: cleanup for spec_node %s matches no removed event", op.OpID, hash)
}

// buildFreshCreate builds the added event plus its task_created for a
// plain new node — not a cleanup, not an epic, not paired with a close.
func (r *Reconciler) buildFreshCreate(cs plan.Changeset, op plan.Op, opRC adapters.OpReceipt, hasEID func(string) bool) ([]mapping.Event, error) {
	eid := deriveEID(cs.GitHead, op.OpID)
	if hasEID(eid) {
		return nil, nil
	}
	md, err := r.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}
	ev := mapping.Event{
		Event:    "added",
		EID:      eid,
		Node:     op.SpecNodeID,
		Name:     nonEmpty(md.Component, op.Title),
		NodeType: nonEmpty(md.NodeType, op.SpecNodeKind),
		Module:   md.Module,
		Before:   nil,
		After:    strPtr(md.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	created := mapping.Event{Event: "task_created", TaskID: opRC.BeadID, For: eid}
	return []mapping.Event{ev, created}, nil
}

// buildModifiedPair builds the one modified event plus both receipts for
// a create+close pair replacing the same node identity. The eid derives
// from the create op's id — the pair is one event, not two — and the
// before-hash comes from the node's current live fold entry (its content
// hash prior to this change). oldBeadID is the retiring task, read off
// the create op's own `blocks` dep — the paired close need not have been
// processed yet. See arch_reconciler.md "The Modified-Node Pair".
func (r *Reconciler) buildModifiedPair(cs plan.Changeset, op plan.Op, opRC adapters.OpReceipt, oldBeadID string, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
	eid := deriveEID(cs.GitHead, op.OpID)
	if hasEID(eid) {
		return nil, nil
	}
	md, err := r.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}

	var before *string
	for _, e := range fold.Entries {
		if e.Key == op.SpecNodeID {
			before = e.Source.After
			break
		}
	}

	ev := mapping.Event{
		Event:    "modified",
		EID:      eid,
		Node:     op.SpecNodeID,
		Name:     nonEmpty(md.Component, op.Title),
		NodeType: nonEmpty(md.NodeType, op.SpecNodeKind),
		Module:   md.Module,
		Before:   before,
		After:    strPtr(md.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	closed := mapping.Event{Event: "task_closed", TaskID: oldBeadID, For: eid}
	created := mapping.Event{Event: "task_created", TaskID: opRC.BeadID, For: eid}
	return []mapping.Event{ev, closed, created}, nil
}

// buildRemoved builds the removed event plus its task_closed for an
// ok close whose reason is "Spec node removed". The node's identity and
// biography (name, node_type, module, path, last content hash) come from
// the journal's live fold entry for the bead being closed — the spec no
// longer carries the node, so this is the only place left to ask.
func (r *Reconciler) buildRemoved(cs plan.Changeset, op plan.Op, opRC adapters.OpReceipt, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != plan.RefBead || op.Target.BeadID == "" {
		return nil, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:bead", op.OpID)
	}
	eid := deriveEID(cs.GitHead, op.OpID)
	if hasEID(eid) {
		return nil, nil
	}

	var entry mapping.FoldEntry
	found := false
	for _, e := range fold.Entries {
		if e.TaskID == op.Target.BeadID {
			entry, found = e, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: no journal entry for bead %s", op.OpID, op.Target.BeadID)
	}

	ev := mapping.Event{
		Event:    "removed",
		EID:      eid,
		Node:     entry.Key,
		Name:     entry.Source.Name,
		NodeType: entry.Source.NodeType,
		Module:   entry.Source.Module,
		Before:   entry.Source.After,
		After:    nil,
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     entry.Source.Path,
	}
	closed := mapping.Event{Event: "task_closed", TaskID: opRC.BeadID, For: eid}
	return []mapping.Event{ev, closed}, nil
}

// modifiedPairClaims maps every old bead id a create op in this batch
// claims via its `blocks` dep — regardless of that create's own receipt
// status. A "Spec node modified" close whose bead appears here but is
// still unhandled after the batch loop lost its pairing to a create that
// errored or was skipped (a partial run); a close whose bead does not
// appear here at all was never paired with a create in the batch at all.
// See arch_reconciler.md "The Modified-Node Pair".
func modifiedPairClaims(cs plan.Changeset) map[string]bool {
	out := map[string]bool{}
	for _, op := range cs.Ops {
		if op.Type != plan.OpCreate {
			continue
		}
		if oldBeadID, ok := blocksDepBeadID(op); ok {
			out[oldBeadID] = true
		}
	}
	return out
}

// buildModifiedFromClose builds the modified event plus its task_closed for
// an ok "Spec node modified" close whose bead no create op in the batch
// ever claimed via a `blocks` dep — the shape ActionClassifier emits for a
// coupled test_section edit (an obsolete action with no replacement
// create). The node still exists post-edit, so its current metadata comes
// from the spec graph; its identity and prior hash come from the journal's
// live fold entry for the bead being closed, exactly as buildRemoved
// resolves a removed node's identity. No task_created is built — there is
// no successor task to pair. See arch_reconciler.md "The Modified-Node
// Pair".
func (r *Reconciler) buildModifiedFromClose(cs plan.Changeset, op plan.Op, opRC adapters.OpReceipt, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != plan.RefBead || op.Target.BeadID == "" {
		return nil, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:bead", op.OpID)
	}
	eid := deriveEID(cs.GitHead, op.OpID)
	if hasEID(eid) {
		return nil, nil
	}

	var entry mapping.FoldEntry
	found := false
	for _, e := range fold.Entries {
		if e.TaskID == op.Target.BeadID {
			entry, found = e, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: no journal entry for bead %s", op.OpID, op.Target.BeadID)
	}

	md, err := r.lookupMetadata(entry.Key)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}

	ev := mapping.Event{
		Event:    "modified",
		EID:      eid,
		Node:     entry.Key,
		Name:     nonEmpty(md.Component, entry.Source.Name),
		NodeType: nonEmpty(md.NodeType, entry.Source.NodeType),
		Module:   md.Module,
		Before:   entry.Source.After,
		After:    strPtr(md.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	closed := mapping.Event{Event: "task_closed", TaskID: opRC.BeadID, For: eid}
	return []mapping.Event{ev, closed}, nil
}

// buildRetarget builds the one modified event plus the task_retargeted
// receipt an ok retarget op implies. The eid derives from the retarget
// op's own id exactly as a create op's eid does; node identity and the
// after-hash come straight off the op (SpecNodeID, SpecHash) rather than
// the spec graph — a retarget carries no cleanup or proposal-epic shape
// to discriminate first. Name, kind and module still come from the spec
// graph, and the before-hash is the node's prior content hash off the
// journal's live fold entry, exactly as buildModifiedPair sources it. No
// bead dies and none is born: task_id is the existing task the op already
// targets, not a freshly assigned one. See arch_reconciler.md "Retarget
// Ops".
func (r *Reconciler) buildRetarget(cs plan.Changeset, op plan.Op, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != plan.RefBead || op.Target.BeadID == "" {
		return nil, fmt.Errorf("ingest: reconcile: op %s: retarget target must be ref:bead", op.OpID)
	}
	eid := deriveEID(cs.GitHead, op.OpID)
	if hasEID(eid) {
		return nil, nil
	}
	md, err := r.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}

	var before *string
	for _, e := range fold.Entries {
		if e.Key == op.SpecNodeID {
			before = e.Source.After
			break
		}
	}

	ev := mapping.Event{
		Event:    "modified",
		EID:      eid,
		Node:     op.SpecNodeID,
		Name:     nonEmpty(md.Component, ""),
		NodeType: nonEmpty(md.NodeType, ""),
		Module:   md.Module,
		Before:   before,
		After:    strPtr(op.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	retargeted := mapping.Event{Event: "task_retargeted", TaskID: op.Target.BeadID, For: eid}
	return []mapping.Event{ev, retargeted}, nil
}

// buildAbsorbed builds one modified event per entry in the changeset's
// top-level absorbed array, plus the single refresh receipt naming the
// batch's newly-built absorbed eids. Absorbed entries are not receipt-
// gated — they describe spec state, not tracker work, so they land
// alongside ok ops on a partial run just the same. Eid derivation is
// shared with RefreshHandler's own (node, before, after) scheme, so a
// per-node absorption is indistinguishable, on the wire, from whole-run
// refresh absorption. An empty absorbed array, or one whose every derived
// eid the journal already carries, constructs nothing — not an empty
// receipt. See arch_reconciler.md "Absorbed Entries".
func (r *Reconciler) buildAbsorbed(cs plan.Changeset, hasEID func(string) bool) ([]mapping.Event, error) {
	var lines []mapping.Event
	var eids []string
	for _, entry := range cs.Absorbed {
		before, after := entry.Before, entry.After
		eid := deriveRefreshEID(entry.Node, &before, &after)
		if hasEID(eid) {
			continue
		}
		md, err := r.lookupMetadata(entry.Node)
		if err != nil {
			return nil, fmt.Errorf("ingest: reconcile: absorbed %s: %w", entry.Node, err)
		}
		lines = append(lines, mapping.Event{
			Event:    "modified",
			EID:      eid,
			Node:     entry.Node,
			Name:     nonEmpty(md.Component, ""),
			NodeType: nonEmpty(md.NodeType, ""),
			Module:   md.Module,
			Before:   strPtr(entry.Before),
			After:    strPtr(entry.After),
			GitHead:  cs.GitHead,
			Proposal: cs.Proposal,
			Path:     md.ContentFile,
		})
		eids = append(eids, eid)
	}
	if len(eids) == 0 {
		return lines, nil
	}
	lines = append(lines, mapping.Event{Event: "refresh", GitHead: cs.GitHead, Absorbed: eids})
	return lines, nil
}

// blocksDepBeadID reports the old bead id an op's lineage dep names, if
// any. ChangesetBuilder attaches this dep to every create replacing an
// obsoleted bead — cleanup and modify-pair creates alike — so its
// presence alone does not select modify-pair handling; buildCreate only
// consults it once cleanup and proposal_epic have been ruled out.
func blocksDepBeadID(op plan.Op) (string, bool) {
	for _, d := range op.Deps {
		if d.Kind == plan.RefBead && d.EdgeType == "blocks" {
			return d.BeadID, true
		}
	}
	return "", false
}

// lookupMetadata turns a missing SpecGraph or a missing node into a
// clearly wrapped error. Only fresh and modify-pair creates call this —
// cleanup and proposal_epic creates skip the spec graph entirely.
func (r *Reconciler) lookupMetadata(specNodeID string) (NodeMetadata, error) {
	if r.SpecGraph == nil {
		return NodeMetadata{}, fmt.Errorf("spec graph not configured")
	}
	md, err := r.SpecGraph.NodeMetadata(specNodeID)
	if err != nil {
		return NodeMetadata{}, fmt.Errorf("spec graph: %w", err)
	}
	return md, nil
}

// pairReceipts builds an op_id → OpReceipt index after asserting that the
// changeset and receipts cover exactly the same op_id set. An imbalance is
// a contract violation by plan or the adapter and is treated as input
// error, not invariant failure.
func pairReceipts(cs plan.Changeset, rc adapters.Receipts) (map[string]adapters.OpReceipt, error) {
	byOp := make(map[string]adapters.OpReceipt, len(rc.Ops))
	for _, or := range rc.Ops {
		if _, dup := byOp[or.OpID]; dup {
			return nil, fmt.Errorf("ingest: reconcile: duplicate receipt op_id %s", or.OpID)
		}
		byOp[or.OpID] = or
	}
	seen := make(map[string]bool, len(cs.Ops))
	for _, op := range cs.Ops {
		if _, ok := byOp[op.OpID]; !ok {
			return nil, fmt.Errorf("ingest: reconcile: no receipt for op %s", op.OpID)
		}
		seen[op.OpID] = true
	}
	for opID := range byOp {
		if !seen[opID] {
			return nil, fmt.Errorf("ingest: reconcile: receipt op_id %s not in changeset", opID)
		}
	}
	return byOp, nil
}

// deriveEID derives a change event's id from the batch's git_head and the
// constructing op's own id. Re-deriving the same pair on a later run
// yields the same eid — the mechanism that makes the whole batch
// idempotent by construction.
func deriveEID(gitHead, opID string) string {
	return gitHead + ":" + opID
}

// checkInvariant1 asserts that every task_created line — across the
// existing journal and the batch under construction — pairs with exactly
// one referent: no two task_created lines may share a for-eid. The
// proposal-slug arm guards only legacy lines already on disk (see
// arch_reconciler.md "Proposal-Epic Ops") — new epic receipts always
// carry for, never proposal. It asserts the same one-referent property,
// in its own namespace, for task_retargeted — a retarget's modified event
// is never the referent of a task_created, so the two kinds are checked
// separately rather than sharing one seen-set. Missing-referent failures
// are caught earlier, during construction (a cleanup with no matching
// removed event, an epic with no registered event, a close-removed with
// no journal entry for its bead) — this pass catches the aggregate
// double-pairing case construction cannot see one op at a time.
func checkInvariant1(existing, batch []mapping.Event) error {
	seenFor := map[string]bool{}
	seenProposal := map[string]bool{}
	seenRetargetFor := map[string]bool{}
	check := func(ev mapping.Event) error {
		switch ev.Event {
		case "task_created":
			if ev.Proposal != "" {
				if seenProposal[ev.Proposal] {
					return fmt.Errorf("ingest: reconcile: invariant 1: proposal %s paired by more than one task_created", ev.Proposal)
				}
				seenProposal[ev.Proposal] = true
				return nil
			}
			if ev.For != "" {
				if seenFor[ev.For] {
					return fmt.Errorf("ingest: reconcile: invariant 1: event %s paired by more than one task_created", ev.For)
				}
				seenFor[ev.For] = true
			}
		case "task_retargeted":
			if ev.For != "" {
				if seenRetargetFor[ev.For] {
					return fmt.Errorf("ingest: reconcile: invariant 1: event %s paired by more than one task_retargeted", ev.For)
				}
				seenRetargetFor[ev.For] = true
			}
		}
		return nil
	}
	for _, ev := range existing {
		if err := check(ev); err != nil {
			return err
		}
	}
	for _, ev := range batch {
		if err := check(ev); err != nil {
			return err
		}
	}
	return nil
}

// checkInvariant2 asserts that every receipt's for-eid, in the batch under
// construction, names an event id present in the journal-plus-batch —
// task_created, task_closed and task_retargeted alike — and that every
// eid a refresh receipt's absorbed list names does too.
func checkInvariant2(existing, batch []mapping.Event) error {
	known := map[string]bool{}
	for _, ev := range existing {
		if ev.EID != "" {
			known[ev.EID] = true
		}
	}
	for _, ev := range batch {
		if ev.EID != "" {
			known[ev.EID] = true
		}
	}
	for _, ev := range batch {
		switch ev.Event {
		case "task_created", "task_closed", "task_retargeted":
			if ev.For != "" && !known[ev.For] {
				return fmt.Errorf("ingest: receipt references unknown event %s", ev.For)
			}
		case "refresh":
			for _, a := range ev.Absorbed {
				if !known[a] {
					return fmt.Errorf("ingest: receipt references unknown event %s", a)
				}
			}
		}
	}
	return nil
}

// checkInvariant5 asserts that every line in the batch validates against
// the journal-line schema before any of it is written.
func checkInvariant5(batch []mapping.Event) error {
	sch, err := getLineSchema()
	if err != nil {
		return err
	}
	for _, ev := range batch {
		raw, err := encodeLine(ev)
		if err != nil {
			return fmt.Errorf("ingest: reconcile: invariant 5: %w", err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("ingest: reconcile: invariant 5: %w", err)
		}
		if err := sch.Validate(doc); err != nil {
			return fmt.Errorf("ingest: reconcile: invariant 5: %s: %w", ev.Event, err)
		}
	}
	return nil
}

var (
	lineSchema     *jsonschema.Schema
	lineSchemaErr  error
	lineSchemaOnce sync.Once
)

// getLineSchema compiles the embedded journal-line schema once and caches
// it. Reconciler owns its own compiled copy rather than reaching into
// mapping's — MappingStore's is a read-time internal, and Reconciler
// (with RefreshHandler) is the format's only writer.
func getLineSchema() (*jsonschema.Schema, error) {
	lineSchemaOnce.Do(func() {
		raw, err := schema.BeadMapSchema()
		if err != nil {
			lineSchemaErr = fmt.Errorf("load journal-line schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			lineSchemaErr = fmt.Errorf("parse journal-line schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("bead-map.schema.json", doc); err != nil {
			lineSchemaErr = fmt.Errorf("add journal-line schema: %w", err)
			return
		}
		lineSchema, lineSchemaErr = c.Compile("bead-map.schema.json")
	})
	return lineSchema, lineSchemaErr
}

// changeEventLine, registeredEventLine, taskReceiptLine and
// taskRetargetedLine mirror the journal-line shapes in
// schema/bead-map.schema.json exactly — changeEventLine always serialises
// its ten required keys (before/after admit null); taskReceiptLine omits
// whichever of for/proposal does not apply, since additionalProperties is
// false on every shape. registeredEventLine has no writer in this
// package — the proposal Registrar appends it through MappingStore
// directly — but Reconciler's tests seed one to exercise the epic-create
// referent lookup, and checkInvariant5 must be able to validate one if it
// ever appeared in a batch.
type changeEventLine struct {
	Event    string  `json:"event"`
	EID      string  `json:"eid"`
	Node     string  `json:"node"`
	Name     string  `json:"name"`
	NodeType string  `json:"node_type"`
	Module   string  `json:"module"`
	Before   *string `json:"before"`
	After    *string `json:"after"`
	GitHead  string  `json:"git_head"`
	Proposal string  `json:"proposal"`
	Path     string  `json:"path,omitempty"`
}

type taskReceiptLine struct {
	Event    string `json:"event"`
	TaskID   string `json:"task_id"`
	For      string `json:"for,omitempty"`
	Proposal string `json:"proposal,omitempty"`
}

// taskRetargetedLine mirrors the taskRetargetedReceipt journal-line shape
// exactly: for is always required (no legacy proposal-slug arm ever
// existed for this kind — see arch_reconciler.md "Retarget Ops") and no
// proposal field is admitted at all.
type taskRetargetedLine struct {
	Event  string `json:"event"`
	TaskID string `json:"task_id"`
	For    string `json:"for"`
}

type registeredEventLine struct {
	Event    string `json:"event"`
	EID      string `json:"eid"`
	Proposal string `json:"proposal"`
	GitHead  string `json:"git_head"`
}

// refreshReceiptLine mirrors the refreshReceipt journal-line shape exactly:
// git_head is nullable (a refresh run with no --git-head records the
// absence as JSON null, not empty string) and absorbed always serialises
// as an array, even when empty — RefreshHandler is this line kind's only
// writer, see arch_refresh.md.
type refreshReceiptLine struct {
	Event    string   `json:"event"`
	GitHead  *string  `json:"git_head"`
	Absorbed []string `json:"absorbed"`
}

// encodeLine renders one mapping.Event as the wire JSON its event kind
// requires. Reconciler and RefreshHandler each use it, via checkInvariant5,
// to schema-validate a batch before committing it through MappingStore's
// Append — the journal's one write path (see arch_reconciler.md "One write
// path, no tracker"). The two components share this encoder so they can
// never drift apart on wire shape.
func encodeLine(ev mapping.Event) ([]byte, error) {
	switch ev.Event {
	case "added", "modified", "removed":
		return json.Marshal(changeEventLine{
			Event: ev.Event, EID: ev.EID, Node: ev.Node, Name: ev.Name,
			NodeType: ev.NodeType, Module: ev.Module, Before: ev.Before, After: ev.After,
			GitHead: ev.GitHead, Proposal: ev.Proposal, Path: ev.Path,
		})
	case "registered":
		return json.Marshal(registeredEventLine{
			Event: ev.Event, EID: ev.EID, Proposal: ev.Proposal, GitHead: ev.GitHead,
		})
	case "task_created", "task_closed":
		return json.Marshal(taskReceiptLine{
			Event: ev.Event, TaskID: ev.TaskID, For: ev.For, Proposal: ev.Proposal,
		})
	case "task_retargeted":
		return json.Marshal(taskRetargetedLine{Event: ev.Event, TaskID: ev.TaskID, For: ev.For})
	case "refresh":
		var gitHead *string
		if ev.GitHead != "" {
			gitHead = strPtr(ev.GitHead)
		}
		absorbed := ev.Absorbed
		if absorbed == nil {
			absorbed = []string{}
		}
		return json.Marshal(refreshReceiptLine{Event: ev.Event, GitHead: gitHead, Absorbed: absorbed})
	default:
		return nil, fmt.Errorf("unknown journal line kind %q", ev.Event)
	}
}

func strPtr(s string) *string { return &s }

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
