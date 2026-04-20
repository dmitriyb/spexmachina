package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// setupImpactDiffFile creates a spec dir, builds a tree, snapshots it,
// modifies a file, runs diff, and writes the diff JSON to a temp file.
// Returns the spec dir path and the diff file path.
func setupImpactDiffFile(t *testing.T) (string, string) {
	t.Helper()
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

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff command failed: %v", err)
	}

	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	return specDir, diffFile
}

// setupMappingFile creates a .bead-map.json with the given records.
func setupMappingFile(t *testing.T, dir string, records []mapping.Record) string {
	t.Helper()
	mapPath := filepath.Join(dir, ".bead-map.json")
	store := mapping.NewFileStore(mapPath)
	for _, r := range records {
		if _, err := store.Create(r); err != nil {
			t.Fatal(err)
		}
	}
	return mapPath
}

func TestFR4_ImpactCommand_ProducesReport(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	comp1Hash := schema.IdentityHash("alpha", "component", "Comp1")
	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: comp1Hash, BeadID: "bead-1", BeadType: "task", Module: "alpha", Component: "Comp1", ContentFile: "spec/alpha/arch_comp1.md", SpecHash: "abc123"},
	})

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	// The change.Path (identity hash) matches directly against the mapping
	// record's SpecNodeID (also an identity hash) — no rekeying. Verify
	// the match produced an obsolete+create pair for the existing bead.
	if report.Summary.ObsoleteCount != 1 {
		t.Errorf("want 1 obsolete, got %d", report.Summary.ObsoleteCount)
	}
	if report.Summary.CreateCount < 1 {
		t.Errorf("want at least 1 create, got %d", report.Summary.CreateCount)
	}
	// Check that the create has the old bead ID as lineage
	for _, c := range report.Creates {
		if c.OldBeadID == "bead-1" {
			return // found the replacement create
		}
	}
	t.Error("want create with old_bead_id=bead-1, not found")
}

func TestFR4_ImpactCommand_CreateForUnmatchedNode(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if report.Summary.CreateCount == 0 {
		t.Fatal("expected create actions for unmatched nodes")
	}
}

func TestFR4_ImpactCommand_NoChanges(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	total := report.Summary.CreateCount + report.Summary.ObsoleteCount
	if total != 0 {
		t.Fatalf("expected 0 actions with no changes, got %d", total)
	}
}

func TestNFR5_ImpactCommand_Deterministic(t *testing.T) {
	specDir, diffFile := setupImpactDiffFile(t)

	comp1Hash := schema.IdentityHash("alpha", "component", "Comp1")
	mapPath := setupMappingFile(t, filepath.Dir(specDir), []mapping.Record{
		{SpecNodeID: comp1Hash, BeadID: "bead-1", BeadType: "task", Module: "alpha", Component: "Comp1", ContentFile: "spec/alpha/arch_comp1.md", SpecHash: "abc123"},
	})

	out1, _ := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	out2, _ := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)

	if out1 != out2 {
		t.Fatalf("determinism: outputs differ\nrun1: %s\nrun2: %s", out1, out2)
	}
}

func TestFR4_ImpactCommand_InvalidDiffJSON(t *testing.T) {
	diffFile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(diffFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runSpex(t, "impact", "--diff", diffFile)
	if err == nil {
		t.Fatal("should fail with invalid diff JSON")
	}
}

func TestFR4_ImpactCommand_NonexistentDiffFile(t *testing.T) {
	_, err := runSpex(t, "impact", "--diff", "/nonexistent/diff.json")
	if err == nil {
		t.Fatal("should fail with nonexistent diff file")
	}
}

func TestFR4_ParseDiffJSON(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"},
			{"path": "module/1/impl_section/1", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"},
			{"path": "module/1/component/2", "type": "removed", "impact": "arch_impl", "module": "alpha", "old_hash": "ddd"}
		]
	}`

	changes, _, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(changes) != 3 {
		t.Fatalf("want 3 changes, got %d", len(changes))
	}

	if changes[0].Type != merkle.Modified {
		t.Errorf("change 0: want Modified, got %v", changes[0].Type)
	}
	if changes[0].Impact != merkle.ArchImpl {
		t.Errorf("change 0: want ArchImpl, got %v", changes[0].Impact)
	}
	if changes[0].Module != "alpha" {
		t.Errorf("change 0: want module alpha, got %s", changes[0].Module)
	}
	if changes[1].Type != merkle.Added {
		t.Errorf("change 1: want Added, got %v", changes[1].Type)
	}
	if changes[2].Type != merkle.Removed {
		t.Errorf("change 2: want Removed, got %v", changes[2].Type)
	}
}

func TestFR4_ParseDiffJSON_InvalidType(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "bogus", "impact": "impl_only", "module": "m"}]}`
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown change type") {
		t.Fatalf("want error about unknown change type, got %v", err)
	}
}

