package ingest

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
)

// fakeSpecGraph satisfies SpecGraph from a flat metadata table. Every
// reconciler test seeds the nodes it cares about; nodes left out are
// reported as missing, which is exactly what invariant 4 should fire on.
type fakeSpecGraph struct {
	nodes map[string]NodeMetadata
}

func newFakeSpecGraph() *fakeSpecGraph {
	return &fakeSpecGraph{nodes: map[string]NodeMetadata{}}
}

func (s *fakeSpecGraph) HasNode(id string) bool {
	_, ok := s.nodes[id]
	return ok
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

// newTestStore creates a fresh fileStore over a temp .bead-map.json,
// seeded with the given records and counter so every test starts from
// a known on-disk state.
func newTestStore(t *testing.T, records []mapping.Record, nextID int) (mapping.Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(path)
	if err := store.Replace(records, nextID); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	return store, path
}

// idem builds the *Idem struct emit attaches to create ops.
func idem(label string) *emit.Idem { return &emit.Idem{Label: label} }

// TestApply_OkCreate covers the "Ok create → record inserted" scenario
// from test_reconciliation.md. A single create op with was_existing=false
// inserts a fresh record at the label's record-id.
func TestApply_OkCreate(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["abc123def456"] = NodeMetadata{
		Module:      "ingest",
		Component:   "Reconciler",
		ContentFile: "spec/ingest/arch_reconciler.md",
		SpecHash:    "hash-abc",
		NodeType:    "component",
	}

	store, _ := newTestStore(t, nil, 42)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{{
			OpID:         "op-1",
			Type:         emit.OpCreate,
			SpecNodeKind: "component",
			SpecNodeID:   "abc123def456",
			Idempotency:  idem("spex:42"),
			Title:        "ingest: Reconciler",
		}},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-1",
			Status:      adapters.OpStatusOk,
			BeadID:      "br-new",
			WasExisting: false,
		}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.RecordsAdded != 1 || sum.OkCreates != 1 {
		t.Errorf("summary = %+v, want 1 added/1 ok create", sum)
	}

	rec, err := store.Get(42)
	if err != nil {
		t.Fatalf("Get(42): %v", err)
	}
	if rec.SpecNodeID != "abc123def456" || rec.BeadID != "br-new" {
		t.Errorf("record = %+v, want spec_node abc123def456 / bead br-new", rec)
	}
	if rec.Module != "ingest" || rec.Component != "Reconciler" || rec.ContentFile != "spec/ingest/arch_reconciler.md" {
		t.Errorf("metadata not materialised: %+v", rec)
	}
	if rec.SpecHash != "hash-abc" {
		t.Errorf("spec_hash = %q, want hash-abc", rec.SpecHash)
	}

	next, err := store.NextRecordID()
	if err != nil {
		t.Fatalf("NextRecordID: %v", err)
	}
	if next != 43 {
		t.Errorf("counter = %d, want 43", next)
	}
}

// TestApply_OkCreate_ProposalEpic covers the "Proposal-Epic Ops" rule
// from arch_reconciler.md: proposal-epic creates skip the SpecGraph
// lookup (their spec_node_id is the proposal stem, not an identity hash)
// and materialise a record with node_type="proposal" and component=<stem>.
// Regression for the bug surfaced on /converge against any fresh proposal:
// the reconciler used to call SpecGraph.NodeMetadata(<stem>) and abort
// with "spec graph: no node <stem>" before any per-op transition committed.
func TestApply_OkCreate_ProposalEpic(t *testing.T) {
	// Empty graph — the proposal stem MUST NOT be looked up. If the
	// implementation forgets the special case, the empty fakeSpecGraph
	// returns "no node" and the test fails.
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, nil, 77)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	stem := "2026-04-29-decouple-contract-gaps"
	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{{
			OpID:         "op-1",
			Type:         emit.OpCreate,
			SpecNodeKind: "proposal_epic",
			SpecNodeID:   stem,
			Idempotency:  idem("spex:77"),
			Title:        "Proposal: " + stem,
		}},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-1",
			Status:      adapters.OpStatusOk,
			BeadID:      "br-epic",
			WasExisting: false,
		}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.RecordsAdded != 1 || sum.OkCreates != 1 {
		t.Errorf("summary = %+v, want 1 added/1 ok create", sum)
	}

	rec, err := store.Get(77)
	if err != nil {
		t.Fatalf("Get(77): %v", err)
	}
	if rec.NodeType != "proposal" {
		t.Errorf("node_type = %q, want proposal (NOT proposal_epic — see arch_reconciler.md)", rec.NodeType)
	}
	if rec.BeadType != "epic" {
		t.Errorf("bead_type = %q, want epic", rec.BeadType)
	}
	if rec.Component != stem {
		t.Errorf("component = %q, want %q (the stem)", rec.Component, stem)
	}
	if rec.Module != "" || rec.ContentFile != "" || rec.SpecHash != "" {
		t.Errorf("proposal-epic record should have empty module/content_file/spec_hash, got %+v", rec)
	}
}

