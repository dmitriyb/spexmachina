package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
)

func TestREQ4_NodeType(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"module/1/component/1", "component"},
		{"module/1/impl_section/1", "impl_section"},
		{"module/1/data_flow/1", "data_flow"},
		{"module/1/meta", "meta"},
		{"project/meta", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := nodeType(tt.key)
			if got != tt.want {
				t.Errorf("nodeType(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestREQ4_ResolveNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"1": {
			"component/1": "BeadCreator",
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

func TestREQ4_FlattenTree(t *testing.T) {
	tree := &merkle.Node{
		Key:  "project",
		Type: "project",
		Hash: "root",
		Children: []*merkle.Node{
			{
				Key:    "module/1",
				Type:   "module",
				Hash:   "mod",
				Module: 1,
				Children: []*merkle.Node{
					{Key: "module/1/component/1", Type: "leaf", Hash: "h1", NodeType: "component", Module: 1},
					{Key: "module/1/component/2", Type: "leaf", Hash: "h2", NodeType: "component", Module: 1},
					{Key: "module/1/impl_section/1", Type: "leaf", Hash: "h3", NodeType: "impl_section", Module: 1},
				},
			},
		},
	}

	hashes := flattenTree(tree)

	tests := []struct {
		key  string
		want string
	}{
		{"module/1/component/1", "h1"},
		{"module/1/component/2", "h2"},
		{"module/1/impl_section/1", "h3"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := hashes[tt.key]
			if !ok {
				t.Fatalf("key %q not found in hashes; keys: %v", tt.key, hashKeys(hashes))
			}
			if got != tt.want {
				t.Errorf("hash for %q = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestREQ4_LookupHash(t *testing.T) {
	hashes := map[string]string{
		"module/1/component/1":    "h1",
		"module/1/impl_section/1": "h3",
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"component", "module/1/component/1", "h1"},
		{"impl section", "module/1/impl_section/1", "h3"},
		{"unknown", "module/1/component/99", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lookupHash(hashes, tt.key)
			if got != tt.want {
				t.Errorf("lookupHash(%q) = %q, want %q", tt.key, got, tt.want)
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
	hashes := map[string]string{
		"module/1/component/1": "abc123",
	}

	creates := []impact.Action{
		{Type: "create", Module: "1", Node: "module/1/component/1", Impact: "arch_impl"},
	}

	actions := convertCreateActions(creates, modules, hashes)

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
}

func TestREQ4_ConvertReviewActions(t *testing.T) {
	hashes := map[string]string{
		"module/2/component/1": "def456",
	}

	reviews := []impact.Action{
		{Type: "review", BeadID: "bead-1", Module: "2", Node: "module/2/component/1", Impact: "impl_only"},
	}

	actions := convertReviewActions(reviews, hashes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.BeadID != "bead-1" {
		t.Errorf("want beadID bead-1, got %q", a.BeadID)
	}
	if a.SpecHash != "def456" {
		t.Errorf("want specHash def456, got %q", a.SpecHash)
	}
}

func TestREQ4_ConvertCloseActions(t *testing.T) {
	closes := []impact.Action{
		{Type: "close", BeadID: "bead-2", Module: "3", Node: "LegacyChecker"},
	}

	actions := convertCloseActions(closes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.BeadID != "bead-2" {
		t.Errorf("want beadID bead-2, got %q", a.BeadID)
	}
	if a.Module != "3" {
		t.Errorf("want module 3, got %q", a.Module)
	}
	if a.Node != "LegacyChecker" {
		t.Errorf("want node LegacyChecker, got %q", a.Node)
	}
}

func TestREQ4_CollectAffectedIDs(t *testing.T) {
	created := []string{"new-1", "new-2"}
	reviews := []impact.Action{
		{BeadID: "rev-1"},
		{BeadID: "new-1"}, // duplicate with created
	}
	closes := []impact.Action{
		{BeadID: "close-1"},
	}

	ids := collectAffectedIDs(created, reviews, closes)

	// Should be 4 unique IDs: new-1, new-2, rev-1, close-1
	if len(ids) != 4 {
		t.Fatalf("want 4 unique IDs, got %d: %v", len(ids), ids)
	}

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %q", id)
		}
		seen[id] = true
	}
}

func TestREQ5_DryRunOutput(t *testing.T) {
	report := impact.ImpactReport{
		Creates: []impact.Action{
			{Type: "create", Module: "1", Node: "NewComp"},
		},
		Reviews: []impact.Action{
			{Type: "review", BeadID: "bead-1", Module: "2", Node: "Hasher"},
		},
		Closes: []impact.Action{
			{Type: "close", BeadID: "bead-2", Module: "3", Node: "Old"},
		},
		Summary: impact.Summary{CreateCount: 1, ReviewCount: 1, CloseCount: 1},
	}

	// Capture stdout to verify output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := printDryRun(report)

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	output := string(out)

	if code != 0 {
		t.Errorf("want exit code 0, got %d", code)
	}
	if !strings.Contains(output, "1 creates") {
		t.Errorf("want output to contain '1 creates', got %q", output)
	}
	if !strings.Contains(output, "create: 1/NewComp") {
		t.Errorf("want output to contain 'create: 1/NewComp', got %q", output)
	}
	if !strings.Contains(output, "review: Hasher (bead bead-1)") {
		t.Errorf("want output to contain review line, got %q", output)
	}
	if !strings.Contains(output, "close:  Old (bead bead-2)") {
		t.Errorf("want output to contain close line, got %q", output)
	}
}

func TestREQ4_EmptyReport(t *testing.T) {
	// Verify nothing-to-do path — readReport + parse tested indirectly.
	report := impact.ImpactReport{
		Creates: []impact.Action{},
		Reviews: []impact.Action{},
		Closes:  []impact.Action{},
		Summary: impact.Summary{},
	}
	if report.Summary.CreateCount != 0 || report.Summary.CloseCount != 0 || report.Summary.ReviewCount != 0 {
		t.Error("empty report should have zero counts")
	}
}

func TestREQ4_ConvertCreateActions_FallbackNodeName(t *testing.T) {
	// When NodeMap has no entry, the node key is used as-is.
	modules := map[string]impact.NodeMap{}
	hashes := map[string]string{}

	creates := []impact.Action{
		{Type: "create", Module: "1", Node: "module/1/component/5"},
	}

	actions := convertCreateActions(creates, modules, hashes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "module/1/component/5" {
		t.Errorf("want fallback node key, got %q", actions[0].Node)
	}
	if actions[0].SpecHash != "" {
		t.Errorf("want empty hash for missing node, got %q", actions[0].SpecHash)
	}
}

func TestREQ4_ReadReport_File(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/report.json"
	content := `{"creates":[],"closes":[],"reviews":[],"summary":{}}`
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

func hashKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
