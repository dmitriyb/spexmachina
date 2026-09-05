package ingest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
)

// Tests for spec/ingest/test_consistency_invariants.md. Invariants 1 and 2
// are exercised in isolation, against InvariantChecker directly, in
// invariant_checker_test.go (TestInvariantChecker_Invariant1_*,
// TestInvariantChecker_Invariant2_*); invariant 3 by
// TestApply_Idempotent_RerunAppendsNothing and invariant 5's schema half
// by TestCheckInvariant5_* (reconciler_test.go) and its profile half by
// TestConsistencyInvariants_Invariant5_ProfileChecksNodeType below, and
// the "old lineage is extended, never rebound" property is proven by
// TestApply_CreateOnKnownNode_ModifiedEventLineageExtended. What only emerges when
// Reconciler.Apply and SnapshotSaver.Save run together against shared
// on-disk state is invariant 4 (snapshot saved iff complete) and the
// spec's Happy Path acceptance — that is what the tests below cover. "One
// snapshot format across both writers" lives in snapshot_format_test.go.

// resolvedProjectContext seeds an empty .spex/ state directory beside the
// fixture project's own spec tree (specDir's parent, the sibling layout
// spec/ and .spex/ share under one project root) and resolves it via
// lifecycle.Resolve — the same call IngestCommand's caller makes before
// handing the Reconciler and Saver their JournalPath and SnapshotPath, per
// arch_snapshot_saver.md's Interface section ("this writer computes no
// location of its own"). Unlike a throwaway root, the returned locations
// live alongside the fixture project under test, so a write there is
// verifiable as belonging to it.
func resolvedProjectContext(t *testing.T, specDir string) *lifecycle.ProjectContext {
	t.Helper()
	projectRoot := filepath.Dir(specDir)
	stateDir := filepath.Join(projectRoot, lifecycle.StateDirName)
	if err := merkle.Save(merkle.EmptyTree(), filepath.Join(stateDir, lifecycle.SnapshotFileName), time.Now().UTC()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, lifecycle.JournalFileName), nil, 0644); err != nil {
		t.Fatalf("seed journal: %v", err)
	}
	ctx, err := lifecycle.Resolve(projectRoot)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return ctx
}

