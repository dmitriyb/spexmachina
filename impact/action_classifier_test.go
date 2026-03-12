package impact

import (
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR3_ClassifyActions_ReviewOnModified(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha"}},
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

func TestFR3_ClassifyActions_ReviewOnAddedWithRecord(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Added, NewHash: "aaa"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha"}},
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
				Change: merkle.Change{Path: "module/1/component/5", Type: merkle.Added, NewHash: "aaa"},
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
				Change: merkle.Change{Path: "module/1/impl_section/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
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
		{Record: mapping.Record{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-old", Module: "alpha"}},
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
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Removed, OldHash: "aaa"},
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

func TestFR3_ClassifyActions_MultipleRecordsPerMatch(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-a", Module: "alpha"},
				{ID: 2, SpecNodeID: "module/1/component/1", BeadID: "bead-b", Module: "alpha"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (one per record), got %d", len(actions))
	}
	if actions[0].BeadID != "bead-a" || actions[1].BeadID != "bead-b" {
		t.Errorf("want bead IDs [bead-a, bead-b], got [%s, %s]", actions[0].BeadID, actions[1].BeadID)
	}
}

func TestNFR5_ClassifyActions_DeterministicSort(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ArchImpl,
				Module: "beta",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/2/component/1", BeadID: "bead-2", Module: "beta"}},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/5", Type: merkle.Added, NewHash: "x"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 2, SpecNodeID: "module/1/component/3", BeadID: "bead-old", Module: "alpha"}},
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
				Change: merkle.Change{Path: "module/1/impl_section/1", Type: merkle.Modified, OldHash: "a", NewHash: "b"},
				Impact: merkle.ImplOnly,
				Module: "alpha",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/1/impl_section/1", BeadID: "bead-1", Module: "alpha"}},
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

func TestFR3_ClassifyActions_OrphanedUsesSpecNodeID(t *testing.T) {
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 1, SpecNodeID: "module/1/impl_section/2", BeadID: "bead-1", Module: "alpha"}},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != "module/1/impl_section/2" {
		t.Errorf("want node 'module/1/impl_section/2', got %q", actions[0].Node)
	}
}