func TestFR4_ParseDiffJSON_InvalidImpact(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "added", "impact": "bogus", "module": "m"}]}`
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown impact level") {
		t.Fatalf("want error about unknown impact level, got %v", err)
	}
}

// runSpexWithStderr is like runSpex but also returns stderr output.
func runSpexWithStderr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newHashCmd(),
		newDiffCmd(),
		newValidateCmd(),
		newImpactCmd(),
		newApplyCmd(),
		newMapCmd(),
		newRenderCmd(),
	)

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	var execErr error
	stdout := captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, errBuf.String(), execErr
}

func TestFR8_ImpactCommand_RejectsDiffWithErrors(t *testing.T) {
	specDir := setupTestSpec(t)
	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	diffJSON := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"}
		],
		"errors": [
			{
				"type": "incomplete_change",
				"message": "Requirement 2 (impact) description changed but implementing component NodeMatcher content leaf unchanged",
				"path": "module/4/meta",
				"related": ["module/4/component/2"]
			}
		]
	}`

	diffFile := filepath.Join(t.TempDir(), "diff_with_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("expected error when diff contains errors, got nil")
	}
	if !strings.Contains(err.Error(), "diff contains 1 error(s)") {
		t.Fatalf("expected error about diff errors, got: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty stdout when diff has errors, got: %s", out)
	}
}

func TestFR8_ImpactCommand_EmptyErrorsArrayProceeds(t *testing.T) {
	specDir := setupTestSpec(t)

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Create a diff with empty errors array — should proceed normally.
	diffJSON := `{"changes": [], "errors": []}`
	diffFile := filepath.Join(t.TempDir(), "empty_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("expected no error with empty errors array, got: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
}

func TestFR8_ParseDiffJSON_ExtractsErrors(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"}
		],
		"errors": [
			{"type": "incomplete_change", "message": "something broken", "path": "module/1/meta", "related": ["module/1/component/1"]}
		]
	}`

	changes, diffErrors, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(changes))
	}
	if len(diffErrors) != 1 {
		t.Fatalf("want 1 error, got %d", len(diffErrors))
	}
	if diffErrors[0].Type != "incomplete_change" {
		t.Errorf("want error type incomplete_change, got %s", diffErrors[0].Type)
	}
	if diffErrors[0].Message != "something broken" {
		t.Errorf("want error message 'something broken', got %s", diffErrors[0].Message)
	}
}

func TestFR8_ParseDiffJSON_NoErrorsField(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"}
		]
	}`

	changes, diffErrors, err := parseDiffJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(changes))
	}
	if len(diffErrors) != 0 {
		t.Fatalf("want 0 errors when field absent, got %d", len(diffErrors))
	}
}