// TestApply_OkCreate_Cleanup covers the "Cleanup-Create Ops" rule from
// arch_reconciler.md: cleanup creates produce no mapping record. The
// counter does not advance. Invariant 1 is exempt.
func TestApply_OkCreate_Cleanup(t *testing.T) {
	// Empty graph — cleanup ops carry the identity hash of a removed
	// spec node, which is not in the graph by design. The reconciler
	// MUST NOT call SpecGraph.NodeMetadata; the cleanup branch returns
	// before any lookup.
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, nil, 50)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{{
			OpID:         "op-1",
			Type:         emit.OpCreate,
			SpecNodeKind: "cleanup",
			SpecNodeID:   "abc123def456",
			Idempotency:  &emit.Idem{Label: "spex:cleanup-abc123def456"},
			Title:        "Code cleanup: m/X",
			Labels:       []string{"spex:cleanup"},
			Deps: []emit.Ref{
				{Kind: emit.RefBead, BeadID: "spexmachina-old", EdgeType: "blocks"},
			},
		}},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-1",
			Status:      adapters.OpStatusOk,
			BeadID:      "br-cleanup",
			WasExisting: false,
		}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// No record materialised.
	if sum.RecordsAdded != 0 {
		t.Errorf("RecordsAdded = %d, want 0 (cleanup creates have no map record)", sum.RecordsAdded)
	}
	// Counter NOT advanced.
	next, err := store.NextRecordID()
	if err != nil {
		t.Fatalf("NextRecordID: %v", err)
	}
	if next != 50 {
		t.Errorf("counter = %d, want 50 (cleanup ops do not consume the cursor)", next)
	}
	// OkCreates IS counted (the op was processed successfully).
	if sum.OkCreates != 1 {
		t.Errorf("OkCreates = %d, want 1", sum.OkCreates)
	}
	// Invariant 1 must NOT have fired — Apply returned nil. (If it had
	// fired, Apply would have returned the invariant error.)
}

// TestApply_OkClose_RemovedDeletes covers the "Ok close on removed →
// record deleted" scenario.
func TestApply_OkClose_RemovedDeletes(t *testing.T) {
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 5, SpecNodeID: "xyz", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
	}, 6)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{{
			OpID:   "op-1",
			Type:   emit.OpClose,
			Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"},
			Reason: "Spec node removed: xyz",
		}},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops:     []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.RecordsDeleted != 1 || sum.OkCloses != 1 {
		t.Errorf("summary = %+v, want 1 deleted/1 ok close", sum)
	}
	if _, err := store.Get(5); err == nil {
		t.Error("record 5 still exists after Spec node removed close")
	}
}

