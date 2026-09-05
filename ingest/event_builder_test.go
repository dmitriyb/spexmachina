package ingest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
)

// Tests for spec/ingest/arch_event_builder.md, calling EventBuilder's
// construction methods directly rather than through Reconciler.Apply —
// the per-op construction table is this component's own contract, and
// these tests pin it independently of how Reconciler eventually
// dispatches to it (spexmachina-ugrs.5). Scenarios mirror
// test_reconciliation.md, including its "Eid predicate sees the
// journal and the in-flight batch".

// newTestEventBuilder seeds a temp journal with journalLines (empty
// journal when none given), folds it exactly as production code would,
// and returns an EventBuilder over that state plus the mutable batch-eid
// set a test can grow between calls to simulate a multi-op batch — the
// same appendBatch role Reconciler plays in the real pipeline.
func newTestEventBuilder(t *testing.T, graph SpecGraph, journalLines ...mapping.Event) (*EventBuilder, map[string]bool) {
	t.Helper()
	dir := t.TempDir()
	journalPath := filepath.Join(dir, ".history.jsonl")
	store := mapping.NewMappingStore(journalPath)
	if len(journalLines) > 0 {
		if err := store.Append(journalLines); err != nil {
			t.Fatalf("seed journal: %v", err)
		}
	}
	existing, err := store.Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	fold, err := store.List()
	if err != nil {
		t.Fatalf("fold journal: %v", err)
	}

	existingEIDs := map[string]bool{}
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
	state := EventBuilderState{
		SpecGraph:        graph,
		Fold:             fold,
		RegisteredByStem: registeredByStem,
		HasEID:           func(eid string) bool { return existingEIDs[eid] || batchEIDs[eid] },
	}
	return NewEventBuilder(state), batchEIDs
}

// markBuilt grows the batch-eid set the way Reconciler's appendBatch
// does, so a subsequent build call in the same test sees these lines as
// already present — the mechanism the batch's idempotency rests on.
func markBuilt(batch map[string]bool, lines []mapping.Event) {
	for _, ev := range lines {
		if ev.EID != "" {
			batch[ev.EID] = true
		}
	}
}

// eventByKind finds the first line of the given event kind, failing the
// test if none is present.
func eventByKind(t *testing.T, lines []mapping.Event, kind string) mapping.Event {
	t.Helper()
	for _, ev := range lines {
		if ev.Event == kind {
			return ev
		}
	}
	t.Fatalf("no %q line in %+v", kind, lines)
	return mapping.Event{}
}

// --- Create paths ---

// TestEventBuilder_BuildCreate_Fresh covers "Ok create → event and
// receipt appended".
func TestEventBuilder_BuildCreate_Fresh(t *testing.T) {
	const node = "abc123def456"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "ingest", Component: "Reconciler", ContentFile: "spec/ingest/arch_reconciler.md", SpecHash: "hash-abc", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234", Proposal: "p"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: node, Idempotency: idem("spex:cafe1234:op-1")}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-new"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildCreate returned %d lines, want 2: %+v", len(lines), lines)
	}
	added := eventByKind(t, lines, "added")
	wantEID := deriveEID("cafe1234", "op-1")
	if added.EID != wantEID || added.Node != node {
		t.Errorf("added event = %+v, want eid=%s node=%s", added, wantEID, node)
	}
	if added.Name != "Reconciler" || added.NodeType != "component" || added.Module != "ingest" {
		t.Errorf("added event metadata = %+v", added)
	}
	if added.Before != nil {
		t.Errorf("added event before = %v, want nil", added.Before)
	}
	if added.After == nil || *added.After != "hash-abc" {
		t.Errorf("added event after = %v, want hash-abc", added.After)
	}
	created := eventByKind(t, lines, "task_created")
	if created.TaskID != "br-new" || created.For != wantEID {
		t.Errorf("task_created = %+v, want task_id=br-new for=%s", created, wantEID)
	}
}

