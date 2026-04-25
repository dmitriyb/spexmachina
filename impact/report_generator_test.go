package impact

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// mkChange is a test helper to build a ClassifiedChange.
func mkChange(path, typ, oldHash, newHash, nodeType, module string) merkle.ClassifiedChange {
	var ct merkle.ChangeType
	switch typ {
	case "added":
		ct = merkle.Added
	case "modified":
		ct = merkle.Modified
	case "removed":
		ct = merkle.Removed
	}
	return merkle.ClassifiedChange{
		Change: merkle.Change{Path: path, Type: ct, OldHash: oldHash, NewHash: newHash, NodeType: nodeType},
		Impact: merkle.ArchImpl,
		Module: module,
	}
}

// Ensure mapping import is used (referenced in S10 test).
var _ = mapping.Record{}

// --- S7: ReportGenerator produces valid JSON with correct structure ---

func TestFR4_S7_GenerateReportGroupsByActionType(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "validator", Node: "SchemaChecker", Reason: "Spec node modified (new): validator/SchemaChecker"},
		{Type: "obsolete", BeadID: "spexmachina-abc", Module: "validator", Node: "SchemaChecker", Reason: "Spec node modified: validator/SchemaChecker"},
		{Type: "create", Module: "merkle", Node: "Hash computation", Reason: "Spec node modified (new): merkle/Hash computation"},
		{Type: "obsolete", BeadID: "spexmachina-def", Module: "merkle", Node: "Hash computation", Reason: "Spec node modified: merkle/Hash computation"},
		{Type: "create", Module: "validator", Node: "OrphanDetector", Reason: "New spec node: validator/OrphanDetector"},
		{Type: "obsolete", BeadID: "spexmachina-ghi", Module: "merkle", Node: "LegacyHasher", Reason: "Spec node removed: merkle/LegacyHasher"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(report.Creates) != 3 {
		t.Errorf("want 3 creates, got %d", len(report.Creates))
	}
	if len(report.Obsoletes) != 3 {
		t.Errorf("want 3 obsoletes, got %d", len(report.Obsoletes))
	}
	if report.Summary.CreateCount != 3 {
		t.Errorf("want create_count 3, got %d", report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 3 {
		t.Errorf("want obsolete_count 3, got %d", report.Summary.ObsoleteCount)
	}
}

// --- S8: ReportGenerator uses 2-space indentation ---

func TestFR4_S8_GenerateReportJSONIndented(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "a", Node: "X"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(buf.String(), "\n")
	foundIndent := false
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			foundIndent = true
			break
		}
	}
	if !foundIndent {
		t.Error("want 2-space indented JSON output")
	}
}

// --- S9: ReportGenerator groups actions correctly ---