// TestApply_ModifiedPair_UpdatesBeadID covers "Modified node:
// close+create → record updated" — same record-id label re-used by
// emit, the close is a no-op, the create rebinds the record.
func TestApply_ModifiedPair_UpdatesBeadID(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["mod1"] = NodeMetadata{
		Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "new-hash", NodeType: "component",
	}

	store, _ := newTestStore(t, []mapping.Record{
		{ID: 10, SpecNodeID: "mod1", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "old-hash"},
	}, 11)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{
			{
				OpID:   "op-1",
				Type:   emit.OpClose,
				Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"},
				Reason: "Spec node modified: mod1",
			},
			{
				OpID:         "op-2",
				Type:         emit.OpCreate,
				SpecNodeKind: "component",
				SpecNodeID:   "mod1",
				Idempotency:  idem("spex:10"),
				Title:        "m: C",
			},
		},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-new", WasExisting: false},
		},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.RecordsUpdated != 1 {
		t.Errorf("summary = %+v, want RecordsUpdated=1", sum)
	}
	rec, err := store.Get(10)
	if err != nil {
		t.Fatalf("Get(10): %v", err)
	}
	if rec.BeadID != "br-new" {
		t.Errorf("bead_id = %q, want br-new", rec.BeadID)
	}
	if rec.SpecNodeID != "mod1" {
		t.Errorf("spec_node_id changed: %q", rec.SpecNodeID)
	}
	if rec.SpecHash != "new-hash" {
		t.Errorf("spec_hash = %q, want new-hash", rec.SpecHash)
	}
}

// TestApply_WasExisting_NoOp covers the "Was_existing=true → idempotent
// no-op" scenario: the adapter matched our label against an existing
// bead, the receipt's bead_id matches our stored bead_id, no changes.
func TestApply_WasExisting_NoOp(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "C", SpecHash: "h"}

	store, _ := newTestStore(t, []mapping.Record{
		{ID: 7, SpecNodeID: "A", BeadID: "br-7", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
	}, 8)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{{
			OpID:         "op-1",
			Type:         emit.OpCreate,
			SpecNodeKind: "component",
			SpecNodeID:   "A",
			Idempotency:  idem("spex:7"),
		}},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops:     []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-7", WasExisting: true}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.RecordsAdded != 0 || sum.RecordsUpdated != 0 {
		t.Errorf("summary = %+v, want zero record changes", sum)
	}
	rec, err := store.Get(7)
	if err != nil {
		t.Fatalf("Get(7): %v", err)
	}
	if rec.BeadID != "br-7" || rec.SpecHash != "h" {
		t.Errorf("record drifted: %+v", rec)
	}
}

// TestApply_WasExisting_DriftIsError verifies that a was_existing=true
// receipt whose bead_id contradicts the stored record is rejected — the
// adapter's idempotent re-match found a different bead than we have.
func TestApply_WasExisting_DriftIsError(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "C"}
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 7, SpecNodeID: "A", BeadID: "br-7", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
	}, 8)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:7")}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-different", WasExisting: true}}}

	if _, err := r.Apply(cs, rc); err == nil {
		t.Fatal("Apply: want drift error, got nil")
	}
	// Original record must remain intact (no commit).
	rec, _ := store.Get(7)
	if rec.BeadID != "br-7" {
		t.Errorf("on-disk record changed despite error: %+v", rec)
	}
}

// TestApply_ErrorStatus_NoMappingChange covers "Error status → op
// skipped, no mapping change". The reconciler counts the error in the
// summary but leaves records untouched.
func TestApply_ErrorStatus_NoMappingChange(t *testing.T) {
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1"),
	}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusError, BeadID: "", Error: "tracker exited 1",
	}}}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Errors != 1 || sum.RecordsAdded != 0 {
		t.Errorf("summary = %+v, want 1 error and zero records added", sum)
	}
	if _, err := store.Get(1); err == nil {
		t.Error("record 1 inserted despite error receipt")
	}
}

// TestApply_SkippedStatus_NoOp covers "Skipped status → op no-op".
func TestApply_SkippedStatus_NoOp(t *testing.T) {
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1")}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusSkipped, Reason: "already labelled"}}}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Skipped != 1 || sum.RecordsAdded != 0 {
		t.Errorf("summary = %+v, want skipped=1", sum)
	}
}

