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

// Tests for spec/ingest/test_consistency_invariants.md scenarios that
// require Reconciler.Apply and SnapshotSaver.Save to run together. The
// per-component tests in reconciler_test.go and snapshot_saver_test.go
// cover invariants 1–5, 7 and the snapshot gate in isolation. The spec's
// happy-path scenario and the invariant 6 scenarios both claim
// ".bead-map.json AND snapshot" updated/untouched in lock-step — that
// integrated property only emerges when both components run against the
// same fixture, which is what these tests do.

// runWithSnapshot drives Reconciler.Apply followed by SnapshotSaver.Save
// against shared on-disk state, mirroring the order IngestCommand wires.
// The helper centralises the integration so the scenario tests stay a
// flat read of inputs → assertions.
func runWithSnapshot(t *testing.T, store mapping.Store, graph SpecGraph, specDir, snapPath string, cs emit.Changeset, rc adapters.Receipts) (ReconcileSummary, bool) {
	t.Helper()
	r := &Reconciler{MappingStore: store, SpecGraph: graph}
	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Reconciler.Apply: %v", err)
	}
	saver := &Saver{SpecDir: specDir, SnapshotPath: snapPath}
	wrote, err := saver.Save(rc.Status)
	if err != nil {
		t.Fatalf("Saver.Save: %v", err)
	}
	return sum, wrote
}

// TestConsistencyInvariants_HappyPath_BeadMapAndSnapshotBothUpdated
// covers the spec's "Happy Path" acceptance from
// test_consistency_invariants.md: a complete-status run with a realistic
// mix (fresh creates, modified pairs, removed close) lands updates in
// both .bead-map.json and spec/.snapshot.json, and the bead-map passes
// schema validation (invariant 7) on commit. The component-isolated
// TestInvariant_HappyPath in reconciler_test.go cannot prove the
// "snapshot updated" half of the spec acceptance — only the integrated
// run proves Reconciler.Apply's commit and SnapshotSaver.Save's atomic
// rename both succeed against the same fixture state.
func TestConsistencyInvariants_HappyPath_BeadMapAndSnapshotBothUpdated(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B", "C", "Mod1", "Mod2"} {
		graph.nodes[id] = NodeMetadata{
			Module: "m", Component: id, ContentFile: id + ".md",
			SpecHash: "h" + id, NodeType: "component",
		}
	}

	initial := []mapping.Record{
		{ID: 10, SpecNodeID: "Mod1", BeadID: "br-old1", BeadType: "feature", Module: "m", Component: "Mod1", ContentFile: "Mod1.md", SpecHash: "old1"},
		{ID: 11, SpecNodeID: "Mod2", BeadID: "br-old2", BeadType: "feature", Module: "m", Component: "Mod2", ContentFile: "Mod2.md", SpecHash: "old2"},
		{ID: 12, SpecNodeID: "Gone", BeadID: "br-gone", BeadType: "feature", Module: "m", Component: "Gone", ContentFile: "Gone.md", SpecHash: "hg"},
	}
	store, mapPath := newTestStore(t, initial, 13)

	// 5 ok creates (3 fresh A/B/C + 2 modified-pair re-creates Mod1/Mod2),
	// 3 ok closes (2 modified, 1 removed). Matches the spec's "5 ok
	// creates, 3 ok closes (2 modified, 1 removed)" shape.
	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-01", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old1"}, Reason: "Spec node modified: Mod1"},
		{OpID: "op-02", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "Mod1", Idempotency: idem("spex:10")},
		{OpID: "op-03", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old2"}, Reason: "Spec node modified: Mod2"},
		{OpID: "op-04", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "Mod2", Idempotency: idem("spex:11")},
		{OpID: "op-05", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-gone"}, Reason: "Spec node removed: Gone"},
		{OpID: "op-06", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:13")},
		{OpID: "op-07", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:14")},
		{OpID: "op-08", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "C", Idempotency: idem("spex:15")},
	}}
	delete(graph.nodes, "Gone") // post-removed: must not resolve in invariant 4

	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-01", Status: adapters.OpStatusOk, BeadID: "br-old1"},
		{OpID: "op-02", Status: adapters.OpStatusOk, BeadID: "br-new1"},
		{OpID: "op-03", Status: adapters.OpStatusOk, BeadID: "br-old2"},
		{OpID: "op-04", Status: adapters.OpStatusOk, BeadID: "br-new2"},
		{OpID: "op-05", Status: adapters.OpStatusOk, BeadID: "br-gone"},
		{OpID: "op-06", Status: adapters.OpStatusOk, BeadID: "br-A"},
		{OpID: "op-07", Status: adapters.OpStatusOk, BeadID: "br-B"},
		{OpID: "op-08", Status: adapters.OpStatusOk, BeadID: "br-C"},
	}}

	sum, wrote := runWithSnapshot(t, store, graph, specDir, snapPath, cs, rc)

	if sum.OkCreates != 5 || sum.OkCloses != 3 {
		t.Errorf("summary = %+v, want 5 ok creates / 3 ok closes", sum)
	}
	if sum.RecordsAdded != 3 || sum.RecordsUpdated != 2 || sum.RecordsDeleted != 1 {
		t.Errorf("summary = %+v, want 3 add / 2 upd / 1 del", sum)
	}

	// Bead-map updates landed: Mod1/Mod2 rebound to new bead_ids; A/B/C
	// inserted; Gone removed.
	for id, wantBead := range map[int]string{10: "br-new1", 11: "br-new2", 13: "br-A", 14: "br-B", 15: "br-C"} {
		rec, err := store.Get(id)
		if err != nil {
			t.Errorf("Get(%d): %v", id, err)
			continue
		}
		if rec.BeadID != wantBead {
			t.Errorf("record %d bead_id = %q, want %q", id, rec.BeadID, wantBead)
		}
	}
	if _, err := store.Get(12); err == nil {
		t.Error("removed record 12 still present after happy path")
	}

	// Bead-map on disk is schema-valid (invariant 7): re-read via a fresh
	// store. fileStore.NewFileStore + Replace already validates on write,
	// but List on a fresh handle proves the persisted bytes round-trip.
	fresh := mapping.NewFileStore(mapPath)
	if _, err := fresh.List(); err != nil {
		t.Errorf("on-disk .bead-map.json failed to round-trip: %v", err)
	}

	// Snapshot was written (invariant 6 complete branch).
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}
	snapData, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snap merkle.Snapshot
	if err := json.Unmarshal(snapData, &snap); err != nil {
		t.Fatalf("snapshot not valid JSON: %v", err)
	}
	if snap.RootHash == "" {
		t.Fatalf("snapshot has empty root_hash: %s", snapData)
	}
}