// runWithSnapshot drives Reconciler.Apply followed by SnapshotSaver.Save
// against shared on-disk state, mirroring the order IngestCommand wires.
// An empty journalPath defers to Reconciler's own <SpecDir>/.history.jsonl
// default, for scenarios that don't go through lifecycle resolution.
func runWithSnapshot(t *testing.T, specDir string, graph SpecGraph, journalPath, snapPath string, cs plan.Changeset, rc adapters.Receipts) (ReconcileSummary, bool) {
	t.Helper()
	r := &Reconciler{SpecDir: specDir, JournalPath: journalPath, SpecGraph: graph}
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
// acceptance: a full complete run with 5 ok creates (2 on known nodes
// whose earlier tasks already finished, so each builds a modified event
// with no task_closed; 1 a cleanup, minting its own removal; 2 on
// brand-new nodes) and 2 ok closes (a removal and a test_section
// fold-back). Every op contributes exactly one event and one receipt —
// there is no merging across ops. All invariants pass; the journal gains
// exactly the expected events and receipts, the snapshot is rewritten,
// and every appended line validates against the journal-line schema
// (proven by Reconciler.Apply not erroring, since invariant 5 is checked
// before any write).
func TestConsistencyInvariants_HappyPath(t *testing.T) {
	const hexSec1 = "dededededede"
	const hexCleanup = "efefefefefef"
	specDir := setupSpecDir(t)
	ctx := resolvedProjectContext(t, specDir)

	graph := newFakeSpecGraph()
	for _, id := range []string{hexMod1, hexMod2, hexA, hexB} {
		graph.nodes[id] = NodeMetadata{Module: "m", Component: id, ContentFile: id + ".md", SpecHash: "new-" + id, NodeType: "component"}
	}
	graph.nodes[hexSec1] = NodeMetadata{Module: "m", Component: "Sec1", ContentFile: "Sec1.md", SpecHash: "new-Sec1", NodeType: "test_section"}

	if err := mapping.NewMappingStore(ctx.JournalPath).Append([]mapping.Event{
		{Event: "added", EID: "seed-Mod1", Node: hexMod1, Name: "Mod1", NodeType: "component", Module: "m", After: strPtr("old-Mod1"), GitHead: "seedhead", Proposal: "seed-p", Path: "Mod1.md"},
		{Event: "task_created", TaskID: "br-old1", For: "seed-Mod1"},
		{Event: "added", EID: "seed-Mod2", Node: hexMod2, Name: "Mod2", NodeType: "component", Module: "m", After: strPtr("old-Mod2"), GitHead: "seedhead", Proposal: "seed-p", Path: "Mod2.md"},
		{Event: "task_created", TaskID: "br-old2", For: "seed-Mod2"},
		{Event: "added", EID: "seed-Gone", Node: hexGone, Name: "Gone", NodeType: "component", Module: "m", After: strPtr("h-gone"), GitHead: "seedhead", Proposal: "seed-p", Path: "Gone.md"},
		{Event: "task_created", TaskID: "br-gone", For: "seed-Gone"},
		{Event: "added", EID: "seed-Sec1", Node: hexSec1, Name: "Sec1", NodeType: "test_section", Module: "m", After: strPtr("old-Sec1"), GitHead: "seedhead", Proposal: "seed-p", Path: "Sec1.md"},
		{Event: "task_created", TaskID: "br-sec1", For: "seed-Sec1"},
		{Event: "added", EID: "seed-Cleanup", Node: hexCleanup, Name: "Cleaned", NodeType: "component", Module: "m", After: strPtr("h-cleaned"), GitHead: "seedhead", Proposal: "seed-p", Path: "Cleaned.md"},
		{Event: "task_created", TaskID: "br-cleaned", For: "seed-Cleanup"},
	}); err != nil {
		t.Fatalf("seed journal: %v", err)
	}

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "cafehappy", Proposal: "happy-p", Ops: []plan.Op{
		{OpID: "op-01", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexMod1, Idempotency: idem("spex:" + hexMod1)},
		{OpID: "op-02", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexMod2, Idempotency: idem("spex:" + hexMod2)},
		{OpID: "op-03", Type: plan.OpCreate, SpecNodeKind: plan.KindCleanup, SpecNodeID: hexCleanup, Idempotency: idem("spex:cafehappy:op-03"), Labels: []string{"spex:cleanup"}},
		{OpID: "op-04", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
		{OpID: "op-05", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:" + hexB)},
		{OpID: "op-06", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-gone"}, Reason: "Spec node removed: m/Gone"},
		{OpID: "op-07", Type: plan.OpClose, Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-sec1"}, Reason: "Spec node modified: m/Sec1"},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-01", Status: adapters.OpStatusOk, TaskID: "br-new1"},
		{OpID: "op-02", Status: adapters.OpStatusOk, TaskID: "br-new2"},
		{OpID: "op-03", Status: adapters.OpStatusOk, TaskID: "br-cleanup"},
		{OpID: "op-04", Status: adapters.OpStatusOk, TaskID: "br-A"},
		{OpID: "op-05", Status: adapters.OpStatusOk, TaskID: "br-B"},
		{OpID: "op-06", Status: adapters.OpStatusOk, TaskID: "br-gone"},
		{OpID: "op-07", Status: adapters.OpStatusOk, TaskID: "br-sec1"},
	}}

	sum, wrote := runWithSnapshot(t, specDir, graph, ctx.JournalPath, ctx.SnapshotPath, cs, rc)

	if sum.OkCreates != 5 || sum.OkCloses != 2 {
		t.Errorf("summary = %+v, want 5 ok creates / 2 ok closes", sum)
	}
	// Every one of the 7 ops contributes exactly one event: 2 modified
	// (known nodes), 1 removed (the cleanup's self-minted removal), 2
	// added (fresh), 1 removed (the removal close), 1 modified (the
	// fold-back close).
	if sum.EventsAppended != 7 {
		t.Errorf("events_appended = %d, want 7", sum.EventsAppended)
	}
	// Every one of the 7 ops contributes exactly one receipt: 5
	// task_created (the 5 creates), 2 task_closed (the 2 closes).
	if sum.ReceiptsAppended != 7 {
		t.Errorf("receipts_appended = %d, want 7", sum.ReceiptsAppended)
	}

	fold, err := mapping.NewMappingStore(ctx.JournalPath).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byKey := map[string]mapping.FoldEntry{}
	for _, e := range fold.Entries {
		byKey[e.Key] = e
	}
	want := map[string]string{hexMod1: "br-new1", hexMod2: "br-new2", hexA: "br-A", hexB: "br-B"}
	for key, task := range want {
		if byKey[key].TaskID != task {
			t.Errorf("fold[%s].TaskID = %q, want %q", key, byKey[key].TaskID, task)
		}
	}
	if byKey[hexCleanup].TaskID != "br-cleanup" || !byKey[hexCleanup].Removed {
		t.Errorf("fold[Cleanup] = %+v, want task br-cleanup, removed", byKey[hexCleanup])
	}
	if !byKey[hexGone].Removed {
		t.Errorf("fold[Gone] = %+v, want removed", byKey[hexGone])
	}

	// Every appended line validated against the journal-line schema — if
	// it hadn't, Apply would have returned an error above.
	if !wrote {
		t.Fatal("Saver.Save reported wrote=false on complete status")
	}
	snapData, err := os.ReadFile(ctx.SnapshotPath)
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

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
		{OpID: "op-2", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexB, Idempotency: idem("spex:" + hexB)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusPartial, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-A"},
		{OpID: "op-2", Status: adapters.OpStatusError, Error: "tracker boom"},
	}}

	sum, wrote := runWithSnapshot(t, specDir, graph, "", snapPath, cs, rc)

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

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexA, Idempotency: idem("spex:" + hexA)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-A"},
	}}

	_, wrote := runWithSnapshot(t, specDir, graph, "", snapPath, cs, rc)

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
// replaces the rebind invariant": a plain create for a node whose earlier
// task is finished — no close beside it — leaves the journal holding
// BOTH pairings — the retired task_created for the old bead stays
// present, with no task_closed after it — and the fold answers with the
// new task only. No assertion demands the old line be gone or closed;
// asserting its continued, unclosed presence IS the test.
func TestConsistencyInvariants_LineageReplacesRebind(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexM] = NodeMetadata{Module: "m", Component: "M", ContentFile: "m.md", SpecHash: "new-hash", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{Event: "added", EID: "E1", Node: hexM, Name: "M", NodeType: "component", Module: "m", After: strPtr("old-hash"), GitHead: "seedhead", Proposal: "seed-p", Path: "m.md"},
		mapping.Event{Event: "task_created", TaskID: "br-old", For: "E1"},
	)

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g2", Proposal: "p2", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "component", SpecNodeID: hexM, Idempotency: idem("spex:" + hexM), Deps: []plan.Ref{{Kind: plan.RefTask, TaskID: "br-old"}}},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-new"},
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
		if ev.Event == "task_closed" {
			t.Errorf("journal = %+v, want no task_closed anywhere — a plain create runs with no close beside it", journal)
		}
	}
	if !oldPairingStillPresent {
		t.Fatal("old task_created (br-old) missing — lineage must not be deleted")
	}

	fold, err := mapping.NewMappingStore(filepath.Join(dir, ".history.jsonl")).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, e := range fold.Entries {
		if e.Key == hexM && e.TaskID != "br-new" {
			t.Errorf("fold[M].TaskID = %q, want br-new", e.TaskID)
		}
	}
}