// TestApply_MixedOps_CarefulOrdering covers the "Mixed ops" scenario
// from test_reconciliation.md: a modified close+create pair, a removed
// close, and a fresh create — final state has the right rebinds and
// deletes.
func TestApply_MixedOps_CarefulOrdering(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["X"] = NodeMetadata{Module: "m", Component: "X", ContentFile: "x.md", SpecHash: "hx", NodeType: "component"}
	graph.nodes["Y"] = NodeMetadata{Module: "m", Component: "Y", ContentFile: "y.md", SpecHash: "hy", NodeType: "component"}

	store, _ := newTestStore(t, []mapping.Record{
		{ID: 5, SpecNodeID: "X", BeadID: "br-A", BeadType: "feature", Module: "m", Component: "X", ContentFile: "x.md", SpecHash: "old"},
		{ID: 6, SpecNodeID: "Z", BeadID: "br-B", BeadType: "feature", Module: "m", Component: "Z", ContentFile: "z.md", SpecHash: "hz"},
	}, 7)
	graph.nodes["Z"] = NodeMetadata{} // Z is on its way out via close-on-removed; reference graph only needs X and Y after apply.

	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{
		Version: 1,
		Ops: []emit.Op{
			{OpID: "op-1", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-A"}, Reason: "Spec node modified: X"},
			{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "X", Idempotency: idem("spex:5")},
			{OpID: "op-3", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-B"}, Reason: "Spec node removed: Z"},
			{OpID: "op-4", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "Y", Idempotency: idem("spex:7")},
		},
	}
	rc := adapters.Receipts{
		Version: 1,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
			{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-A2", WasExisting: false},
			{OpID: "op-3", Status: adapters.OpStatusOk, BeadID: "br-B"},
			{OpID: "op-4", Status: adapters.OpStatusOk, BeadID: "br-Y", WasExisting: false},
		},
	}

	// After Z is closed-on-removed it must not remain in the spec graph
	// for invariant 4 to pass. Drop it from the fake graph.
	delete(graph.nodes, "Z")

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	recX, err := store.Get(5)
	if err != nil {
		t.Fatalf("Get(5): %v", err)
	}
	if recX.BeadID != "br-A2" || recX.SpecNodeID != "X" {
		t.Errorf("record 5 = %+v, want bead br-A2 / spec X", recX)
	}
	recY, err := store.Get(7)
	if err != nil {
		t.Fatalf("Get(7): %v", err)
	}
	if recY.BeadID != "br-Y" || recY.SpecNodeID != "Y" {
		t.Errorf("record 7 = %+v, want bead br-Y / spec Y", recY)
	}
	if _, err := store.Get(6); err == nil {
		t.Error("record 6 (Z) still present after close-on-removed")
	}
}

// TestApply_CounterAdvances covers "Counter Advance": three ok creates
// at spex:100..102 land the counter at 103.
func TestApply_CounterAdvances(t *testing.T) {
	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, SpecHash: "h" + id, NodeType: "component"}
	}
	store, _ := newTestStore(t, nil, 100)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:100")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:101")},
		{OpID: "op-3", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:102")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB"},
		{OpID: "op-3", Status: adapters.OpStatusOk, BeadID: "brC"},
	}}
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	next, _ := store.NextRecordID()
	if next != 103 {
		t.Errorf("next = %d, want 103", next)
	}
}

// TestApply_CounterAdvances_OnlyCommittedLabels covers the second bullet
// of "Counter Advance": two oks at 100/101 plus one error at 102 leaves
// the counter at 102 — the error op never reserved storage.
func TestApply_CounterAdvances_OnlyCommittedLabels(t *testing.T) {
	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, SpecHash: "h" + id, NodeType: "component"}
	}
	store, _ := newTestStore(t, nil, 100)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:100")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:101")},
		{OpID: "op-3", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:102")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB"},
		{OpID: "op-3", Status: adapters.OpStatusError, Error: "tracker boom"},
	}}
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	next, _ := store.NextRecordID()
	if next != 102 {
		t.Errorf("next = %d, want 102", next)
	}
}

// TestApply_RejectsMissingReceipt verifies the changeset/receipts pair
// is op_id-balanced before reconciliation runs. A missing receipt is an
// emit-or-adapter contract bug, not a runtime invariant.
func TestApply_RejectsMissingReceipt(t *testing.T) {
	graph := newFakeSpecGraph()
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:2")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
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
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1")}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-stray", Status: adapters.OpStatusOk, BeadID: "br-stray"},
	}}
	_, err := r.Apply(cs, rc)
	if err == nil || !strings.Contains(err.Error(), "op-stray") {
		t.Errorf("Apply: got %v, want extra-receipt error", err)
	}
}

