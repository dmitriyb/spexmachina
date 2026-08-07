package impact

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// depFixture holds identity hashes used by the dependency-collection scenarios
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
	HUNK string // an unknown node type the gate must drop
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
		HUNK: schema.IdentityHash("merkle", "widget", "Hash computation"),
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
				Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []Pairing{
				{SpecNodeID: h.SCHK, TaskID: "spex-001", Module: "validator", Name: "SchemaChecker"},
			},
		},
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: h.HUNK, Type: merkle.Modified, OldHash: "ddd", NewHash: "eee", NodeType: "widget"},
				Impact: merkle.ImplOnly,
				Module: "merkle",
			},
			Records: []Pairing{
				{SpecNodeID: h.HUNK, TaskID: "spex-003", Module: "merkle", Name: "Hash computation"},
			},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: h.NEW, Type: merkle.Added, NewHash: "fff", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
		},
	}
	orphaned := []Orphaned{
		{
			Record:   Pairing{SpecNodeID: h.LEGACY, TaskID: "spex-010", Module: "merkle", Name: "LegacyHasher"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(nil, matches, unmatched, orphaned)

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
				Change: merkle.Change{Key: hash, Type: merkle.Modified, OldHash: "aaa", NewHash: "bbb", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "render",
			},
		},
	}

	actions := ClassifyActions(nil, nil, unmatched, nil)

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
				Change: merkle.Change{Key: h.REG, Type: merkle.Added, NewHash: "new111", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "proposal",
			},
			Records: []Pairing{
				{SpecNodeID: h.REG, TaskID: "spex-020", Module: "proposal", Name: "Registrar"},
			},
		},
	}

	actions := ClassifyActions(nil, matches, nil, nil)

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
				Change: merkle.Change{Key: hash, Type: merkle.Removed, OldHash: "aaa", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "schema",
			},
		},
	}

	actions := ClassifyActions(nil, nil, unmatched, nil)

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
				Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "validator",
			},
			Records: []Pairing{
				{SpecNodeID: h.SCHK, TaskID: "spex-001", Module: "validator", Name: "SchemaChecker"},
				{SpecNodeID: h.SCHK, TaskID: "spex-005", Module: "validator", Name: "SchemaChecker"},
			},
		},
	}

	actions := ClassifyActions(nil, matches, nil, nil)

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
			Record:   Pairing{SpecNodeID: h.LEGACY, TaskID: "spex-010", Module: "merkle", Name: "LegacyHasher", BeadStatus: "closed"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(nil, nil, nil, orphaned)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (obsolete + cleanup create), got %d: %+v", len(actions), actions)
	}

	assertHasAction(t, actions, "obsolete", "spex-010", "merkle", "LegacyHasher", "Spec node removed: merkle/LegacyHasher")
	assertHasAction(t, actions, "create", "", "merkle", "LegacyHasher", "Code cleanup: merkle/LegacyHasher")

	// The cleanup create must carry OldBeadID so downstream emitters can
	// record --deps blocks:<old-bead-id>, giving the cleanup bead a lineage
	// link back to the obsoleted bead it replaces. Regression guard for the
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
			Record:   Pairing{SpecNodeID: draft, TaskID: "spex-011", Module: "merkle", Name: "DraftHasher", BeadStatus: "open"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(nil, nil, nil, orphaned)

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
						Change: merkle.Change{Key: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
						Impact: level,
						Module: "alpha",
					},
					Records: []Pairing{
						{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Comp1"},
					},
				},
			}

			actions := ClassifyActions(nil, matches, nil, nil)

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

// --- an unrecognised node type is always filtered; data_flow is not ---

func TestFR3_ClassifyActions_UnknownNodeTypeFiltered(t *testing.T) {
	impl := schema.IdentityHash("render", "widget", "Section1")
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: impl, Type: merkle.Added, NewHash: "aaa", NodeType: "widget"},
			Impact: merkle.ImplOnly, Module: "render",
		}},
	}
	actions := ClassifyActions(nil, nil, unmatched, nil)
	if len(actions) != 0 {
		t.Errorf("want 0 actions for an unknown node type, got %d: %+v", len(actions), actions)
	}
}

// --- api is always filtered ---

