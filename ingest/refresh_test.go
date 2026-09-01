package ingest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
)

// refreshFixture bundles the state a RefreshHandler test needs: a spec
// dir with two modules, a snapshot matching the initial spec state, and
// a seeded journal recording each module's component as already created
// (task_created) — the state a normal-mode run would have left behind.
type refreshFixture struct {
	specDir  string
	snapPath string
	// widgetID/handlerID are component spec_node_ids with journal
	// entries and open tasks; testID is a record-less test_section leaf
	// and flowID a record-less data_flow leaf.
	widgetID  string
	handlerID string
	testID    string
	flowID    string
}

var refreshClock = func() time.Time {
	return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
}

// setupRefreshFixture writes the fixture spec, snapshots it, and seeds
// the journal with an added event + open task_created per component —
// clean state; tests introduce drift by editing content files after.
func setupRefreshFixture(t *testing.T) refreshFixture {
	t.Helper()
	specDir := t.TempDir()

	fx := refreshFixture{
		specDir:   specDir,
		snapPath:  filepath.Join(specDir, ".snapshot.json"),
		widgetID:  "aabbccddee01",
		handlerID: "aabbccddee02",
		testID:    "aabbccddee03",
		flowID:    "aabbccddee04",
	}

	writeFile(t, specDir, "project.json", `{
		"name": "refresh-fixture",
		"modules": [
			{"id": "aabbccddee10", "name": "alpha", "path": "alpha"},
			{"id": "aabbccddee20", "name": "beta", "path": "beta"}
		]
	}`)
	alphaDir := filepath.Join(specDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+fx.widgetID+`", "name": "Widget", "content": "arch_widget.md"}
		],
		"test_sections": [
			{"id": "`+fx.testID+`", "name": "Widget logic", "content": "test_widget_logic.md"}
		],
		"apis": [
			{"id": "aabbccddee06", "name": "spex widget list", "group": "cli"}
		]
	}`)
	writeFile(t, alphaDir, "arch_widget.md", "# Widget\n")
	writeFile(t, alphaDir, "test_widget_logic.md", "# Widget logic\n")

	betaDir := filepath.Join(specDir, "beta")
	if err := os.MkdirAll(betaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+fx.handlerID+`", "name": "Handler", "content": "arch_handler.md"}
		],
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)
	writeFile(t, betaDir, "arch_handler.md", "# Handler\n")
	writeFile(t, betaDir, "flow_handler.md", "# Handler pipeline\n")

	widgetHash, err := merkle.HashFile(filepath.Join(alphaDir, "arch_widget.md"))
	if err != nil {
		t.Fatal(err)
	}
	handlerHash, err := merkle.HashFile(filepath.Join(betaDir, "arch_handler.md"))
	if err != nil {
		t.Fatal(err)
	}

	seedJournal(t, specDir,
		mapping.Event{Event: "added", EID: "head0:op-widget", Node: fx.widgetID, Name: "Widget", NodeType: "component", Module: "aabbccddee10", After: strPtr(widgetHash), GitHead: "head0", Path: "spec/alpha/arch_widget.md"},
		mapping.Event{Event: "task_created", TaskID: "br-widget", For: "head0:op-widget"},
		mapping.Event{Event: "added", EID: "head0:op-handler", Node: fx.handlerID, Name: "Handler", NodeType: "component", Module: "aabbccddee20", After: strPtr(handlerHash), GitHead: "head0", Path: "spec/beta/arch_handler.md"},
		mapping.Event{Event: "task_created", TaskID: "br-handler", For: "head0:op-handler"},
	)

	tree := buildFixtureTree(t, specDir)
	if err := writeAtomic(fx.snapPath, tree, refreshClock()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	return fx
}

func buildFixtureTree(t *testing.T, specDir string) *merkle.Node {
	t.Helper()
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	return tree
}

func (fx refreshFixture) handler() *RefreshHandler {
	return &RefreshHandler{
		SnapshotPath: fx.snapPath,
		Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
		Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
		Now:          refreshClock,
	}
}

// closeTask appends a task_closed receipt for taskID, paired to the
// node's own added-event eid — standing in for a bead the normal
// pipeline (or an earlier partial run) already retired.
func closeTask(t *testing.T, specDir, forEID, taskID string) {
	t.Helper()
	seedJournal(t, specDir, mapping.Event{Event: "task_closed", TaskID: taskID, For: forEID})
}

// closeTask (above) and this fixture both build on seedJournal,
// readJournal and journalBytes — defined once in reconciler_test.go and
// shared across every test in this package.

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// assertSnapshotIsCurrent checks that the run rebaselined the snapshot
// onto the spec as it stands — the whole point of absorbing a
// structural change rather than refusing it.
func assertSnapshotIsCurrent(t *testing.T, fx refreshFixture) {
	t.Helper()
	assertSnapshotMatchesSpec(t, fx.specDir, fx.snapPath)
}

// assertSnapshotMatchesSpec is assertSnapshotIsCurrent over a bare
// (specDir, snapPath) pair, for fixtures that are not a refreshFixture.
func assertSnapshotMatchesSpec(t *testing.T, specDir, snapPath string) {
	t.Helper()
	want := buildFixtureTree(t, specDir)
	got, err := merkle.Load(snapPath)
	if err != nil {
		t.Fatalf("load rewritten snapshot: %v", err)
	}
	if got.Hash != want.Hash {
		t.Errorf("snapshot root: want current %s, got %s", want.Hash, got.Hash)
	}
}

// eventForNode returns the last added/modified/removed event in events
// whose Node matches key.
func eventForNode(events []mapping.Event, key string) (mapping.Event, bool) {
	var found mapping.Event
	ok := false
	for _, ev := range events {
		if (ev.Event == "added" || ev.Event == "modified" || ev.Event == "removed") && ev.Node == key {
			found, ok = ev, true
		}
	}
	return found, ok
}

// TestRefresh_ModifiedOnlyDiff_AppendsEventsAndRewritesSnapshot covers
// the headline scenario: content-only edits — to a task-owning
// component and to a record-less test_section alike — are absorbed as
// modified events, and the snapshot is rewritten to the current state.
func TestRefresh_ModifiedOnlyDiff_AppendsEventsAndRewritesSnapshot(t *testing.T) {
	fx := setupRefreshFixture(t)

	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler (revised)\n")
	writeFile(t, filepath.Join(fx.specDir, "alpha"), "test_widget_logic.md", "# Widget logic (clarified)\n")

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if summary.EventsAppended != 2 {
		t.Errorf("events_appended: want 2, got %d", summary.EventsAppended)
	}
	if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
		t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
	}

	handlerHash, err := merkle.HashFile(filepath.Join(fx.specDir, "beta", "arch_handler.md"))
	if err != nil {
		t.Fatal(err)
	}
	testHash, err := merkle.HashFile(filepath.Join(fx.specDir, "alpha", "test_widget_logic.md"))
	if err != nil {
		t.Fatal(err)
	}

	events := readJournal(t, fx.specDir)
	handlerEv, ok := eventForNode(events, fx.handlerID)
	if !ok || handlerEv.Event != "modified" || handlerEv.After == nil || *handlerEv.After != handlerHash {
		t.Errorf("handler event: want modified after=%s, got %+v", handlerHash, handlerEv)
	}
	testEv, ok := eventForNode(events, fx.testID)
	if !ok || testEv.Event != "modified" || testEv.After == nil || *testEv.After != testHash {
		t.Errorf("test_section event: want modified after=%s, got %+v", testHash, testEv)
	}

	last := events[len(events)-1]
	if last.Event != "refresh" {
		t.Fatalf("want the batch closed by a refresh receipt, got %+v", last)
	}
	wantAbsorbed := map[string]bool{handlerEv.EID: true, testEv.EID: true}
	if len(last.Absorbed) != 2 || !wantAbsorbed[last.Absorbed[0]] || !wantAbsorbed[last.Absorbed[1]] {
		t.Errorf("refresh receipt absorbed: want exactly {%s, %s}, got %v", handlerEv.EID, testEv.EID, last.Absorbed)
	}

	assertSnapshotIsCurrent(t, fx)
}

// TestREQ_e68653819f38_Refresh_RefusesAddedComponent covers the
// structural gate on the side the type filter must never open: a
// component is bead-producing, so an added one is a bead that was
// never created. Absorbing it would baseline the node into the
// snapshot and remove it from `spex diff` permanently. Both files stay
// byte-identical.
func TestREQ_e68653819f38_Refresh_RefusesAddedComponent(t *testing.T) {
	fx := setupRefreshFixture(t)
	alphaDir := filepath.Join(fx.specDir, "alpha")
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+fx.widgetID+`", "name": "Widget", "content": "arch_widget.md"},
			{"id": "aabbccddee99", "name": "NewThing", "content": "arch_new_thing.md"}
		],
		"test_sections": [
			{"id": "`+fx.testID+`", "name": "Widget logic", "content": "test_widget_logic.md"}
		]
	}`)
	writeFile(t, alphaDir, "arch_new_thing.md", "# New thing\n")

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "added_entries" {
		t.Fatalf("want RefreshRefusal added_entries, got %v", err)
	}
	if !strings.Contains(err.Error(), "aabbccddee99") {
		t.Errorf("refusal must name the added entry: %v", err)
	}
	if !strings.Contains(err.Error(), "normal pipeline") {
		t.Errorf("refusal must point at the normal pipeline: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestREQ_e68653819f38_Refresh_RefusesRemovedDataFlow covers the other
// structural gate: data_flow is not on the absorbable list, so deleting
// one refuses the run with the use-the-normal-pipeline error and no
// file changes. The Handler component stays in place so the removal is
// the only diff entry.
func TestREQ_e68653819f38_Refresh_RefusesRemovedDataFlow(t *testing.T) {
	fx := setupRefreshFixture(t)
	betaDir := filepath.Join(fx.specDir, "beta")
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+fx.handlerID+`", "name": "Handler", "content": "arch_handler.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(betaDir, "flow_handler.md")); err != nil {
		t.Fatal(err)
	}

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "removed_entries" {
		t.Fatalf("want RefreshRefusal removed_entries, got %v", err)
	}
	if !strings.Contains(err.Error(), fx.flowID) {
		t.Errorf("refusal must name the removed entry %s: %v", fx.flowID, err)
	}
	if !strings.Contains(err.Error(), "normal pipeline") {
		t.Errorf("refusal must point at the normal pipeline: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestREQ_e68653819f38_Refresh_AbsorbsAbsorbableStructuralSet covers
// the absorbable side of the type filter across every kind that owns
// one: a new requirement, an api added and another removed, and a
// component removed whose task was already closed. A test that only
// exercised the refusal side would pass against an implementation that
// refused everything.
func TestREQ_e68653819f38_Refresh_AbsorbsAbsorbableStructuralSet(t *testing.T) {
	fx := setupRefreshFixture(t)

	closeTask(t, fx.specDir, "head0:op-handler", "br-handler")

	writeFile(t, fx.specDir, "project.json", `{
		"name": "refresh-fixture",
		"modules": [
			{"id": "aabbccddee10", "name": "alpha", "path": "alpha"},
			{"id": "aabbccddee20", "name": "beta", "path": "beta"}
		],
		"requirements": [
			{"id": "aabbccddee09", "type": "functional", "title": "Fixture requirement"}
		]
	}`)
	alphaDir := filepath.Join(fx.specDir, "alpha")
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+fx.widgetID+`", "name": "Widget", "content": "arch_widget.md"}
		],
		"test_sections": [
			{"id": "`+fx.testID+`", "name": "Widget logic", "content": "test_widget_logic.md"}
		],
		"apis": [
			{"id": "aabbccddee07", "name": "spex widget create", "group": "cli"}
		]
	}`)
	betaDir := filepath.Join(fx.specDir, "beta")
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(betaDir, "arch_handler.md")); err != nil {
		t.Fatal(err)
	}

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if summary.EventsAppended != 4 {
		t.Errorf("events_appended: want 4 (requirement added, api added, api removed, component removed), got %d", summary.EventsAppended)
	}
	if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
		t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
	}

	events := readJournal(t, fx.specDir)
	reqEv, ok := eventForNode(events, "aabbccddee09")
	if !ok || reqEv.Event != "added" || reqEv.NodeType != "requirement" {
		t.Errorf("want an added requirement event, got %+v (ok=%v)", reqEv, ok)
	}
	addedAPI, ok := eventForNode(events, "aabbccddee07")
	if !ok || addedAPI.Event != "added" || addedAPI.NodeType != "api" {
		t.Errorf("want an added api event, got %+v (ok=%v)", addedAPI, ok)
	}
	removedAPI, ok := eventForNode(events, "aabbccddee06")
	if !ok || removedAPI.Event != "removed" || removedAPI.NodeType != "api" {
		t.Errorf("want a removed api event, got %+v (ok=%v)", removedAPI, ok)
	}
	removedComp, ok := eventForNode(events, fx.handlerID)
	if !ok || removedComp.Event != "removed" || removedComp.NodeType != "component" || removedComp.Name != "Handler" {
		t.Errorf("want a removed component event named Handler, got %+v (ok=%v)", removedComp, ok)
	}

	assertSnapshotIsCurrent(t, fx)
}

