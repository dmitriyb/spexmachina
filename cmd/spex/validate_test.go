package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/dmitriyb/spexmachina/validator"
)

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

func TestFR7_ValidateCommand_NonexistentDir(t *testing.T) {
	_, err := runSpex(t, "validate", "--spec-dir", "/nonexistent/spec/dir")
	if err == nil {
		t.Fatal("want error for nonexistent dir, got nil")
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

func TestFR7_ValidateCommand_WarningsDoNotFail(t *testing.T) {
	specDir := setupSpecWithOrphans(t)

	out, err := runSpex(t, "validate", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("want no error when only warnings, got %v", err)
	}

	var report validator.ValidationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output should be valid JSON: %v\noutput: %s", err, out)
	}
	if report.WarningCount == 0 {
		t.Fatal("want at least 1 warning for orphan spec")
	}
	if !report.Valid {
		t.Fatal("report should be valid when only warnings exist")
	}
}

// setupInvalidTestSpec creates a spec with a missing content file.
func setupInvalidTestSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := dir + "/alpha"
	if err := makeDir(alphaDir); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md"}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md", "describes": [1]}
		]
	}`)

	return dir
}

// setupSpecWithOrphans creates a valid spec with orphan requirements (warnings only).
func setupSpecWithOrphans(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeTestFile(t, dir, "project.json", `{
		"name": "test-project",
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha"}
		]
	}`)

	alphaDir := dir + "/alpha"
	if err := makeDir(alphaDir); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, alphaDir, "module.json", `{
		"name": "alpha",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Req1"},
			{"id": 2, "type": "functional", "title": "Req2"}
		],
		"components": [
			{"id": 1, "name": "Comp1", "content": "arch_comp1.md", "implements": [1]}
		],
		"impl_sections": [
			{"id": 1, "name": "Impl1", "content": "impl_comp1.md", "describes": [1]}
		]
	}`)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n")
	writeTestFile(t, alphaDir, "impl_comp1.md", "# Impl1\n")

	return dir
}

func makeDir(path string) error {
	return os.MkdirAll(path, 0755)
}
