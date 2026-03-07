package impact

import (
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR3_ClassifyActions_ReviewOnModified(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_comp1.md", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Beads: []BeadSpec{{ID: "bead-1", Module: "alpha", Component: "Comp1"}},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Type != "review" {
		t.Errorf("want type review, got %q", a.Type)
	}
	if a.BeadID != "bead-1" {
		t.Errorf("want bead ID bead-1, got %q", a.BeadID)
	}
	if a.Impact != "arch_impl" {
		t.Errorf("want impact arch_impl, got %q", a.Impact)
	}
	if !strings.Contains(a.Reason, "modified") {
		t.Errorf("want reason containing 'modified', got %q", a.Reason)
	}
}

func TestFR3_ClassifyActions_ReviewOnAddedWithBead(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_comp1.md", Type: merkle.Added, NewHash: "aaa"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Beads: []BeadSpec{{ID: "bead-1", Module: "alpha", Component: "Comp1"}},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Type != "review" {
		t.Errorf("want type review, got %q", actions[0].Type)
	}
	if !strings.Contains(actions[0].Reason, "added node") {
		t.Errorf("want reason containing 'added node', got %q", actions[0].Reason)
	}
}

func TestFR3_ClassifyActions_CreateOnAddedUnmatched(t *testing.T) {
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_new.md", Type: merkle.Added, NewHash: "aaa"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}

	actions := ClassifyActions(nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Type != "create" {
		t.Errorf("want type create, got %q", a.Type)
	}
	if a.BeadID != "" {
		t.Errorf("want empty bead ID, got %q", a.BeadID)
	}
	if !strings.Contains(a.Reason, "New spec node") {
		t.Errorf("want reason containing 'New spec node', got %q", a.Reason)
	}
}

func TestFR3_ClassifyActions_CreateOnModifiedUnmatched(t *testing.T) {
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/impl/impl_data.md", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
				Impact: merkle.ImplOnly,
				Module: "alpha",
			},
		},
	}

	actions := ClassifyActions(nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Type != "create" {
		t.Errorf("want type create, got %q", actions[0].Type)
	}
}

func TestFR3_ClassifyActions_CloseOnOrphaned(t *testing.T) {
	orphaned := []Orphaned{
		{Bead: BeadSpec{ID: "bead-old", Module: "alpha", Component: "OldComp"}},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	a := actions[0]
	if a.Type != "close" {
		t.Errorf("want type close, got %q", a.Type)
	}
	if a.BeadID != "bead-old" {
		t.Errorf("want bead ID bead-old, got %q", a.BeadID)
	}
	if !strings.Contains(a.Reason, "removed") {
		t.Errorf("want reason containing 'removed', got %q", a.Reason)
	}
}

func TestFR3_ClassifyActions_NoActionOnRemovedUnmatched(t *testing.T) {
	// Removed nodes without beads should produce no action.
	// NodeMatcher filters these out (doesn't include them in unmatched),
	// but if somehow passed, ClassifyActions should still produce nothing.
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_gone.md", Type: merkle.Removed, OldHash: "aaa"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}

	actions := ClassifyActions(nil, unmatched, nil)

	if len(actions) != 0 {
		t.Fatalf("want 0 actions for removed+unmatched, got %d", len(actions))
	}
}

func TestFR3_ClassifyActions_MultipleBeadsPerMatch(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_comp1.md", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Beads: []BeadSpec{
				{ID: "bead-a", Module: "alpha", Component: "Comp1"},
				{ID: "bead-b", Module: "alpha", Component: "Comp1"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (one per bead), got %d", len(actions))
	}
	if actions[0].BeadID != "bead-a" || actions[1].BeadID != "bead-b" {
		t.Errorf("want bead IDs [bead-a, bead-b], got [%s, %s]", actions[0].BeadID, actions[1].BeadID)
	}
}

func TestNFR5_ClassifyActions_DeterministicSort(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/beta/arch/arch_b.md", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ArchImpl,
				Module: "beta",
			},
			Beads: []BeadSpec{{ID: "bead-2", Module: "beta", Component: "B"}},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/arch/arch_new.md", Type: merkle.Added, NewHash: "x"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}
	orphaned := []Orphaned{
		{Bead: BeadSpec{ID: "bead-old", Module: "alpha", Component: "Gone"}},
	}

	actions := ClassifyActions(matches, unmatched, orphaned)

	if len(actions) != 3 {
		t.Fatalf("want 3 actions, got %d", len(actions))
	}

	// Sorted by type: close < create < review
	wantTypes := []string{"close", "create", "review"}
	for i, wt := range wantTypes {
		if actions[i].Type != wt {
			t.Errorf("actions[%d].Type = %q, want %q", i, actions[i].Type, wt)
		}
	}
}

func TestFR3_ClassifyActions_EmptyInputs(t *testing.T) {
	actions := ClassifyActions(nil, nil, nil)
	if actions != nil {
		t.Errorf("want nil for empty inputs, got %v", actions)
	}
}

func TestFR3_ClassifyActions_ImplOnlyImpact(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "proj/alpha/impl/impl_data.md", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ImplOnly,
				Module: "alpha",
			},
			Beads: []BeadSpec{{ID: "bead-1", Module: "alpha", ImplSection: "Data processing"}},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Impact != "impl_only" {
		t.Errorf("want impact impl_only, got %q", actions[0].Impact)
	}
}

func TestFR3_ClassifyActions_OrphanedUsesImplSection(t *testing.T) {
	orphaned := []Orphaned{
		{Bead: BeadSpec{ID: "bead-1", Module: "alpha", ImplSection: "Data flow"}},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "Data flow" {
		t.Errorf("want node 'Data flow', got %q", actions[0].Node)
	}
}