// TestEventBuilder_BuildCreate_WasExistingIdempotent covers "Was_existing=true
// → idempotent no-op": the eid predicate already answers for the derived
// eid (regardless of the receipt's was_existing flag, which EventBuilder
// never reads — dedup is eid-only), so nothing is built.
func TestEventBuilder_BuildCreate_WasExistingIdempotent(t *testing.T) {
	const node = "aaaaaaaaaaaa"
	wantEID := deriveEID("cafe1234", "op-1")
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "h", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: wantEID, Node: node, Name: "A", NodeType: "component", Module: "m", After: strPtr("h"), GitHead: "cafe1234", Proposal: "p"},
		mapping.Event{Event: "task_created", TaskID: "br-7", For: wantEID},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234", Proposal: "p"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: node, Idempotency: idem("spex:" + wantEID)}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-7", WasExisting: true}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("BuildCreate returned %d lines, want 0 (idempotent no-op): %+v", len(lines), lines)
	}
}

// TestEventBuilder_BuildCreate_KnownNode_ModifiedEvent covers "Ok create
// on a known node → modified event, no task_closed": the create op
// itself carries no change type — added-vs-modified is derived from the
// journal fold's latest change event for the node.
func TestEventBuilder_BuildCreate_KnownNode_ModifiedEvent(t *testing.T) {
	const node = "bbbbbbbbbbbb"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "B", ContentFile: "B.md", SpecHash: "h-new", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed-B", Node: node, Name: "B", NodeType: "component", Module: "m", After: strPtr("h-old"), GitHead: "seedhead", Proposal: "seed-p"},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed-B"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234", Proposal: "p"}
	op := plan.Op{
		OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: node,
		Idempotency: idem("spex:cafe1234:op-1"),
	}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-new"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildCreate returned %d lines, want 2: %+v", len(lines), lines)
	}
	modified := eventByKind(t, lines, "modified")
	wantEID := deriveEID("cafe1234", "op-1")
	if modified.EID != wantEID || modified.Node != node {
		t.Errorf("modified event = %+v, want eid=%s node=%s", modified, wantEID, node)
	}
	if modified.Before == nil || *modified.Before != "h-old" {
		t.Errorf("modified event before = %v, want h-old", modified.Before)
	}
	if modified.After == nil || *modified.After != "h-new" {
		t.Errorf("modified event after = %v, want h-new", modified.After)
	}
	created := eventByKind(t, lines, "task_created")
	if created.TaskID != "br-new" || created.For != wantEID {
		t.Errorf("task_created = %+v, want task_id=br-new for=%s", created, wantEID)
	}
	if _, err := findEvent(lines, "task_closed"); err == nil {
		t.Error("BuildCreate built a task_closed — the journal never records a task's completion")
	}
}

// TestEventBuilder_BuildCreate_ReAdded covers "Ok create on a re-added
// node → added event": the journal's latest change event for the node
// is a removal (after null), so the node is born again rather than
// modified.
func TestEventBuilder_BuildCreate_ReAdded(t *testing.T) {
	const node = "feedface0005"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", ContentFile: "N.md", SpecHash: "h-2", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("h-1")},
		mapping.Event{Event: "task_created", TaskID: "br-1", For: "seed"},
		mapping.Event{Event: "removed", EID: "removed-1", Node: node, Name: "N", NodeType: "component", Module: "m", Before: strPtr("h-1")},
		mapping.Event{Event: "task_closed", TaskID: "br-1", For: "removed-1"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: node, Idempotency: idem("spex:cafe1234:op-1")}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-2"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildCreate returned %d lines, want 2: %+v", len(lines), lines)
	}
	added := eventByKind(t, lines, "added")
	if added.Before != nil {
		t.Errorf("added event before = %v, want nil", added.Before)
	}
	created := eventByKind(t, lines, "task_created")
	if created.TaskID != "br-2" || created.For != added.EID {
		t.Errorf("task_created = %+v, want task_id=br-2 for=%s", created, added.EID)
	}
}

// TestEventBuilder_BuildCreate_ProposalEpic covers "Proposal-epic create
// → receipt references the registered event, no spec-graph lookup". The
// spec graph carries no nodes at all — a lookup attempt would fail — so
// a passing test proves BuildCreate never reaches it for this kind.
func TestEventBuilder_BuildCreate_ProposalEpic(t *testing.T) {
	const stem = "2026-04-29-decouple-contract-gaps"
	const registeredEID = "beef0001:" + stem
	graph := newFakeSpecGraph() // empty on purpose
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "registered", EID: registeredEID, Proposal: stem, GitHead: "beef0001"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "beef0001", Proposal: stem}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: plan.KindProposalEpic, SpecNodeID: stem, Idempotency: idem("spex:" + registeredEID)}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-epic"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("BuildCreate returned %d lines, want 1 (task_created only): %+v", len(lines), lines)
	}
	created := lines[0]
	if created.Event != "task_created" || created.For != registeredEID || created.TaskID != "br-epic" {
		t.Errorf("task_created = %+v, want for=%s task_id=br-epic", created, registeredEID)
	}
}

