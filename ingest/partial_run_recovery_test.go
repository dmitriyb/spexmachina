package ingest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// Tests for spec/ingest/test_partial_run_recovery.md scenarios that
// require Reconciler.Apply and SnapshotSaver.Save to run together. The
// reconciler-isolation tests in reconciler_test.go cover the per-op
// state transitions (Run 1 ok/error mix, Run 2 fresh create, adapter-side
// was_existing match). The integrated path proves the snapshot-gating
// half of the spec acceptance: that after Run 1 the on-disk snapshot is
// byte-for-byte unchanged so the next emit recomputes against the
// pre-run baseline, and that after Run 2 the snapshot's root_hash
// matches an independently-computed merkle root over the spec tree.

// TestPartialRunRecovery_TwoRunSequence_SnapshotGated drives the full
// two-run sequence from test_partial_run_recovery.md end-to-end against
// shared on-disk state. Run 1 reconciles A,B and the close, errors C,
// and leaves the snapshot untouched. Run 2 commits C and saves the
// snapshot. The asserted shape mirrors the spec's "After Run 1 / After
// Run 2" assertions (records, counter, snapshot write).
func TestPartialRunRecovery_TwoRunSequence_SnapshotGated(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	// Seed a recognisable baseline so we can byte-compare after Run 1.
	baseline := []byte(`{"baseline":true,"root_hash":"deadbeef"}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C"} {
		graph.nodes[id] = NodeMetadata{
			Module: "m", Component: id, ContentFile: id + ".md",
			SpecHash: "h" + id, NodeType: "component",
		}
	}
	// Pre-run state has one record (Old) that the close-on-removed will delete.
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 41, SpecNodeID: "Old", BeadID: "br-old", BeadType: "feature", Module: "m", Component: "Old", ContentFile: "Old.md", SpecHash: "h"},
	}, 42)
	delete(graph.nodes, "Old") // post-removed: invariant 4 must not resolve it

	// Run 1: 3 creates A,B,C and 1 close on Old. C errors. Top-level partial.
	run1CS := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:42")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:43")},
		{OpID: "op-3", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44")},
		{OpID: "op-4", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"}, Reason: "Spec node removed: Old"},
	}}
	run1RC := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brA"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "brB"},
		{OpID: "op-3", Status: adapters.OpStatusError, Error: "tracker boom"},
		{OpID: "op-4", Status: adapters.OpStatusOk, BeadID: "br-old"},
	}}

	_, wrote1 := runWithSnapshot(t, store, graph, specDir, snapPath, run1CS, run1RC)

	// After Run 1: bead-map records {42: A→brA, 43: B→brB}, no record for C,
	// removed record 41 is gone, counter at 44.
	if rec, err := store.Get(42); err != nil || rec.BeadID != "brA" {
		t.Errorf("after Run 1: record 42 = %+v err %v, want bead brA", rec, err)
	}
	if rec, err := store.Get(43); err != nil || rec.BeadID != "brB" {
		t.Errorf("after Run 1: record 43 = %+v err %v, want bead brB", rec, err)
	}
	if _, err := store.Get(44); err == nil {
		t.Error("after Run 1: error op committed record 44")
	}
	if _, err := store.Get(41); err == nil {
		t.Error("after Run 1: close-on-removed left record 41 behind")
	}
	if next, _ := store.NextRecordID(); next != 44 {
		t.Errorf("after Run 1: counter = %d, want 44", next)
	}

	// Snapshot gate fired: wrote=false and snapshot bytes unchanged.
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

	// Run 2: emit re-emits C at the same record-id (44, since the counter
	// never advanced past it in Run 1). Adapter creates fresh, status complete.
	run2CS := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44")},
	}}
	run2RC := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: false},
	}}

	_, wrote2 := runWithSnapshot(t, store, graph, specDir, snapPath, run2CS, run2RC)

	// After Run 2: record 44 inserted, counter at 45.
	if rec, err := store.Get(44); err != nil || rec.BeadID != "brC" {
		t.Errorf("after Run 2: record 44 = %+v err %v, want bead brC", rec, err)
	}
	if next, _ := store.NextRecordID(); next != 45 {
		t.Errorf("after Run 2: counter = %d, want 45", next)
	}

	// Snapshot rewritten: wrote=true and stale baseline replaced.
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
	var snap merkle.Snapshot
	if err := json.Unmarshal(got2, &snap); err != nil {
		t.Fatalf("Run 2: snapshot not valid JSON: %v", err)
	}
	if snap.RootHash == "" || snap.RootHash == "deadbeef" {
		t.Errorf("Run 2: snapshot root_hash = %q, want fresh non-stale value", snap.RootHash)
	}
}

// TestPartialRunRecovery_SnapshotMatchesIndependentMerkle covers the
// "Snapshot Correctness" assertion from test_partial_run_recovery.md:
// after Run 2 (complete), the persisted snapshot's root_hash must equal
// an independently-computed merkle.BuildTree value over the same spec
// directory. This guards against the saver caching a stale tree from a
// prior run instead of rebuilding from the current spec.
func TestPartialRunRecovery_SnapshotMatchesIndependentMerkle(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	graph := newFakeSpecGraph()
	graph.nodes["C"] = NodeMetadata{
		Module: "m", Component: "C", ContentFile: "C.md",
		SpecHash: "hC", NodeType: "component",
	}
	// Mirror the post-Run-1 starting state: A, B already reconciled,
	// counter at 44, only C left to create.
	store, _ := newTestStore(t, []mapping.Record{
		{ID: 42, SpecNodeID: "A", BeadID: "brA", BeadType: "feature", Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "hA"},
		{ID: 43, SpecNodeID: "B", BeadID: "brB", BeadType: "feature", Module: "m", Component: "B", ContentFile: "B.md", SpecHash: "hB"},
	}, 44)
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "hA", NodeType: "component"}
	graph.nodes["B"] = NodeMetadata{Module: "m", Component: "B", ContentFile: "B.md", SpecHash: "hB", NodeType: "component"}

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC"},
	}}

	_, wrote := runWithSnapshot(t, store, graph, specDir, snapPath, cs, rc)
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}

	// Independent recomputation: BuildTree against the same specDir.
	want, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("independent BuildTree: %v", err)
	}

	// Read back the saved snapshot.
	loaded, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load saved snapshot: %v", err)
	}
	if loaded.Hash != want.Hash {
		t.Errorf("snapshot root_hash = %q, want %q (independent BuildTree)", loaded.Hash, want.Hash)
	}
}

// TestPartialRunRecovery_AdapterSideDuplicate_Integrated covers the
// "Partial with Adapter-Side Duplicates" edge case from
// test_partial_run_recovery.md against the integrated reconciler+saver
// path. The Run 1 adapter died after creating C in the tracker but
// before writing the receipt; Run 2's adapter sees the existing label
// match and reports was_existing=true. The integrated run must commit
// the recovered record AND save the snapshot — otherwise C's work would
// stay invisible to the next emit.
func TestPartialRunRecovery_AdapterSideDuplicate_Integrated(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	graph := newFakeSpecGraph()
	graph.nodes["C"] = NodeMetadata{
		Module: "m", Component: "C", ContentFile: "C.md",
		SpecHash: "hC", NodeType: "component",
	}
	// Same starting state as Run 2 in the prior test, but with no local
	// record for C — the adapter has the bead, we don't.
	store, _ := newTestStore(t, nil, 44)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:44")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "brC", WasExisting: true},
	}}

	_, wrote := runWithSnapshot(t, store, graph, specDir, snapPath, cs, rc)

	// Record materialised normally despite was_existing=true.
	rec, err := store.Get(44)
	if err != nil {
		t.Fatalf("Get(44): %v", err)
	}
	if rec.BeadID != "brC" || rec.SpecNodeID != "C" {
		t.Errorf("recovered record = %+v, want bead brC / spec C", rec)
	}

	// Snapshot saved (complete status fired the gate).
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}
	loaded, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if loaded.Hash == "" {
		t.Error("snapshot root_hash is empty after adapter-duplicate recovery run")
	}
}
