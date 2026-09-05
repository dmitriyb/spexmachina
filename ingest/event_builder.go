package ingest

import (
	"fmt"
	"strings"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
)

// EventBuilderState is the per-run state every EventBuilder construction
// path shares — the eid predicate over journal and in-flight batch, the
// journal fold and the registered-by-stem map. Reconciler assembles it
// once per run and hands it to EventBuilder at construction, instead of
// threading it through every call. See spec/ingest/arch_event_builder.md,
// "Per-Run State".
type EventBuilderState struct {
	// SpecGraph resolves a fresh or modified node's current name, kind and
	// module — the identity a change event must carry, because the event
	// is the only record of it once the node is gone. Cleanup and
	// proposal-epic creates never reach it.
	SpecGraph SpecGraph
	// Fold is the journal's live pairings, epic slugs and legacy lines, as
	// of the start of this run.
	Fold mapping.Fold
	// RegisteredByStem maps a proposal stem to its `registered` event's
	// eid, for proposal-epic creates.
	RegisteredByStem map[string]string
	// HasEID answers whether an eid is already present, in the journal or
	// in the batch constructed so far. It is mutated as the batch grows,
	// so a mid-batch collision is caught exactly as a journal-side
	// duplicate — the mechanism that makes the batch idempotent.
	HasEID func(eid string) bool
}

// EventBuilder constructs journal lines per action class from an op (or
// absorbed entry) and its receipt: the create paths (node-bearing,
// cleanup, epic), retargets, closes (removal and fold-back) and absorbed
// entries — every receipt references an event. Reconciler builds one
// per run and calls it once per op. See
// spec/ingest/arch_event_builder.md and requirements 539030e8c5a4,
// 7191a50f7447, 7900dcd38c4a, fd6f08ef34fa in spec/ingest/module.json.
type EventBuilder struct {
	State EventBuilderState
}

// NewEventBuilder constructs an EventBuilder over the given per-run
// state. See EventBuilderState and arch_event_builder.md's "Per-Run
// State".
func NewEventBuilder(state EventBuilderState) *EventBuilder {
	return &EventBuilder{State: state}
}

// BuildCreate answers the journal lines an ok create op implies,
// discriminating spec_node_kind (proposal_epic, cleanup, other) before
// consulting the spec graph. See "Node-Bearing Creates", "Proposal-Epic
// Ops" and "Cleanup-Create Ops" in arch_event_builder.md.
func (b *EventBuilder) BuildCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	switch op.SpecNodeKind {
	case plan.KindProposalEpic:
		return buildEpicCreate(op, receipt, b.State.Fold, b.State.RegisteredByStem)
	case plan.KindCleanup:
		return b.buildCleanupCreate(cs, op, receipt)
	default:
		return b.buildNodeCreate(cs, op, receipt)
	}
}

// BuildClose answers the journal lines an ok close op implies: the
// removed event plus task_closed on a "Spec node removed" reason, or the
// modified event plus task_closed — built from the close alone, with no
// task_created — on a "Spec node modified" reason (the test_section
// fold-back shape; see "Fold-Back Closes" in arch_event_builder.md).
func (b *EventBuilder) BuildClose(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	switch {
	case strings.HasPrefix(op.Reason, ReasonRemovedPrefix):
		return b.buildRemoved(cs, op, receipt)
	case strings.HasPrefix(op.Reason, ReasonModifiedPrefix):
		if op.Target == nil || op.Target.Kind != plan.RefTask || op.Target.TaskID == "" {
			return nil, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:task", op.OpID)
		}
		return b.buildModifiedFromClose(cs, op, receipt)
	default:
		return nil, nil
	}
}