// TestConsistencyInvariants_Invariant5_EncoderRefusesAtOwnBoundary covers
// "Invariant 5: the encoder refuses at its own boundary": handing
// JournalEncoder a deliberately schema-invalid event directly — no
// changeset, no reconciliation run around it — refuses the line naming
// the violated constraint before any write path is reached. This
// exercises invariant 5 against the component that owns it rather than
// only through the integrated Reconciler.Apply run above, so a future
// caller of the encoder inherits the gate rather than re-implementing it.
func TestConsistencyInvariants_Invariant5_EncoderRefusesAtOwnBoundary(t *testing.T) {
	invalid := mapping.Event{
		Event: "added", EID: "e1", Node: "", Name: "x", NodeType: "component",
		Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}
	err := NewJournalEncoder().Validate(invalid)
	if err == nil {
		t.Fatal("Validate: want error for schema-invalid line, got nil")
	}
	if !strings.Contains(err.Error(), "node") {
		t.Errorf("Validate error = %v, want it to name the violated constraint (node)", err)
	}
}

// TestConsistencyInvariants_Invariant5_ProfileChecksNodeType covers the
// second half of "Invariant 5: schema-invalid line refused": a change
// event whose node_type names a kind the resolved profile does not
// declare passes the journal-line schema — which fixes only the field's
// shape — but is refused by the encoder's profile check, with the error
// naming the kind; the identical line is appended once the resolved
// profile declares it. Driven through Reconciler.Apply against a fixture
// spec dir with no profile.json yet for the first call, then a
// profile.json declaring "endpoint" written before the second (the
// pattern refresh_test.go uses for
// TestREQ_e68653819f38_Refresh_ProfileDeclaredTypeRefusedBothDirections),
// so it is the second assertion that pins reconciler.go's own
// schema.ResolveProfile(r.SpecDir) call rather than only checkInvariant5
// in isolation: a reconciler.go that swapped that call for a hardcoded
// schema.DefaultProfile() would still correctly refuse the first Apply —
// DefaultProfile declares no "endpoint" either — but would wrongly keep
// refusing the second Apply once profile.json declares "endpoint",
// instead of appending.
//
// The kind under test is "endpoint", per spec/ingest/test_consistency_invariants.md:92-96:
// node_type "endpoint" under the default profile (which declares no such
// type), refused; then rerun under a profile declaring "endpoint",
// appended. "endpoint" is not one of MappingStore's built-in kinds — bead
// spexmachina-swvx.9 removed MappingStore's node_type enum in favor of the
// journal-line schema's `^[a-z][a-z0-9_]*$` shape check, so no kind is
// more or less round-trippable through MappingStore.Append than another;
// this scenario now exists solely to pin the profile-gate transition, not
// MappingStore's write path.
func TestConsistencyInvariants_Invariant5_ProfileChecksNodeType(t *testing.T) {
	const hexEndpoint = "ddeeddeeddee"
	specDir := setupSpecDir(t)

	graph := newFakeSpecGraph()
	graph.nodes[hexEndpoint] = NodeMetadata{Module: "m", Component: "Widget Endpoint", NodeType: "endpoint"}

	cs := plan.Changeset{Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p", Ops: []plan.Op{
		{OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "endpoint", SpecNodeID: hexEndpoint, Idempotency: idem("spex:" + hexEndpoint)},
	}}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{
		{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-endpoint"},
	}}

	r := &Reconciler{SpecDir: specDir, SpecGraph: graph}

	// No profile.json yet: resolves to schema.DefaultProfile(), which
	// declares no "endpoint" type.
	before := journalBytes(t, specDir)
	if _, err := r.Apply(cs, rc); err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("Apply under the default profile: got %v, want error naming %q", err, "endpoint")
	}
	if after := journalBytes(t, specDir); !bytes.Equal(before, after) {
		t.Fatalf("journal mutated by a refused profile check: before %q after %q", before, after)
	}

	// A profile.json declaring "endpoint" resolves to a different answer
	// for the same specDir via the same ResolveProfile call.
	writeFile(t, specDir, "profile.json", `{
		"node_types": [
			{"name": "endpoint", "plural_key": "endpoints", "scope": "module", "requires_content": true}
		]
	}`)

	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("Apply once profile.json declares endpoint: unexpected error %v", err)
	}

	journal := readJournal(t, specDir)
	appended := false
	for _, ev := range journal {
		if ev.Event == "added" && ev.Node == hexEndpoint && ev.NodeType == "endpoint" {
			appended = true
		}
	}
	if !appended {
		t.Fatalf("journal = %+v, want the endpoint node's added event appended once the profile declares it", journal)
	}
}

