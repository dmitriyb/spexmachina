package impact

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFR4_GenerateReportGroupsByActionType(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "validator", Node: "ContentResolver", Impact: "arch_impl", Reason: "New spec node: validator/ContentResolver"},
		{Type: "close", BeadID: "spexmachina-abc", Module: "validator", Node: "LegacyChecker", Reason: "Spec node removed: validator/LegacyChecker"},
		{Type: "review", BeadID: "spexmachina-def", Module: "merkle", Node: "Hasher", Impact: "impl_only", Reason: "Spec node modified (impl_only): merkle/Hasher"},
		{Type: "create", Module: "impact", Node: "Reporter", Impact: "arch_impl", Reason: "New spec node: impact/Reporter"},
		{Type: "review", BeadID: "spexmachina-ghi", Module: "schema", Node: "Loader", Impact: "arch_impl", Reason: "Spec node modified (arch_impl): schema/Loader"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(report.Creates) != 2 {
		t.Errorf("want 2 creates, got %d", len(report.Creates))
	}
	if len(report.Closes) != 1 {
		t.Errorf("want 1 close, got %d", len(report.Closes))
	}
	if len(report.Reviews) != 2 {
		t.Errorf("want 2 reviews, got %d", len(report.Reviews))
	}
}

func TestFR4_GenerateReportSummaryCounts(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "a", Node: "X"},
		{Type: "create", Module: "a", Node: "Y"},
		{Type: "close", BeadID: "b-1", Module: "a", Node: "Z"},
		{Type: "review", BeadID: "b-2", Module: "a", Node: "W"},
		{Type: "review", BeadID: "b-3", Module: "a", Node: "V"},
		{Type: "review", BeadID: "b-4", Module: "a", Node: "U"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if report.Summary.CreateCount != 2 {
		t.Errorf("want create_count 2, got %d", report.Summary.CreateCount)
	}
	if report.Summary.CloseCount != 1 {
		t.Errorf("want close_count 1, got %d", report.Summary.CloseCount)
	}
	if report.Summary.ReviewCount != 3 {
		t.Errorf("want review_count 3, got %d", report.Summary.ReviewCount)
	}
}

func TestFR4_GenerateReportEmptyActions(t *testing.T) {
	var buf bytes.Buffer
	if err := GenerateReport(nil, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Empty arrays, not null.
	output := buf.String()
	if !strings.Contains(output, `"creates": []`) {
		t.Error("want creates as empty array, got null or missing")
	}
	if !strings.Contains(output, `"closes": []`) {
		t.Error("want closes as empty array, got null or missing")
	}
	if !strings.Contains(output, `"reviews": []`) {
		t.Error("want reviews as empty array, got null or missing")
	}

	if report.Summary.CreateCount != 0 {
		t.Errorf("want create_count 0, got %d", report.Summary.CreateCount)
	}
	if report.Summary.CloseCount != 0 {
		t.Errorf("want close_count 0, got %d", report.Summary.CloseCount)
	}
	if report.Summary.ReviewCount != 0 {
		t.Errorf("want review_count 0, got %d", report.Summary.ReviewCount)
	}
}

func TestFR4_GenerateReportJSONIndented(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "a", Node: "X"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2-space indentation means lines starting with "  ".
	lines := strings.Split(buf.String(), "\n")
	foundIndent := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") {
			foundIndent = true
			break
		}
	}
	if !foundIndent {
		t.Error("want 2-space indented JSON output")
	}
}

func TestNFR5_GenerateReportDeterministic(t *testing.T) {
	actions := []Action{
		{Type: "review", BeadID: "b-1", Module: "m", Node: "A"},
		{Type: "create", Module: "m", Node: "B"},
		{Type: "close", BeadID: "b-2", Module: "m", Node: "C"},
	}

	var buf1, buf2 bytes.Buffer
	if err := GenerateReport(actions, &buf1); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if err := GenerateReport(actions, &buf2); err != nil {
		t.Fatalf("run 2: %v", err)
	}

	if buf1.String() != buf2.String() {
		t.Error("want identical output for same input, got different results")
	}
}

func TestFR4_GenerateReportPreservesActionFields(t *testing.T) {
	actions := []Action{
		{
			Type:   "review",
			BeadID: "spexmachina-def",
			Module: "merkle",
			Node:   "Hasher",
			Impact: "impl_only",
			Reason: "Spec node modified (impl_only): merkle/Hasher",
		},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(report.Reviews) != 1 {
		t.Fatalf("want 1 review, got %d", len(report.Reviews))
	}

	r := report.Reviews[0]
	if r.Type != "review" {
		t.Errorf("want type review, got %q", r.Type)
	}
	if r.BeadID != "spexmachina-def" {
		t.Errorf("want bead_id spexmachina-def, got %q", r.BeadID)
	}
	if r.Module != "merkle" {
		t.Errorf("want module merkle, got %q", r.Module)
	}
	if r.Node != "Hasher" {
		t.Errorf("want node Hasher, got %q", r.Node)
	}
	if r.Impact != "impl_only" {
		t.Errorf("want impact impl_only, got %q", r.Impact)
	}
	if r.Reason != "Spec node modified (impl_only): merkle/Hasher" {
		t.Errorf("want reason preserved, got %q", r.Reason)
	}
}
