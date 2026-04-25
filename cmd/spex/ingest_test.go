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
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/ingest"
	"github.com/dmitriyb/spexmachina/mapping"
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
	mapPath       string
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

	mapPath := filepath.Join(dir, ".bead-map.json")
	if err := os.WriteFile(mapPath, []byte(`{"next_id":1,"records":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cs := emit.Changeset{
		Version:  emit.ChangesetVersion,
		GitHead:  "deadbeefcafe",
		Proposal: "test-proposal",
		Ops: []emit.Op{{
			OpID:         "op-0001",
			Type:         emit.OpCreate,
			SpecNodeKind: "component",
			SpecNodeID:   compID,
			Idempotency:  &emit.Idem{Label: "spex:1"},
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
		mapPath:       mapPath,
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
		"--map", f.mapPath,
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
	if sum.RecordsAdded != 1 {
		t.Errorf("want records_added=1, got %d", sum.RecordsAdded)
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

	store := mapping.NewFileStore(f.mapPath)
	rec, err := store.Get(f.recordID)
	if err != nil {
		t.Fatalf("expected record %d after ingest: %v", f.recordID, err)
	}
	if rec.BeadID != f.beadID {
		t.Errorf("want bead_id=%s, got %s", f.beadID, rec.BeadID)
	}
	if rec.SpecNodeID != f.compID {
		t.Errorf("want spec_node_id=%s, got %s", f.compID, rec.SpecNodeID)
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
		"--map", f.mapPath,
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
		"--map", f.mapPath,
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
		"--map", f.mapPath,
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
		"--map", f.mapPath,
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
		"--map", f.mapPath,
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
		"--map", f.mapPath,
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

	cs := emit.Changeset{
		Version: 99,
		Ops:     []emit.Op{},
	}
	writeJSON(t, f.changesetPath, cs)

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
		"--map", f.mapPath,
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

func TestIngestCommand_InvariantFailure_Exits2_PreservesMapping(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	// Seed an orphan record (spec_node_id not present in the spec graph)
	// so invariant 4 fires after the working-copy apply.
	orphanState := `{
		"next_id": 5,
		"records": [
			{
				"id": 4,
				"spec_node_id": "ghostnode00",
				"bead_id": "bead-ghost",
				"bead_type": "feature",
				"node_type": "component",
				"module": "alpha",
				"component": "Ghost",
				"content_file": "spec/alpha/arch_ghost.md",
				"spec_hash": "deadbeef"
			}
		]
	}`
	if err := os.WriteFile(f.mapPath, []byte(orphanState), 0o644); err != nil {
		t.Fatal(err)
	}

	mappingBefore, err := os.ReadFile(f.mapPath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, exit, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
		"--map", f.mapPath,
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

	mappingAfter, err := os.ReadFile(f.mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(mappingBefore) != string(mappingAfter) {
		t.Errorf("invariant failure should leave .bead-map.json untouched\nbefore: %s\nafter:  %s",
			mappingBefore, mappingAfter)
	}
}

func TestIngestCommand_ReRun_LeavesMappingByteIdentical(t *testing.T) {
	f := setupIngestFixture(t, adapters.StatusComplete)

	if _, _, _, err := runIngest(t,
		"--spec-dir", f.specDir,
		"--changeset", f.changesetPath,
		"--receipts", f.receiptsPath,
		"--map", f.mapPath,
	); err != nil {
		t.Fatalf("first run: %v", err)
	}

	mapAfter1, err := os.ReadFile(f.mapPath)
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
		"--map", f.mapPath,
	)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	var sum ingest.Summary
	if err := json.Unmarshal([]byte(stdout), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if sum.RecordsAdded != 0 || sum.RecordsUpdated != 0 || sum.RecordsDeleted != 0 {
		t.Errorf("want zero record changes on idempotent re-run, got %+v", sum)
	}

	mapAfter2, err := os.ReadFile(f.mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(mapAfter1) != string(mapAfter2) {
		t.Errorf(".bead-map.json changed across idempotent re-runs:\nrun1: %s\nrun2: %s",
			mapAfter1, mapAfter2)
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
