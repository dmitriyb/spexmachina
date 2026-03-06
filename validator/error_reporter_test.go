package validator

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestREQ7_ReportSortsErrorsBeforeWarnings(t *testing.T) {
	errs := []ValidationError{
		{Check: "dag", Severity: "warning", Path: "a/module.json", Message: "unused dep"},
		{Check: "schema", Severity: "error", Path: "b/module.json", Message: "missing field"},
		{Check: "id", Severity: "error", Path: "a/module.json", Message: "duplicate id"},
	}

	var buf bytes.Buffer
	if err := Report(errs, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if report.Valid {
		t.Fatal("expected valid=false, got true")
	}
	if report.ErrorCount != 2 {
		t.Fatalf("want error_count=2, got %d", report.ErrorCount)
	}
	if report.WarningCount != 1 {
		t.Fatalf("want warning_count=1, got %d", report.WarningCount)
	}
	if len(report.Errors) != 3 {
		t.Fatalf("want 3 entries, got %d", len(report.Errors))
	}

	// First two should be errors (sorted by path), last should be warning
	if report.Errors[0].Severity != "error" || report.Errors[0].Path != "a/module.json" {
		t.Errorf("errors[0]: want error at a/module.json, got %s at %s", report.Errors[0].Severity, report.Errors[0].Path)
	}
	if report.Errors[1].Severity != "error" || report.Errors[1].Path != "b/module.json" {
		t.Errorf("errors[1]: want error at b/module.json, got %s at %s", report.Errors[1].Severity, report.Errors[1].Path)
	}
	if report.Errors[2].Severity != "warning" {
		t.Errorf("errors[2]: want warning, got %s", report.Errors[2].Severity)
	}
}

func TestREQ7_ReportValidWhenNoErrors(t *testing.T) {
	errs := []ValidationError{
		{Check: "orphan", Severity: "warning", Path: "x.md", Message: "unreferenced file"},
	}

	var buf bytes.Buffer
	if err := Report(errs, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if !report.Valid {
		t.Fatal("expected valid=true when only warnings present")
	}
	if report.ErrorCount != 0 {
		t.Fatalf("want error_count=0, got %d", report.ErrorCount)
	}
	if report.WarningCount != 1 {
		t.Fatalf("want warning_count=1, got %d", report.WarningCount)
	}
}

func TestREQ7_ReportEmptyErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := Report(nil, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if !report.Valid {
		t.Fatal("expected valid=true for empty errors")
	}
	if report.ErrorCount != 0 {
		t.Fatalf("want error_count=0, got %d", report.ErrorCount)
	}
	if report.WarningCount != 0 {
		t.Fatalf("want warning_count=0, got %d", report.WarningCount)
	}
}

func TestREQ7_ReportTTYIndentation(t *testing.T) {
	errs := []ValidationError{
		{Check: "schema", Severity: "error", Path: "project.json", Message: "bad"},
	}

	var compact bytes.Buffer
	if err := Report(errs, &compact, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pretty bytes.Buffer
	if err := Report(errs, &pretty, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(compact.String(), "\n  ") {
		t.Error("compact output should not contain indentation")
	}
	if !strings.Contains(pretty.String(), "\n  ") {
		t.Error("pretty output should contain indentation")
	}
}

func TestREQ7_ReportDoesNotMutateInput(t *testing.T) {
	errs := []ValidationError{
		{Check: "b", Severity: "warning", Path: "z", Message: "w"},
		{Check: "a", Severity: "error", Path: "a", Message: "e"},
	}

	origFirst := errs[0]
	var buf bytes.Buffer
	if err := Report(errs, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if errs[0] != origFirst {
		t.Error("Report mutated the input slice")
	}
}
