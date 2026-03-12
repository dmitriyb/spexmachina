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

	// Create .bead-map.json with two records.
	mapPath := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(mapPath)
	store.Create(mapping.Record{
		SpecNodeID:  "alpha/component/1",
		BeadID:      "test-abc",
		Module:      "alpha",
		Component:   "Comp1",
		ContentFile: "spec/alpha/arch_comp1.md",
		SpecHash:    "hash1",
		BeadStatus:  "closed",
	})
	store.Create(mapping.Record{
		SpecNodeID:  "alpha/component/2",
		BeadID:      "test-def",
		Module:      "alpha",
		Component:   "Comp2",
		ContentFile: "spec/alpha/arch_comp2.md",
		SpecHash:    "hash2",
		BeadStatus:  "open",
	})

	return dir, mapPath
}

func TestFR3_MapGet_ValidRecord(t *testing.T) {
	_, mapFile := setupMapTestSpec(t)

	out := captureStdout(t, func() {
		code := runMapGet([]string{"-map-file", mapFile, "1"})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

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
	_, mapFile := setupMapTestSpec(t)

	code := runMapGet([]string{"-map-file", mapFile, "999"})
	if code != 1 {
		t.Fatalf("want exit 1 for unknown record, got %d", code)
	}
}

func TestFR3_MapGet_InvalidID(t *testing.T) {
	_, mapFile := setupMapTestSpec(t)

	code := runMapGet([]string{"-map-file", mapFile, "notanumber"})
	if code != 1 {
		t.Fatalf("want exit 1 for invalid ID, got %d", code)
	}
}

func TestFR3_MapGet_MissingArg(t *testing.T) {
	_, mapFile := setupMapTestSpec(t)

	code := runMapGet([]string{"-map-file", mapFile})
	if code != 1 {
		t.Fatalf("want exit 1 for missing arg, got %d", code)
	}
}

func TestFR3_MapList_AllRecords(t *testing.T) {
	_, mapFile := setupMapTestSpec(t)

	out := captureStdout(t, func() {
		code := runMapList([]string{"-map-file", mapFile})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

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

	out := captureStdout(t, func() {
		code := runMapList([]string{"-map-file", mapFile})
		if code != 0 {
			t.Fatalf("want exit 0, got %d", code)
		}
	})

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

	out := captureStdout(t, func() {
		code := runMapList([]string{"-map-file", mapFile})
		if code != 0 {
			t.Fatalf("want exit 0 when no mapping file exists, got %d", code)
		}
	})

	var records []mapping.Record
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if len(records) != 0 {
		t.Fatalf("want empty array, got %d records", len(records))
	}
}

func TestFR3_Check_ReadyBead(t *testing.T) {
	specDir, mapFile := setupMapTestSpec(t)

	// Update record 1 hash to match the actual merkle tree hash.
	spec, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		t.Fatalf("build spec graph: %v", err)
	}
	actualHash, err := spec.NodeHash("alpha/component/1")
	if err != nil {
		t.Fatalf("get node hash: %v", err)
	}
	store := mapping.NewFileStore(mapFile)
	if err := store.UpdateSpecHash(1, actualHash); err != nil {
		t.Fatalf("update hash: %v", err)
	}

	// Comp1 has no uses, so it should be ready.
	out := captureStdout(t, func() {
		code := runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "test-abc"})
		if code != 0 {
			t.Fatalf("want exit 0 for ready bead, got %d", code)
		}
	})

	var result mapping.PreflightResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Status != "ready" {
		t.Fatalf("want status ready, got %s", result.Status)
	}
	if result.Record.BeadID != "test-abc" {
		t.Fatalf("want bead_id test-abc, got %s", result.Record.BeadID)
	}
}

func TestFR3_Check_BlockedBead(t *testing.T) {
	specDir, mapFile := setupMapTestSpec(t)

	// Update record 2 hash to match actual merkle hash.
	spec, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		t.Fatalf("build spec graph: %v", err)
	}
	actualHash, err := spec.NodeHash("alpha/component/2")
	if err != nil {
		t.Fatalf("get node hash: %v", err)
	}
	store := mapping.NewFileStore(mapFile)
	if err := store.UpdateSpecHash(2, actualHash); err != nil {
		t.Fatalf("update hash: %v", err)
	}

	// Comp2 uses Comp1, and Comp1's bead_status is "closed", so it should be ready.
	// But let's make Comp1 open to test blocking.
	rec, err := store.Get(1)
	if err != nil {
		t.Fatalf("get record 1: %v", err)
	}
	store.Delete(1)
	rec.BeadStatus = "open"
	store.Create(rec)

	out := captureStdout(t, func() {
		code := runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "test-def"})
		if code != 1 {
			t.Fatalf("want exit 1 for blocked bead, got %d", code)
		}
	})

	var result mapping.PreflightResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Status != "blocked" {
		t.Fatalf("want status blocked, got %s", result.Status)
	}
	if len(result.Blockers) == 0 {
		t.Fatal("want at least one blocker")
	}
}

func TestFR3_Check_UnknownBead(t *testing.T) {
	specDir, mapFile := setupMapTestSpec(t)

	code := runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "unknown-bead-id"})
	if code != 1 {
		t.Fatalf("want exit 1 for unknown bead, got %d", code)
	}
}

func TestFR3_Check_StaleBead(t *testing.T) {
	specDir, mapFile := setupMapTestSpec(t)

	// Record 1 has spec_hash "hash1" which won't match the actual merkle hash.
	// This should trigger a stale result.
	out := captureStdout(t, func() {
		code := runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "test-abc"})
		if code != 1 {
			t.Fatalf("want exit 1 for stale bead, got %d", code)
		}
	})

	var result mapping.PreflightResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if result.Status != "stale" {
		t.Fatalf("want status stale, got %s", result.Status)
	}
	if result.StaleHash == "" {
		t.Fatal("stale result should include current hash")
	}
}

func TestFR3_MapCommand_Dispatch(t *testing.T) {
	code := runMap(nil)
	if code != 1 {
		t.Fatalf("want exit 1 for no subcommand, got %d", code)
	}
}

func TestFR3_MapCommand_UnknownSubcommand(t *testing.T) {
	code := runMap([]string{"unknown"})
	if code != 1 {
		t.Fatalf("want exit 1 for unknown subcommand, got %d", code)
	}
}

func TestNFR4_Check_Deterministic(t *testing.T) {
	specDir, mapFile := setupMapTestSpec(t)

	spec, err := mapping.NewSpecGraph(specDir)
	if err != nil {
		t.Fatalf("build spec graph: %v", err)
	}
	actualHash, err := spec.NodeHash("alpha/component/1")
	if err != nil {
		t.Fatalf("get node hash: %v", err)
	}
	store := mapping.NewFileStore(mapFile)
	if err := store.UpdateSpecHash(1, actualHash); err != nil {
		t.Fatalf("update hash: %v", err)
	}

	out1 := captureStdout(t, func() {
		runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "test-abc"})
	})
	out2 := captureStdout(t, func() {
		runCheck([]string{"-map-file", mapFile, "-spec-dir", specDir, "test-abc"})
	})

	if out1 != out2 {
		t.Fatalf("determinism: outputs differ:\nrun1: %s\nrun2: %s", out1, out2)
	}
}
