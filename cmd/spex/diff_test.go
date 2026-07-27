package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// runDiff executes `spex diff` with the given args and returns stdout, stderr,
// and the process exit code, mirroring main.go's exit-code and stderr
// handling: the exit code is 0 on success, the error's ExitCode() when it
// implements that interface, or 1 otherwise. The returned error message is
// written to stderr exactly as main.go does, so E1/E2 stderr assertions hold.
func runDiff(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newDiffCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"diff"}, args...))

	var execErr error
	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})

	if execErr != nil {
		fmt.Fprintln(errBuf, execErr)
		exitCode = 1
		var ec interface{ ExitCode() int }
		if errors.As(execErr, &ec) {
			exitCode = ec.ExitCode()
		}
	}
	return stdout, errBuf.String(), exitCode
}

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
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
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

// setupDiffTestSpecWithDataFlow extends setupDiffTestSpec with a data_flow
// leaf so tests can exercise the contract impact level. Returns the spec
// directory plus the identity hashes for the component and the data_flow.
func setupDiffTestSpecWithDataFlow(t *testing.T) (specDir, compHash, flowHash string) {
	t.Helper()
	dir := t.TempDir()

	compHash = schema.IdentityHash("alpha", "component", "Comp1")
	flowHash = schema.IdentityHash("alpha", "data_flow", "Flow1")

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
	modJSON := `{
		"name": "alpha",
		"components": [
			{"id": "` + compHash + `", "name": "Comp1", "content": "arch_comp1.md"}
		],
		"data_flows": [
			{"id": "` + flowHash + `", "name": "Flow1", "content": "flow_flow1.md", "uses": ["` + compHash + `"]}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", modJSON)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeTestFile(t, alphaDir, "flow_flow1.md", "# Flow1 data flow\n")

	return dir, compHash, flowHash
}

func TestFR5_DiffCommand_DataFlowIsContract(t *testing.T) {
	specDir, _, flowHash := setupDiffTestSpecWithDataFlow(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	flowPath := filepath.Join(specDir, "alpha", "flow_flow1.md")
	if err := os.WriteFile(flowPath, []byte("# Flow1 data flow CHANGED\n"), 0644); err != nil {
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

	foundContract := false
	for _, c := range result.Changes {
		if c.Path == flowHash {
			foundContract = true
			if c.Impact != "contract" {
				t.Fatalf("want impact %q for data_flow %s, got %q", "contract", flowHash, c.Impact)
			}
			if c.NodeType != "data_flow" {
				t.Fatalf("want node_type %q, got %q", "data_flow", c.NodeType)
			}
		}
	}
	if !foundContract {
		t.Fatalf("expected change for data_flow hash %s, got: %+v", flowHash, result.Changes)
	}
	if result.Summary.ByImpact["contract"] == 0 {
		t.Fatalf("expected summary.by_impact to count contract changes, got: %+v", result.Summary.ByImpact)
	}
}

func TestFR5_DiffCommand_ContractInHumanSummary(t *testing.T) {
	specDir, _, _ := setupDiffTestSpecWithDataFlow(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	flowPath := filepath.Join(specDir, "alpha", "flow_flow1.md")
	if err := os.WriteFile(flowPath, []byte("# Flow1 data flow CHANGED\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "diff", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if !strings.Contains(out, "1 contract") {
		t.Fatalf("human summary should report contract count for data_flow change, got: %s", out)
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

// E2: a corrupted snapshot is an IO/parse failure — exit 1, and stderr names
// the offending snapshot path. Distinct from the exit-2 errors-array case.
func TestFR4_E2_DiffCommand_CorruptedSnapshot(t *testing.T) {
	specDir := setupTestSpec(t)

	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if err := os.WriteFile(snapshotPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode != 1 {
		t.Fatalf("want exit 1 on corrupted snapshot, got %d", exitCode)
	}
	if !strings.Contains(stderr, snapshotPath) {
		t.Fatalf("stderr should name the snapshot path %q, got: %s", snapshotPath, stderr)
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

	const projReq1ID = "000000000001"
	req1Hash := schema.IdentityHash("alpha", "requirement", "Req 1")
	req2Hash := schema.IdentityHash("alpha", "requirement", "Req 2")
	compAHash := schema.IdentityHash("alpha", "component", "CompA")
	compBHash := schema.IdentityHash("alpha", "component", "CompB")
	implAHash := schema.IdentityHash("alpha", "impl_section", "Impl1")

	proj := `{
		"name": "test-project",
		"requirements": [
			{"id": "` + projReq1ID + `", "type": "functional", "title": "Proj Req 1"}
		],
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
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
			{"id": "` + req1Hash + `", "type": "functional", "title": "Req 1", "preq_id": "` + projReq1ID + `"},
			{"id": "` + req2Hash + `", "type": "functional", "title": "Req 2", "preq_id": "` + projReq1ID + `"}
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
	const projReq1ID = "000000000001"
	req1Hash := schema.IdentityHash("alpha", "requirement", "Req 1")
	req2Hash := schema.IdentityHash("alpha", "requirement", "Req 2")
	compAHash := schema.IdentityHash("alpha", "component", "CompA")
	compBHash := schema.IdentityHash("alpha", "component", "CompB")
	implAHash := schema.IdentityHash("alpha", "impl_section", "Impl1")

	return `{
		"name": "alpha",
		"requirements": [
			{"id": "` + req1Hash + `", "type": "functional", "title": "` + req1Title + `", "preq_id": "` + projReq1ID + `"},
			{"id": "` + req2Hash + `", "type": "functional", "title": "` + req2Title + `", "preq_id": "` + projReq1ID + `"}
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
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify a requirement leaf by changing the module.json requirement description.
	// This changes the requirement node hash but NOT the implementing component.
	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))

	out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 2 {
		t.Fatalf("want exit code 2 when errors array is non-empty, got %d", exitCode)
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

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify requirement without component change.
	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))

	out, _, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode != 2 {
		t.Fatalf("want exit code 2 when errors array is non-empty, got %d", exitCode)
	}

	if !strings.Contains(out, "error:") {
		t.Fatalf("human output should prefix completeness findings with error:, got: %s", out)
	}
	if strings.Contains(out, "warning") {
		t.Fatalf("human output must not use warning terminology for errors, got: %s", out)
	}
	if !strings.Contains(out, "incomplete_change") {
		t.Fatalf("human output should contain error type, got: %s", out)
	}
}

func TestFR8_DiffCommand_NoCompletenessErrors_WhenComplete(t *testing.T) {
	specDir := setupTestSpecWithRequirements(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify both the requirement AND all implementing components.
	// Changing module.json (requirement title) also triggers meta-only checks
	// for all components, so both must change.
	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))
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

// snapshotIncompleteSpec writes a fresh snapshot for the requirements fixture,
// then mutates a requirement leaf without touching its implementing component
// so the next diff carries a CompletenessChecker error.
func snapshotIncompleteSpec(t *testing.T) (specDir string) {
	t.Helper()
	specDir = setupTestSpecWithRequirements(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	alphaDir := filepath.Join(specDir, "alpha")
	writeTestFile(t, alphaDir, "module.json", mutatedAlphaModule(t, "Req 1 CHANGED", "Req 2"))
	return specDir
}

// TestFR8_E5_DiffCommand_ExitCodeSemantics locks the exit-code contract from
// arch_diff_command.md: changes alone exit 0, no changes exit 0, and any
// non-empty errors array exits 2 while still emitting the full diff on stdout.
func TestFR8_E5_DiffCommand_ExitCodeSemantics(t *testing.T) {
	t.Run("changes but no errors exits 0", func(t *testing.T) {
		specDir, _, _ := setupDiffTestSpec(t)
		tree, err := merkle.BuildTree(specDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
			t.Fatal(err)
		}
		implPath := filepath.Join(specDir, "alpha", "impl_comp1.md")
		if err := os.WriteFile(implPath, []byte("# Changed implementation\n"), 0644); err != nil {
			t.Fatal(err)
		}

		stdout, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
		if exitCode != 0 {
			t.Fatalf("want exit 0 when there are changes but no errors, got %d", exitCode)
		}
		var result diffOutput
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
		}
		if result.Summary.Total == 0 {
			t.Fatal("expected the impl-only change to be reported")
		}
		if len(result.Errors) != 0 {
			t.Fatalf("expected no errors for an impl-only change, got: %+v", result.Errors)
		}
	})

	t.Run("no changes exits 0", func(t *testing.T) {
		specDir := setupTestSpec(t)
		tree, err := merkle.BuildTree(specDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
			t.Fatal(err)
		}

		_, _, exitCode := runDiff(t, "--spec-dir", specDir)
		if exitCode != 0 {
			t.Fatalf("want exit 0 when there are no changes, got %d", exitCode)
		}
	})

	t.Run("non-empty errors exits 2 with full diff on stdout", func(t *testing.T) {
		specDir := snapshotIncompleteSpec(t)

		stdout, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
		if exitCode != 2 {
			t.Fatalf("want exit 2 when errors array is non-empty, got %d", exitCode)
		}
		var result diffOutput
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("full diff must still be valid JSON on stdout: %v\noutput: %s", err, stdout)
		}
		if len(result.Changes) == 0 {
			t.Fatal("full diff (changes) must still be emitted on stdout under exit 2")
		}
		if len(result.Errors) == 0 {
			t.Fatal("full diff (errors) must still be emitted on stdout under exit 2")
		}
	})

	t.Run("text output renders findings under error heading", func(t *testing.T) {
		specDir := snapshotIncompleteSpec(t)

		stdout, _, exitCode := runDiff(t, "--spec-dir", specDir)
		if exitCode != 2 {
			t.Fatalf("want exit 2 for text output with errors, got %d", exitCode)
		}
		if !strings.Contains(stdout, "error(s):") {
			t.Fatalf("text output should render findings under an error(s): heading, got: %s", stdout)
		}
		if !strings.Contains(stdout, "error:") {
			t.Fatalf("text output should prefix each finding line with error:, got: %s", stdout)
		}
		if strings.Contains(stdout, "warning") {
			t.Fatalf("text output must never use warning terminology, got: %s", stdout)
		}
	})
}

// TestFR8_E6_DiffCommand_ErrorTerminologyMatch locks the terminology contract:
// JSON exposes a top-level errors array (never warnings) with type/message/
// path/related entries, and the text output labels the same findings error:,
// never warning:.
func TestFR8_E6_DiffCommand_ErrorTerminologyMatch(t *testing.T) {
	specDir := snapshotIncompleteSpec(t)

	jsonOut, _, jsonExit := runDiff(t, "--json", "--spec-dir", specDir)
	if jsonExit != 2 {
		t.Fatalf("want exit 2 for JSON run with errors, got %d", jsonExit)
	}

	// Raw-string check: the JSON keys themselves must not drift to warnings.
	if strings.Contains(jsonOut, "\"warnings\"") || strings.Contains(jsonOut, "warning") {
		t.Fatalf("JSON output must not contain warning terminology, got: %s", jsonOut)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(jsonOut), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, jsonOut)
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected a non-empty errors array")
	}
	for _, e := range result.Errors {
		if e.Type == "" {
			t.Fatalf("each error entry must carry a type, got: %+v", e)
		}
		if e.Message == "" {
			t.Fatalf("each error entry must carry a message, got: %+v", e)
		}
		if e.Path == "" {
			t.Fatalf("each error entry must carry a path, got: %+v", e)
		}
		if e.Related == nil {
			t.Fatalf("each error entry must carry a related field, got: %+v", e)
		}
	}

	textOut, _, textExit := runDiff(t, "--spec-dir", specDir)
	if textExit != 2 {
		t.Fatalf("want exit 2 for text run with errors, got %d", textExit)
	}
	if !strings.Contains(textOut, "error:") {
		t.Fatalf("text output should label findings with error:, got: %s", textOut)
	}
	if strings.Contains(textOut, "warning") {
		t.Fatalf("text output must not contain warning terminology, got: %s", textOut)
	}
}

// setupRemovalSpec writes a one-module spec whose arch leaf names two
// components, snapshots it, then deletes CompB from module.json and from its
// own arch leaf while CompA's leaf goes on naming it. The next diff therefore
// reports CompB as removed with its name still in the corpus, which is the
// only way to reach the surviving-name path through the real command.
func setupRemovalSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	compAHash := schema.IdentityHash("alpha", "component", "CompA")
	compBHash := schema.IdentityHash("alpha", "component", "CompB")

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "`+schema.IdentityHash("module", "alpha")+`", "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+compAHash+`", "name": "CompA", "content": "arch_comp_a.md"},
			{"id": "`+compBHash+`", "name": "CompB", "content": "arch_comp_b.md"}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_comp_a.md", "# CompA\n\nCompA delegates to CompB.\n")
	writeTestFile(t, alphaDir, "arch_comp_b.md", "# CompB\n")

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(dir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+compAHash+`", "name": "CompA", "content": "arch_comp_a.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(alphaDir, "arch_comp_b.md")); err != nil {
		t.Fatal(err)
	}

	return dir
}

// TestFR8_DiffCommand_SurvivingName_JSON covers the wiring `spex diff` adds on
// top of CheckRemovedNames: the DiffError it builds, the site it reports as
// the path, and the exit-2 halt that stops the changeset pipeline. The checker
// has its own tests; none of them exercise this translation.
func TestFR8_DiffCommand_SurvivingName_JSON(t *testing.T) {
	specDir := setupRemovalSpec(t)
	compBHash := schema.IdentityHash("alpha", "component", "CompB")

	out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 2 {
		t.Fatalf("want exit 2 while a removed name survives, got %d\noutput: %s", exitCode, out)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	var found *merkle.DiffError
	for i, e := range result.Errors {
		if e.Type == "surviving_name" {
			found = &result.Errors[i]
		}
	}
	if found == nil {
		t.Fatalf("want a surviving_name error, got: %+v", result.Errors)
	}
	if !strings.Contains(found.Message, `component "CompB"`) || !strings.Contains(found.Message, compBHash) {
		t.Fatalf("message must name the node and its hash, got %q", found.Message)
	}
	if !strings.Contains(found.Message, "1 site(s)") {
		t.Fatalf("message must carry the site count, got %q", found.Message)
	}
	if found.Path != "alpha/arch_comp_a.md:3" {
		t.Fatalf("want the first site as the path, got %q", found.Path)
	}
	if len(found.Related) != 1 || found.Related[0] != found.Path {
		t.Fatalf("related must carry every site, got %v", found.Related)
	}
}

// TestFR8_DiffCommand_SurvivingName_HumanOutput: the same finding under the
// text renderer, which shares the error heading with CompletenessChecker.
func TestFR8_DiffCommand_SurvivingName_HumanOutput(t *testing.T) {
	specDir := setupRemovalSpec(t)

	out, _, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode != 2 {
		t.Fatalf("want exit 2, got %d\noutput: %s", exitCode, out)
	}
	if !strings.Contains(out, "error: [surviving_name]") {
		t.Fatalf("text output should render the finding as an error, got: %s", out)
	}
	if !strings.Contains(out, "alpha/arch_comp_a.md:3") {
		t.Fatalf("text output should name the surviving site, got: %s", out)
	}
	if strings.Contains(out, "warning") {
		t.Fatalf("text output must never use warning terminology, got: %s", out)
	}
}

// TestFR8_DiffCommand_SweptRemovalPasses: once the mention is gone the removal
// is complete and the pipeline is free to advance. Without this, the two tests
// above would pass just as well on a checker that failed every removal.
func TestFR8_DiffCommand_SweptRemovalPasses(t *testing.T) {
	specDir := setupRemovalSpec(t)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp_a.md", "# CompA\n\nCompA delegates to nothing.\n")

	out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 0 {
		t.Fatalf("want exit 0 for a swept removal, got %d\noutput: %s", exitCode, out)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("want no errors once the mention is swept, got: %+v", result.Errors)
	}
}

// TestFR8_DiffCommand_UnverifiableRemovalNoted: retiring the module as well is
// the highest-volume removal case, and when the module name cannot be
// recovered the sweep cannot check anything at all. It says so on stdout under
// a note heading rather than exiting 0 in silence. A note is not a violation,
// so the exit code stays 0 and the errors array stays empty.
func TestFR8_DiffCommand_UnverifiableRemovalNoted(t *testing.T) {
	specDir := setupRemovalSpec(t)

	// Retire the module too: alpha leaves project.json, so the diff can only
	// report its children against the module identity hash, and the name
	// "alpha" is nowhere in what remains of the corpus.
	writeTestFile(t, specDir, "project.json", `{"name": "test-project", "modules": []}`)
	if err := os.RemoveAll(filepath.Join(specDir, "alpha")); err != nil {
		t.Fatal(err)
	}

	out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 0 {
		t.Fatalf("an unverifiable removal is not a violation, got exit %d\noutput: %s", exitCode, out)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("want no errors, got: %+v", result.Errors)
	}
	if len(result.Notes) != 1 {
		t.Fatalf("want exactly 1 note, got: %+v", result.Notes)
	}
	if result.Notes[0].Type != "unverifiable_module" {
		t.Fatalf("want an unverifiable_module note, got: %+v", result.Notes[0])
	}
	if !strings.Contains(result.Notes[0].Message, "could not be checked") {
		t.Fatalf("note must say the sweep did not run, got %q", result.Notes[0].Message)
	}

	textOut, _, _ := runDiff(t, "--spec-dir", specDir)
	if !strings.Contains(textOut, "note: [unverifiable_module]") {
		t.Fatalf("text output should render the note, got: %s", textOut)
	}
}

// setupRetiredModuleSpec retires a whole module: alpha declares Retiree, beta's
// leaf names it, and after the snapshot alpha leaves project.json and disk
// entirely. What remains never writes the word "alpha", so the module name is
// unrecoverable from the corpus — the case where `spex diff` can only report
// the removed component against a module identity hash.
func setupRetiredModuleSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	retireeHash := schema.IdentityHash("alpha", "component", "Retiree")
	keeperHash := schema.IdentityHash("beta", "component", "Keeper")

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "`+schema.IdentityHash("module", "alpha")+`", "name": "alpha", "path": "alpha"},
			{"id": "`+schema.IdentityHash("module", "beta")+`", "name": "beta", "path": "beta"}
		]
	}`)

	for _, sub := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(dir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+retireeHash+`", "name": "Retiree", "content": "arch_retiree.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(dir, "alpha"), "arch_retiree.md", "# Retiree\n")
	writeTestFile(t, filepath.Join(dir, "beta"), "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+keeperHash+`", "name": "Keeper", "content": "arch_keeper.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(dir, "beta"), "arch_keeper.md", "# Keeper\n\nKeeper used to delegate to Retiree.\n")

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(dir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "`+schema.IdentityHash("module", "beta")+`", "name": "beta", "path": "beta"}
		]
	}`)
	if err := os.RemoveAll(filepath.Join(dir, "alpha")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestFR8_DiffCommand_BeadMapRecoversRetiredModule is the wiring for the
// --map flag, and the hole it closes. The surviving mention of Retiree is
// identical in both runs; only the hash → name source differs. Without a map
// the sweep can prove nothing about a module whose name was swept out of the
// prose, and discloses that; with the map ingest already wrote, the same run
// recovers "alpha", reports the survivor and halts the pipeline. The sweep's
// reach must not depend on how thoroughly the module name itself was swept.
func TestFR8_DiffCommand_BeadMapRecoversRetiredModule(t *testing.T) {
	specDir := setupRetiredModuleSpec(t)
	retireeHash := schema.IdentityHash("alpha", "component", "Retiree")

	mapPath := filepath.Join(t.TempDir(), ".bead-map.json")
	writeTestFile(t, filepath.Dir(mapPath), ".bead-map.json", `{
		"next_id": 2,
		"records": [
			{
				"id": 1,
				"spec_node_id": "`+retireeHash+`",
				"bead_id": "test-retiree",
				"bead_type": "task",
				"module": "alpha",
				"component": "Retiree",
				"content_file": "spec/alpha/arch_retiree.md",
				"spec_hash": "deadbeef"
			}
		]
	}`)

	t.Run("no map: unverifiable, disclosed but not gating", func(t *testing.T) {
		out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir, "--map", filepath.Join(specDir, "absent.json"))
		if exitCode != 0 {
			t.Fatalf("an unverifiable removal is not a violation, got exit %d\noutput: %s", exitCode, out)
		}
		var result diffOutput
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
		}
		if len(result.Errors) != 0 {
			t.Fatalf("want no errors without a hash -> name source, got: %+v", result.Errors)
		}
		if len(result.Notes) != 1 || result.Notes[0].Type != "unverifiable_module" {
			t.Fatalf("want exactly one unverifiable_module note, got: %+v", result.Notes)
		}
	})

	t.Run("bead map: the same survivor is reported", func(t *testing.T) {
		out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir, "--map", mapPath)
		if exitCode != 2 {
			t.Fatalf("want exit 2 once the module name is recovered, got %d\noutput: %s", exitCode, out)
		}
		var result diffOutput
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
		}
		if len(result.Notes) != 0 {
			t.Fatalf("a recovered module needs no note, got: %+v", result.Notes)
		}
		var found *merkle.DiffError
		for i, e := range result.Errors {
			if e.Type == "surviving_name" {
				found = &result.Errors[i]
			}
		}
		if found == nil {
			t.Fatalf("want a surviving_name error, got: %+v", result.Errors)
		}
		if !strings.Contains(found.Message, `component "Retiree"`) || !strings.Contains(found.Message, retireeHash) {
			t.Fatalf("message must name the node and its hash, got %q", found.Message)
		}
		if found.Path != "beta/arch_keeper.md:3" {
			t.Fatalf("want the surviving site as the path, got %q", found.Path)
		}
	})
}