// TestFR3_ClassifyActions_APIFiltered pins the absence of "api" from
// beadProducingTypes. An api is a contract surface (merkle classifies it as
// Contract, alongside data_flow) but it is deliberately not bead-producing:
// the components named in its provided_by array carry the work, so a bead per
// api would duplicate them. That invariant is expressed only by omission from
// the beadProducingTypes map, so nothing but this test stops an "api": true
// entry from being added there.
func TestFR3_ClassifyActions_APIFiltered(t *testing.T) {
	api := schema.IdentityHash("cli", "api", "spex diff")

	tests := []struct {
		name       string
		changeType merkle.ChangeType
	}{
		{"added", merkle.Added},
		{"modified", merkle.Modified},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unmatched := []Unmatched{
				{Change: merkle.ClassifiedChange{
					Change: merkle.Change{Key: api, Type: tt.changeType, NewHash: "aaa", NodeType: "api"},
					Impact: merkle.Contract, Module: "cli",
				}},
			}
			actions := ClassifyActions(nil, nil, unmatched, nil)
			if len(actions) != 0 {
				t.Errorf("want 0 actions for api, got %d: %+v", len(actions), actions)
			}
		})
	}
}

// --- E1: Empty inputs produce empty result ---

func TestFR3_E1_ClassifyActions_EmptyInputs(t *testing.T) {
	actions := ClassifyActions(nil, nil, nil, nil)
	if len(actions) != 0 {
		t.Errorf("want 0 actions for nil inputs, got %d", len(actions))
	}
}

// --- E5: Duplicate actions are preserved, not deduplicated ---

func TestFR3_E5_ClassifyActions_DuplicatesPreserved(t *testing.T) {
	hash := schema.IdentityHash("alpha", "component", "Comp1")
	change := merkle.ClassifiedChange{
		Change: merkle.Change{Key: hash, Type: merkle.Added, NewHash: "aaa", NodeType: "component"},
		Impact: merkle.ArchImpl,
		Module: "alpha",
	}

	matches := []Match{
		{
			Change:  change,
			Records: []Pairing{{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Comp1"}},
		},
	}
	unmatched := []Unmatched{{Change: change}}

	actions := ClassifyActions(nil, matches, unmatched, nil)

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
				Change: merkle.Change{Key: betaHash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "beta",
			},
			Records: []Pairing{{SpecNodeID: betaHash, TaskID: "bead-2", Module: "beta", Name: "Comp2"}},
		},
	}
	unmatched := []Unmatched{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: alphaNew, Type: merkle.Added, NewHash: "x", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
		},
	}
	orphaned := []Orphaned{
		{Record: Pairing{SpecNodeID: alphaOld, TaskID: "bead-old", Module: "alpha", Name: "OldComp"}, NodeType: "component"},
	}

	for i := 0; i < 5; i++ {
		actions := ClassifyActions(nil, matches, unmatched, orphaned)
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
				Change: merkle.Change{Key: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []Pairing{
				{SpecNodeID: hash, TaskID: "bead-old", Module: "alpha", Name: "Comp"},
			},
		},
	}

	actions := ClassifyActions(nil, matches, nil, nil)

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
				Change: merkle.Change{Key: hash, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Impact: merkle.ArchImpl,
				Module: "alpha",
			},
			Records: []Pairing{
				{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Comp"},
			},
		},
	}

	actions := ClassifyActions(nil, matches, nil, nil)

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
		{Record: Pairing{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Gone"}, NodeType: "component"},
	}

	actions := ClassifyActions(nil, nil, nil, orphaned)
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
		{Record: Pairing{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Comp"}, NodeType: "component"},
	}

	actions := ClassifyActions(nil, nil, nil, orphaned)

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
		{Record: Pairing{SpecNodeID: hash, TaskID: "bead-1", Module: "alpha", Name: "Comp", BeadStatus: "in_progress"}, NodeType: "component"},
	}

	actions := ClassifyActions(nil, nil, nil, orphaned)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Type != "obsolete" {
		t.Errorf("want obsolete, got %q", actions[0].Type)
	}
}

// ========================
// DepSpecNodeIDs collection tests (requirement a3ecff50de68)
// ========================

// Components `uses` contribute dependency spec_node_ids directly (no bead lookup).

