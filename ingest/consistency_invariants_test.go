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

// Tests for spec/ingest/test_consistency_invariants.md. Invariants 1, 2,
// 3 and 5 — the ones Reconciler itself asserts — are exercised in
// isolation in reconciler_test.go (TestCheckInvariant1_*,
// TestCheckInvariant2_*, TestApply_Idempotent_RerunAppendsNothing,
// TestCheckInvariant5_*) and the "lineage replaces the rebind invariant"
// property is proven by TestApply_ModifiedPair_LineageExtendedNotRebound.
// What only emerges when Reconciler.Apply and SnapshotSaver.Save run
// together against shared on-disk state is invariant 4 (snapshot saved
// iff complete) and the spec's Happy Path acceptance — that is what the
// tests below cover. "One snapshot format across both writers" lives in
// snapshot_format_test.go.

// runWithSnapshot drives Reconciler.Apply followed by SnapshotSaver.Save
// against shared on-disk state, mirroring the order IngestCommand wires.
func runWithSnapshot(t *testing.T, specDir string, graph SpecGraph, snapPath string, cs emit.Changeset, rc adapters.Receipts) (ReconcileSummary, bool) {
	t.Helper()
	r := &Reconciler{SpecDir: specDir, SpecGraph: graph}
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

// TestConsistencyInvariants_HappyPath covers the spec's "Happy Path"
// acceptance: a full complete run with 5 ok creates and 3 ok closes — 2
// of the creates paired with 2 of the closes as modify pairs, the third
// close a removal. All invariants pass; the journal gains exactly the
// expected events and receipts, the snapshot is rewritten, and every
// appended line validates against the journal-line schema (proven by
// Reconciler.Apply not erroring, since invariant 5 is checked before any
// write).
func TestConsistencyInvariants_HappyPath(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	graph := newFakeSpecGraph()
	for _, id := range []string{hexMod1, hexMod2, hexA, hexB, hexC} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "new-" + id, NodeType: "component"}
	}

	seedJournal(t, specDir,
		mapping.Event{Event: "added", EID: "seed-Mod1", Node: hexMod1, Name: "Mod1", NodeType: "component", Module: "m", After: strPtr("old-Mod1"), GitHead: "seedhead", Proposal: "seed-p", Path: "Mod1.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old1", For: "seed-Mod1"},
		mapping.Event{Event: "added", EID: "seed-Mod2", Node: hexMod2, Name: "Mod2", NodeType: "component", Module: "m", After: strPtr("old-Mod2"), GitHead: "seedhead", Proposal: "seed-p", Path: "Mod2.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old2", For: "seed-Mod2"},
		mapping.Event{Event: "added", EID: "seed-Gone", Node: hexGone, Name: "Gone", NodeType: "component", Module: "m", After: strPtr("h-gone"), GitHead: "seedhead", Proposal: "seed-p", Path: "Gone.md"},
		mapping.Event{Event: "task_created", TaskID: "br-gone", For: "seed-Gone"},
	)

	cs := emit.Changeset{Version: emit.ChangesetVersion, GitHead: "cafehappy", Proposal: "happy-p", Ops: []emit.Op{
		{OpID: "op-01", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old1"}, Reason: "Spec node modified: m/Mod1"},
		{OpID: "op-02", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexMod1, Idempotency: idem("spex:" + hexMod1), Deps: []emit.Ref{{Kind: emit.RefBead, BeadID: "br-old1", EdgeType: "blocks"}}},
		{OpID: "op-03", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old2"}, Reason: "Spec node modified: m/Mod2"},
		{OpID: "op-04", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexMod2, Idempotency: idem("spex:" + hexMod2), Deps: []emit.Ref{{Kind: emit.RefBead, BeadID: "br-old2", EdgeType: "blocks"}}},
		{OpID: "op-05", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-gone"}, Reason: "Spec node removed: m/Gone"},
		{OpID: "op-06", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
		{OpID: "op-07", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:" + hexB)},
		{OpID: "op-08", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexC, Idempotency: idem("spex:" + hexC)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-01", Status: adapters.OpStatusOk, BeadID: "br-old1"},
		{OpID: "op-02", Status: adapters.OpStatusOk, BeadID: "br-new1"},
		{OpID: "op-03", Status: adapters.OpStatusOk, BeadID: "br-old2"},
		{OpID: "op-04", Status: adapters.OpStatusOk, BeadID: "br-new2"},
		{OpID: "op-05", Status: adapters.OpStatusOk, BeadID: "br-gone"},
		{OpID: "op-06", Status: adapters.OpStatusOk, BeadID: "br-A"},
		{OpID: "op-07", Status: adapters.OpStatusOk, BeadID: "br-B"},
		{OpID: "op-08", Status: adapters.OpStatusOk, BeadID: "br-C"},
	}}

	sum, wrote := runWithSnapshot(t, specDir, graph, snapPath, cs, rc)

	if sum.OkCreates != 5 || sum.OkCloses != 3 {
		t.Errorf("summary = %+v, want 5 ok creates / 3 ok closes", sum)
	}
	// 2 modified events + 1 removed event + 2 added events (A, B, C count
	// as 3, but only A/B/C — wait: 2 modify pairs contribute 1 event each
	// (2), the removal contributes 1 event (1), the 3 fresh creates
	// contribute 1 event each (3) = 6 events.
	if sum.EventsAppended != 6 {
		t.Errorf("events_appended = %d, want 6", sum.EventsAppended)
	}
	// Receipts: 2 modify pairs × 2 receipts (closed+created) = 4, 1
	// removed close × 1 receipt (closed) = 1, 3 fresh creates × 1 receipt
	// (created) = 3. Total 8.
	if sum.ReceiptsAppended != 8 {
		t.Errorf("receipts_appended = %d, want 8", sum.ReceiptsAppended)
	}

	fold, err := mapping.NewMappingStore(specDir).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byKey := map[string]mapping.FoldEntry{}
	for _, e := range fold.Entries {
		byKey[e.Key] = e
	}
	want := map[string]string{hexMod1: "br-new1", hexMod2: "br-new2", hexA: "br-A", hexB: "br-B", hexC: "br-C"}
	for key, task := range want {
		if byKey[key].TaskID != task {
			t.Errorf("fold[%s].TaskID = %q, want %q", key, byKey[key].TaskID, task)
		}
	}
	if !byKey[hexGone].Removed {
		t.Errorf("fold[Gone] = %+v, want removed", byKey[hexGone])
	}

	// Every appended line validated against the journal-line schema — if
	// it hadn't, Apply would have returned an error above.
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

// TestConsistencyInvariants_Invariant4_PartialLeavesSnapshotUntouched
// covers invariant 4's partial branch: Reconciler.Apply still appends
// the ok op's journal lines, but SnapshotSaver.Save must leave
// spec/.snapshot.json byte-for-byte unchanged.
func TestConsistencyInvariants_Invariant4_PartialLeavesSnapshotUntouched(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	baseline := []byte(`{"baseline":true,"root_hash":"deadbeef"}`)
	if err := os.WriteFile(snapPath, baseline, 0644); err != nil {
		t.Fatalf("seed baseline: %v", err)
	}

	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "h", NodeType: "component"}

	cs := emit.Changeset{Version: emit.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:" + hexB)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
		{OpID: "op-2", Status: adapters.OpStatusError, Error: "tracker boom"},
	}}

	sum, wrote := runWithSnapshot(t, specDir, graph, snapPath, cs, rc)

	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event / 1 receipt (only the ok op)", sum)
	}
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

	journal := readJournal(t, specDir)
	if len(journal) != 2 || journal[0].Node != hexA {
		t.Errorf("journal = %+v, want the ok op's added+task_created for A only", journal)
	}
}

// TestConsistencyInvariants_Invariant4_CompleteSavesSnapshot covers
// invariant 4's complete branch: a complete-status run replaces any
// pre-existing snapshot with a fresh tree in the same run that appends
// the journal lines.
func TestConsistencyInvariants_Invariant4_CompleteSavesSnapshot(t *testing.T) {
	specDir := setupSpecDir(t)
	snapPath := filepath.Join(specDir, ".snapshot.json")

	stale := []byte(`{"stale":true,"root_hash":"00000000"}`)
	if err := os.WriteFile(snapPath, stale, 0644); err != nil {
		t.Fatalf("seed stale snapshot: %v", err)
	}

	graph := newFakeSpecGraph()
	graph.nodes[hexA] = NodeMetadata{Module: "m", Component: "A", ContentFile: "a.md", SpecHash: "h", NodeType: "component"}

	cs := emit.Changeset{Version: emit.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-A"},
	}}

	_, wrote := runWithSnapshot(t, specDir, graph, snapPath, cs, rc)

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

// TestConsistencyInvariants_LineageReplacesRebind covers "Lineage
// replaces the rebind invariant": after a modified-node close+create pair
// runs, the journal holds BOTH pairings — the retired task_created for
// the old bead stays present — and the fold answers with the new task
// only. No assertion demands the old line be gone; asserting its
// continued presence IS the test.
func TestConsistencyInvariants_LineageReplacesRebind(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexM] = NodeMetadata{Module: "m", Component: "M", ContentFile: "m.md", SpecHash: "new-hash", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: "E1", Node: hexM, Name: "M", NodeType: "component", Module: "m", After: strPtr("old-hash"), GitHead: "seedhead", Proposal: "seed-p", Path: "m.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "E1"},
	)

	cs := emit.Changeset{Version: emit.ChangesetVersion, GitHead: "g2", Proposal: "p2", Ops: []emit.Op{
		{OpID: "op-1", Type: emit.OpClose, Target: &emit.Ref{Kind: emit.RefBead, BeadID: "br-old"}, Reason: "Spec node modified: m/M"},
		{OpID: "op-2", Type: emit.OpCreate, SpecNodeKind: "component", SpecNodeID: hexM, Idempotency: idem("spex:" + hexM), Deps: []emit.Ref{{Kind: emit.RefBead, BeadID: "br-old", EdgeType: "blocks"}}},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, BeadID: "br-old"},
		{OpID: "op-2", Status: adapters.OpStatusOk, BeadID: "br-new"},
	}}

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	journal := readJournal(t, dir)
	oldPairingStillPresent := false
	for _, ev := range journal {
		if ev.Event == "task_created" && ev.TaskID == "br-old" {
			oldPairingStillPresent = true
		}
	}
	if !oldPairingStillPresent {
		t.Fatal("old task_created (br-old) missing — lineage must not be deleted")
	}

	fold, err := mapping.NewMappingStore(dir).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range fold.Entries {
		if e.Key == hexM && e.TaskID != "br-new" {
			t.Errorf("fold[M].TaskID = %q, want br-new", e.TaskID)
		}
	}
}
