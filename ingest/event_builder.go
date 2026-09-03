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
// journal fold, the same-batch removals, the registered-by-stem map and
// the modified-handled set. Reconciler assembles it once per run and
// hands it to EventBuilder at construction, instead of threading it
// through every call. See spec/ingest/arch_event_builder.md, "Per-Run
// State".
type EventBuilderState struct {
	// SpecGraph resolves a fresh or modified node's current name, kind and
	// module — the identity a change event must carry, because the event
	// is the only record of it once the node is gone. Cleanup and
	// proposal-epic creates never reach it.
	SpecGraph SpecGraph
	// Fold is the journal's live pairings, epic slugs and legacy lines, as
	// of the start of this run.
	Fold mapping.Fold
	// SameBatchRemovals maps a node identity hash this batch's ok
	// "Spec node removed" closes will retire to the eid their removed
	// event will carry — resolved before any op is processed, so a
	// cleanup create resolves its referent regardless of op order.
	SameBatchRemovals map[string]string
	// RegisteredByStem maps a proposal stem to its `registered` event's
	// eid, for proposal-epic creates.
	RegisteredByStem map[string]string
	// ModifiedHandled records which beads' modified pairs an earlier
	// create in this batch has already built, so the paired close
	// constructs nothing twice.
	ModifiedHandled map[string]bool
	// HasEID answers whether an eid is already present, in the journal or
	// in the batch constructed so far. It is mutated as the batch grows,
	// so a mid-batch collision is caught exactly as a journal-side
	// duplicate — the mechanism that makes the batch idempotent.
	HasEID func(eid string) bool
}

// EventBuilder constructs journal lines per action class from an op (or
// absorbed entry) and its receipt: the create paths (plain, cleanup,
// epic), the modified-node pair, retargets, removal closes and absorbed
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
	if state.ModifiedHandled == nil {
		state.ModifiedHandled = map[string]bool{}
	}
	return &EventBuilder{State: state}
}

// BuildCreate answers the journal lines an ok create op implies,
// discriminating spec_node_kind (proposal_epic, cleanup, other) before
// consulting the spec graph. A plain create whose `blocks` dep names an
// old bead is the modified-node pair's create half — it builds the
// modified event and both receipts in one call, off the dep, without
// waiting for the paired close (see "The Modified-Node Pair"). See
// "Proposal-Epic Ops" and "Cleanup-Create Ops" in arch_event_builder.md.
//
// TODO(bead:spexmachina-swvx.22): "The Modified-Node Pair" this default
// case builds via blocksDepBeadID is retired in the current
// arch_event_builder.md ("Node-Bearing Creates" / "Fold-Back Closes"):
// a node-bearing create no longer carries a `blocks` dep at all, and
// added-vs-modified is derived instead from the journal fold's latest
// change event for op.SpecNodeID (no event, or one with a null after,
// means added; one with an after hash means modified, with that hash as
// before). A superseded task gets no task_closed — the fold-back close
// path below covers the one case that still needs one. This is the
// handoff EventBuilder's own bead consumes; do not build it here.
func (b *EventBuilder) BuildCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	switch op.SpecNodeKind {
	case plan.KindProposalEpic:
		return buildEpicCreate(op, receipt, b.State.Fold, b.State.RegisteredByStem)
	case plan.KindCleanup:
		return buildCleanupCreate(op, receipt, b.State.Fold, b.State.SameBatchRemovals)
	default:
		if oldBeadID, ok := blocksDepBeadID(op); ok {
			// Recorded so a later call to BuildClose for oldBeadID's close
			// op short-circuits without constructing anything — the pair
			// is already built. See BuildClose.
			b.State.ModifiedHandled[oldBeadID] = true
			return b.buildModifiedPair(cs, op, receipt, oldBeadID)
		}
		return b.buildFreshCreate(cs, op, receipt)
	}
}

