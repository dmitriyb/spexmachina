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
	"github.com/dmitriyb/spexmachina/internal/perf"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// runValidate executes `spex validate` with the given args and returns
// stdout, stderr, and the process exit code, mirroring main.go's exit-code
// and stderr handling — see diff_test.go's runDiff, which this follows.
func runValidate(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newValidateCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"validate"}, args...))

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

// parseReport unmarshals a validate command's stdout into a ValidationReport,
// failing the test if the output is not valid JSON.
func parseReport(t *testing.T, out string) validator.ValidationReport {
	t.Helper()
	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	return report
}

// hasCheck reports whether the report carries at least one error from the
// named checker.
func hasCheck(report validator.ValidationReport, check string) bool {
	for _, e := range report.Errors {
		if e.Check == check {
			return true
		}
	}
	return false
}

func TestFR7_ValidateCommand_ValidSpec(t *testing.T) {
	specDir := setupTestSpec(t)

	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error for valid spec, got %v", err)
	}

	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if !report.Valid {
		t.Fatalf("report should be valid, got errors: %v", report.Errors)
	}
	if report.ErrorCount != 0 {
		t.Fatalf("want 0 errors, got %d", report.ErrorCount)
	}
}

func TestFR7_ValidateCommand_InvalidSpec_Exit1(t *testing.T) {
	specDir := setupInvalidTestSpec(t)

	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for invalid spec, got nil")
	}

	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if report.Valid {
		t.Fatal("report should not be valid for invalid spec")
	}
	if report.ErrorCount == 0 {
		t.Fatal("want at least 1 error for invalid spec")
	}
}

// TestFR7_ValidateCommand_ExitStatusMatchesReportValid pins that the exit
// status and the `valid` field of the JSON report are decided by one rule.
// Both derive from the ValidationReport that Report serializes, so a report
// with entries can never be printed alongside a zero exit status, nor an
// empty report alongside a non-zero one.
func TestFR7_ValidateCommand_ExitStatusMatchesReportValid(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T) string
	}{
		{"valid spec", setupTestSpec},
		{"invalid spec", setupInvalidTestSpec},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runSpex(t, "validate", "--spec-dir", tt.setup(t))

			var report validator.ValidationReport
			if jsonErr := json.Unmarshal([]byte(out), &report); jsonErr != nil {
				t.Fatalf("output should be valid JSON: %v\noutput: %s", jsonErr, out)
			}

			if report.Valid != (err == nil) {
				t.Fatalf("report.valid=%v but exit error=%v; the two must agree", report.Valid, err)
			}
			if report.Valid != (report.ErrorCount == 0) {
				t.Fatalf("report.valid=%v but error_count=%d", report.Valid, report.ErrorCount)
			}
			if report.WarningCount != 0 {
				t.Fatalf("want warning_count=0, got %d", report.WarningCount)
			}
		})
	}
}

// V12: a non-existent --spec-dir exits 1 with an error indicating
// project.json was not found, and the output is still valid JSON.
func TestFR7_ValidateCommand_NonexistentDir(t *testing.T) {
	out, err := runSpex(t, "validate", "--spec-dir", "/nonexistent/spec/dir")
	if err == nil {
		t.Fatal("want error for nonexistent dir, got nil")
	}

	report := parseReport(t, out) // must still be valid JSON, not a panic trace
	if report.Valid {
		t.Fatal("want valid=false")
	}
	if !hasCheck(report, "schema") {
		t.Fatalf("want a schema check error, got: %v", report.Errors)
	}

	var found bool
	for _, e := range report.Errors {
		if strings.Contains(e.Message, "project.json") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want an error message indicating project.json was not found, got: %v", report.Errors)
	}
}

func TestFR7_ValidateCommand_AggregatesAllCheckers(t *testing.T) {
	specDir := setupInvalidTestSpec(t)

	out, _ := runSpex(t, "validate", "--spec-dir", specDir)

	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}

	checks := map[string]bool{}
	for _, e := range report.Errors {
		checks[e.Check] = true
	}
	if len(checks) == 0 {
		t.Fatal("want errors from at least one checker")
	}
}