func TestDepSpecNodeIDs_ComponentUsesEdgeCollected(t *testing.T) {
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
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.AC, Type: merkle.Added, NewHash: "aaa", NodeType: "component"},
			Module: "impact",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d: %+v", len(actions), actions)
	}
	assertContains(t, actions[0].DepSpecNodeIDs, h.NM)
	assertNotContains(t, actions[0].DepSpecNodeIDs, h.AC) // self-reference is filtered
}

// Bead status is irrelevant — the classifier no longer peeks at records.

func TestDepSpecNodeIDs_IgnoresBeadStatus(t *testing.T) {
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
	// Records include a closed bead for NM — the classifier must not read
	// them; that filtering moves to emit's Resolver.
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: h.AC, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "component"},
				Module: "impact",
			},
			Records: []Pairing{
				{SpecNodeID: h.AC, TaskID: "spex-ac", Module: "impact", Name: "ActionClassifier", BeadStatus: "open"},
			},
		},
	}

	actions := ClassifyActions(graph, matches, nil, nil)

	var create *Action
	for i := range actions {
		if actions[i].Type == "create" {
			create = &actions[i]
			break
		}
	}
	if create == nil {
		t.Fatal("no create action")
	}
	assertContains(t, create.DepSpecNodeIDs, h.NM)
}

// Transitive requires_module walk collects every reachable module's components.

func TestDepSpecNodeIDs_TransitiveRequiresModule(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: h.ModA, Name: "modA", RequiresModule: []string{h.ModB}, Components: []mapping.ComponentInfo{{ID: h.CompA, Name: "CompA"}}},
			"modB": {ID: h.ModB, Name: "modB", RequiresModule: []string{h.ModC}, Components: []mapping.ComponentInfo{{ID: h.CompB, Name: "CompB"}}},
			"modC": {ID: h.ModC, Name: "modC", Components: []mapping.ComponentInfo{{ID: h.CompC, Name: "CompC"}}},
		},
		modulesByID: map[string]string{h.ModA: "modA", h.ModB: "modB", h.ModC: "modC"},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.CompA, Type: merkle.Added, NewHash: "x", NodeType: "component"},
			Module: "modA",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	assertContains(t, actions[0].DepSpecNodeIDs, h.CompB)
	assertContains(t, actions[0].DepSpecNodeIDs, h.CompC)
}

// Direct component `uses` are NOT transitive — only the first hop is collected.

func TestDepSpecNodeIDs_ComponentUsesNotTransitive(t *testing.T) {
	h := newDepFixture()
	modID := schema.IdentityHash("module", "mod")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"mod": {
				ID:   modID,
				Name: "mod",
				Components: []mapping.ComponentInfo{
					{ID: h.X, Name: "X", Uses: []string{h.Y}},
					{ID: h.Y, Name: "Y", Uses: []string{h.Z}},
					{ID: h.Z, Name: "Z"},
				},
			},
		},
		modulesByID: map[string]string{modID: "mod"},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.X, Type: merkle.Added, NewHash: "x", NodeType: "component"},
			Module: "mod",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	assertContains(t, actions[0].DepSpecNodeIDs, h.Y)
	assertNotContains(t, actions[0].DepSpecNodeIDs, h.Z)
}

// Uses + requires_module edges both contribute.

func TestDepSpecNodeIDs_MixedUsesAndRequiresModule(t *testing.T) {
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
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.X, Type: merkle.Added, NewHash: "x", NodeType: "component"},
			Module: "modA",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	assertContains(t, actions[0].DepSpecNodeIDs, h.Y)     // direct uses
	assertContains(t, actions[0].DepSpecNodeIDs, h.CompB) // transitive requires_module
}

// Cycle detection must terminate without infinite recursion.

func TestDepSpecNodeIDs_RequiresModuleCycle(t *testing.T) {
	h := newDepFixture()
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"modA": {ID: h.ModA, Name: "modA", RequiresModule: []string{h.ModB}, Components: []mapping.ComponentInfo{{ID: h.CompA, Name: "CompA"}}},
			"modB": {ID: h.ModB, Name: "modB", RequiresModule: []string{h.ModA}, Components: []mapping.ComponentInfo{{ID: h.CompB, Name: "CompB"}}},
		},
		modulesByID: map[string]string{h.ModA: "modA", h.ModB: "modB"},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.CompA, Type: merkle.Added, NewHash: "x", NodeType: "component"},
			Module: "modA",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	// modB is reachable from modA; its component's identity hash must appear.
	// Must terminate.
	assertContains(t, actions[0].DepSpecNodeIDs, h.CompB)
}

