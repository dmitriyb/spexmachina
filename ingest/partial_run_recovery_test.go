package ingest

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
)

// Tests for spec/ingest/test_partial_run_recovery.md. These drive the
// integrated Reconciler+Saver path (via runWithSnapshot, defined in
// consistency_invariants_test.go) so the snapshot-gating half of the
// spec's acceptance is proven alongside the per-op journal behavior:
// after Run 1 (partial) the snapshot is untouched, and after Run 2
// (complete) its root_hash matches an independently-computed merkle root.

// TestPartialRunRecovery_TwoRunSequence drives the full two-run sequence
// from test_partial_run_recovery.md: Run 1 reconciles A, B and the close,
// errors C, and leaves the snapshot untouched. Run 2 commits C and saves
// the snapshot; A and B's journal lines are byte-identical to their Run 1
// form.
func TestPartialRunRecovery_TwoRunSequence(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	baseline := []byte(`{"baseline":true,"root_hash":"deadbeef"}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	graph := newFakeSpecGraph()
	for _, id := range []string{hexA, hexB, hexC} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}
	seedJournal(t, specDir,
		mapping.Event{Event: "added", EID: "seed-Old", Node: hexOld, Name: "Old", NodeType: "component", Module: "m", After: strPtr("h-old"), GitHead: "seedhead", Proposal: "seed-p", Path: "Old.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "seed-Old"},
	)

	run1CS := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "run1head", Proposal: "p1", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:run1head:op-1")},
		{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:run1head:op-2")},
		{OpID: "op-3", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexC, Idempotency: idem("spex:run1head:op-3")},
		{OpID: "op-4", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefBead, BeadID: "br-old"}, Reason: "Spec node removed: m/Old"},
	}}
	run1RC := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB"},
		{OpID: "op-3", Status: adapters.OpStatusError, Error: "tracker boom"},
		{OpID: "op-4", Status: adapters.OpStatusOk, BeadID: "br-old"},
	}}

	_, wrote1 := runWithSnapshot(t, specDir, graph, "", snapPath, run1CS, run1RC)

	journalAfterRun1 := readJournal(t, specDir)
	// Seed (2 lines) + A's added/task_created + B's added/task_created +
	// removed/task_closed for Old. Nothing for C.
	if len(journalAfterRun1) != 8 {
		t.Fatalf("journal after Run 1 has %d lines, want 8: %+v", len(journalAfterRun1), journalAfterRun1)
	}
	for _, ev := range journalAfterRun1 {
		if ev.Node == hexC {
			t.Fatalf("Run 1: error op C appears in journal: %+v", ev)
		}
	}
	if wrote1 {
		t.Error("Run 1: Saver.Save reported wrote=true on partial status")
	}
	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("Run 1: read snapshot: %v", err)
	}
	if string(got) != string(baseline) {
		t.Fatalf("Run 1: snapshot mutated on partial run:\n got %s\nwant %s", got, baseline)
	}
	run1Bytes := journalBytes(t, specDir)

	// Run 2: plan re-emits C fresh (a new commit head, same identity
	// hash). Adapter creates it, status complete.
	run2CS := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "run2head", Proposal: "p1", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexC, Idempotency: idem("spex:run2head:op-1")},
	}}
	run2RC := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: false},
	}}

	_, wrote2 := runWithSnapshot(t, specDir, graph, "", snapPath, run2CS, run2RC)

	run2Bytes := journalBytes(t, specDir)
	if !bytes.HasPrefix(run2Bytes, run1Bytes) {
		t.Fatalf("Run 2 journal does not extend Run 1's bytes verbatim:\nrun1: %s\nrun2: %s", run1Bytes, run2Bytes)
	}

	journalAfterRun2 := readJournal(t, specDir)
	if len(journalAfterRun2) != 10 {
		t.Fatalf("journal after Run 2 has %d lines, want 10: %+v", len(journalAfterRun2), journalAfterRun2)
	}
	foundC := false
	for _, ev := range journalAfterRun2[8:] {
		if ev.Event == "added" && ev.Node == hexC {
			foundC = true
		}
	}
	if !foundC {
		t.Error("Run 2: no added event for C")
	}

	if !wrote2 {
		t.Fatal("Run 2: Saver.Save reported wrote=false on complete status")
	}
	got2, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("Run 2: read snapshot: %v", err)
	}
	if string(got2) == string(baseline) {
		t.Fatal("Run 2: snapshot still contains baseline bytes after complete run")
	}
}

// TestPartialRunRecovery_AdapterSideDuplicate covers "Partial with
// Adapter-Side Duplicates": the dead Run 1 adapter created A, B and C in
// the tracker but died before writing receipts.json at all, so no
// receipts file exists and Run 1 never reached ingest. The discipline is
// re-run the adapter with the SAME changeset (never re-emit) — its op
// ids and labels are byte-identical to the dead run's, so the adapter's
// exact-match probe finds all three pre-existing tasks and reports
// was_existing=true "for C, and for A and B alike". The reconciler must
// materialise all three pairings normally rather than refuse the run or
// duplicate any of them.
func TestPartialRunRecovery_AdapterSideDuplicate(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	graph := newFakeSpecGraph()
	for _, id := range []string{hexA, hexB, hexC} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}

	// This scenario's own dead Run 1 emitted only these three creates (it
	// never touched br-old, unlike TwoRunSequence's four-op Run 1); this
	// is a re-run of the adapter with that identical changeset, not a
	// fresh plan run, so op ids and labels match exactly.
	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "run1head", Proposal: "p1", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:run1head:op-1")},
		{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:run1head:op-2")},
		{OpID: "op-3", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexC, Idempotency: idem("spex:run1head:op-3")},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA", WasExisting: true},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB", WasExisting: true},
		{OpID: "op-3", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: true},
	}}

	_, wrote := runWithSnapshot(t, specDir, graph, "", snapPath, cs, rc)

	journal := readJournal(t, specDir)
	if len(journal) != 6 {
		t.Fatalf("journal has %d lines, want 6: %+v", len(journal), journal)
	}
	if journal[0].Event != "added" || journal[0].Node != hexA {
		t.Errorf("line 1 = %+v, want added event for A", journal[0])
	}
	if journal[1].Event != "task_created" || journal[1].TaskID != "brA" {
		t.Errorf("line 2 = %+v, want task_created for brA", journal[1])
	}
	if journal[2].Event != "added" || journal[2].Node != hexB {
		t.Errorf("line 3 = %+v, want added event for B", journal[2])
	}
	if journal[3].Event != "task_created" || journal[3].TaskID != "brB" {
		t.Errorf("line 4 = %+v, want task_created for brB", journal[3])
	}
	if journal[4].Event != "added" || journal[4].Node != hexC {
		t.Errorf("line 5 = %+v, want added event for C", journal[4])
	}
	if journal[5].Event != "task_created" || journal[5].TaskID != "brC" {
		t.Errorf("line 6 = %+v, want task_created for brC", journal[5])
	}
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}
}

// TestPartialRunRecovery_SnapshotMatchesIndependentMerkle covers "Snapshot
// Correctness": after a complete run, the persisted snapshot's root_hash
// equals an independently-computed merkle.BuildTree value over the same
// spec directory.
func TestPartialRunRecovery_SnapshotMatchesIndependentMerkle(t *testing.T) {
	specDir := setupSpecDir(t)
	ctx := resolvedProjectContext(t, specDir)

	graph := newFakeSpecGraph()
	graph.nodes[hexC] = NodeMetadata{Module: "m", Component: "C", ContentFile: "C.md", SpecHash: "hC", NodeType: "component"}

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexC, Idempotency: idem("spex:g:op-1")},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC"},
	}}

	_, wrote := runWithSnapshot(t, specDir, graph, ctx.JournalPath, ctx.SnapshotPath, cs, rc)
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}

	want, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("independent BuildTree: %v", err)
	}
	loaded, err := merkle.Load(ctx.SnapshotPath)
	if err != nil {
		t.Fatalf("load saved snapshot: %v", err)
	}
	if loaded.Hash != want.Hash {
		t.Errorf("snapshot root_hash = %q, want %q (independent BuildTree)", loaded.Hash, want.Hash)
	}
}