func TestFR7_ValidateCommand_StructuredJSON(t *testing.T) {
	specDir := setupTestSpec(t)

	out, _ := runSpex(t, "validate", "--spec-dir", specDir)

	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON report: %v\noutput: %s", err, out)
	}

	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
	for _, field := range []string{"valid", "error_count", "warning_count", "errors"} {
		if _, ok := raw[field]; !ok {
			t.Fatalf("report missing field %q", field)
		}
	}
}

func TestFR7_ValidateCommand_DefaultDir(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := runSpex(t, "validate")
	if err == nil {
		t.Fatal("want error when default spec/ missing, got nil")
	}
}

// V10: a valid spec at the default spec/ directory validates and exits 0
// when no --spec-dir is given.
func TestFR7_ValidateCommand_DefaultDirValidSpec(t *testing.T) {
	tmp := t.TempDir()
	specDir := filepath.Join(tmp, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeValidSpecFiles(t, specDir)

	t.Chdir(tmp)
	out, err := runSpex(t, "validate")
	if err != nil {
		t.Fatalf("want no error for valid default spec/, got %v\noutput: %s", err, out)
	}
	report := parseReport(t, out)
	if !report.Valid {
		t.Fatalf("want valid=true, got errors: %v", report.Errors)
	}
}

// V13: the validator can validate spex-machina's own spec directory.
func TestFR7_ValidateCommand_SelfValidate(t *testing.T) {
	specDir := filepath.Join("..", "..", "spec")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error self-validating spec/, got %v\noutput: %s", err, out)
	}
	report := parseReport(t, out)
	if !report.Valid {
		t.Fatalf("want valid=true self-validating own spec, got errors: %v", report.Errors)
	}
}

// V14: stdout is piped, non-TTY output, so JSON is compact — single line,
// no indentation.
func TestFR7_ValidateCommand_PipedOutputIsCompact(t *testing.T) {
	specDir := setupTestSpec(t)

	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error for valid spec, got %v", err)
	}

	if strings.Contains(out, "\n  ") {
		t.Errorf("want compact JSON with no indentation, got: %s", out)
	}
	if n := strings.Count(strings.TrimRight(out, "\n"), "\n"); n != 0 {
		t.Errorf("want a single line of JSON, got %d embedded newlines: %s", n, out)
	}
}

// V2: a schema violation in project.json exits 1 with a "schema" check error.
func TestFR7_ValidateCommand_SchemaErrorExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "missing_name")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for schema violation, got nil")
	}
	report := parseReport(t, out)
	if report.Valid {
		t.Fatal("want valid=false")
	}
	if !hasCheck(report, "schema") {
		t.Fatalf("want a schema check error, got: %v", report.Errors)
	}
}

// V3: a component referencing a content file that does not exist exits 1
// with a "content" check error.
func TestFR7_ValidateCommand_ContentErrorExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "content_missing")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for missing content file, got nil")
	}
	report := parseReport(t, out)
	if report.Valid {
		t.Fatal("want valid=false")
	}
	if !hasCheck(report, "content") {
		t.Fatalf("want a content check error, got: %v", report.Errors)
	}
}

// V4: a module dependency cycle exits 1 with a "dag" check error.
func TestFR7_ValidateCommand_DAGCycleExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "dag_module_cycle")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for DAG cycle, got nil")
	}
	report := parseReport(t, out)
	if !hasCheck(report, "dag") {
		t.Fatalf("want a dag check error, got: %v", report.Errors)
	}
}

// V6: a duplicate identity hash exits 1 with an "id" check error.
func TestFR7_ValidateCommand_IDDuplicationExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "id_dup")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for duplicate ID, got nil")
	}
	report := parseReport(t, out)
	if !hasCheck(report, "id") {
		t.Fatalf("want an id check error, got: %v", report.Errors)
	}
}

// V7: a module name mismatch exits 1 with a "name_consistency" check error.
func TestFR7_ValidateCommand_NameMismatchExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "name_case_mismatch")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for name mismatch, got nil")
	}
	report := parseReport(t, out)
	if !hasCheck(report, "name_consistency") {
		t.Fatalf("want a name_consistency check error, got: %v", report.Errors)
	}
}