// TestConsistencyInvariants_Invariant1_RetargetPairing covers "Invariant
// 1: retarget pairing": a clean ok retarget appends exactly the modified
// event plus its task_retargeted — no task_closed, no task_created — and
// a hand-built dangling variant (a task_retargeted whose for names an eid
// absent from journal and batch alike) is refused before anything is
// written.
func TestConsistencyInvariants_Invariant1_RetargetPairing(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexR] = NodeMetadata{Module: "m", Component: "R", ContentFile: "r.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpRetarget, SpecNodeID: hexR, SpecHash: "new-r",
			Target: &plan.Ref{Kind: plan.RefTask, TaskID: "br-open"},
			Labels: []string{"spex:g:op-1"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-open"}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event / 1 receipt", sum)
	}
	journal := readJournal(t, dir)
	if len(journal) != 2 || journal[0].Event != "modified" || journal[1].Event != "task_retargeted" {
		t.Fatalf("journal = %+v, want exactly [modified, task_retargeted]", journal)
	}

	// Invariant 3 holds unchanged: a re-run appends nothing.
	before := journalBytes(t, dir)
	if _, err := r.Apply(cs, rc); err != nil {
		t.Fatalf("re-run Apply: %v", err)
	}
	if after := journalBytes(t, dir); !bytes.Equal(before, after) {
		t.Fatalf("re-run mutated the journal:\nbefore: %s\nafter:  %s", before, after)
	}

	// The dangling variant: a task_retargeted whose for names an eid
	// neither the journal nor the batch contains.
	dangling := []mapping.Event{{Event: "task_retargeted", TaskID: "br-open", For: "no-such-eid"}}
	if err := NewInvariantChecker().Check(journal, dangling); err == nil {
		t.Fatal("InvariantChecker.Check: want error for dangling task_retargeted referent, got nil")
	}
}