// TestApply_AtomicOnInvariantFailure covers the transactional contract:
// an invariant violation must leave .bead-map.json untouched on disk.
// Here invariant 4 fires because the create's spec_node_id is missing
// from the spec graph (orphan).
func TestApply_AtomicOnInvariantFailure(t *testing.T) {
	graph := newFakeSpecGraph()
	// Deliberately do not register "ghost" — the create will leave an
	// orphan record. Metadata lookup still needs to succeed for the
	// create handler to populate the record, so seed metadata-only.
	graph.nodes["ghost"] = NodeMetadata{Module: "m", Component: "C", SpecHash: "h", NodeType: "component"}
	graph.nodes["live"] = NodeMetadata{Module: "m", Component: "C2", SpecHash: "h2", NodeType: "component"}

	store, _ := newTestStore(t, []mapping.Record{
		{ID: 1, SpecNodeID: "live", BeadID: "br-live", BeadType: "feature", Module: "m", Component: "C2", ContentFile: "f.md", SpecHash: "h2"},
	}, 2)

	r := &Reconciler{
		MappingStore: store,
		SpecGraph: &droppingGraph{
			inner: graph,
			drop:  map[string]bool{"ghost": true},
		},
	}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "ghost", Idempotency: idem("spex:2"),
	}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-ghost",
	}}}

	if _, err := r.Apply(cs, rc); err == nil {
		t.Fatal("Apply: want invariant 4 error, got nil")
	}
	// Pre-existing record must still be there; the new orphan must not.
	if _, err := store.Get(1); err != nil {
		t.Errorf("Get(1) after rollback: %v", err)
	}
	if _, err := store.Get(2); err == nil {
		t.Error("orphan record committed despite invariant failure")
	}
}

// droppingGraph hides specific spec_node_ids from HasNode while still
// returning their metadata via NodeMetadata. This simulates a
// mid-reconcile divergence — useful for forcing invariant 4 to fire on
// records the per-op handlers have already produced.
type droppingGraph struct {
	inner *fakeSpecGraph
	drop  map[string]bool
}

func (g *droppingGraph) HasNode(id string) bool {
	if g.drop[id] {
		return false
	}
	return g.inner.HasNode(id)
}

func (g *droppingGraph) NodeMetadata(id string) (NodeMetadata, error) {
	return g.inner.NodeMetadata(id)
}

// ---- Invariant tests ----

// TestInvariant1_OkCreateMissingRecord exercises the spec scenario for
// invariant 1 by injecting a state where an ok create has no record. We
// simulate the "reconciler bug" by calling assertInvariants directly on
// a working copy that the per-op handlers never populated.
func TestInvariant1_OkCreateMissingRecord(t *testing.T) {
	r := &Reconciler{}
	wc := newWorkingCopy(nil, 1)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1"),
	}}}
	receiptsByOp := map[string]adapters.OpReceipt{
		"op-1": {OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
	}
	err := r.assertInvariants(wc, cs, receiptsByOp)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Errorf("got %v, want invariant 1 error", err)
	}
}

// TestInvariant2_RemovedLeavesRecord covers invariant 2: a close-on-
// removed op whose record was not actually deleted from the working
// copy must surface a clear error.
func TestInvariant2_RemovedLeavesRecord(t *testing.T) {
	r := &Reconciler{}
	wc := newWorkingCopy([]mapping.Record{
		{ID: 5, SpecNodeID: "X", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "X", ContentFile: "x.md", SpecHash: "h"},
	}, 6)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID:   "op-1",
		Type:   emit.OpClose,
		Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"},
		Reason: "Spec node removed: X",
	}}}
	receiptsByOp := map[string]adapters.OpReceipt{
		"op-1": {OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"},
	}
	err := r.assertInvariants(wc, cs, receiptsByOp)
	if err == nil || !strings.Contains(err.Error(), "invariant 2") {
		t.Errorf("got %v, want invariant 2 error", err)
	}
}

