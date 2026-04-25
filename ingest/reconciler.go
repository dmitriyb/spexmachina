package ingest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
)

// SpecGraph supplies the spec-side metadata the Reconciler needs to
// materialise new mapping records and to detect orphans (invariant 4).
// IngestCommand wires this from the merkle tree plus the parsed module
// specs; the Reconciler itself depends only on this narrow surface.
type SpecGraph interface {
	HasNode(specNodeID string) bool
	NodeMetadata(specNodeID string) (NodeMetadata, error)
}

// NodeMetadata is the subset of spec-graph data that materialises onto a
// fresh mapping.Record at create time. ContentFile is the canonical path
// of the arch markdown for the node; SpecHash is the merkle leaf hash.
type NodeMetadata struct {
	Module      string
	Component   string
	ContentFile string
	SpecHash    string
	NodeType    string
}

// ReconcileSummary is the per-op tally Reconciler.Apply returns. The CLI
// folds OkCreates+OkCloses into Summary.Ok before serialising to stdout.
type ReconcileSummary struct {
	OkCreates      int
	OkCloses       int
	Skipped        int
	Errors         int
	RecordsAdded   int
	RecordsUpdated int
	RecordsDeleted int
}

// Reconciler consumes a paired changeset+receipts and applies per-op
// state transitions to the mapping store. It enforces invariants 1–5
// and 7 against an in-memory working copy and only commits when every
// invariant holds. Invariant 6 lives in SnapshotSaver; the IngestCommand
// orchestrates both.
type Reconciler struct {
	MappingStore mapping.Store
	SpecGraph    SpecGraph
}

// Apply pairs each changeset op with its receipt, applies the per-op
// transition table to a working copy, asserts invariants, then commits
// atomically via mapping.Store.Replace. The on-disk file is untouched
// on any error.
func (r *Reconciler) Apply(cs emit.Changeset, rc adapters.Receipts) (ReconcileSummary, error) {
	receiptsByOp, err := pairReceipts(cs, rc)
	if err != nil {
		return ReconcileSummary{}, err
	}

	working, err := r.loadWorking()
	if err != nil {
		return ReconcileSummary{}, err
	}

	var sum ReconcileSummary
	for _, op := range cs.Ops {
		opRC := receiptsByOp[op.OpID]
		if err := r.applyOp(working, op, opRC, &sum); err != nil {
			return ReconcileSummary{}, err
		}
	}

	if err := r.assertInvariants(working, cs, receiptsByOp); err != nil {
		return ReconcileSummary{}, err
	}

	if err := r.MappingStore.Replace(working.snapshot()); err != nil {
		return ReconcileSummary{}, fmt.Errorf("ingest: reconcile: commit: %w", err)
	}
	return sum, nil
}

// pairReceipts builds an op_id → OpReceipt index after asserting that
// the changeset and receipts cover exactly the same op_id set. An
// imbalance is a contract violation by emit or the adapter and is
// treated as input error, not invariant failure.
func pairReceipts(cs emit.Changeset, rc adapters.Receipts) (map[string]adapters.OpReceipt, error) {
	receiptsByOp := make(map[string]adapters.OpReceipt, len(rc.Ops))
	for _, or := range rc.Ops {
		if _, dup := receiptsByOp[or.OpID]; dup {
			return nil, fmt.Errorf("ingest: reconcile: duplicate receipt op_id %s", or.OpID)
		}
		receiptsByOp[or.OpID] = or
	}
	for _, op := range cs.Ops {
		if _, ok := receiptsByOp[op.OpID]; !ok {
			return nil, fmt.Errorf("ingest: reconcile: no receipt for op %s", op.OpID)
		}
	}
	if len(receiptsByOp) != len(cs.Ops) {
		seen := make(map[string]bool, len(cs.Ops))
		for _, op := range cs.Ops {
			seen[op.OpID] = true
		}
		for opID := range receiptsByOp {
			if !seen[opID] {
				return nil, fmt.Errorf("ingest: reconcile: receipt op_id %s not in changeset", opID)
			}
		}
	}
	return receiptsByOp, nil
}

// workingCopy is the in-memory mutation buffer Reconciler.Apply works on
// before committing. It indexes records by id and by bead_id so the
// per-op handlers can do their lookups without rescanning a slice.
type workingCopy struct {
	byID     map[int]mapping.Record
	byBeadID map[string]int
	nextID   int
}

