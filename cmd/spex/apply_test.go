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

func (s *noopStore) Create(r mapping.Record) (int, error)                      { return 0, nil }
func (s *noopStore) Get(id int) (mapping.Record, error)                        { return mapping.Record{}, fmt.Errorf("not found") }
func (s *noopStore) GetByBead(beadID string) (mapping.Record, error)           { return mapping.Record{}, fmt.Errorf("not found") }
func (s *noopStore) GetBySpecNode(specNodeID string) ([]mapping.Record, error) { return nil, fmt.Errorf("not found") }
func (s *noopStore) Update(id int, updates map[string]string) error            { return nil }
func (s *noopStore) Delete(id int) error                                       { return nil }
func (s *noopStore) List() ([]mapping.Record, error)                           { return nil, nil }

func TestREQ4_ResolveNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"validator": {
			"aabbccddeeff": "SchemaChecker",
			"112233445566": "Content section",
		},
	}

	tests := []struct {
		name   string
		module string
		node   string
		want   string
	}{
		{"known identity hash", "validator", "aabbccddeeff", "SchemaChecker"},
		{"another known hash", "validator", "112233445566", "Content section"},
		{"unknown hash", "validator", "deadbeefcafe", "deadbeefcafe"},
		{"unknown module", "merkle", "aabbccddeeff", "aabbccddeeff"},
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

func TestREQ4_ConvertCreateActions_PassesSpecNodeIDThrough(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"validator": {
			"aabbccddeeff": "ContentResolver",
		},
	}

	creates := []impact.Action{
		{Type: "create", Module: "validator", Node: "aabbccddeeff", NodeType: "component", SpecNodeID: "aabbccddeeff", SpecHash: "abc123"},
	}

	actions := convertCreateActions(creates, modules, nil, &noopStore{}, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Module != "validator" {
		t.Errorf("want module validator, got %q", a.Module)
	}
	if a.Node != "ContentResolver" {
		t.Errorf("want node ContentResolver, got %q", a.Node)
	}
	if a.NodeType != "component" {
		t.Errorf("want nodeType component, got %q", a.NodeType)
	}
	if a.SpecHash != "abc123" {
		t.Errorf("want specHash abc123, got %q", a.SpecHash)
	}
	if a.SpecNodeID != "aabbccddeeff" {
		t.Errorf("want specNodeID aabbccddeeff, got %q", a.SpecNodeID)
	}
}

func TestREQ4_ConvertCreateActions_FallbackNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{}

	creates := []impact.Action{
		{Type: "create", Module: "validator", Node: "aabbccddeeff", NodeType: "component", SpecNodeID: "aabbccddeeff", SpecHash: "xyz"},
	}

	actions := convertCreateActions(creates, modules, nil, &noopStore{}, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "aabbccddeeff" {
		t.Errorf("want fallback node key (identity hash), got %q", actions[0].Node)
	}
	if actions[0].SpecHash != "xyz" {
		t.Errorf("want specHash xyz, got %q", actions[0].SpecHash)
	}
}

func TestREQ4_ConvertCreateActions_ContentFileFromExistingRecord(t *testing.T) {
	store := newSpecNodeStore()
	store.addRecord(mapping.Record{
		ID:          1,
		SpecNodeID:  "aabbccddeeff",
		BeadID:      "old-bead-1",
		ContentFile: "spec/validator/arch_content_resolver.md",
	})

	creates := []impact.Action{
		{Type: "create", Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "aabbccddeeff", OldBeadID: "old-bead-1", SpecHash: "h1"},
	}

	actions := convertCreateActions(creates, map[string]impact.NodeMap{}, nil, store, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].SpecNodeID != "aabbccddeeff" {
		t.Errorf("want specNodeID from impact action, got %q", actions[0].SpecNodeID)
	}
	if actions[0].ContentFile != "spec/validator/arch_content_resolver.md" {
		t.Errorf("want ContentFile from existing record, got %q", actions[0].ContentFile)
	}
	if actions[0].OldBeadID != "old-bead-1" {
		t.Errorf("want OldBeadID old-bead-1, got %q", actions[0].OldBeadID)
	}
}

func TestREQ4_ConvertCreateActions_ContentFileFromSpecGraph(t *testing.T) {
	contents := map[string]ContentMap{
		"validator": {
			"aabbccddeeff": "spec/validator/arch_new_component.md",
		},
	}

	creates := []impact.Action{
		{Type: "create", Module: "validator", Node: "NewComponent", NodeType: "component", SpecNodeID: "aabbccddeeff", SpecHash: "h1"},
	}

	actions := convertCreateActions(creates, map[string]impact.NodeMap{}, contents, &noopStore{}, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].ContentFile != "spec/validator/arch_new_component.md" {
		t.Errorf("want ContentFile from spec graph, got %q", actions[0].ContentFile)
	}
}

func TestREQ4_ConvertObsoleteActions(t *testing.T) {
	obsoletes := []impact.Action{
		{Type: "obsolete", BeadID: "bead-2", Module: "validator", Node: "LegacyChecker", SpecNodeID: "deadbeefcafe", ChangeType: "removed", Reason: "Spec node removed"},
		{Type: "obsolete", BeadID: "bead-3", Module: "merkle", Node: "Hasher", SpecNodeID: "112233445566", ChangeType: "modified", Reason: "Spec node modified"},
	}

	actions := convertObsoleteActions(obsoletes)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions, got %d", len(actions))
	}
	if actions[0].ChangeType != "removed" {
		t.Errorf("want ChangeType removed for removed node, got %q", actions[0].ChangeType)
	}
	if actions[0].SpecNodeID != "deadbeefcafe" {
		t.Errorf("want SpecNodeID deadbeefcafe to flow through, got %q", actions[0].SpecNodeID)
	}
	if actions[1].ChangeType != "modified" {
		t.Errorf("want ChangeType modified for modified node, got %q", actions[1].ChangeType)
	}
	if actions[1].SpecNodeID != "112233445566" {
		t.Errorf("want SpecNodeID 112233445566 to flow through, got %q", actions[1].SpecNodeID)
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

// specNodeStore is an in-memory store that supports GetBySpecNode.
type specNodeStore struct {
	noopStore
	records []mapping.Record
}

func newSpecNodeStore() *specNodeStore {
	return &specNodeStore{}
}

func (s *specNodeStore) addRecord(r mapping.Record) {
	s.records = append(s.records, r)
}

func (s *specNodeStore) GetBySpecNode(specNodeID string) ([]mapping.Record, error) {
	var matches []mapping.Record
	for _, r := range s.records {
		if r.SpecNodeID == specNodeID {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("not found: %s", specNodeID)
	}
	return matches, nil
}
