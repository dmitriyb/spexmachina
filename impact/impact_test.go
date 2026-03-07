package impact

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR2_MatchNodes_ComponentMatch(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/validator/arch/arch_schema_checker.md", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}
	beads := []BeadSpec{
		{ID: "bead-1", Module: "validator", Component: "SchemaChecker"},
		{ID: "bead-2", Module: "validator", Component: "OtherComp"},
	}

	matched, unmatched, orphaned := MatchNodes(changes, beads)

	if len(matched) != 1 {
		t.Fatalf("want 1 match, got %d", len(matched))
	}
	if len(matched[0].Beads) != 1 || matched[0].Beads[0].ID != "bead-1" {
		t.Fatalf("want bead-1 matched, got %+v", matched[0].Beads)
	}
	if len(unmatched) != 0 {
		t.Fatalf("want 0 unmatched, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Fatalf("want 0 orphaned, got %d", len(orphaned))
	}
}

func TestFR2_MatchNodes_ImplSectionMatch(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/merkle/impl/impl_hash_algo.md", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "merkle",
		},
	}
	beads := []BeadSpec{
		{ID: "bead-1", Module: "merkle", ImplSection: "HashAlgo"},
	}

	matched, unmatched, _ := MatchNodes(changes, beads)

	if len(matched) != 1 {
		t.Fatalf("want 1 match, got %d", len(matched))
	}
	if matched[0].Beads[0].ID != "bead-1" {
		t.Fatalf("want bead-1, got %s", matched[0].Beads[0].ID)
	}
	if len(unmatched) != 0 {
		t.Fatalf("want 0 unmatched, got %d", len(unmatched))
	}
}

func TestFR2_MatchNodes_Unmatched(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/validator/arch/arch_new_thing.md", Type: merkle.Added},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, unmatched, _ := MatchNodes(changes, nil)

	if len(matched) != 0 {
		t.Fatalf("want 0 matched, got %d", len(matched))
	}
	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched, got %d", len(unmatched))
	}
}

func TestFR2_MatchNodes_StructuralAffectsAllModuleBeads(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/validator/module.json", Type: merkle.Modified},
			Impact: merkle.Structural,
			Module: "validator",
		},
	}
	beads := []BeadSpec{
		{ID: "bead-1", Module: "validator", Component: "A"},
		{ID: "bead-2", Module: "validator", Component: "B"},
		{ID: "bead-3", Module: "other", Component: "C"},
	}

	matched, _, _ := MatchNodes(changes, beads)

	if len(matched) != 1 {
		t.Fatalf("want 1 match group, got %d", len(matched))
	}
	if len(matched[0].Beads) != 2 {
		t.Fatalf("want 2 beads affected, got %d", len(matched[0].Beads))
	}
}

func TestFR3_ClassifyActions_DecisionTable(t *testing.T) {
	tests := []struct {
		name       string
		changeType merkle.ChangeType
		hasBead    bool
		wantAction string
	}{
		{"added no bead", merkle.Added, false, "create"},
		{"added with bead", merkle.Added, true, "review"},
		{"modified no bead", merkle.Modified, false, "create"},
		{"modified with bead", merkle.Modified, true, "review"},
		{"removed no bead", merkle.Removed, false, ""},
		{"removed with bead", merkle.Removed, true, "close"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := merkle.ClassifiedChange{
				Change: merkle.Change{
					Path: "project/validator/arch/arch_checker.md",
					Type: tt.changeType,
				},
				Impact: merkle.ArchImpl,
				Module: "validator",
			}

			var matched []Match
			var unmatched []merkle.ClassifiedChange
			var orphaned []BeadSpec

			if tt.hasBead {
				if tt.changeType == merkle.Removed {
					// Removed + bead = orphaned path
					orphaned = []BeadSpec{{ID: "bead-1", Module: "validator", Component: "Checker"}}
				} else {
					matched = []Match{{Change: change, Beads: []BeadSpec{{ID: "bead-1", Module: "validator", Component: "Checker"}}}}
				}
			} else {
				if tt.changeType != merkle.Removed {
					unmatched = []merkle.ClassifiedChange{change}
				}
				// removed + no bead = nothing to add
			}

			actions := ClassifyActions(matched, unmatched, orphaned)

			if tt.wantAction == "" {
				if len(actions) != 0 {
					t.Fatalf("want no actions, got %d: %+v", len(actions), actions)
				}
				return
			}

			if len(actions) != 1 {
				t.Fatalf("want 1 action, got %d: %+v", len(actions), actions)
			}
			if actions[0].Type != tt.wantAction {
				t.Fatalf("want action %q, got %q", tt.wantAction, actions[0].Type)
			}
		})
	}
}

