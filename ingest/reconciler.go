package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
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
func (r *Reconciler) Apply(cs emit.Changeset, rc adapters.Receipts) (ReconcileSummary, error) {
	receiptsByOp, err := pairReceipts(cs, rc)
	if err != nil {
		return ReconcileSummary{}, err
	}

	store := mapping.NewMappingStore(r.SpecDir)
	existing, err := store.Parse()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}
	fold, err := store.List()
	if err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: read journal: %w", err)
	}

	existingEIDs := make(map[string]bool, len(existing))
	for _, ev := range existing {
		if ev.EID != "" {
			existingEIDs[ev.EID] = true
		}
	}
	batchEIDs := map[string]bool{}
	hasEID := func(eid string) bool { return existingEIDs[eid] || batchEIDs[eid] }

	removals := sameBatchRemovals(cs, receiptsByOp, fold)

	var (
		sum             ReconcileSummary
		batch           []mapping.Event
		modifiedHandled = map[string]bool{}
		pendingModified = map[string]emit.Op{}
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
		case emit.OpLabel, emit.OpTag:
			// Labels and tags carry no journal consequence and no tally.

		case emit.OpClose:
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
					if op.Target == nil || op.Target.Kind != emit.RefBead || op.Target.BeadID == "" {
						return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:bead", op.OpID)
					}
					// Emit orders the paired create before this close (see
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

		case emit.OpCreate:
			switch opRC.Status {
			case adapters.OpStatusOk:
				sum.OkCreates++
				lines, err := r.buildCreate(cs, op, opRC, fold, modifiedHandled, removals, hasEID)
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
		op := pendingModified[beadIDs[0]]
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: op %s: modified close for bead %s has no paired create in this batch", op.OpID, beadIDs[0])
	}

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
		if err := appendJournal(r.journalPath(), batch); err != nil {
			return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: commit: %w", err)
		}
	}

	for _, ev := range batch {
		switch ev.Event {
		case "added", "modified", "removed":
			sum.EventsAppended++
		case "task_created", "task_closed":
			sum.ReceiptsAppended++
		}
	}
	return sum, nil
}

func (r *Reconciler) journalPath() string {
	return filepath.Join(r.SpecDir, ".history.jsonl")
}

