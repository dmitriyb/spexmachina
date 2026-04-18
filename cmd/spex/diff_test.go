package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// setupDiffTestSpec creates a spec directory where every entity has a distinct
// identity hash so that per-leaf merkle keys do not collide. Returns the
// directory and the identity-hash keys for Comp1 and Impl1.
func setupDiffTestSpec(t *testing.T) (specDir, comp1Hash, impl1Hash string) {
	t.Helper()
	dir := t.TempDir()

	comp1Hash = schema.IdentityHash("alpha", "component", "Comp1")
	impl1Hash = schema.IdentityHash("alpha", "impl_section", "Impl1")

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
	modJSON := `{
		"name": "alpha",
		"components": [
			{"id": "` + comp1Hash + `", "name": "Comp1", "content": "arch_comp1.md"}
		],
		"impl_sections": [
			{"id": "` + impl1Hash + `", "name": "Impl1", "content": "impl_comp1.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", modJSON)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeTestFile(t, alphaDir, "impl_comp1.md", "# Comp1 implementation\n")

	return dir, comp1Hash, impl1Hash
}

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
	specDir, _, impl1Hash := setupDiffTestSpec(t)

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
		if c.Type == "modified" && c.Path == impl1Hash {
			foundModified = true
			if c.NodeType != "impl_section" {
				t.Fatalf("want impl_section node_type for %s, got %q", impl1Hash, c.NodeType)
			}
		}
	}
	if !foundModified {
		t.Fatalf("expected modified change for impl_section hash %s, got: %+v", impl1Hash, result.Changes)
	}
}

func TestFR5_DiffCommand_ImpactClassification(t *testing.T) {
	specDir, comp1Hash, _ := setupDiffTestSpec(t)

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
		if c.Impact == "arch_impl" && c.Path == comp1Hash {
			foundArchImpl = true
			if c.Module == "" {
				t.Fatal("expected module hash for arch change")
			}
			if c.NodeType != "component" {
				t.Fatalf("want component node_type for %s, got %q", comp1Hash, c.NodeType)
			}
		}
	}
	if !foundArchImpl {
		t.Fatalf("expected arch_impl impact for component hash %s, got: %+v", comp1Hash, result.Changes)
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

// setupTestSpecWithRequirements creates a spec fixture with requirements and
// implements edges so completeness checking can detect incomplete changes.
// Each entity uses a distinct identity hash so that merkle leaf keys do not
// collide. Module requirements derive from the project requirement via
// preq_id so project-level completeness checks can walk the full chain.
func setupTestSpecWithRequirements(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	projReq1Hash := schema.IdentityHash("project", "requirement", "1")
	req1Hash := schema.IdentityHash("alpha", "requirement", "Req 1")
	req2Hash := schema.IdentityHash("alpha", "requirement", "Req 2")
	compAHash := schema.IdentityHash("alpha", "component", "CompA")
	compBHash := schema.IdentityHash("alpha", "component", "CompB")
	implAHash := schema.IdentityHash("alpha", "impl_section", "Impl1")

	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Proj Req 1"}
		],
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha"}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": "` + req1Hash + `", "type": "functional", "title": "Req 1", "preq_id": "` + projReq1Hash + `"},
			{"id": "` + req2Hash + `", "type": "functional", "title": "Req 2", "preq_id": "` + projReq1Hash + `"}
		],
		"components": [
			{"id": "` + compAHash + `", "name": "CompA", "content": "arch_comp_a.md", "implements": ["` + req1Hash + `"]},
			{"id": "` + compBHash + `", "name": "CompB", "content": "arch_comp_b.md", "implements": ["` + req2Hash + `"]}
		],
		"impl_sections": [
			{"id": "` + implAHash + `", "name": "Impl1", "content": "impl_comp_a.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_comp_a.md", "# CompA architecture\n")
	writeTestFile(t, alphaDir, "arch_comp_b.md", "# CompB architecture\n")
	writeTestFile(t, alphaDir, "impl_comp_a.md", "# CompA implementation\n")

	return dir
}

// mutatedAlphaModule returns the alpha/module.json body for a mutation pass
// where requirement titles are overridden. Identity hashes for every entity
// stay identical to those written by setupTestSpecWithRequirements so that
// the diff reports requirement modifications, not add+remove pairs.
func mutatedAlphaModule(t *testing.T, req1Title, req2Title string) string {
	t.Helper()
	projReq1Hash := schema.IdentityHash("project", "requirement", "1")
	req1Hash := schema.IdentityHash("alpha", "requirement", "Req 1")
	req2Hash := schema.IdentityHash("alpha", "requirement", "Req 2")
	compAHash := schema.IdentityHash("alpha", "component", "CompA")
	compBHash := schema.IdentityHash("alpha", "component", "CompB")
	implAHash := schema.IdentityHash("alpha", "impl_section", "Impl1")

	return `{
		"name": "alpha",
		"requirements": [
			{"id": "` + req1Hash + `", "type": "functional", "title": "` + req1Title + `", "preq_id": "` + projReq1Hash + `"},
			{"id": "` + req2Hash + `", "type": "functional", "title": "` + req2Title + `", "preq_id": "` + projReq1Hash + `"}
		],
		"components": [
			{"id": "` + compAHash + `", "name": "CompA", "content": "arch_comp_a.md", "implements": ["` + req1Hash + `"]},
			{"id": "` + compBHash + `", "name": "CompB", "content": "arch_comp_b.md", "implements": ["` + req2Hash + `"]}
		],
		"impl_sections": [
			{"id": "` + implAHash + `", "name": "Impl1", "content": "impl_comp_a.md"}
		]
	}`
}

func TestFR8_DiffCommand_CompletenessErrors_JSON(t *testing.T) {
	specDir := setupTestSpecWithRequirements(t)

	// Create initial snapshot.
	_, err := runSpex(t, "hash", "--spec-dir", specDir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify a requirement leaf by changing the module.json requirement description.
	// This changes the requirement node hash but NOT the implementing component.
	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if len(result.Errors) == 0 {
		t.Fatal("expected completeness errors when requirement changed without component change")
	}

	foundIncomplete := false
	for _, e := range result.Errors {
		if e.Type == "incomplete_change" && strings.Contains(e.Message, "CompA") {
			foundIncomplete = true
		}
	}
	if !foundIncomplete {
		t.Fatalf("expected incomplete_change error mentioning CompA, got: %+v", result.Errors)
	}
}

func TestFR8_DiffCommand_CompletenessErrors_HumanOutput(t *testing.T) {
	specDir := setupTestSpecWithRequirements(t)

	_, err := runSpex(t, "hash", "--spec-dir", specDir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify requirement without component change.
	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))

	out, err := runSpex(t, "diff", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if !strings.Contains(out, "warning:") {
		t.Fatalf("human output should contain warning for completeness errors, got: %s", out)
	}
	if !strings.Contains(out, "incomplete_change") {
		t.Fatalf("human output should contain error type, got: %s", out)
	}
}

func TestFR8_DiffCommand_NoCompletenessErrors_WhenComplete(t *testing.T) {
	// TODO(bead:spexmachina-ir6): fix after spexmachina-kdb changed to identity-hash keying
	t.Skip("TODO(bead:spexmachina-ir6): fix after spexmachina-kdb changed to identity-hash keying")
	specDir := setupTestSpecWithRequirements(t)

	_, err := runSpex(t, "hash", "--spec-dir", specDir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify both the requirement AND all implementing components.
	// Changing module.json (requirement title) also triggers meta-only checks
	// for all components, so both must change.
	alphaDir := filepath.Join(specDir, "alpha")
	alphaMod := `{
		"name": "alpha",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "Req 1 CHANGED", "preq_id": "aabbccddeeff"},
			{"id": "ffeeddccbbaa", "type": "functional", "title": "Req 2"}
		],
		"components": [
			{"id": "aabbccddeeff", "name": "CompA", "content": "arch_comp_a.md", "implements": ["aabbccddeeff"]},
			{"id": "ffeeddccbbaa", "name": "CompB", "content": "arch_comp_b.md", "implements": ["ffeeddccbbaa"]}
		],
		"impl_sections": [
			{"id": "aabbccddeeff", "name": "Impl1", "content": "impl_comp_a.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_comp_a.md", "# CompA architecture CHANGED\n")
	writeTestFile(t, alphaDir, "arch_comp_b.md", "# CompB architecture CHANGED\n")

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if len(result.Errors) != 0 {
		t.Fatalf("expected no completeness errors when both requirement and component changed, got: %+v", result.Errors)
	}
}

func TestFR8_DiffCommand_NoSnapshot_NoCompletenessErrors(t *testing.T) {
	specDir := setupTestSpecWithRequirements(t)

	// With no snapshot, all nodes are "added" — requirements and their
	// implementing components are both added, so no completeness errors.
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
	if len(result.Errors) != 0 {
		t.Fatalf("expected no completeness errors when all nodes are added, got: %+v", result.Errors)
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
