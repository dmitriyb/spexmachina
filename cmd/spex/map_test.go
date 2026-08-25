package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// writeTestJournal writes the journal at dir's resolved .spex/ location
// with the given raw lines, one per line.
func writeTestJournal(t *testing.T, dir string, lines []string) {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	stateDir := projectStateDir(dir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", stateDir, err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, lifecycle.JournalFileName), []byte(content), 0644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

// seedMapSnapshot writes an empty-tree snapshot at dir's resolved .spex/
// location, marking dir's project as initialised for the lifecycle
// pre-flight map.go's commands run — same as cmd/spex/diff.go's own tests
// seed before exercising a command that expects to run past the
// uninitialised-project refusal. It seeds the snapshot alone, deliberately:
// TestFR_MapList_NoJournal relies on a project whose snapshot exists but
// whose journal does not, to exercise the broken-project branch.
func seedMapSnapshot(t *testing.T, dir string) {
	t.Helper()
	stateDir := projectStateDir(dir)
	if err := merkle.Save(merkle.EmptyTree(), filepath.Join(stateDir, lifecycle.SnapshotFileName), time.Now()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
}

func TestFR_MapGet_ByIdentityHash(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"ActionClassifier","node_type":"component","module":"impact","before":null,"after":"h1","git_head":"cafe1234","proposal":"prop1"}`,
		`{"event":"task_created","for":"e1","task_id":"spexmachina-abc"}`,
	})

	out, err := runSpex(t, "map", "get", "--spec-dir", dir, "a1b2c3d4e5f6")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if got["node"] != "a1b2c3d4e5f6" {
		t.Errorf("want node a1b2c3d4e5f6, got %v", got["node"])
	}
	if got["task_id"] != "spexmachina-abc" {
		t.Errorf("want task_id spexmachina-abc, got %v", got["task_id"])
	}
	if got["name"] != "ActionClassifier" {
		t.Errorf("want name ActionClassifier, got %v", got["name"])
	}
}

func TestFR_MapGet_ByTaskID(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"ActionClassifier","node_type":"component","module":"impact","before":null,"after":"h1","git_head":"cafe1234","proposal":"prop1"}`,
		`{"event":"task_created","for":"e1","task_id":"spexmachina-abc"}`,
	})

	byHash, err := runSpex(t, "map", "get", "--spec-dir", dir, "a1b2c3d4e5f6")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	byTask, err := runSpex(t, "map", "get", "--spec-dir", dir, "spexmachina-abc")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if byHash != byTask {
		t.Fatalf("hash and task-id lookups should be identical: %q vs %q", byHash, byTask)
	}
}

func TestFR_MapGet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, nil)

	_, err := runSpex(t, "map", "get", "--spec-dir", dir, "deadbeefdead")
	if err == nil {
		t.Fatal("want error for unknown key, got nil")
	}
}

