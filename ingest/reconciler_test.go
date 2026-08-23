package ingest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/plan"
)

// Fixed 12-hex-char identity hashes for test node ids — the journal-line
// schema requires this exact shape on every change event's "node" field,
// so short mnemonic letters ("A", "X", ...) are not valid fixture data.
const (
	hexA    = "a1a1a1a1a1a1"
	hexB    = "b2b2b2b2b2b2"
	hexC    = "c3c3c3c3c3c3"
	hexX    = "111111111111"
	hexY    = "222222222222"
	hexZ    = "333333333333"
	hexM    = "444444444444"
	hexOld  = "555555555555"
	hexMod1 = "666666666666"
	hexMod2 = "777777777777"
	hexGone = "888888888888"
	hexR    = "999999999999"
	hexAbs1 = "aaaaaaaaaaaa"
	hexAbs2 = "bbbbbbbbbbbb"
)

// fakeSpecGraph satisfies SpecGraph from a flat metadata table. Every
// reconciler test seeds the nodes it cares about; nodes left out are
// reported as missing, which is exactly what a fresh/modified create with
// an unregistered spec_node_id should surface.
type fakeSpecGraph struct {
	nodes map[string]NodeMetadata
}

func newFakeSpecGraph() *fakeSpecGraph {
	return &fakeSpecGraph{nodes: map[string]NodeMetadata{}}
}

func (s *fakeSpecGraph) NodeMetadata(id string) (NodeMetadata, error) {
	md, ok := s.nodes[id]
	if !ok {
		return NodeMetadata{}, &nodeNotFoundError{id: id}
	}
	return md, nil
}

type nodeNotFoundError struct{ id string }

func (e *nodeNotFoundError) Error() string { return "fake spec graph: no node " + e.id }

// idem builds the *Idem struct plan attaches to create ops.
func idem(label string) *plan.Idem { return &plan.Idem{Label: label} }

// newTestReconciler creates a Reconciler over a fresh temp spec dir with
// no journal on disk yet — the "bootstrap" state every scenario starts
// from unless it calls seedJournal first.
func newTestReconciler(t *testing.T, graph SpecGraph) (*Reconciler, string) {
	t.Helper()
	dir := t.TempDir()
	return &Reconciler{SpecDir: dir, SpecGraph: graph}, dir
}

// seedJournal writes an initial spec/.history.jsonl via the production
// append path, so every fixture's on-disk shape is exactly what a real
// Reconciler.Apply would have produced.
func seedJournal(t *testing.T, specDir string, lines ...mapping.Event) {
	t.Helper()
	if err := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl")).Append(lines); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
}

// readJournal parses the current on-disk journal.
func readJournal(t *testing.T, specDir string) []mapping.Event {
	t.Helper()
	events, err := mapping.NewMappingStore(filepath.Join(specDir, ".history.jsonl")).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	return events
}

// journalBytes reads the raw journal file, treating "does not exist" as
// empty — the pre-first-write state every atomicity assertion starts
// from.
func journalBytes(t *testing.T, specDir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(specDir, ".history.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read journal: %v", err)
	}
	return data
}