// TestEventBuilder_BuildCreate_ProposalEpic_NoRegisteredEvent covers
// "Proposal-epic create without a registered event → invariant failure".
func TestEventBuilder_BuildCreate_ProposalEpic_NoRegisteredEvent(t *testing.T) {
	const stem = "2026-04-29-decouple-contract-gaps"
	graph := newFakeSpecGraph()
	b, _ := newTestEventBuilder(t, graph) // no registered event seeded

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "beef0001", Proposal: stem}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: plan.KindProposalEpic, SpecNodeID: stem}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-epic"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err == nil {
		t.Fatalf("BuildCreate: want error, got lines %+v", lines)
	}
	if !strings.Contains(err.Error(), stem) {
		t.Errorf("error %q does not name the slug %q", err.Error(), stem)
	}
	if lines != nil {
		t.Errorf("BuildCreate returned lines on error: %+v", lines)
	}
}

// TestEventBuilder_BuildCreate_Cleanup_PriorRemovedEvent covers "Cleanup
// create → receipt pairs with the prior removed event".
func TestEventBuilder_BuildCreate_Cleanup_PriorRemovedEvent(t *testing.T) {
	const node = "abc123def456"
	const removedEID = "E1"
	graph := newFakeSpecGraph() // empty: cleanup creates never reach the spec graph
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("h")},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed"},
		mapping.Event{Event: "removed", EID: removedEID, Node: node, Name: "N", NodeType: "component", Module: "m", Before: strPtr("h")},
		mapping.Event{Event: "task_closed", TaskID: "br-old", For: removedEID},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: plan.KindCleanup, SpecNodeID: node, Idempotency: idem("spex:" + removedEID)}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-cleanup"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("BuildCreate returned %d lines, want 1: %+v", len(lines), lines)
	}
	created := lines[0]
	if created.Event != "task_created" || created.For != removedEID || created.TaskID != "br-cleanup" {
		t.Errorf("task_created = %+v, want for=%s task_id=br-cleanup", created, removedEID)
	}
}

// TestEventBuilder_BuildCreate_Cleanup_MintsRemoval covers "Cleanup
// create → the cleanup mints the removal itself": the node's task
// finished with no close accompanying this cleanup in the batch, so
// EventBuilder itself must mint the removal — no close op does it.
func TestEventBuilder_BuildCreate_Cleanup_MintsRemoval(t *testing.T) {
	const node = "abc123def456"
	graph := newFakeSpecGraph() // empty: cleanup creates never reach the spec graph
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("aaa")},
		mapping.Event{Event: "task_created", TaskID: "br-gone", For: "seed"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: plan.KindCleanup, SpecNodeID: node, Idempotency: idem("spex:cafe1234:op-1")}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-cleanup"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildCreate returned %d lines, want 2: %+v", len(lines), lines)
	}
	removed := eventByKind(t, lines, "removed")
	wantEID := deriveEID("cafe1234", "op-1")
	if removed.EID != wantEID || removed.Node != node {
		t.Errorf("removed event = %+v, want eid=%s node=%s", removed, wantEID, node)
	}
	if removed.Before == nil || *removed.Before != "aaa" || removed.After != nil {
		t.Errorf("removed event before/after = %v/%v, want aaa/nil", removed.Before, removed.After)
	}
	if removed.Name != "N" || removed.NodeType != "component" || removed.Module != "m" {
		t.Errorf("removed event metadata = %+v, want the fold entry's biography", removed)
	}
	created := eventByKind(t, lines, "task_created")
	if created.TaskID != "br-cleanup" || created.For != wantEID {
		t.Errorf("task_created = %+v, want task_id=br-cleanup for=%s", created, wantEID)
	}
	if _, err := findEvent(lines, "task_closed"); err == nil {
		t.Error("BuildCreate built a task_closed for the finished task — the journal never records a task's completion")
	}
}

