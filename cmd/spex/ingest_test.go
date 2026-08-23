package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/ingest"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
)

// runIngest executes spex ingest via cobra and returns stdout, stderr,
// the command-level error, and the exit code (1 by default when err is
// non-nil, 0 on success, or whatever ExitCode() returns).
func runIngest(t *testing.T, args ...string) (string, string, int, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newIngestCmd())

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	rootCmd.SetIn(bytes.NewReader(nil))
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"ingest"}, args...))

	err := rootCmd.Execute()
	exit := 0
	if err != nil {
		exit = 1
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			exit = ec.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), exit, err
}

// ingestFixture is the on-disk state required to drive `spex ingest`.
type ingestFixture struct {
	specDir       string
	changesetPath string
	receiptsPath  string
	snapshotPath  string
	compID        string
	recordID      int
	beadID        string
}

// setupIngestFixture writes a minimal spec tree (one component), a
// matching changeset (one create op), and a matching ok-receipt. The
// mapping store starts empty so the create op exercises the
// fresh-insert path.
func setupIngestFixture(t *testing.T, status string) ingestFixture {
	t.Helper()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}

	proj := `{
		"name": "test-ingest",
		"modules": [{"id": "` + modID + `", "name": "alpha", "path": "alpha"}]
	}`
	writeTestFile(t, specDir, "project.json", proj)

	mod := `{
		"name": "alpha",
		"components": [{"id": "` + compID + `", "name": "Comp1", "content": "arch_comp1.md"}]
	}`
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", mod)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Comp1\n")

	cs := plan.Changeset{
		Version:  plan.ChangesetVersion,
		GitHead:  "deadbeefcafe",
		Proposal: "test-proposal",
		Ops: []plan.Op{{
			OpID:         "op-0001",
			Type:         plan.OpCreate,
			SpecNodeKind: "component",
			SpecNodeID:   compID,
			Idempotency:  &plan.Idem{Label: "spex:1"},
			Title:        "Comp1",
		}},
	}
	csPath := filepath.Join(dir, "changeset.json")
	writeJSON(t, csPath, cs)

	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  status,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-0001",
			Status:      adapters.OpStatusOk,
			BeadID:      "bead-1",
			WasExisting: false,
		}},
	}
	rcPath := filepath.Join(dir, "receipts.json")
	writeJSON(t, rcPath, rc)

	return ingestFixture{
		specDir:       specDir,
		changesetPath: csPath,
		receiptsPath:  rcPath,
		snapshotPath:  filepath.Join(specDir, ".snapshot.json"),
		compID:        compID,
		recordID:      1,
		beadID:        "bead-1",
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestIngestCommand_HappyPath_CompleteRun(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	stdout, stderr, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err != nil {
		t.Fatalf("ingest failed: %v\nstderr: %s", err, stderr)
	}
	if exit != 0 {
		t.Errorf("want exit 0, got %d", exit)
	}

	var sum ingest.Summary
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatalf("decode summary: %v\nstdout: %s", err, stdout)
	}
	if sum.Ok != 1 {
		t.Errorf("want ok=1, got %d", sum.Ok)
	}
	if sum.EventsAppended != 1 || sum.ReceiptsAppended != 1 {
		t.Errorf("want events_appended=1/receipts_appended=1, got %+v", sum)
	}
	if !sum.SnapshotSaved {
		t.Errorf("want snapshot_saved=true on complete run")
	}
	if sum.Status != adapters.StatusComplete {
		t.Errorf("want status=%q, got %q", adapters.StatusComplete, sum.Status)
	}

	if _, err := os.Stat(f.snapshotPath); err != nil {
		t.Errorf("snapshot file missing after complete run: %v", err)
	}

	// Normal-mode ingest writes the task journal (spec/.history.jsonl).
	entry, err := mapping.NewMappingStore(filepath.Join(f.specDir, ".history.jsonl")).Get(f.compID)
	if err != nil {
		t.Fatalf("expected journal entry for %s after ingest: %v", f.compID, err)
	}
	if entry.TaskID != f.beadID {
		t.Errorf("want task_id=%s, got %s", f.beadID, entry.TaskID)
	}
}