// TestInvariant3_ModifiedPointsToOldBead covers invariant 3: a
// close-on-modified target whose record still references the old
// bead_id is rejected.
func TestInvariant3_ModifiedPointsToOldBead(t *testing.T) {
	r := &Reconciler{}
	wc := newWorkingCopy([]mapping.Record{
		{ID: 10, SpecNodeID: "M", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "M", ContentFile: "m.md", SpecHash: "h"},
	}, 11)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID:   "op-1",
		Type:   emit.OpClose,
		Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"},
		Reason: "Spec node modified: M",
	}}}
	receiptsByOp := map[string]adapters.OpReceipt{
		"op-1": {OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"},
	}
	err := r.assertInvariants(wc, cs, receiptsByOp)
	if err == nil || !strings.Contains(err.Error(), "invariant 3") {
		t.Errorf("got %v, want invariant 3 error", err)
	}
}

// TestInvariant4_OrphanRecord covers invariant 4: a record whose
// spec_node_id is missing from the live spec graph is an orphan.
func TestInvariant4_OrphanRecord(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["live"] = NodeMetadata{Module: "m", Component: "C"}

	r := &Reconciler{SpecGraph: graph}
	wc := newWorkingCopy([]mapping.Record{
		{ID: 1, SpecNodeID: "orphan1", BeadID: "br-1", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
	}, 2)

	err := r.assertInvariants(wc, emit.Changeset{Version: 1}, map[string]adapters.OpReceipt{})
	if err == nil || !strings.Contains(err.Error(), "invariant 4") {
		t.Errorf("got %v, want invariant 4 error", err)
	}
}

// TestInvariant5_DuplicateSpecNode covers invariant 5: two records with
// the same spec_node_id are forbidden (proposal-epic exemption aside).
func TestInvariant5_DuplicateSpecNode(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["dupe"] = NodeMetadata{Module: "m", Component: "C"}
	r := &Reconciler{SpecGraph: graph}

	wc := newWorkingCopy([]mapping.Record{
		{ID: 1, SpecNodeID: "dupe", BeadID: "br-1", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
		{ID: 2, SpecNodeID: "dupe", BeadID: "br-2", BeadType: "feature", Module: "m", Component: "C", ContentFile: "f.md", SpecHash: "h"},
	}, 3)
	err := r.assertInvariants(wc, emit.Changeset{Version: 1}, map[string]adapters.OpReceipt{})
	if err == nil || !strings.Contains(err.Error(), "invariant 5") {
		t.Errorf("got %v, want invariant 5 error", err)
	}
}

// TestInvariant5_ExemptsProposalEpic verifies proposal records are
// allowed to share spec_node_id, mirroring fileStore.Create's exemption.
func TestInvariant5_ExemptsProposalEpic(t *testing.T) {
	r := &Reconciler{}
	wc := newWorkingCopy([]mapping.Record{
		{ID: 1, NodeType: "proposal", SpecNodeID: "P", BeadID: "br-1", BeadType: "epic", Module: "", Component: "P", ContentFile: "", SpecHash: ""},
		{ID: 2, NodeType: "proposal", SpecNodeID: "P", BeadID: "br-2", BeadType: "epic", Module: "", Component: "P", ContentFile: "", SpecHash: ""},
	}, 3)
	if err := r.assertInvariants(wc, emit.Changeset{Version: 1}, map[string]adapters.OpReceipt{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestInvariant7_SchemaViolationRejectedByReplace covers invariant 7
// indirectly by routing through Store.Replace, which validates the
// candidate state against the bead-map JSON Schema. A record missing
// required fields must be rejected.
func TestInvariant7_SchemaViolationRejectedByReplace(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{} // empty metadata: no module, no component, no content_file.
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1"),
	}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A",
	}}}
	if _, err := r.Apply(cs, rc); err == nil {
		t.Fatal("Apply: want schema validation error, got nil")
	}
}

// TestInvariant_HappyPath covers the spec's happy-path acceptance: a
// realistic mix of creates and closes with full metadata passes every
// invariant and updates the store.
func TestInvariant_HappyPath(t *testing.T) {
	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C", "D", "E", "Mod1", "Mod2"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}

	initial := []mapping.Record{
		{ID: 10, SpecNodeID: "Mod1", BeadID: "br-old1", BeadType: "feature", Module: "m", Component: "Mod1", ContentFile: "Mod1.md", SpecHash: "old"},
		{ID: 11, SpecNodeID: "Mod2", BeadID: "br-old2", BeadType: "feature", Module: "m", Component: "Mod2", ContentFile: "Mod2.md", SpecHash: "old"},
		{ID: 12, SpecNodeID: "GoneSoon", BeadID: "br-gone", BeadType: "feature", Module: "m", Component: "GoneSoon", ContentFile: "Gone.md", SpecHash: "h"},
	}
	store, _ := newTestStore(t, initial, 13)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-01", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old1"}, Reason: "Spec node modified: Mod1"},
		{OpID: "op-02", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "Mod1", Idempotency: idem("spex:10")},
		{OpID: "op-03", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old2"}, Reason: "Spec node modified: Mod2"},
		{OpID: "op-04", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "Mod2", Idempotency: idem("spex:11")},
		{OpID: "op-05", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-gone"}, Reason: "Spec node removed: GoneSoon"},
		{OpID: "op-06", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:13")},
		{OpID: "op-07", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:14")},
		{OpID: "op-08", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:15")},
		{OpID: "op-09", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "D", Idempotency: idem("spex:16")},
		{OpID: "op-10", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "E", Idempotency: idem("spex:17")},
	}}
	delete(graph.nodes, "GoneSoon")

	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-01", Status: adapters.OpStatusOk, BeadID: "br-old1"},
		{OpID: "op-02", Status: adapters.OpStatusOk, BeadID: "br-new1"},
		{OpID: "op-03", Status: adapters.OpStatusOk, BeadID: "br-old2"},
		{OpID: "op-04", Status: adapters.OpStatusOk, BeadID: "br-new2"},
		{OpID: "op-05", Status: adapters.OpStatusOk, BeadID: "br-gone"},
		{OpID: "op-06", Status: adapters.OpStatusOk, BeadID: "br-A"},
		{OpID: "op-07", Status: adapters.OpStatusOk, BeadID: "br-B"},
		{OpID: "op-08", Status: adapters.OpStatusOk, BeadID: "br-C"},
		{OpID: "op-09", Status: adapters.OpStatusOk, BeadID: "br-D"},
		{OpID: "op-10", Status: adapters.OpStatusOk, BeadID: "br-E"},
	}}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.OkCreates != 7 || sum.OkCloses != 3 {
		t.Errorf("summary = %+v, want 7 ok creates / 3 ok closes", sum)
	}
	if sum.RecordsAdded != 5 || sum.RecordsUpdated != 2 || sum.RecordsDeleted != 1 {
		t.Errorf("summary = %+v, want 5 add / 2 upd / 1 del", sum)
	}
	if _, err := store.Get(12); err == nil {
		t.Error("removed record 12 still present")
	}
	rec10, _ := store.Get(10)
	if rec10.BeadID != "br-new1" {
		t.Errorf("Mod1 record bead_id = %q, want br-new1", rec10.BeadID)
	}
}

