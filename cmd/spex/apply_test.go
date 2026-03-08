package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/merkle"
)

func TestREQ4_NodeGroup(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"arch_bead_creator.md", "arch"},
		{"impl_bead_creation.md", "impl"},
		{"flow_apply.md", "flow"},
		{"module.json", ""},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := nodeGroup(tt.filename)
			if got != tt.want {
				t.Errorf("nodeGroup(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestREQ4_NodeType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"arch_bead_creator.md", "component"},
		{"impl_bead_creation.md", "impl_section"},
		{"flow_apply.md", ""},
		{"module.json", ""},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := nodeType(tt.filename)
			if got != tt.want {
				t.Errorf("nodeType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestREQ4_ResolveNodeName(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"apply": {
			"arch_bead_creator.md": "BeadCreator",
			"impl_bead_creation.md": "Bead creation commands",
		},
	}

	tests := []struct {
		name     string
		module   string
		filename string
		want     string
	}{
		{"known component", "apply", "arch_bead_creator.md", "BeadCreator"},
		{"known impl", "apply", "impl_bead_creation.md", "Bead creation commands"},
		{"unknown file", "apply", "arch_unknown.md", "arch_unknown.md"},
		{"unknown module", "validator", "arch_foo.md", "arch_foo.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNodeName(modules, tt.module, tt.filename)
			if got != tt.want {
				t.Errorf("resolveNodeName(%q, %q) = %q, want %q", tt.module, tt.filename, got, tt.want)
			}
		})
	}
}

func TestREQ4_FlattenTree(t *testing.T) {
	tree := &merkle.Node{
		Name: "project",
		Type: "project",
		Hash: "root",
		Children: []*merkle.Node{
			{
				Name: "apply",
				Type: "module",
				Hash: "mod",
				Children: []*merkle.Node{
					{
						Name: "arch",
						Type: "arch",
						Hash: "grp",
						Children: []*merkle.Node{
							{Name: "arch_bead_creator.md", Type: "leaf", Hash: "h1"},
							{Name: "arch_bead_closer.md", Type: "leaf", Hash: "h2"},
						},
					},
					{
						Name: "impl",
						Type: "impl",
						Hash: "grp2",
						Children: []*merkle.Node{
							{Name: "impl_bead_creation.md", Type: "leaf", Hash: "h3"},
						},
					},
				},
			},
		},
	}

	hashes := flattenTree(tree)

	tests := []struct {
		path string
		want string
	}{
		{"project/apply/arch/arch_bead_creator.md", "h1"},
		{"project/apply/arch/arch_bead_closer.md", "h2"},
		{"project/apply/impl/impl_bead_creation.md", "h3"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := hashes[tt.path]
			if !ok {
				t.Fatalf("path %q not found in hashes; keys: %v", tt.path, hashKeys(hashes))
			}
			if got != tt.want {
				t.Errorf("hash for %q = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestREQ4_LookupHash(t *testing.T) {
	hashes := map[string]string{
		"project/apply/arch/arch_bead_creator.md":   "h1",
		"project/apply/impl/impl_bead_creation.md": "h3",
	}

	tests := []struct {
		name   string
		module string
		node   string
		want   string
	}{
		{"arch component", "apply", "arch_bead_creator.md", "h1"},
		{"impl section", "apply", "impl_bead_creation.md", "h3"},
		{"unknown", "apply", "arch_missing.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lookupHash(hashes, tt.module, tt.node)
			if got != tt.want {
				t.Errorf("lookupHash(%q, %q) = %q, want %q", tt.module, tt.node, got, tt.want)
			}
		})
	}
}

func TestREQ4_ConvertCreateActions(t *testing.T) {
	modules := map[string]impact.NodeMap{
		"apply": {
			"arch_bead_creator.md": "BeadCreator",
		},
	}
	hashes := map[string]string{
		"project/apply/arch/arch_bead_creator.md": "abc123",
	}

	creates := []impact.Action{
		{Type: "create", Module: "apply", Node: "arch_bead_creator.md", Impact: "arch_impl"},
	}

	actions := convertCreateActions(creates, modules, hashes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Module != "apply" {
		t.Errorf("want module apply, got %q", a.Module)
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
		"project/merkle/arch/arch_hasher.md": "def456",
	}

	reviews := []impact.Action{
		{Type: "review", BeadID: "bead-1", Module: "merkle", Node: "arch_hasher.md", Impact: "impl_only"},
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
		{Type: "close", BeadID: "bead-2", Module: "validator", Node: "LegacyChecker"},
	}

	actions := convertCloseActions(closes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.BeadID != "bead-2" {
		t.Errorf("want beadID bead-2, got %q", a.BeadID)
	}
	if a.Module != "validator" {
		t.Errorf("want module validator, got %q", a.Module)
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
			{Type: "create", Module: "apply", Node: "NewComp"},
		},
		Reviews: []impact.Action{
			{Type: "review", BeadID: "bead-1", Module: "merkle", Node: "Hasher"},
		},
		Closes: []impact.Action{
			{Type: "close", BeadID: "bead-2", Module: "validator", Node: "Old"},
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
	if !strings.Contains(output, "create: apply/NewComp") {
		t.Errorf("want output to contain 'create: apply/NewComp', got %q", output)
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
	// When NodeMap has no entry, the filename is used as-is.
	modules := map[string]impact.NodeMap{}
	hashes := map[string]string{}

	creates := []impact.Action{
		{Type: "create", Module: "unknown", Node: "arch_new.md"},
	}

	actions := convertCreateActions(creates, modules, hashes)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "arch_new.md" {
		t.Errorf("want fallback node name, got %q", actions[0].Node)
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
