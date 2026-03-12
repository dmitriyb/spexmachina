package impact

import (
	"sort"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

func TestFR2_MatchComponentChangeToRecord(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb"},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact", Component: "BeadReader"},
	}

	matches, unmatched, orphaned := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].Change.Path != changes[0].Path {
		t.Errorf("want change path %s, got %s", changes[0].Path, matches[0].Change.Path)
	}
	if len(matches[0].Records) != 1 || matches[0].Records[0].BeadID != "b1" {
		t.Errorf("want record with bead b1, got %+v", matches[0].Records)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned, got %d", len(orphaned))
	}
}

func TestFR2_MatchImplSectionChangeToRecord(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/impl_section/1", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/impl_section/1", BeadID: "b1", Module: "impact"},
	}

	matches, unmatched, _ := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if matches[0].Records[0].BeadID != "b1" {
		t.Errorf("want bead b1, got %s", matches[0].Records[0].BeadID)
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
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
	}

	_, unmatched, _ := MatchNodes(changes, records)

	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched, got %d", len(unmatched))
	}
	if unmatched[0].Change.Path != changes[0].Path {
		t.Errorf("want path %s, got %s", changes[0].Path, unmatched[0].Change.Path)
	}
}

func TestFR2_OrphanedRecordFromRemovedChange(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Removed, OldHash: "aaa"},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
	}

	matches, _, orphaned := MatchNodes(changes, records)

	if len(matches) != 0 {
		t.Errorf("want 0 matches for removed change, got %d", len(matches))
	}
	if len(orphaned) != 1 {
		t.Fatalf("want 1 orphaned, got %d", len(orphaned))
	}
	if orphaned[0].Record.BeadID != "b1" {
		t.Errorf("want orphaned record with bead b1, got %s", orphaned[0].Record.BeadID)
	}
}

func TestFR2_StructuralChangeMatchesAllModuleRecords(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/meta", Type: merkle.Modified},
			Impact: merkle.Structural,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
		{ID: 2, SpecNodeID: "module/1/component/2", BeadID: "b2", Module: "impact"},
		{ID: 3, SpecNodeID: "module/2/component/1", BeadID: "b3", Module: "merkle"},
	}

	matches, _, _ := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Records) != 2 {
		t.Fatalf("want 2 records matched, got %d", len(matches[0].Records))
	}
	ids := []string{matches[0].Records[0].BeadID, matches[0].Records[1].BeadID}
	sort.Strings(ids)
	if ids[0] != "b1" || ids[1] != "b2" {
		t.Errorf("want beads b1 and b2, got %v", ids)
	}
}

func TestFR2_ProjectMetaMatchesAllRecords(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "project/meta", Type: merkle.Modified},
			Impact: merkle.Structural,
			Module: "",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
		{ID: 2, SpecNodeID: "module/2/component/1", BeadID: "b2", Module: "merkle"},
	}

	matches, _, _ := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(matches[0].Records))
	}
}

func TestFR2_MultipleRecordsPerNode(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "impl-bead", Module: "impact"},
		{ID: 2, SpecNodeID: "module/1/component/1", BeadID: "review-bead", Module: "impact"},
	}

	matches, _, _ := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match, got %d", len(matches))
	}
	if len(matches[0].Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(matches[0].Records))
	}
}

func TestFR2_RemovedChangeNoRecordIsIgnored(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/99", Type: merkle.Removed},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}

	matches, unmatched, orphaned := MatchNodes(changes, nil)

	if len(matches) != 0 {
		t.Errorf("want 0 matches, got %d", len(matches))
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched (removed with no record), got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (no record references this), got %d", len(orphaned))
	}
}

func TestFR2_OrphanNotCreatedIfRecordAlsoMatchesNonRemoved(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Removed},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
	}

	matches, _, orphaned := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Errorf("want 1 match, got %d", len(matches))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (record also matched non-removed), got %d", len(orphaned))
	}
}

func TestNFR5_DeterministicOutput(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/component/2", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
		{
			Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "z-bead", Module: "impact"},
		{ID: 2, SpecNodeID: "module/1/component/2", BeadID: "a-bead", Module: "impact"},
	}

	// Run multiple times to verify determinism.
	for i := 0; i < 5; i++ {
		matches, _, _ := MatchNodes(changes, records)

		if len(matches) != 2 {
			t.Fatalf("run %d: want 2 matches, got %d", i, len(matches))
		}
		// Changes order is preserved from input.
		if matches[0].Change.Path != changes[0].Path {
			t.Errorf("run %d: match order not preserved", i)
		}
		// Records within each match are sorted by bead ID.
		if matches[0].Records[0].BeadID != "a-bead" {
			t.Errorf("run %d: want first record a-bead, got %s", i, matches[0].Records[0].BeadID)
		}
		if matches[1].Records[0].BeadID != "z-bead" {
			t.Errorf("run %d: want first record z-bead, got %s", i, matches[1].Records[0].BeadID)
		}
	}
}

func TestNFR5_EmptyInputs(t *testing.T) {
	matches, unmatched, orphaned := MatchNodes(nil, nil)
	if len(matches) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for nil inputs, got %d matches, %d unmatched, %d orphaned",
			len(matches), len(unmatched), len(orphaned))
	}

	matches, unmatched, orphaned = MatchNodes([]merkle.ClassifiedChange{}, []mapping.Record{})
	if len(matches) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for empty inputs, got %d matches, %d unmatched, %d orphaned",
			len(matches), len(unmatched), len(orphaned))
	}
}

func TestFR2_CrossModuleNoMatch(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified},
			Impact: merkle.ArchImpl,
			Module: "merkle",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "b1", Module: "impact"},
	}

	_, unmatched, _ := MatchNodes(changes, records)

	if len(unmatched) != 1 {
		t.Fatalf("want 1 unmatched (different spec node ID), got %d", len(unmatched))
	}
}

func TestFR2_DataFlowChangeMatchesBySpecNodeID(t *testing.T) {
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Path: "module/1/data_flow/1", Type: merkle.Modified},
			Impact: merkle.ImplOnly,
			Module: "impact",
		},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "module/1/data_flow/1", BeadID: "b1", Module: "impact"},
	}

	matches, unmatched, _ := MatchNodes(changes, records)

	if len(matches) != 1 {
		t.Fatalf("want 1 match for data_flow, got %d", len(matches))
	}
	if matches[0].Records[0].BeadID != "b1" {
		t.Errorf("want bead b1, got %s", matches[0].Records[0].BeadID)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
}