// TestEventBuilder_BuildCreate_Cleanup_AfterReAdd covers "Cleanup create
// after a re-add → a fresh removal, not the old one": the node's
// earliest removal (E1) is not its latest state, so the cleanup must not
// reference it.
func TestEventBuilder_BuildCreate_Cleanup_AfterReAdd(t *testing.T) {
	const node = "abc123def456"
	graph := newFakeSpecGraph()
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "removed", EID: "E1", Node: node, Name: "N", NodeType: "component", Module: "m", Before: strPtr("old")},
		mapping.Event{Event: "added", EID: "E2", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("new")},
		mapping.Event{Event: "task_created", TaskID: "br-new", For: "E2"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: plan.KindCleanup, SpecNodeID: node, Idempotency: idem("spex:cafe1234:op-1")}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-cleanup"}

	lines, err := b.BuildCreate(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildCreate returned %d lines, want 2: %+v", len(lines), lines)
	}
	removed := eventByKind(t, lines, "removed")
	wantEID := deriveEID("cafe1234", "op-1")
	if removed.EID != wantEID {
		t.Errorf("removed event eid = %q, want %q (E1 is not the node's latest state)", removed.EID, wantEID)
	}
	if removed.Before == nil || *removed.Before != "new" {
		t.Errorf("removed event before = %v, want new", removed.Before)
	}
	created := eventByKind(t, lines, "task_created")
	if created.For != wantEID {
		t.Errorf("task_created.for = %q, want %q, not the old removal E1", created.For, wantEID)
	}
}

// --- Close paths ---

// TestEventBuilder_BuildClose_Removed covers "Ok close on removed →
// removed event and task_closed appended".
func TestEventBuilder_BuildClose_Removed(t *testing.T) {
	const node = "beadbead0001"
	graph := newFakeSpecGraph()
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("h")},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{OpID: "op-1", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-old"}, Reason: "Spec node removed: m/N"}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-old"}

	lines, err := b.BuildClose(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildClose: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildClose returned %d lines, want 2: %+v", len(lines), lines)
	}
	removed := eventByKind(t, lines, "removed")
	if removed.Node != node || removed.Name != "N" || removed.NodeType != "component" || removed.Module != "m" {
		t.Errorf("removed event = %+v, want the node's biography", removed)
	}
	closed := eventByKind(t, lines, "task_closed")
	if closed.TaskID != "br-old" || closed.For != removed.EID {
		t.Errorf("task_closed = %+v, want task_id=br-old for=%s", closed, removed.EID)
	}
}

// TestEventBuilder_BuildClose_ModifiedAlwaysBuildsOwnPair proves the
// retired "Modified-Node Pair" claim-tracking is gone: a "Spec node
// modified" close builds its own modified event plus task_closed
// unconditionally, even when the same changeset also carries a create
// for a different node (there is no lineage dep left for it to claim the
// close with).
func TestEventBuilder_BuildClose_ModifiedAlwaysBuildsOwnPair(t *testing.T) {
	const closingNode = "bbbbbbbbbbbb"
	const otherNode = "cccccccccccc"
	createOp := plan.Op{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: otherNode}
	closeOp := plan.Op{OpID: "op-2", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-old"}, Reason: "Spec node modified (new): m/N"}
	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234", Ops: []plan.Op{createOp, closeOp}}

	graph := newFakeSpecGraph()
	graph.nodes[closingNode] = NodeMetadata{Module: "m", Component: "B", ContentFile: "B.md", SpecHash: "h-new", NodeType: "component"}
	graph.nodes[otherNode] = NodeMetadata{Module: "m", Component: "C", ContentFile: "C.md", SpecHash: "h-other", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: closingNode, Name: "N", NodeType: "component", Module: "m", After: strPtr("h")},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed"},
	)

	// The unrelated create builds independently, first.
	if _, err := b.BuildCreate(cs, createOp, adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-other"}); err != nil {
		t.Fatalf("BuildCreate: %v", err)
	}

	lines, err := b.BuildClose(cs, closeOp, adapters.OpReceipt{OpID: "op-2", Status: adapters.OpStatusOk, TaskID: "br-old"})
	if err != nil {
		t.Fatalf("BuildClose: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildClose returned %d lines, want 2 (modified + task_closed, built on its own): %+v", len(lines), lines)
	}
	modified := eventByKind(t, lines, "modified")
	if modified.Node != closingNode {
		t.Errorf("modified event node = %q, want %q", modified.Node, closingNode)
	}
	closed := eventByKind(t, lines, "task_closed")
	if closed.TaskID != "br-old" || closed.For != modified.EID {
		t.Errorf("task_closed = %+v, want task_id=br-old for=%s", closed, modified.EID)
	}
}