// TestApply_OkCreate_EventAndReceiptAppended covers the "Ok create →
// event and receipt appended" scenario from test_reconciliation.md.
func TestApply_OkCreate_EventAndReceiptAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["abc123def456"] = NodeMetadata{
		Module:      "ingest",
		Component:   "Reconciler",
		ContentFile: "spec/ingest/arch_reconciler.md",
		SpecHash:    "hash-abc",
		NodeType:    "component",
	}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version:  plan.ChangesetVersion,
		GitHead:  "cafe1234",
		Proposal: "p",
		Ops: []plan.Op{{
			OpID:         "op-1",
			Type:         plan.OpCreate,
			SpecNodeKind: "component",
			SpecNodeID:   "abc123def456",
			Idempotency:  idem("spex:cafe1234:op-1"),
			Title:        "ingest: Reconciler",
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-new", WasExisting: false,
		}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 1 || sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 ok create / 1 event / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 2 {
		t.Fatalf("journal has %d lines, want 2: %+v", len(journal), journal)
	}
	added := journal[0]
	if added.Event != "added" || added.Node != "abc123def456" {
		t.Fatalf("line 1 = %+v, want added event for abc123def456", added)
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
	if added.GitHead != "cafe1234" || added.Proposal != "p" {
		t.Errorf("added event git_head/proposal = %q/%q", added.GitHead, added.Proposal)
	}
	if added.Path != "spec/ingest/arch_reconciler.md" {
		t.Errorf("added event path = %q", added.Path)
	}
	if added.EID == "" {
		t.Fatal("added event has empty eid")
	}

	created := journal[1]
	if created.Event != "task_created" || created.TaskID != "br-new" || created.For != added.EID {
		t.Errorf("task_created = %+v, want for=%s task_id=br-new", created, added.EID)
	}
}

// TestApply_OkClose_RemovedAppendsRemovedEventAndTaskClosed covers "Ok
// close on removed → removed event and task_closed appended".
func TestApply_OkClose_RemovedAppendsRemovedEventAndTaskClosed(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{
			Event: "added", EID: "E1", Node: "beadbead0001", Name: "Widget",
			NodeType: "component", Module: "m", After: strPtr("h1"),
			GitHead: "seedhead", Proposal: "seed-proposal", Path: "m/arch_widget.md",
		},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "E1"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafe5678", Proposal: "p2",
		Ops: []plan.Op{{
			OpID:   "op-1",
			Type:   plan.OpClose,
			Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-old"},
			Reason: "Spec node removed: m/Widget",
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCloses != 1 || sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 ok close / 1 event / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 4 {
		t.Fatalf("journal has %d lines, want 4 (2 seed + 2 new): %+v", len(journal), journal)
	}
	removed := journal[2]
	if removed.Event != "removed" || removed.Node != "beadbead0001" {
		t.Fatalf("line 3 = %+v, want removed event for beadbead0001", removed)
	}
	if removed.Name != "Widget" || removed.NodeType != "component" || removed.Module != "m" || removed.Path != "m/arch_widget.md" {
		t.Errorf("removed event did not inherit biography: %+v", removed)
	}
	if removed.Before == nil || *removed.Before != "h1" {
		t.Errorf("removed event before = %v, want h1", removed.Before)
	}
	if removed.After != nil {
		t.Errorf("removed event after = %v, want nil", removed.After)
	}
	closed := journal[3]
	if closed.Event != "task_closed" || closed.TaskID != "br-old" || closed.For != removed.EID {
		t.Errorf("task_closed = %+v, want for=%s task_id=br-old", closed, removed.EID)
	}

	// The earlier pairing lines remain untouched: nothing is ever deleted.
	if journal[0].Event != "added" || journal[1].Event != "task_created" {
		t.Errorf("seed lines mutated: %+v / %+v", journal[0], journal[1])
	}
}

// TestApply_ModifiedPair_LineageExtendedNotRebound covers "Modified node:
// close+create → lineage extended, not rebound".
func TestApply_ModifiedPair_LineageExtendedNotRebound(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["beadbead0002"] = NodeMetadata{
		Module: "m", Component: "Widget", ContentFile: "m/arch_widget.md",
		SpecHash: "new-hash", NodeType: "component",
	}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{
			Event: "added", EID: "E1", Node: "beadbead0002", Name: "Widget",
			NodeType: "component", Module: "m", After: strPtr("old-hash"),
			GitHead: "seedhead", Proposal: "seed-proposal", Path: "m/arch_widget.md",
		},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "E1"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafe0002", Proposal: "p3",
		Ops: []plan.Op{
			{
				OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "beadbead0002",
				Idempotency: idem("spex:cafe0002:op-1"),
				Deps:        []plan.Ref{{Kind: plan.RefBead, BeadID: "br-old", EdgeType: "blocks"}},
			},
			{OpID: "op-2", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-old"}, Reason: "Spec node modified: m/Widget"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-new", WasExisting: false},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-old"},
		},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 1 || sum.OkCloses != 1 || sum.EventsAppended != 1 || sum.ReceiptsAppended != 2 {
		t.Errorf("summary = %+v, want 1 create/1 close, 1 event, 2 receipts", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 5 {
		t.Fatalf("journal has %d lines, want 5 (2 seed + 3 new): %+v", len(journal), journal)
	}
	modified := journal[2]
	if modified.Event != "modified" || modified.Node != "beadbead0002" {
		t.Fatalf("line 3 = %+v, want modified event", modified)
	}
	if modified.Before == nil || *modified.Before != "old-hash" {
		t.Errorf("modified before = %v, want old-hash", modified.Before)
	}
	if modified.After == nil || *modified.After != "new-hash" {
		t.Errorf("modified after = %v, want new-hash", modified.After)
	}
	closed := journal[3]
	created := journal[4]
	if closed.Event != "task_closed" || closed.TaskID != "br-old" || closed.For != modified.EID {
		t.Errorf("task_closed = %+v, want for=%s task_id=br-old", closed, modified.EID)
	}
	if created.Event != "task_created" || created.TaskID != "br-new" || created.For != modified.EID {
		t.Errorf("task_created = %+v, want for=%s task_id=br-new", created, modified.EID)
	}

	// The fold now answers beadbead0002 → br-new; the br-old pairing
	// remains as lineage (nothing deleted).
	fold, err := mapping.NewMappingStore(filepath.Join(dir, ".history.jsonl")).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, e := range fold.Entries {
		if e.Key == "beadbead0002" {
			found = true
			if e.TaskID != "br-new" {
				t.Errorf("fold entry task_id = %q, want br-new", e.TaskID)
			}
		}
	}
	if !found {
		t.Fatal("no fold entry for beadbead0002")
	}
	if journal[1].Event != "task_created" || journal[1].TaskID != "br-old" {
		t.Errorf("br-old lineage line missing or altered: %+v", journal[1])
	}
}

// TestApply_WasExisting_IdempotentNoOp covers "Was_existing=true →
// idempotent no-op": the reconciler's own idempotency is eid-derived, not
// keyed off the receipt's was_existing flag — replaying the identical
// (git_head, op_id) finds the eid already present and appends nothing.
func TestApply_WasExisting_IdempotentNoOp(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "h", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	gitHead, opID := "cafe0000", "op-1"
	eid := deriveEID(gitHead, opID)
	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: eid, Node: hexA, Name: "A", NodeType: "component", Module: "m", After: strPtr("h"), GitHead: gitHead, Proposal: "p"},
		mapping.Event{Event: "task_created", TaskID: "br-7", For: eid},
	)
	before := journalBytes(t, dir)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: gitHead, Proposal: "p",
		Ops: []plan.Op{{OpID: opID, Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: opID, Status: adapters.OpStatusOk, BeadID: "br-7", WasExisting: true}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 1 || sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("summary = %+v, want 1 ok create / zero appends", sum)
	}
	after := journalBytes(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatalf("journal mutated on idempotent replay:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestApply_ErrorStatus_NothingAppended covers "Error status → op
// skipped, nothing appended".
func TestApply_ErrorStatus_NothingAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:A")}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusError, Error: "bead_cli exited 1"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Errors != 1 || sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("summary = %+v, want 1 error / zero appends", sum)
	}
	if len(readJournal(t, dir)) != 0 {
		t.Error("error receipt appended lines")
	}
}

// TestApply_SkippedStatus_NothingAppended covers "Skipped status → op
// no-op".
func TestApply_SkippedStatus_NothingAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:A")}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusSkipped, Reason: "already labeled"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Skipped != 1 || sum.EventsAppended != 0 {
		t.Errorf("summary = %+v, want skipped=1 / zero appends", sum)
	}
	if len(readJournal(t, dir)) != 0 {
		t.Error("skipped receipt appended lines")
	}
}

// TestApply_MixedOps_OrderedAppend covers the "Mixed ops: one batch,
// ordered append" scenario: a modified lineage pair, a removed close, and
// a fresh create land in op order.
func TestApply_MixedOps_OrderedAppend(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexX] = NodeMetadata{Module: "m", Component: "X", ContentFile: "x.md", SpecHash: "new-x", NodeType: "component"}
	graph.nodes[hexY] = NodeMetadata{Module: "m", Component: "Y", ContentFile: "y.md", SpecHash: "hy", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: "seedX", Node: hexX, Name: "X", NodeType: "component", Module: "m", After: strPtr("old-x"), GitHead: "seedhead", Proposal: "seed-p", Path: "x.md"},
		mapping.Event{Event: "task_created", TaskID: "br-A", For: "seedX"},
		mapping.Event{Event: "added", EID: "seedZ", Node: hexZ, Name: "Z", NodeType: "component", Module: "m", After: strPtr("hz"), GitHead: "seedhead", Proposal: "seed-p", Path: "z.md"},
		mapping.Event{Event: "task_created", TaskID: "br-B", For: "seedZ"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafe9999", Proposal: "p4",
		Ops: []plan.Op{
			{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexX, Idempotency: idem("spex:" + hexX), Deps: []plan.Ref{{Kind: plan.RefBead, BeadID: "br-A", EdgeType: "blocks"}}},
			{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexY, Idempotency: idem("spex:" + hexY)},
			{OpID: "op-3", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-A"}, Reason: "Spec node modified: m/X"},
			{OpID: "op-4", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-B"}, Reason: "Spec node removed: m/Z"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A2", WasExisting: false},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-Y", WasExisting: false},
			{OpID: "op-3", Status: adapters.OpStatusOk, BeadID: "br-A"},
			{OpID: "op-4", Status: adapters.OpStatusOk, BeadID: "br-B"},
		},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 2 || sum.OkCloses != 2 || sum.EventsAppended != 3 || sum.ReceiptsAppended != 4 {
		t.Errorf("summary = %+v, want 2 creates/2 closes, 3 events, 4 receipts", sum)
	}

	journal := readJournal(t, dir)
	newLines := journal[4:]
	if len(newLines) != 7 {
		t.Fatalf("got %d new lines, want 7: %+v", len(newLines), newLines)
	}
	wantKinds := []string{"modified", "task_closed", "task_created", "added", "task_created", "removed", "task_closed"}
	for i, want := range wantKinds {
		if newLines[i].Event != want {
			t.Errorf("new line %d = %q, want %q", i, newLines[i].Event, want)
		}
	}

	fold, err := mapping.NewMappingStore(filepath.Join(dir, ".history.jsonl")).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byKey := map[string]mapping.FoldEntry{}
	for _, e := range fold.Entries {
		byKey[e.Key] = e
	}
	if byKey[hexX].TaskID != "br-A2" {
		t.Errorf("X fold = %+v, want task br-A2", byKey[hexX])
	}
	if !byKey[hexZ].Removed {
		t.Errorf("Z fold = %+v, want removed", byKey[hexZ])
	}
	if byKey[hexY].TaskID != "br-Y" {
		t.Errorf("Y fold = %+v, want task br-Y", byKey[hexY])
	}
}

// TestApply_ProposalEpicCreate_ReferencesRegisteredEvent covers "Proposal-
// epic create → receipt references the registered event, no spec-graph
// lookup". The spec graph is empty on purpose: if the reconciler ever
// looks the stem up, the fake graph's ErrNotFound-style error surfaces and
// this test fails.
func TestApply_ProposalEpicCreate_ReferencesRegisteredEvent(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	stem := "2026-04-29-decouple-contract-gaps"
	registeredEID := "beef0001:" + stem
	seedJournal(t, dir, mapping.Event{Event: "registered", EID: registeredEID, Proposal: stem, GitHead: "beef0001"})

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: stem,
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "proposal_epic",
			SpecNodeID: stem, Idempotency: idem("spex:" + registeredEID), Title: "Proposal: " + stem,
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-epic", WasExisting: false}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 0 events / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 2 {
		t.Fatalf("journal has %d lines, want 2 (seed + epic receipt): %+v", len(journal), journal)
	}
	line := journal[1]
	if line.Event != "task_created" || line.For != registeredEID || line.TaskID != "br-epic" || line.Proposal != "" {
		t.Errorf("epic receipt = %+v, want task_created/for=%s/task_id=br-epic/no proposal", line, registeredEID)
	}
}

// TestApply_ProposalEpicCreate_NoRegisteredEvent_InvariantFailure covers
// "Proposal-epic create without a registered event → invariant failure":
// plan refuses to build such an op, so its arrival marks a malformed
// changeset and nothing is appended.
func TestApply_ProposalEpicCreate_NoRegisteredEvent_InvariantFailure(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	stem := "2026-04-29-decouple-contract-gaps"
	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: stem,
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "proposal_epic",
			SpecNodeID: stem, Idempotency: idem("spex:beef0001:" + stem), Title: "Proposal: " + stem,
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-epic", WasExisting: false}},
	}

	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), stem) {
		t.Fatalf("Apply: got %v, want error naming the slug %s", err, stem)
	}
	if len(journalBytes(t, dir)) != 0 {
		t.Error("journal file written despite refused batch")
	}
}

