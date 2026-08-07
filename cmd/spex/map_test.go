package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// writeTestJournal writes spec/.history.jsonl under dir with the given
// raw lines, one per line.
func writeTestJournal(t *testing.T, dir string, lines []string) {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	writeTestFile(t, dir, ".history.jsonl", content)
}

func TestFR_MapGet_ByIdentityHash(t *testing.T) {
	dir := t.TempDir()
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
	writeTestJournal(t, dir, nil)

	_, err := runSpex(t, "map", "get", "--spec-dir", dir, "deadbeefdead")
	if err == nil {
		t.Fatal("want error for unknown key, got nil")
	}
}

func TestFR_MapList_FoldedLinkage(t *testing.T) {
	dir := t.TempDir()
	writeTestJournal(t, dir, []string{
		`{"event":"added","eid":"e1","node":"a1b2c3d4e5f6","name":"Comp1","node_type":"component","module":"m","before":null,"after":"h1","git_head":"g1","proposal":"p1"}`,
		`{"event":"task_created","for":"e1","task_id":"task-1"}`,
		`{"event":"removed","eid":"e2","node":"dddddddddddd","name":"CompY","node_type":"component","module":"modD","before":"h1","after":null,"git_head":"g2","proposal":"remove-prop"}`,
		`{"event":"task_closed","for":"e2","task_id":"task-Y"}`,
		`{"event":"task_created","for":"e2","task_id":"br-cleanup"}`,
	})

	out, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output should be valid JSON array: %v\noutput: %s", err, out)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d: %s", len(entries), out)
	}

	var removed map[string]any
	for _, e := range entries {
		if e["node"] == "dddddddddddd" {
			removed = e
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
}

func TestFR_MapList_NoJournal(t *testing.T) {
	dir := t.TempDir()

	out, err := runSpex(t, "map", "list", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("want no error when no journal exists, got %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("output should be valid JSON array: %v\noutput: %s", err, out)
	}
	if len(entries) != 0 {
		t.Fatalf("want empty array, got %d entries", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, ".history.jsonl")); err == nil {
		t.Fatal("map list must not create the journal file")
	}
}

func TestFR_MapList_MalformedLine(t *testing.T) {
	dir := t.TempDir()
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
}

// setupMapContextTestSpec creates a spec directory with project.json, one
// module declaring a live component, and a journal pairing that component
// with a task plus recording one removed node's biography.
func setupMapContextTestSpec(t *testing.T) (specDir string) {
	t.Helper()
	dir := t.TempDir()

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

func TestFR_MapCommand_NoSubcommand(t *testing.T) {
	_, err := runSpex(t, "map")
	// cobra prints help for group commands with no subcommand — no error
	// but also no action taken
	if err != nil {
		t.Fatalf("map with no subcommand should not error, got %v", err)
	}
}