// buildCreate discriminates a create op's spec_node_kind before anything
// else, per arch_reconciler.md — a proposal-epic's spec_node_id is a
// proposal stem, not an identity hash, and a cleanup's spec_node_id names
// an already-removed node; neither may reach the spec graph.
func (r *Reconciler) buildCreate(cs emit.Changeset, op emit.Op, opRC adapters.OpReceipt, fold mapping.Fold, modifiedHandled map[string]bool, removals map[string]string, hasEID func(string) bool) ([]mapping.Event, error) {
	switch op.SpecNodeKind {
	case "proposal_epic":
		return buildEpicCreate(op, opRC, fold), nil
	case "cleanup":
		return buildCleanupCreate(op, opRC, fold, removals)
	default:
		if oldBeadID, ok := blocksDepBeadID(op); ok {
			// Emit orders this create before the close that retires
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
// emit placed the cleanup's create before or after its own removal close
// (real changesets always put it before — see arch_reconciler.md
// "Ordering"). Node hash comes from the fold's live entry for the close's
// target bead, exactly as buildRemoved resolves it.
func sameBatchRemovals(cs emit.Changeset, receiptsByOp map[string]adapters.OpReceipt, fold mapping.Fold) map[string]string {
	out := map[string]string{}
	for _, op := range cs.Ops {
		if op.Type != emit.OpClose || !strings.HasPrefix(op.Reason, ReasonRemovedPrefix) {
			continue
		}
		if receiptsByOp[op.OpID].Status != adapters.OpStatusOk {
			continue
		}
		if op.Target == nil || op.Target.Kind != emit.RefBead || op.Target.BeadID == "" {
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
// implies. Dedup is fold-based, not eid-based: an epic's task_created has
// no change event to key an eid off, so a re-run recognises an existing
// epic by its slug already appearing in the fold. See arch_reconciler.md
// "Proposal-Epic Ops".
func buildEpicCreate(op emit.Op, opRC adapters.OpReceipt, fold mapping.Fold) []mapping.Event {
	stem := op.SpecNodeID
	for _, e := range fold.Entries {
		if e.Key == stem {
			return nil // idempotent no-op: the epic already exists
		}
	}
	return []mapping.Event{{Event: "task_created", TaskID: opRC.BeadID, Proposal: stem}}
}

// buildCleanupCreate builds the one-line receipt a cleanup create implies:
// a task_created whose for names the removed node's own removal event.
// The journal is checked first — a live (not yet removed) fold entry for
// the hash means the removal is still pending in this same batch, so it
// falls through to removals, the precomputed same-batch removal map (see
// sameBatchRemovals); a hash that matches neither is a malformed
// changeset, not a fallback. See arch_reconciler.md "Cleanup-Create Ops".
func buildCleanupCreate(op emit.Op, opRC adapters.OpReceipt, fold mapping.Fold, removals map[string]string) ([]mapping.Event, error) {
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
func (r *Reconciler) buildFreshCreate(cs emit.Changeset, op emit.Op, opRC adapters.OpReceipt, hasEID func(string) bool) ([]mapping.Event, error) {
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
func (r *Reconciler) buildModifiedPair(cs emit.Changeset, op emit.Op, opRC adapters.OpReceipt, oldBeadID string, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
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
func (r *Reconciler) buildRemoved(cs emit.Changeset, op emit.Op, opRC adapters.OpReceipt, fold mapping.Fold, hasEID func(string) bool) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != emit.RefBead || op.Target.BeadID == "" {
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

// blocksDepBeadID reports the old bead id an op's lineage dep names, if
// any. ChangesetBuilder attaches this dep to every create replacing an
// obsoleted bead — cleanup and modify-pair creates alike — so its
// presence alone does not select modify-pair handling; buildCreate only
// consults it once cleanup and proposal_epic have been ruled out.
func blocksDepBeadID(op emit.Op) (string, bool) {
	for _, d := range op.Deps {
		if d.Kind == emit.RefBead && d.EdgeType == "blocks" {
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
// a contract violation by emit or the adapter and is treated as input
// error, not invariant failure.
func pairReceipts(cs emit.Changeset, rc adapters.Receipts) (map[string]adapters.OpReceipt, error) {
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
// one referent: no two task_created lines may share a for-eid or a
// proposal slug. Missing-referent failures are caught earlier, during
// construction (a cleanup with no matching removed event, a close-removed
// with no journal entry for its bead) — this pass catches the aggregate
// double-pairing case construction cannot see one op at a time.
func checkInvariant1(existing, batch []mapping.Event) error {
	seenFor := map[string]bool{}
	seenProposal := map[string]bool{}
	check := func(ev mapping.Event) error {
		if ev.Event != "task_created" {
			return nil
		}
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
// construction, names an event id present in the journal-plus-batch.
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
		if (ev.Event == "task_created" || ev.Event == "task_closed") && ev.For != "" {
			if !known[ev.For] {
				return fmt.Errorf("ingest: receipt references unknown event %s", ev.For)
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

// changeEventLine and taskReceiptLine mirror the two journal-line shapes
// in schema/bead-map.schema.json exactly — changeEventLine always
// serialises its ten required keys (before/after admit null);
// taskReceiptLine omits whichever of for/proposal does not apply, since
// additionalProperties is false on both shapes.
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

// encodeLine renders one mapping.Event as the wire JSON its event kind
// requires. It is used both to append new lines and, via checkInvariant5,
// to schema-validate them before the append commits.
func encodeLine(ev mapping.Event) ([]byte, error) {
	switch ev.Event {
	case "added", "modified", "removed":
		return json.Marshal(changeEventLine{
			Event: ev.Event, EID: ev.EID, Node: ev.Node, Name: ev.Name,
			NodeType: ev.NodeType, Module: ev.Module, Before: ev.Before, After: ev.After,
			GitHead: ev.GitHead, Proposal: ev.Proposal, Path: ev.Path,
		})
	case "task_created", "task_closed":
		return json.Marshal(taskReceiptLine{
			Event: ev.Event, TaskID: ev.TaskID, For: ev.For, Proposal: ev.Proposal,
		})
	default:
		return nil, fmt.Errorf("unknown journal line kind %q", ev.Event)
	}
}

// appendJournal appends lines to the journal at path, preserving existing
// bytes verbatim: read what is there, append the newly encoded lines,
// write to a temp file and rename into place. A crash before rename
// leaves the destination untouched, and a re-run that finds nothing new
// to append never calls this at all — see Reconciler.Apply.
func appendJournal(path string, lines []mapping.Event) error {
	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read %s: %w", path, err)
		}
		existing = nil
	}

	var buf bytes.Buffer
	buf.Write(existing)
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	for _, ev := range lines {
		raw, err := encodeLine(ev)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}

	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func strPtr(s string) *string { return &s }

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