// Component with no uses / no requires_module yields empty DepSpecNodeIDs.

func TestDepSpecNodeIDs_EmptyWhenNoEdges(t *testing.T) {
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
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: standalone, Type: merkle.Added, NewHash: "s", NodeType: "component"},
			Module: "mod",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions[0].DepSpecNodeIDs) != 0 {
		t.Errorf("want empty DepSpecNodeIDs, got %v", actions[0].DepSpecNodeIDs)
	}
}

// Data_flow add-on: a component create gains the data_flow's SpecNodeID when
// that data_flow is also created in the same batch and lists the component in
// its uses array.

func TestDepSpecNodeIDs_DataFlowAddOn(t *testing.T) {
	h := newDepFixture()
	flowID := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"merkle": {
				ID:   h.ModMerkle,
				Name: "merkle",
				Components: []mapping.ComponentInfo{
					{ID: h.HASH, Name: "Hasher"},
				},
				DataFlows: []mapping.DataFlowInfo{
					{ID: flowID, Name: "HashFlow", Uses: []string{h.HASH}},
				},
			},
		},
		modulesByID: map[string]string{h.ModMerkle: "merkle"},
	}
	// Both data_flow and component are created in the same batch.
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: flowID, Type: merkle.Added, NewHash: "f", NodeType: "data_flow"},
			Module: "merkle",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.HASH, Type: merkle.Added, NewHash: "c", NodeType: "component"},
			Module: "merkle",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	var hasherAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == h.HASH {
			hasherAction = &actions[i]
		}
	}
	if hasherAction == nil {
		t.Fatal("no create action for Hasher")
	}
	assertContains(t, hasherAction.DepSpecNodeIDs, flowID)
}

// Component NOT listed in a data_flow's uses does not pick up the flow's ID.

func TestDepSpecNodeIDs_DataFlowUnrelatedComponent(t *testing.T) {
	h := newDepFixture()
	flowID := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"merkle": {
				ID:   h.ModMerkle,
				Name: "merkle",
				Components: []mapping.ComponentInfo{
					{ID: h.HASH, Name: "Hasher"},
					{ID: h.LEGACY, Name: "LegacyHasher"},
				},
				DataFlows: []mapping.DataFlowInfo{
					// Flow only involves Hasher, not LegacyHasher.
					{ID: flowID, Name: "HashFlow", Uses: []string{h.HASH}},
				},
			},
		},
		modulesByID: map[string]string{h.ModMerkle: "merkle"},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: flowID, Type: merkle.Added, NewHash: "f", NodeType: "data_flow"},
			Module: "merkle",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.LEGACY, Type: merkle.Added, NewHash: "l", NodeType: "component"},
			Module: "merkle",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	var legacyAction *Action
	for i := range actions {
		if actions[i].SpecNodeID == h.LEGACY {
			legacyAction = &actions[i]
		}
	}
	if legacyAction == nil {
		t.Fatal("no create action for LegacyHasher")
	}
	assertNotContains(t, legacyAction.DepSpecNodeIDs, flowID)
}

// Data_flow add-on only fires when the flow is in the same batch. A flow that
// already exists in the graph but is not part of the current changes does not
// contribute — emit handles existing dependencies via ref:bead / ref:spec_node.

func TestDepSpecNodeIDs_DataFlowNotInBatchIgnored(t *testing.T) {
	h := newDepFixture()
	flowID := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"merkle": {
				ID:   h.ModMerkle,
				Name: "merkle",
				Components: []mapping.ComponentInfo{
					{ID: h.HASH, Name: "Hasher"},
				},
				DataFlows: []mapping.DataFlowInfo{
					{ID: flowID, Name: "HashFlow", Uses: []string{h.HASH}},
				},
			},
		},
		modulesByID: map[string]string{h.ModMerkle: "merkle"},
	}
	// Only the component is in the batch; the flow is pre-existing.
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.HASH, Type: merkle.Added, NewHash: "c", NodeType: "component"},
			Module: "merkle",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	assertNotContains(t, actions[0].DepSpecNodeIDs, flowID)
}