func newWorkingCopy(records []mapping.Record, nextID int) *workingCopy {
	wc := &workingCopy{
		byID:     make(map[int]mapping.Record, len(records)),
		byBeadID: make(map[string]int, len(records)),
		nextID:   nextID,
	}
	for _, r := range records {
		wc.byID[r.ID] = r
		if r.BeadID != "" {
			wc.byBeadID[r.BeadID] = r.ID
		}
	}
	return wc
}

func (wc *workingCopy) put(r mapping.Record) {
	if existing, ok := wc.byID[r.ID]; ok {
		if existing.BeadID != "" && existing.BeadID != r.BeadID {
			delete(wc.byBeadID, existing.BeadID)
		}
	}
	wc.byID[r.ID] = r
	if r.BeadID != "" {
		wc.byBeadID[r.BeadID] = r.ID
	}
}

func (wc *workingCopy) deleteByBeadID(beadID string) (mapping.Record, bool) {
	id, ok := wc.byBeadID[beadID]
	if !ok {
		return mapping.Record{}, false
	}
	r := wc.byID[id]
	delete(wc.byID, id)
	delete(wc.byBeadID, beadID)
	return r, true
}

func (wc *workingCopy) advanceCounter(min int) {
	if wc.nextID < min {
		wc.nextID = min
	}
}

func (wc *workingCopy) snapshot() ([]mapping.Record, int) {
	out := make([]mapping.Record, 0, len(wc.byID))
	for _, r := range wc.byID {
		out = append(out, r)
	}
	return out, wc.nextID
}

func (r *Reconciler) loadWorking() (*workingCopy, error) {
	records, err := r.MappingStore.List()
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: load records: %w", err)
	}
	nextID, err := r.MappingStore.NextRecordID()
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: load next-id: %w", err)
	}
	return newWorkingCopy(records, nextID), nil
}

func (r *Reconciler) applyOp(wc *workingCopy, op emit.Op, rc adapters.OpReceipt, sum *ReconcileSummary) error {
	switch rc.Status {
	case adapters.OpStatusError:
		sum.Errors++
		return nil
	case adapters.OpStatusSkipped:
		sum.Skipped++
		return nil
	case adapters.OpStatusOk:
		// fall through
	default:
		return fmt.Errorf("ingest: reconcile: op %s: unknown receipt status %q", op.OpID, rc.Status)
	}

	switch op.Type {
	case emit.OpCreate:
		sum.OkCreates++
		return r.applyCreate(wc, op, rc, sum)
	case emit.OpClose:
		sum.OkCloses++
		return r.applyClose(wc, op, rc, sum)
	case emit.OpLabel, emit.OpTag:
		// Label / tag ops carry no mapping consequence.
		return nil
	default:
		return fmt.Errorf("ingest: reconcile: op %s: unknown type %q", op.OpID, op.Type)
	}
}

// applyCreate handles the four create variants from arch_reconciler.md:
// fresh insert, was_existing idempotent re-match, modified-pair update
// (label re-used by emit), and the duplicate-record-id error case.
func (r *Reconciler) applyCreate(wc *workingCopy, op emit.Op, rc adapters.OpReceipt, sum *ReconcileSummary) error {
	if op.Idempotency == nil {
		return fmt.Errorf("ingest: reconcile: op %s: create has no idempotency label", op.OpID)
	}
	recID, err := parseRecordID(op.Idempotency.Label)
	if err != nil {
		return fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}

	if rc.WasExisting {
		rec, ok := wc.byID[recID]
		if ok {
			if rec.BeadID != rc.BeadID {
				return fmt.Errorf("ingest: reconcile: op %s: was_existing bead_id %s does not match stored %s", op.OpID, rc.BeadID, rec.BeadID)
			}
			// Idempotent no-op: store already in the target state.
			return nil
		}
		// Recovery case: the tracker has a bead under our label (the
		// adapter died last run after creating but before writing its
		// receipt), but the local store has no record. Materialise the
		// record now so the next run does not re-emit the same create.
		// See test_partial_run_recovery.md: "Partial with Adapter-Side
		// Duplicates".
	}

	existing, had := wc.byID[recID]
	if had {
		if existing.SpecNodeID != op.SpecNodeID {
			return fmt.Errorf("ingest: reconcile: op %s: record id %d collision (existing spec_node %s, op spec_node %s)", op.OpID, recID, existing.SpecNodeID, op.SpecNodeID)
		}
		// Modified-node pair: same record-id, new bead_id. Keep the
		// record's identity (id, spec_node_id, content_file) and refresh
		// the volatile fields.
		md, err := r.lookupMetadata(op.SpecNodeID)
		if err != nil {
			return fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
		}
		existing.BeadID = rc.BeadID
		if md.SpecHash != "" {
			existing.SpecHash = md.SpecHash
		}
		if md.NodeType != "" {
			existing.NodeType = md.NodeType
		}
		existing.BeadType = beadTypeFor(op.SpecNodeKind)
		wc.put(existing)
		sum.RecordsUpdated++
		wc.advanceCounter(recID + 1)
		return nil
	}

	md, err := r.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}
	rec := mapping.Record{
		ID:          recID,
		SpecNodeID:  op.SpecNodeID,
		BeadID:      rc.BeadID,
		BeadType:    beadTypeFor(op.SpecNodeKind),
		NodeType:    nonEmpty(md.NodeType, op.SpecNodeKind),
		Module:      md.Module,
		Component:   nonEmpty(md.Component, op.Title),
		ContentFile: md.ContentFile,
		SpecHash:    md.SpecHash,
	}
	wc.put(rec)
	sum.RecordsAdded++
	wc.advanceCounter(recID + 1)
	return nil
}

