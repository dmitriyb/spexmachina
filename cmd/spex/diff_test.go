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

	"github.com/dmitriyb/spexmachina/adapters"
	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/ingest"
	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
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
func setupDiffTestSpec(t *testing.T) (specDir, comp1Hash, test1Hash string) {
	t.Helper()
	dir := t.TempDir()

	comp1Hash = schema.IdentityHash("alpha", "component", "Comp1")
	test1Hash = schema.IdentityHash("alpha", "test_section", "Test1")

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
		"test_sections": [
			{"id": "` + test1Hash + `", "name": "Test1", "content": "test_comp1.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", modJSON)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeTestFile(t, alphaDir, "test_comp1.md", "# Comp1 tests\n")

	return dir, comp1Hash, test1Hash
}

// S1/S2: a freshly initialised project — the default snapshot location
// seeded with the canonical empty tree, exactly what `spex init` writes —
// reports every leaf as added, and the --json structure carries path,
// type, impact, module and node_type on every entry, suitable for piping
// to `spex plan` or `jq`. This is the only route to the everything-added
// bootstrap output; a directory that merely lacks a snapshot file is E7's
// not-a-project refusal, never this path.
func TestFR4_S1_S2_DiffCommand_BootstrapAllAdded(t *testing.T) {
	specDir := setupTestSpec(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	for _, key := range []string{"changes", "errors", "summary"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("JSON output missing top-level key %q, got: %s", key, out)
		}
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total == 0 {
		t.Fatal("expected changes against an empty-tree snapshot")
	}

	for _, c := range result.Changes {
		if c.Type != "added" {
			t.Fatalf("all changes should be 'added' against an empty-tree snapshot, got %q for %s", c.Type, c.Path)
		}
		if c.Path == "" || c.Impact == "" || c.NodeType == "" {
			t.Fatalf("every change must carry path/impact/node_type, got: %+v", c)
		}
		// Module is empty only for the project-level meta leaf
		// (meta/project); every module-scoped leaf, meta/<module> included,
		// carries a module name.
		if c.Module == "" && c.Path != "meta/project" {
			t.Fatalf("every change but the project-level meta leaf must carry a module, got: %+v", c)
		}
	}
}

func TestFR4_DiffCommand_NoChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

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
	specDir, _, test1Hash := setupDiffTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

	testPath := filepath.Join(specDir, "alpha", "test_comp1.md")
	if err := os.WriteFile(testPath, []byte("# Changed tests\n"), 0644); err != nil {
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

	if result.Summary.Total != 1 {
		t.Fatalf("want no other leaves listed as changed, got %d: %+v", result.Summary.Total, result.Changes)
	}

	foundModified := false
	for _, c := range result.Changes {
		if c.Type == "modified" && c.Path == test1Hash {
			foundModified = true
			if c.NodeType != "test_section" {
				t.Fatalf("want test_section node_type for %s, got %q", test1Hash, c.NodeType)
			}
			if c.Impact != "impl_only" {
				t.Fatalf("want impl_only impact for %s, got %q", test1Hash, c.Impact)
			}
		}
	}
	if !foundModified {
		t.Fatalf("expected modified change for test_section hash %s, got: %+v", test1Hash, result.Changes)
	}
}

func TestFR5_DiffCommand_ImpactClassification(t *testing.T) {
	specDir, comp1Hash, _ := setupDiffTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, tree, time.Now())

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

// S6: --snapshot overrides the comparison target, but it does not bypass
// the pre-flight — a custom baseline is a comparison choice, not an
// exemption from being a project, so the default location must resolve to
// an initialised project independent of what --snapshot points at.
func TestFR4_DiffCommand_CustomSnapshotPath(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())
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
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

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

	stateDir := projectStateDir(specDir)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(stateDir, lifecycle.SnapshotFileName)
	if err := os.WriteFile(snapshotPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, lifecycle.JournalFileName), nil, 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode != exitNotAProject {
		t.Fatalf("want the not-a-project exit code for a broken project, got %d", exitCode)
	}
	if !strings.Contains(stderr, snapshotPath) {
		t.Fatalf("stderr should name the snapshot path %q, got: %s", snapshotPath, stderr)
	}
	if !strings.Contains(stderr, "spex doctor") {
		t.Fatalf("stderr should name 'spex doctor' for a broken project, got: %s", stderr)
	}
}

// E2b: a malformed spec/profile.json is a single early refusal. The profile
// is resolved ahead of any tree building, so the failure surfaces as one
// error naming the profile file, before a diff report ever reaches stdout —
// never as a cascade of downstream node-type-lookup failures.
func TestFR4_E2b_DiffCommand_MalformedProfile(t *testing.T) {
	specDir := setupTestSpec(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	profilePath := filepath.Join(specDir, "profile.json")
	writeTestFile(t, specDir, "profile.json", "{not valid json")

	out, stderr, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode != 1 {
		t.Fatalf("want exit 1 for a malformed profile, got %d\nstderr: %s", exitCode, stderr)
	}
	if out != "" {
		t.Fatalf("no diff report should print once the profile fails to resolve, got stdout: %s", out)
	}
	if !strings.Contains(stderr, profilePath) {
		t.Fatalf("stderr should name the profile file %q, got: %s", profilePath, stderr)
	}
	// The resolution's own single early error, not a tree-builder failure
	// wrapping it: profile resolution must run (and fail) before
	// merkle.BuildTree is ever called, not merely happen to be caught by
	// BuildTree's own internal (and redundant, for this failure) profile
	// resolution once tree building is already underway.
	if strings.Contains(stderr, "build tree") {
		t.Fatalf("profile resolution must fail ahead of tree building, not surface through it, got: %s", stderr)
	}
	if !strings.Contains(stderr, "diff: schema: resolve profile") {
		t.Fatalf("stderr should be the profile resolution's own single early error, got: %s", stderr)
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
	testAHash := schema.IdentityHash("alpha", "test_section", "Test1")

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
		"test_sections": [
			{"id": "` + testAHash + `", "name": "Test1", "content": "test_comp_a.md"}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_comp_a.md", "# CompA architecture\n")
	writeTestFile(t, alphaDir, "arch_comp_b.md", "# CompB architecture\n")
	writeTestFile(t, alphaDir, "test_comp_a.md", "# CompA tests\n")

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
	testAHash := schema.IdentityHash("alpha", "test_section", "Test1")

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
		"test_sections": [
			{"id": "` + testAHash + `", "name": "Test1", "content": "test_comp_a.md"}
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
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	// Against an empty-tree (bootstrap) snapshot, all nodes are "added" —
	// requirements and their implementing components are both added, so no
	// completeness errors.
	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total == 0 {
		t.Fatal("expected changes against an empty-tree snapshot")
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no completeness errors when all nodes are added, got: %+v", result.Errors)
	}
}

func TestNFR6_DiffCommand_Deterministic(t *testing.T) {
	specDir := setupTestSpec(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

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
	seedProjectState(t, specDir, tree, time.Now())

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
		seedProjectState(t, specDir, tree, time.Now())
		testPath := filepath.Join(specDir, "alpha", "test_comp1.md")
		if err := os.WriteFile(testPath, []byte("# Changed tests\n"), 0644); err != nil {
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
		seedProjectState(t, specDir, tree, time.Now())

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
	seedProjectState(t, dir, tree, time.Now())

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

// setupProfileEndpointRemovalSpec builds a fixture whose spec/profile.json —
// the default profile plus one added type, mirroring
// TestS11_BuildTree_ProfileDeclaredContentTypeGetsALeaf's fixture — declares
// a content-bearing, module-scoped "endpoint" type marked name-declarable.
// The endpoint is live at snapshot time and its name survives in prose
// elsewhere in the module; the caller then removes it to exercise the same
// sweep the built-in component/api pair gets, per test_merkle_commands.md's
// second S8: "the sweep iterates the node types the resolved profile marks
// name-declarable ... rather than a fixed pair".
func setupProfileEndpointRemovalSpec(t *testing.T) (specDir, endpointHash string) {
	t.Helper()
	dir := t.TempDir()

	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
		NameDeclarable:  true,
	})
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "profile.json", string(profileJSON))

	modID := schema.IdentityHash("module", "api")
	endpointHash = schema.IdentityHash("api", "endpoint", "GET /v1/widgets")

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [{"id": "`+modID+`", "name": "api", "path": "api"}]
	}`)

	modDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, modDir, "module.json", `{
		"name": "api",
		"endpoints": [
			{"id": "`+endpointHash+`", "name": "GET /v1/widgets", "content": "endpoint_widgets.md"}
		]
	}`)
	writeTestFile(t, modDir, "endpoint_widgets.md", "# GET /v1/widgets\n")
	writeTestFile(t, modDir, "arch_notes.md", "GET /v1/widgets is documented here.\n")

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, dir, tree, time.Now())

	// Remove the endpoint; arch_notes.md still names it.
	writeTestFile(t, modDir, "module.json", `{"name": "api"}`)
	if err := os.Remove(filepath.Join(modDir, "endpoint_widgets.md")); err != nil {
		t.Fatal(err)
	}

	return dir, endpointHash
}

// TestFR8_S8_DiffCommand_ProfileDeclaredTypeSwept covers test_merkle_commands.md's
// second S8: a profile-declared type flagged name-declarable is swept on the
// same terms as the built-in component/api pair, because the sweep iterates
// profile.NodeTypes' NameDeclarable flag rather than a fixed pair.
func TestFR8_S8_DiffCommand_ProfileDeclaredTypeSwept(t *testing.T) {
	specDir, endpointHash := setupProfileEndpointRemovalSpec(t)

	out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 2 {
		t.Fatalf("want exit 2 while a removed endpoint's name survives, got %d\noutput: %s", exitCode, out)
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
		t.Fatalf("want a surviving_name error for the profile-declared endpoint type, got: %+v", result.Errors)
	}
	if !strings.Contains(found.Message, `endpoint "GET /v1/widgets"`) || !strings.Contains(found.Message, endpointHash) {
		t.Fatalf("message must name the node and its hash, got %q", found.Message)
	}
	if found.Path != "api/arch_notes.md:1" {
		t.Fatalf("want the surviving mention as the path, got %q", found.Path)
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
	seedProjectState(t, dir, tree, time.Now())

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

// TestFR8_DiffCommand_JournalRecoversRetiredModule is the wiring for the task
// journal at its resolved location, and the hole it closes now that the
// retired --map flag is gone. The surviving mention of Retiree is identical
// in both runs; only the hash → name source differs. Without a journal the
// sweep can prove nothing about a module whose name was swept out of the
// prose, and discloses that; with the journal ingest already wrote, the same
// run recovers "alpha", reports the survivor and halts the pipeline. The
// sweep's reach must not depend on how thoroughly the module name itself was
// swept.
func TestFR8_DiffCommand_JournalRecoversRetiredModule(t *testing.T) {
	retireeHash := schema.IdentityHash("alpha", "component", "Retiree")

	t.Run("no journal: unverifiable, disclosed but not gating", func(t *testing.T) {
		specDir := setupRetiredModuleSpec(t)
		out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
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

	t.Run("journal: the same survivor is reported", func(t *testing.T) {
		specDir := setupRetiredModuleSpec(t)
		writeTestFile(t, projectStateDir(specDir), lifecycle.JournalFileName,
			`{"event":"removed","eid":"e1","node":"`+retireeHash+`","name":"Retiree","node_type":"component","module":"alpha","before":"deadbeef","after":null,"git_head":"headsha1","proposal":"test-removal"}`+"\n")

		out, _, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
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

// TestFR8_DiffCommand_MalformedJournalDegradesGently pins what actually
// happens to a malformed journal once it lives at its resolved .spex/
// location: the lifecycle pre-flight (arch_diff_command.md's own first
// Responsibilities bullet: "refusing an uninitialised or broken project with
// the pre-flight's own error and exit code") parses that same file before
// CheckRemovedNames's own gentle degradation ever runs, and
// arch_project_resolver.md/test_resolver.md are unambiguous that an
// unparseable journal makes the project broken. So the run is refused with
// the not-a-spex-project exit code naming `spex doctor`, not degraded to a
// note — see drifts/drift-spexmachina-uiei.8-diff-journal.json, which reports
// the leaf's own now-unreachable "degrades gently" sentence. The gentle
// degradation this test's name still describes is exercised directly at the
// library level by
// validator.TestREQ_6f8284df92a2_MalformedJournalStillNotes, which calls
// CheckRemovedNames without going through the pre-flight.
func TestFR8_DiffCommand_MalformedJournalDegradesGently(t *testing.T) {
	specDir := setupRetiredModuleSpec(t)
	writeTestFile(t, projectStateDir(specDir), lifecycle.JournalFileName, "{not valid json\n")

	_, stderr, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != exitNotAProject {
		t.Fatalf("a malformed journal is broken project state once the pre-flight reads it, want exit %d, got %d\nstderr: %s", exitNotAProject, exitCode, stderr)
	}
	if !strings.Contains(stderr, "spex doctor") {
		t.Fatalf("stderr should name 'spex doctor', got: %s", stderr)
	}
}

// E1: a directory missing project.json is an input error (exit 1), not the
// not-a-project refusal — the default snapshot location is present (an
// initialised project), but the authored spec itself will not read.
func TestFR4_E1_DiffCommand_MissingProjectJSON(t *testing.T) {
	dir := t.TempDir()
	seedProjectState(t, dir, merkle.EmptyTree(), time.Now())

	_, stderr, exitCode := runDiff(t, "--spec-dir", dir)
	if exitCode != 1 {
		t.Fatalf("want exit 1 for a missing project.json, got %d\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "project.json") {
		t.Fatalf("stderr should mention project.json, got: %s", stderr)
	}
}

// E3: with no --spec-dir given, the root command's persistent flag default
// ("spec/") resolves beneath the current working directory. spex diff takes
// no positional directory argument.
func TestFR4_E3_DiffCommand_DefaultSpecDir(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeValidSpecFiles(t, specDir)
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

	t.Chdir(tmp)
	out, err := runSpex(t, "diff", "--json")
	if err != nil {
		t.Fatalf("want no error resolving the default spec/ dir, got %v\noutput: %s", err, out)
	}
	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if result.Summary.Total != 0 {
		t.Fatalf("expected 0 changes against the matching default-dir snapshot, got %d", result.Summary.Total)
	}
}

// E4: --json output is pipeable — stdout carries only the JSON payload, no
// ANSI escapes or interactive formatting.
func TestFR4_E4_DiffCommand_Pipeable(t *testing.T) {
	specDir := setupTestSpec(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("stdout must not contain ANSI escape codes, got: %q", out)
	}
	var result diffOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("stdout must be exactly one JSON payload: %v\noutput: %q", err, out)
	}
}

// E7: an uninitialised directory — no snapshot at the resolved default
// location at all — is refused with the stable not-a-spex-project exit
// code, distinct from 1 and 2, and reports nothing as added.
func TestFR4_E7_DiffCommand_UninitialisedIsNotAProject(t *testing.T) {
	specDir := setupTestSpec(t)

	stdout, stderr, exitCode := runDiff(t, "--spec-dir", specDir)
	if exitCode == 1 || exitCode == 2 {
		t.Fatalf("want the not-a-project exit code distinct from 1 and 2, got %d", exitCode)
	}
	if exitCode == 0 {
		t.Fatal("want a non-zero exit code for an uninitialised directory")
	}
	if exitCode != exitNotAProject {
		t.Fatalf("want exitNotAProject (%d), got %d", exitNotAProject, exitCode)
	}
	if !strings.Contains(stderr, "spex init") {
		t.Fatalf("stderr should name 'spex init', got: %s", stderr)
	}
	if strings.Contains(stdout, "added") {
		t.Fatalf("nothing should be reported as added for an uninitialised directory, got: %s", stdout)
	}
}

// writeCompleteIngestFixture writes a minimal changeset (one create op for
// compID) and a matching complete-status receipt, so a test can drive
// `spex ingest` the same way S8 describes: "writing a complete-status
// receipts file and invoking spex ingest". Reconciler only processes the
// ops it is given — it does not need to cover every leaf the diff reports
// — so one op is enough to exercise SnapshotSaver's write path.
func writeCompleteIngestFixture(t *testing.T, dir, compID string) (changesetPath, receiptsPath string) {
	t.Helper()

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
	changesetPath = filepath.Join(dir, "changeset.json")
	writeJSON(t, changesetPath, cs)

	rc := adapters.Receipts{
		Version: adapters.ReceiptsVersion,
		Status:  adapters.StatusComplete,
		Ops: []adapters.OpReceipt{{
			OpID:        "op-0001",
			Status:      adapters.OpStatusOk,
			BeadID:      "bead-1",
			WasExisting: false,
		}},
	}
	receiptsPath = filepath.Join(dir, "receipts.json")
	writeJSON(t, receiptsPath, rc)
	return changesetPath, receiptsPath
}

// runIngestAtResolvedLocation drives the ingest library directly against
// the .spex/ locations the lifecycle pre-flight resolves, standing in for
// `spex ingest` until its own beads (spexmachina-uiei.11..13) migrate the
// CLI command itself off <spec-dir>/.snapshot.json and
// <spec-dir>/.history.jsonl. It proves the diff/ingest bootstrap-then-
// steady-state cycle holds once a writer targets the resolved location,
// same as spex diff's own reader now does.
func runIngestAtResolvedLocation(t *testing.T, specDir, changesetPath, receiptsPath string) {
	t.Helper()
	cs, err := loadChangeset(changesetPath)
	if err != nil {
		t.Fatalf("load changeset: %v", err)
	}
	rc, err := loadReceipts(receiptsPath)
	if err != nil {
		t.Fatalf("load receipts: %v", err)
	}
	ctx, err := lifecycle.Resolve(resolveProjectRoot(specDir))
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}
	graph, err := newIngestSpecGraph(specDir)
	if err != nil {
		t.Fatalf("load spec graph: %v", err)
	}

	reconciler := &ingest.Reconciler{SpecDir: specDir, JournalPath: ctx.JournalPath, SpecGraph: graph}
	if _, err := reconciler.Apply(cs, rc); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	saver := &ingest.Saver{SpecDir: specDir, SnapshotPath: ctx.SnapshotPath}
	if _, err := saver.Save(rc.Status); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
}

// S8: the full bootstrap-then-steady-state cycle through diff alone. A
// fresh diff reports everything added; a real ingest cycle (simulated with
// a complete-status receipts file) writes the first snapshot; two edits
// then show up as exactly two modified leaves; a second ingest converges
// the snapshot; a third diff reports zero changes. Proves diff-then-ingest-
// then-diff converges and that bootstrap and steady state share the same
// component composition.
func TestFR4_S8_DiffCommand_BootstrapThenSteadyStateCycle(t *testing.T) {
	specDir, comp1Hash, _ := setupDiffTestSpec(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now())

	// First diff: everything added.
	out1, _, exit1 := runDiff(t, "--json", "--spec-dir", specDir)
	if exit1 != 0 {
		t.Fatalf("want exit 0 for the first (bootstrap) diff, got %d\noutput: %s", exit1, out1)
	}
	var r1 diffOutput
	if err := json.Unmarshal([]byte(out1), &r1); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out1)
	}
	if r1.Summary.Total == 0 {
		t.Fatal("want every leaf reported as added on the first diff")
	}
	for _, c := range r1.Changes {
		if c.Type != "added" {
			t.Fatalf("first diff should report only added changes, got %q for %s", c.Type, c.Path)
		}
	}

	// First simulated complete ingest writes the first real snapshot.
	runDir := t.TempDir()
	csPath, rcPath := writeCompleteIngestFixture(t, runDir, comp1Hash)
	runIngestAtResolvedLocation(t, specDir, csPath, rcPath)

	// Modify both leaves.
	writeTestFile(t, filepath.Join(specDir, "alpha"), "test_comp1.md", "# Changed tests\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Changed architecture\n")

	// Second diff: exactly two modified leaves.
	out2, _, exit2 := runDiff(t, "--json", "--spec-dir", specDir)
	if exit2 != 0 {
		t.Fatalf("want exit 0 for the second diff, got %d\noutput: %s", exit2, out2)
	}
	var r2 diffOutput
	if err := json.Unmarshal([]byte(out2), &r2); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out2)
	}
	if r2.Summary.Total != 2 {
		t.Fatalf("want exactly 2 modified leaves on the second diff, got %d: %+v", r2.Summary.Total, r2.Changes)
	}
	for _, c := range r2.Changes {
		if c.Type != "modified" {
			t.Fatalf("second diff should report only modified changes, got %q for %s", c.Type, c.Path)
		}
	}

	// Second simulated complete ingest converges the snapshot.
	runIngestAtResolvedLocation(t, specDir, csPath, rcPath)

	// Third diff: zero changes, snapshot now matches current state.
	out3, _, exit3 := runDiff(t, "--json", "--spec-dir", specDir)
	if exit3 != 0 {
		t.Fatalf("want exit 0 for the third diff, got %d\noutput: %s", exit3, out3)
	}
	var r3 diffOutput
	if err := json.Unmarshal([]byte(out3), &r3); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out3)
	}
	if r3.Summary.Total != 0 {
		t.Fatalf("want 0 changes on the third diff, got %d: %+v", r3.Summary.Total, r3.Changes)
	}
}

// S5: modifying a module's module.json (the "meta" leaf) is a structural
// change — the highest impact level. The completeness checker may also flag
// the module's existing components as having an unchanged content leaf; that
// is a separate concern from the impact classification this test verifies.
func TestFR5_S5_DiffCommand_StructuralImpact(t *testing.T) {
	specDir, _, _ := setupDiffTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

	modPath := filepath.Join(specDir, "alpha", "module.json")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, append(data, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	out, stderr, exitCode := runDiff(t, "--json", "--spec-dir", specDir)
	if exitCode != 0 && exitCode != 2 {
		t.Fatalf("want exit 0 or 2, got %d\nstdout: %s\nstderr: %s", exitCode, out, stderr)
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	const metaKey = "meta/000000000001"
	found := false
	for _, c := range result.Changes {
		if c.Path == metaKey {
			found = true
			if c.Type != "modified" {
				t.Fatalf("want modified change for %s, got %q", metaKey, c.Type)
			}
			if c.Impact != "structural" {
				t.Fatalf("want structural impact for %s, got %q", metaKey, c.Impact)
			}
		}
	}
	if !found {
		t.Fatalf("expected modified change for %s, got: %+v", metaKey, result.Changes)
	}
}

// S7: multiple leaves changed together — the JSON output lists both changes
// individually, each carrying the owning module's name, and
// summary.by_impact aggregates by impact level with no per-module aggregate
// alongside it (a caller wanting a module's max impact computes it over the
// changes array itself).
func TestFR5_S7_DiffCommand_ModuleLevelAggregation(t *testing.T) {
	specDir, comp1Hash, test1Hash := setupDiffTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	seedProjectState(t, specDir, tree, time.Now())

	writeTestFile(t, filepath.Join(specDir, "alpha"), "test_comp1.md", "# Changed tests\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Changed architecture\n")

	out, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	for _, key := range []string{"changes", "errors", "summary"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("JSON output missing top-level key %q, got: %s", key, out)
		}
	}
	for key := range raw {
		if key != "changes" && key != "errors" && key != "summary" && key != "notes" {
			t.Fatalf("unexpected top-level key %q — no per-module aggregate is emitted, got: %s", key, out)
		}
	}

	var result diffOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	if result.Summary.Total != 2 {
		t.Fatalf("want exactly 2 changes, got %d: %+v", result.Summary.Total, result.Changes)
	}

	byPath := make(map[string]diffChange, len(result.Changes))
	for _, c := range result.Changes {
		byPath[c.Path] = c
	}

	testChange, ok := byPath[test1Hash]
	if !ok {
		t.Fatalf("expected change for test_section hash %s, got: %+v", test1Hash, result.Changes)
	}
	if testChange.Module != "alpha" {
		t.Fatalf("want module %q for test_section change, got %q", "alpha", testChange.Module)
	}

	archChange, ok := byPath[comp1Hash]
	if !ok {
		t.Fatalf("expected change for component hash %s, got: %+v", comp1Hash, result.Changes)
	}
	if archChange.Module != "alpha" {
		t.Fatalf("want module %q for component change, got %q", "alpha", archChange.Module)
	}

	if result.Summary.ByImpact["impl_only"] != 1 {
		t.Fatalf("want 1 impl_only in summary.by_impact, got %+v", result.Summary.ByImpact)
	}
	if result.Summary.ByImpact["arch_impl"] != 1 {
		t.Fatalf("want 1 arch_impl in summary.by_impact, got %+v", result.Summary.ByImpact)
	}
}