// TestFR7_ImpactCommand_ResolvesDepBeadIDs verifies that the impact command
// populates DepBeadIDs on create actions via ResolveDeps post-processing.
// Setup: module "beta" requires module "alpha". Alpha has an open bead.
// When beta's component is modified, the create action should carry alpha's
// open bead ID in DepBeadIDs (requires_module transitive resolution).
func TestFR7_ImpactCommand_ResolvesDepBeadIDs(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	alphaID := schema.IdentityHash("module", "alpha")
	betaID := schema.IdentityHash("module", "beta")
	alphaCompID := schema.IdentityHash("alpha", "component", "AlphaComp")
	alphaImplID := schema.IdentityHash("alpha", "impl_section", "AlphaImpl")
	alphaTestID := schema.IdentityHash("alpha", "test_section", "AlphaTest")
	betaCompID := schema.IdentityHash("beta", "component", "BetaComp")
	betaImplID := schema.IdentityHash("beta", "impl_section", "BetaImpl")
	betaTestID := schema.IdentityHash("beta", "test_section", "BetaTest")

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": "` + alphaID + `", "name": "alpha", "path": "alpha"},
			{"id": "` + betaID + `", "name": "beta", "path": "beta", "requires_module": ["` + alphaID + `"]}
		]
	}`
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", proj)

	alphaDir := filepath.Join(specDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+alphaCompID+`", "name": "AlphaComp", "content": "arch_alpha.md"}
		],
		"impl_sections": [
			{"id": "`+alphaImplID+`", "name": "AlphaImpl", "content": "impl_alpha.md"}
		],
		"test_sections": [
			{"id": "`+alphaTestID+`", "name": "AlphaTest", "content": "test_alpha.md", "describes": ["`+alphaCompID+`"]}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_alpha.md", "# Alpha comp\n")
	writeTestFile(t, alphaDir, "impl_alpha.md", "# Alpha impl\n")
	writeTestFile(t, alphaDir, "test_alpha.md", "# Alpha test\n")

	betaDir := filepath.Join(specDir, "beta")
	if err := os.MkdirAll(betaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, betaDir, "module.json", `{
		"name": "beta",
		"components": [
			{"id": "`+betaCompID+`", "name": "BetaComp", "content": "arch_beta.md"}
		],
		"impl_sections": [
			{"id": "`+betaImplID+`", "name": "BetaImpl", "content": "impl_beta.md"}
		],
		"test_sections": [
			{"id": "`+betaTestID+`", "name": "BetaTest", "content": "test_beta.md", "describes": ["`+betaCompID+`"]}
		]
	}`)
	writeTestFile(t, betaDir, "arch_beta.md", "# Beta comp\n")
	writeTestFile(t, betaDir, "impl_beta.md", "# Beta impl\n")
	writeTestFile(t, betaDir, "test_beta.md", "# Beta test\n")

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify beta's arch file to trigger a change.
	writeTestFile(t, betaDir, "arch_beta.md", "# Beta comp CHANGED\n")

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	diffDir := t.TempDir()
	diffFile := filepath.Join(diffDir, "diff.json")
	writeTestFile(t, diffDir, "diff.json", diffJSON)

	mapPath := setupMappingFile(t, dir, []mapping.Record{
		{SpecNodeID: alphaCompID, BeadID: "bead-alpha", BeadType: "feature", Module: "alpha", Component: "AlphaComp", ContentFile: "spec/alpha/arch_alpha.md", SpecHash: "aaa", BeadStatus: "open"},
		{SpecNodeID: betaCompID, BeadID: "bead-beta", BeadType: "feature", Module: "beta", Component: "BetaComp", ContentFile: "spec/beta/arch_beta.md", SpecHash: "bbb", BeadStatus: "open"},
	})

	// Run impact.
	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	// Find the create action for beta's component — it should have
	// bead-alpha in DepBeadIDs (via requires_module resolution).
	for _, c := range report.Creates {
		if c.Module == "beta" && c.OldBeadID == "bead-beta" {
			if len(c.DepBeadIDs) == 0 {
				t.Fatal("want DepBeadIDs populated for beta create, got empty")
			}
			for _, dep := range c.DepBeadIDs {
				if dep == "bead-alpha" {
					return // success
				}
			}
			t.Fatalf("want bead-alpha in DepBeadIDs, got %v", c.DepBeadIDs)
		}
	}
	t.Fatal("create action for beta component with OldBeadID=bead-beta not found")
}

