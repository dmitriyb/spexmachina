package validator

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestREQ7_ReportSortsErrorsByPath(t *testing.T) {
	errs := []ValidationError{
		{Check: "dag", Severity: "error", Path: "c/module.json", Message: "unused dep"},
		{Check: "schema", Severity: "error", Path: "b/module.json", Message: "missing field"},
		{Check: "id", Severity: "error", Path: "a/module.json", Message: "duplicate id"},
	}

	var buf bytes.Buffer
	if _, err := Report(errs, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if report.Valid {
		t.Fatal("expected valid=false, got true")
	}
	if report.ErrorCount != 3 {
		t.Fatalf("want error_count=3, got %d", report.ErrorCount)
	}
	if report.WarningCount != 0 {
		t.Fatalf("want warning_count=0, got %d", report.WarningCount)
	}
	if len(report.Errors) != 3 {
		t.Fatalf("want 3 entries, got %d", len(report.Errors))
	}

	wantPaths := []string{"a/module.json", "b/module.json", "c/module.json"}
	for i, want := range wantPaths {
		if report.Errors[i].Path != want {
			t.Errorf("errors[%d]: want path %s, got %s", i, want, report.Errors[i].Path)
		}
	}
}

func TestREQ7_ReportEmptyErrors(t *testing.T) {
	var buf bytes.Buffer
	if _, err := Report(nil, &buf, false); err != nil {
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

// TestREQ7_ReportReturnsTheReportItWrote pins that the returned report is the
// one serialized to w. Callers derive their exit status from it, so the two
// must never differ — including for an entry whose Severity is unset, which no
// checker produces but which the exported field still permits, and which under
// the old severity-scanning exit rule would have printed valid=false alongside
// a zero exit status.
func TestREQ7_ReportReturnsTheReportItWrote(t *testing.T) {
	tests := []struct {
		name string
		errs []ValidationError
	}{
		{"no errors", nil},
		{"severity error", []ValidationError{
			{Check: "schema", Severity: "error", Path: "project.json", Message: "bad"},
		}},
		{"severity unset", []ValidationError{
			{Check: "schema", Path: "project.json", Message: "bad"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			got, err := Report(tt.errs, &buf, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var written ValidationReport
			if err := json.Unmarshal(buf.Bytes(), &written); err != nil {
				t.Fatalf("unmarshal report: %v", err)
			}

			if got.Valid != written.Valid {
				t.Errorf("returned valid=%v, wrote valid=%v", got.Valid, written.Valid)
			}
			if got.ErrorCount != written.ErrorCount {
				t.Errorf("returned error_count=%d, wrote error_count=%d", got.ErrorCount, written.ErrorCount)
			}
			if got.WarningCount != 0 || written.WarningCount != 0 {
				t.Errorf("want warning_count=0, returned %d and wrote %d", got.WarningCount, written.WarningCount)
			}
			if want := len(tt.errs) == 0; got.Valid != want {
				t.Errorf("want valid=%v for %d entries, got %v", want, len(tt.errs), got.Valid)
			}
		})
	}
}

// TestREQ7_ReportAlwaysEmitsWarningCountKey pins the JSON contract: no checker
// emits warnings any more, but `warning_count` must still appear in the output
// (as 0) because gates and CI assert on `.warning_count == 0`.
func TestREQ7_ReportAlwaysEmitsWarningCountKey(t *testing.T) {
	tests := []struct {
		name string
		errs []ValidationError
	}{
		{"no errors", nil},
		{"with errors", []ValidationError{
			{Check: "schema", Severity: "error", Path: "project.json", Message: "bad"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := Report(tt.errs, &buf, false); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal report: %v", err)
			}
			v, ok := raw["warning_count"]
			if !ok {
				t.Fatal("report is missing the warning_count key")
			}
			if string(v) != "0" {
				t.Fatalf("want warning_count=0, got %s", v)
			}
		})
	}
}

func TestREQ7_ReportTTYIndentation(t *testing.T) {
	errs := []ValidationError{
		{Check: "schema", Severity: "error", Path: "project.json", Message: "bad"},
	}

	var compact bytes.Buffer
	if _, err := Report(errs, &compact, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pretty bytes.Buffer
	if _, err := Report(errs, &pretty, true); err != nil {
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
		{Check: "b", Severity: "error", Path: "z", Message: "w"},
		{Check: "a", Severity: "error", Path: "a", Message: "e"},
	}

	origFirst := errs[0]
	var buf bytes.Buffer
	if _, err := Report(errs, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if errs[0] != origFirst {
		t.Error("Report mutated the input slice")
	}
}
