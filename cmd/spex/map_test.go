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

// setupMapContextTestSpec creates a spec directory with a populated
// .bead-map.json — spex map context still resolves through the retired
// mapping.Store/Record until ContextResolver migrates onto MappingStore
// (spexmachina-y0wc.20).
func setupMapContextTestSpec(t *testing.T) (specDir string, mapFilePath string) {
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
		]
	}`)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n")
	writeTestFile(t, alphaDir, "arch_comp2.md", "# Comp2\n")

	mapPath := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(mapPath)
	if _, err := store.Create(mapping.Record{
		SpecNodeID:  "aabbccddeeff",
		BeadID:      "test-abc",
		BeadType:    "task",
		Module:      "alpha",
		Component:   "Comp1",
		ContentFile: "spec/alpha/arch_comp1.md",
		SpecHash:    "hash1",
		BeadStatus:  "closed",
	}); err != nil {
		t.Fatalf("create record: %v", err)
	}

	return dir, mapPath
}

func TestFR_MapContext_ValidRecord(t *testing.T) {
	specDir, mapFile := setupMapContextTestSpec(t)

	out, err := runSpex(t, "map", "context", "--map-file", mapFile, "--spec-dir", specDir, "1")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result mapping.ContextResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Record.ID != 1 {
		t.Errorf("want record ID 1, got %d", result.Record.ID)
	}
	if result.ArchFile == "" {
		t.Error("want non-empty arch_file")
	}
	if result.ModuleFile == "" {
		t.Error("want non-empty module_file")
	}
}

func TestFR_MapContext_UnknownRecord(t *testing.T) {
	specDir, mapFile := setupMapContextTestSpec(t)

	_, err := runSpex(t, "map", "context", "--map-file", mapFile, "--spec-dir", specDir, "999")
	if err == nil {
		t.Fatal("want error for unknown record, got nil")
	}
}

func TestFR_MapContext_InvalidID(t *testing.T) {
	_, mapFile := setupMapContextTestSpec(t)

	_, err := runSpex(t, "map", "context", "--map-file", mapFile, "notanumber")
	if err == nil {
		t.Fatal("want error for invalid ID, got nil")
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
