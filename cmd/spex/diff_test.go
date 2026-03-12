package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR4_DiffCommand_NoSnapshot_AllAdded(t *testing.T) {
	specDir := setupTestSpec(t)

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total == 0 {
		t.Fatal("expected changes when no snapshot exists")
	}

	for _, c := range result.Changes {
		if c.Type != "added" {
			t.Fatalf("all changes should be 'added' with no snapshot, got %q for %s", c.Type, c.Path)
		}
	}
}

func TestFR4_DiffCommand_NoChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total != 0 {
		t.Fatalf("expected 0 changes when nothing changed, got %d", result.Summary.Total)
	}
}

func TestFR4_DiffCommand_Modified(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	implPath := filepath.Join(specDir, "alpha", "impl_comp1.md")
	if err := os.WriteFile(implPath, []byte("# Changed implementation\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total == 0 {
		t.Fatal("expected changes after modifying a file")
	}

	foundModified := false
	for _, c := range result.Changes {
		if c.Type == "modified" && c.Path == "module/1/impl_section/1" {
			foundModified = true
		}
	}
	if !foundModified {
		t.Fatal("expected modified change for module/1/impl_section/1")
	}
}

func TestFR5_DiffCommand_ImpactClassification(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	archPath := filepath.Join(specDir, "alpha", "arch_comp1.md")
	if err := os.WriteFile(archPath, []byte("# Changed architecture\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	foundArchImpl := false
	for _, c := range result.Changes {
		if c.Impact == "arch_impl" && c.Path == "module/1/component/1" {
			foundArchImpl = true
			if c.Module == "" {
				t.Fatal("expected module name for arch change")
			}
		}
	}
	if !foundArchImpl {
		t.Fatalf("expected arch_impl impact for component change, got: %+v", result.Changes)
	}
}

func TestFR4_DiffCommand_CustomSnapshotPath(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(t.TempDir(), "custom-snapshot.json")
	if err := merkle.Save(tree, customPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--json", "--snapshot", customPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total != 0 {
		t.Fatalf("expected 0 changes with matching snapshot, got %d", result.Summary.Total)
	}
}

func TestFR4_DiffCommand_HumanOutput_NoChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if !strings.Contains(out, "no changes") {
		t.Fatalf("human output should say 'no changes', got: %s", out)
	}
}

func TestFR4_DiffCommand_HumanOutput_WithChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	out, err := runSpex(t, "diff", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if !strings.Contains(out, "change(s)") {
		t.Fatalf("human output should contain change summary, got: %s", out)
	}
	if !strings.Contains(out, "added") {
		t.Fatalf("human output should mention added changes, got: %s", out)
	}
}

func TestFR4_DiffCommand_NonexistentDir(t *testing.T) {
	_, err := runSpex(t, "diff", "--spec-dir", "/nonexistent/path")
	if err == nil {
		t.Fatal("should fail with nonexistent dir")
	}
}

func TestNFR6_DiffCommand_Deterministic(t *testing.T) {
	specDir := setupTestSpec(t)

	out1, _ := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	out2, _ := runSpex(t, "diff", "--json", "--spec-dir", specDir)

	var r1, r2 diffOutput
	if err := json.Unmarshal([]byte(out1), &r1); err != nil {
		t.Fatalf("unmarshal first run: %v", err)
	}
	if err := json.Unmarshal([]byte(out2), &r2); err != nil {
		t.Fatalf("unmarshal second run: %v", err)
	}

	if len(r1.Changes) != len(r2.Changes) {
		t.Fatalf("determinism: change count differs: %d vs %d", len(r1.Changes), len(r2.Changes))
	}
	for i := range r1.Changes {
		if r1.Changes[i].Path != r2.Changes[i].Path {
			t.Fatalf("determinism: change %d path differs: %s vs %s", i, r1.Changes[i].Path, r2.Changes[i].Path)
		}
		if r1.Changes[i].Type != r2.Changes[i].Type {
			t.Fatalf("determinism: change %d type differs: %s vs %s", i, r1.Changes[i].Type, r2.Changes[i].Type)
		}
	}
}