// BuildClose answers the journal lines an ok close op implies: the
// removed event plus task_closed on a "Spec node removed" reason, or —
// when no create in the batch claimed the bead — the modified event
// plus task_closed on a "Spec node modified" reason. When a create in
// the batch does claim the bead (via a `blocks` dep), this call
// constructs nothing: either that create already built the whole pair,
// recognised here via EventBuilderState.ModifiedHandled (ok), or it
// errored/was skipped — never reaching BuildCreate, so ModifiedHandled
// stays unset — and the claim is instead recognised by the static
// changeset scan, so the pair stays incomplete for this partial run —
// either way there is nothing left for the close to add. See "The
// Modified-Node Pair" in arch_event_builder.md.
//
// TODO(bead:spexmachina-swvx.22): per the current arch_event_builder.md
// ("Fold-Back Closes"), the ModifiedHandled/claimedByCreate discrimination
// below is retired along with the modified-node pair: a "Spec node
// modified" close always builds its own modified event plus task_closed,
// unconditionally — no create in the same batch ever claims it, because
// node-bearing creates no longer carry a lineage dep. Once BuildCreate
// stops populating ModifiedHandled, this case reduces to always calling
// buildModifiedFromClose.
func (b *EventBuilder) BuildClose(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	switch {
	case strings.HasPrefix(op.Reason, ReasonRemovedPrefix):
		return b.buildRemoved(cs, op, receipt)
	case strings.HasPrefix(op.Reason, ReasonModifiedPrefix):
		if op.Target == nil || op.Target.Kind != plan.RefTask || op.Target.TaskID == "" {
			return nil, fmt.Errorf("ingest: reconcile: op %s: close target must be ref:task", op.OpID)
		}
		if b.State.ModifiedHandled[op.Target.TaskID] {
			return nil, nil
		}
		if claimedByCreate(cs, op.Target.TaskID) {
			return nil, nil
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

// buildFreshCreate builds the added event plus its task_created for a
// plain new node — not a cleanup, not an epic, not paired with a close.
func (b *EventBuilder) buildFreshCreate(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt) ([]mapping.Event, error) {
	eid := deriveEID(cs.GitHead, op.OpID)
	if b.State.HasEID(eid) {
		return nil, nil
	}
	md, err := b.lookupMetadata(op.SpecNodeID)
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
	created := mapping.Event{Event: "task_created", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{ev, created}, nil
}

// buildModifiedPair builds the one modified event plus both receipts
// for a create+close pair replacing the same node identity. The eid
// derives from the create op's own id — the pair is one event, not two
// — and the before-hash comes from the node's current live fold entry.
// oldBeadID is the retiring task, read off the create op's own `blocks`
// dep by the caller — the paired close need not have been processed
// yet. See "The Modified-Node Pair" in arch_event_builder.md.
func (b *EventBuilder) buildModifiedPair(cs plan.Changeset, op plan.Op, receipt adapters.OpReceipt, oldBeadID string) ([]mapping.Event, error) {
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
		Name:     nonEmpty(md.Component, op.Title),
		NodeType: nonEmpty(md.NodeType, op.SpecNodeKind),
		Module:   md.Module,
		Before:   b.priorHash(op.SpecNodeID),
		After:    strPtr(md.SpecHash),
		GitHead:  cs.GitHead,
		Proposal: cs.Proposal,
		Path:     md.ContentFile,
	}
	closed := mapping.Event{Event: "task_closed", TaskID: oldBeadID, For: eid}
	created := mapping.Event{Event: "task_created", TaskID: receipt.TaskID, For: eid}
	return []mapping.Event{ev, closed, created}, nil
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
// for an ok "Spec node modified" close whose bead no create op in the
// batch ever claimed via a `blocks` dep — the shape ActionClassifier
// emits for a coupled test_section edit (an obsolete action with no
// replacement create). The node still exists post-edit, so its current
// metadata comes from the spec graph; its identity and prior hash come
// from the journal's live fold entry for the bead being closed, exactly
// as buildRemoved resolves a removed node's identity. No task_created
// is built — there is no successor task to pair. See "The Modified-Node
// Pair" in arch_event_builder.md.
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

// claimedByCreate reports whether any create op in the changeset — ok,
// error or skipped alike — claims beadID via a `blocks` dep. BuildClose
// falls back to this once EventBuilderState.ModifiedHandled has nothing
// to say — the errored/skipped-create case, since such a create never
// reaches BuildCreate and so never sets ModifiedHandled: the answer is
// a static property of the changeset, not of how much of the batch has
// been processed so far, so it does not matter whether the claiming
// create ran before or after this close in op order.
func claimedByCreate(cs plan.Changeset, beadID string) bool {
	for _, op := range cs.Ops {
		if op.Type != plan.OpCreate {
			continue
		}
		if oldBeadID, ok := blocksDepBeadID(op); ok && oldBeadID == beadID {
			return true
		}
	}
	return false
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

// buildCleanupCreate builds the one-line receipt a cleanup create implies:
// a task_created whose for names the removed node's own removal event.
// The journal is checked first — a live (not yet removed) fold entry for
// the hash means the removal is still pending in this same batch, so it
// falls through to sameBatchRemovals; a hash that matches neither is a
// malformed changeset, not a fallback. See arch_event_builder.md
// "Cleanup-Create Ops".
//
// TODO(bead:spexmachina-swvx.22): per the current arch_event_builder.md
// "Cleanup-Create Ops", a cleanup create no longer waits on a same-batch
// removal close at all — resolve the referent from the node's latest
// change event in the fold: if it is already a `removed` event, name its
// eid (the case sameBatchRemovals covers today); otherwise the cleanup
// itself mints the removal — one `removed` change event with eid derived
// from the cleanup op's own (git_head, op_id), before taken from the
// latest event's after, after null, identity from the fold entry —
// followed by the task_created naming it. A hash the journal has never
// seen at all stays an invariant failure. Once this lands,
// EventBuilderState.SameBatchRemovals and Reconciler's sameBatchRemovals
// helper have no remaining caller.
func buildCleanupCreate(op plan.Op, receipt adapters.OpReceipt, fold mapping.Fold, sameBatchRemovals map[string]string) ([]mapping.Event, error) {
	hash := op.SpecNodeID
	for _, e := range fold.Entries {
		if e.Key != hash || !e.Removed {
			continue
		}
		if e.TaskID != "" {
			return nil, nil // idempotent no-op: cleanup already landed for this removal
		}
		return []mapping.Event{{Event: "task_created", TaskID: receipt.TaskID, For: e.Source.EID}}, nil
	}
	if eid, ok := sameBatchRemovals[hash]; ok {
		return []mapping.Event{{Event: "task_created", TaskID: receipt.TaskID, For: eid}}, nil
	}
	return nil, fmt.Errorf("ingest: reconcile: invariant 1: op %s: cleanup for spec_node %s matches no removed event", op.OpID, hash)
}

// blocksDepBeadID reports the old bead id an op's lineage dep names, if
// any. ChangesetBuilder attaches this dep to every create replacing an
// obsoleted bead — cleanup and modify-pair creates alike — so its
// presence alone does not select modify-pair handling; BuildCreate only
// consults it once cleanup and proposal_epic have been ruled out.
func blocksDepBeadID(op plan.Op) (string, bool) {
	for _, d := range op.Deps {
		if d.Kind == plan.RefTask && d.EdgeType == "blocks" {
			return d.TaskID, true
		}
	}
	return "", false
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
// live entry yet (a genuinely new node).
func (b *EventBuilder) priorHash(specNodeID string) *string {
	for _, e := range b.State.Fold.Entries {
		if e.Key == specNodeID {
			return e.Source.After
		}
	}
	return nil
}

// lookupMetadata turns a missing SpecGraph or a missing node into a
// clearly wrapped error. Only fresh creates, modify-pair creates,
// modified-from-close builds, retargets and absorbed entries call this
// — cleanup and proposal_epic creates skip the spec graph entirely.
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
