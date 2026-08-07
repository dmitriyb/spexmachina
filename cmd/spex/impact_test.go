package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// impactFixture is the spec/diff/journal/beads harness from
// spec/impact/test_impact_command.md's Setup section: a spec tree with
// validator/ and merkle/ modules, a pre-change snapshot, a diff whose
// changes are SchemaChecker (modified), CoupledSectionChecker (added),
// Hasher (modified) and DiffEngine (removed), and a three-entry journal
// tying each surviving node to its task (spex-001, spex-003, spex-010).
type impactFixture struct {
	specDir         string
	diffFile        string
	beadsFile       string // spex-010 open — no cleanup gate fires
	beadsClosedFile string // spex-010 closed — cleanup gate fires
	schkID          string
	cscID           string
	hasrID          string
	diffID          string
}

func setupImpactCommandFixture(t *testing.T) impactFixture {
	t.Helper()
	specDir := t.TempDir()

	validatorModID := schema.IdentityHash("module", "validator")
	merkleModID := schema.IdentityHash("module", "merkle")
	schkID := schema.IdentityHash("validator", "component", "SchemaChecker")
	cscID := schema.IdentityHash("validator", "component", "CoupledSectionChecker")
	hasrID := schema.IdentityHash("merkle", "component", "Hasher")
	diffID := schema.IdentityHash("merkle", "component", "DiffEngine")

	writeTestFile(t, specDir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "`+validatorModID+`", "name": "validator", "path": "validator"},
			{"id": "`+merkleModID+`", "name": "merkle", "path": "merkle"}
		]
	}`)

	validatorDir := filepath.Join(specDir, "validator")
	if err := os.MkdirAll(validatorDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, validatorDir, "module.json", `{
		"name": "validator",
		"components": [
			{"id": "`+schkID+`", "name": "SchemaChecker", "content": "arch_schema_checker.md"}
		]
	}`)
	writeTestFile(t, validatorDir, "arch_schema_checker.md", "# SchemaChecker\n")

	merkleDir := filepath.Join(specDir, "merkle")
	if err := os.MkdirAll(merkleDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, merkleDir, "module.json", `{
		"name": "merkle",
		"components": [
			{"id": "`+hasrID+`", "name": "Hasher", "content": "arch_hasher.md"},
			{"id": "`+diffID+`", "name": "DiffEngine", "content": "arch_diff_engine.md"}
		]
	}`)
	writeTestFile(t, merkleDir, "arch_hasher.md", "# Hasher\n")
	writeTestFile(t, merkleDir, "arch_diff_engine.md", "# DiffEngine\n")

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Apply the changes the setup section documents.
	writeTestFile(t, validatorDir, "arch_schema_checker.md", "# SchemaChecker CHANGED\n")
	writeTestFile(t, validatorDir, "module.json", `{
		"name": "validator",
		"components": [
			{"id": "`+schkID+`", "name": "SchemaChecker", "content": "arch_schema_checker.md"},
			{"id": "`+cscID+`", "name": "CoupledSectionChecker", "content": "arch_coupled_section_checker.md"}
		]
	}`)
	writeTestFile(t, validatorDir, "arch_coupled_section_checker.md", "# CoupledSectionChecker\n")

	writeTestFile(t, merkleDir, "arch_hasher.md", "# Hasher CHANGED\n")
	writeTestFile(t, merkleDir, "module.json", `{
		"name": "merkle",
		"components": [
			{"id": "`+hasrID+`", "name": "Hasher", "content": "arch_hasher.md"}
		]
	}`)
	if err := os.Remove(filepath.Join(merkleDir, "arch_diff_engine.md")); err != nil {
		t.Fatal(err)
	}

	diffJSON, err := runSpex(t, "diff", "--json", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	writeTestJournal(t, specDir, []string{
		fmt.Sprintf(`{"event":"added","eid":"E1","node":"%s","name":"SchemaChecker","node_type":"component","module":"validator","before":null,"after":"h1","git_head":"headsha1","proposal":"p1"}`, schkID),
		`{"event":"task_created","for":"E1","task_id":"spex-001"}`,
		fmt.Sprintf(`{"event":"added","eid":"E2","node":"%s","name":"Hasher","node_type":"component","module":"merkle","before":null,"after":"h2","git_head":"headsha1","proposal":"p1"}`, hasrID),
		`{"event":"task_created","for":"E2","task_id":"spex-003"}`,
		fmt.Sprintf(`{"event":"added","eid":"E3","node":"%s","name":"DiffEngine","node_type":"component","module":"merkle","before":null,"after":"h3","git_head":"headsha1","proposal":"p1"}`, diffID),
		`{"event":"task_created","for":"E3","task_id":"spex-010"}`,
	})

	beadsFile := filepath.Join(t.TempDir(), "beads.json")
	if err := os.WriteFile(beadsFile, []byte(fmt.Sprintf(`{"issues":[
		{"id":"spex-001","status":"open","labels":["spex:%s"]},
		{"id":"spex-003","status":"open","labels":["spex:%s"]},
		{"id":"spex-010","status":"open","labels":["spex:%s"]}
	]}`, schkID, hasrID, diffID)), 0o644); err != nil {
		t.Fatal(err)
	}

	beadsClosedFile := filepath.Join(t.TempDir(), "beads_closed.json")
	if err := os.WriteFile(beadsClosedFile, []byte(fmt.Sprintf(`{"issues":[
		{"id":"spex-001","status":"open","labels":["spex:%s"]},
		{"id":"spex-003","status":"open","labels":["spex:%s"]},
		{"id":"spex-010","status":"closed","labels":["spex:%s"]}
	]}`, schkID, hasrID, diffID)), 0o644); err != nil {
		t.Fatal(err)
	}

	return impactFixture{
		specDir:         specDir,
		diffFile:        diffFile,
		beadsFile:       beadsFile,
		beadsClosedFile: beadsClosedFile,
		schkID:          schkID,
		cscID:           cscID,
		hasrID:          hasrID,
		diffID:          diffID,
	}
}

// writeEmptyBeadsFile writes an empty tracker-list JSON to a temp file for
// tests that don't care about bead enrichment but need a valid --beads
// input.
func writeEmptyBeadsFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "beads.json")
	if err := os.WriteFile(path, []byte(`{"issues":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runSpexWithStderr is like runSpex but also returns stderr output.
func runSpexWithStderr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newDiffCmd(),
		newValidateCmd(),
		newImpactCmd(),
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

// runSpexStdin runs the CLI with os.Stdin fed from the given string. This is
// separate from cobra's SetIn: ImpactCommand reads os.Stdin directly (it
// must accept a real pipe, per the Interface contract), not cmd.InOrStdin().
func runSpexStdin(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })

	go func() {
		_, _ = io.WriteString(w, stdin)
		w.Close()
	}()

	return runSpex(t, args...)
}

