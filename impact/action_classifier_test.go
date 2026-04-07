package impact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

// --- S1: ActionClassifier produces correct actions for each category ---

func TestFR3_S1_ClassifyActions_FullScenario(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: "module/2/component/1", BeadID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
			},
		},
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/3/impl_section/1", Type: merkle.Modified, OldHash: "ddd", NewHash: "eee", NodeType: "impl_section"},
				Impact: merkle.ImplOnly,
				Module: "merkle",
			},
			Records: []mapping.Record{
				{ID: 3, SpecNodeID: "module/3/impl_section/1", BeadID: "spex-003", Module: "merkle", Component: "Hash computation", SpecHash: "ghi789"},
			},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/2/component/4", Type: merkle.Added, NewHash: "fff", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
		},
	}
	orphaned := []Orphaned{
		{
			Record: mapping.Record{ID: 10, SpecNodeID: "module/3/component/99", BeadID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000"},
		},
	}

	actions := ClassifyActions(matches, unmatched, orphaned)

	// Expect 6 actions: 2 obsolete+create pairs for modified, 1 create for added, 1 obsolete for orphaned
	if len(actions) != 6 {
		t.Fatalf("want 6 actions, got %d: %+v", len(actions), actions)
	}

	// Check action types: should be sorted (create < obsolete)
	var creates, obsoletes []Action
	for _, a := range actions {
		switch a.Type {
		case "create":
			creates = append(creates, a)
		case "obsolete":
			obsoletes = append(obsoletes, a)
		default:
			t.Errorf("unexpected action type %q", a.Type)
		}
	}

	if len(creates) != 3 {
		t.Errorf("want 3 creates, got %d", len(creates))
	}
	if len(obsoletes) != 3 {
		t.Errorf("want 3 obsoletes, got %d", len(obsoletes))
	}

	// Check specific actions exist
	assertHasAction(t, actions, "obsolete", "spex-001", "validator", "SchemaChecker", "Spec node modified: validator/SchemaChecker")
	assertHasAction(t, actions, "create", "", "validator", "SchemaChecker", "Spec node modified (new): validator/SchemaChecker")
	assertHasAction(t, actions, "obsolete", "spex-003", "merkle", "Hash computation", "Spec node modified: merkle/Hash computation")
	assertHasAction(t, actions, "create", "", "merkle", "Hash computation", "Spec node modified (new): merkle/Hash computation")
	assertHasAction(t, actions, "create", "", "validator", "module/2/component/4", "New spec node: validator/module/2/component/4")
	assertHasAction(t, actions, "obsolete", "spex-010", "merkle", "LegacyHasher", "Spec node removed: merkle/LegacyHasher")
}

// --- S2: Modified node without a matching bead ---

func TestFR3_S2_ClassifyActions_ModifiedUnmatched(t *testing.T) {
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/7/component/1", Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "render",
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
	if a.SpecHash != "bbb" {
		t.Errorf("want spec hash bbb, got %q", a.SpecHash)
	}
}

// --- S3: Added node with an existing bead (unexpected case) ---

func TestFR3_S3_ClassifyActions_AddedWithExistingBead(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/6/component/1", Type: merkle.Added, NewHash: "new111", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "proposal",
			},
			Records: []mapping.Record{
				{ID: 20, SpecNodeID: "module/6/component/1", BeadID: "spex-020", Module: "proposal", Component: "Registrar", SpecHash: "old111"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (obsolete + create), got %d", len(actions))
	}

	assertHasAction(t, actions, "obsolete", "spex-020", "proposal", "Registrar", "Spec node modified: proposal/Registrar")
	found := false
	for _, a := range actions {
		if a.Type == "create" && a.OldBeadID == "spex-020" {
			found = true
		}
	}
	if !found {
		t.Error("create action should have OldBeadID = spex-020")
	}
}

// --- S4: Removed node without a matching bead ---

func TestFR3_S4_ClassifyActions_RemovedNoRecord(t *testing.T) {
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/99", Type: merkle.Removed, OldHash: "aaa", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "schema",
			},
		},
	}

	actions := ClassifyActions(nil, unmatched, nil)

	if len(actions) != 0 {
		t.Fatalf("want 0 actions for removed+unmatched, got %d", len(actions))
	}
}

// --- S5: Multiple beads per matched node ---