// Non-component creates (data_flow, test_section) do not get uses/requires_module
// deps — that walk only applies to component creates.

func TestDepSpecNodeIDs_NonComponentCreateNoUsesWalk(t *testing.T) {
	h := newDepFixture()
	flowID := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"merkle": {
				ID:             h.ModMerkle,
				Name:           "merkle",
				RequiresModule: []string{h.ModA},
				Components: []mapping.ComponentInfo{
					{ID: h.HASH, Name: "Hasher"},
				},
				DataFlows: []mapping.DataFlowInfo{
					{ID: flowID, Name: "HashFlow", Uses: []string{h.HASH}},
				},
			},
			"modA": {
				ID:   h.ModA,
				Name: "modA",
				Components: []mapping.ComponentInfo{
					{ID: h.CompA, Name: "CompA"},
				},
			},
		},
		modulesByID: map[string]string{h.ModMerkle: "merkle", h.ModA: "modA"},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: flowID, Type: merkle.Added, NewHash: "f", NodeType: "data_flow"},
			Module: "merkle",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	// A data_flow create does NOT transitively collect its module's requires_module
	// closure — only component creates do that walk.
	if len(actions[0].DepSpecNodeIDs) != 0 {
		t.Errorf("want empty DepSpecNodeIDs for data_flow create, got %v", actions[0].DepSpecNodeIDs)
	}
}

// Nil graph => no dep collection; other classification paths still work.

func TestDepSpecNodeIDs_NilGraphLeavesDepsEmpty(t *testing.T) {
	h := newDepFixture()
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: h.AC, Type: merkle.Added, NewHash: "a", NodeType: "component"},
			Module: "impact",
		}},
	}

	actions := ClassifyActions(nil, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if len(actions[0].DepSpecNodeIDs) != 0 {
		t.Errorf("want empty DepSpecNodeIDs with nil graph, got %v", actions[0].DepSpecNodeIDs)
	}
}

// Obsolete actions never carry DepSpecNodeIDs — dependency info belongs on
// creates only.

func TestDepSpecNodeIDs_ObsoleteCarriesNoDeps(t *testing.T) {
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
	orphaned := []Orphaned{
		{
			Record:   Pairing{SpecNodeID: h.AC, TaskID: "spex-ac", Module: "impact", Name: "ActionClassifier"},
			NodeType: "component",
		},
	}

	actions := ClassifyActions(graph, nil, nil, orphaned)

	for _, a := range actions {
		if a.Type == "obsolete" && len(a.DepSpecNodeIDs) != 0 {
			t.Errorf("obsolete action must not carry DepSpecNodeIDs, got %v", a.DepSpecNodeIDs)
		}
	}
}

// ========================
// Data-flow and test_section gating (proposal 2026-04-12-data-flow-contract-layer)
// ========================

// --- Data flow added without a matching bead produces a create action ---

func TestFR8_ClassifyActions_DataFlowAddedProducesBead(t *testing.T) {
	flow := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: flow, Type: merkle.Added, NewHash: "ff1", NodeType: "data_flow"},
			Impact: merkle.Contract, Module: "merkle",
		}},
	}

	actions := ClassifyActions(nil, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action for added data_flow, got %d: %+v", len(actions), actions)
	}
	a := actions[0]
	if a.Type != "create" {
		t.Errorf("want type create, got %q", a.Type)
	}
	if a.NodeType != "data_flow" {
		t.Errorf("want NodeType data_flow, got %q", a.NodeType)
	}
	if a.SpecNodeID != flow {
		t.Errorf("want SpecNodeID %q, got %q", flow, a.SpecNodeID)
	}
}

// --- Data flow modified with a matching bead produces obsolete+create ---

