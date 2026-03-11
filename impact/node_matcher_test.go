package impact

import (
	"sort"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR2_MatchComponentChangeToComponentBead(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "BeadReader"},
	}

	matches, unmatched, orphaned := MatchNodes(changes, beads, modules)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].Change.Path != changes[0].Path {
		t.Errorf("want change path %s, got %s", changes[0].Path, matches[0].Change.Path)
	}
	if len(matches[0].Beads) != 1 || matches[0].Beads[0].ID != "b1" {
		t.Errorf("want bead b1, got %+v", matches[0].Beads)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned, got %d", len(orphaned))
	}
}

func TestFR2_MatchImplChangeViaNodeMap(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/impl_section/1", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", ImplSection: "Bead metadata reading"},
	}
	modules := map[string]NodeMap{
		"1": {"impl_section/1": "Bead metadata reading"},
	}

	matches, unmatched, _ := MatchNodes(changes, beads, modules)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].Beads[0].ID != "b1" {
		t.Errorf("want bead b1, got %s", matches[0].Beads[0].ID)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
}

func TestFR2_UnmatchedAddedChange(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/5", Type: merkle.Added},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "BeadReader"},
	}

	_, unmatched, _ := MatchNodes(changes, beads, modules)

	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched, got %d", len(unmatched))
	}
	if unmatched[0].Change.Path != changes[0].Path {
		t.Errorf("want path %s, got %s", changes[0].Path, unmatched[0].Change.Path)
	}
}

func TestFR2_OrphanedBeadFromRemovedChange(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Removed, OldHash: "aaa"},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "BeadReader"},
	}

	matches, _, orphaned := MatchNodes(changes, beads, modules)

	if len(matches) != 0 {
		t.Errorf("want 0 matches for removed change, got %d", len(matches))
	}
	if len(orphaned) != 1 {
		t.Fatalf("want 1 orphaned, got %d", len(orphaned))
	}
	if orphaned[0].Bead.ID != "b1" {
		t.Errorf("want orphaned bead b1, got %s", orphaned[0].Bead.ID)
	}
}

func TestFR2_StructuralChangeMatchesAllModuleBeads(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/meta", Type: merkle.Modified},
			Impact: merkle.Structural,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
		{ID: "b2", Module: "1", Component: "NodeMatcher"},
		{ID: "b3", Module: "2", Component: "Hasher"},
	}

	matches, _, _ := MatchNodes(changes, beads, nil)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Beads) != 2 {
		t.Fatalf("want 2 beads matched, got %d", len(matches[0].Beads))
	}
	ids := []string{matches[0].Beads[0].ID, matches[0].Beads[1].ID}
	sort.Strings(ids)
	if ids[0] != "b1" || ids[1] != "b2" {
		t.Errorf("want beads b1 and b2, got %v", ids)
	}
}

func TestFR2_ProjectMetaMatchesAllBeads(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/meta", Type: merkle.Modified},
			Impact: merkle.Structural,
			Module: "",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
		{ID: "b2", Module: "2", Component: "Hasher"},
	}

	matches, _, _ := MatchNodes(changes, beads, nil)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Beads) != 2 {
		t.Fatalf("want 2 beads, got %d", len(matches[0].Beads))
	}
}

func TestFR2_MultipleBeadsPerNode(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "impl-bead", Module: "1", Component: "BeadReader"},
		{ID: "review-bead", Module: "1", Component: "BeadReader"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "BeadReader"},
	}

	matches, _, _ := MatchNodes(changes, beads, modules)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Beads) != 2 {
		t.Fatalf("want 2 beads, got %d", len(matches[0].Beads))
	}
}

func TestFR2_RemovedChangeNoBeadIsIgnored(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/99", Type: merkle.Removed},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}

	matches, unmatched, orphaned := MatchNodes(changes, nil, nil)

	if len(matches) != 0 {
		t.Errorf("want 0 matches, got %d", len(matches))
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched (removed with no bead), got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (no bead references this), got %d", len(orphaned))
	}
}