func TestFR3_S5_ClassifyActions_MultipleBeadsPerNode(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: "module/2/component/1", BeadID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
				{ID: 5, SpecNodeID: "module/2/component/1", BeadID: "spex-005", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	// Two obsoletes (one per old bead) + two creates (new replacements)
	if len(actions) != 4 {
		t.Fatalf("want 4 actions, got %d: %+v", len(actions), actions)
	}

	var obsoletes, creates []Action
	for _, a := range actions {
		switch a.Type {
		case "obsolete":
			obsoletes = append(obsoletes, a)
		case "create":
			creates = append(creates, a)
		}
	}
	if len(obsoletes) != 2 {
		t.Errorf("want 2 obsoletes, got %d", len(obsoletes))
	}
	if len(creates) != 2 {
		t.Errorf("want 2 creates, got %d", len(creates))
	}
}

// --- S5b: Removed node with closed bead (cleanup) ---

func TestFR6_S5b_ClassifyActions_RemovedClosedBead(t *testing.T) {
	orphaned := []Orphaned{
		{
			Record: mapping.Record{ID: 10, SpecNodeID: "module/3/component/99", BeadID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000", BeadStatus: "closed"},
		},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (obsolete + cleanup create), got %d: %+v", len(actions), actions)
	}

	assertHasAction(t, actions, "obsolete", "spex-010", "merkle", "LegacyHasher", "Spec node removed: merkle/LegacyHasher")
	assertHasAction(t, actions, "create", "", "merkle", "LegacyHasher", "Code cleanup: merkle/LegacyHasher")
}

// --- S5c: Removed node with open bead (no cleanup) ---

func TestFR6_S5c_ClassifyActions_RemovedOpenBead(t *testing.T) {
	orphaned := []Orphaned{
		{
			Record: mapping.Record{ID: 11, SpecNodeID: "module/3/component/98", BeadID: "spex-011", Module: "merkle", Component: "DraftHasher", SpecHash: "yyy000", BeadStatus: "open"},
		},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action (obsolete only), got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != "obsolete" {
		t.Errorf("want obsolete, got %q", actions[0].Type)
	}
	if actions[0].BeadID != "spex-011" {
		t.Errorf("want bead spex-011, got %q", actions[0].BeadID)
	}
}

// --- S6: Impact level does not change action type ---

func TestFR3_S6_ClassifyActions_ImpactLevelDoesNotChangeType(t *testing.T) {
	levels := []merkle.ImpactLevel{merkle.ImplOnly, merkle.ArchImpl, merkle.Structural}
	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			matches := []Match{
				{
					Change: merkle.ClassifiedChange{
						Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
						Impact: level,
						Module: "alpha",
					},
					Records: []mapping.Record{
						{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp1"},
					},
				},
			}

			actions := ClassifyActions(matches, nil, nil)

			if len(actions) != 2 {
				t.Fatalf("want 2 actions (obsolete + create), got %d", len(actions))
			}
			var hasObsolete, hasCreate bool
			for _, a := range actions {
				if a.Type == "obsolete" {
					hasObsolete = true
				}
				if a.Type == "create" {
					hasCreate = true
				}
			}
			if !hasObsolete || !hasCreate {
				t.Error("want both obsolete and create regardless of impact level")
			}
		})
	}
}

// --- Non-bead node types are filtered ---

func TestFR3_ClassifyActions_NonBeadTypesFiltered(t *testing.T) {
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Path: "module/7/impl_section/1", Type: merkle.Added, NewHash: "aaa", NodeType: "impl_section"},
			Impact: merkle.ImplOnly, Module: "render",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Path: "module/7/data_flow/1", Type: merkle.Added, NewHash: "bbb", NodeType: "data_flow"},
			Impact: merkle.ImplOnly, Module: "render",
		}},
	}
	actions := ClassifyActions(nil, unmatched, nil)
	if len(actions) != 0 {
		t.Errorf("want 0 actions for non-bead types, got %d: %+v", len(actions), actions)
	}
}

// --- E1: Empty inputs produce empty result ---

func TestFR3_E1_ClassifyActions_EmptyInputs(t *testing.T) {
	actions := ClassifyActions(nil, nil, nil)
	if len(actions) != 0 {
		t.Errorf("want 0 actions for nil inputs, got %d", len(actions))
	}
}

// --- E5: Duplicate actions are preserved, not deduplicated ---