// applyClose handles the two close-reason discriminations: removed
// (delete record) and modified (no-op; the paired create updates the
// record). Reasons we don't recognise are treated conservatively as
// no-ops at the mapping level — the adapter still recorded the close.
func (r *Reconciler) applyClose(wc *workingCopy, op emit.Op, rc adapters.OpReceipt, sum *ReconcileSummary) error {
	switch {
	case strings.HasPrefix(op.Reason, ReasonRemovedPrefix):
		if op.Target == nil || op.Target.Kind != emit.RefBead || op.Target.BeadID == "" {
			return fmt.Errorf("ingest: reconcile: op %s: close target must be ref:bead", op.OpID)
		}
		if _, ok := wc.deleteByBeadID(op.Target.BeadID); ok {
			sum.RecordsDeleted++
		}
		return nil
	case strings.HasPrefix(op.Reason, ReasonModifiedPrefix):
		// Paired create handles the record update; close is a no-op at
		// the mapping level.
		return nil
	default:
		return nil
	}
}

// lookupMetadata is a small wrapper that turns a missing SpecGraph or a
// missing node into a clear, prefixed error. Reconciler.Apply needs the
// graph for fresh creates and modified-pair updates; tests inject a fake
// to keep this layer pure.
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

// assertInvariants enforces the post-apply consistency rules from
// arch_reconciler.md. Only invariants 1–5 and 7 belong here; invariant
// 6 (snapshot saved iff complete) is enforced by SnapshotSaver. We run
// the rules in numeric order so the first-failing message is also the
// most upstream cause.
func (r *Reconciler) assertInvariants(wc *workingCopy, cs emit.Changeset, receiptsByOp map[string]adapters.OpReceipt) error {
	if err := r.checkInvariant1(wc, cs, receiptsByOp); err != nil {
		return err
	}
	if err := r.checkInvariant2(wc, cs, receiptsByOp); err != nil {
		return err
	}
	if err := r.checkInvariant3(wc, cs, receiptsByOp); err != nil {
		return err
	}
	if err := r.checkInvariant4(wc); err != nil {
		return err
	}
	if err := r.checkInvariant5(wc); err != nil {
		return err
	}
	// Invariant 7 is enforced by mapping.Store.Replace, which validates
	// the candidate state against the bead-map JSON Schema before
	// renaming the temp file into place.
	return nil
}

func (r *Reconciler) checkInvariant1(wc *workingCopy, cs emit.Changeset, receiptsByOp map[string]adapters.OpReceipt) error {
	for _, op := range cs.Ops {
		if op.Type != emit.OpCreate {
			continue
		}
		rc := receiptsByOp[op.OpID]
		if rc.Status != adapters.OpStatusOk {
			continue
		}
		if op.Idempotency == nil {
			continue
		}
		recID, err := parseRecordID(op.Idempotency.Label)
		if err != nil {
			return fmt.Errorf("ingest: reconcile: invariant 1: op %s: %w", op.OpID, err)
		}
		rec, ok := wc.byID[recID]
		if !ok {
			return fmt.Errorf("ingest: reconcile: invariant 1: ok create op %s has no mapping record for spec_node %s", op.OpID, op.SpecNodeID)
		}
		if rec.BeadID != rc.BeadID {
			return fmt.Errorf("ingest: reconcile: invariant 1: op %s record %d points to bead_id %s, expected %s", op.OpID, recID, rec.BeadID, rc.BeadID)
		}
	}
	return nil
}