// BuildRetarget answers the modified event plus task_retargeted receipt
// an ok retarget op implies. The eid derives from the retarget op's own
// id exactly as a create op's does; node identity and the after-hash
// come straight off the op rather than the spec graph — a retarget
// carries no cleanup or proposal-epic shape to discriminate first. No
// bead dies and none is born: task_id is the existing task the op
// already targets. See "Retarget Ops" in arch_event_builder.md.
func (b *EventBuilder) BuildRetarget(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != plan.RefTask || op.Target.TaskID == "" {
		return nil, fmt.Errorf("ingest: reconcile: op %s: retarget target must be ref:task", op.OpID)
	}
	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	md, err := b.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}

	ev := mapping.Event{
		Event:    "modified",
		EID:      eid,
		Node:     op.SpecNodeID,
		Name:     nonEmpty(md.Component, ""),
		NodeType: nonEmpty(md.NodeType, ""),
		Module:   md.Module,
		Before:   b.priorHash(op.SpecNodeID),
		After:    strPtr(op.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	retargeted := mapping.Event{Event: "task_retargeted", TaskID: op.Target.TaskID, For: eid}
	return []mapping.Event{ev, retargeted}, nil
}

// BuildAbsorbed answers one modified event per entry in the changeset's
// top-level absorbed array, plus the single refresh receipt naming the
// batch's newly-built absorbed eids. Absorbed entries are not
// receipt-gated — they describe spec state, not tracker work — so this
// is called unconditionally, regardless of what the batch's ops found.
// An empty absorbed array, or one whose every derived eid the predicate
// already answers for, constructs nothing, not an empty receipt. See
// "Absorbed Entries" in arch_event_builder.md.
func (b *EventBuilder) BuildAbsorbed(cs plan.Changeset) ([]mapping.Event, error) {
	var lines []mapping.Event
	var eids []string
	for _, entry := range cs.Absorbed {
		before, after := entry.Before, entry.After
		eid := deriveRefreshEID(entry.Node, &before, &after)
		if b.State.HasEID(eid) {
			continue
		}
		md, err := b.lookupMetadata(entry.Node)
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

// buildNodeCreate builds the change event plus its task_created for a
// node-bearing create — not a cleanup, not an epic. The event's kind is
// derived from the journal fold's latest change event for the node, not
// carried by the op: no entry at all, or one whose after is null (the
// node was removed and is now coming back), yields an added event with
// before null; an entry carrying an after hash yields a modified event
// with that hash as before. A node's earlier pairing (if any) is left
// exactly as it is — nothing closes it, because the journal never
// records a task's completion. See "Node-Bearing Creates" in
// arch_event_builder.md.
func (b *EventBuilder) buildNodeCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	md, err := b.lookupMetadata(op.SpecNodeID)
	if err != nil {
		return nil, fmt.Errorf("ingest: reconcile: op %s: %w", op.OpID, err)
	}
	before := b.priorHash(op.SpecNodeID)
	kind := "added"
	if before != nil {
		kind = "modified"
	}
	ev := mapping.Event{
		Event:    kind,
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
	created := mapping.Event{Event: "task_created", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{ev, created}, nil
}

// buildRemoved builds the removed event plus its task_closed for an ok
// close whose reason is "Spec node removed". The node's identity and
// biography (name, node_type, module, path, last content hash) come
// from the journal's live fold entry for the bead being closed — the
// spec no longer carries the node, so this is the only place left to
// ask.
func (b *EventBuilder) buildRemoved(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	if op.Target == nil || op.Target.Kind != plan.RefTask || op.Target.TaskID == "" {
		return nil, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:task", op.OpID)
	}
	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	entry, found := b.foldEntryByTask(op.Target.TaskID)
	if !found {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: no journal entry for bead %s", op.OpID, op.Target.TaskID)
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
	closed := mapping.Event{Event: "task_closed", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{ev, closed}, nil
}

// buildModifiedFromClose builds the modified event plus its task_closed
// for an ok "Spec node modified" close — the shape ActionClassifier
// emits for a coupled test_section edit whose section's coverage folds
// into another component, cancelling the section's own open task. The
// node still exists post-edit, so its current metadata comes from the
// spec graph; its identity and prior hash come from the journal's live
// fold entry for the bead being closed, exactly as buildRemoved resolves
// a removed node's identity. No task_created is built — there is no
// successor task to pair. See "Fold-Back Closes" in
// arch_event_builder.md.
func (b *EventBuilder) buildModifiedFromClose(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	entry, found := b.foldEntryByTask(op.Target.TaskID)
	if !found {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: no journal entry for bead %s", op.OpID, op.Target.TaskID)
	}
	md, err := b.lookupMetadata(entry.Key)
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
	closed := mapping.Event{Event: "task_closed", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{ev, closed}, nil
}

// buildEpicCreate builds the one-line receipt a proposal-epic create
// implies: a task_created whose for names the proposal's registered event.
// Dedup is fold-based, not eid-based: an epic's task_created has no change
// event of its own to key an eid off, so a re-run recognises an existing
// epic by its slug already appearing in the fold, keyed there off the
// registered event's referent. A stem with no registered event in the
// journal is a malformed changeset — plan refuses to build such an op, so
// its arrival here is an invariant failure, not a fallback. See
// arch_event_builder.md "Proposal-Epic Ops".
func buildEpicCreate(op plan.Op, receipt adapters.OpReceipt, fold mapping.Fold, registeredByStem map[string]string) ([]mapping.Event, error) {
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
	return []mapping.Event{{Event: "task_created", TaskID: receipt.TaskID, For: eid}}, nil
}

// buildCleanupCreate builds the journal lines a cleanup create implies.
// It discriminates on the op's spec node kind before anything else — the
// spec graph no longer holds the node, so only the fold is consulted.
// The referent is resolved from the node's latest change event: if that
// event is already a `removed` one — the removal landed in an earlier
// batch whose cleanup never did — the task_created's for names its eid
// and no new change event is built; otherwise the cleanup mints the
// removal itself, from its own (git_head, op_id), with before taken from
// the latest event's after and identity from the fold entry — the
// biography that outlives the node. A hash the journal has never seen at
// all is a malformed changeset, not a fallback. See "Cleanup-Create Ops"
// in arch_event_builder.md.
func (b *EventBuilder) buildCleanupCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	hash := op.SpecNodeID
	entry, found := b.foldEntryByKey(hash)
	if !found {
		return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: cleanup for spec_node %s matches no journal history", op.OpID, hash)
	}
	if entry.Removed {
		if entry.TaskID != "" {
			return nil, nil // idempotent no-op: cleanup already landed for this removal
		}
		return []mapping.Event{{Event: "task_created", TaskID: receipt.TaskID, For: entry.Source.EID}}, nil
	}

	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	removed := mapping.Event{
		Event:    "removed",
		EID:      eid,
		Node:     hash,
		Name:     entry.Source.Name,
		NodeType: entry.Source.NodeType,
		Module:   entry.Source.Module,
		Before:   entry.Source.After,
		After:    nil,
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     entry.Source.Path,
	}
	created := mapping.Event{Event: "task_created", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{removed, created}, nil
}

// foldEntryByKey finds the fold entry currently reachable by a node
// identity hash — the entry a cleanup create's spec_node_id resolves to,
// whether the node is still live or was already removed.
func (b *EventBuilder) foldEntryByKey(key string) (mapping.FoldEntry, bool) {
	for _, e := range b.State.Fold.Entries {
		if e.Key == key {
			return e, true
		}
	}
	return mapping.FoldEntry{}, false
}

// foldEntryByTask finds the fold entry currently reachable by taskID —
// the entry a close op's target bead resolves to, whether the node is
// still live or was already removed.
func (b *EventBuilder) foldEntryByTask(taskID string) (mapping.FoldEntry, bool) {
	for _, e := range b.State.Fold.Entries {
		if e.TaskID == taskID {
			return e, true
		}
	}
	return mapping.FoldEntry{}, false
}

// priorHash answers specNodeID's content hash immediately before this
// batch, off the journal's live fold entry — nil when the node has no
// live entry yet (a genuinely new node) or when its latest entry is a
// removal (After is null there too).
func (b *EventBuilder) priorHash(specNodeID string) *string {
	entry, found := b.foldEntryByKey(specNodeID)
	if !found {
		return nil
	}
	return entry.Source.After
}

// lookupMetadata turns a missing SpecGraph or a missing node into a
// clearly wrapped error. Node-bearing creates, modified-from-close
// builds, retargets and absorbed entries call this — cleanup and
// proposal_epic creates skip the spec graph entirely.
func (b *EventBuilder) lookupMetadata(specNodeID string) (NodeMetadata, error) {
	if b.State.SpecGraph == nil {
		return NodeMetadata{}, fmt.Errorf("spec graph not configured")
	}
	md, err := b.State.SpecGraph.NodeMetadata(specNodeID)
	if err != nil {
		return NodeMetadata{}, fmt.Errorf("spec graph: %w", err)
	}
	return md, nil
}