func TestFR3_E5_ClassifyActions_DuplicatesPreserved(t *testing.T) {
	change := merkle.ClassifiedChange{
		Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Added, NewHash: "aaa", NodeType: "component"},
		Impact: merkle.ArchImpl,
		Module: "alpha",
	}

	// Same change appears in both matches and unmatched
	matches := []Match{
		{
			Change:  change,
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp1"}},
		},
	}
	unmatched := []Unmatched{{Change: change}}

	actions := ClassifyActions(matches, unmatched, nil)

	// Should produce actions for both: obsolete+create for match, create for unmatched = 3 total
	if len(actions) != 3 {
		t.Fatalf("want 3 actions (no deduplication), got %d", len(actions))
	}
}

// --- NFR5: Deterministic sort ---

func TestNFR5_ClassifyActions_DeterministicSort(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/2/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "beta",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: "module/2/component/1", BeadID: "bead-2", Module: "beta", Component: "Comp2"}},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/5", Type: merkle.Added, NewHash: "x", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 2, SpecNodeID: "module/1/component/3", BeadID: "bead-old", Module: "alpha", Component: "OldComp"}},
	}

	for i := 0; i < 5; i++ {
		actions := ClassifyActions(matches, unmatched, orphaned)
		// Sorted by type: create < obsolete
		prevType := ""
		for _, a := range actions {
			if a.Type < prevType {
				t.Errorf("run %d: actions not sorted by type: got %q after %q", i, a.Type, prevType)
			}
			prevType = a.Type
		}
	}
}

// --- OldBeadID propagation ---

func TestFR3_ClassifyActions_OldBeadIDOnCreate(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-old", Module: "alpha", Component: "Comp"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	for _, a := range actions {
		if a.Type == "create" {
			if a.OldBeadID != "bead-old" {
				t.Errorf("want OldBeadID bead-old, got %q", a.OldBeadID)
			}
			if a.SpecHash != "b" {
				t.Errorf("want SpecHash b, got %q", a.SpecHash)
			}
		}
	}
}

// --- NodeType propagation ---

func TestFR3_ClassifyActions_NodeTypePropagated(t *testing.T) {
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: "module/1/component/1", Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	for _, a := range actions {
		if a.NodeType != "component" {
			t.Errorf("want NodeType component, got %q", a.NodeType)
		}
	}
}

// --- Orphaned without status defaults to simple obsolete (no cleanup) ---

func TestFR6_ClassifyActions_OrphanedNoStatusDefaultObsolete(t *testing.T) {
	orphaned := []Orphaned{
		{
			Record: mapping.Record{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp"},
		},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Type != "obsolete" {
		t.Errorf("want obsolete, got %q", actions[0].Type)
	}
}

// --- Orphaned with in_progress status: obsolete only, no cleanup ---

func TestFR6_ClassifyActions_OrphanedInProgressBead(t *testing.T) {
	orphaned := []Orphaned{
		{
			Record: mapping.Record{ID: 1, SpecNodeID: "module/1/component/1", BeadID: "bead-1", Module: "alpha", Component: "Comp", BeadStatus: "in_progress"},
		},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Type != "obsolete" {
		t.Errorf("want obsolete, got %q", actions[0].Type)
	}
}

// ========================
// Dependency Resolution Tests
// ========================

// --- D1: Component uses edge resolves to open dependency bead ---

func TestFR7_D1_ResolveDeps_UsesOpenBead(t *testing.T) {
	// Component X (id=3) uses component Y (id=2). Y has open bead spex-050.
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   4,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: 2, Name: "NodeMatcher", Uses: nil},
					{ID: 3, Name: "ActionClassifier", Uses: []int{2}},
				},
			},
		},
	}
	records := []mapping.Record{
		{ID: 50, SpecNodeID: "impact/component/2", BeadID: "spex-050", Module: "impact", Component: "NodeMatcher", BeadStatus: "open"},
		{ID: 51, SpecNodeID: "impact/component/3", BeadID: "spex-051", Module: "impact", Component: "ActionClassifier", BeadStatus: "open"},
	}

	action := Action{
		Type:     "create",
		Module:   "impact",
		Node:     "ActionClassifier",
		NodeType: "component",
	}

	deps := ResolveDeps(graph, records, action, "impact/component/3")
	assertContains(t, deps, "spex-050")
}

// --- D2: Component uses edge skips closed dependency bead ---

func TestFR7_D2_ResolveDeps_UsesClosedBead(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   4,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: 2, Name: "NodeMatcher", Uses: nil},
					{ID: 3, Name: "ActionClassifier", Uses: []int{2}},
				},
			},
		},
	}
	records := []mapping.Record{
		{ID: 51, SpecNodeID: "impact/component/2", BeadID: "spex-051", Module: "impact", Component: "NodeMatcher", BeadStatus: "closed"},
	}

	action := Action{Type: "create", Module: "impact", Node: "ActionClassifier", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "impact/component/3")

	if len(deps) != 0 {
		t.Errorf("want 0 deps (closed bead skipped), got %v", deps)
	}
}

