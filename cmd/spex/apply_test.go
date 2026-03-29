package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
)

// noopStore is a minimal mapping.Store for testing conversions.
type noopStore struct{}

func (s *noopStore) Create(r mapping.Record) (int, error)                    { return 0, nil }
func (s *noopStore) Get(id int) (mapping.Record, error)                      { return mapping.Record{}, fmt.Errorf("not found") }
func (s *noopStore) GetByBead(beadID string) (mapping.Record, error)         { return mapping.Record{}, fmt.Errorf("not found") }
func (s *noopStore) GetBySpecNode(specNodeID string) ([]mapping.Record, error) { return nil, fmt.Errorf("not found") }
func (s *noopStore) Update(id int, updates map[string]string) error          { return nil }
func (s *noopStore) Delete(id int) error                                     { return nil }
func (s *noopStore) List() ([]mapping.Record, error)                         { return nil, nil }

func TestREQ4_ResolveNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"1": {
			"component/1":    "BeadCreator",
			"impl_section/2": "Bead creation commands",
		},
	}

	tests := []struct {
		name   string
		module string
		node   string
		want   string
	}{
		{"known component via spec-ID", "1", "module/1/component/1", "BeadCreator"},
		{"known impl via spec-ID", "1", "module/1/impl_section/2", "Bead creation commands"},
		{"unknown node", "1", "module/1/component/99", "module/1/component/99"},
		{"unknown module", "2", "module/2/component/1", "module/2/component/1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNodeName(modules, tt.module, tt.node)
			if got != tt.want {
				t.Errorf("resolveNodeName(%q, %q) = %q, want %q", tt.module, tt.node, got, tt.want)
			}
		})
	}
}

func TestREQ4_ConvertCreateActions(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"1": {
			"component/1": "BeadCreator",
		},
	}

	creates := []impact.Action{
		{Type: "create", Module: "1", Node: "module/1/component/1", NodeType: "component", SpecHash: "abc123"},
	}

	actions := convertCreateActions(creates, modules, &noopStore{})

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Module != "1" {
		t.Errorf("want module 1, got %q", a.Module)
	}
	if a.Node != "BeadCreator" {
		t.Errorf("want node BeadCreator, got %q", a.Node)
	}
	if a.NodeType != "component" {
		t.Errorf("want nodeType component, got %q", a.NodeType)
	}
	if a.SpecHash != "abc123" {
		t.Errorf("want specHash abc123, got %q", a.SpecHash)
	}
	if a.SpecNodeID != "1/component/1" {
		t.Errorf("want specNodeID 1/component/1, got %q", a.SpecNodeID)
	}
}

func TestREQ4_ConvertCreateActions_FallbackNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{}

	creates := []impact.Action{
		{Type: "create", Module: "1", Node: "module/1/component/5", NodeType: "component", SpecHash: "xyz"},
	}

	actions := convertCreateActions(creates, modules, &noopStore{})

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "module/1/component/5" {
		t.Errorf("want fallback node key, got %q", actions[0].Node)
	}
	if actions[0].SpecHash != "xyz" {
		t.Errorf("want specHash xyz, got %q", actions[0].SpecHash)
	}
}

func TestREQ4_ConvertCreateActions_OldBeadIDLookup(t *testing.T) {
	store := &lookupStore{
		records: map[string]mapping.Record{
			"old-bead-1": {SpecNodeID: "validator/component/1"},
		},
	}
	modules := map[string]impact.NodeMap{}

	creates := []impact.Action{
		{Type: "create", Module: "validator", Node: "ContentResolver", NodeType: "component", OldBeadID: "old-bead-1", SpecHash: "h1"},
	}

	actions := convertCreateActions(creates, modules, store)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].SpecNodeID != "validator/component/1" {
		t.Errorf("want specNodeID from mapping record, got %q", actions[0].SpecNodeID)
	}
	if actions[0].OldBeadID != "old-bead-1" {
		t.Errorf("want OldBeadID old-bead-1, got %q", actions[0].OldBeadID)
	}
}

func TestREQ4_ConvertObsoleteActions(t *testing.T) {
	obsoletes := []impact.Action{
		{Type: "obsolete", BeadID: "bead-2", Module: "3", Node: "LegacyChecker", ChangeType: "removed", Reason: "Spec node removed: 3/LegacyChecker"},
		{Type: "obsolete", BeadID: "bead-3", Module: "3", Node: "Hasher", ChangeType: "modified", Reason: "Spec node modified: 3/Hasher"},
	}

	actions := convertObsoleteActions(obsoletes)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions, got %d", len(actions))
	}
	if actions[0].ChangeType != "removed" {
		t.Errorf("want ChangeType removed for removed node, got %q", actions[0].ChangeType)
	}
	if actions[1].ChangeType != "modified" {
		t.Errorf("want ChangeType modified for modified node, got %q", actions[1].ChangeType)
	}
}

func TestREQ4_DeriveSpecNodeID(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		node     string
		nodeType string
		want     string
	}{
		{"merkle key component", "validator", "module/1/component/2", "component", "validator/component/2"},
		{"merkle key module", "validator", "module/1", "module", "validator/module"},
		{"human name fallback", "validator", "ContentResolver", "component", "validator/component"},
		{"no node type", "validator", "ContentResolver", "", "validator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveSpecNodeID(tt.module, tt.node, tt.nodeType)
			if got != tt.want {
				t.Errorf("deriveSpecNodeID(%q, %q, %q) = %q, want %q", tt.module, tt.node, tt.nodeType, got, tt.want)
			}
		})
	}
}

func TestREQ4_EmptyReport(t *testing.T) {
	report := impact.ImpactReport{
		Creates:   []impact.Action{},
		Obsoletes: []impact.Action{},
		Summary:   impact.Summary{},
	}
	if report.Summary.CreateCount != 0 || report.Summary.ObsoleteCount != 0 {
		t.Error("empty report should have zero counts")
	}
}

func TestREQ4_ReadReport_File(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/report.json"
	content := `{"creates":[],"obsoletes":[],"summary":{}}`
	os.WriteFile(path, []byte(content), 0644)

	data, err := readReport(path)
	if err != nil {
		t.Fatalf("readReport: %v", err)
	}
	if !strings.Contains(string(data), "creates") {
		t.Error("want report data containing 'creates'")
	}
}

func TestREQ4_ReadReport_MissingFile(t *testing.T) {
	_, err := readReport("/nonexistent/report.json")
	if err == nil {
		t.Fatal("want error for missing file, got nil")
	}
}

// --- S12: --proposal flag is required ---

func TestREQ8_S12_ProposalFlagRequired(t *testing.T) {
	cmd := newApplyCmd()
	cmd.SetArgs([]string{"--report", "/dev/null"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("want error when --proposal is omitted, got nil")
	}
	if !strings.Contains(err.Error(), "proposal") {
		t.Errorf("want error mentioning 'proposal', got: %v", err)
	}
}

// lookupStore is a minimal store that returns records by bead ID.
type lookupStore struct {
	noopStore
	records map[string]mapping.Record
}

func (s *lookupStore) GetByBead(beadID string) (mapping.Record, error) {
	if r, ok := s.records[beadID]; ok {
		return r, nil
	}
	return mapping.Record{}, fmt.Errorf("not found: %s", beadID)
}