// TestEventBuilder_BuildClose_ModifiedNoClaim_UnknownBead covers
// "Fold-back close naming a task unknown to the journal → refused before
// append".
func TestEventBuilder_BuildClose_ModifiedNoClaim_UnknownBead(t *testing.T) {
	graph := newFakeSpecGraph()
	b, _ := newTestEventBuilder(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g"}
	op := plan.Op{OpID: "op-1", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-unknown"}, Reason: "Spec node modified (new): m/N"}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-unknown"}

	lines, err := b.BuildClose(cs, op, receipt)
	if err == nil {
		t.Fatalf("BuildClose: want error, got lines %+v", lines)
	}
	if !strings.Contains(err.Error(), "op-1") || !strings.Contains(err.Error(), "br-unknown") {
		t.Errorf("error %q does not name the op and the unclaimed bead", err.Error())
	}
}

// TestEventBuilder_BuildClose_ModifiedNoClaim_LiveBead covers "Fold-back
// close, task live in the journal → modified event from the close alone"
// — the shape ActionClassifier emits for a coupled test_section edit.
func TestEventBuilder_BuildClose_ModifiedNoClaim_LiveBead(t *testing.T) {
	const node = "cccccccccccc"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", ContentFile: "N.md", SpecHash: "h-new", NodeType: "test_section"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "test_section", Module: "m", After: strPtr("h-old")},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-old"}, Reason: "Spec node modified (new): m/N"},
	}}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-old"}

	lines, err := b.BuildClose(cs, cs.Ops[0], receipt)
	if err != nil {
		t.Fatalf("BuildClose: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildClose returned %d lines, want 2 (modified + task_closed, no task_created): %+v", len(lines), lines)
	}
	if _, err := findEvent(lines, "task_created"); err == nil {
		t.Errorf("BuildClose built a task_created with no successor task: %+v", lines)
	}
	modified := eventByKind(t, lines, "modified")
	if modified.Node != node || modified.Before == nil || *modified.Before != "h-old" || modified.After == nil || *modified.After != "h-new" {
		t.Errorf("modified event = %+v, want node=%s before=h-old after=h-new", modified, node)
	}
	closed := eventByKind(t, lines, "task_closed")
	if closed.TaskID != "br-old" || closed.For != modified.EID {
		t.Errorf("task_closed = %+v, want task_id=br-old for=%s", closed, modified.EID)
	}
}

func findEvent(lines []mapping.Event, kind string) (mapping.Event, error) {
	for _, ev := range lines {
		if ev.Event == kind {
			return ev, nil
		}
	}
	return mapping.Event{}, errNotFound
}

var errNotFound = &notFoundError{}

type notFoundError struct{}

func (*notFoundError) Error() string { return "not found" }

// --- Retarget ---