// --- D3: requires_module resolves to all open component beads ---

func TestFR7_D3_ResolveDeps_RequiresModuleOpenBeads(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:             4,
				Name:           "impact",
				RequiresModule: []int{3},
				Components: []mapping.ComponentInfo{
					{ID: 3, Name: "ActionClassifier", Uses: nil},
				},
			},
			"merkle": {
				ID:   3,
				Name: "merkle",
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "Hasher"},
					{ID: 2, Name: "TreeBuilder"},
					{ID: 3, Name: "SnapshotStore"},
				},
			},
		},
		modulesByID: map[int]string{3: "merkle", 4: "impact"},
	}
	records := []mapping.Record{
		{ID: 60, SpecNodeID: "merkle/component/1", BeadID: "spex-060", Module: "merkle", BeadStatus: "open"},
		{ID: 61, SpecNodeID: "merkle/component/2", BeadID: "spex-061", Module: "merkle", BeadStatus: "closed"},
		{ID: 62, SpecNodeID: "merkle/component/3", BeadID: "spex-062", Module: "merkle", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "impact", Node: "ActionClassifier", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "impact/component/3")

	assertContains(t, deps, "spex-060")
	assertContains(t, deps, "spex-062")
	assertNotContains(t, deps, "spex-061")
}

// --- D4: Transitive requires_module resolution ---

func TestFR7_D4_ResolveDeps_TransitiveRequiresModule(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: 1, Name: "modA", RequiresModule: []int{2}, Components: []mapping.ComponentInfo{{ID: 1, Name: "CompA"}}},
			"modB": {ID: 2, Name: "modB", RequiresModule: []int{3}, Components: []mapping.ComponentInfo{{ID: 1, Name: "CompB"}}},
			"modC": {ID: 3, Name: "modC", Components: []mapping.ComponentInfo{{ID: 1, Name: "CompC"}}},
		},
		modulesByID: map[int]string{1: "modA", 2: "modB", 3: "modC"},
	}
	records := []mapping.Record{
		{ID: 70, SpecNodeID: "modB/component/1", BeadID: "spex-070", Module: "modB", BeadStatus: "open"},
		{ID: 71, SpecNodeID: "modC/component/1", BeadID: "spex-071", Module: "modC", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "CompA", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "modA/component/1")

	assertContains(t, deps, "spex-070")
	assertContains(t, deps, "spex-071")
}

// --- D5: Component uses edges are NOT transitive ---

func TestFR7_D5_ResolveDeps_UsesNotTransitive(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   1,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "X", Uses: []int{2}},
					{ID: 2, Name: "Y", Uses: []int{3}},
					{ID: 3, Name: "Z"},
				},
			},
		},
	}
	records := []mapping.Record{
		{ID: 80, SpecNodeID: "mod/component/2", BeadID: "spex-080", Module: "mod", BeadStatus: "open"},
		{ID: 81, SpecNodeID: "mod/component/3", BeadID: "spex-081", Module: "mod", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "mod/component/1")

	assertContains(t, deps, "spex-080")
	assertNotContains(t, deps, "spex-081")
}

// --- D6: Mixed uses and requires_module dependencies ---

func TestFR7_D6_ResolveDeps_MixedUsesAndRequiresModule(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {
				ID:             1,
				Name:           "modA",
				RequiresModule: []int{2},
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "X", Uses: []int{2}},
					{ID: 2, Name: "Y"},
				},
			},
			"modB": {
				ID:   2,
				Name: "modB",
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "CompB"},
				},
			},
		},
		modulesByID: map[int]string{1: "modA", 2: "modB"},
	}
	records := []mapping.Record{
		{ID: 90, SpecNodeID: "modA/component/2", BeadID: "spex-090", Module: "modA", BeadStatus: "open"},
		{ID: 91, SpecNodeID: "modB/component/1", BeadID: "spex-091", Module: "modB", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "X", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "modA/component/1")

	assertContains(t, deps, "spex-090")
	assertContains(t, deps, "spex-091")
}

// --- D7: No dependencies when all beads are closed ---