func hasCleanupCreate(report impact.ImpactReport, oldBeadID string) bool {
	for _, c := range report.Creates {
		if c.OldBeadID == oldBeadID && strings.Contains(c.Reason, "Code cleanup") {
			return true
		}
	}
	return false
}

// S1: Full pipeline — diff file to JSON report on stdout.
func TestS1_FullPipeline(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if len(report.Creates) != 3 {
		t.Fatalf("want 3 creates, got %d: %+v", len(report.Creates), report.Creates)
	}
	wantCreates := []struct {
		Module, Node, Reason, OldBeadID string
	}{
		{"merkle", "Hasher", "Spec node modified (new): merkle/Hasher", "spex-003"},
		{"validator", "CoupledSectionChecker", "New spec node: validator/CoupledSectionChecker", ""},
		{"validator", "SchemaChecker", "Spec node modified (new): validator/SchemaChecker", "spex-001"},
	}
	for i, w := range wantCreates {
		c := report.Creates[i]
		if c.Module != w.Module || c.Node != w.Node || c.Reason != w.Reason || c.OldBeadID != w.OldBeadID {
			t.Errorf("create[%d]: want %+v, got %+v", i, w, c)
		}
	}

	if len(report.Obsoletes) != 3 {
		t.Fatalf("want 3 obsoletes, got %d: %+v", len(report.Obsoletes), report.Obsoletes)
	}
	wantObsoletes := []struct {
		BeadID, Module, Node, Reason, ChangeType string
	}{
		{"spex-010", "merkle", "DiffEngine", "Spec node removed: merkle/DiffEngine", "removed"},
		{"spex-003", "merkle", "Hasher", "Spec node modified: merkle/Hasher", "modified"},
		{"spex-001", "validator", "SchemaChecker", "Spec node modified: validator/SchemaChecker", "modified"},
	}
	for i, w := range wantObsoletes {
		o := report.Obsoletes[i]
		if o.BeadID != w.BeadID || o.Module != w.Module || o.Node != w.Node || o.Reason != w.Reason || o.ChangeType != w.ChangeType {
			t.Errorf("obsolete[%d]: want %+v, got %+v", i, w, o)
		}
	}

	if report.Summary.CreateCount != 3 || report.Summary.ObsoleteCount != 3 {
		t.Errorf("want summary {3,3}, got %+v", report.Summary)
	}
}