// TestApply_CleanupCreate_PairsWithPriorRemovedEvent covers "Cleanup
// create → receipt pairs with the prior removed event".
func TestApply_CleanupCreate_PairsWithPriorRemovedEvent(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir, mapping.Event{
		Event: "removed", EID: "E1", Node: "abc123def456", Name: "Old", NodeType: "component",
		Module: "m", Before: strPtr("h"), GitHead: "seedhead", Proposal: "seed-p", Path: "m/old.md",
	})

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "cleanup", SpecNodeID: "abc123def456",
			Idempotency: idem("spex:E1"), Labels: []string{"spex:cleanup"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-cleanup", WasExisting: false}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 0 events / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 2 {
		t.Fatalf("journal has %d lines, want 2: %+v", len(journal), journal)
	}
	line := journal[1]
	if line.Event != "task_created" || line.For != "E1" || line.TaskID != "br-cleanup" {
		t.Errorf("cleanup receipt = %+v, want for=E1/task_id=br-cleanup", line)
	}
}

// TestApply_CleanupCreate_PairsWithSameBatchRemoval covers the case real
// plan output actually produces: the cleanup create and the close that
// removes its node land in the SAME batch, with the create ordered before
// the close (plan/builder.go orders every changeset create-before-close).
// At the time the cleanup create is processed, neither the journal's fold
// nor the batch-so-far shows the node as removed yet — the referent has
// to come from the precomputed same-batch removal map, not a scan of
// already-appended lines.
func TestApply_CleanupCreate_PairsWithSameBatchRemoval(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{
			Event: "added", EID: "E1", Node: hexGone, Name: "Gone", NodeType: "component",
			Module: "m", After: strPtr("h-gone"), GitHead: "seedhead", Proposal: "seed-p", Path: "m/gone.md",
		},
		mapping.Event{Event: "task_created", TaskID: "br-gone", For: "E1"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafebeef", Proposal: "p",
		Ops: []plan.Op{
			{
				OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "cleanup", SpecNodeID: hexGone,
				Idempotency: idem("spex:cafebeef:op-2"), Labels: []string{"spex:cleanup"},
			},
			{OpID: "op-2", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-gone"}, Reason: "Spec node removed: m/Gone"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-cleanup", WasExisting: false},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-gone"},
		},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 2 {
		t.Errorf("summary = %+v, want 1 event / 2 receipts", sum)
	}

	journal := readJournal(t, dir)
	newLines := journal[2:]
	if len(newLines) != 3 {
		t.Fatalf("got %d new lines, want 3: %+v", len(newLines), newLines)
	}
	cleanupReceipt, removed, closedReceipt := newLines[0], newLines[1], newLines[2]
	if removed.Event != "removed" || removed.Node != hexGone {
		t.Fatalf("line 2 = %+v, want removed event for %s", removed, hexGone)
	}
	if cleanupReceipt.Event != "task_created" || cleanupReceipt.TaskID != "br-cleanup" || cleanupReceipt.For != removed.EID {
		t.Errorf("cleanup receipt = %+v, want for=%s task_id=br-cleanup", cleanupReceipt, removed.EID)
	}
	if closedReceipt.Event != "task_closed" || closedReceipt.TaskID != "br-gone" || closedReceipt.For != removed.EID {
		t.Errorf("task_closed = %+v, want for=%s task_id=br-gone", closedReceipt, removed.EID)
	}
}

// TestApply_ModifiedClose_UnknownBead_RefusedBeforeAppend covers the
// remaining malformed-changeset shape: a "Spec node modified" close naming
// a bead the journal has never heard of (no fold entry at all) has no
// identity to build a modified event from, so it is refused rather than
// silently dropped.
func TestApply_ModifiedClose_UnknownBead_RefusedBeforeAppend(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{
			{OpID: "op-1", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-orphan"}, Reason: "Spec node modified: m/Orphan"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-orphan"}},
	}

	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), "br-orphan") {
		t.Fatalf("Apply: got %v, want error naming br-orphan", err)
	}
	if len(journalBytes(t, dir)) != 0 {
		t.Error("journal file written despite refused batch")
	}
}

