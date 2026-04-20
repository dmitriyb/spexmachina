package impact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// depFixture holds identity hashes used by the dependency-resolution scenarios
// below. Computing them via schema.IdentityHash keeps the test data honest —
// the same derivation used by production — while letting fixtures refer to
// symbolic names (NM, AC, MOD_IMPACT, ...) rather than raw hex.
type depFixture struct {
	// Components in module "impact".
	NM string // NodeMatcher
	AC string // ActionClassifier
	// Components in module "merkle".
	HASH   string // Hasher
	TREE   string // TreeBuilder
	SNAP   string // SnapshotStore
	LEGACY string // LegacyHasher
	// Module identity hashes.
	ModImpact string
	ModMerkle string
	ModA      string
	ModB      string
	ModC      string
	// Generic components used in synthetic modA/modB/modC scenarios.
	X, Y, Z string
	CompA   string
	CompB   string
	CompC   string
	// Matched-scenario components.
	SCHK string // SchemaChecker (module validator)
	HCMP string // Hash computation (module merkle, impl_section)
	NEW  string // new added component without a record
	REG  string // proposal/Registrar
}

func newDepFixture() depFixture {
	return depFixture{
		NM:     schema.IdentityHash("impact", "component", "NodeMatcher"),
		AC:     schema.IdentityHash("impact", "component", "ActionClassifier"),
		HASH:   schema.IdentityHash("merkle", "component", "Hasher"),
		TREE:   schema.IdentityHash("merkle", "component", "TreeBuilder"),
		SNAP:   schema.IdentityHash("merkle", "component", "SnapshotStore"),
		LEGACY: schema.IdentityHash("merkle", "component", "LegacyHasher"),

		ModImpact: schema.IdentityHash("module", "impact"),
		ModMerkle: schema.IdentityHash("module", "merkle"),
		ModA:      schema.IdentityHash("module", "modA"),
		ModB:      schema.IdentityHash("module", "modB"),
		ModC:      schema.IdentityHash("module", "modC"),

		X: schema.IdentityHash("mod", "component", "X"),
		Y: schema.IdentityHash("mod", "component", "Y"),
		Z: schema.IdentityHash("mod", "component", "Z"),

		CompA: schema.IdentityHash("modA", "component", "CompA"),
		CompB: schema.IdentityHash("modB", "component", "CompB"),
		CompC: schema.IdentityHash("modC", "component", "CompC"),

		SCHK: schema.IdentityHash("validator", "component", "SchemaChecker"),
		HCMP: schema.IdentityHash("merkle", "impl_section", "Hash computation"),
		NEW:  schema.IdentityHash("validator", "component", "OrphanDetector"),
		REG:  schema.IdentityHash("proposal", "component", "Registrar"),
	}
}

// --- S1: ActionClassifier produces correct actions for each category ---