// S2: Diff input from stdin (pipe).
func TestS2_StdinDiff(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	want, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	diffBytes, err := os.ReadFile(fx.diffFile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := runSpexStdin(t, string(diffBytes), "impact", "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if got != want {
		t.Fatalf("stdin output differs from file output\nwant: %s\ngot: %s", want, got)
	}
}

// S3: Diff input from stdin with --diff set to "-".
func TestS3_StdinWithDashFlag(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	want, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	diffBytes, err := os.ReadFile(fx.diffFile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := runSpexStdin(t, string(diffBytes), "impact", "--diff", "-", "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if got != want {
		t.Fatalf("--diff - output differs from file output\nwant: %s\ngot: %s", want, got)
	}
}

// S4: No changes — empty diff produces empty report.
func TestS4_EmptyDiffProducesEmptyReport(t *testing.T) {
	specDir := setupTestSpec(t)
	diffFile := filepath.Join(t.TempDir(), "empty_diff.json")
	if err := os.WriteFile(diffFile, []byte(`{"changes": [], "errors": []}`), 0644); err != nil {
		t.Fatal(err)
	}
	beadsFile := writeEmptyBeadsFile(t)

	out, err := runSpex(t, "impact", "--diff", diffFile, "--beads", beadsFile, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("empty diff is not an error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
	if len(report.Creates) != 0 || len(report.Obsoletes) != 0 {
		t.Fatalf("want empty creates/obsoletes, got %+v", report)
	}
	if report.Summary.CreateCount != 0 || report.Summary.ObsoleteCount != 0 {
		t.Fatalf("want zero counts, got %+v", report.Summary)
	}
}

// S5: --json flag is accepted explicitly (default and only supported format).
func TestS5_ExplicitJSONFlag(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--json", "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
}

// S6: Pipeline composition — spex diff piped into spex impact.
func TestS6_PipelineComposition(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	diffOut, err := runSpex(t, "diff", "--json", "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	out, err := runSpexStdin(t, diffOut, "impact", "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
	if report.Summary.CreateCount == 0 && report.Summary.ObsoleteCount == 0 {
		t.Fatal("want a non-empty report from the composed pipeline")
	}
}

// S7: Exit code 0 on success, exit code 1 on error.
func TestS7_ExitCodes(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	if _, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir); err != nil {
		t.Fatalf("want exit 0, got %v", err)
	}

	_, stderr, err := runSpexWithStderr(t, "impact", "--diff", "nonexistent_file.json", "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err == nil {
		t.Fatal("want exit 1 for missing diff file")
	}
	if !strings.Contains(err.Error(), "read diff") && !strings.Contains(stderr, "read diff") {
		t.Fatalf("want error about the missing file, got err=%v stderr=%s", err, stderr)
	}
}

// S8: --beads drives the cleanup gate.
func TestS8_BeadsDrivesCleanupGate(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsClosedFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}

	if report.Summary.CreateCount != 4 || report.Summary.ObsoleteCount != 3 {
		t.Fatalf("want summary {4,3}, got %+v", report.Summary)
	}
	if !hasCleanupCreate(report, "spex-010") {
		t.Fatalf("want a cleanup create for spex-010 (merkle/DiffEngine), creates=%+v", report.Creates)
	}
}

// S9: --beads omitted, and --bead-cli supplied, are both inert.
func TestS9_BeadsOmittedAndBeadCLIInert(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	base, err := runSpex(t, "impact", "--diff", fx.diffFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(base), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, base)
	}
	if report.Summary.CreateCount != 3 || report.Summary.ObsoleteCount != 3 {
		t.Fatalf("want summary {3,3} without --beads, got %+v", report.Summary)
	}
	if hasCleanupCreate(report, "spex-010") {
		t.Fatalf("want no cleanup create without --beads; creates=%+v", report.Creates)
	}

	for _, beadCLI := range []string{"br", "bd", "./anything"} {
		out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--bead-cli", beadCLI, "--spec-dir", fx.specDir)
		if err != nil {
			t.Fatalf("--bead-cli %s: want no error, got %v", beadCLI, err)
		}
		if out != base {
			t.Fatalf("--bead-cli %s: want byte-identical output to the flag-omitted run\nbase: %s\ngot: %s", beadCLI, base, out)
		}
	}
}

// S10: Deterministic output across runs.
func TestS10_Deterministic(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	var outputs []string
	for i := 0; i < 5; i++ {
		out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
		if err != nil {
			t.Fatalf("run %d: want no error, got %v", i, err)
		}
		outputs = append(outputs, out)
	}
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Fatalf("run %d differs from run 0\nrun0: %s\nrun%d: %s", i, outputs[0], i, outputs[i])
		}
	}
}

