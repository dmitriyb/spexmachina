package impact

import (
	"reflect"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// fixtureHashes holds the canonical identity hashes used across the bead-matching
// test scenarios in spec/impact/test_bead_matching.md. Computed once so that
// fixtures stay readable (SCHK_HASH, HASR_HASH, ...) while still exercising the
// real IdentityHash derivation used in production.
type fixtureHashes struct {
	SCHK     string // validator/component/SchemaChecker
	HASR     string // merkle/component/Hasher
	HTST     string // merkle/test_section/Hashing tests
	NEW      string // new (added) component without a record
	REMOVED  string // removed component
	VALIDMOD string // module/validator
	MERKLMOD string // module/merkle
}

func newFixture() fixtureHashes {
	return fixtureHashes{
		SCHK:     schema.IdentityHash("validator", "component", "SchemaChecker"),
		HASR:     schema.IdentityHash("merkle", "component", "Hasher"),
		HTST:     schema.IdentityHash("merkle", "test_section", "Hashing tests"),
		NEW:      schema.IdentityHash("validator", "component", "NewChecker"),
		REMOVED:  schema.IdentityHash("merkle", "component", "LegacyHasher"),
		VALIDMOD: schema.IdentityHash("module", "validator"),
		MERKLMOD: schema.IdentityHash("module", "merkle"),
	}
}

// basePairings are the journal-fold pairings shared by S3–S5 scenarios: one
// entry per node, matching the fixture's `added` + `task_created` pairs.
func basePairings(h fixtureHashes) []Pairing {
	return []Pairing{
		{SpecNodeID: h.SCHK, TaskID: "spex-001", NodeType: "component", Module: "validator", Name: "SchemaChecker"},
		{SpecNodeID: h.HASR, TaskID: "spex-002", NodeType: "component", Module: "merkle", Name: "Hasher"},
		{SpecNodeID: h.HTST, TaskID: "spex-003", NodeType: "test_section", Module: "merkle", Name: "Hashing tests"},
	}
}

// S3: NodeMatcher produces correct matched, unmatched, and orphaned lists.
func TestFR2_S3_MatchedUnmatchedOrphaned(t *testing.T) {
	h := newFixture()
	pairings := basePairings(h)
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD, OldHash: "aaa", NewHash: "bbb"},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
		{
			Change: merkle.Change{Key: h.HTST, Type: merkle.Modified, NodeType: "test_section", Module: h.MERKLMOD, OldHash: "ccc", NewHash: "ddd"},
			Impact: merkle.ImplOnly,
			Module: "merkle",
		},
		{
			Change: merkle.Change{Key: h.NEW, Type: merkle.Added, NodeType: "component", Module: h.VALIDMOD, NewHash: "eee"},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
		{
			Change: merkle.Change{Key: h.REMOVED, Type: merkle.Removed, NodeType: "component", Module: h.MERKLMOD, OldHash: "fff"},
			Impact: merkle.ArchImpl,
			Module: "merkle",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 2 {
		t.Fatalf("want 2 matched, got %d (%+v)", len(matched), matched)
	}

	matchedByKey := map[string]Match{}
	for _, m := range matched {
		matchedByKey[m.Change.Key] = m
	}
	if m, ok := matchedByKey[h.SCHK]; !ok || len(m.Records) != 1 || m.Records[0].BeadID != "spex-001" {
		t.Errorf("want SCHK → spex-001, got %+v", m)
	}
	if m, ok := matchedByKey[h.HTST]; !ok || len(m.Records) != 1 || m.Records[0].BeadID != "spex-003" {
		t.Errorf("want HTST → spex-003, got %+v", m)
	}

	if len(unmatched) != 1 || unmatched[0].Change.Key != h.NEW {
		t.Errorf("want 1 unmatched (NEW), got %+v", unmatched)
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (no removed node has a matching pairing), got %+v", orphaned)
	}
}

// S4: NodeMatcher handles multiple beads per spec node — across a node's
// lineage, not simultaneously. The fold answers with one current pairing per
// node (latest task-bearing event wins): after a task_closed followed by a
// second task_created, MatchNodes sees just that current pairing. The prior
// task remains reachable through the journal history, not through matching.
func TestFR2_S4_MultipleBeadsPerNode(t *testing.T) {
	h := newFixture()
	pairings := []Pairing{
		{SpecNodeID: h.SCHK, TaskID: "spex-001-followup", NodeType: "component", Module: "validator", Name: "SchemaChecker"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, _, _ := MatchNodes(changes, pairings)

	if len(matched) != 1 {
		t.Fatalf("want 1 match, got %d", len(matched))
	}
	if len(matched[0].Records) != 1 {
		t.Fatalf("want 1 current record for SCHK, got %d", len(matched[0].Records))
	}
	if matched[0].Records[0].BeadID != "spex-001-followup" {
		t.Errorf("want current pairing spex-001-followup, got %+v", matched[0].Records[0])
	}
}

// S5: NodeMatcher uses direct identity-hash comparison.
//
// Two distinct spec nodes always have distinct identity hashes. A pairing for
// SchemaChecker (a component) must never match a test_section change, even
// when the test_section lives in a logically related module.
func TestFR2_S5_DirectIdentityHashComparison(t *testing.T) {
	h := newFixture()
	if h.HASR == h.HTST {
		t.Fatalf("fixture precondition failed: component and test_section hashes collided")
	}

	pairings := []Pairing{
		{SpecNodeID: h.HASR, TaskID: "hasr-bead", NodeType: "component", Module: "merkle", Name: "Hasher"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.HTST, Type: merkle.Modified, NodeType: "test_section", Module: h.MERKLMOD},
			Impact: merkle.ImplOnly,
			Module: "merkle",
		},
	}

	matched, unmatched, _ := MatchNodes(changes, pairings)

	if len(matched) != 0 {
		t.Errorf("want 0 matched (HTST != HASR by exact string equality), got %+v", matched)
	}
	if len(unmatched) != 1 || unmatched[0].Change.Key != h.HTST {
		t.Errorf("want HTST as unmatched, got %+v", unmatched)
	}
}

// S6: Structural changes produce zero matches, zero unmatched, zero orphans.
func TestFR2_S6_StructuralChangesSkipped(t *testing.T) {
	h := newFixture()
	pairings := basePairings(h)
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: "meta/project", Type: merkle.Modified, NodeType: "meta", Module: ""},
			Impact: merkle.Structural,
			Module: "",
		},
		{
			Change: merkle.Change{Key: "meta/" + h.VALIDMOD, Type: merkle.Modified, NodeType: "meta", Module: h.VALIDMOD},
			Impact: merkle.Structural,
			Module: "validator",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 0 {
		t.Errorf("want 0 matches for structural changes, got %d", len(matched))
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched for structural changes, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned for structural changes, got %d", len(orphaned))
	}
}

// S7: Deterministic matching — identical inputs produce identical output,
// including when the pairing slice is shuffled between runs.
func TestNFR5_S7_DeterministicOutput(t *testing.T) {
	h := newFixture()
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.HASR, Type: merkle.Modified, NodeType: "component", Module: h.MERKLMOD},
			Impact: merkle.ArchImpl,
			Module: "merkle",
		},
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	orderA := basePairings(h)
	orderB := []Pairing{orderA[2], orderA[0], orderA[1]}

	matchedA, unmatchedA, orphanedA := MatchNodes(changes, orderA)
	matchedB, unmatchedB, orphanedB := MatchNodes(changes, orderB)

	if !reflect.DeepEqual(matchedA, matchedB) {
		t.Errorf("matched differs across shuffled runs:\nA=%+v\nB=%+v", matchedA, matchedB)
	}
	if !reflect.DeepEqual(unmatchedA, unmatchedB) {
		t.Errorf("unmatched differs across shuffled runs:\nA=%+v\nB=%+v", unmatchedA, unmatchedB)
	}
	if !reflect.DeepEqual(orphanedA, orphanedB) {
		t.Errorf("orphaned differs across shuffled runs:\nA=%+v\nB=%+v", orphanedA, orphanedB)
	}
	if len(matchedA) != 2 {
		t.Fatalf("want 2 matches, got %d", len(matchedA))
	}
	// Change order from input is preserved (HASR first, SCHK second).
	if matchedA[0].Change.Key != h.HASR || matchedA[1].Change.Key != h.SCHK {
		t.Errorf("change order not preserved: %+v", matchedA)
	}
}