func TestFR3_S1_ClassifyActions_FullScenario(t *testing.T) {
	h := newDepFixture()
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: h.SCHK, Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: h.SCHK, BeadID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
			},
		},
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: h.HCMP, Type: merkle.Modified, OldHash: "ddd", NewHash: "eee", NodeType: "impl_section"},
				Impact: merkle.ImplOnly,
				Module: "merkle",
			},
			Records: []mapping.Record{
				{ID: 3, SpecNodeID: h.HCMP, BeadID: "spex-003", Module: "merkle", Component: "Hash computation", SpecHash: "ghi789"},
			},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: h.NEW, Type: merkle.Added, NewHash: "fff", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
		},
	}
	orphaned := []Orphaned{
		{
			Record:   mapping.Record{ID: 10, SpecNodeID: h.LEGACY, BeadID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(matches, unmatched, orphaned)

	// Expect 6 actions: 2 obsolete+create pairs for modified, 1 create for added, 1 obsolete for orphaned.
	if len(actions) != 6 {
		t.Fatalf("want 6 actions, got %d: %+v", len(actions), actions)
	}

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

	assertHasAction(t, actions, "obsolete", "spex-001", "validator", "SchemaChecker", "Spec node modified: validator/SchemaChecker")
	assertHasAction(t, actions, "create", "", "validator", "SchemaChecker", "Spec node modified (new): validator/SchemaChecker")
	assertHasAction(t, actions, "obsolete", "spex-003", "merkle", "Hash computation", "Spec node modified: merkle/Hash computation")
	assertHasAction(t, actions, "create", "", "merkle", "Hash computation", "Spec node modified (new): merkle/Hash computation")
	assertHasAction(t, actions, "create", "", "validator", h.NEW, fmt.Sprintf("New spec node: validator/%s", h.NEW))
	assertHasAction(t, actions, "obsolete", "spex-010", "merkle", "LegacyHasher", "Spec node removed: merkle/LegacyHasher")
}

// --- S2: Modified node without a matching bead ---

func TestFR3_S2_ClassifyActions_ModifiedUnmatched(t *testing.T) {
	hash := schema.IdentityHash("render", "component", "MarkdownRenderer")
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: hash, Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
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
	if a.SpecNodeID != hash {
		t.Errorf("want SpecNodeID %q, got %q", hash, a.SpecNodeID)
	}
}

// --- S3: Added node with an existing bead (unexpected case) ---

func TestFR3_S3_ClassifyActions_AddedWithExistingBead(t *testing.T) {
	h := newDepFixture()
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: h.REG, Type: merkle.Added, NewHash: "new111", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "proposal",
			},
			Records: []mapping.Record{
				{ID: 20, SpecNodeID: h.REG, BeadID: "spex-020", Module: "proposal", Component: "Registrar", SpecHash: "old111"},
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
	hash := schema.IdentityHash("schema", "component", "DeprecatedLoader")
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: hash, Type: merkle.Removed, OldHash: "aaa", NodeType: "component"},
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
	h := newDepFixture()
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: h.SCHK, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: h.SCHK, BeadID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
				{ID: 5, SpecNodeID: h.SCHK, BeadID: "spex-005", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
			},
		},
	}

	actions := ClassifyActions(matches, nil, nil)

	// Two obsoletes (one per old bead) + two creates (new replacements).
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
	h := newDepFixture()
	orphaned := []Orphaned{
		{
			Record:   mapping.Record{ID: 10, SpecNodeID: h.LEGACY, BeadID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000", BeadStatus: "closed"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(nil, nil, orphaned)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (obsolete + cleanup create), got %d: %+v", len(actions), actions)
	}

	assertHasAction(t, actions, "obsolete", "spex-010", "merkle", "LegacyHasher", "Spec node removed: merkle/LegacyHasher")
	assertHasAction(t, actions, "create", "", "merkle", "LegacyHasher", "Code cleanup: merkle/LegacyHasher")

	// The cleanup create must carry OldBeadID so BeadCreator can emit
	// --deps blocks:<old-bead-id>, giving the cleanup bead a lineage link
	// back to the obsoleted bead it replaces. Regression guard for the
	// second phase of spexmachina-idd.
	var cleanup *Action
	for i := range actions {
		if actions[i].Type == "create" {
			cleanup = &actions[i]
			break
		}
	}
	if cleanup == nil {
		t.Fatal("no create action found")
	}
	if cleanup.OldBeadID != "spex-010" {
		t.Errorf("want OldBeadID=spex-010 on cleanup action, got %q", cleanup.OldBeadID)
	}
}

// --- S5c: Removed node with open bead (no cleanup) ---