func TestFR_MapGet_IntegerKeyGone(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"ActionClassifier","node_type":"component","module":"impact","before":null,"after":"h1","git_head":"cafe1234","proposal":"prop1"}`,
		`{"event":"task_created","for":"e1","task_id":"spexmachina-abc"}`,
	})

	// "1" is not 12-hex-shaped, so it is tried as a task id — and none
	// exists — rather than falling back to any integer record-id parse.
	_, err := runSpex(t, "map", "get", "--spec-dir", dir, "1")
	if err == nil {
		t.Fatal("want error for integer-shaped key, got nil")
	}
}

func TestFR_MapList_FoldedLinkage(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"Comp1","node_type":"component","module":"m","before":null,"after":"h1","git_head":"g1","proposal":"p1"}`,
		`{"event":"task_created","for":"e1","task_id":"task-1"}`,
		`{"event":"removed","eid":"e2","node":"dddddddddddd","name":"CompY","node_type":"component","module":"modD","before":"h1","after":null,"git_head":"g2","proposal":"remove-prop"}`,
		`{"event":"task_closed","for":"e2","task_id":"task-Y"}`,
		`{"event":"task_created","for":"e2","task_id":"br-cleanup"}`,
		`{"event":"registered","eid":"e9","proposal":"2026-08-11-slug","git_head":"g9"}`,
		`{"event":"task_created","for":"e9","task_id":"epic-1"}`,
		`{"event":"task_created","proposal":"legacy-slug","task_id":"epic-legacy"}`,
	})

	out, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output should be valid JSON array: %v\noutput: %s", err, out)
	}
	if len(entries) != 4 {
		t.Fatalf("want 4 entries, got %d: %s", len(entries), out)
	}

	var removed, registeredEpic, legacyEpic map[string]any
	for _, e := range entries {
		switch {
		case e["node"] == "dddddddddddd":
			removed = e
		case e["proposal"] == "2026-08-11-slug":
			registeredEpic = e
		case e["proposal"] == "legacy-slug":
			legacyEpic = e
		}
	}
	if removed == nil {
		t.Fatalf("want an entry for the removed node, got %s", out)
	}
	if removed["removed"] != true {
		t.Errorf("removed node with a cleanup task should still report removed=true, got %v", removed["removed"])
	}
	if removed["task_id"] != "br-cleanup" {
		t.Errorf("removed node should adopt the cleanup task id, got %v", removed["task_id"])
	}

	if registeredEpic == nil {
		t.Fatalf("want an entry for the registered-sourced epic, got %s", out)
	}
	if registeredEpic["task_id"] != "epic-1" {
		t.Errorf("registered epic task_id: want epic-1, got %v", registeredEpic["task_id"])
	}
	if registeredEpic["git_head"] != "g9" {
		t.Errorf("registered epic git_head: want g9, got %v", registeredEpic["git_head"])
	}
	if _, hasNode := registeredEpic["node"]; hasNode {
		t.Errorf("registered epic should carry no node field, got %v", registeredEpic["node"])
	}

	if legacyEpic == nil {
		t.Fatalf("want an entry for the legacy slug-keyed epic, got %s", out)
	}
	if legacyEpic["task_id"] != "epic-legacy" {
		t.Errorf("legacy epic task_id: want epic-legacy, got %v", legacyEpic["task_id"])
	}
	if _, hasNode := legacyEpic["node"]; hasNode {
		t.Errorf("legacy epic should carry no node field, got %v", legacyEpic["node"])
	}

	// Both epic entries must also surface individually through `map get`,
	// by their task id, with the same shape `map list` folded them into.
	registeredGet, err := runSpex(t, "map", "get", "--spec-dir", dir, "epic-1")
	if err != nil {
		t.Fatalf("map get epic-1: want no error, got %v", err)
	}
	var registeredGot map[string]any
	if err := json.Unmarshal([]byte(registeredGet), &registeredGot); err != nil {
		t.Fatalf("map get epic-1: output should be valid JSON: %v\noutput: %s", err, registeredGet)
	}
	if registeredGot["proposal"] != "2026-08-11-slug" || registeredGot["git_head"] != "g9" || registeredGot["task_id"] != "epic-1" {
		t.Errorf("map get epic-1: want the registered-sourced epic entry, got %+v", registeredGot)
	}
	if _, hasNode := registeredGot["node"]; hasNode {
		t.Errorf("map get epic-1: should carry no node field, got %v", registeredGot["node"])
	}

	legacyGet, err := runSpex(t, "map", "get", "--spec-dir", dir, "epic-legacy")
	if err != nil {
		t.Fatalf("map get epic-legacy: want no error, got %v", err)
	}
	var legacyGot map[string]any
	if err := json.Unmarshal([]byte(legacyGet), &legacyGot); err != nil {
		t.Fatalf("map get epic-legacy: output should be valid JSON: %v\noutput: %s", err, legacyGet)
	}
	if legacyGot["proposal"] != "legacy-slug" || legacyGot["task_id"] != "epic-legacy" {
		t.Errorf("map get epic-legacy: want the legacy slug-keyed epic entry, got %+v", legacyGot)
	}
	if _, hasNode := legacyGot["node"]; hasNode {
		t.Errorf("map get epic-legacy: should carry no node field, got %v", legacyGot["node"])
	}
}

// TestFR_MapList_NoProjectState covers test_map_command.md's "No journal
// exists" edge case, the "directory with no project state at all" branch:
// the pre-flight refuses before the store is consulted, naming spex init
// and exiting with the not-a-spex-project code.
func TestFR_MapList_NoProjectState(t *testing.T) {
	dir := t.TempDir()

	_, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error when no project state exists, got nil")
	}
	if exitCodeOf(err) != exitNotAProject {
		t.Fatalf("want exit code %d, got %d (%v)", exitNotAProject, exitCodeOf(err), err)
	}
	if !strings.Contains(err.Error(), "spex init") {
		t.Fatalf("want error naming 'spex init', got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(projectStateDir(dir), lifecycle.JournalFileName)); statErr == nil {
		t.Fatal("map list must not create the journal file")
	}
}

