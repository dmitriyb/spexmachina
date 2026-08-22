package validator

import (
	"bytes"
	"encoding/json"
	"reflect"
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
	if _, err := Report(errs, nil, &buf, false); err != nil {
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
	if _, err := Report(nil, nil, &buf, false); err != nil {
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
			got, err := Report(tt.errs, nil, &buf, false)
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
			if _, err := Report(tt.errs, nil, &buf, false); err != nil {
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
	if _, err := Report(errs, nil, &compact, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pretty bytes.Buffer
	if _, err := Report(errs, nil, &pretty, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(compact.String(), "\n  ") {
		t.Error("compact output should not contain indentation")
	}
	if !strings.Contains(pretty.String(), "\n  ") {
		t.Error("pretty output should contain indentation")
	}
}

// TestREQ7_ReportSingleErrorStructure pins R2: a single ValidationError
// round-trips through Report with every field intact.
func TestREQ7_ReportSingleErrorStructure(t *testing.T) {
	errs := []ValidationError{
		{Check: "schema", Severity: "error", Path: "project.json:name", Message: "required field missing"},
	}

	var buf bytes.Buffer
	if _, err := Report(errs, nil, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if report.Valid {
		t.Error("want valid=false")
	}
	if report.ErrorCount != 1 {
		t.Errorf("want error_count=1, got %d", report.ErrorCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("want warning_count=0, got %d", report.WarningCount)
	}
	if len(report.Errors) != 1 || report.Errors[0] != errs[0] {
		t.Errorf("want errors=%v, got %v", errs, report.Errors)
	}
}

// TestREQ7_ReportAggregatesMultipleCheckers pins R6: errors from every
// checker survive aggregation, none dropped.
func TestREQ7_ReportAggregatesMultipleCheckers(t *testing.T) {
	checks := []string{"schema", "content", "dag", "id", "name_consistency", "test_coverage"}
	errs := make([]ValidationError, len(checks))
	for i, c := range checks {
		errs[i] = ValidationError{Check: c, Severity: "error", Path: "x", Message: "bad"}
	}

	var buf bytes.Buffer
	if _, err := Report(errs, nil, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if len(report.Errors) != len(checks) {
		t.Fatalf("want %d entries, got %d", len(checks), len(report.Errors))
	}
	got := make(map[string]bool)
	for _, e := range report.Errors {
		got[e.Check] = true
	}
	for _, c := range checks {
		if !got[c] {
			t.Errorf("missing entry from checker %q", c)
		}
	}
}

// TestREQ7_ReportOutputValidJSONWithSpecialCharacters pins R7: messages
// containing quotes, newlines and unicode still serialize as valid JSON.
func TestREQ7_ReportOutputValidJSONWithSpecialCharacters(t *testing.T) {
	errs := []ValidationError{
		{Check: "schema", Severity: "error", Path: "x", Message: `has "quotes", a` + "\n" + `newline, and unicode: café 日本語`},
	}

	var buf bytes.Buffer
	if _, err := Report(errs, nil, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if report.Errors[0].Message != errs[0].Message {
		t.Errorf("want message %q, got %q", errs[0].Message, report.Errors[0].Message)
	}
}

// TestREQ7_ReportNotesAlongsideEmptyErrors pins R10: a note with an empty
// error list still yields valid=true and carries the note without touching
// the verdict or counts.
func TestREQ7_ReportNotesAlongsideEmptyErrors(t *testing.T) {
	notes := []ValidationNote{
		{Type: "pending_derivation", Message: "requirement pending", Related: []string{"a65bbd37c7ec"}},
	}

	var buf bytes.Buffer
	if _, err := Report(nil, notes, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if !report.Valid {
		t.Error("want valid=true")
	}
	if report.ErrorCount != 0 {
		t.Errorf("want error_count=0, got %d", report.ErrorCount)
	}
	if report.WarningCount != 0 {
		t.Errorf("want warning_count=0, got %d", report.WarningCount)
	}
	if len(report.Errors) != 0 {
		t.Errorf("want empty errors, got %v", report.Errors)
	}
	if len(report.Notes) != 1 || !reflect.DeepEqual(report.Notes[0], notes[0]) {
		t.Errorf("want notes=%v, got %v", notes, report.Notes)
	}
}

// TestREQ7_ReportOmitsNotesKeyWhenEmpty pins R11: with no disclosure notes,
// the serialized document has no "notes" key at all — a clean run emits
// exactly the four-key document, so no existing consumer observes a change.
func TestREQ7_ReportOmitsNotesKeyWhenEmpty(t *testing.T) {
	tests := []struct {
		name  string
		errs  []ValidationError
		notes []ValidationNote
	}{
		{"no errors, nil notes", nil, nil},
		{"errors, nil notes", []ValidationError{{Check: "schema", Severity: "error", Path: "x", Message: "bad"}}, nil},
		{"errors, empty notes slice", []ValidationError{{Check: "schema", Severity: "error", Path: "x", Message: "bad"}}, []ValidationNote{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if _, err := Report(tt.errs, tt.notes, &buf, false); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
				t.Fatalf("unmarshal report: %v", err)
			}
			if _, ok := raw["notes"]; ok {
				t.Error("want no notes key in output, but it is present")
			}
			if len(raw) != 4 {
				t.Errorf("want exactly 4 keys, got %d: %v", len(raw), raw)
			}
		})
	}
}

// TestREQ7_ReportErrorsAndNotesCoexist pins R12: an error and a note in the
// same run land in their own arrays without leaking into each other.
func TestREQ7_ReportErrorsAndNotesCoexist(t *testing.T) {
	errs := []ValidationError{
		{Check: "requirement_coverage", Severity: "error", Path: "x", Message: "uncovered"},
	}
	notes := []ValidationNote{
		{Type: "pending_derivation", Message: "requirement pending", Related: []string{"a65bbd37c7ec"}},
	}

	var buf bytes.Buffer
	if _, err := Report(errs, notes, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ValidationReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}

	if report.Valid {
		t.Error("want valid=false")
	}
	if report.ErrorCount != 1 {
		t.Errorf("want error_count=1, got %d", report.ErrorCount)
	}
	if len(report.Errors) != 1 || report.Errors[0] != errs[0] {
		t.Errorf("want errors=%v, got %v", errs, report.Errors)
	}
	if len(report.Notes) != 1 || !reflect.DeepEqual(report.Notes[0], notes[0]) {
		t.Errorf("want notes=%v, got %v", notes, report.Notes)
	}
}

func TestREQ7_ReportDoesNotMutateInput(t *testing.T) {
	errs := []ValidationError{
		{Check: "b", Severity: "error", Path: "z", Message: "w"},
		{Check: "a", Severity: "error", Path: "a", Message: "e"},
	}

	origFirst := errs[0]
	var buf bytes.Buffer
	if _, err := Report(errs, nil, &buf, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if errs[0] != origFirst {
		t.Error("Report mutated the input slice")
	}
}