func TestFR6_S5c_ClassifyActions_RemovedOpenBead(t *testing.T) {
	draft := schema.IdentityHash("merkle", "component", "DraftHasher")
	orphaned := []Orphaned{
		{
			Record:   mapping.Record{ID: 11, SpecNodeID: draft, BeadID: "spex-011", Module: "merkle", Component: "DraftHasher", SpecHash: "yyy000", BeadStatus: "open"},
			NodeType: "component",
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
	hash := schema.IdentityHash("alpha", "component", "Comp1")
	levels := []merkle.ImpactLevel{merkle.ImplOnly, merkle.ArchImpl, merkle.Structural}
	for _, level := range levels {
		t.Run(level.String(), func(t *testing.T) {
			matches := []Match{
				{
					Change: merkle.ClassifiedChange{
						Change: merkle.Change{Path: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
						Impact: level,
						Module: "alpha",
					},
					Records: []mapping.Record{
						{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Comp1"},
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
	impl := schema.IdentityHash("render", "impl_section", "Section1")
	flow := schema.IdentityHash("render", "data_flow", "Flow1")
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Path: impl, Type: merkle.Added, NewHash: "aaa", NodeType: "impl_section"},
			Impact: merkle.ImplOnly, Module: "render",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Path: flow, Type: merkle.Added, NewHash: "bbb", NodeType: "data_flow"},
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
	hash := schema.IdentityHash("alpha", "component", "Comp1")
	change := merkle.ClassifiedChange{
		Change: merkle.Change{Path: hash, Type: merkle.Added, NewHash: "aaa", NodeType: "component"},
		Impact: merkle.ArchImpl,
		Module: "alpha",
	}

	matches := []Match{
		{
			Change:  change,
			Records: []mapping.Record{{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Comp1"}},
		},
	}
	unmatched := []Unmatched{{Change: change}}

	actions := ClassifyActions(matches, unmatched, nil)

	// Obsolete + create from the match, plus a create from the unmatched entry = 3.
	if len(actions) != 3 {
		t.Fatalf("want 3 actions (no deduplication), got %d", len(actions))
	}
}

// --- NFR5: Deterministic sort ---

func TestNFR5_ClassifyActions_DeterministicSort(t *testing.T) {
	betaHash := schema.IdentityHash("beta", "component", "Comp2")
	alphaNew := schema.IdentityHash("alpha", "component", "New")
	alphaOld := schema.IdentityHash("alpha", "component", "OldComp")
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: betaHash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "beta",
			},
			Records: []mapping.Record{{ID: 1, SpecNodeID: betaHash, BeadID: "bead-2", Module: "beta", Component: "Comp2"}},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: alphaNew, Type: merkle.Added, NewHash: "x", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 2, SpecNodeID: alphaOld, BeadID: "bead-old", Module: "alpha", Component: "OldComp"}, NodeType: "component"},
	}

	for i := 0; i < 5; i++ {
		actions := ClassifyActions(matches, unmatched, orphaned)
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
	hash := schema.IdentityHash("alpha", "component", "Comp")
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: hash, BeadID: "bead-old", Module: "alpha", Component: "Comp"},
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
			if a.SpecNodeID != hash {
				t.Errorf("want SpecNodeID %q, got %q", hash, a.SpecNodeID)
			}
		}
	}
}

// --- NodeType propagation ---

func TestFR3_ClassifyActions_NodeTypePropagated(t *testing.T) {
	hash := schema.IdentityHash("alpha", "component", "Comp")
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Path: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []mapping.Record{
				{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Comp"},
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

// --- SpecNodeID propagation from orphaned records ---

func TestFR3_ClassifyActions_SpecNodeIDFromOrphan(t *testing.T) {
	hash := schema.IdentityHash("alpha", "component", "Gone")
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Gone"}, NodeType: "component"},
	}

	actions := ClassifyActions(nil, nil, orphaned)
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].SpecNodeID != hash {
		t.Errorf("want SpecNodeID %q on obsolete, got %q", hash, actions[0].SpecNodeID)
	}
	if actions[0].NodeType != "component" {
		t.Errorf("want NodeType propagated from orphan, got %q", actions[0].NodeType)
	}
}

// --- Orphaned without status defaults to simple obsolete (no cleanup) ---

func TestFR6_ClassifyActions_OrphanedNoStatusDefaultObsolete(t *testing.T) {
	hash := schema.IdentityHash("alpha", "component", "Comp")
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Comp"}, NodeType: "component"},
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
	hash := schema.IdentityHash("alpha", "component", "Comp")
	orphaned := []Orphaned{
		{Record: mapping.Record{ID: 1, SpecNodeID: hash, BeadID: "bead-1", Module: "alpha", Component: "Comp", BeadStatus: "in_progress"}, NodeType: "component"},
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
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   h.ModImpact,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: h.NM, Name: "NodeMatcher"},
					{ID: h.AC, Name: "ActionClassifier", Uses: []string{h.NM}},
				},
			},
		},
		modulesByID: map[string]string{h.ModImpact: "impact"},
	}
	records := []mapping.Record{
		{ID: 50, SpecNodeID: h.NM, BeadID: "spex-050", Module: "impact", Component: "NodeMatcher", BeadStatus: "open"},
		{ID: 51, SpecNodeID: h.AC, BeadID: "spex-051", Module: "impact", Component: "ActionClassifier", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "impact", Node: "ActionClassifier", NodeType: "component", SpecNodeID: h.AC}
	deps := ResolveDeps(graph, records, action)

	assertContains(t, deps, "spex-050")
	assertNotContains(t, deps, "spex-051") // the action itself, not a dep
}

// --- D2: Component uses edge skips closed dependency bead ---

func TestFR7_D2_ResolveDeps_UsesClosedBead(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   h.ModImpact,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: h.NM, Name: "NodeMatcher"},
					{ID: h.AC, Name: "ActionClassifier", Uses: []string{h.NM}},
				},
			},
		},
		modulesByID: map[string]string{h.ModImpact: "impact"},
	}
	records := []mapping.Record{
		{ID: 51, SpecNodeID: h.NM, BeadID: "spex-051", Module: "impact", Component: "NodeMatcher", BeadStatus: "closed"},
	}

	action := Action{Type: "create", Module: "impact", Node: "ActionClassifier", NodeType: "component", SpecNodeID: h.AC}
	deps := ResolveDeps(graph, records, action)

	if len(deps) != 0 {
		t.Errorf("want 0 deps (closed bead skipped), got %v", deps)
	}
}