// V15: an uncovered component exits 1 with a "test_coverage" check error.
func TestFR7_ValidateCommand_TestCoverageErrorExit1(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "coverage_one_uncovered")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for uncovered component, got nil")
	}
	report := parseReport(t, out)
	if !hasCheck(report, "test_coverage") {
		t.Fatalf("want a test_coverage check error, got: %v", report.Errors)
	}
}

// V8: a spec with a schema error, a content error and a DAG cycle at once
// reports errors from all three checkers in a single run — no checker
// short-circuits the sequence.
func TestFR7_ValidateCommand_MultipleCheckerErrorsAllReported(t *testing.T) {
	dir := t.TempDir()
	comp1Hash := schema.IdentityHash("alpha", "component", "Comp1")

	// project.json omits the required "name" field (schema error) and
	// declares two modules with a circular requires_module (dag error).
	writeTestFile(t, dir, "project.json", `{
		"modules": [
			{"id": "aaaaaaaaaaaa", "name": "alpha", "path": "alpha", "requires_module": ["bbbbbbbbbbbb"]},
			{"id": "bbbbbbbbbbbb", "name": "beta", "path": "beta", "requires_module": ["aaaaaaaaaaaa"]}
		]
	}`)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Comp1's content file does not exist (content error).
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+comp1Hash+`", "name": "Comp1", "content": "missing.md"}
		]
	}`)

	betaDir := filepath.Join(dir, "beta")
	if err := os.MkdirAll(betaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, betaDir, "module.json", `{"name": "beta"}`)

	out, err := runSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	report := parseReport(t, out)

	checks := map[string]bool{}
	for _, e := range report.Errors {
		checks[e.Check] = true
	}
	for _, want := range []string{"schema", "content", "dag"} {
		if !checks[want] {
			t.Errorf("want an error from checker %q, got checks: %v", want, checks)
		}
	}
}

// V9: checkers run in the fixed order and none short-circuits — a module.json
// that fails to parse produces one error per checker, all ten at that path.
// The final report is sorted by path only, so ties come back in whatever
// order the sort produced; assert the set of checks at that path, not their
// sequence.
func TestFR7_ValidateCommand_ChecksRunInFixedOrder(t *testing.T) {
	specDir := filepath.Join("..", "..", "validator", "testdata", "name_invalid_json")
	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err == nil {
		t.Fatal("want error for unparseable module.json, got nil")
	}
	report := parseReport(t, out)

	wantChecks := []string{
		"schema", "content", "link", "id", "id_derivation",
		"dag", "name_consistency", "test_coverage", "requirement_coverage", "coupled_section",
	}
	if len(report.Errors) != len(wantChecks) {
		t.Fatalf("want %d entries (one per checker), got %d: %v", len(wantChecks), len(report.Errors), report.Errors)
	}
	gotChecks := map[string]bool{}
	for _, e := range report.Errors {
		if e.Path != "alpha/module.json" {
			t.Errorf("want path=alpha/module.json, got %q", e.Path)
		}
		gotChecks[e.Check] = true
	}
	for _, c := range wantChecks {
		if !gotChecks[c] {
			t.Errorf("missing entry from checker %q", c)
		}
	}
}

// S17: a malformed spec/profile.json is a single early refusal, resolved
// once ahead of any check — never a per-check cascade of schema-conformance
// errors once each checker resolves the profile for itself. Mirrors
// diff_test.go's TestFR4_E2b_DiffCommand_MalformedProfile.
func TestFR7_S17_ValidateCommand_MalformedProfile(t *testing.T) {
	specDir := setupTestSpec(t)

	profilePath := filepath.Join(specDir, "profile.json")
	writeTestFile(t, specDir, "profile.json", "{not valid json")

	out, stderr, exitCode := runValidate(t, "--spec-dir", specDir)
	if exitCode != 1 {
		t.Fatalf("want exit 1 for a malformed profile, got %d\nstderr: %s", exitCode, stderr)
	}
	if out != "" {
		t.Fatalf("no validation report should print once the profile fails to resolve, got stdout: %s", out)
	}
	if !strings.Contains(stderr, profilePath) {
		t.Fatalf("stderr should name the profile file %q, got: %s", profilePath, stderr)
	}
	if !strings.Contains(stderr, "validate: schema: resolve profile") {
		t.Fatalf("stderr should be the profile resolution's own single early error, got: %s", stderr)
	}
}