// TestApply_ModifiedClose_NoPairedCreate_BuildsModifiedFromCloseAlone
// covers the shape ActionClassifier emits for a coupled test_section edit:
// an "obsolete" action with no replacement create (plan/action_classifier.go
// "coupled" branch). No create in the batch claims the bead via a `blocks`
// dep, but the bead is live in the journal, so the close alone must build
// the modified event and its task_closed — no task_created, since there is
// no successor task.
func TestApply_ModifiedClose_NoPairedCreate_BuildsModifiedFromCloseAlone(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexMod1] = NodeMetadata{
		Module: "m", Component: "Section", ContentFile: "m/test_section.md",
		SpecHash: "new-hash", NodeType: "test_section",
	}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{
			Event: "added", EID: "E1", Node: hexMod1, Name: "Section",
			NodeType: "test_section", Module: "m", After: strPtr("old-hash"),
			GitHead: "seedhead", Proposal: "seed-p", Path: "m/test_section.md",
		},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "E1"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafeeeee", Proposal: "p5",
		Ops: []plan.Op{
			{OpID: "op-1", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-old"}, Reason: "Spec node modified: m/Section"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCloses != 1 || sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 ok close / 1 event / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 4 {
		t.Fatalf("journal has %d lines, want 4 (2 seed + 2 new): %+v", len(journal), journal)
	}
	modified := journal[2]
	if modified.Event != "modified" || modified.Node != hexMod1 {
		t.Fatalf("line 3 = %+v, want modified event for %s", modified, hexMod1)
	}
	if modified.Before == nil || *modified.Before != "old-hash" {
		t.Errorf("modified before = %v, want old-hash", modified.Before)
	}
	if modified.After == nil || *modified.After != "new-hash" {
		t.Errorf("modified after = %v, want new-hash", modified.After)
	}
	closed := journal[3]
	if closed.Event != "task_closed" || closed.TaskID != "br-old" || closed.For != modified.EID {
		t.Errorf("task_closed = %+v, want for=%s task_id=br-old", closed, modified.EID)
	}
}