func TestFR3_ClassifyActions_ReasonFormat(t *testing.T) {
	matched := []Match{{
		Change: merkle.ClassifiedChange{
			Change: merkle.Change{Path: "project/merkle/arch/arch_hasher.md", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "merkle",
		},
		Beads: []BeadSpec{{ID: "b-1", Module: "merkle", Component: "Hasher"}},
	}}

	actions := ClassifyActions(matched, nil, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if !strings.Contains(actions[0].Reason, "merkle/Hasher") {
		t.Fatalf("reason should contain module/node, got: %s", actions[0].Reason)
	}
	if !strings.Contains(actions[0].Reason, "impl_only") {
		t.Fatalf("reason should contain impact level, got: %s", actions[0].Reason)
	}
}

func TestFR4_GenerateReport_JSON(t *testing.T) {
	actions := []Action{
		{Type: "create", Module: "validator", Node: "NewComp", Impact: "arch_impl", Reason: "New spec node: validator/NewComp"},
		{Type: "close", BeadID: "b-1", Module: "validator", Node: "OldComp", Reason: "Spec node removed: validator/OldComp"},
		{Type: "review", BeadID: "b-2", Module: "merkle", Node: "Hasher", Impact: "impl_only", Reason: "Spec node modified (impl_only): merkle/Hasher"},
	}

	var buf bytes.Buffer
	if err := GenerateReport(actions, &buf); err != nil {
		t.Fatalf("generate report: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("parse report: %v\nraw: %s", err, buf.String())
	}

	if report.Summary.CreateCount != 1 {
		t.Fatalf("want 1 create, got %d", report.Summary.CreateCount)
	}
	if report.Summary.CloseCount != 1 {
		t.Fatalf("want 1 close, got %d", report.Summary.CloseCount)
	}
	if report.Summary.ReviewCount != 1 {
		t.Fatalf("want 1 review, got %d", report.Summary.ReviewCount)
	}
}

func TestFR4_GenerateReport_EmptyReport(t *testing.T) {
	var buf bytes.Buffer
	if err := GenerateReport(nil, &buf); err != nil {
		t.Fatalf("generate report: %v", err)
	}

	var report ImpactReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("parse report: %v", err)
	}

	if report.Summary.CreateCount != 0 || report.Summary.CloseCount != 0 || report.Summary.ReviewCount != 0 {
		t.Fatalf("empty report should have zero counts, got %+v", report.Summary)
	}
	if report.Creates == nil || report.Closes == nil || report.Reviews == nil {
		t.Fatal("empty arrays should be present, not null")
	}
}

func TestNFR5_Deterministic(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{Change: merkle.Change{Path: "project/b/arch/arch_z.md", Type: merkle.Added}, Impact: merkle.ArchImpl, Module: "b"},
		{Change: merkle.Change{Path: "project/a/arch/arch_y.md", Type: merkle.Added}, Impact: merkle.ArchImpl, Module: "a"},
		{Change: merkle.Change{Path: "project/a/impl/impl_x.md", Type: merkle.Modified}, Impact: merkle.ImplOnly, Module: "a"},
	}

	run := func() string {
		matched, unmatched, orphaned := MatchNodes(changes, nil)
		actions := ClassifyActions(matched, unmatched, orphaned)
		var buf bytes.Buffer
		if err := GenerateReport(actions, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	r1 := run()
	r2 := run()
	if r1 != r2 {
		t.Fatalf("non-deterministic output:\nrun1: %s\nrun2: %s", r1, r2)
	}
}

func Test_resolveNode(t *testing.T) {
	tests := []struct {
		path     string
		wantName string
		wantType string
	}{
		{"project/validator/arch/arch_schema_checker.md", "SchemaChecker", "component"},
		{"project/merkle/impl/impl_hash_algo.md", "HashAlgo", "impl_section"},
		{"project/merkle/flow/flow_tree_build.md", "TreeBuild", "flow"},
		{"project/validator/module.json", "module.json", "structural"},
		{"project/project.json", "project.json", "structural"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			name, nodeType := resolveNode(tt.path)
			if name != tt.wantName {
				t.Errorf("name: want %q, got %q", tt.wantName, name)
			}
			if nodeType != tt.wantType {
				t.Errorf("type: want %q, got %q", tt.wantType, nodeType)
			}
		})
	}
}

func Test_snakeToPascal(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"schema_checker", "SchemaChecker"},
		{"hasher", "Hasher"},
		{"tree_build", "TreeBuild"},
		{"a_b_c", "ABC"},
	}
	for _, tt := range tests {
		got := snakeToPascal(tt.in)
		if got != tt.want {
			t.Errorf("snakeToPascal(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