// TestApply_PartialRunRecovery_Run1 covers the Run 1 setup of
// test_partial_run_recovery.md against the Reconciler in isolation: A,B
// commit, C errors, the close commits, and the persisted counter
// reflects only the ok creates.
func TestApply_PartialRunRecovery_Run1(t *testing.T) {
	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 41, SpecNodeID: "Old", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "Old", ContentFile: "Old.md", SpecHash: "h"},
	}, 42)
	delete(graph.nodes, "Old")

	r := &Reconciler{MappingStore: store, SpecGraph: graph}
	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:42")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:43")},
		{OpID: "op-3", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44")},
		{OpID: "op-4", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"}, Reason: "Spec node removed: Old"},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB"},
		{OpID: "op-3", Status: adapters.OpStatusError, Error: "boom"},
		{OpID: "op-4", Status: adapters.OpStatusOk, BeadID: "br-old"},
	}}
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if rec, err := store.Get(42); err != nil || rec.BeadID != "brA" {
		t.Errorf("record 42 = %+v err %v", rec, err)
	}
	if rec, err := store.Get(43); err != nil || rec.BeadID != "brB" {
		t.Errorf("record 43 = %+v err %v", rec, err)
	}
	if _, err := store.Get(44); err == nil {
		t.Error("error op committed record 44")
	}
	if _, err := store.Get(41); err == nil {
		t.Error("close-on-removed left record 41 behind")
	}
	next, _ := store.NextRecordID()
	if next != 44 {
		t.Errorf("counter after Run 1 = %d, want 44", next)
	}
}