// TestApply_ModifyPairCreateErrored_ClosePartialRunTolerated covers the
// partial-receipts shape from the PR discussion: a modify-pair's create
// errors while its paired close comes back ok, alongside an unrelated ok
// create. The errored create must not poison the rest of the batch — the
// unrelated create still lands, and the orphaned close constructs nothing
// rather than failing the whole run.
func TestApply_ModifyPairCreateErrored_ClosePartialRunTolerated(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "new-a", NodeType: "component"}
	graph.nodes[hexB] = NodeMetadata{Module: "m", Component: "B", ContentFile: "b.md", SpecHash: "hb", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: "seedA", Node: hexA, Name: "A", NodeType: "component", Module: "m", After: strPtr("old-a"), GitHead: "seedhead", Proposal: "seed-p", Path: "a.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seedA"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafe4321", Proposal: "p6",
		Ops: []plan.Op{
			{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA), Deps: []plan.Ref{{Kind: plan.RefBead, BeadID: "br-old", EdgeType: "blocks"}}},
			{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:" + hexB)},
			{OpID: "op-3", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-old"}, Reason: "Spec node modified: m/A"},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusError, Error: "tracker boom"},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-B", WasExisting: false},
			{OpID: "op-3", Status: adapters.OpStatusOk, BeadID: "br-old"},
		},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 1 || sum.OkCloses != 1 || sum.Errors != 1 {
		t.Errorf("summary = %+v, want 1 ok create / 1 ok close / 1 error", sum)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event / 1 receipt appended (B only)", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 4 {
		t.Fatalf("journal has %d lines, want 4 (2 seed + 2 new): %+v", len(journal), journal)
	}
	for _, ev := range journal[2:] {
		if ev.Node == hexA {
			t.Fatalf("errored create for A appears in journal: %+v", ev)
		}
		if ev.TaskID == "br-old" {
			t.Fatalf("orphaned close for br-old appears in journal: %+v", ev)
		}
	}
	added := journal[2]
	if added.Event != "added" || added.Node != hexB {
		t.Fatalf("line 3 = %+v, want added event for B", added)
	}
	created := journal[3]
	if created.Event != "task_created" || created.TaskID != "br-B" || created.For != added.EID {
		t.Errorf("task_created = %+v, want for=%s task_id=br-B", created, added.EID)
	}
}