// S8: Structural changes coexist with leaf-level changes — only leaf changes
// produce matches.
func TestFR2_S8_StructuralCoexistsWithLeafChanges(t *testing.T) {
	h := newFixture()
	pairings := []Pairing{
		{SpecNodeID: h.SCHK, TaskID: "spex-001", NodeType: "component", Module: "validator", Name: "SchemaChecker"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: "meta/project", Type: merkle.Modified, NodeType: "meta"},
			Impact: merkle.Structural,
			Module: "",
		},
		{
			Change: merkle.Change{Key: "meta/" + h.VALIDMOD, Type: merkle.Modified, NodeType: "meta", Module: h.VALIDMOD},
			Impact: merkle.Structural,
			Module: "validator",
		},
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD, OldHash: "aaa", NewHash: "bbb"},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 1 {
		t.Fatalf("want 1 match (leaf only), got %d", len(matched))
	}
	if matched[0].Change.Key != h.SCHK {
		t.Errorf("want matched path %s, got %s", h.SCHK, matched[0].Change.Key)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned, got %d", len(orphaned))
	}
}

// S9: No rekeying — pairings and changes share one format.
//
// Regression guard for the deleted buildMerkleIndex helper. A pairing with a
// SpecNodeID that looks like the old merkle-key format ("module/1/component/1")
// must not match a change with an identity-hash key, and vice versa. The match
// happens on raw string equality only.
func TestFR2_S9_NoRekeying(t *testing.T) {
	h := newFixture()
	oldMerkleKey := "module/1/component/1"
	oldBeadMapKey := "validator/component/SchemaChecker"

	pairings := []Pairing{
		{SpecNodeID: oldMerkleKey, TaskID: "legacy-merkle-bead", Module: "validator"},
		{SpecNodeID: oldBeadMapKey, TaskID: "legacy-beadmap-bead", Module: "validator"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, unmatched, _ := MatchNodes(changes, pairings)

	if len(matched) != 0 {
		t.Errorf("want 0 matches (no key rewriting), got %+v", matched)
	}
	if len(unmatched) != 1 || unmatched[0].Change.Key != h.SCHK {
		t.Errorf("want SCHK as unmatched — identity hash must not be rewritten into the legacy formats: %+v", unmatched)
	}
}

// E1: Change for a node whose module has no mapping pairings — appears as
// unmatched (not a panic).
func TestFR2_E1_NoRecordsForModule(t *testing.T) {
	h := newFixture()
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, nil)

	if len(matched) != 0 {
		t.Errorf("want 0 matched for empty pairings, got %d", len(matched))
	}
	if len(unmatched) != 1 || unmatched[0].Change.Key != h.SCHK {
		t.Errorf("want SCHK as unmatched, got %+v", unmatched)
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned, got %d", len(orphaned))
	}
}

// E2: Pairing for a node that has no changes — does not appear in any output list.
func TestFR2_E2_RecordWithoutMatchingChange(t *testing.T) {
	h := newFixture()
	pairings := basePairings(h) // three pairings
	changes := []merkle.ClassifiedChange{
		// Only SCHK is in the diff; HASR and HTST pairings should be silent.
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 1 {
		t.Fatalf("want 1 matched, got %d", len(matched))
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (untouched pairings are silent), got %+v", orphaned)
	}
}

// E3: Removed change with no matching pairing — no orphan is created.
func TestFR2_E3_RemovedChangeNoRecord(t *testing.T) {
	h := newFixture()
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.REMOVED, Type: merkle.Removed, NodeType: "component", Module: h.MERKLMOD, OldHash: "fff"},
			Impact: merkle.ArchImpl,
			Module: "merkle",
		},
	}

	matched, unmatched, orphaned := MatchNodes(changes, nil)

	if len(matched) != 0 {
		t.Errorf("want 0 matched, got %d", len(matched))
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched (removed with no pairing is silent), got %d", len(unmatched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (no pairing references this), got %d", len(orphaned))
	}
}