func TestFR8_ClassifyActions_DataFlowModifiedMatched(t *testing.T) {
	flow := schema.IdentityHash("merkle", "data_flow", "HashFlow")
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: flow, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "data_flow"},
				Impact: merkle.Contract, Module: "merkle",
			},
			Records: []Pairing{
				{SpecNodeID: flow, TaskID: "spex-flow", Module: "merkle", Name: "HashFlow"},
			},
		},
	}

	actions := ClassifyActions(nil, matches, nil, nil)

	if len(actions) != 2 {
		t.Fatalf("want 2 actions (obsolete+create), got %d: %+v", len(actions), actions)
	}
	var hasObsolete, hasCreate bool
	for _, a := range actions {
		if a.NodeType != "data_flow" {
			t.Errorf("want NodeType=data_flow on every action, got %q", a.NodeType)
		}
		if a.Type == "obsolete" && a.BeadID == "spex-flow" {
			hasObsolete = true
		}
		if a.Type == "create" && a.OldBeadID == "spex-flow" {
			hasCreate = true
		}
	}
	if !hasObsolete || !hasCreate {
		t.Errorf("want obsolete(spex-flow)+create(OldBeadID=spex-flow), got %+v", actions)
	}
}

// --- test_section with len(describes) == 1 is bundled into its component ---

func TestFR8_ClassifyActions_TestSectionSingleDescribesSkipped(t *testing.T) {
	h := newDepFixture()
	testID := schema.IdentityHash("impact", "test_section", "MatcherTests")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   h.ModImpact,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: h.NM, Name: "NodeMatcher"},
				},
				TestSections: []mapping.TestSectionInfo{
					{ID: testID, Name: "MatcherTests", Describes: []string{h.NM}},
				},
			},
		},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: testID, Type: merkle.Added, NewHash: "t1", NodeType: "test_section"},
			Impact: merkle.ImplOnly, Module: "impact",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 0 {
		t.Fatalf("want 0 actions for single-describes test_section, got %d: %+v", len(actions), actions)
	}
}

// --- test_section with len(describes) >= 2 produces a task bead ---

func TestFR8_ClassifyActions_TestSectionMultiDescribesProducesBead(t *testing.T) {
	h := newDepFixture()
	testID := schema.IdentityHash("impact", "test_section", "ClassifyReport")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   h.ModImpact,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: h.AC, Name: "ActionClassifier"},
					{ID: h.NM, Name: "NodeMatcher"},
				},
				TestSections: []mapping.TestSectionInfo{
					{ID: testID, Name: "ClassifyReport", Describes: []string{h.AC, h.NM}},
				},
			},
		},
	}
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: testID, Type: merkle.Added, NewHash: "t2", NodeType: "test_section"},
			Impact: merkle.ImplOnly, Module: "impact",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action for multi-describes test_section, got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != "create" || actions[0].NodeType != "test_section" {
		t.Errorf("want create/test_section action, got %+v", actions[0])
	}
}

// --- test_section gate falls back to produce-bead when graph is nil ---

func TestFR8_ClassifyActions_TestSectionNilGraphDefaultsToProduce(t *testing.T) {
	testID := schema.IdentityHash("mod", "test_section", "T")
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: testID, Type: merkle.Added, NewHash: "t3", NodeType: "test_section"},
			Impact: merkle.ImplOnly, Module: "mod",
		}},
	}

	// nil graph: classifier cannot prove the coupling rule; keep the action.
	actions := ClassifyActions(nil, nil, unmatched, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action when graph is nil (safe fallback), got %d: %+v", len(actions), actions)
	}
}

// --- Matched test_section with len(describes) == 1 obsoletes only, no create ---