// TestApply_CleanupCreate_NoReferentRefusedBeforeAppend covers "Receipt
// referencing nothing → refused before append": a cleanup whose hash
// matches no removed event anywhere is a malformed changeset, not a
// fallback.
func TestApply_CleanupCreate_NoReferentRefusedBeforeAppend(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "cleanup", SpecNodeID: "nosuchnode00",
			Idempotency: idem("spex:nosuchnode00"),
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-x"}},
	}

	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Apply: got %v, want invariant 1 error", err)
	}
	if len(journalBytes(t, dir)) != 0 {
		t.Error("journal file written despite refused batch")
	}
}

// TestApply_AtomicOnConstructionFailure proves batch atomicity across a
// multi-op call: an earlier op's lines must not land when a later op in
// the same batch fails construction.
func TestApply_AtomicOnConstructionFailure(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "h", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{
			{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
			{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "cleanup", SpecNodeID: "nosuchnode00", Idempotency: idem("spex:nosuchnode00")},
		},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-x"},
		},
	}

	if _, err := r.Apply(cs, rc); err == nil {
		t.Fatal("Apply: want error, got nil")
	}
	if len(journalBytes(t, dir)) != 0 {
		t.Error("earlier op's lines landed despite later op's construction failure")
	}
}

// TestApply_Idempotent_RerunAppendsNothing covers "Idempotent Append":
// re-running Reconciler.Apply with the same changeset+receipts pair over
// the already-appended journal appends zero lines and reports success —
// asserted by content equality, not just line count.
func TestApply_Idempotent_RerunAppendsNothing(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "h", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A", WasExisting: false}},
	}

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := journalBytes(t, dir)

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("second Apply summary = %+v, want zero appends", sum)
	}
	after := journalBytes(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatalf("re-run mutated the journal:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestApply_RejectsMissingReceipt verifies the changeset/receipts pair is
// op_id-balanced before reconciliation runs.
func TestApply_RejectsMissingReceipt(t *testing.T) {
	graph := newFakeSpecGraph()
	r, _ := newTestReconciler(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:A")},
		{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:B")},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
	}}
	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), "no receipt for op op-2") {
		t.Errorf("Apply: got %v, want missing-receipt error", err)
	}
}

// TestApply_RejectsExtraReceipt verifies the converse: an extra receipt
// op_id is also rejected as a contract violation.
func TestApply_RejectsExtraReceipt(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "A"}
	r, _ := newTestReconciler(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:A")},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-stray", Status: adapters.OpStatusOk, BeadID: "br-stray"},
	}}
	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), "op-stray") {
		t.Errorf("Apply: got %v, want extra-receipt error", err)
	}
}

// TestDeriveEID_DeterministicAndDistinct covers "eid derived from
// (git_head, op_id)": stable across calls, distinct per input.
func TestDeriveEID_DeterministicAndDistinct(t *testing.T) {
	if deriveEID("g1", "op-1") != deriveEID("g1", "op-1") {
		t.Error("deriveEID is not deterministic")
	}
	if deriveEID("g1", "op-1") == deriveEID("g2", "op-1") {
		t.Error("deriveEID ignores git_head")
	}
	if deriveEID("g1", "op-1") == deriveEID("g1", "op-2") {
		t.Error("deriveEID ignores op_id")
	}
}

// TestApply_OkRetarget_ModifiedEventAndTaskRetargetedAppended covers "Ok
// retarget → modified event and task_retargeted receipt appended": no
// task_closed, no task_created — the task neither died nor was born, and
// the fold answers with the new event's task id.
func TestApply_OkRetarget_ModifiedEventAndTaskRetargetedAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexR] = NodeMetadata{Module: "m", Component: "R", ContentFile: "r.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: "seedR", Node: hexR, Name: "R", NodeType: "component", Module: "m", After: strPtr("old-r"), GitHead: "seedhead", Proposal: "seed-p", Path: "r.md"},
		mapping.Event{Event: "task_created", TaskID: "br-open", For: "seedR"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafe1234", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: hexR, SpecHash: "new-r",
			Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-open"},
			Labels: []string{"spex:cafe1234:op-1"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-open"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 4 {
		t.Fatalf("journal has %d lines, want 4 (2 seed + 2 new): %+v", len(journal), journal)
	}
	modified := journal[2]
	if modified.Event != "modified" || modified.Node != hexR {
		t.Fatalf("line 3 = %+v, want modified event for %s", modified, hexR)
	}
	if modified.Before == nil || *modified.Before != "old-r" {
		t.Errorf("modified before = %v, want old-r", modified.Before)
	}
	if modified.After == nil || *modified.After != "new-r" {
		t.Errorf("modified after = %v, want new-r", modified.After)
	}
	retargeted := journal[3]
	if retargeted.Event != "task_retargeted" || retargeted.TaskID != "br-open" || retargeted.For != modified.EID {
		t.Errorf("task_retargeted = %+v, want for=%s task_id=br-open", retargeted, modified.EID)
	}

	fold, err := mapping.NewMappingStore(filepath.Join(dir, ".history.jsonl")).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, e := range fold.Entries {
		if e.Key == hexR {
			found = true
			if e.TaskID != "br-open" {
				t.Errorf("fold entry task_id = %q, want br-open", e.TaskID)
			}
			if e.Source.EID != modified.EID {
				t.Errorf("fold entry sourced from %q, want the new modified event %q", e.Source.EID, modified.EID)
			}
		}
	}
	if !found {
		t.Fatal("no fold entry for retargeted node")
	}
}

// TestApply_Retarget_ReRun_Idempotent covers "Retarget re-run →
// idempotent no-op": both lines dedup by derived event id.
func TestApply_Retarget_ReRun_Idempotent(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexR] = NodeMetadata{Module: "m", Component: "R", ContentFile: "r.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: hexR, SpecHash: "new-r",
			Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-open"},
			Labels: []string{"spex:g:op-1"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-open"}},
	}

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := journalBytes(t, dir)

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("second Apply summary = %+v, want zero appends", sum)
	}
	after := journalBytes(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatalf("re-run mutated the journal:\nbefore: %s\nafter:  %s", before, after)
	}
}