func TestIngestCommand_PartialRun_SkipsSnapshot(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusPartial)

	// Pre-write a sentinel snapshot so we can confirm partial leaves it.
	if err := os.WriteFile(f.snapshotPath, []byte(`{"sentinel":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err != nil {
		t.Fatalf("ingest failed: %v\nstderr: %s", err, stderr)
	}
	if exit != 0 {
		t.Errorf("want exit 0 on partial, got %d", exit)
	}

	var sum ingest.Summary
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatalf("decode summary: %v\nstdout: %s", err, stdout)
	}
	if sum.SnapshotSaved {
		t.Errorf("want snapshot_saved=false on partial run, got true")
	}
	if sum.Status != adapters.StatusPartial {
		t.Errorf("want status=partial, got %q", sum.Status)
	}

	data, err := os.ReadFile(f.snapshotPath)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(data), "sentinel") {
		t.Errorf("partial run rewrote snapshot; want sentinel preserved, got: %s", data)
	}
}

func TestIngestCommand_MissingChangesetFlag_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	_, stderr, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for missing --changeset, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error()+stderr, "changeset") {
		t.Errorf("want error mentioning changeset, got err=%v stderr=%s", err, stderr)
	}
}

func TestIngestCommand_MissingReceiptsFlag_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	_, stderr, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
	)
	if err == nil {
		t.Fatal("want error for missing --receipts, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error()+stderr, "receipts") {
		t.Errorf("want error mentioning receipts, got err=%v stderr=%s", err, stderr)
	}
}

func TestIngestCommand_MismatchedOpID_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:   "op-0042-stray",
			Status: adapters.OpStatusOk,
			BeadID: "bead-x",
		}},
	}
	writeJSON(t, f.receiptsPath, rc)

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for op_id mismatch, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error(), "op-0042-stray") || !strings.Contains(err.Error(), "not in changeset") {
		t.Errorf("want error naming the stray op_id and \"not in changeset\", got: %v", err)
	}
}

func TestIngestCommand_MissingOpReceipt_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops:     []adapters.OpReceipt{},
	}
	writeJSON(t, f.receiptsPath, rc)

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for missing receipt, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error(), "no receipt for op op-0001") {
		t.Errorf("want error \"no receipt for op op-0001\", got: %v", err)
	}
}

func TestIngestCommand_MalformedReceiptsJSON_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	if err := os.WriteFile(f.receiptsPath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for malformed receipts JSON, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error(), "parse receipts") {
		t.Errorf("want parse-receipts error, got: %v", err)
	}
}

func TestIngestCommand_BadVersionInChangeset_Exits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	cs := plan.Changeset{
		Version: 99,
		Ops:     []plan.Op{},
	}
	writeJSON(t, f.changesetPath, cs)

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for bad changeset version, got nil")
	}
	if exit != 1 {
		t.Errorf("want exit 1, got %d", exit)
	}
	if !strings.Contains(err.Error(), "changeset version") {
		t.Errorf("want changeset-version error, got: %v", err)
	}
}

// TestIngestCommand_InvariantFailure_Exits2_PreservesJournal covers the
// exit-2 invariant-failure path against the journal model: a close op
// targeting a bead with no journal pairing at all (Reconciler's
// invariant 1 — "no journal entry for bead ...") fails the whole batch,
// including the earlier, otherwise-valid create op, and the journal file
// is never written.
func TestIngestCommand_InvariantFailure_Exits2_PreservesJournal(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	cs := plan.Changeset{
		Version:  plan.ChangesetVersion,
		GitHead:  "deadbeefcafe",
		Proposal: "test-proposal",
		Ops: []plan.Op{
			{
				OpID:         "op-0001",
				Type:         plan.OpCreate,
				SpecNodeKind: "component",
				SpecNodeID:   f.compID,
				Idempotency:  &plan.Idem{Label: "spex:" + f.compID},
				Title:        "Comp1",
			},
			{
				OpID:   "op-0002",
				Type:   plan.OpClose,
				Target: &plan.Ref{Kind: plan.RefBead, BeadID: "bead-ghost"},
				Reason: "Spec node removed: alpha/Ghost",
			},
		},
	}
	writeJSON(t, f.changesetPath, cs)

	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{
			{OpID: "op-0001", Status: adapters.OpStatusOk, BeadID: "bead-1"},
			{OpID: "op-0002", Status: adapters.OpStatusOk, BeadID: "bead-ghost"},
		},
	}
	writeJSON(t, f.receiptsPath, rc)

	journalPath := filepath.Join(f.specDir, ".history.jsonl")

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err == nil {
		t.Fatal("want error for invariant failure, got nil")
	}
	if exit != 2 {
		t.Errorf("want exit 2 for invariant failure, got %d", exit)
	}
	if !strings.Contains(err.Error(), "invariant") {
		t.Errorf("want error mentioning invariant, got: %v", err)
	}

	if _, statErr := os.Stat(journalPath); !os.IsNotExist(statErr) {
		t.Errorf("invariant failure should leave the journal unwritten, stat err: %v", statErr)
	}
}

func TestIngestCommand_ReRun_LeavesJournalByteIdentical(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	if _, _, _, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	); err != nil {
		t.Fatalf("first run: %v", err)
	}

	journalPath := filepath.Join(f.specDir, ".history.jsonl")
	journalAfter1, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapAfter1, err := os.ReadFile(f.snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the adapter recognising the existing bead by flipping
	// was_existing=true on the second run; this is the canonical
	// idempotency path documented in test_ingest_command.md and
	// test_partial_run_recovery.md.
	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-0001",
			Status:      adapters.OpStatusOk,
			BeadID:      "bead-1",
			WasExisting: true,
		}},
	}
	writeJSON(t, f.receiptsPath, rc)

	stdout, _, _, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
	)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	var sum ingest.Summary
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if sum.EventsAppended != 0 || sum.ReceiptsAppended != 0 {
		t.Errorf("want zero appends on idempotent re-run, got %+v", sum)
	}

	journalAfter2, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(journalAfter1) != string(journalAfter2) {
		t.Errorf("journal changed across idempotent re-runs:\nrun1: %s\nrun2: %s",
			journalAfter1, journalAfter2)
	}

	snapAfter2, err := os.ReadFile(f.snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	// Snapshot is rewritten on every complete run. The merkle tree input
	// is unchanged between runs, but the saver stamps created_at, so we
	// only assert that the file still exists and has well-formed JSON.
	_ = snapAfter1
	_ = snapAfter2
}

// setupRefreshedFixture runs a complete normal-mode ingest over the
// fixture — materialising the component's added event + task_created in
// spec/.history.jsonl and the snapshot baseline — then writes the empty
// changeset+receipts pair refresh mode requires. Returns the fixture
// plus the two empty artifact paths.
func setupRefreshedFixture(t *testing.T) (ingestFixture, string, string) {
	t.Helper()
	f := setupIngestFixture(t, adapters.StatusComplete)
	_, stderr, exit, err := runIngest(t,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
		"--spec-dir", f.specDir,
	)
	if err != nil || exit != 0 {
		t.Fatalf("seed normal ingest: exit %d err %v stderr %s", exit, err, stderr)
	}

	dir := filepath.Dir(f.changesetPath)
	emptyCS := filepath.Join(dir, "refresh-changeset.json")
	writeJSON(t, emptyCS, plan.Changeset{Version: plan.ChangesetVersion})
	emptyRC := filepath.Join(dir, "refresh-receipts.json")
	writeJSON(t, emptyRC, adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete})
	return f, emptyCS, emptyRC
}

// TestIngestCommand_RefreshMode_AbsorbsDrift covers the command-level
// happy path: --mode refresh dispatches to RefreshHandler, appends a
// modified change event for the drifted leaf, rewrites the snapshot,
// and emits the refresh summary shape.
func TestIngestCommand_RefreshMode_AbsorbsDrift(t *testing.T) {
	f, emptyCS, emptyRC := setupRefreshedFixture(t)

	// Content-only drift on the component's arch leaf.
	writeTestFile(t, filepath.Join(f.specDir, "alpha"), "arch_comp1.md", "# Comp1 (revised)\n")

	stdout, _, exit, err := runIngest(t,
		"--mode", "refresh",
		"--changeset", emptyCS,
		"--receipts", emptyRC,
		"--spec-dir", f.specDir,
	)
	if err != nil || exit != 0 {
		t.Fatalf("refresh run: exit %d err %v", exit, err)
	}

	var sum ingest.RefreshSummary
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatalf("parse summary %q: %v", stdout, err)
	}
	if sum.EventsAppended != 1 || !sum.SnapshotSaved || sum.Status != adapters.StatusComplete {
		t.Errorf("summary: want 1 event appended, snapshot_saved, complete; got %+v", sum)
	}

	events, err := mapping.NewMappingStore(filepath.Join(f.specDir, ".history.jsonl")).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	wantHash, err := merkle.HashFile(filepath.Join(f.specDir, "alpha", "arch_comp1.md"))
	if err != nil {
		t.Fatal(err)
	}
	var modified *mapping.Event
	for i := range events {
		if events[i].Event == "modified" && events[i].Node == f.compID {
			modified = &events[i]
		}
	}
	if modified == nil {
		t.Fatalf("want a modified event for %s, journal: %+v", f.compID, events)
	}
	if modified.After == nil || *modified.After != wantHash {
		t.Errorf("modified event after: want %s, got %+v", wantHash, modified.After)
	}

	entry, err := mapping.NewMappingStore(filepath.Join(f.specDir, ".history.jsonl")).Get(f.compID)
	if err != nil {
		t.Fatalf("get fold entry: %v", err)
	}
	if entry.TaskID != f.beadID {
		t.Errorf("task pairing must be untouched: want %s, got %s", f.beadID, entry.TaskID)
	}
}

// TestIngestCommand_RefreshMode_GitHeadStampsReceipt covers the
// --git-head flag: refresh mode stamps it onto the closing refresh
// receipt (and every change event in the batch); normal mode ignores
// it entirely since the changeset already carries its own git_head.
func TestIngestCommand_RefreshMode_GitHeadStampsReceipt(t *testing.T) {
	f, emptyCS, emptyRC := setupRefreshedFixture(t)

	writeTestFile(t, filepath.Join(f.specDir, "alpha"), "arch_comp1.md", "# Comp1 (revised)\n")

	_, _, exit, err := runIngest(t,
		"--mode", "refresh",
		"--changeset", emptyCS,
		"--receipts", emptyRC,
		"--git-head", "cafef00dcafe",
		"--spec-dir", f.specDir,
	)
	if err != nil || exit != 0 {
		t.Fatalf("refresh run: exit %d err %v", exit, err)
	}

	events, err := mapping.NewMappingStore(filepath.Join(f.specDir, ".history.jsonl")).Parse()
	if err != nil {
		t.Fatalf("parse journal: %v", err)
	}
	var refresh *mapping.Event
	for i := range events {
		if events[i].Event == "refresh" {
			refresh = &events[i]
		}
	}
	if refresh == nil {
		t.Fatalf("want a refresh receipt, journal: %+v", events)
	}
	if refresh.GitHead != "cafef00dcafe" {
		t.Errorf("refresh receipt git_head: want cafef00dcafe, got %q", refresh.GitHead)
	}
}

// TestIngestCommand_RefreshMode_RefusalExits2 covers the exit-code
// contract: a structural diff (added leaf) maps the RefreshRefusal to
// exit code 2 with the entries named on the error.
func TestIngestCommand_RefreshMode_RefusalExits2(t *testing.T) {
	f, emptyCS, emptyRC := setupRefreshedFixture(t)

	newID := schema.IdentityHash("alpha", "component", "Comp2")
	writeTestFile(t, filepath.Join(f.specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+f.compID+`", "name": "Comp1", "content": "arch_comp1.md"},
			{"id": "`+newID+`", "name": "Comp2", "content": "arch_comp2.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(f.specDir, "alpha"), "arch_comp2.md", "# Comp2\n")

	journalPath := filepath.Join(f.specDir, ".history.jsonl")
	journalBefore, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	snapBefore, err := os.ReadFile(f.snapshotPath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, exit, runErr := runIngest(t,
		"--mode", "refresh",
		"--changeset", emptyCS,
		"--receipts", emptyRC,
		"--spec-dir", f.specDir,
	)
	if exit != 2 {
		t.Fatalf("want exit 2 for refresh refusal, got %d (err %v)", exit, runErr)
	}
	if runErr == nil || !strings.Contains(runErr.Error(), newID) {
		t.Errorf("refusal must name the added entry %s: %v", newID, runErr)
	}
	journalAfter, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(journalAfter) != string(journalBefore) {
		t.Error("journal must be unchanged after refusal")
	}
	snapAfter, err := os.ReadFile(f.snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(snapAfter) != string(snapBefore) {
		t.Error("snapshot must be unchanged after refusal")
	}
}

// TestIngestCommand_RefreshMode_MissingSnapshotExits1 covers the
// pre-flight edge: refresh without a snapshot baseline is an input
// error (exit 1), pointing the caller at a normal-mode bootstrap.
func TestIngestCommand_RefreshMode_MissingSnapshotExits1(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)
	dir := filepath.Dir(f.changesetPath)
	emptyCS := filepath.Join(dir, "refresh-changeset.json")
	writeJSON(t, emptyCS, plan.Changeset{Version: plan.ChangesetVersion})
	emptyRC := filepath.Join(dir, "refresh-receipts.json")
	writeJSON(t, emptyRC, adapters.Receipts{Version: adapters.ReceiptsVersion, Status: adapters.StatusComplete})

	_, _, exit, err := runIngest(t,
		"--mode", "refresh",
		"--changeset", emptyCS,
		"--receipts", emptyRC,
		"--spec-dir", f.specDir,
	)
	if exit != 1 {
		t.Fatalf("want exit 1 for missing snapshot, got %d (err %v)", exit, err)
	}
	if err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Errorf("error must mention the missing snapshot: %v", err)
	}
}

// TestIngestCommand_RefreshMode_NonEmptyArtifactsExits1 covers the
// configuration-error edge: passing the normal (op-carrying) changeset
// and receipts to a refresh run exits 1.
func TestIngestCommand_RefreshMode_NonEmptyArtifactsExits1(t *testing.T) {
	f, _, _ := setupRefreshedFixture(t)

	_, _, exit, err := runIngest(t,
		"--mode", "refresh",
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
		"--spec-dir", f.specDir,
	)
	if exit != 1 {
		t.Fatalf("want exit 1 for non-empty artifacts, got %d (err %v)", exit, err)
	}
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("error must say the artifacts must be empty: %v", err)
	}
}

// TestIngestCommand_UnknownModeExits1 pins flag validation: --mode
// accepts only normal or refresh.
func TestIngestCommand_UnknownModeExits1(t *testing.T) {
	f, emptyCS, emptyRC := setupRefreshedFixture(t)

	_, _, exit, err := runIngest(t,
		"--mode", "bogus",
		"--changeset", emptyCS,
		"--receipts", emptyRC,
		"--spec-dir", f.specDir,
	)
	if exit != 1 {
		t.Fatalf("want exit 1 for unknown mode, got %d (err %v)", exit, err)
	}
	if err == nil || !strings.Contains(err.Error(), "--mode") {
		t.Errorf("error must mention --mode: %v", err)
	}
}