// TestConsistencyInvariants_PartialRun_SnapshotUntouched covers
// invariant 6's partial branch from test_consistency_invariants.md:
// when receipts top-level status is partial, Reconciler.Apply still
// commits the ok ops to .bead-map.json, but SnapshotSaver.Save MUST
// leave spec/.snapshot.json byte-for-byte unchanged. This is the
// "unfinished operations resurface" property: the next emit must diff
// against the pre-run baseline, not a partial state.
func TestConsistencyInvariants_PartialRun_SnapshotUntouched(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	// Seed a recognisable baseline so we can byte-compare after Save.
	baseline := []byte(`{"baseline":true,"root_hash":"deadbeef"}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	graph := newFakeSpecGraph()
	for _, id := range []string{"A", "B"} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "h" + id, NodeType: "component"}
	}
	store, _ := newTestStore(t, nil, 100)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:100")},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "B", Idempotency: idem("spex:101")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
		{OpID: "op-2", Status: adapters.OpStatusError, Error: "tracker boom"},
	}}

	_, wrote := runWithSnapshot(t, store, graph, specDir, snapPath, cs, rc)

	// Reconciler still committed the ok op to bead-map (partial state is
	// internally consistent).
	rec, err := store.Get(100)
	if err != nil || rec.BeadID != "br-A" {
		t.Errorf("ok op commit: rec=%+v err=%v, want bead br-A", rec, err)
	}

	// Saver gate fired: wrote=false and snapshot bytes unchanged.
	if wrote {
		t.Error("Saver.Save reported wrote=true on partial status")
	}
	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) != string(baseline) {
		t.Fatalf("snapshot mutated on partial run:\n got %s\nwant %s", got, baseline)
	}
}

// TestConsistencyInvariants_CompleteRun_OverwritesPriorSnapshot covers
// the second half of invariant 6 from test_consistency_invariants.md:
// a complete-status run replaces any pre-existing snapshot with a fresh
// tree. The pre-existing baseline must be gone after Save returns —
// otherwise the next emit would diff against stale bytes.
func TestConsistencyInvariants_CompleteRun_OverwritesPriorSnapshot(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	stale := []byte(`{"stale":true,"root_hash":"00000000"}`)
	if err := os.WriteFile(snapPath, stale, 0644); err != nil {
		t.Fatalf("seed stale snapshot: %v", err)
	}

	graph := newFakeSpecGraph()
	graph.nodes["A"] = NodeMetadata{Module: "m", Component: "A", ContentFile: "A.md", SpecHash: "hA", NodeType: "component"}
	store, _ := newTestStore(t, nil, 1)

	cs := emit.Changeset{Version: 1, Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: "A", Idempotency: idem("spex:1")},
	}}
	rc := adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
	}}

	_, wrote := runWithSnapshot(t, store, graph, specDir, snapPath, cs, rc)

	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}
	got, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if string(got) == string(stale) {
		t.Fatal("snapshot still contains stale bytes after complete run")
	}
	var snap merkle.Snapshot
	if err := json.Unmarshal(got, &snap); err != nil {
		t.Fatalf("snapshot not valid JSON: %v", err)
	}
	if snap.RootHash == "" || snap.RootHash == "00000000" {
		t.Errorf("snapshot root_hash = %q, want fresh non-stale value", snap.RootHash)
	}
}