// TestFR_MapList_NoJournal covers the other branch of the same edge case:
// a state directory (a snapshot exists — the interim initialised signal)
// missing its journal is a broken project, not an empty answer. The old
// empty-array response survives only at the MappingStore library layer,
// which this pre-flight now sits in front of.
func TestFR_MapList_NoJournal(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)

	_, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error when the journal is missing, got nil")
	}
	if !strings.Contains(err.Error(), "spex doctor") {
		t.Fatalf("want error naming 'spex doctor', got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(projectStateDir(dir), lifecycle.JournalFileName)); statErr == nil {
		t.Fatal("map list must not create the journal file")
	}
}

func TestFR_MapList_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"aaaaaaaaaaaa","name":"Foo","node_type":"component","module":"m","before":null,"after":"h1","git_head":"g1","proposal":"p1"}`,
		`{"event":"task_created","for":"e1","task_id":"t1"}`,
		`not-json`,
	})

	_, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for malformed journal line, got nil")
	}
	if !strings.Contains(err.Error(), "map:") || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("want error naming the file and line, got %v", err)
	}
	if !strings.Contains(err.Error(), "spex doctor") {
		t.Fatalf("want the pre-flight's broken-project error naming 'spex doctor', got %v", err)
	}
	if exitCodeOf(err) != exitNotAProject {
		t.Fatalf("want exit code %d, got %d (%v)", exitNotAProject, exitCodeOf(err), err)
	}
}

// setupMapContextTestSpec creates a spec directory with project.json, one
// module declaring a live component, and a journal pairing that component
// with a task plus recording one removed node's biography.
func setupMapContextTestSpec(t *testing.T) (specDir string) {
	t.Helper()
	dir := t.TempDir()
	seedMapSnapshot(t, dir)

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "aabbccddeeff", "name": "Comp1", "content": "arch_comp1.md"},
			{"id": "ffeeddccbbaa", "name": "Comp2", "content": "arch_comp2.md", "uses": ["aabbccddeeff"]}
		],
		"test_sections": [
			{"id": "333333333333", "name": "Comp1 tests", "content": "test_comp1.md", "describes": ["aabbccddeeff"]}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n")
	writeTestFile(t, alphaDir, "arch_comp2.md", "# Comp2\n")
	writeTestFile(t, alphaDir, "test_comp1.md", "# Comp1 tests\n")

	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"aabbccddeeff","name":"Comp1","node_type":"component","module":"alpha","before":null,"after":"h1","git_head":"cafe1234","proposal":"prop1"}`,
		`{"event":"task_created","for":"e1","task_id":"test-abc"}`,
		`{"event":"added","eid":"e2","node":"dddddddddddd","name":"Retired","node_type":"component","module":"alpha","before":null,"after":"h2","git_head":"babe0000","proposal":"prop2"}`,
		`{"event":"task_created","for":"e2","task_id":"test-def"}`,
		`{"event":"removed","eid":"e3","node":"dddddddddddd","name":"Retired","node_type":"component","module":"alpha","before":"h2","after":null,"git_head":"cafe5678","proposal":"prop3"}`,
		`{"event":"added","eid":"e4","node":"ffeeddccbbaa","name":"Comp2","node_type":"component","module":"alpha","before":null,"after":"h4","git_head":"feed0001","proposal":"prop4"}`,
	})

	return dir
}

func TestFR_MapContext_LiveNode(t *testing.T) {
	specDir := setupMapContextTestSpec(t)

	out, err := runSpex(t, "map", "context", "--spec-dir", specDir, "aabbccddeeff")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result mapping.ContextResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Removed {
		t.Fatal("want live result, got Removed=true")
	}
	if result.ArchFile == "" {
		t.Error("want non-empty arch_file")
	}
	if result.ModuleFile == "" {
		t.Error("want non-empty module_file")
	}
	if len(result.TestFiles) == 0 {
		t.Error("want non-empty test_files")
	}
	// The event bracket rides alongside the file set, off the node's latest
	// task-bearing journal event — an added, so before_head is empty.
	if result.Eid != "e1" || result.Event != "added" {
		t.Errorf("want bracket eid=e1 event=added, got eid=%q event=%q", result.Eid, result.Event)
	}
	if result.BeforeHead != "" {
		t.Errorf("want empty before_head for an added event, got %q", result.BeforeHead)
	}
	if result.AfterHead != "cafe1234" {
		t.Errorf("want after_head cafe1234, got %q", result.AfterHead)
	}
}

func TestFR_MapContext_LiveNode_NoTaskBearingEvent(t *testing.T) {
	specDir := setupMapContextTestSpec(t)

	// Comp2 (ffeeddccbbaa) has a change event in the journal but no
	// task_created receipt referencing it, so it carries no task-bearing
	// event — the file set still resolves normally, but the bracket is null.
	out, err := runSpex(t, "map", "context", "--spec-dir", specDir, "ffeeddccbbaa")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result mapping.ContextResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Removed {
		t.Fatal("want live result, got Removed=true")
	}
	if result.ArchFile == "" {
		t.Error("want non-empty arch_file")
	}
	if result.Eid != "" || result.Event != "" || result.BeforeHead != "" || result.AfterHead != "" {
		t.Errorf("want null bracket fields for a node with no task-bearing event, got %+v", result)
	}
}