// TestConsistencyInvariants_Invariant1_AbsorbedBatchClosesUnderOneRefreshReceipt
// covers "Invariant 1: absorbed batch closes under one refresh receipt":
// a clean run with two absorbed entries appends two modified events and
// exactly one refresh receipt naming both eids and nothing else; a
// hand-built refresh receipt naming an eid no absorbed event carries is
// refused before the write, per invariant 2's no-unknown-referent rule.
func TestConsistencyInvariants_Invariant1_AbsorbedBatchClosesUnderOneRefreshReceipt(t *testing.T) {
	graph := newFakeSpecGraph()
	graph.nodes[hexAbs1] = NodeMetadata{Module: "m", Component: "Abs1", ContentFile: "abs1.md", NodeType: "component"}
	graph.nodes[hexAbs2] = NodeMetadata{Module: "m", Component: "Abs2", ContentFile: "abs2.md", NodeType: "component"}
	r, dir := newTestReconciler(t, graph)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "g", Proposal: "p",
		Absorbed: []plan.AbsorbedEntry{
			{Node: hexAbs1, Before: "aaa", After: "bbb", Reason: "typo sweep"},
			{Node: hexAbs2, Before: "ccc", After: "ddd", Reason: "typo sweep"},
		},
	}
	rc := adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 2 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 2 events / 1 receipt", sum)
	}
	journal := readJournal(t, dir)
	if len(journal) != 3 {
		t.Fatalf("journal has %d lines, want 3 (2 modified + 1 refresh): %+v", len(journal), journal)
	}
	refresh := journal[2]
	if refresh.Event != "refresh" || len(refresh.Absorbed) != 2 {
		t.Fatalf("refresh receipt = %+v, want exactly 2 absorbed eids", refresh)
	}
	if refresh.Absorbed[0] != journal[0].EID || refresh.Absorbed[1] != journal[1].EID {
		t.Errorf("refresh.Absorbed = %v, want [%s, %s]", refresh.Absorbed, journal[0].EID, journal[1].EID)
	}

	// The dangling variant: a refresh receipt naming an eid no absorbed
	// event carries.
	dangling := []mapping.Event{{Event: "refresh", GitHead: "g", Absorbed: []string{journal[0].EID, "no-such-eid"}}}
	if err := NewInvariantChecker().Check(journal, dangling); err == nil {
		t.Fatal("InvariantChecker.Check: want error for dangling absorbed referent, got nil")
	}
}