// V16: a project requirement declaring derivation "pending" that nothing
// derives is reported as a disclosure note, not an error — the run still
// exits 0. Removing the derivation field from the same fixture flips the run
// to exit 1 with a requirement_coverage error, proving the field (and only
// the field) downgrades the finding.
func TestFR7_ValidateCommand_PendingDerivationIsNoteNotError(t *testing.T) {
	buildSpec := func(t *testing.T, pending bool) string {
		t.Helper()
		dir := t.TempDir()

		derivation := ""
		if pending {
			derivation = `, "derivation": "pending"`
		}
		writeTestFile(t, dir, "project.json", `{
			"name": "test-project",
			"modules": [
				{"id": "000000000001", "name": "alpha", "path": "alpha"}
			],
			"requirements": [
				{"id": "aaaaaaaaaaaa", "title": "Feature A", "type": "functional", "priority": 1},
				{"id": "bbbbbbbbbbbb", "title": "Feature B", "type": "functional", "priority": 2`+derivation+`}
			]
		}`)

		alphaDir := filepath.Join(dir, "alpha")
		if err := os.MkdirAll(alphaDir, 0755); err != nil {
			t.Fatal(err)
		}
		modReqHash := schema.IdentityHash("alpha", "requirement", "Mod Feat A")
		compHash := schema.IdentityHash("alpha", "component", "Comp1")
		testHash := schema.IdentityHash("alpha", "test_section", "Comp1 tests")
		writeTestFile(t, alphaDir, "module.json", `{
			"name": "alpha",
			"requirements": [
				{"id": "`+modReqHash+`", "preq_id": "aaaaaaaaaaaa", "name": "Mod Feat A", "type": "functional"}
			],
			"components": [
				{"id": "`+compHash+`", "name": "Comp1", "content": "arch_comp1.md", "implements": ["`+modReqHash+`"]}
			],
			"test_sections": [
				{"id": "`+testHash+`", "name": "Comp1 tests", "content": "test_comp1.md", "describes": ["`+compHash+`"]}
			]
		}`)
		writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
		writeTestFile(t, alphaDir, "test_comp1.md", "# Comp1 tests\n")

		return dir
	}

	t.Run("pending derivation is a note, exit 0", func(t *testing.T) {
		out, err := runSpex(t, "validate", "--spec-dir", buildSpec(t, true))
		if err != nil {
			t.Fatalf("want no error, got %v\noutput: %s", err, out)
		}
		report := parseReport(t, out)
		if !report.Valid {
			t.Fatalf("want valid=true, got errors: %v", report.Errors)
		}
		if report.ErrorCount != 0 {
			t.Fatalf("want error_count=0, got %d", report.ErrorCount)
		}
		if report.WarningCount != 0 {
			t.Fatalf("want warning_count=0, got %d", report.WarningCount)
		}
		if len(report.Notes) != 1 {
			t.Fatalf("want exactly 1 note, got %d: %v", len(report.Notes), report.Notes)
		}
		if report.Notes[0].Type != "pending_derivation" {
			t.Fatalf("want note type=pending_derivation, got %q", report.Notes[0].Type)
		}
		if len(report.Notes[0].Related) != 1 || report.Notes[0].Related[0] != "bbbbbbbbbbbb" {
			t.Fatalf("want note naming requirement bbbbbbbbbbbb, got %v", report.Notes[0].Related)
		}
	})

	t.Run("without derivation field, same gap is an error, exit 1", func(t *testing.T) {
		out, err := runSpex(t, "validate", "--spec-dir", buildSpec(t, false))
		if err == nil {
			t.Fatal("want error, got nil")
		}
		report := parseReport(t, out)
		if report.Valid {
			t.Fatal("want valid=false")
		}
		if !hasCheck(report, "requirement_coverage") {
			t.Fatalf("want a requirement_coverage check error, got: %v", report.Errors)
		}
		if len(report.Notes) != 0 {
			t.Fatalf("want no notes once the gap is an error, got: %v", report.Notes)
		}
	})
}

// E1: an empty directory with no project.json exits 1 with a structured
// error rather than a panic.
func TestFR7_ValidateCommand_EmptyDirNoProjectJSON(t *testing.T) {
	dir := t.TempDir()

	out, err := runSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for missing project.json, got nil")
	}
	_ = parseReport(t, out) // must still be valid JSON, not a panic trace
}