func TestFR8_ClassifyActions_TestSectionCoupledMatchedObsoleteOnly(t *testing.T) {
	h := newDepFixture()
	testID := schema.IdentityHash("impact", "test_section", "MatcherTests")
	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"impact": {
				ID:   h.ModImpact,
				Name: "impact",
				Components: []mapping.ComponentInfo{
					{ID: h.NM, Name: "NodeMatcher"},
				},
				TestSections: []mapping.TestSectionInfo{
					{ID: testID, Name: "MatcherTests", Describes: []string{h.NM}},
				},
			},
		},
	}
	matches := []Match{
		{
			Change: merkle.ClassifiedChange{
				Change: merkle.Change{Key: testID, Type: merkle.Modified, OldHash: "a", NewHash: "b", NodeType: "test_section"},
				Impact: merkle.ImplOnly, Module: "impact",
			},
			Records: []Pairing{
				{SpecNodeID: testID, TaskID: "spex-test", Module: "impact", Name: "MatcherTests"},
			},
		},
	}

	actions := ClassifyActions(graph, matches, nil, nil)

	if len(actions) != 1 {
		t.Fatalf("want 1 action (obsolete only for coupled test_section), got %d: %+v", len(actions), actions)
	}
	if actions[0].Type != "obsolete" || actions[0].BeadID != "spex-test" {
		t.Errorf("want obsolete(spex-test), got %+v", actions[0])
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

// stubSpecGraph implements a minimal SpecGraph for testing dependency collection.
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

// TestClassifyActions_ResolvesAddedNodeNames verifies that added spec nodes
// (data_flow, test_section, component) get their human-readable name on
// Action.Node, not the raw identity hash. Spec contract: arch_action_classifier.md
// states Action.Node is "affected spec node name", and reason templates use
// {node_name}. Prior to the fix, only matched (modified) nodes resolved names;
// unmatched (added) nodes carried the identity hash. Regression guard for
// bug spexmachina-sjm.
func TestClassifyActions_ResolvesAddedNodeNames(t *testing.T) {
	compID := schema.IdentityHash("emit", "component", "ChangesetBuilder")
	flowID := schema.IdentityHash("emit", "data_flow", "Emit flow")
	testID := schema.IdentityHash("emit", "test_section", "Resolver and sorter tests")

	graph := &stubSpecGraph{
		modules: map[string]mapping.ModuleInfo{
			"emit": {
				Name: "emit",
				Components: []mapping.ComponentInfo{
					{ID: compID, Name: "ChangesetBuilder"},
				},
				DataFlows: []mapping.DataFlowInfo{
					{ID: flowID, Name: "Emit flow"},
				},
				TestSections: []mapping.TestSectionInfo{
					{ID: testID, Name: "Resolver and sorter tests", Describes: []string{"a", "b"}},
				},
			},
		},
	}

	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: compID, Type: merkle.Added, NewHash: "h1", NodeType: "component"},
			Module: "emit",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: flowID, Type: merkle.Added, NewHash: "h2", NodeType: "data_flow"},
			Module: "emit",
		}},
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: testID, Type: merkle.Added, NewHash: "h3", NodeType: "test_section"},
			Module: "emit",
		}},
	}

	actions := ClassifyActions(graph, nil, unmatched, nil)

	want := map[string]string{
		compID: "ChangesetBuilder",
		flowID: "Emit flow",
		testID: "Resolver and sorter tests",
	}
	if len(actions) != len(want) {
		t.Fatalf("want %d actions, got %d", len(want), len(actions))
	}
	for _, a := range actions {
		expectedName, ok := want[a.SpecNodeID]
		if !ok {
			t.Errorf("unexpected action for spec_node_id %s", a.SpecNodeID)
			continue
		}
		if a.Node != expectedName {
			t.Errorf("spec_node_id %s: want Node=%q, got %q", a.SpecNodeID, expectedName, a.Node)
		}
		expectedReason := fmt.Sprintf("New spec node: emit/%s", expectedName)
		if a.Reason != expectedReason {
			t.Errorf("spec_node_id %s: want Reason=%q, got %q", a.SpecNodeID, expectedReason, a.Reason)
		}
	}
}

// TestClassifyActions_ResolvesNodeName_NilGraphFallback guards the backward-
// compatible fallback: with no graph supplied, Action.Node falls back to the
// identity hash. Several existing tests rely on this by passing nil.
func TestClassifyActions_ResolvesNodeName_NilGraphFallback(t *testing.T) {
	specNodeID := schema.IdentityHash("emit", "data_flow", "Emit flow")
	unmatched := []Unmatched{
		{Change: merkle.ClassifiedChange{
			Change: merkle.Change{Key: specNodeID, Type: merkle.Added, NewHash: "h1", NodeType: "data_flow"},
			Module: "emit",
		}},
	}
	actions := ClassifyActions(nil, nil, unmatched, nil)
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	if actions[0].Node != specNodeID {
		t.Errorf("nil graph: want fallback Node=%s, got %q", specNodeID, actions[0].Node)
	}
}