// TestConsistencyInvariants_Invariant1_CleanupSelfMintedRemoval covers
// "Invariant 1: a cleanup's self-minted removal is one referent": a
// cleanup create for a node with no prior removed event mints its own
// removal from the op's own (git_head, op_id) and pairs exactly one
// task_created to it; re-running the identical changeset+receipts pair
// appends nothing, because the minted removal's eid derives from that
// same op rather than from anything the re-run could construct anew.
func TestConsistencyInvariants_Invariant1_CleanupSelfMintedRemoval(t *testing.T) {
	const hexCleanup = "eeffeeffeeff"
	graph := newFakeSpecGraph()
	r, dir := newTestReconciler(t, graph)

	seedJournal(t, dir,
		mapping.Event{
			Event: "added", EID: "E1", Node: hexCleanup, Name: "Cleaned", NodeType: "component",
			Module: "m", After: strPtr("h-cleaned"), GitHead: "seedhead", Proposal: "seed-p", Path: "m/cleaned.md",
		},
		mapping.Event{Event: "task_created", TaskID: "br-cleaned", For: "E1"},
	)

	cs := plan.Changeset{
		Version: plan.ChangesetVersion, GitHead: "cafecleanup", Proposal: "p",
		Ops: []plan.Op{{
			OpID: "op-1", Type: plan.OpCreate, SpecNodeKind: "cleanup", SpecNodeID: hexCleanup,
			Idempotency: idem("spex:cafecleanup:op-1"), Labels: []string{"spex:cleanup"},
		}},
	}
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk, TaskID: "br-cleanup", WasExisting: false}},
	}

	sum, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("summary = %+v, want 1 event (the minted removal) / 1 receipt", sum)
	}

	journal := readJournal(t, dir)
	newLines := journal[2:]
	if len(newLines) != 2 {
		t.Fatalf("got %d new lines, want 2 (removed + task_created): %+v", len(newLines), newLines)
	}
	removed, receipt := newLines[0], newLines[1]
	if removed.Event != "removed" || removed.Node != hexCleanup {
		t.Fatalf("line 1 = %+v, want removed event for %s", removed, hexCleanup)
	}
	if receipt.Event != "task_created" || receipt.TaskID != "br-cleanup" || receipt.For != removed.EID {
		t.Fatalf("line 2 = %+v, want task_created for=%s task_id=br-cleanup", receipt, removed.EID)
	}

	// Re-running the identical pair appends nothing: the removal's eid
	// derives from (git_head, op_id) — the same op — so it dedups exactly
	// like an op-born create event would.
	before := journalBytes(t, dir)
	sum2, err := r.Apply(cs, rc)
	if err != nil {
		t.Fatalf("re-run Apply: %v", err)
	}
	if sum2.EventsAppended != 0 || sum2.ReceiptsAppended != 0 {
		t.Errorf("re-run summary = %+v, want zero appends", sum2)
	}
	after := journalBytes(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatalf("re-run mutated the journal:\nbefore: %s\nafter:  %s", before, after)
	}
}