// S11: Report output is suitable for piping to spex emit — JSON only,
// terminated by a newline, nothing else on stdout.
func TestS11_ReportSuitableForPiping(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", fx.beadsFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("want stdout to end in a newline, got %q", out)
	}
	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("stdout must contain only the JSON report: %v\noutput: %q", err, out)
	}
}

// S12: Large diff with many changes completes quickly and reports correctly.
func TestS12_LargeDiffPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-diff performance test in short mode")
	}

	specDir := t.TempDir()

	const numModules = 20
	const compsPerModule = 25 // 500 total changes

	type changeSpec struct {
		path, module, nodeType string
	}

	var projModules []string
	var changes []changeSpec
	for m := 0; m < numModules; m++ {
		modName := fmt.Sprintf("mod%02d", m)
		modID := schema.IdentityHash("module", modName)
		projModules = append(projModules, fmt.Sprintf(`{"id":"%s","name":"%s","path":"%s"}`, modID, modName, modName))

		modDir := filepath.Join(specDir, modName)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatal(err)
		}

		var comps []string
		for c := 0; c < compsPerModule; c++ {
			compName := fmt.Sprintf("Comp%02d", c)
			compID := schema.IdentityHash(modName, "component", compName)
			content := fmt.Sprintf("arch_%s.md", strings.ToLower(compName))
			comps = append(comps, fmt.Sprintf(`{"id":"%s","name":"%s","content":"%s"}`, compID, compName, content))
			writeTestFile(t, modDir, content, "# "+compName+"\n")
			changes = append(changes, changeSpec{path: compID, module: modName, nodeType: "component"})
		}
		writeTestFile(t, modDir, "module.json", `{"name":"`+modName+`","components":[`+strings.Join(comps, ",")+`]}`)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"perf-test","modules":[`+strings.Join(projModules, ",")+`]}`)

	var changeJSON []string
	for _, c := range changes {
		changeJSON = append(changeJSON, fmt.Sprintf(
			`{"path":"%s","type":"modified","impact":"arch_impl","module":"%s","node_type":"%s","old_hash":"aaa","new_hash":"bbb"}`,
			c.path, c.module, c.nodeType))
	}
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(`{"changes":[`+strings.Join(changeJSON, ",")+`],"errors":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	var beadLines []string
	for i := 0; i < 300; i++ {
		m := i % numModules
		c := i % compsPerModule
		compID := schema.IdentityHash(fmt.Sprintf("mod%02d", m), "component", fmt.Sprintf("Comp%02d", c))
		beadLines = append(beadLines, fmt.Sprintf(`{"id":"bead-%d","status":"open","labels":["spex:%s"]}`, i, compID))
	}
	beadsFile := filepath.Join(t.TempDir(), "beads.json")
	if err := os.WriteFile(beadsFile, []byte(`{"issues":[`+strings.Join(beadLines, ",")+`]}`), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	out, err := runSpex(t, "impact", "--diff", diffFile, "--beads", beadsFile, "--spec-dir", specDir)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("want completion under 5s, took %v", elapsed)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v", err)
	}

	wantCreates := numModules * compsPerModule
	if report.Summary.CreateCount != wantCreates {
		t.Errorf("want %d creates (every modified node is unmatched, no journal entries), got %d", wantCreates, report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 0 {
		t.Errorf("want 0 obsoletes, got %d", report.Summary.ObsoleteCount)
	}
}

// E1: --beads names a file that does not exist.
func TestE1_BeadsFileMissing(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	out, _, err := runSpexWithStderr(t, "impact", "--diff", fx.diffFile, "--beads", "/nonexistent/beads.json", "--spec-dir", fx.specDir)
	if err == nil {
		t.Fatal("want error for missing --beads file")
	}
	if !strings.Contains(err.Error(), "impact: read beads:") {
		t.Fatalf("want 'impact: read beads:' context, got %v", err)
	}
	if out != "" {
		t.Fatalf("want empty stdout, got %q", out)
	}
}

// E2: Bead file parses but names no spec-managed bead — dropped, not an
// error; a bead with no id at all is malformed input and IS an error.
func TestE2_BeadFileNoSpecManagedBeads(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	noLabels := filepath.Join(t.TempDir(), "beads_no_labels.json")
	if err := os.WriteFile(noLabels, []byte(`[{"id":"bead-x","status":"open","labels":["other:thing"]}]`), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", noLabels, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want exit 0 (unmanaged beads are dropped, not an error), got %v", err)
	}

	base, err := runSpex(t, "impact", "--diff", fx.diffFile, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if out != base {
		t.Fatalf("want output to match the --beads-omitted run\nwith-unmanaged-beads: %s\nomitted: %s", out, base)
	}
}

func TestE2_BeadFileMissingID(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	badFile := filepath.Join(t.TempDir(), "beads_missing_id.json")
	if err := os.WriteFile(badFile, []byte(`[{"status":"open","labels":["spex:`+fx.schkID+`"]}]`), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runSpexWithStderr(t, "impact", "--diff", fx.diffFile, "--beads", badFile, "--spec-dir", fx.specDir)
	if err == nil {
		t.Fatal("want error for a bead with no id")
	}
	if !strings.Contains(err.Error(), "index 0") && !strings.Contains(stderr, "index 0") {
		t.Fatalf("want error naming the offending index, got err=%v stderr=%s", err, stderr)
	}
}

// E3: Bead file holds malformed JSON.
func TestE3_BeadFileMalformedJSON(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	badFile := filepath.Join(t.TempDir(), "beads_broken.json")
	if err := os.WriteFile(badFile, []byte(`{"broken":`), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", badFile, "--spec-dir", fx.specDir)
	if err == nil {
		t.Fatal("want error for malformed beads JSON")
	}
	if !strings.Contains(err.Error(), "impact: read beads:") {
		t.Fatalf("want 'impact: read beads:' context, got %v", err)
	}
	if out != "" {
		t.Fatalf("want empty stdout, got %q", out)
	}
}

// E4: Diff file contains malformed JSON.
func TestE4_DiffFileMalformedJSON(t *testing.T) {
	specDir := setupTestSpec(t)
	diffFile := filepath.Join(t.TempDir(), "bad_diff.json")
	if err := os.WriteFile(diffFile, []byte(`[{"path": "foo"`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for malformed diff JSON")
	}
	if !strings.Contains(err.Error(), "parse diff JSON") {
		t.Fatalf("want error referencing diff parsing, got %v", err)
	}
}

// E5: Diff file with zero-length content.
func TestE5_DiffFileEmpty(t *testing.T) {
	specDir := setupTestSpec(t)
	diffFile := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(diffFile, []byte(``), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for a zero-length diff file")
	}
}

// E7: Spec directory with no modules — unmatched changes still report as
// creates and the command does not panic on the missing module.json.
func TestE7_DiffReferencesUnknownModules(t *testing.T) {
	specDir := t.TempDir()
	writeTestFile(t, specDir, "project.json", `{"name":"empty-project","modules":[]}`)

	ghostID := schema.IdentityHash("ghost", "component", "GhostComp")
	diffJSON := fmt.Sprintf(`{"changes":[{"path":"%s","type":"added","impact":"arch_impl","module":"ghost","node_type":"component","new_hash":"zzz"}],"errors":[]}`, ghostID)
	diffFile := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error/panic for an unknown module, got %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
	if report.Summary.CreateCount != 1 {
		t.Errorf("want 1 create for the unmatched added node, got %d", report.Summary.CreateCount)
	}
}

// E8: Bead file carries two beads claiming the same node. The join keeps one
// status per node key, and the LAST entry in the file wins — order, not
// status value, decides the outcome.
func TestE8_DuplicateBeadClaimsLastWins(t *testing.T) {
	fx := setupImpactCommandFixture(t)

	closedLast := filepath.Join(t.TempDir(), "beads_closed_last.json")
	if err := os.WriteFile(closedLast, []byte(fmt.Sprintf(`{"issues":[
		{"id":"claim-a","status":"open","labels":["spex:%s"]},
		{"id":"claim-b","status":"closed","labels":["spex:%s"]}
	]}`, fx.diffID, fx.diffID)), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", closedLast, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
	if !hasCleanupCreate(report, "spex-010") {
		t.Fatalf("want a cleanup create when the LAST claim on the node is closed; creates=%+v", report.Creates)
	}

	openLast := filepath.Join(t.TempDir(), "beads_open_last.json")
	if err := os.WriteFile(openLast, []byte(fmt.Sprintf(`{"issues":[
		{"id":"claim-a","status":"closed","labels":["spex:%s"]},
		{"id":"claim-b","status":"open","labels":["spex:%s"]}
	]}`, fx.diffID, fx.diffID)), 0644); err != nil {
		t.Fatal(err)
	}

	out2, err := runSpex(t, "impact", "--diff", fx.diffFile, "--beads", openLast, "--spec-dir", fx.specDir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	var report2 impact.ImpactReport
	if err := json.Unmarshal([]byte(out2), &report2); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out2)
	}
	if hasCleanupCreate(report2, "spex-010") {
		t.Fatalf("want no cleanup create when the LAST claim on the node is open; creates=%+v", report2.Creates)
	}
}

// E9: Diff input contains errors — impact refuses to proceed.
func TestE9_DiffWithErrorsRefusesToProceed(t *testing.T) {
	specDir := setupTestSpec(t)

	diffJSON := `{
		"changes": [
			{"path": "a1b2c3d4e5f6", "type": "modified", "impact": "arch_impl", "module": "impact", "node_type": "component"}
		],
		"errors": [
			{
				"type": "incomplete_change",
				"message": "Requirement 'Match changed nodes to beads' (impact) description changed but implementing component NodeMatcher content leaf unchanged",
				"path": "0011223344aa",
				"related": ["a1b2c3d4e5f6"]
			}
		]
	}`
	diffFile := filepath.Join(t.TempDir(), "diff_with_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := runSpexWithStderr(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error when diff contains errors")
	}
	if !strings.Contains(err.Error(), "diff contains 1 error(s)") {
		t.Fatalf("want error about diff errors, got: %v", err)
	}
	if !strings.Contains(stderr, "Requirement 'Match changed nodes to beads'") {
		t.Fatalf("want the error message on stderr, got: %s", stderr)
	}
	if out != "" {
		t.Fatalf("want empty stdout, got: %s", out)
	}
}

func TestE9_MultipleErrorsAllPrinted(t *testing.T) {
	specDir := setupTestSpec(t)

	diffJSON := `{
		"changes": [],
		"errors": [
			{"type": "incomplete_change", "message": "first error message", "path": "aaaaaaaaaaaa", "related": null},
			{"type": "incomplete_change", "message": "second error message", "path": "bbbbbbbbbbbb", "related": null}
		]
	}`
	diffFile := filepath.Join(t.TempDir(), "multi_errors.json")
	if err := os.WriteFile(diffFile, []byte(diffJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runSpexWithStderr(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error when diff contains errors")
	}
	if !strings.Contains(err.Error(), "diff contains 2 error(s)") {
		t.Fatalf("want error about 2 diff errors, got: %v", err)
	}
	if !strings.Contains(stderr, "first error message") {
		t.Fatalf("want first error in stderr, got: %s", stderr)
	}
	if !strings.Contains(stderr, "second error message") {
		t.Fatalf("want second error in stderr, got: %s", stderr)
	}
}

// E10: Diff input contains an empty errors array — impact proceeds normally.
func TestE10_EmptyErrorsArrayProceeds(t *testing.T) {
	specDir := setupTestSpec(t)
	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := merkle.Save(tree, filepath.Join(specDir, ".snapshot.json"), time.Now()); err != nil {
		t.Fatal(err)
	}

	diffFile := filepath.Join(t.TempDir(), "empty_errors.json")
	if err := os.WriteFile(diffFile, []byte(`{"changes": [], "errors": []}`), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error with an empty errors array, got: %v", err)
	}
	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON report: %v\noutput: %s", err, out)
	}
}

// TestFR7_ImpactCommand_PopulatesDepSpecNodeIDs verifies that the impact
// command populates DepSpecNodeIDs on create actions with identity hashes
// (not bead IDs — emit's Resolver does that).
// Setup: module "beta" requires module "alpha". When beta's component is
// modified, the create action should carry alpha component's identity hash
// in DepSpecNodeIDs (requires_module transitive resolution).
func TestFR7_ImpactCommand_PopulatesDepSpecNodeIDs(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	alphaID := schema.IdentityHash("module", "alpha")
	betaID := schema.IdentityHash("module", "beta")
	alphaCompID := schema.IdentityHash("alpha", "component", "AlphaComp")
	alphaTestID := schema.IdentityHash("alpha", "test_section", "AlphaTest")
	betaCompID := schema.IdentityHash("beta", "component", "BetaComp")
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
		"test_sections": [
			{"id": "`+alphaTestID+`", "name": "AlphaTest", "content": "test_alpha.md", "describes": ["`+alphaCompID+`"]}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_alpha.md", "# Alpha comp\n")
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
		"test_sections": [
			{"id": "`+betaTestID+`", "name": "BetaTest", "content": "test_beta.md", "describes": ["`+betaCompID+`"]}
		]
	}`)
	writeTestFile(t, betaDir, "arch_beta.md", "# Beta comp\n")
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

	writeTestJournal(t, specDir, []string{
		fmt.Sprintf(`{"event":"added","eid":"e1","node":"%s","name":"AlphaComp","node_type":"component","module":"alpha","before":null,"after":"aaa","git_head":"g1","proposal":"p1"}`, alphaCompID),
		`{"event":"task_created","for":"e1","task_id":"bead-alpha"}`,
		fmt.Sprintf(`{"event":"added","eid":"e2","node":"%s","name":"BetaComp","node_type":"component","module":"beta","before":null,"after":"bbb","git_head":"g1","proposal":"p1"}`, betaCompID),
		`{"event":"task_created","for":"e2","task_id":"bead-beta"}`,
	})

	out, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	// Find the create action for beta's component — it should have
	// alphaCompID in DepSpecNodeIDs (via requires_module resolution).
	for _, c := range report.Creates {
		if c.Module == "beta" && c.OldBeadID == "bead-beta" {
			if len(c.DepSpecNodeIDs) == 0 {
				t.Fatal("want DepSpecNodeIDs populated for beta create, got empty")
			}
			for _, dep := range c.DepSpecNodeIDs {
				if dep == alphaCompID {
					return // success
				}
			}
			t.Fatalf("want %s in DepSpecNodeIDs, got %v", alphaCompID, c.DepSpecNodeIDs)
		}
	}
	t.Fatal("create action for beta component with OldBeadID=bead-beta not found")
}

// TestFR7_ImpactCommand_UsesEdgePopulatesDepSpecNodeIDs verifies that
// intra-module component `uses` edges contribute identity hashes to
// DepSpecNodeIDs on the consumer's create action.
func TestFR7_ImpactCommand_UsesEdgePopulatesDepSpecNodeIDs(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "mod")
	baseID := schema.IdentityHash("mod", "component", "Base")
	userID := schema.IdentityHash("mod", "component", "User")
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
		"test_sections": [
			{"id": "`+baseTestID+`", "name": "BaseTest", "content": "test_base.md", "describes": ["`+baseID+`"]},
			{"id": "`+userTestID+`", "name": "UserTest", "content": "test_user.md", "describes": ["`+userID+`"]}
		]
	}`)
	writeTestFile(t, modDir, "arch_base.md", "# Base\n")
	writeTestFile(t, modDir, "arch_user.md", "# User\n")
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

	writeTestJournal(t, specDir, []string{
		fmt.Sprintf(`{"event":"added","eid":"e1","node":"%s","name":"Base","node_type":"component","module":"mod","before":null,"after":"aaa","git_head":"g1","proposal":"p1"}`, baseID),
		`{"event":"task_created","for":"e1","task_id":"bead-base"}`,
		fmt.Sprintf(`{"event":"added","eid":"e2","node":"%s","name":"User","node_type":"component","module":"mod","before":null,"after":"bbb","git_head":"g1","proposal":"p1"}`, userID),
		`{"event":"task_created","for":"e2","task_id":"bead-user"}`,
	})

	out, err := runSpex(t, "impact", "--diff", diffFile, "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("impact: %v", err)
	}

	var report impact.ImpactReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}

	for _, c := range report.Creates {
		if c.Module == "mod" && c.OldBeadID == "bead-user" {
			for _, dep := range c.DepSpecNodeIDs {
				if dep == baseID {
					return // success: uses edge contributed baseID
				}
			}
			t.Fatalf("want %s in DepSpecNodeIDs (uses edge), got %v", baseID, c.DepSpecNodeIDs)
		}
	}
	t.Fatal("create action for User component with OldBeadID=bead-user not found")
}

func TestFR4_ParseDiffJSON(t *testing.T) {
	input := `{
		"changes": [
			{"path": "module/1/component/1", "type": "modified", "impact": "arch_impl", "module": "alpha", "old_hash": "aaa", "new_hash": "bbb"},
			{"path": "module/1/test_section/1", "type": "added", "impact": "impl_only", "module": "alpha", "new_hash": "ccc"},
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

// TestPairingsFromFold verifies the journal-fold adapter: node entries copy
// straight across by identity hash, and proposal-epic entries (no Source.Node)
// are dropped since they never match a diff change.
func TestPairingsFromFold(t *testing.T) {
	fold := mapping.Fold{Entries: []mapping.FoldEntry{
		{
			Key:    "aaaaaaaaaaaa",
			TaskID: "task-1",
			Source: mapping.Event{Node: "aaaaaaaaaaaa", Name: "Comp1", NodeType: "component", Module: "m"},
		},
		{
			Key:    "2026-04-18-proposal",
			TaskID: "epic-1",
			Source: mapping.Event{Proposal: "2026-04-18-proposal"},
		},
	}}

	got := pairingsFromFold(fold)
	if len(got) != 1 {
		t.Fatalf("want 1 pairing (proposal entry dropped), got %d: %+v", len(got), got)
	}
	if got[0].SpecNodeID != "aaaaaaaaaaaa" || got[0].TaskID != "task-1" || got[0].NodeType != "component" || got[0].Module != "m" || got[0].Name != "Comp1" {
		t.Errorf("unexpected pairing: %+v", got[0])
	}
}

// TestEnrichPairingsWithBeadStatus verifies the helper copies live bead
// statuses onto journal pairings by joining on SpecNodeID. The third pairing
// has no matching bead and must keep its empty BeadStatus — the cleanup gate
// at action_classifier.go defaults closed for safety.
func TestEnrichPairingsWithBeadStatus(t *testing.T) {
	beads := []impact.BeadSpec{
		{ID: "bead-1", Status: "closed", SpecNodeID: "aaaaaaaaaaaa", Labels: []string{"spex:aaaaaaaaaaaa"}},
		{ID: "bead-2", Status: "open", SpecNodeID: "bbbbbbbbbbbb", Labels: []string{"spex:bbbbbbbbbbbb"}},
	}
	pairings := []impact.Pairing{
		{SpecNodeID: "aaaaaaaaaaaa", TaskID: "task-1"},
		{SpecNodeID: "bbbbbbbbbbbb", TaskID: "task-2"},
		{SpecNodeID: "cccccccccccc", TaskID: "task-3"},
	}

	out := enrichPairingsWithBeadStatus(beads, pairings)

	want := map[string]string{
		"aaaaaaaaaaaa": "closed",
		"bbbbbbbbbbbb": "open",
		"cccccccccccc": "",
	}
	for _, p := range out {
		if p.BeadStatus != want[p.SpecNodeID] {
			t.Errorf("pairing %s: want BeadStatus %q, got %q", p.SpecNodeID, want[p.SpecNodeID], p.BeadStatus)
		}
	}
}

// TestEnrichPairingsWithBeadStatus_EmptyInput verifies nil pairing input is
// returned untouched.
func TestEnrichPairingsWithBeadStatus_EmptyInput(t *testing.T) {
	out := enrichPairingsWithBeadStatus(nil, nil)
	if out != nil {
		t.Errorf("want nil slice, got %v", out)
	}
}