// Orphaned pairing from a removed change — pairing is reported as orphaned.
func TestFR2_OrphanedRecordFromRemovedChange(t *testing.T) {
	h := newFixture()
	pairings := []Pairing{
		{SpecNodeID: h.HASR, TaskID: "spex-002", NodeType: "component", Module: "merkle", Name: "Hasher"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.HASR, Type: merkle.Removed, NodeType: "component", Module: h.MERKLMOD, OldHash: "aaa"},
			Impact: merkle.ArchImpl,
			Module: "merkle",
		},
	}

	matched, _, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 0 {
		t.Errorf("want 0 matches for removed change, got %d", len(matched))
	}
	if len(orphaned) != 1 {
		t.Fatalf("want 1 orphaned, got %d", len(orphaned))
	}
	if orphaned[0].Record.BeadID != "spex-002" {
		t.Errorf("want orphaned pairing with bead spex-002, got %s", orphaned[0].Record.BeadID)
	}
}

// When a pairing is matched by a non-removed change, a removed change against
// the same pairing must not produce an orphan.
func TestFR2_OrphanNotCreatedIfRecordAlsoMatchesNonRemoved(t *testing.T) {
	h := newFixture()
	pairings := []Pairing{
		{SpecNodeID: h.SCHK, TaskID: "spex-001", NodeType: "component", Module: "validator"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Modified, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
		{
			Change: merkle.Change{Key: h.SCHK, Type: merkle.Removed, NodeType: "component", Module: h.VALIDMOD},
			Impact: merkle.ArchImpl,
			Module: "validator",
		},
	}

	matched, _, orphaned := MatchNodes(changes, pairings)

	if len(matched) != 1 {
		t.Errorf("want 1 match, got %d", len(matched))
	}
	if len(orphaned) != 0 {
		t.Errorf("want 0 orphaned (pairing also matched non-removed), got %d", len(orphaned))
	}
}

// Data flow changes match by identity hash just like components.
func TestFR2_DataFlowChangeMatchesByIdentityHash(t *testing.T) {
	flowHash := schema.IdentityHash("impact", "data_flow", "Impact analysis flow")
	modHash := schema.IdentityHash("module", "impact")

	pairings := []Pairing{
		{SpecNodeID: flowHash, TaskID: "flow-bead", NodeType: "data_flow", Module: "impact"},
	}
	changes := []merkle.ClassifiedChange{
		{
			Change: merkle.Change{Key: flowHash, Type: merkle.Modified, NodeType: "data_flow", Module: modHash},
			Impact: merkle.ImplOnly,
			Module: "impact",
		},
	}

	matched, unmatched, _ := MatchNodes(changes, pairings)

	if len(matched) != 1 {
		t.Fatalf("want 1 match for data_flow, got %d", len(matched))
	}
	if matched[0].Records[0].BeadID != "flow-bead" {
		t.Errorf("want bead flow-bead, got %s", matched[0].Records[0].BeadID)
	}
	if len(unmatched) != 0 {
		t.Errorf("want 0 unmatched, got %d", len(unmatched))
	}
}

func TestNFR5_EmptyInputs(t *testing.T) {
	matched, unmatched, orphaned := MatchNodes(nil, nil)
	if len(matched) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for nil inputs, got %d matched, %d unmatched, %d orphaned",
			len(matched), len(unmatched), len(orphaned))
	}

	matched, unmatched, orphaned = MatchNodes([]merkle.ClassifiedChange{}, []Pairing{})
	if len(matched) != 0 || len(unmatched) != 0 || len(orphaned) != 0 {
		t.Errorf("want all empty for empty inputs, got %d matched, %d unmatched, %d orphaned",
			len(matched), len(unmatched), len(orphaned))
	}
}