func TestFR_MapContext_RemovedNode(t *testing.T) {
	specDir := setupMapContextTestSpec(t)

	out, err := runSpex(t, "map", "context", "--spec-dir", specDir, "dddddddddddd")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result mapping.ContextResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if !result.Removed {
		t.Fatal("want removed result, got Removed=false")
	}
	if result.Name != "Retired" || result.NodeType != "component" || result.Module != "alpha" {
		t.Errorf("want biography fields, got %+v", result)
	}
	if result.Proposal != "prop3" {
		t.Errorf("want removing proposal prop3, got %q", result.Proposal)
	}
	if result.Eid != "e3" || result.Event != "removed" {
		t.Errorf("want bracket eid=e3 event=removed, got eid=%q event=%q", result.Eid, result.Event)
	}
	if result.AfterHead != "cafe5678" || result.BeforeHead != "babe0000" {
		t.Errorf("want git_head refs bracketing the final change, got before=%q after=%q", result.BeforeHead, result.AfterHead)
	}
}

func TestFR_MapContext_UnknownKey(t *testing.T) {
	specDir := setupMapContextTestSpec(t)

	_, err := runSpex(t, "map", "context", "--spec-dir", specDir, "deadbeefdead")
	if err == nil {
		t.Fatal("want error for unknown key, got nil")
	}
}

// buildSpexBinary compiles the spex CLI to a fresh subdirectory of the
// repo's gitignored bin/ dir (not the process temp dir, which may be
// mounted noexec) and returns the binary path. True subprocesses, each
// with their own stdout, are required to exercise concurrent CLI
// invocations: the RunE handlers encode straight to the process-global
// os.Stdout, which in-process concurrent goroutines would race on.
func buildSpexBinary(t *testing.T) string {
	t.Helper()
	binRoot := filepath.Join("..", "..", "bin")
	if err := os.MkdirAll(binRoot, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", binRoot, err)
	}
	binDir, err := os.MkdirTemp(binRoot, "spex-test-")
	if err != nil {
		t.Fatalf("mkdir temp bin dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(binDir) })

	binPath := filepath.Join(binDir, "spex")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build spex binary: %v\n%s", err, out)
	}
	return binPath
}

func TestFR_MapCommand_ConcurrentInvocations(t *testing.T) {
	dir := t.TempDir()
	seedMapSnapshot(t, dir)
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"Comp1","node_type":"component","module":"m","before":null,"after":"h1","git_head":"g1","proposal":"p1"}`,
		`{"event":"task_created","for":"e1","task_id":"task-1"}`,
		`{"event":"added","eid":"e2","node":"0f1e2d3c4b5a","name":"Comp2","node_type":"component","module":"m","before":null,"after":"h2","git_head":"g2","proposal":"p1"}`,
		`{"event":"task_created","for":"e2","task_id":"task-2"}`,
	})

	binPath := buildSpexBinary(t)
	wantTask := map[string]string{"a1b2c3d4e5f6": "task-1", "0f1e2d3c4b5a": "task-2"}
	keys := []string{"a1b2c3d4e5f6", "0f1e2d3c4b5a"}

	var wg sync.WaitGroup
	outs := make([]string, len(keys))
	errs := make([]error, len(keys))
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			out, err := exec.Command(binPath, "map", "get", "--spec-dir", dir, key).Output()
			outs[i] = string(out)
			errs[i] = err
		}(i, key)
	}
	wg.Wait()

	for i, key := range keys {
		if errs[i] != nil {
			t.Fatalf("key %s: want no error, got %v", key, errs[i])
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(outs[i]), &got); err != nil {
			t.Fatalf("key %s: output should be valid JSON: %v\noutput: %s", key, err, outs[i])
		}
		if got["node"] != key {
			t.Errorf("key %s: want node %v, got %v", key, key, got["node"])
		}
		if got["task_id"] != wantTask[key] {
			t.Errorf("key %s: want task_id %s, got %v", key, wantTask[key], got["task_id"])
		}
	}
}

func TestFR_MapCommand_NoSubcommand(t *testing.T) {
	_, err := runSpex(t, "map")
	// cobra prints help for group commands with no subcommand — no error
	// but also no action taken
	if err != nil {
		t.Fatalf("map with no subcommand should not error, got %v", err)
	}
}