// E2: project.json with zero modules. test_validation_pipeline.md's E2 says
// this exits 0, but schema/project.schema.json puts minItems:1 on modules,
// so today's shipped schema rejects the array before any other check runs.
// That contradiction is filed as drifts/drift-spexmachina-okib.7.json rather
// than resolved here; this test pins the behavior the current schema
// actually produces — exit 1, rejected by the schema check alone — so it
// fails if either the schema or the checker pipeline changes that outcome
// without the drift being triaged first.
func TestFR7_ValidateCommand_ZeroModules(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "project.json", `{"name": "empty-project", "modules": []}`)

	out, err := runSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for zero modules under the current schema's minItems:1, got nil")
	}
	report := parseReport(t, out)
	if report.Valid {
		t.Fatal("want valid=false")
	}
	if !hasCheck(report, "schema") {
		t.Fatalf("zero modules should be rejected by the schema check, got: %v", report.Errors)
	}
}

// E4: a spec producing 500 validation errors reports all 500, with no
// truncation.
func TestFR7_ValidateCommand_LargeErrorCount(t *testing.T) {
	const n = 500
	dir := t.TempDir()

	// A filler module that derives none of the requirements below, so the
	// schema's modules minItems:1 is satisfied without covering anything.
	var sb strings.Builder
	sb.WriteString(`{"name": "large-error-count", "modules": [{"id": "aaaaaaaaaaaa", "name": "filler", "path": "filler"}], "requirements": [`)
	for i := 1; i <= n; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"id": "%012x", "title": "Req %d", "type": "functional", "priority": 1}`, i, i)
	}
	sb.WriteString(`]}`)
	writeTestFile(t, dir, "project.json", sb.String())

	fillerDir := filepath.Join(dir, "filler")
	if err := os.MkdirAll(fillerDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, fillerDir, "module.json", `{"name": "filler"}`)

	out, err := runSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for uncovered requirements, got nil")
	}
	report := parseReport(t, out)
	if report.ErrorCount != n {
		t.Fatalf("want error_count=%d, got %d", n, report.ErrorCount)
	}
	if len(report.Errors) != n {
		t.Fatalf("want %d entries in errors array, got %d", n, len(report.Errors))
	}
}

// E5: a relative --spec-dir is resolved to an absolute path before checkers
// run, but the paths in the report stay relative to the spec directory, not
// absolute filesystem paths.
func TestFR7_ValidateCommand_RelativePathResolution(t *testing.T) {
	base := t.TempDir()
	specDir := filepath.Join(base, "myspec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
		]
	}`)
	alphaDir := filepath.Join(specDir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "aabbccddeeff", "name": "Comp1", "content": "arch_comp1.md"}
		]
	}`)

	t.Chdir(base)
	out, err := runSpex(t, "validate", "--spec-dir", "./myspec")
	if err == nil {
		t.Fatal("want error for missing content file, got nil")
	}
	report := parseReport(t, out)
	if len(report.Errors) == 0 {
		t.Fatal("want at least one error")
	}
	for _, e := range report.Errors {
		if strings.HasPrefix(e.Path, "/") || strings.HasPrefix(e.Path, base) {
			t.Errorf("want a path relative to the spec dir, got absolute-looking path %q", e.Path)
		}
	}
}

// E6: a checker's error message containing a quote character still
// serializes as valid, correctly-escaped JSON.
func TestFR7_ValidateCommand_SpecialCharactersInMessage(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "project.json", `{
		"name": "quote-test",
		"modules": [
			{"id": "000000000001", "name": "it's", "path": "alpha"}
		]
	}`)
	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{"name": "IT'S"}`)

	out, err := runSpex(t, "validate", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for name mismatch, got nil")
	}
	report := parseReport(t, out)

	found := false
	for _, e := range report.Errors {
		if e.Check == "name_consistency" && strings.Contains(e.Message, "'") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want a name_consistency error mentioning the quoted name, got: %v", report.Errors)
	}
}