func TestFR4_S9_GenerateReportSummaryCounts(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "a", Node: "X"},
		{Type: "create", Module: "a", Node: "Y"},
		{Type: "obsolete", BeadID: "b-1", Module: "a", Node: "Z"},
		{Type: "obsolete", BeadID: "b-2", Module: "a", Node: "W"},
		{Type: "obsolete", BeadID: "b-3", Module: "a", Node: "V"},
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
	if len(report.Obsoletes) != 3 {
		t.Errorf("want 3 obsoletes, got %d", len(report.Obsoletes))
	}
	if report.Summary.CreateCount != 2 {
		t.Errorf("want create_count 2, got %d", report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 3 {
		t.Errorf("want obsolete_count 3, got %d", report.Summary.ObsoleteCount)
	}
}

// --- E1: Empty inputs produce empty report ---

func TestFR4_E1_GenerateReportEmptyActions(t *testing.T) {
	var buf bytes.Buffer
	if err := GenerateReport(nil, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"creates": []`) {
		t.Error("want creates as empty array, got null or missing")
	}
	if !strings.Contains(output, `"obsoletes": []`) {
		t.Error("want obsoletes as empty array, got null or missing")
	}

	if report.Summary.CreateCount != 0 {
		t.Errorf("want create_count 0, got %d", report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 0 {
		t.Errorf("want obsolete_count 0, got %d", report.Summary.ObsoleteCount)
	}
}

// --- E2: ReportGenerator handles nil writer ---

func TestFR4_E2_GenerateReportNilWriter(t *testing.T) {
	actions := []Action{{Type: "create", Module: "a", Node: "X"}}
	err := GenerateReport(actions, nil)
	if err == nil {
		t.Fatal("want error for nil writer, got nil")
	}
}

// --- E3: Actions with empty strings in fields ---

func TestFR4_E3_GenerateReportEmptyStringFields(t *testing.T) {
	actions := []Action{{Type: "create", Module: "", Node: ""}}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON with empty fields: %v", err)
	}
	if len(report.Creates) != 1 {
		t.Errorf("want 1 create, got %d", len(report.Creates))
	}
}

// --- E4: Very large action list ---

func TestFR4_E4_GenerateReportLargeActionList(t *testing.T) {
	actions := make([]Action, 10000)
	for i := range actions {
		if i%2 == 0 {
			actions[i] = Action{Type: "create", Module: "mod", Node: "node"}
		} else {
			actions[i] = Action{Type: "obsolete", BeadID: "bead", Module: "mod", Node: "node"}
		}
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if report.Summary.CreateCount != 5000 {
		t.Errorf("want 5000 creates, got %d", report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 5000 {
		t.Errorf("want 5000 obsoletes, got %d", report.Summary.ObsoleteCount)
	}
}

// --- E6: Round-trip JSON ---

func TestFR4_E6_GenerateReportRoundTrip(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "validator", Node: "SchemaChecker", SpecHash: "abc123", Reason: "New spec node: validator/SchemaChecker"},
		{Type: "obsolete", BeadID: "spex-001", Module: "merkle", Node: "Hasher", Reason: "Spec node removed: merkle/Hasher"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(report.Creates) != 1 {
		t.Fatalf("want 1 create, got %d", len(report.Creates))
	}
	c := report.Creates[0]
	if c.Module != "validator" || c.Node != "SchemaChecker" || c.SpecHash != "abc123" {
		t.Errorf("round-trip mismatch: got %+v", c)
	}

	if len(report.Obsoletes) != 1 {
		t.Fatalf("want 1 obsolete, got %d", len(report.Obsoletes))
	}
	o := report.Obsoletes[0]
	if o.BeadID != "spex-001" || o.Module != "merkle" || o.Node != "Hasher" {
		t.Errorf("round-trip mismatch: got %+v", o)
	}
}

// --- NFR5: Deterministic output ---

func TestNFR5_GenerateReportDeterministic(t *testing.T) {
	actions := []Action{
		{Type: "obsolete", BeadID: "b-1", Module: "m", Node: "A"},
		{Type: "create", Module: "m", Node: "B"},
		{Type: "obsolete", BeadID: "b-2", Module: "m", Node: "C"},
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

// --- D10: ReportGenerator includes DepSpecNodeIDs in create action JSON ---

func TestFR4_D10_GenerateReportIncludesDepSpecNodeIDs(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "impact", Node: "ActionClassifier", DepSpecNodeIDs: []string{"abc123", "def456"}, Reason: "New spec node"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"dep_spec_node_ids"`) {
		t.Error("want dep_spec_node_ids in JSON output")
	}
	if !strings.Contains(output, `"abc123"`) || !strings.Contains(output, `"def456"`) {
		t.Error("want both dep spec_node ids in output")
	}
}

// --- D11: ReportGenerator omits dep_spec_node_ids when empty ---

func TestFR4_D11_GenerateReportOmitsEmptyDepSpecNodeIDs(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "impact", Node: "ActionClassifier", Reason: "New spec node"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, `"dep_spec_node_ids"`) {
		t.Error("want dep_spec_node_ids omitted for empty/nil DepSpecNodeIDs")
	}
}

// --- S10: Full pipeline - ClassifyActions into GenerateReport ---

func TestFR4_S10_FullPipeline(t *testing.T) {
	actions := ClassifyActions(
		nil,
		[]Match{
			{
				Change: mkChange("module/1/component/1", "modified", "a", "b", "component", "alpha"),
				Records: []mapping.Record{
					{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp1"},
				},
			},
		},
		[]Unmatched{
			{Change: mkChange("module/1/component/5", "added", "", "c", "component", "alpha")},
		},
		nil,
	)

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// 2 creates (1 from modified match + 1 from added unmatched), 1 obsolete
	if report.Summary.CreateCount != 2 {
		t.Errorf("want 2 creates, got %d", report.Summary.CreateCount)
	}
	if report.Summary.ObsoleteCount != 1 {
		t.Errorf("want 1 obsolete, got %d", report.Summary.ObsoleteCount)
	}
}

// --- Preserved action fields ---

func TestFR4_GenerateReportPreservesActionFields(t *testing.T) {
	actions := []Action{
		{
			Type:     "obsolete",
			BeadID:   "spexmachina-def",
			Module:   "merkle",
			Node:     "Hasher",
			NodeType: "component",
			Reason:   "Spec node modified: merkle/Hasher",
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

	if len(report.Obsoletes) != 1 {
		t.Fatalf("want 1 obsolete, got %d", len(report.Obsoletes))
	}

	r := report.Obsoletes[0]
	if r.Type != "obsolete" {
		t.Errorf("want type obsolete, got %q", r.Type)
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
	if r.Reason != "Spec node modified: merkle/Hasher" {
		t.Errorf("want reason preserved, got %q", r.Reason)
	}
}