// TestEventBuilder_BuildRetarget_Ok covers "Ok retarget → modified event
// and task_retargeted appended".
func TestEventBuilder_BuildRetarget_Ok(t *testing.T) {
	const node = "beadbead0003"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", ContentFile: "N.md", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("h-old")},
		mapping.Event{Event: "task_created", TaskID: "br-open", For: "seed"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{
		OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: node, SpecHash: "h-new",
		Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-open"},
		Labels: []string{"spex:cafe1234:op-1"},
	}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-open"}

	lines, err := b.BuildRetarget(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildRetarget: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildRetarget returned %d lines, want 2: %+v", len(lines), lines)
	}
	wantEID := deriveEID("cafe1234", "op-1")
	modified := eventByKind(t, lines, "modified")
	if modified.EID != wantEID || modified.Node != node {
		t.Errorf("modified event = %+v, want eid=%s node=%s", modified, wantEID, node)
	}
	if modified.After == nil || *modified.After != "h-new" {
		t.Errorf("modified event after = %v, want h-new", modified.After)
	}
	retargeted := eventByKind(t, lines, "task_retargeted")
	if retargeted.TaskID != "br-open" || retargeted.For != wantEID {
		t.Errorf("task_retargeted = %+v, want task_id=br-open for=%s", retargeted, wantEID)
	}
	if _, err := findEvent(lines, "task_closed"); err == nil {
		t.Error("BuildRetarget built a task_closed — no bead dies on a retarget")
	}
	if _, err := findEvent(lines, "task_created"); err == nil {
		t.Error("BuildRetarget built a task_created — no bead is born on a retarget")
	}
}

// TestEventBuilder_BuildRetarget_Idempotent covers "Retarget re-run →
// idempotent no-op".
func TestEventBuilder_BuildRetarget_Idempotent(t *testing.T) {
	const node = "beadbead0003"
	wantEID := deriveEID("cafe1234", "op-1")
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: "seed", Node: node, Name: "N", NodeType: "component", Module: "m", After: strPtr("h-old")},
		mapping.Event{Event: "task_created", TaskID: "br-open", For: "seed"},
		mapping.Event{Event: "modified", EID: wantEID, Node: node, Name: "N", NodeType: "component", Module: "m", Before: strPtr("h-old"), After: strPtr("h-new")},
		mapping.Event{Event: "task_retargeted", TaskID: "br-open", For: wantEID},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}
	op := plan.Op{OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: node, SpecHash: "h-new", Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-open"}}
	receipt := adapters.OpReceipt{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-open"}

	lines, err := b.BuildRetarget(cs, op, receipt)
	if err != nil {
		t.Fatalf("BuildRetarget: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("BuildRetarget returned %d lines, want 0 (idempotent no-op): %+v", len(lines), lines)
	}
}

// --- Absorbed ---

// TestEventBuilder_BuildAbsorbed_Entry covers "Absorbed entry → modified
// event and refresh receipt appended".
func TestEventBuilder_BuildAbsorbed_Entry(t *testing.T) {
	const node = "beadbead0004"
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", ContentFile: "N.md", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Absorbed: []plan.AbsorbedEntry{
		{Node: node, Before: "aaa", After: "bbb", Reason: "typo sweep"},
	}}

	lines, err := b.BuildAbsorbed(cs)
	if err != nil {
		t.Fatalf("BuildAbsorbed: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("BuildAbsorbed returned %d lines, want 2: %+v", len(lines), lines)
	}
	wantEID := deriveRefreshEID(node, strPtr("aaa"), strPtr("bbb"))
	modified := eventByKind(t, lines, "modified")
	if modified.EID != wantEID || modified.Node != node {
		t.Errorf("modified event = %+v, want eid=%s node=%s", modified, wantEID, node)
	}
	if modified.Before == nil || *modified.Before != "aaa" || modified.After == nil || *modified.After != "bbb" {
		t.Errorf("modified event before/after = %v/%v, want aaa/bbb", modified.Before, modified.After)
	}
	refresh := eventByKind(t, lines, "refresh")
	if len(refresh.Absorbed) != 1 || refresh.Absorbed[0] != wantEID {
		t.Errorf("refresh.Absorbed = %+v, want [%s]", refresh.Absorbed, wantEID)
	}
	if refresh.GitHead != "g" {
		t.Errorf("refresh.GitHead = %q, want g", refresh.GitHead)
	}
	if _, err := findEvent(lines, "task_created"); err == nil {
		t.Error("BuildAbsorbed built a task receipt — absorption owes no tracker work")
	}
}

// TestEventBuilder_BuildAbsorbed_Idempotent covers "Absorbed re-run →
// idempotent no-op".
func TestEventBuilder_BuildAbsorbed_Idempotent(t *testing.T) {
	const node = "beadbead0004"
	wantEID := deriveRefreshEID(node, strPtr("aaa"), strPtr("bbb"))
	graph := newFakeSpecGraph()
	graph.nodes[node] = NodeMetadata{Module: "m", Component: "N", NodeType: "component"}
	b, _ := newTestEventBuilder(t, graph,
		mapping.Event{Event: "modified", EID: wantEID, Node: node, Name: "N", NodeType: "component", Module: "m", Before: strPtr("aaa"), After: strPtr("bbb")},
		mapping.Event{Event: "refresh", GitHead: "g", Absorbed: []string{wantEID}},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Absorbed: []plan.AbsorbedEntry{
		{Node: node, Before: "aaa", After: "bbb", Reason: "typo sweep"},
	}}

	lines, err := b.BuildAbsorbed(cs)
	if err != nil {
		t.Fatalf("BuildAbsorbed: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("BuildAbsorbed returned %d lines, want 0 (idempotent no-op): %+v", len(lines), lines)
	}
}

// TestEventBuilder_BuildAbsorbed_Empty covers "An empty absorbed array
// constructs nothing, not an empty receipt."
func TestEventBuilder_BuildAbsorbed_Empty(t *testing.T) {
	graph := newFakeSpecGraph()
	b, _ := newTestEventBuilder(t, graph)

	lines, err := b.BuildAbsorbed(plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g"})
	if err != nil {
		t.Fatalf("BuildAbsorbed: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("BuildAbsorbed returned %d lines for an empty absorbed array, want 0: %+v", len(lines), lines)
	}
}

// --- Eid predicate ---

// TestEventBuilder_EidPredicate_SeesJournalAndBatch pins
// test_reconciliation.md's "Eid predicate sees the journal and the
// in-flight batch": the predicate is EventBuilder's own per-run state,
// and it must answer for both a journal-side duplicate and a duplicate
// constructed earlier in this same batch.
func TestEventBuilder_EidPredicate_SeesJournalAndBatch(t *testing.T) {
	const nodeA = "111111111111"
	const nodeB = "222222222222"
	wantEIDA := deriveEID("cafe1234", "op-a")
	graph := newFakeSpecGraph()
	graph.nodes[nodeA] = NodeMetadata{Module: "m", Component: "A", NodeType: "component", SpecHash: "hA"}
	graph.nodes[nodeB] = NodeMetadata{Module: "m", Component: "B", NodeType: "component", SpecHash: "hB"}

	// A is a journal-side duplicate: the same create op re-emitted.
	b, batch := newTestEventBuilder(t, graph,
		mapping.Event{Event: "added", EID: wantEIDA, Node: nodeA, Name: "A", NodeType: "component", Module: "m", After: strPtr("hA")},
		mapping.Event{Event: "task_created", TaskID: "br-A", For: wantEIDA},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafe1234"}

	opA := plan.Op{OpID: "op-a", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: nodeA}
	linesA, err := b.BuildCreate(cs, opA, adapters.OpReceipt{OpID: "op-a", Status: adapters.OpStatusOk, TaskID: "br-A"})
	if err != nil {
		t.Fatalf("BuildCreate A: %v", err)
	}
	if len(linesA) != 0 {
		t.Fatalf("BuildCreate A returned %d lines, want 0 (journal-side duplicate): %+v", len(linesA), linesA)
	}

	// B is fresh: builds and lands in the batch.
	opB := plan.Op{OpID: "op-b", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: nodeB}
	linesB, err := b.BuildCreate(cs, opB, adapters.OpReceipt{OpID: "op-b", Status: adapters.OpStatusOk, TaskID: "br-B"})
	if err != nil {
		t.Fatalf("BuildCreate B: %v", err)
	}
	if len(linesB) != 2 {
		t.Fatalf("BuildCreate B returned %d lines, want 2: %+v", len(linesB), linesB)
	}
	markBuilt(batch, linesB)

	// A second op for B's own node, deterministically colliding on B's
	// derived eid (same node, but framed as a same-batch collision on the
	// eid the predicate must now already answer for from the batch, not
	// the journal).
	wantEIDB := deriveEID("cafe1234", "op-b")
	if !batch[wantEIDB] {
		t.Fatalf("batch eid set does not carry B's eid %s after markBuilt", wantEIDB)
	}
	opBAgain := plan.Op{OpID: "op-b", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: nodeB}
	linesBAgain, err := b.BuildCreate(cs, opBAgain, adapters.OpReceipt{OpID: "op-b", Status: adapters.OpStatusOk, TaskID: "br-B"})
	if err != nil {
		t.Fatalf("BuildCreate B (re-derived): %v", err)
	}
	if len(linesBAgain) != 0 {
		t.Fatalf("BuildCreate B re-derived returned %d lines, want 0 (in-flight batch duplicate): %+v", len(linesBAgain), linesBAgain)
	}
}