// --- D3: requires_module resolves to all open component beads ---

func TestFR7_D3_ResolveDeps_RequiresModuleOpenBeads(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:             h.ModImpact,
				Name:           "impact",
				RequiresModule: []string{h.ModMerkle},
				Components: []mapping.ComponentInfo{
					{ID: h.AC, Name: "ActionClassifier"},
				},
			},
			"merkle": {
				ID:   h.ModMerkle,
				Name: "merkle",
				Components: []mapping.ComponentInfo{
					{ID: h.HASH, Name: "Hasher"},
					{ID: h.TREE, Name: "TreeBuilder"},
					{ID: h.SNAP, Name: "SnapshotStore"},
				},
			},
		},
		modulesByID: map[string]string{h.ModMerkle: "merkle", h.ModImpact: "impact"},
	}
	records := []mapping.Record{
		{ID: 60, SpecNodeID: h.HASH, BeadID: "spex-060", Module: "merkle", BeadStatus: "open"},
		{ID: 61, SpecNodeID: h.TREE, BeadID: "spex-061", Module: "merkle", BeadStatus: "closed"},
		{ID: 62, SpecNodeID: h.SNAP, BeadID: "spex-062", Module: "merkle", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "impact", Node: "ActionClassifier", NodeType: "component", SpecNodeID: h.AC}
	deps := ResolveDeps(graph, records, action)

	assertContains(t, deps, "spex-060")
	assertContains(t, deps, "spex-062")
	assertNotContains(t, deps, "spex-061")
}

// --- D4: Transitive requires_module resolution ---

