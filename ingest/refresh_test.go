package ingest

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
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
		Changeset:    &emit.Changeset{Version: emit.ChangesetVersion},
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

// TestREQ_e68653819f38_Refresh_RefusesAddedModuleMeta is the direct
// guard on writing the allow-list as the complement of impact's
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

// TestRefresh_MissingSnapshotRefused covers the edge case: refresh's
// diff baseline is the snapshot; without one the run is refused before
// touching anything.
func TestRefresh_MissingSnapshotRefused(t *testing.T) {
	fx := setupRefreshFixture(t)
	if err := os.Remove(fx.snapPath); err != nil {
		t.Fatal(err)
	}

	_, err := fx.handler().Apply(fx.specDir)
	if !errors.Is(err, ErrRefreshRequiresSnapshot) {
		t.Fatalf("want ErrRefreshRequiresSnapshot, got %v", err)
	}
}

// TestRefresh_NonEmptyArtifactsRefused covers the configuration-error
// edge case: any op in the changeset or receipts refuses the run.
func TestRefresh_NonEmptyArtifactsRefused(t *testing.T) {
	fx := setupRefreshFixture(t)

	h := fx.handler()
	h.Changeset = &emit.Changeset{Version: emit.ChangesetVersion, Ops: []emit.Op{{OpID: "op-1", Type: emit.OpCreate}}}
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

// TestREQ_e68653819f38_Refresh_TypeFilterMatrix pins refreshAbsorbable
// cell by cell: for every node type a merkle diff can carry, in both
// structural directions, refresh either absorbs the change or refuses it
// with a specific RefreshRefusal.Kind. Any single-cell edit to the
// allow-list — dropping an admitted direction, or admitting a refused
// one — fails a row here.
//
// The journal is empty in every row so the live-pairing gate can never
// stand in for the type filter: a refusal below is the type filter's
// own, and an absorbed removal is not an artefact of a task that
// happened to already be closed. (The live-pairing gate's own role in
// making component removals safe is covered by
// RefusesRemovedComponentWithLiveTaskPairing and
// AbsorbsAbsorbableStructuralSet.)
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
			snapPath := filepath.Join(specDir, ".snapshot.json")
			if err := writeAtomic(snapPath, buildFixtureTree(t, specDir), refreshClock()); err != nil {
				t.Fatalf("seed snapshot: %v", err)
			}

			writeRefreshTypeSpec(t, specDir, tc.variant, tc.added)

			journalBefore := journalBytes(t, specDir)
			snapBefore := readBytes(t, snapPath)

			h := &RefreshHandler{
				SnapshotPath: snapPath,
				Changeset:    &emit.Changeset{Version: emit.ChangesetVersion},
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
// the allow-list's *domain*, which the matrix above cannot: a key naming
// a node type no merkle diff can carry is dead configuration. It reads
// as a deliberate decision about both directions of that type while the
// gate never consults it, and it drops silently out of the matrix, which
// can only cover types the tree builder actually emits.
//
// The reachable set is observed rather than asserted — it is whatever
// node types the type-filter fixture's own variants produce — so growing
// merkle's leaf vocabulary relaxes this guard automatically. "module" is
// the live example: merkle labels interior module nodes with Type
// "module" but leaves their NodeType empty, and Diff reports leaves
// only, so refreshAbsorbable["module"] could never be looked up.
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

	for nodeType := range refreshAbsorbable {
		if !reachable[nodeType] {
			t.Errorf("refreshAbsorbable names %q, a node type no merkle diff carries: "+
				"the entry is unreachable, so its added/removed decision is never consulted", nodeType)
		}
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