func (r *Reconciler) checkInvariant2(wc *workingCopy, cs emit.Changeset, receiptsByOp map[string]adapters.OpReceipt) error {
	for _, op := range cs.Ops {
		if op.Type != emit.OpClose {
			continue
		}
		rc := receiptsByOp[op.OpID]
		if rc.Status != adapters.OpStatusOk {
			continue
		}
		if !strings.HasPrefix(op.Reason, ReasonRemovedPrefix) {
			continue
		}
		if op.Target == nil {
			continue
		}
		if _, lingering := wc.byBeadID[op.Target.BeadID]; lingering {
			return fmt.Errorf("ingest: reconcile: invariant 2: removed bead %s still has mapping record", op.Target.BeadID)
		}
	}
	return nil
}

func (r *Reconciler) checkInvariant3(wc *workingCopy, cs emit.Changeset, receiptsByOp map[string]adapters.OpReceipt) error {
	// Index closes-on-modified by their target bead_id so we can
	// confirm the paired create rebound the record off it.
	closedModified := make(map[string]bool)
	for _, op := range cs.Ops {
		if op.Type != emit.OpClose {
			continue
		}
		if !strings.HasPrefix(op.Reason, ReasonModifiedPrefix) {
			continue
		}
		if receiptsByOp[op.OpID].Status != adapters.OpStatusOk {
			continue
		}
		if op.Target == nil {
			continue
		}
		closedModified[op.Target.BeadID] = true
	}
	for old := range closedModified {
		if _, ok := wc.byBeadID[old]; ok {
			return fmt.Errorf("ingest: reconcile: invariant 3: modified bead %s record still points to old bead_id", old)
		}
	}
	return nil
}

func (r *Reconciler) checkInvariant4(wc *workingCopy) error {
	if r.SpecGraph == nil {
		return nil
	}
	for _, rec := range wc.byID {
		// Proposal-epic records reference the proposal ref, not a spec
		// node hash, so they are exempt from the spec-graph orphan
		// check by design.
		if rec.NodeType == "proposal" {
			continue
		}
		if !r.SpecGraph.HasNode(rec.SpecNodeID) {
			return fmt.Errorf("ingest: reconcile: invariant 4: orphan record for spec_node %s (bead %s)", rec.SpecNodeID, rec.BeadID)
		}
	}
	return nil
}

func (r *Reconciler) checkInvariant5(wc *workingCopy) error {
	bySpecNode := make(map[string]int)
	for _, rec := range wc.byID {
		// Proposal-epic records intentionally share spec_node_id across
		// runs (one per apply); see fileStore.Create which already
		// exempts them. Skip duplicates here for the same reason.
		if rec.NodeType == "proposal" {
			continue
		}
		bySpecNode[rec.SpecNodeID]++
	}
	for id, n := range bySpecNode {
		if n > 1 {
			return fmt.Errorf("ingest: reconcile: invariant 5: duplicate records for spec_node %s (%d copies)", id, n)
		}
	}
	return nil
}

// parseRecordID strips the "spex:" prefix from an idempotency label and
// returns the integer record-id. A malformed label is a contract
// violation by emit's Labeler, not user input.
func parseRecordID(label string) (int, error) {
	const prefix = adapters.IdempotencyLabelPrefix
	if !strings.HasPrefix(label, prefix) {
		return 0, fmt.Errorf("idempotency label %q missing %q prefix", label, prefix)
	}
	rest := strings.TrimPrefix(label, prefix)
	id, err := strconv.Atoi(rest)
	if err != nil {
		return 0, fmt.Errorf("idempotency label %q: %w", label, err)
	}
	if id < 0 {
		return 0, fmt.Errorf("idempotency label %q: negative record id", label)
	}
	return id, nil
}

// beadTypeFor maps emit's SpecNodeKind vocabulary onto the bead-type
// vocabulary the tracker expects.
func beadTypeFor(specNodeKind string) string {
	switch specNodeKind {
	case "proposal_epic":
		return "epic"
	case "data_flow":
		return "feature"
	case "test_section":
		return "task"
	case "component":
		return "feature"
	case "":
		return ""
	default:
		return "task"
	}
}

func nonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}
