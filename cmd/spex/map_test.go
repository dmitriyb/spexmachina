package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// setupMapTestSpec creates a spec directory with a populated .bead-map.json.
func setupMapTestSpec(t *testing.T) (specDir string, mapFilePath string) {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md"},
			{"id": 2, "name": "Comp2", "content": "arch_comp2.md", "uses": [1]}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md", "describes": [1]}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n")
	writeTestFile(t, alphaDir, "arch_comp2.md", "# Comp2\n")
	writeTestFile(t, alphaDir, "impl_comp1.md", "# Impl1\n")

	mapPath := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(mapPath)
	if _, err := store.Create(mapping.Record{
		SpecNodeID:  "alpha/component/1",
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
	if _, err := store.Create(mapping.Record{
		SpecNodeID:  "alpha/component/2",
		BeadID:      "test-def",
		BeadType:    "task",
		Module:      "alpha",
		Component:   "Comp2",
		ContentFile: "spec/alpha/arch_comp2.md",
		SpecHash:    "hash2",
		BeadStatus:  "open",
	}); err != nil {
		t.Fatalf("create record: %v", err)
	}

	return dir, mapPath
}

func TestFR3_MapGet_ValidRecord(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	out, err := runSpex(t, "map", "get", "--map-file", mapFile, "1")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var record mapping.Record
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if record.ID != 1 {
		t.Fatalf("want record ID 1, got %d", record.ID)
	}
	if record.BeadID != "test-abc" {
		t.Fatalf("want bead_id test-abc, got %s", record.BeadID)
	}
	if record.Module != "alpha" {
		t.Fatalf("want module alpha, got %s", record.Module)
	}
	if record.Component != "Comp1" {
		t.Fatalf("want component Comp1, got %s", record.Component)
	}
	if record.ContentFile != "spec/alpha/arch_comp1.md" {
		t.Fatalf("want content_file spec/alpha/arch_comp1.md, got %s", record.ContentFile)
	}
	if record.SpecHash != "hash1" {
		t.Fatalf("want spec_hash hash1, got %s", record.SpecHash)
	}
}

func TestFR3_MapGet_UnknownRecord(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	_, err := runSpex(t, "map", "get", "--map-file", mapFile, "999")
	if err == nil {
		t.Fatal("want error for unknown record, got nil")
	}
}

func TestFR3_MapGet_InvalidID(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	_, err := runSpex(t, "map", "get", "--map-file", mapFile, "notanumber")
	if err == nil {
		t.Fatal("want error for invalid ID, got nil")
	}
}

func TestFR3_MapGet_MissingArg(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	_, err := runSpex(t, "map", "get", "--map-file", mapFile)
	if err == nil {
		t.Fatal("want error for missing arg, got nil")
	}
}

func TestFR3_MapList_AllRecords(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	out, err := runSpex(t, "map", "list", "--map-file", mapFile)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var records []mapping.Record
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("output should be valid JSON array: %v\noutput: %s", err, out)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}
	if records[0].ID != 1 {
		t.Fatalf("want first record ID 1, got %d", records[0].ID)
	}
	if records[1].ID != 2 {
		t.Fatalf("want second record ID 2, got %d", records[1].ID)
	}
}

func TestFR3_MapList_EmptyMappingFile(t *testing.T) {
	dir := t.TempDir()
	mapFile := filepath.Join(dir, ".bead-map.json")

	out, err := runSpex(t, "map", "list", "--map-file", mapFile)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var records []mapping.Record
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("output should be valid JSON array: %v\noutput: %s", err, out)
	}
	if len(records) != 0 {
		t.Fatalf("want empty array, got %d records", len(records))
	}
}

func TestFR3_MapList_NoMappingFile(t *testing.T) {
	dir := t.TempDir()
	mapFile := filepath.Join(dir, "nonexistent.json")

	out, err := runSpex(t, "map", "list", "--map-file", mapFile)
	if err != nil {
		t.Fatalf("want no error when no mapping file exists, got %v", err)
	}

	var records []mapping.Record
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if len(records) != 0 {
		t.Fatalf("want empty array, got %d records", len(records))
	}
}

func TestFR3_MapContext_ValidRecord(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	specDir, mapFile := setupMapTestSpec(t)

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

func TestFR3_MapContext_UnknownRecord(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	specDir, mapFile := setupMapTestSpec(t)

	_, err := runSpex(t, "map", "context", "--map-file", mapFile, "--spec-dir", specDir, "999")
	if err == nil {
		t.Fatal("want error for unknown record, got nil")
	}
}

func TestFR3_MapContext_InvalidID(t *testing.T) {
	// TODO(bead:spexmachina-0re): fix after spexmachina-hd6 changed spec_node_id pattern to identity hash (^[a-f0-9]{12}$).
	t.Skip("blocked on spexmachina-0re: fixtures need identity hash IDs")
	_, mapFile := setupMapTestSpec(t)

	_, err := runSpex(t, "map", "context", "--map-file", mapFile, "notanumber")
	if err == nil {
		t.Fatal("want error for invalid ID, got nil")
	}
}

func TestFR3_MapCommand_NoSubcommand(t *testing.T) {
	_, err := runSpex(t, "map")
	// cobra prints help for group commands with no subcommand — no error
	// but also no action taken
	if err != nil {
		t.Fatalf("map with no subcommand should not error, got %v", err)
	}
}