// TestFR7_ImpactCommand_UsesEdgePopulatesDepBeadIDs verifies that intra-module
// component `uses` edges are resolved into DepBeadIDs.
func TestFR7_ImpactCommand_UsesEdgePopulatesDepBeadIDs(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "mod")
	baseID := schema.IdentityHash("mod", "component", "Base")
	userID := schema.IdentityHash("mod", "component", "User")
	baseImplID := schema.IdentityHash("mod", "impl_section", "BaseImpl")
	userImplID := schema.IdentityHash("mod", "impl_section", "UserImpl")
	baseTestID := schema.IdentityHash("mod", "test_section", "BaseTest")
	userTestID := schema.IdentityHash("mod", "test_section", "UserTest")

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": "` + modID + `", "name": "mod", "path": "mod"}
		]
	}`
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", proj)

	modDir := filepath.Join(specDir, "mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Component User uses component Base.
	writeTestFile(t, modDir, "module.json", `{
		"name": "mod",
		"components": [
			{"id": "`+baseID+`", "name": "Base", "content": "arch_base.md"},
			{"id": "`+userID+`", "name": "User", "content": "arch_user.md", "uses": ["`+baseID+`"]}
		],
		"impl_sections": [
			{"id": "`+baseImplID+`", "name": "BaseImpl", "content": "impl_base.md"},
			{"id": "`+userImplID+`", "name": "UserImpl", "content": "impl_user.md"}
		],
		"test_sections": [
			{"id": "`+baseTestID+`", "name": "BaseTest", "content": "test_base.md", "describes": ["`+baseID+`"]},
			{"id": "`+userTestID+`", "name": "UserTest", "content": "test_user.md", "describes": ["`+userID+`"]}
		]
	}`)
	writeTestFile(t, modDir, "arch_base.md", "# Base\n")
	writeTestFile(t, modDir, "arch_user.md", "# User\n")
	writeTestFile(t, modDir, "impl_base.md", "# Base impl\n")
	writeTestFile(t, modDir, "impl_user.md", "# User impl\n")
	writeTestFile(t, modDir, "test_base.md", "# Base test\n")
	writeTestFile(t, modDir, "test_user.md", "# User test\n")

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify User's arch file.
	writeTestFile(t, modDir, "arch_user.md", "# User CHANGED\n")

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	diffDir := t.TempDir()
	diffFile := filepath.Join(diffDir, "diff.json")
	writeTestFile(t, diffDir, "diff.json", diffJSON)

	mapPath := setupMappingFile(t, dir, []mapping.Record{
		{SpecNodeID: baseID, BeadID: "bead-base", BeadType: "feature", Module: "mod", Component: "Base", ContentFile: "spec/mod/arch_base.md", SpecHash: "aaa", BeadStatus: "open"},
		{SpecNodeID: userID, BeadID: "bead-user", BeadType: "feature", Module: "mod", Component: "User", ContentFile: "spec/mod/arch_user.md", SpecHash: "bbb", BeadStatus: "open"},
	})

	out, err := runSpex(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	for _, c := range report.Creates {
		if c.Module == "mod" && c.OldBeadID == "bead-user" {
			for _, dep := range c.DepBeadIDs {
				if dep == "bead-base" {
					return // success: uses edge resolved
				}
			}
			t.Fatalf("want bead-base in DepBeadIDs (uses edge), got %v", c.DepBeadIDs)
		}
	}
	t.Fatal("create action for User component with OldBeadID=bead-user not found")
}

// TestEnrichRecordsWithBeadStatus verifies the helper copies live bead
// statuses onto mapping records by matching on the spex:<record-id> label.
// Regression guard for spexmachina-idd: previously BeadStatus was never
// populated on production code paths, so cleanup creates never fired.
func TestEnrichRecordsWithBeadStatus(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "br-stub")
	jsonFile := filepath.Join(dir, "beads.json")

	// Envelope shape with three beads: one closed, one open, one unrelated.
	// Record 99 has no corresponding bead — its BeadStatus must stay empty.
	payload := `{"issues":[
		{"id":"bead-1","status":"closed","labels":["spex:10"]},
		{"id":"bead-2","status":"open","labels":["spex:11"]},
		{"id":"unrelated","status":"open","labels":["team:backend"]}
	]}`
	if err := os.WriteFile(jsonFile, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ncat "+jsonFile+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	records := []mapping.Record{
		{ID: 10, BeadID: "bead-1", SpecNodeID: "aaa", Module: "alpha", Component: "A", ContentFile: "a.md"},
		{ID: 11, BeadID: "bead-2", SpecNodeID: "bbb", Module: "alpha", Component: "B", ContentFile: "b.md"},
		{ID: 99, BeadID: "bead-missing", SpecNodeID: "ccc", Module: "alpha", Component: "Gone", ContentFile: "c.md"},
	}

	enriched, err := enrichRecordsWithBeadStatus(t.Context(), stub, records)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(enriched) != 3 {
		t.Fatalf("want 3 records, got %d", len(enriched))
	}
	if enriched[0].BeadStatus != "closed" {
		t.Errorf("record 10: want closed, got %q", enriched[0].BeadStatus)
	}
	if enriched[1].BeadStatus != "open" {
		t.Errorf("record 11: want open, got %q", enriched[1].BeadStatus)
	}
	if enriched[2].BeadStatus != "" {
		t.Errorf("record 99 (no matching bead): want empty, got %q", enriched[2].BeadStatus)
	}
}

// TestEnrichRecordsWithBeadStatus_EmptyInput verifies zero-record input
// short-circuits without invoking the bead CLI.
func TestEnrichRecordsWithBeadStatus_EmptyInput(t *testing.T) {
	out, err := enrichRecordsWithBeadStatus(t.Context(), "/does/not/exist", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("want nil slice, got %v", out)
	}
}

func TestFR8_ImpactCommand_MultipleErrorsAllPrinted(t *testing.T) {
	specDir := setupTestSpec(t)
	mapPath := setupMappingFile(t, filepath.Dir(specDir), nil)

	diffJSON := `{
		"changes": [],
		"errors": [
			{"type": "incomplete_change", "message": "first error message", "path": "module/1/meta", "related": []},
			{"type": "incomplete_change", "message": "second error message", "path": "module/2/meta", "related": []}
		]
	}`

	diffFile := filepath.Join(t.TempDir(), "multi_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runSpexWithStderr(t, "impact", "--diff", diffFile, "--map", mapPath, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("expected error when diff contains errors, got nil")
	}
	if !strings.Contains(err.Error(), "diff contains 2 error(s)") {
		t.Fatalf("expected error about 2 diff errors, got: %v", err)
	}
	if !strings.Contains(stderr, "first error message") {
		t.Fatalf("expected first error in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "second error message") {
		t.Fatalf("expected second error in stderr, got: %s", stderr)
	}
}