// TestApply_Retarget_ErrorReceipt_NothingAppended covers "Retarget with
// error receipt → nothing appended".
func TestApply_Retarget_ErrorReceipt_NothingAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: hexR, SpecHash: "new-r",
			Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-open"},
			Labels: []string{"spex:g:op-1"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusError, Error: "tracker boom"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Errors != 1 || sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("summary = %+v, want 1 error / zero appends", sum)
	}
	if len(readJournal(t, dir)) != 0 {
		t.Error("errored retarget appended lines")
	}
}

// TestApply_Absorbed_ModifiedEventAndRefreshReceiptAppended covers
// "Absorbed entry → modified event and refresh receipt appended": an
// empty-ops changeset carrying one absorbed entry.
func TestApply_Absorbed_ModifiedEventAndRefreshReceiptAppended(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexAbs1] = NodeMetadata{Module: "m", Component: "Abs1", ContentFile: "abs1.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafeabs1", Proposal: "p",
		Absorbed: []plan.AbsorbedEntry{{Node: hexAbs1, Before: "aaa", After: "bbb", Reason: "typo sweep"}},
	}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 2 {
		t.Fatalf("journal has %d lines, want 2: %+v", len(journal), journal)
	}
	modified := journal[0]
	if modified.Event != "modified" || modified.Node != hexAbs1 {
		t.Fatalf("line 1 = %+v, want modified event for %s", modified, hexAbs1)
	}
	if modified.Before == nil || *modified.Before != "aaa" || modified.After == nil || *modified.After != "bbb" {
		t.Errorf("modified before/after = %v/%v, want aaa/bbb", modified.Before, modified.After)
	}
	refresh := journal[1]
	if refresh.Event != "refresh" || len(refresh.Absorbed) != 1 || refresh.Absorbed[0] != modified.EID {
		t.Errorf("refresh receipt = %+v, want absorbed=[%s]", refresh, modified.EID)
	}
}

// TestApply_Absorbed_EmptyArray_ConstructsNothing covers "An empty
// absorbed array constructs nothing, not an empty receipt."
func TestApply_Absorbed_EmptyArray_ConstructsNothing(t *testing.T) {
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p"}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("summary = %+v, want zero appends", sum)
	}
	if len(readJournal(t, dir)) != 0 {
		t.Error("empty absorbed array appended lines")
	}
}

// TestApply_Absorbed_LandOnPartialRuns covers "Absorbed entries land on
// partial runs too": an errored create alongside an absorbed entry still
// lands the absorbed entry — absorption is not receipt-gated.
func TestApply_Absorbed_LandOnPartialRuns(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexAbs1] = NodeMetadata{Module: "m", Component: "Abs1", ContentFile: "abs1.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops:      []plan.Op{{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)}},
		Absorbed: []plan.AbsorbedEntry{{Node: hexAbs1, Before: "aaa", After: "bbb", Reason: "typo sweep"}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusError, Error: "tracker boom"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Errors != 1 || sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 error, 1 event / 1 receipt (absorbed only)", sum)
	}

	journal := readJournal(t, dir)
	if len(journal) != 2 {
		t.Fatalf("journal has %d lines, want 2 (absorbed only): %+v", len(journal), journal)
	}
	if journal[0].Event != "modified" || journal[0].Node != hexAbs1 {
		t.Errorf("line 1 = %+v, want modified event for %s", journal[0], hexAbs1)
	}
	if journal[1].Event != "refresh" {
		t.Errorf("line 2 = %+v, want refresh receipt", journal[1])
	}
}

// TestApply_Absorbed_ReRun_Idempotent covers "Absorbed re-run →
// idempotent no-op": the (node, before, after) derivation finds the event
// present, and an empty remainder appends no second refresh receipt.
func TestApply_Absorbed_ReRun_Idempotent(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexAbs1] = NodeMetadata{Module: "m", Component: "Abs1", ContentFile: "abs1.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Absorbed: []plan.AbsorbedEntry{{Node: hexAbs1, Before: "aaa", After: "bbb", Reason: "typo sweep"}},
	}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete}

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	before := journalBytes(t, dir)

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("second Apply summary = %+v, want zero appends", sum)
	}
	after := journalBytes(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatalf("re-run mutated the journal:\nbefore: %s\nafter:  %s", before, after)
	}
}

// ---- Invariant unit tests (direct, low-level) ----