func TestFR2_OrphanNotCreatedIfBeadAlsoMatchesNonRemoved(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Removed},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "BeadReader"},
	}

	matches, _, orphaned := MatchNodes(changes, beads, modules)

	if len(matches) != 1 {
		t.Errorf("want 1 match, got %d", len(matches))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (bead also matched non-removed), got %d", len(orphaned))
	}
}

func TestNFR5_DeterministicOutput(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/2", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "z-bead", Module: "1", Component: "BeadReader"},
		{ID: "a-bead", Module: "1", Component: "NodeMatcher"},
	}
	modules := map[string]NodeMap{
		"1": {
			"component/1": "BeadReader",
			"component/2": "NodeMatcher",
		},
	}

	// Run multiple times to verify determinism.
	for i := 0; i < 5; i++ {
		matches, _, _ := MatchNodes(changes, beads, modules)

		if len(matches) != 2 {
			t.Fatalf("run %d: want 2 matches, got %d", i, len(matches))
		}
		// Changes order is preserved from input.
		if matches[0].Change.Path != changes[0].Path {
			t.Errorf("run %d: match order not preserved", i)
		}
		// Beads within each match are sorted by ID.
		if matches[0].Beads[0].ID != "a-bead" {
			t.Errorf("run %d: want first bead a-bead, got %s", i, matches[0].Beads[0].ID)
		}
		if matches[1].Beads[0].ID != "z-bead" {
			t.Errorf("run %d: want first bead z-bead, got %s", i, matches[1].Beads[0].ID)
		}
	}
}

func TestNFR5_EmptyInputs(t *testing.T) {
	matches, unmatched, orphaned := MatchNodes(nil, nil, nil)
	if len(matches) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for nil inputs, got %d matches, %d unmatched, %d orphaned",
			len(matches), len(unmatched), len(orphaned))
	}

	matches, unmatched, orphaned = MatchNodes([]merkle.ClassifiedChange{}, []BeadSpec{}, nil)
	if len(matches) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for empty inputs, got %d matches, %d unmatched, %d orphaned",
			len(matches), len(unmatched), len(orphaned))
	}
}

func TestFR2_SnakeToPascal(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bead_reader", "BeadReader"},
		{"node_matcher", "NodeMatcher"},
		{"hasher", "Hasher"},
		{"impact_command", "ImpactCommand"},
		{"a_b_c", "ABC"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := snakeToPascal(tt.input)
			if got != tt.want {
				t.Errorf("snakeToPascal(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFR2_NodeMapResolutionOverridesAutoDerive(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "1",
		},
	}
	// NodeMap resolves ID to a name.
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "CustomName"},
	}
	modules := map[string]NodeMap{
		"1": {"component/1": "CustomName"},
	}

	matches, _, _ := MatchNodes(changes, beads, modules)

	if len(matches) != 1 {
		t.Fatalf("want 1 match via NodeMap, got %d", len(matches))
	}
	if matches[0].Beads[0].ID != "b1" {
		t.Errorf("want bead b1, got %s", matches[0].Beads[0].ID)
	}
}

func TestFR2_DataFlowChangeWithoutNodeMapIsUnmatched(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/data_flow/1", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "1",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "BeadReader"},
	}

	_, unmatched, _ := MatchNodes(changes, beads, nil)

	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched for data_flow change without NodeMap, got %d", len(unmatched))
	}
}

func TestFR2_CrossModuleNoMatch(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "2",
		},
	}
	beads := []BeadSpec{
		{ID: "b1", Module: "1", Component: "Hasher"},
	}
	modules := map[string]NodeMap{
		"2": {"component/1": "Hasher"},
	}

	_, unmatched, _ := MatchNodes(changes, beads, modules)

	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched (different module), got %d", len(unmatched))
	}
}