func TestFR7_D4_ResolveDeps_TransitiveRequiresModule(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: h.ModA, Name: "modA", RequiresModule: []string{h.ModB}, Components: []mapping.ComponentInfo{{ID: h.CompA, Name: "CompA"}}},
			"modB": {ID: h.ModB, Name: "modB", RequiresModule: []string{h.ModC}, Components: []mapping.ComponentInfo{{ID: h.CompB, Name: "CompB"}}},
			"modC": {ID: h.ModC, Name: "modC", Components: []mapping.ComponentInfo{{ID: h.CompC, Name: "CompC"}}},
		},
		modulesByID: map[string]string{h.ModA: "modA", h.ModB: "modB", h.ModC: "modC"},
	}
	records := []mapping.Record{
		{ID: 70, SpecNodeID: h.CompB, BeadID: "spex-070", Module: "modB", BeadStatus: "open"},
		{ID: 71, SpecNodeID: h.CompC, BeadID: "spex-071", Module: "modC", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "CompA", NodeType: "component", SpecNodeID: h.CompA}
	deps := ResolveDeps(graph, records, action)

	assertContains(t, deps, "spex-070")
	assertContains(t, deps, "spex-071")
}

// --- D5: Component uses edges are NOT transitive ---

func TestFR7_D5_ResolveDeps_UsesNotTransitive(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   schema.IdentityHash("module", "mod"),
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: h.X, Name: "X", Uses: []string{h.Y}},
					{ID: h.Y, Name: "Y", Uses: []string{h.Z}},
					{ID: h.Z, Name: "Z"},
				},
			},
		},
	}
	records := []mapping.Record{
		{ID: 80, SpecNodeID: h.Y, BeadID: "spex-080", Module: "mod", BeadStatus: "open"},
		{ID: 81, SpecNodeID: h.Z, BeadID: "spex-081", Module: "mod", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component", SpecNodeID: h.X}
	deps := ResolveDeps(graph, records, action)

	assertContains(t, deps, "spex-080")
	assertNotContains(t, deps, "spex-081")
}

// --- D6: Mixed uses and requires_module dependencies ---

func TestFR7_D6_ResolveDeps_MixedUsesAndRequiresModule(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {
				ID:             h.ModA,
				Name:           "modA",
				RequiresModule: []string{h.ModB},
				Components: []mapping.ComponentInfo{
					{ID: h.X, Name: "X", Uses: []string{h.Y}},
					{ID: h.Y, Name: "Y"},
				},
			},
			"modB": {
				ID:   h.ModB,
				Name: "modB",
				Components: []mapping.ComponentInfo{
					{ID: h.CompB, Name: "CompB"},
				},
			},
		},
		modulesByID: map[string]string{h.ModA: "modA", h.ModB: "modB"},
	}
	records := []mapping.Record{
		{ID: 90, SpecNodeID: h.Y, BeadID: "spex-090", Module: "modA", BeadStatus: "open"},
		{ID: 91, SpecNodeID: h.CompB, BeadID: "spex-091", Module: "modB", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "X", NodeType: "component", SpecNodeID: h.X}
	deps := ResolveDeps(graph, records, action)

	assertContains(t, deps, "spex-090")
	assertContains(t, deps, "spex-091")
}

// --- D7: No dependencies when all beads are closed ---

func TestFR7_D7_ResolveDeps_AllClosed(t *testing.T) {
	h := newDepFixture()
	modID := schema.IdentityHash("module", "mod")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:             modID,
				Name:           "mod",
				RequiresModule: []string{h.ModB},
				Components: []mapping.ComponentInfo{
					{ID: h.X, Name: "X", Uses: []string{h.Y}},
					{ID: h.Y, Name: "Y"},
				},
			},
			"modB": {ID: h.ModB, Name: "modB", Components: []mapping.ComponentInfo{{ID: h.CompB, Name: "CompB"}}},
		},
		modulesByID: map[string]string{modID: "mod", h.ModB: "modB"},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: h.Y, BeadID: "b1", Module: "mod", BeadStatus: "closed"},
		{ID: 2, SpecNodeID: h.CompB, BeadID: "b2", Module: "modB", BeadStatus: "closed"},
	}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component", SpecNodeID: h.X}
	deps := ResolveDeps(graph, records, action)

	if len(deps) != 0 {
		t.Errorf("want 0 deps (all closed), got %v", deps)
	}
}

// --- D8: No dependencies for nodes without uses or requires_module ---

