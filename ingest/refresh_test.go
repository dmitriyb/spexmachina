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
// a seeded bead-map whose records' spec_hash values match the snapshot.
type refreshFixture struct {
	specDir  string
	snapPath string
	mapPath  string
	store    mapping.Store
	// widgetID/handlerID are component spec_node_ids with bead-map
	// records; implID is a record-less impl_section leaf.
	widgetID  string
	handlerID string
	implID    string
}

var refreshClock = func() time.Time {
	return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
}

// setupRefreshFixture writes the fixture spec, snapshots it, and seeds
// the bead-map with records whose spec_hash matches the snapshot state
// (i.e., clean — tests introduce drift by editing content files after).
func setupRefreshFixture(t *testing.T) refreshFixture {
	t.Helper()
	specDir := t.TempDir()

	fx := refreshFixture{
		specDir:   specDir,
		snapPath:  filepath.Join(specDir, ".snapshot.json"),
		widgetID:  "aabbccddee01",
		handlerID: "aabbccddee02",
		implID:    "aabbccddee03",
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
		"impl_sections": [
			{"id": "`+fx.implID+`", "name": "Widget logic", "content": "impl_widget_logic.md"}
		]
	}`)
	writeFile(t, alphaDir, "arch_widget.md", "# Widget\n")
	writeFile(t, alphaDir, "impl_widget_logic.md", "# Widget logic\n")

	betaDir := filepath.Join(specDir, "beta")
	if err := os.MkdirAll(betaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, betaDir, "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+fx.handlerID+`", "name": "Handler", "content": "arch_handler.md"}
		]
	}`)
	writeFile(t, betaDir, "arch_handler.md", "# Handler\n")

	// Snapshot the initial state — the diff baseline.
	tree := buildFixtureTree(t, specDir)
	if err := writeAtomic(fx.snapPath, tree, refreshClock()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	hashes := map[string]string{}
	collectLeafHashes(hashes, tree)

	records := []mapping.Record{
		{ID: 1, SpecNodeID: fx.widgetID, BeadID: "br-widget", BeadType: "feature", NodeType: "component", Module: "alpha", Component: "Widget", ContentFile: "spec/alpha/arch_widget.md", SpecHash: hashes[fx.widgetID], BeadStatus: "closed"},
		{ID: 2, SpecNodeID: fx.handlerID, BeadID: "br-handler", BeadType: "feature", NodeType: "component", Module: "beta", Component: "Handler", ContentFile: "spec/beta/arch_handler.md", SpecHash: hashes[fx.handlerID], BeadStatus: "open"},
		{ID: 3, SpecNodeID: "2026-06-01-some-proposal", BeadID: "br-epic", BeadType: "epic", NodeType: "proposal", Module: "", Component: "2026-06-01-some-proposal", ContentFile: "", SpecHash: "", BeadStatus: "open"},
	}
	fx.store, fx.mapPath = newTestStore(t, records, 4)
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
		Store:        fx.store,
		SnapshotPath: fx.snapPath,
		Changeset:    &emit.Changeset{Version: 1},
		Receipts:     &adapters.Receipts{Version: 1, Status: adapters.StatusComplete},
		Now:          refreshClock,
	}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func recordByID(t *testing.T, store mapping.Store, id int) mapping.Record {
	t.Helper()
	rec, err := store.Get(id)
	if err != nil {
		t.Fatalf("get record %d: %v", id, err)
	}
	return rec
}

// TestRefresh_ModifiedOnlyDiff_UpdatesStaleHashes covers the headline
// scenario: content-only edits are absorbed — stale records' spec_hash
// updates, untouched records stay byte-identical, and the snapshot is
// rewritten to the current state.
func TestRefresh_ModifiedOnlyDiff_UpdatesStaleHashes(t *testing.T) {
	fx := setupRefreshFixture(t)

	// Drift two leaves: one with a record (Handler) and one without
	// (the impl_section) — both are content-only modifications.
	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler (revised)\n")
	writeFile(t, filepath.Join(fx.specDir, "alpha"), "impl_widget_logic.md", "# Widget logic (clarified)\n")

	widgetBefore := recordByID(t, fx.store, 1)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if summary.RecordsUpdated != 1 {
		t.Errorf("records_updated: want 1 (Handler), got %d", summary.RecordsUpdated)
	}
	if summary.RecordsUnchanged != 2 {
		t.Errorf("records_unchanged: want 2 (Widget + proposal), got %d", summary.RecordsUnchanged)
	}
	if !summary.SnapshotSaved || summary.Status != adapters.StatusComplete {
		t.Errorf("want snapshot_saved=true status=complete, got %+v", summary)
	}

	tree := buildFixtureTree(t, fx.specDir)
	hashes := map[string]string{}
	collectLeafHashes(hashes, tree)

	if got := recordByID(t, fx.store, 2); got.SpecHash != hashes[fx.handlerID] {
		t.Errorf("handler spec_hash: want current %s, got %s", hashes[fx.handlerID], got.SpecHash)
	}
	if got := recordByID(t, fx.store, 1); got != widgetBefore {
		t.Errorf("widget record must be untouched: before %+v, after %+v", widgetBefore, got)
	}

	snapTree, err := merkle.Load(fx.snapPath)
	if err != nil {
		t.Fatalf("load rewritten snapshot: %v", err)
	}
	if snapTree.Hash != tree.Hash {
		t.Errorf("snapshot root: want current %s, got %s", tree.Hash, snapTree.Hash)
	}
}

// TestRefresh_RefusesOnAddedEntries covers the structural gate: a new
// content leaf in the diff refuses the run and leaves both files
// byte-identical.
func TestRefresh_RefusesOnAddedEntries(t *testing.T) {
	fx := setupRefreshFixture(t)
	alphaDir := filepath.Join(fx.specDir, "alpha")
	writeFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+fx.widgetID+`", "name": "Widget", "content": "arch_widget.md"},
			{"id": "aabbccddee99", "name": "NewThing", "content": "arch_new_thing.md"}
		],
		"impl_sections": [
			{"id": "`+fx.implID+`", "name": "Widget logic", "content": "impl_widget_logic.md"}
		]
	}`)
	writeFile(t, alphaDir, "arch_new_thing.md", "# New thing\n")

	mapBefore := readBytes(t, fx.mapPath)
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
	if got := readBytes(t, fx.mapPath); string(got) != string(mapBefore) {
		t.Error("bead-map must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestRefresh_RefusesOnRemovedEntries covers the other structural gate:
// deleting a leaf refuses the run with the same use-the-normal-pipeline
// error and no file changes.
func TestRefresh_RefusesOnRemovedEntries(t *testing.T) {
	fx := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx.specDir, "beta"), "module.json", `{
		"name": "beta"
	}`)

	mapBefore := readBytes(t, fx.mapPath)
	snapBefore := readBytes(t, fx.snapPath)

	_, err := fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "removed_entries" {
		t.Fatalf("want RefreshRefusal removed_entries, got %v", err)
	}
	if !strings.Contains(err.Error(), fx.handlerID) {
		t.Errorf("refusal must name the removed entry %s: %v", fx.handlerID, err)
	}
	if got := readBytes(t, fx.mapPath); string(got) != string(mapBefore) {
		t.Error("bead-map must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestRefresh_RefusesOnOrphanRecord covers the orphan gate: a bead-map
// record whose spec_node_id has no live spec node names both the node
// and the bead in the refusal, and neither file changes. The proposal
// record (spec_node_id = proposal ref) is exempt by design.
func TestRefresh_RefusesOnOrphanRecord(t *testing.T) {
	fx := setupRefreshFixture(t)
	records, err := fx.store.List()
	if err != nil {
		t.Fatal(err)
	}
	records = append(records, mapping.Record{
		ID: 9, SpecNodeID: "deaddeadbeef", BeadID: "br-ghost", BeadType: "feature",
		NodeType: "component", Module: "alpha", Component: "Ghost",
		ContentFile: "spec/alpha/arch_ghost.md", SpecHash: "h",
	})
	if err := fx.store.Replace(records, 10); err != nil {
		t.Fatal(err)
	}

	mapBefore := readBytes(t, fx.mapPath)
	snapBefore := readBytes(t, fx.snapPath)

	_, err = fx.handler().Apply(fx.specDir)
	var refusal *RefreshRefusal
	if !errors.As(err, &refusal) || refusal.Kind != "orphan_record" {
		t.Fatalf("want RefreshRefusal orphan_record, got %v", err)
	}
	if !strings.Contains(err.Error(), "deaddeadbeef") || !strings.Contains(err.Error(), "br-ghost") {
		t.Errorf("refusal must name spec_node_id and bead_id: %v", err)
	}
	if got := readBytes(t, fx.mapPath); string(got) != string(mapBefore) {
		t.Error("bead-map must be byte-identical after refusal")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical after refusal")
	}
}

// TestRefresh_CleanSpecIsNoOp covers the no-drift scenario: refresh on
// a spec byte-identical to the snapshot succeeds, updates nothing, and
// rewrites neither file (so git status is unperturbed).
func TestRefresh_CleanSpecIsNoOp(t *testing.T) {
	fx := setupRefreshFixture(t)

	mapBefore := readBytes(t, fx.mapPath)
	snapBefore := readBytes(t, fx.snapPath)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if summary.RecordsUpdated != 0 || summary.SnapshotSaved {
		t.Errorf("want zero updates and no snapshot write, got %+v", summary)
	}
	if summary.Status != adapters.StatusComplete {
		t.Errorf("status: want complete, got %q", summary.Status)
	}
	if got := readBytes(t, fx.mapPath); string(got) != string(mapBefore) {
		t.Error("bead-map must be byte-identical on a clean no-op")
	}
	if got := readBytes(t, fx.snapPath); string(got) != string(snapBefore) {
		t.Error("snapshot must be byte-identical on a clean no-op")
	}
}

// TestRefresh_RerunIsIdempotent covers the idempotency scenario: a
// second refresh over the state the first one produced updates zero
// records and leaves both files byte-identical.
func TestRefresh_RerunIsIdempotent(t *testing.T) {
	fx := setupRefreshFixture(t)
	writeFile(t, filepath.Join(fx.specDir, "beta"), "arch_handler.md", "# Handler v2\n")

	if _, err := fx.handler().Apply(fx.specDir); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	mapAfterFirst := readBytes(t, fx.mapPath)
	snapAfterFirst := readBytes(t, fx.snapPath)

	summary, err := fx.handler().Apply(fx.specDir)
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if summary.RecordsUpdated != 0 {
		t.Errorf("second run: want zero updates, got %d", summary.RecordsUpdated)
	}
	if got := readBytes(t, fx.mapPath); string(got) != string(mapAfterFirst) {
		t.Error("bead-map must end byte-identical to the first run's state")
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
	h.Changeset = &emit.Changeset{Version: 1, Ops: []emit.Op{{OpID: "op-1", Type: emit.OpCreate}}}
	if _, err := h.Apply(fx.specDir); !errors.Is(err, ErrRefreshNonEmptyArtifacts) {
		t.Fatalf("non-empty changeset: want ErrRefreshNonEmptyArtifacts, got %v", err)
	}

	h = fx.handler()
	h.Receipts = &adapters.Receipts{Version: 1, Status: adapters.StatusComplete, Ops: []adapters.OpReceipt{{OpID: "op-1", Status: adapters.OpStatusOk}}}
	if _, err := h.Apply(fx.specDir); !errors.Is(err, ErrRefreshNonEmptyArtifacts) {
		t.Fatalf("non-empty receipts: want ErrRefreshNonEmptyArtifacts, got %v", err)
	}
}

// TestRefresh_SnapshotWriteFailureRollsBackBeadMap covers the atomicity
// edge case: when the snapshot write fails after the bead-map write,
// the bead-map is rolled back so both files stay at the pre-refresh
// state — they move together or not at all.
func TestRefresh_SnapshotWriteFailureRollsBackBeadMap(t *testing.T) {
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

	mapBefore := readBytes(t, fx.mapPath)

	h := fx.handler()
	h.SnapshotPath = lockedSnap
	_, err := h.Apply(fx.specDir)
	if err == nil {
		t.Fatal("want snapshot write failure, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error must name the failing step: %v", err)
	}
	if got := readBytes(t, fx.mapPath); string(got) != string(mapBefore) {
		t.Error("bead-map must be rolled back to its pre-refresh content")
	}
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