// TestApply_PartialRunRecovery_Run2 simulates the follow-up run: C is
// re-emitted at the same record-id (44), the adapter creates it fresh,
// and the reconciler commits C alongside the unchanged A and B.
func TestApply_PartialRunRecovery_Run2(t *testing.T) {
	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 42, SpecNodeID: "A", BeadID: "brA", BeadType: "feature", Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "hA"},
		{ID: 43, SpecNodeID: "B", BeadID: "brB", BeadType: "feature", Module: "m", Component: "B", ContentFile: "B.md", SpecHash: "hB"},
	}, 44)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44"),
	}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: false,
	}}}
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if rec, err := store.Get(44); err != nil || rec.BeadID != "brC" {
		t.Errorf("record 44 = %+v err %v", rec, err)
	}
	next, _ := store.NextRecordID()
	if next != 45 {
		t.Errorf("counter after Run 2 = %d, want 45", next)
	}
}

// TestApply_PartialRecovery_AdapterDuplicate covers the
// "Partial with Adapter-Side Duplicates" edge case from
// test_partial_run_recovery.md: the dead Run 1 adapter created C in the
// tracker but never wrote a receipt, so Run 2's emit re-emits C with a
// freshly reserved label, the adapter sees a duplicate label match and
// reports was_existing=true. The reconciler must materialise the record
// from the receipt rather than refusing the run — otherwise the work
// would never be reconciled.
func TestApply_PartialRecovery_AdapterDuplicate(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["C"] = NodeMetadata{Module: "m", Component: "C", ContentFile: "C.md", SpecHash: "hC", NodeType: "component"}
	store, _ := newTestStore(t, nil, 44)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44"),
	}}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: true,
	}}}
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	rec, err := store.Get(44)
	if err != nil {
		t.Fatalf("Get(44): %v", err)
	}
	if rec.BeadID != "brC" || rec.SpecNodeID != "C" {
		t.Errorf("recovered record = %+v, want bead brC / spec C", rec)
	}
	next, _ := store.NextRecordID()
	if next != 45 {
		t.Errorf("counter = %d, want 45", next)
	}
}

// TestApply_Idempotent_RerunIsNoOp covers Re-run idempotency from
// module.json's requirements: applying the same changeset+receipts twice
// produces the same end state with no extra mutations on the second run.
func TestApply_Idempotent_RerunIsNoOp(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "hA", NodeType: "component"}
	store, _ := newTestStore(t, nil, 1)
	r := &Reconciler{MappingStore: store, SpecGraph: graph}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{{
		OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1"),
	}}}
	// First run: fresh creation.
	rc1 := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A", WasExisting: false,
	}}}
	if _, err := r.Apply(cs, rc1); err != nil {
		t.Fatalf("Apply run 1: %v", err)
	}
	// Second run: was_existing=true (the adapter's idempotency path
	// matched the bead it created on run 1).
	rc2 := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{
		OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A", WasExisting: true,
	}}}
	sum, err := r.Apply(cs, rc2)
	if err != nil {
		t.Fatalf("Apply run 2: %v", err)
	}
	if sum.RecordsAdded != 0 || sum.RecordsUpdated != 0 {
		t.Errorf("summary = %+v, want zero mutations on re-run", sum)
	}
}

// TestParseRecordID covers the small parser the reconciler uses for
// idempotency.label. Worth exercising directly because a bad label is a
// systemic emit-side bug.
func TestParseRecordID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"happy", "spex:42", 42, false},
		{"zero", "spex:0", 0, false},
		{"missing prefix", "42", 0, true},
		{"non-numeric", "spex:abc", 0, true},
		{"negative", "spex:-1", 0, true},
		{"empty", "", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRecordID(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