func TestFR7_D8_ResolveDeps_NoDeps(t *testing.T) {
	standalone := schema.IdentityHash("mod", "component", "Standalone")
	modID := schema.IdentityHash("module", "mod")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   modID,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: standalone, Name: "Standalone"},
				},
			},
		},
		modulesByID: map[string]string{modID: "mod"},
	}

	action := Action{Type: "create", Module: "mod", Node: "Standalone", NodeType: "component", SpecNodeID: standalone}
	deps := ResolveDeps(graph, nil, action)

	if len(deps) != 0 {
		t.Errorf("want 0 deps, got %v", deps)
	}
}

// --- D9: Cycle detection in requires_module ---

func TestFR7_D9_ResolveDeps_CycleDetection(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: h.ModA, Name: "modA", RequiresModule: []string{h.ModB}, Components: []mapping.ComponentInfo{{ID: h.CompA, Name: "CompA"}}},
			"modB": {ID: h.ModB, Name: "modB", RequiresModule: []string{h.ModA}, Components: []mapping.ComponentInfo{{ID: h.CompB, Name: "CompB"}}},
		},
		modulesByID: map[string]string{h.ModA: "modA", h.ModB: "modB"},
	}
	records := []mapping.Record{
		{ID: 1, SpecNodeID: h.CompA, BeadID: "b-a", Module: "modA", BeadStatus: "open"},
		{ID: 2, SpecNodeID: h.CompB, BeadID: "b-b", Module: "modB", BeadStatus: "open"},
	}

	action := Action{Type: "create", Module: "modA", Node: "CompA", NodeType: "component", SpecNodeID: h.CompA}

	// Must terminate — no infinite recursion on the modA↔modB cycle.
	deps := ResolveDeps(graph, records, action)
	// modB is reachable from modA; its open bead should appear.
	assertContains(t, deps, "b-b")
}

// --- D12: Deps with beads created in same apply run ---

func TestFR7_D12_ResolveDeps_NoBeadForNewComponent(t *testing.T) {
	h := newDepFixture()
	modID := schema.IdentityHash("module", "mod")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   modID,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: h.X, Name: "X", Uses: []string{h.Y}},
					{ID: h.Y, Name: "Y"},
				},
			},
		},
		modulesByID: map[string]string{modID: "mod"},
	}
	// No records for Y — it is being created in the same run.
	records := []mapping.Record{}

	action := Action{Type: "create", Module: "mod", Node: "X", NodeType: "component", SpecNodeID: h.X}
	deps := ResolveDeps(graph, records, action)

	if len(deps) != 0 {
		t.Errorf("want 0 deps (Y has no bead yet), got %v", deps)
	}
}

// --- ResolveDeps ignores non-component actions ---

func TestFR7_ResolveDeps_NonComponentActionReturnsNil(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {ID: h.ModImpact, Name: "impact", Components: []mapping.ComponentInfo{{ID: h.AC, Name: "ActionClassifier", Uses: []string{h.NM}}}},
		},
	}
	records := []mapping.Record{{ID: 1, SpecNodeID: h.NM, BeadID: "spex-999", Module: "impact", BeadStatus: "open"}}

	action := Action{Type: "create", Module: "impact", NodeType: "test_section", SpecNodeID: h.AC}
	if deps := ResolveDeps(graph, records, action); deps != nil {
		t.Errorf("want nil deps for non-component action, got %v", deps)
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
	modulesByID map[string]string
}

func (s *stubSpecGraph) ModuleByName(name string) (mapping.ModuleInfo, error) {
	m, ok := s.modules[name]
	if !ok {
		return mapping.ModuleInfo{}, fmt.Errorf("module %q not found", name)
	}
	return m, nil
}

func (s *stubSpecGraph) ModuleByID(id string) (mapping.ModuleInfo, error) {
	name, ok := s.modulesByID[id]
	if !ok {
		return mapping.ModuleInfo{}, fmt.Errorf("module id %s not found", id)
	}
	return s.ModuleByName(name)
}