// TestREQ_e68653819f38_Refresh_RefusesRemovedComponentWithLiveTaskPairing
// pins the boundary that makes absorbing component removals safe: the
// type filter admits the removal, and the live-pairing gate then
// refuses it because the node's task is still open — no task_closed
// answers br-handler anywhere in the journal.
func TestREQ_e68653819f38_Refresh_RefusesRemovedComponentWithLiveTaskPairing(t *testing.T) {
	fx := setupRefreshFixture(t)
	betaDir := filepath.Join(fx.specDir, "beta")
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(betaDir, "arch_handler.md")); err != nil {
		t.Fatal(err)
	}

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "live_task_pairing" {
		t.Fatalf("want RefreshRefusal live_task_pairing, got %v", err)
	}
	if !strings.Contains(err.Error(), fx.handlerID) || !strings.Contains(err.Error(), "br-handler") {
		t.Errorf("refusal must name the node identity hash and the live task id: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestREQ_e68653819f38_Refresh_RefusesRemovedComponentWithOpenCleanupPairing
// pins the live-pairing gate's most important case: a removed node whose
// open task is a cleanup bead, not the node's own original task. A
// cleanup create's task_created pairs to the removal event itself, so
// fold() folds it to Removed:true — a check keyed off that flag would
// short-circuit before ever looking at TaskID/closedTaskIDs, exactly the
// bug this test guards against. The scenario mirrors a partial
// normal-mode run: the removal is journaled, the original task closed,
// and a cleanup task opened against the removal's own eid, but the
// snapshot is left untouched, so the entry is still in refresh's diff.
func TestREQ_e68653819f38_Refresh_RefusesRemovedComponentWithOpenCleanupPairing(t *testing.T) {
	fx := setupRefreshFixture(t)

	seedJournal(t, fx.specDir,
		mapping.Event{Event: "removed", EID: "head1:op-rm", Node: fx.handlerID, Name: "Handler", NodeType: "component", Module: "aabbccddee20", Before: strPtr("deadbeefcafe"), GitHead: "head1", Path: "spec/beta/arch_handler.md"},
		mapping.Event{Event: "task_closed", TaskID: "br-handler", For: "head1:op-rm"},
		mapping.Event{Event: "task_created", TaskID: "br-cleanup", For: "head1:op-rm"},
	)

	betaDir := filepath.Join(fx.specDir, "beta")
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(betaDir, "arch_handler.md")); err != nil {
		t.Fatal(err)
	}

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "live_task_pairing" {
		t.Fatalf("want RefreshRefusal live_task_pairing, got %v", err)
	}
	if !strings.Contains(err.Error(), fx.handlerID) || !strings.Contains(err.Error(), "br-cleanup") {
		t.Errorf("refusal must name the node identity hash and the open cleanup task id: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestREQ_e68653819f38_Refresh_MetaModifiedAbsorbedWithoutEvent pins the
// meta-modified carve-out: the journal-line schema's node_type enum has
// no "meta", so a modified envelope leaf is folded into the snapshot
// rewrite without a change event. The run's refresh receipt still lands,
// with an empty absorbed list, and nothing else is appended.
func TestREQ_e68653819f38_Refresh_MetaModifiedAbsorbedWithoutEvent(t *testing.T) {
	fx := setupRefreshFixture(t)
	// Drift only the beta envelope: a description edit moves the
	// meta/<beta-hash> leaf (hashed from module.json's bytes) and no
	// node leaf.
	writeFile(t, filepath.Join(fx.specDir, "beta"), "module.json", `{
		"name": "beta",
		"description": "meta-only drift",
		"components": [
			{"id": "`+fx.handlerID+`", "name": "Handler", "content": "arch_handler.md"}
		],
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)

	journalBefore := readJournal(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("meta-only drift must absorb cleanly: %v", err)
	}
	if summary.EventsAppended != 0 {
		t.Errorf("a meta modification yields no change event, got events_appended=%d", summary.EventsAppended)
	}
	if !summary.SnapshotSaved {
		t.Error("the drift is real: the snapshot must be rewritten")
	}
	if got := readBytes(t, fx.snapPath); string(got) == string(snapBefore) {
		t.Error("snapshot bytes must move with the envelope leaf")
	}

	after := readJournal(t, fx.specDir)
	if want := len(journalBefore) + 1; len(after) != want {
		t.Fatalf("exactly one line (the refresh receipt) is appended: want %d lines, got %d", want, len(after))
	}
	receipt := after[len(after)-1]
	if receipt.Event != "refresh" {
		t.Fatalf("the appended line must be the refresh receipt, got %q", receipt.Event)
	}
	if len(receipt.Absorbed) != 0 {
		t.Errorf("no change event exists for the receipt to name: absorbed must be empty, got %v", receipt.Absorbed)
	}
}

// TestREQ_e68653819f38_Refresh_RefusesAddedModuleMeta is the direct
// guard on writing the allow-list as the complement of the classifier's
// bead-producing set: "meta" is not bead-producing, so that negation
// would silently baseline a whole new module — envelope leaf and all —
// into the snapshot. Refresh runs neither `spex validate` nor the
// completeness checker, so nothing downstream would ever surface it.
func TestREQ_e68653819f38_Refresh_RefusesAddedModuleMeta(t *testing.T) {
	fx := setupRefreshFixture(t)
	writeFile(t, fx.specDir, "project.json", `{
		"name": "refresh-fixture",
		"modules": [
			{"id": "aabbccddee10", "name": "alpha", "path": "alpha"},
			{"id": "aabbccddee20", "name": "beta", "path": "beta"},
			{"id": "aabbccddee30", "name": "gamma", "path": "gamma"}
		]
	}`)
	gammaDir := filepath.Join(fx.specDir, "gamma")
	if err := os.MkdirAll(gammaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, gammaDir, "module.json", `{"name": "gamma"}`)

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "added_entries" {
		t.Fatalf("want RefreshRefusal added_entries, got %v", err)
	}
	if !strings.Contains(err.Error(), "meta/aabbccddee30") {
		t.Errorf("refusal must name the added module envelope leaf: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestRefresh_CleanSpecIsNoOp covers the no-drift scenario: refresh on
// a spec byte-identical to the snapshot succeeds, appends nothing, and
// rewrites neither file (so git status is unperturbed).
func TestRefresh_CleanSpecIsNoOp(t *testing.T) {
	fx := setupRefreshFixture(t)

	journalBefore := journalBytes(t, fx.specDir)
	snapBefore := readBytes(t, fx.snapPath)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if summary.EventsAppended != 0 || summary.SnapshotSaved {
		t.Errorf("want zero events and no snapshot write, got %+v", summary)
	}
	if summary.Status != adapters.StatusComplete {
		t.Errorf("status: want complete, got %q", summary.Status)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical on a clean no-op")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical on a clean no-op")
	}
}

// TestRefresh_RerunIsIdempotent covers the idempotency scenario: a
// second refresh over the state the first one produced finds no more
// drift (the snapshot now matches the spec) and appends nothing.
func TestRefresh_RerunIsIdempotent(t *testing.T) {
	fx := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler v2\n")

	if _, err := fx.handler().Apply(fx.specDir); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	journalAfterFirst := journalBytes(t, fx.specDir)
	snapAfterFirst := readBytes(t, fx.snapPath)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if summary.EventsAppended != 0 || summary.SnapshotSaved {
		t.Errorf("second run: want zero events and no snapshot write, got %+v", summary)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalAfterFirst) {
		t.Error("journal must end byte-identical to the first run's state")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapAfterFirst) {
		t.Error("snapshot must end byte-identical to the first run's state")
	}
}

// TestREQ_e68653819f38_Refresh_ReAddedThenRemovedRequirementProducesChangeEvent
// covers a requirement cycling add -> remove -> re-add -> remove again.
// fold() only ever mutates a node's entry on a "removed" change event or
// a task_created; requirements never get a task_created (that's the
// whole reason they're absorbable), so a skip check keyed off the fold's
// Removed flag stays pinned at true after the first removal even once
// the node is back — the fourth step would then be dropped with zero
// change events, an "amnesiac" absorption arch_refresh.md rules out. The
// check must instead read the node's latest change event
// (lastChangeByNode), which self-corrects on the re-add.
//
// Run twice: once with a distinct title per cycle (so the derived eids
// never collide across cycles) and once with the title held constant
// (so the re-add and the second removal each derive the exact same eid
// as their first occurrence). Both must still append one change event
// per step — deriveRefreshEID colliding with an earlier occurrence is a
// reason to disambiguate the id, never a reason to drop the event (see
// TestREQ_e68653819f38_Refresh_FlappingContentDoesNotDuplicateEID for the
// one case — a Modified re-diff of an already-recorded transition —
// where dropping is correct instead).
func TestREQ_e68653819f38_Refresh_ReAddedThenRemovedRequirementProducesChangeEvent(t *testing.T) {
	projectJSON := func(title string) string {
		if title == "" {
			return `{
				"name": "refresh-fixture",
				"modules": [
					{"id": "aabbccddee10", "name": "alpha", "path": "alpha"},
					{"id": "aabbccddee20", "name": "beta", "path": "beta"}
				]
			}`
		}
		return `{
			"name": "refresh-fixture",
			"modules": [
				{"id": "aabbccddee10", "name": "alpha", "path": "alpha"},
				{"id": "aabbccddee20", "name": "beta", "path": "beta"}
			],
			"requirements": [
				{"id": "aabbccddee09", "type": "functional", "title": "` + title + `"}
			]
		}`
	}

	cases := []struct {
		name  string
		steps []struct {
			name  string
			title string // empty means the requirement is absent this step
			event string
		}
	}{
		{
			name: "varying title",
			steps: []struct {
				name  string
				title string
				event string
			}{
				{"add", "Fixture requirement", "added"},
				{"remove", "", "removed"},
				{"re-add", "Fixture requirement, returned", "added"},
				{"remove again", "", "removed"},
			},
		},
		{
			name: "constant title",
			steps: []struct {
				name  string
				title string
				event string
			}{
				{"add", "Fixture requirement", "added"},
				{"remove", "", "removed"},
				{"re-add (identical title)", "Fixture requirement", "added"},
				{"remove again", "", "removed"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := setupRefreshFixture(t)
			seenEIDs := map[string]int{}

			for _, step := range tc.steps {
				writeFile(t, fx.specDir, "project.json", projectJSON(step.title))

				summary, err := fx.handler().Apply(fx.specDir)
				if err != nil {
					t.Fatalf("%s: Apply: %v", step.name, err)
				}
				if summary.EventsAppended != 1 {
					t.Fatalf("%s: events_appended: want 1, got %d", step.name, summary.EventsAppended)
				}
				if !summary.SnapshotSaved {
					t.Errorf("%s: want snapshot_saved=true, got %+v", step.name, summary)
				}

				events := readJournal(t, fx.specDir)
				ev, ok := eventForNode(events, "aabbccddee09")
				if !ok || ev.Event != step.event || ev.NodeType != "requirement" {
					t.Fatalf("%s: want a %s requirement event, got %+v (ok=%v)", step.name, step.event, ev, ok)
				}
				seenEIDs[ev.EID]++
				if seenEIDs[ev.EID] > 1 {
					t.Errorf("%s: eid %s reused from an earlier step; every eid must be unique", step.name, ev.EID)
				}
			}
		})
	}
}

// TestREQ_e68653819f38_Refresh_FlappingContentDoesNotDuplicateEID covers
// deriveRefreshEID's collision hazard directly: a leaf that flaps
// v2 -> v1 -> v2 -> v1 -> v2 -> v1 across six refreshes makes every
// odd-numbered run's derived eid byte-identical to run 1's, and every
// even-numbered run's byte-identical to run 2's — deriveRefreshEID depends
// only on (node, before, after), and the flap only ever visits two states.
// Each of these six runs is still a real content transition relative to
// the snapshot the previous run left behind, so each must still append its
// own change event — dropping any of them (as a raw eid-seen skip would
// from run 3 onward) leaves that transition amnesiac: absorbed into the
// snapshot with no journal line and no receipt naming it. The collision is
// resolved by disambiguating the id (uniqueRefreshEID), never by skipping
// construction; only a re-diff of the exact same transition the node's
// latest journaled event already records — a partial run's stale
// snapshot, not a flap — skips.
func TestREQ_e68653819f38_Refresh_FlappingContentDoesNotDuplicateEID(t *testing.T) {
	fx := setupRefreshFixture(t)
	betaDir := filepath.Join(fx.specDir, "beta")

	// setupRefreshFixture seeds arch_handler.md with v1's content, so the
	// snapshot baseline already matches v1 before run 1.
	v1 := "# Handler\n"
	v2 := "# Handler (revised)\n"
	sequence := []string{v2, v1, v2, v1, v2, v1}

	seenEIDs := map[string]int{}
	for i, content := range sequence {
		run := i + 1
		writeFile(t, betaDir, "arch_handler.md", content)
		summary, err := fx.handler().Apply(fx.specDir)
		if err != nil {
			t.Fatalf("run%d Apply: %v", run, err)
		}
		if summary.EventsAppended != 1 {
			t.Fatalf("run%d events_appended: want 1 (a real content transition, not a re-diff of the already-journaled one), got %d", run, summary.EventsAppended)
		}
		if !summary.SnapshotSaved {
			t.Errorf("run%d: want snapshot_saved=true, got %+v", run, summary)
		}

		events := readJournal(t, fx.specDir)
		ev, ok := eventForNode(events, fx.handlerID)
		if !ok || ev.Event != "modified" {
			t.Fatalf("run%d: want the latest journal line for %s to be a modified event, got %+v (ok=%v)", run, fx.handlerID, ev, ok)
		}
		seenEIDs[ev.EID]++
		if seenEIDs[ev.EID] > 1 {
			t.Errorf("run%d: eid %s reused from an earlier run; every eid must be unique", run, ev.EID)
		}
	}

	events := readJournal(t, fx.specDir)
	modifiedCount := 0
	eidCounts := map[string]int{}
	for _, ev := range events {
		if ev.Event == "modified" && ev.Node == fx.handlerID {
			modifiedCount++
		}
		if ev.EID != "" {
			eidCounts[ev.EID]++
		}
	}
	if modifiedCount != len(sequence) {
		t.Errorf("modified events journaled across %d real content transitions: want %d, got %d", len(sequence), len(sequence), modifiedCount)
	}
	for eid, count := range eidCounts {
		if count > 1 {
			t.Errorf("eid %s appears %d times in the journal; every eid must be unique", eid, count)
		}
	}

	assertSnapshotIsCurrent(t, fx)
}

// TestREQ_e68653819f38_Refresh_RemovedReAddedRemovedComponentEIDsUnique
// covers the collision the lastChangeByNode "currently removed" skip does
// not catch: a component removed by refresh, restored verbatim by the
// normal pipeline, then removed by refresh again. deriveRefreshEID depends
// only on (node, before, after); the content is byte-identical both times,
// so the second removed event's derived eid collides with the first's. The
// intervening re-add (its own op-based eid, journaled by the normal
// pipeline, not refresh) moves the node's latest event to "added", so the
// lastChangeByNode skip does not fire and a removed event is constructed
// both times — it must get its own eid rather than duplicate the first's.
func TestREQ_e68653819f38_Refresh_RemovedReAddedRemovedComponentEIDsUnique(t *testing.T) {
	fx := setupRefreshFixture(t)
	closeTask(t, fx.specDir, "head0:op-handler", "br-handler")

	betaDir := filepath.Join(fx.specDir, "beta")
	handlerContent := "# Handler\n"

	removeHandler := func() {
		writeFile(t, betaDir, "module.json", `{
			"name": "beta",
			"data_flows": [
				{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
			]
		}`)
		if err := os.Remove(filepath.Join(betaDir, "arch_handler.md")); err != nil {
			t.Fatal(err)
		}
	}

	removeHandler()
	summary1, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("run1 Apply: %v", err)
	}
	if summary1.EventsAppended != 1 {
		t.Fatalf("run1 events_appended: want 1, got %d", summary1.EventsAppended)
	}
	removedEv1, ok := eventForNode(readJournal(t, fx.specDir), fx.handlerID)
	if !ok || removedEv1.Event != "removed" {
		t.Fatalf("run1: want a removed handler event, got %+v (ok=%v)", removedEv1, ok)
	}

	// Simulate the normal pipeline restoring the identical component:
	// added + task_created + task_closed, snapshot baselined — the state
	// thread 5's report reproduces this against.
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+fx.handlerID+`", "name": "Handler", "content": "arch_handler.md"}
		],
		"data_flows": [
			{"id": "`+fx.flowID+`", "name": "Handler pipeline", "content": "flow_handler.md"}
		]
	}`)
	writeFile(t, betaDir, "arch_handler.md", handlerContent)
	handlerHash, err := merkle.HashFile(filepath.Join(betaDir, "arch_handler.md"))
	if err != nil {
		t.Fatal(err)
	}
	seedJournal(t, fx.specDir,
		mapping.Event{Event: "added", EID: "head9:op-handler2", Node: fx.handlerID, Name: "Handler", NodeType: "component", Module: "aabbccddee20", After: strPtr(handlerHash), GitHead: "head9", Path: "spec/beta/arch_handler.md"},
		mapping.Event{Event: "task_created", TaskID: "br-handler2", For: "head9:op-handler2"},
		mapping.Event{Event: "task_closed", TaskID: "br-handler2", For: "head9:op-handler2"},
	)
	if err := writeAtomic(fx.snapPath, buildFixtureTree(t, fx.specDir), refreshClock()); err != nil {
		t.Fatalf("baseline snapshot after re-add: %v", err)
	}

	removeHandler()
	summary2, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("run2 Apply: %v", err)
	}
	if summary2.EventsAppended != 1 {
		t.Fatalf("run2 events_appended: want 1, got %d", summary2.EventsAppended)
	}

	events := readJournal(t, fx.specDir)
	removedEv2, ok := eventForNode(events, fx.handlerID)
	if !ok || removedEv2.Event != "removed" {
		t.Fatalf("run2: want a removed handler event, got %+v (ok=%v)", removedEv2, ok)
	}
	if removedEv2.EID == removedEv1.EID {
		t.Errorf("run2's removed eid must differ from run1's, got byte-identical %q", removedEv2.EID)
	}

	eidCounts := map[string]int{}
	for _, ev := range events {
		if ev.EID != "" {
			eidCounts[ev.EID]++
		}
	}
	for eid, count := range eidCounts {
		if count > 1 {
			t.Errorf("eid %s appears %d times in the journal; every eid must be unique", eid, count)
		}
	}

	assertSnapshotIsCurrent(t, fx)
}

// TestREQ_e68653819f38_Refresh_RefusesEmptyJournal covers the bootstrap
// guard: a project whose snapshot is present — spex init seeds one at
// birth — but whose journal contains zero lines has never completed a
// cycle, so refresh (which absorbs drift *between* cycles) refuses
// before ever computing the diff, regardless of whether the spec itself
// has drifted from the snapshot. The guard keys on the journal rather
// than snapshot presence for exactly this reason: file-existence alone
// can no longer stand in for "a cycle has completed".
func TestREQ_e68653819f38_Refresh_RefusesEmptyJournal(t *testing.T) {
	setup := func(t *testing.T) (specDir, snapPath string) {
		t.Helper()
		specDir = t.TempDir()
		writeFile(t, specDir, "project.json", `{
			"name": "refresh-fixture",
			"modules": [
				{"id": "aabbccddee10", "name": "alpha", "path": "alpha"}
			]
		}`)
		alphaDir := filepath.Join(specDir, "alpha")
		if err := os.MkdirAll(alphaDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, alphaDir, "module.json", `{
			"name": "alpha",
			"components": [
				{"id": "aabbccddee01", "name": "Widget", "content": "arch_widget.md"}
			]
		}`)
		writeFile(t, alphaDir, "arch_widget.md", "# Widget\n")

		snapPath = filepath.Join(specDir, ".snapshot.json")
		if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
			t.Fatalf("seed snapshot: %v", err)
		}
		return specDir, snapPath
	}

	assertRefused := func(t *testing.T, specDir, snapPath string) {
		t.Helper()
		snapBefore := readBytes(t, snapPath)
		h := &RefreshHandler{
			SnapshotPath: snapPath,
			Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
			Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
			Now:          refreshClock,
		}
		_, err := h.Apply(specDir)
		if !errors.Is(err, ErrRefreshNoCompletedCycle) {
			t.Fatalf("want ErrRefreshNoCompletedCycle, got %v", err)
		}
		if !strings.Contains(err.Error(), "completed cycle") || !strings.Contains(err.Error(), "normal pipeline") {
			t.Errorf("refusal must indicate refresh requires a completed cycle via the normal pipeline: %v", err)
		}
		if got := readBytes(t, snapPath); string(got) != string(snapBefore) {
			t.Error("snapshot must be byte-identical after refusal")
		}
	}

	t.Run("missing journal file", func(t *testing.T) {
		specDir, snapPath := setup(t)
		assertRefused(t, specDir, snapPath)
		if _, statErr := os.Stat(filepath.Join(specDir, ".history.jsonl")); statErr == nil {
			t.Error("journal must not be created by a refused run")
		}
	})

	t.Run("present but empty journal file", func(t *testing.T) {
		specDir, snapPath := setup(t)
		journalPath := filepath.Join(specDir, ".history.jsonl")
		if err := os.WriteFile(journalPath, nil, 0644); err != nil {
			t.Fatal(err)
		}
		assertRefused(t, specDir, snapPath)
		if got := readBytes(t, journalPath); len(got) != 0 {
			t.Errorf("journal must stay empty, got %q", got)
		}
	})
}

// TestRefresh_NonEmptyArtifactsRefused covers the configuration-error
// edge case: any op in the changeset or receipts refuses the run.
func TestRefresh_NonEmptyArtifactsRefused(t *testing.T) {
	fx := setupRefreshFixture(t)

	h := fx.handler()
	h.Changeset = &plan.Changeset{Version: plan.ChangesetVersion, Ops: []plan.Op{{OpID: "op-1", Type: plan.OpCreate}}}
	if _, err := h.Apply(fx.specDir); !errors.Is(err, ErrRefreshNonEmptyArtifacts) {
		t.Fatalf("non-empty changeset: want ErrRefreshNonEmptyArtifacts, got %v", err)
	}

	h = fx.handler()
	h.Receipts = &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk}}}
	if _, err := h.Apply(fx.specDir); !errors.Is(err, ErrRefreshNonEmptyArtifacts) {
		t.Fatalf("non-empty receipts: want ErrRefreshNonEmptyArtifacts, got %v", err)
	}
}

// TestRefresh_SnapshotWriteFailureRollsBackJournal covers the
// atomicity edge case: when the snapshot write fails after the journal
// append, the journal is rolled back so both files stay at the
// pre-refresh state — they move together or not at all.
func TestRefresh_SnapshotWriteFailureRollsBackJournal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based write failure not portable to windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}

	fx := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler v2\n")

	// Move the snapshot into a directory that turns read-only after
	// seeding, so writeAtomic's temp-file creation fails mid-commit.
	roDir := filepath.Join(t.TempDir(), "ro")
	if err := os.MkdirAll(roDir, 0755); err != nil {
		t.Fatal(err)
	}
	lockedSnap := filepath.Join(roDir, ".snapshot.json")
	if err := os.Rename(fx.snapPath, lockedSnap); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(roDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(roDir, 0755) })

	journalBefore := journalBytes(t, fx.specDir)

	h := fx.handler()
	h.SnapshotPath = lockedSnap
	_, err := h.Apply(fx.specDir)
	if err == nil {
		t.Fatal("want snapshot write failure, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error must name the failing step: %v", err)
	}
	if got := journalBytes(t, fx.specDir); string(got) != string(journalBefore) {
		t.Error("journal must be rolled back to its pre-refresh content")
	}
}

// TestRefresh_GitHead_RecordsGivenValueOrRecordedAbsence covers the
// receipt's --git-head contract: a run given a value stamps it onto
// every constructed change event and records it as a JSON string on the
// refresh receipt; a run given none records the absence as JSON null,
// not the empty string.
func TestRefresh_GitHead_RecordsGivenValueOrRecordedAbsence(t *testing.T) {
	fx := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler v2\n")

	head := "cafebabe1234"
	h := fx.handler()
	h.GitHead = &head
	if _, err := h.Apply(fx.specDir); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	raw := string(journalBytes(t, fx.specDir))
	if !strings.Contains(raw, `"git_head":"cafebabe1234"`) {
		t.Errorf("journal must record the given git_head as a string:\n%s", raw)
	}
	if strings.Contains(raw, `"git_head":null`) {
		t.Errorf("journal must not record a null git_head when one was given:\n%s", raw)
	}

	events := readJournal(t, fx.specDir)
	handlerEv, ok := eventForNode(events, fx.handlerID)
	if !ok || handlerEv.GitHead != head {
		t.Errorf("modified event git_head: want %q, got %+v", head, handlerEv)
	}

	// Second fixture: no --git-head given.
	fx2 := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx2.specDir, "beta"), "arch_handler.md", "# Handler v2\n")
	if _, err := fx2.handler().Apply(fx2.specDir); err != nil {
		t.Fatalf("Apply (absent git-head): %v", err)
	}
	raw2 := string(journalBytes(t, fx2.specDir))
	if !strings.Contains(raw2, `"git_head":null`) {
		t.Errorf("journal must record the absence of --git-head as null on the refresh receipt:\n%s", raw2)
	}
}

// Identity-hash keys for the type-filter fixture's varying nodes. The
// anchor component is in every state; each of the others is the single
// node one matrix row toggles.
const (
	refreshTypeAlphaID      = "aabbccddee10"
	refreshTypeAnchorID     = "aabbccddee11"
	refreshTypeComponentID  = "aabbccddee51"
	refreshTypeImplID       = "aabbccddee52"
	refreshTypeAPIID        = "aabbccddee53"
	refreshTypeFlowID       = "aabbccddee54"
	refreshTypeTestID       = "aabbccddee55"
	refreshTypeModuleReqID  = "aabbccddee56"
	refreshTypeProjectReqID = "aabbccddee57"
	refreshTypeGammaID      = "aabbccddee58"
)

// setContentFile writes a markdown leaf when its node is declared and
// deletes it when it is not, so a node the spec no longer references
// never leaves a stray file behind.
func setContentFile(t *testing.T, dir, name, content string, present bool) {
	t.Helper()
	if present {
		writeFile(t, dir, name, content)
		return
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove %s: %v", name, err)
	}
}

// writeRefreshTypeSpec writes the type-filter fixture into specDir. Each
// variant names exactly one node; present toggles whether that node is
// declared. Writing the spec twice — once with present=false, once with
// present=true — yields a diff carrying exactly one structural entry,
// and the caller picks its direction by choosing which of the two states
// it snapshots. The envelope leaves (project.json, module.json) also
// differ between the states, but only ever as *modifications*, which the
// structural gate does not inspect.
func writeRefreshTypeSpec(t *testing.T, specDir, variant string, present bool) {
	t.Helper()
	on := func(v string) bool { return variant == v && present }

	// "meta" is toggled by declaring a whole extra module: gamma holds no
	// content nodes, so its envelope leaf is the only entry it adds.
	gammaModule := ""
	if on("meta") {
		gammaModule = `,
			{"id": "` + refreshTypeGammaID + `", "name": "gamma", "path": "gamma"}`
	}
	projectReqs := ""
	if on("project_requirement") {
		projectReqs = `,
		"requirements": [
			{"id": "` + refreshTypeProjectReqID + `", "type": "functional", "title": "Fixture project requirement"}
		]`
	}
	writeFile(t, specDir, "project.json", `{
		"name": "refresh-type-filter",
		"modules": [
			{"id": "`+refreshTypeAlphaID+`", "name": "alpha", "path": "alpha"}`+gammaModule+`
		]`+projectReqs+`
	}`)

	alphaDir := filepath.Join(specDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}

	components := `{"id": "` + refreshTypeAnchorID + `", "name": "Anchor", "content": "arch_anchor.md"}`
	if on("component") {
		components += `,{"id": "` + refreshTypeComponentID + `", "name": "Extra", "content": "arch_extra.md"}`
	}
	// Assembled as fragments rather than one literal because each optional
	// section must vanish entirely — an empty array is a different leaf.
	sections := []string{`"name": "alpha"`, `"components": [` + components + `]`}
	if on("module_requirement") {
		sections = append(sections, `"requirements": [{"id": "`+refreshTypeModuleReqID+`", "preq_id": "`+refreshTypeProjectReqID+`", "type": "functional", "title": "Fixture module requirement"}]`)
	}
	if on("project_api") {
		sections = append(sections, `"apis": [{"id": "`+refreshTypeImplID+`", "name": "spex extra two", "group": "cli"}]`)
	}
	if on("api") {
		sections = append(sections, `"apis": [{"id": "`+refreshTypeAPIID+`", "name": "spex extra", "group": "cli"}]`)
	}
	if on("data_flow") {
		sections = append(sections, `"data_flows": [{"id": "`+refreshTypeFlowID+`", "name": "Extra flow", "content": "flow_extra.md"}]`)
	}
	if on("test_section") {
		sections = append(sections, `"test_sections": [{"id": "`+refreshTypeTestID+`", "name": "Extra tests", "content": "test_extra.md"}]`)
	}
	writeFile(t, alphaDir, "module.json", "{"+strings.Join(sections, ",")+"}")

	writeFile(t, alphaDir, "arch_anchor.md", "# Anchor\n")
	setContentFile(t, alphaDir, "arch_extra.md", "# Extra component\n", on("component"))
	setContentFile(t, alphaDir, "flow_extra.md", "# Extra flow\n", on("data_flow"))
	setContentFile(t, alphaDir, "test_extra.md", "# Extra tests\n", on("test_section"))

	gammaDir := filepath.Join(specDir, "gamma")
	if on("meta") {
		if err := os.MkdirAll(gammaDir, 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, gammaDir, "module.json", `{"name": "gamma"}`)
	} else if err := os.RemoveAll(gammaDir); err != nil {
		t.Fatalf("remove gamma: %v", err)
	}
}

// TestREQ_e68653819f38_Refresh_TypeFilterMatrix pins the default
// profile's absorbable table cell by cell: for every node type a merkle
// diff can carry, in both
// structural directions, refresh either absorbs the change or refuses it
// with a specific RefreshRefusal.Kind. Any single-cell edit to the
// allow-list — dropping an admitted direction, or admitting a refused
// one — fails a row here.
//
// The journal carries a single bootstrap-only entry in every row — an
// "added" event for the ever-present anchor component, with no
// task_created and so no fold entry at all — just enough to clear the
// empty-journal bootstrap guard without ever standing in for the
// live-pairing gate: a refusal below is the type filter's own, and an
// absorbed removal is not an artefact of a task that happened to already
// be closed. (The live-pairing gate's own role in making component
// removals safe is covered by RefusesRemovedComponentWithLiveTaskPairing
// and AbsorbsAbsorbableStructuralSet.)
//
// requirement is covered at both levels — a module requirement in alpha
// and a project requirement in project.json — because tree_builder
// reaches the two through different paths. Removing a component is
// absorbed here where the dedicated component tests only reach the gate
// through a journal with a task pairing.
func TestREQ_e68653819f38_Refresh_TypeFilterMatrix(t *testing.T) {
	cases := []struct {
		name string
		// variant is the node writeRefreshTypeSpec toggles; key is the
		// diff-entry key a refusal must name.
		variant string
		key     string
		// added true means the node appears after the snapshot was taken
		// (an addition); false means it disappears (a removal).
		added bool
		// wantKind is "" when the change is absorbed, otherwise the
		// RefreshRefusal.Kind the run must refuse with.
		wantKind string
	}{
		{"module requirement added", "module_requirement", refreshTypeModuleReqID, true, ""},
		{"module requirement removed", "module_requirement", refreshTypeModuleReqID, false, ""},
		{"project requirement added", "project_requirement", refreshTypeProjectReqID, true, ""},
		{"project requirement removed", "project_requirement", refreshTypeProjectReqID, false, ""},
		{"api added", "api", refreshTypeAPIID, true, ""},
		{"api removed", "api", refreshTypeAPIID, false, ""},
		{"component added", "component", refreshTypeComponentID, true, "added_entries"},
		{"component removed", "component", refreshTypeComponentID, false, ""},
		{"meta added", "meta", "meta/" + refreshTypeGammaID, true, "added_entries"},
		{"meta removed", "meta", "meta/" + refreshTypeGammaID, false, "removed_entries"},
		{"data_flow added", "data_flow", refreshTypeFlowID, true, "added_entries"},
		{"data_flow removed", "data_flow", refreshTypeFlowID, false, "removed_entries"},
		{"test_section added", "test_section", refreshTypeTestID, true, "added_entries"},
		{"test_section removed", "test_section", refreshTypeTestID, false, "removed_entries"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			specDir := t.TempDir()
			// Baseline is the state without the node for an addition, the
			// state with it for a removal.
			writeRefreshTypeSpec(t, specDir, tc.variant, !tc.added)

			anchorHash, err := merkle.HashFile(filepath.Join(specDir, "alpha", "arch_anchor.md"))
			if err != nil {
				t.Fatal(err)
			}
			seedJournal(t, specDir, mapping.Event{
				Event: "added", EID: "bootstrap0:op-anchor", Node: refreshTypeAnchorID,
				Name: "Anchor", NodeType: "component", Module: refreshTypeAlphaID,
				After: strPtr(anchorHash), GitHead: "bootstrap0", Path: "spec/alpha/arch_anchor.md",
			})

			snapPath := filepath.Join(specDir, ".snapshot.json")
			if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}

			writeRefreshTypeSpec(t, specDir, tc.variant, tc.added)

			journalBefore := journalBytes(t, specDir)
			snapBefore := readBytes(t, snapPath)

			h := &RefreshHandler{
				SnapshotPath: snapPath,
				Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
				Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
				Now:          refreshClock,
			}
			summary, err := h.Apply(specDir)

			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("want the %s %s absorbed, got %v", tc.variant, directionWord(tc.added), err)
				}
				if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
					t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
				}
				assertSnapshotMatchesSpec(t, specDir, snapPath)
				return
			}

			var refusal *RefreshRefusal
			if !errors.As(err, &refusal) {
				t.Fatalf("want the %s %s refused with a RefreshRefusal, got err=%v summary=%+v",
					tc.variant, directionWord(tc.added), err, summary)
			}
			if refusal.Kind != tc.wantKind {
				t.Fatalf("refusal kind: want %q, got %q (%v)", tc.wantKind, refusal.Kind, err)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Errorf("refusal must name the %s entry %s: %v", tc.variant, tc.key, err)
			}
			if !strings.Contains(err.Error(), "normal pipeline") {
				t.Errorf("refusal must point at the normal pipeline: %v", err)
			}
			if got := journalBytes(t, specDir); string(got) != string(journalBefore) {
				t.Error("journal must be byte-identical after refusal")
			}
			if got := readBytes(t, snapPath); string(got) != string(snapBefore) {
				t.Error("snapshot must be byte-identical after refusal")
			}
		})
	}
}

// TestREQ_e68653819f38_Refresh_AbsorbableNamesOnlyReachableTypes guards
// the default profile's absorbable table's *domain*, which the matrix
// above cannot: a key naming a node type no merkle diff can carry is dead
// configuration. It reads as a deliberate decision about both directions
// of that type while the gate never consults it, and it drops silently
// out of the matrix, which can only cover types the tree builder
// actually emits.
//
// The reachable set is observed rather than asserted — it is whatever
// node types the type-filter fixture's own variants produce — so growing
// merkle's leaf vocabulary relaxes this guard automatically. "module" is
// the live example: merkle labels interior module nodes with Type
// "module" but leaves their NodeType empty, and Diff reports leaves
// only, so DefaultProfile().Absorbable["module"] could never be looked
// up — the default profile's table simply carries no "module" row.
// Nothing in Profile.Validate would stop a profile.json from declaring a
// node_types entry named "module" (or "meta") and an absorbable entry
// for it; isAbsorbable refuses both names unconditionally, ahead of the
// profile lookup, regardless of what any profile declares — see
// TestREQ_e68653819f38_Refresh_MetaAndModuleAlwaysRefusedBothDirections.
func TestREQ_e68653819f38_Refresh_AbsorbableNamesOnlyReachableTypes(t *testing.T) {
	variants := []string{
		"module_requirement", "project_requirement",
		"api", "component", "meta", "data_flow", "test_section",
	}
	reachable := map[string]bool{}
	for _, variant := range variants {
		specDir := t.TempDir()
		writeRefreshTypeSpec(t, specDir, variant, false)
		without := buildFixtureTree(t, specDir)
		writeRefreshTypeSpec(t, specDir, variant, true)
		for _, c := range merkle.Diff(buildFixtureTree(t, specDir), without) {
			reachable[c.NodeType] = true
		}
	}

	for nodeType := range schema.DefaultProfile().Absorbable {
		if !reachable[nodeType] {
			t.Errorf("the default profile's absorbable table names %q, a node type no merkle diff carries: "+
				"the entry is unreachable, so its added/removed decision is never consulted", nodeType)
		}
	}
}

// TestREQ_e68653819f38_Refresh_MetaAndModuleAlwaysRefusedBothDirections
// proves the "meta"/"module" refusal is the handler's own fixed rule,
// not merely an artefact of the default profile omitting the two names:
// nothing in schema.Profile.Validate stops a profile.json from declaring
// a node_types entry named "meta" or "module" and marking it absorbable
// in both directions — Validate only rejects an absorbable key naming a
// type the profile never declared (schema/profile.go:180-182) — so a
// careless or malicious profile.json must not be able to grant either
// absorption. The "meta" half is exercised end-to-end: the module
// envelope leaf the gamma toggle adds/removes is a diff entry merkle
// actually emits with NodeType "meta" (tree_builder.go:54, :115), and
// refresh must refuse it despite the profile's explicit grant. "module"
// is never a NodeType a merkle diff entry carries (Diff reports leaves
// only — see AbsorbableNamesOnlyReachableTypes above), so its half is
// checked directly against isAbsorbable, the same function the gate
// calls.
func TestREQ_e68653819f38_Refresh_MetaAndModuleAlwaysRefusedBothDirections(t *testing.T) {
	maliciousProfileJSON := `{
		"node_types": [
			{"name": "component", "plural_key": "components", "scope": "module", "requires_content": true},
			{"name": "meta", "plural_key": "metas", "scope": "module"},
			{"name": "module", "plural_key": "modules_extra", "scope": "module"}
		],
		"absorbable": {
			"component": {"added": true, "removed": true},
			"meta": {"added": true, "removed": true},
			"module": {"added": true, "removed": true}
		}
	}`

	var maliciousProfile schema.Profile
	if err := json.Unmarshal([]byte(maliciousProfileJSON), &maliciousProfile); err != nil {
		t.Fatal(err)
	}
	if err := maliciousProfile.Validate(); err != nil {
		t.Fatalf("a profile declaring \"meta\" and \"module\" node types must validate cleanly — that is the exploit this test guards against: %v", err)
	}

	for _, nodeType := range []string{"meta", "module"} {
		for _, dir := range []merkle.ChangeType{merkle.Added, merkle.Removed} {
			if isAbsorbable(nodeType, dir, &maliciousProfile) {
				t.Errorf("isAbsorbable(%q, %v, profile) = true against a profile that declares it absorbable; want the fixed rule to refuse regardless of the profile", nodeType, dir)
			}
		}
	}

	for _, added := range []bool{true, false} {
		name, wantKind := "meta added", "added_entries"
		if !added {
			name, wantKind = "meta removed", "removed_entries"
		}
		t.Run(name, func(t *testing.T) {
			specDir := t.TempDir()
			writeFile(t, specDir, "profile.json", maliciousProfileJSON)
			writeRefreshTypeSpec(t, specDir, "meta", !added)

			anchorHash, err := merkle.HashFile(filepath.Join(specDir, "alpha", "arch_anchor.md"))
			if err != nil {
				t.Fatal(err)
			}
			seedJournal(t, specDir, mapping.Event{
				Event: "added", EID: "bootstrap0:op-anchor", Node: refreshTypeAnchorID,
				Name: "Anchor", NodeType: "component", Module: refreshTypeAlphaID,
				After: strPtr(anchorHash), GitHead: "bootstrap0", Path: "spec/alpha/arch_anchor.md",
			})

			snapPath := filepath.Join(specDir, ".snapshot.json")
			if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}

			writeRefreshTypeSpec(t, specDir, "meta", added)

			h := &RefreshHandler{
				SnapshotPath: snapPath,
				Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
				Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
				Now:          refreshClock,
			}
			_, err = h.Apply(specDir)

			var refusal *RefreshRefusal
			if !errors.As(err, &refusal) || refusal.Kind != wantKind {
				t.Fatalf("want RefreshRefusal %s despite the profile declaring \"meta\" absorbable in both directions, got %v", wantKind, err)
			}
			if !strings.Contains(err.Error(), "meta/"+refreshTypeGammaID) {
				t.Errorf("refusal must name the meta entry: %v", err)
			}
		})
	}
}

// TestREQ_e68653819f38_Refresh_AbsorbableTableIsResolvedProfileDeclaration
// covers arch_refresh.md's "The absorbable table is the resolved
// profile's declaration": the per-type, per-direction table
// RefreshHandler consults is read off schema.ResolveProfile's Absorbable
// map, not compiled into the handler. A representative sample of the
// type-filter matrix — an absorbed api addition and a refused component
// addition — must behave identically whether profile.json is absent (the
// built-in default) or present and byte-identical to
// schema.DefaultProfile(): a default-profile run must be
// indistinguishable from the pre-profile binary.
func TestREQ_e68653819f38_Refresh_AbsorbableTableIsResolvedProfileDeclaration(t *testing.T) {
	defaultProfileJSON, err := json.Marshal(schema.DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		variant  string
		key      string
		added    bool
		wantKind string
	}{
		{"api added: absorbed", "api", refreshTypeAPIID, true, ""},
		{"component added: refused", "component", refreshTypeComponentID, true, "added_entries"},
	}

	for _, tc := range cases {
		for _, withProfile := range []bool{false, true} {
			profileLabel := "no profile.json"
			if withProfile {
				profileLabel = "profile.json byte-identical to default"
			}
			t.Run(tc.name+"/"+profileLabel, func(t *testing.T) {
				specDir := t.TempDir()
				if withProfile {
					writeFile(t, specDir, "profile.json", string(defaultProfileJSON))
				}
				writeRefreshTypeSpec(t, specDir, tc.variant, !tc.added)

				anchorHash, err := merkle.HashFile(filepath.Join(specDir, "alpha", "arch_anchor.md"))
				if err != nil {
					t.Fatal(err)
				}
				seedJournal(t, specDir, mapping.Event{
					Event: "added", EID: "bootstrap0:op-anchor", Node: refreshTypeAnchorID,
					Name: "Anchor", NodeType: "component", Module: refreshTypeAlphaID,
					After: strPtr(anchorHash), GitHead: "bootstrap0", Path: "spec/alpha/arch_anchor.md",
				})

				snapPath := filepath.Join(specDir, ".snapshot.json")
				if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
					t.Fatalf("seed snapshot: %v", err)
				}

				writeRefreshTypeSpec(t, specDir, tc.variant, tc.added)

				h := &RefreshHandler{
					SnapshotPath: snapPath,
					Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
					Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
					Now:          refreshClock,
				}
				summary, err := h.Apply(specDir)

				if tc.wantKind == "" {
					if err != nil {
						t.Fatalf("want the %s addition absorbed, got %v", tc.variant, err)
					}
					if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
						t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
					}
					return
				}

				var refusal *RefreshRefusal
				if !errors.As(err, &refusal) || refusal.Kind != tc.wantKind {
					t.Fatalf("want RefreshRefusal %s, got %v", tc.wantKind, err)
				}
				if !strings.Contains(err.Error(), tc.key) {
					t.Errorf("refusal must name the %s entry %s: %v", tc.variant, tc.key, err)
				}
			})
		}
	}
}

// TestREQ_e68653819f38_Refresh_ProfileDeclaredTypeRefusedBothDirections
// covers the other half of the same scenario: a profile declaring a
// node type ("endpoint") the built-in default never heard of, absorbable
// in neither direction, refuses an added endpoint with the same
// "use the normal pipeline" error the non-absorbable built-ins draw —
// the gate's policy is entirely the profile's declaration, not a
// hardcoded set of known type names.
func TestREQ_e68653819f38_Refresh_ProfileDeclaredTypeRefusedBothDirections(t *testing.T) {
	const alphaID = "aabbccddee10"
	const anchorID = "aabbccddee11"
	const endpointID = "aabbccddee61"

	specDir := t.TempDir()
	writeFile(t, specDir, "profile.json", `{
		"node_types": [
			{"name": "component", "plural_key": "components", "scope": "module", "requires_content": true},
			{"name": "endpoint", "plural_key": "endpoints", "scope": "module", "requires_content": true}
		],
		"absorbable": {
			"component": {"added": false, "removed": true},
			"endpoint": {"added": false, "removed": false}
		}
	}`)

	writeEndpointSpec := func(withEndpoint bool) {
		writeFile(t, specDir, "project.json", `{
			"name": "profile-endpoint-fixture",
			"modules": [{"id": "`+alphaID+`", "name": "alpha", "path": "alpha"}]
		}`)
		alphaDir := filepath.Join(specDir, "alpha")
		if err := os.MkdirAll(alphaDir, 0755); err != nil {
			t.Fatal(err)
		}
		endpoints := ""
		if withEndpoint {
			endpoints = `,"endpoints": [{"id": "` + endpointID + `", "name": "Get widget", "content": "endpoint_get_widget.md"}]`
		}
		writeFile(t, alphaDir, "module.json", `{
			"name": "alpha",
			"components": [{"id": "`+anchorID+`", "name": "Anchor", "content": "arch_anchor.md"}]`+endpoints+`
		}`)
		writeFile(t, alphaDir, "arch_anchor.md", "# Anchor\n")
		setContentFile(t, alphaDir, "endpoint_get_widget.md", "# Get widget\n", withEndpoint)
	}

	writeEndpointSpec(false)

	anchorHash, err := merkle.HashFile(filepath.Join(specDir, "alpha", "arch_anchor.md"))
	if err != nil {
		t.Fatal(err)
	}
	seedJournal(t, specDir, mapping.Event{
		Event: "added", EID: "bootstrap0:op-anchor", Node: anchorID,
		Name: "Anchor", NodeType: "component", Module: alphaID,
		After: strPtr(anchorHash), GitHead: "bootstrap0", Path: "spec/alpha/arch_anchor.md",
	})

	snapPath := filepath.Join(specDir, ".snapshot.json")
	if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	writeEndpointSpec(true)

	journalBefore := journalBytes(t, specDir)
	snapBefore := readBytes(t, snapPath)

	h := &RefreshHandler{
		SnapshotPath: snapPath,
		Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
		Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
		Now:          refreshClock,
	}
	_, err = h.Apply(specDir)

	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "added_entries" {
		t.Fatalf("want RefreshRefusal added_entries, got %v", err)
	}
	if !strings.Contains(err.Error(), endpointID) {
		t.Errorf("refusal must name the added endpoint entry: %v", err)
	}
	if !strings.Contains(err.Error(), "normal pipeline") {
		t.Errorf("refusal must point at the normal pipeline: %v", err)
	}
	if got := journalBytes(t, specDir); string(got) != string(journalBefore) {
		t.Error("journal must be byte-identical after refusal")
	}
	if got := readBytes(t, snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestREQ_e68653819f38_Refresh_AbsorbableTableOverridesBuiltInDefault
// proves the gate reads the resolved profile rather than a table
// compiled into the handler: a profile that flips a built-in default —
// declaring "api" absorbable in neither direction, and "component"
// absorbable on addition too — must flip RefreshHandler's behavior
// accordingly. A hardcoded fallback would refuse the component addition
// and absorb the api addition regardless of what any profile.json says;
// only reading the table off the resolved profile changes the outcome
// with it.
func TestREQ_e68653819f38_Refresh_AbsorbableTableOverridesBuiltInDefault(t *testing.T) {
	cases := []struct {
		name    string
		variant string
		key     string
	}{
		{"api addition, declared unabsorbable, refuses despite default absorbing it", "api", refreshTypeAPIID},
		{"component addition, declared absorbable, absorbs despite default refusing it", "component", refreshTypeComponentID},
	}

	overriddenProfileJSON := `{
		"node_types": [
			{"name": "requirement", "plural_key": "requirements", "scope": "project"},
			{"name": "requirement", "plural_key": "requirements", "scope": "module"},
			{"name": "component", "plural_key": "components", "scope": "module", "requires_content": true},
			{"name": "data_flow", "plural_key": "data_flows", "scope": "module", "requires_content": true},
			{"name": "test_section", "plural_key": "test_sections", "scope": "module", "requires_content": true},
			{"name": "api", "plural_key": "apis", "scope": "module"}
		],
		"absorbable": {
			"requirement": {"added": true, "removed": true},
			"api": {"added": false, "removed": false},
			"component": {"added": true, "removed": true},
			"data_flow": {"added": false, "removed": false},
			"test_section": {"added": false, "removed": false}
		}
	}`

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			specDir := t.TempDir()
			writeFile(t, specDir, "profile.json", overriddenProfileJSON)
			writeRefreshTypeSpec(t, specDir, tc.variant, false)

			anchorHash, err := merkle.HashFile(filepath.Join(specDir, "alpha", "arch_anchor.md"))
			if err != nil {
				t.Fatal(err)
			}
			seedJournal(t, specDir, mapping.Event{
				Event: "added", EID: "bootstrap0:op-anchor", Node: refreshTypeAnchorID,
				Name: "Anchor", NodeType: "component", Module: refreshTypeAlphaID,
				After: strPtr(anchorHash), GitHead: "bootstrap0", Path: "spec/alpha/arch_anchor.md",
			})

			snapPath := filepath.Join(specDir, ".snapshot.json")
			if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}

			writeRefreshTypeSpec(t, specDir, tc.variant, true)

			h := &RefreshHandler{
				SnapshotPath: snapPath,
				Changeset:    &plan.Changeset{Version: plan.ChangesetVersion},
				Receipts:     &adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete},
				Now:          refreshClock,
			}
			summary, err := h.Apply(specDir)

			switch tc.variant {
			case "api":
				var refusal *RefreshRefusal
				if !errors.As(err, &refusal) || refusal.Kind != "added_entries" {
					t.Fatalf("want RefreshRefusal added_entries (profile declares api unabsorbable), got %v", err)
				}
				if !strings.Contains(err.Error(), tc.key) {
					t.Errorf("refusal must name the added entry %s: %v", tc.key, err)
				}
			case "component":
				if err != nil {
					t.Fatalf("want component addition absorbed (profile declares it absorbable), got %v", err)
				}
				if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
					t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
				}
			}
		})
	}
}

// directionWord renders a matrix row's direction for failure messages.
func directionWord(added bool) string {
	if added {
		return "addition"
	}
	return "removal"
}

// TestRefreshRefusal_ErrorNamesKindEntriesAndHint pins the structured
// error rendering the IngestCommand surfaces on stderr.
func TestRefreshRefusal_ErrorNamesKindEntriesAndHint(t *testing.T) {
	e := &RefreshRefusal{Kind: "added_entries", Entries: []string{"aaa", "bbb"}, Hint: "use the normal pipeline"}
	msg := e.Error()
	for _, want := range []string{"added_entries", "aaa", "bbb", "use the normal pipeline"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q must contain %q", msg, want)
		}
	}
}