func TestFR7_D7_ResolveDeps_AllClosed(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:             1,
				Name:           "mod",
				RequiresModule: []int{2},
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "X", Uses: []int{2}},
					{ID: 2, Name: "Y"},
				},
			},
			"modB": {ID: 2, Name: "modB", Components: []mapping.ComponentInfo{{ID: 1, Name: "CompB"}}},
		},
		modulesByID: map[int]string{1: "mod", 2: "modB"},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "mod/component/2", BeadID: "b1", Module: "mod", BeadStatus: "closed"},
		{ID: 2, SpecNodeID: "modB/component/1", BeadID: "b2", Module: "modB", BeadStatus: "closed"},
	}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "mod/component/1")

	if len(deps) != 0 {
		t.Errorf("want 0 deps (all closed), got %v", deps)
	}
}

// --- D8: No dependencies for nodes without uses or requires_module ---

func TestFR7_D8_ResolveDeps_NoDeps(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   1,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "Standalone"},
				},
			},
		},
	}

	action := Action{Type: "create", Module: "mod", Node: "Standalone", NodeType: "component"}
	deps := ResolveDeps(graph, nil, action, "mod/component/1")

	if len(deps) != 0 {
		t.Errorf("want 0 deps, got %v", deps)
	}
}

// --- D9: Cycle detection in requires_module ---

func TestFR7_D9_ResolveDeps_CycleDetection(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: 1, Name: "modA", RequiresModule: []int{2}, Components: []mapping.ComponentInfo{{ID: 1, Name: "CompA"}}},
			"modB": {ID: 2, Name: "modB", RequiresModule: []int{1}, Components: []mapping.ComponentInfo{{ID: 1, Name: "CompB"}}},
		},
		modulesByID: map[int]string{1: "modA", 2: "modB"},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: "modA/component/1", BeadID: "b-a", Module: "modA", BeadStatus: "open"},
		{ID: 2, SpecNodeID: "modB/component/1", BeadID: "b-b", Module: "modB", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "CompA", NodeType: "component"}

	// Should not infinite loop — must terminate
	deps := ResolveDeps(graph, records, action, "modA/component/1")
	// Should collect open beads from modB (reachable) but not re-collect from modA (cycle)
	assertContains(t, deps, "b-b")
}

// --- D12: Deps with beads created in same apply run ---

func TestFR7_D12_ResolveDeps_NoBeadForNewComponent(t *testing.T) {
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   1,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: 1, Name: "X", Uses: []int{2}},
					{ID: 2, Name: "Y"},
				},
			},
		},
	}
	// No records for Y — it's being created in the same run
	records := []mapping.Record{}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component"}
	deps := ResolveDeps(graph, records, action, "mod/component/1")

	if len(deps) != 0 {
		t.Errorf("want 0 deps (Y has no bead yet), got %v", deps)
	}
}

// ========================
// Test Helpers
// ========================

func assertHasAction(t *testing.T, actions []Action, typ, beadID, module, node, reason string) {
	t.Helper()
	for _, a := range actions {
		if a.Type == typ && a.BeadID == beadID && a.Module == module && a.Node == node {
			if !strings.Contains(a.Reason, reason) {
				t.Errorf("action (%s, %s, %s, %s) reason = %q, want containing %q", typ, beadID, module, node, a.Reason, reason)
			}
			return
		}
	}
	t.Errorf("want action (type=%s, beadID=%s, module=%s, node=%s), not found in %+v", typ, beadID, module, node, actions)
}

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("want %q in %v", want, slice)
}

func assertNotContains(t *testing.T, slice []string, unwanted string) {
	t.Helper()
	for _, s := range slice {
		if s == unwanted {
			t.Errorf("do not want %q in %v", unwanted, slice)
			return
		}
	}
}

// stubSpecGraph implements a minimal SpecGraph for testing dependency resolution.
type stubSpecGraph struct {
	modules     map[string]mapping.ModuleInfo
	modulesByID map[int]string
}

func (s *stubSpecGraph) ModuleByName(name string) (mapping.ModuleInfo, error) {
	m, ok := s.modules[name]
	if !ok {
		return mapping.ModuleInfo{}, fmt.Errorf("module %q not found", name)
	}
	return m, nil
}

func (s *stubSpecGraph) ModuleByID(id int) (mapping.ModuleInfo, error) {
	name, ok := s.modulesByID[id]
	if !ok {
		return mapping.ModuleInfo{}, fmt.Errorf("module id %d not found", id)
	}
	return s.ModuleByName(name)
}

func (s *stubSpecGraph) NodeHash(specNodeID string) (string, error) {
	return "", fmt.Errorf("not implemented in stub")
}