// E7: a spec with 100 modules, 10 requirements, 5 components and 5
// test_sections per module (2000 module-scoped nodes plus the 100 module
// declarations) validates in under a second, per requirement b42c5cdf874b
// (fast validation).
func TestFR7_ValidateCommand_PerformanceBudget(t *testing.T) {
	dir := buildLargeValidSpec(t, 100, 10, 5, 5)

	var (
		out string
		err error
	)
	perf.Within(t, time.Second, func() {
		out, err = runSpex(t, "validate", "--spec-dir", dir)
	})

	if err != nil {
		t.Fatalf("want no error for a valid large spec, got %v\noutput: %s", err, out)
	}
	report := parseReport(t, out)
	if !report.Valid {
		t.Fatalf("want valid=true, got errors: %v", report.Errors)
	}
}

// buildLargeValidSpec writes a fully valid spec with the given number of
// modules, each carrying reqsPerModule requirements, compsPerModule
// components (each implementing two requirements) and testsPerModule test
// sections (each describing one component). compsPerModule*2 must equal
// reqsPerModule, and testsPerModule must equal compsPerModule, so every
// requirement is implemented and every component is covered.
func buildLargeValidSpec(t *testing.T, modules, reqsPerModule, compsPerModule, testsPerModule int) string {
	t.Helper()
	dir := t.TempDir()

	proj := schema.Project{Name: "large-project"}
	for m := 0; m < modules; m++ {
		modName := fmt.Sprintf("module%d", m)
		modID := fmt.Sprintf("%012x", m+1)
		preqID := fmt.Sprintf("%012x", 0x100000+m)
		priority := 1

		proj.Modules = append(proj.Modules, schema.Module{ID: modID, Name: modName, Path: modName})
		proj.Requirements = append(proj.Requirements, schema.Requirement{
			ID:       preqID,
			Type:     "functional",
			Title:    fmt.Sprintf("Project requirement %d", m),
			Priority: &priority,
		})

		modDir := filepath.Join(dir, modName)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatal(err)
		}

		modSpec := schema.ModuleSpec{Name: modName}
		reqIDs := make([]string, reqsPerModule)
		for r := 0; r < reqsPerModule; r++ {
			title := fmt.Sprintf("Req%d", r)
			id := schema.IdentityHash(modName, "requirement", title)
			reqIDs[r] = id
			modSpec.Requirements = append(modSpec.Requirements, schema.ModuleRequirement{
				ID:     id,
				PreqID: preqID,
				Type:   "functional",
				Title:  title,
			})
		}

		reqsPerComp := reqsPerModule / compsPerModule
		compIDs := make([]string, compsPerModule)
		for c := 0; c < compsPerModule; c++ {
			name := fmt.Sprintf("Comp%d", c)
			id := schema.IdentityHash(modName, "component", name)
			compIDs[c] = id
			content := fmt.Sprintf("arch_comp%d.md", c)
			modSpec.Components = append(modSpec.Components, schema.Component{
				ID:         id,
				Name:       name,
				Content:    content,
				Implements: reqIDs[c*reqsPerComp : (c+1)*reqsPerComp],
			})
			writeTestFile(t, modDir, content, fmt.Sprintf("# %s architecture\n", name))
		}

		compsPerTest := compsPerModule / testsPerModule
		for ts := 0; ts < testsPerModule; ts++ {
			name := fmt.Sprintf("Test%d", ts)
			id := schema.IdentityHash(modName, "test_section", name)
			content := fmt.Sprintf("test_comp%d.md", ts)
			modSpec.TestSections = append(modSpec.TestSections, schema.TestSection{
				ID:        id,
				Name:      name,
				Content:   content,
				Describes: compIDs[ts*compsPerTest : (ts+1)*compsPerTest],
			})
			writeTestFile(t, modDir, content, fmt.Sprintf("# %s tests\n", name))
		}

		modBytes, err := json.Marshal(modSpec)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, modDir, "module.json", string(modBytes))
	}

	projBytes, err := json.Marshal(proj)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "project.json", string(projBytes))

	return dir
}

// setupInvalidTestSpec creates a spec with a missing content file.
func setupInvalidTestSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := dir + "/alpha"
	if err := makeDir(alphaDir); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "aabbccddeeff", "name": "Comp1", "content": "arch_comp1.md"}
		]
	}`)

	return dir
}

func makeDir(path string) error {
	return os.MkdirAll(path, 0755)
}