// TestCheckInvariant1_DoublePairing covers "Invariant 1: double pairing
// refused": two task_created receipts pairing with the same change event.
func TestCheckInvariant1_DoublePairing(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t1", For: "eid-1"},
		{Event: "task_created", TaskID: "t2", For: "eid-1"},
	}
	err := checkInvariant1(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("checkInvariant1: got %v, want invariant 1 error", err)
	}
}

// TestCheckInvariant1_ProposalDoublePairing covers the epic analogue: two
// task_created receipts both claiming the same proposal slug.
func TestCheckInvariant1_ProposalDoublePairing(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t1", Proposal: "stem"},
		{Event: "task_created", TaskID: "t2", Proposal: "stem"},
	}
	err := checkInvariant1(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("checkInvariant1: got %v, want invariant 1 error", err)
	}
}

// TestCheckInvariant1_RetargetDoublePairing covers the retarget analogue:
// two task_retargeted receipts pairing with the same modified event.
func TestCheckInvariant1_RetargetDoublePairing(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_retargeted", TaskID: "t1", For: "eid-1"},
		{Event: "task_retargeted", TaskID: "t2", For: "eid-1"},
	}
	err := checkInvariant1(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("checkInvariant1: got %v, want invariant 1 error", err)
	}
}

// TestCheckInvariant2_DanglingReference covers "Invariant 2: dangling
// receipt reference".
func TestCheckInvariant2_DanglingReference(t *testing.T) {
	batch := []mapping.Event{{Event: "task_created", TaskID: "t1", For: "missing-eid"}}
	err := checkInvariant2(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("checkInvariant2: got %v, want %q", err, want)
	}
}

// TestCheckInvariant2_RetargetDanglingReference covers the retarget
// analogue: a task_retargeted receipt's for names an eid neither the
// journal nor the batch contains.
func TestCheckInvariant2_RetargetDanglingReference(t *testing.T) {
	batch := []mapping.Event{{Event: "task_retargeted", TaskID: "t1", For: "missing-eid"}}
	err := checkInvariant2(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("checkInvariant2: got %v, want %q", err, want)
	}
}

// TestCheckInvariant2_AbsorbedDanglingReference covers "Invariant 2's
// no-unknown-referent rule covers the absorbed list exactly as it covers
// for": a refresh receipt naming an eid no absorbed event carries.
func TestCheckInvariant2_AbsorbedDanglingReference(t *testing.T) {
	batch := []mapping.Event{
		{Event: "modified", EID: "e1"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e1", "missing-eid"}},
	}
	err := checkInvariant2(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("checkInvariant2: got %v, want %q", err, want)
	}
}

// TestCheckInvariant2_KnownAgainstExisting confirms a receipt whose for
// resolves against the EXISTING journal (not just the batch) passes.
func TestCheckInvariant2_KnownAgainstExisting(t *testing.T) {
	existing := []mapping.Event{{Event: "added", EID: "E1"}}
	batch := []mapping.Event{{Event: "task_created", TaskID: "t1", For: "E1"}}
	if err := checkInvariant2(existing, batch); err != nil {
		t.Fatalf("checkInvariant2: unexpected error %v", err)
	}
}

// TestCheckInvariant5_SchemaInvalidLine covers "Invariant 5:
// schema-invalid line refused": a change event with an invalid node hash
// fails schema validation before any write.
func TestCheckInvariant5_SchemaInvalidLine(t *testing.T) {
	batch := []mapping.Event{{
		Event: "added", EID: "e1", Node: "", Name: "x", NodeType: "component",
		Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}}
	err := checkInvariant5(batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 5") {
		t.Fatalf("checkInvariant5: got %v, want invariant 5 error", err)
	}
}

// TestCheckInvariant5_ValidLinePasses is the control: a well-formed line
// of each kind must validate cleanly.
func TestCheckInvariant5_ValidLinePasses(t *testing.T) {
	batch := []mapping.Event{
		{Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "x", NodeType: "component", Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p"},
		{Event: "task_created", TaskID: "t1", For: "e1"},
	}
	if err := checkInvariant5(batch); err != nil {
		t.Fatalf("checkInvariant5: unexpected error %v", err)
	}
}

// TestCheckInvariant5_RetargetAndRefreshLinesValidate covers the two line
// kinds this bead adds: a well-formed task_retargeted receipt and a
// well-formed refresh receipt must both validate cleanly.
func TestCheckInvariant5_RetargetAndRefreshLinesValidate(t *testing.T) {
	batch := []mapping.Event{
		{Event: "modified", EID: "e1", Node: "aabbccddeeff", Name: "x", NodeType: "component", Module: "m", Before: strPtr("h0"), After: strPtr("h1"), GitHead: "g", Proposal: "p"},
		{Event: "task_retargeted", TaskID: "t1", For: "e1"},
		{Event: "modified", EID: "e2", Node: "112233445566", Name: "y", NodeType: "component", Module: "m", Before: strPtr("h2"), After: strPtr("h3"), GitHead: "g", Proposal: "p"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e2"}},
	}
	if err := checkInvariant5(batch); err != nil {
		t.Fatalf("checkInvariant5: unexpected error %v", err)
	}
}
