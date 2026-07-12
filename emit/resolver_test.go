package emit

import (
	"fmt"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// fakeSpecGraph is an in-memory SpecGraph for Resolver priority tests.
// Lookups miss when the requested key is not pre-loaded so callers can
// exercise the fallback paths explicitly.
type fakeSpecGraph struct {
	components   map[string]Component
	moduleReqs   map[string]ModuleRequirement
	projectReqs  map[string]ProjectRequirement
	paths        map[string]NodePaths
}

func newFakeSpecGraph() *fakeSpecGraph {
	return &fakeSpecGraph{
		components:  make(map[string]Component),
		moduleReqs:  make(map[string]ModuleRequirement),
		projectReqs: make(map[string]ProjectRequirement),
		paths:       make(map[string]NodePaths),
	}
}

func (g *fakeSpecGraph) Component(id string) (Component, bool) {
	c, ok := g.components[id]
	return c, ok
}

func (g *fakeSpecGraph) ModuleRequirement(id string) (ModuleRequirement, bool) {
	r, ok := g.moduleReqs[id]
	return r, ok
}

func (g *fakeSpecGraph) ProjectRequirement(id string) (ProjectRequirement, bool) {
	r, ok := g.projectReqs[id]
	return r, ok
}

func (g *fakeSpecGraph) Paths(id string) (NodePaths, bool) {
	p, ok := g.paths[id]
	return p, ok
}

func intPtr(i int) *int { return &i }

// fakeStore is an in-memory mapping.Store double for Resolver and
// Labeler tests. GetBySpecNode, GetByProposalEpic, and GetByBead are
// exercised; the other methods return errors so any accidental
// dependency fails loudly.
type fakeStore struct {
	bySpecNode map[string][]mapping.Record
	byBead     map[string]mapping.Record
	epic       map[string]mapping.Record
	err        error
	nextID     int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		bySpecNode: make(map[string][]mapping.Record),
		byBead:     make(map[string]mapping.Record),
		epic:       make(map[string]mapping.Record),
		nextID:     1,
	}
}

func (s *fakeStore) GetBySpecNode(id string) ([]mapping.Record, error) {
	if s.err != nil {
		return nil, s.err
	}
	recs, ok := s.bySpecNode[id]
	if !ok {
		return nil, fmt.Errorf("fakeStore: %w: %s", mapping.ErrNotFound, id)
	}
	return recs, nil
}

func (s *fakeStore) GetByProposalEpic(proposal string) (mapping.Record, error) {
	r, ok := s.epic[proposal]
	if !ok {
		return mapping.Record{}, fmt.Errorf("fakeStore: %w: %s", mapping.ErrNotFound, proposal)
	}
	return r, nil
}

func (s *fakeStore) Create(mapping.Record) (int, error) {
	return 0, fmt.Errorf("fakeStore.Create: not implemented")
}
func (s *fakeStore) Get(int) (mapping.Record, error) {
	return mapping.Record{}, fmt.Errorf("fakeStore.Get: not implemented")
}
func (s *fakeStore) GetByBead(beadID string) (mapping.Record, error) {
	r, ok := s.byBead[beadID]
	if !ok {
		return mapping.Record{}, fmt.Errorf("fakeStore: %w: %s", mapping.ErrNotFound, beadID)
	}
	return r, nil
}
func (s *fakeStore) Update(int, map[string]string) error {
	return fmt.Errorf("fakeStore.Update: not implemented")
}
func (s *fakeStore) Delete(int) error    { return fmt.Errorf("fakeStore.Delete: not implemented") }
func (s *fakeStore) List() ([]mapping.Record, error) {
	return nil, fmt.Errorf("fakeStore.List: not implemented")
}
func (s *fakeStore) NextRecordID() (int, error) {
	return s.nextID, nil
}
func (s *fakeStore) Replace([]mapping.Record, int) error {
	return fmt.Errorf("fakeStore.Replace: not implemented")
}

// TestResolveDeps_ClassifiesEachShape covers the spec scenario:
// deps [A, B, C] — A has an open mapping record, B is in-batch, C has
// no record. Expected: ref:bead, ref:op, ref:spec_node respectively. The
// input order is preserved for determinism.
func TestResolveDeps_ClassifiesEachShape(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["A"] = []mapping.Record{
		{ID: 1, SpecNodeID: "A", BeadID: "br-1", BeadStatus: "open"},
	}
	r := &Resolver{
		MappingStore: store,
		Batch:        map[string]string{"B": "op-007"},
	}

	got, err := r.ResolveDeps([]string{"A", "B", "C"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(deps): want 3, got %d (%+v)", len(got), got)
	}

	if got[0].Kind != RefBead || got[0].BeadID != "br-1" {
		t.Errorf("dep[0] (A): want ref:bead br-1, got %+v", got[0])
	}
	if got[1].Kind != RefOp || got[1].OpID != "op-007" {
		t.Errorf("dep[1] (B): want ref:op op-007, got %+v", got[1])
	}
	if got[2].Kind != RefSpecNode || got[2].SpecNodeID != "C" {
		t.Errorf("dep[2] (C): want ref:spec_node C, got %+v", got[2])
	}
}

// TestResolveDeps_DropsClosedBeadDeps verifies that a dep whose mapping
// record is closed is omitted entirely — the work is satisfied so no edge
// should be emitted.
func TestResolveDeps_DropsClosedBeadDeps(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["closed-dep"] = []mapping.Record{
		{ID: 5, SpecNodeID: "closed-dep", BeadID: "br-old", BeadStatus: "closed"},
	}
	store.bySpecNode["open-dep"] = []mapping.Record{
		{ID: 6, SpecNodeID: "open-dep", BeadID: "br-keep", BeadStatus: "open"},
	}
	r := &Resolver{MappingStore: store, Batch: map[string]string{}}

	got, err := r.ResolveDeps([]string{"closed-dep", "open-dep"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(deps): want 1 (closed dropped), got %d (%+v)", len(got), got)
	}
	if got[0].Kind != RefBead || got[0].BeadID != "br-keep" {
		t.Errorf("surviving dep: want ref:bead br-keep, got %+v", got[0])
	}
}

// TestResolveDeps_BatchBeatsMappingRecord covers the edge case where a
// dep spec_node_id is BOTH in r.Batch and has an open mapping record.
// Per the impl spec, ref:op wins — the in-batch op is the authoritative
// latest work and the mapping record may be stale mid-batch.
func TestResolveDeps_BatchBeatsMappingRecord(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["both"] = []mapping.Record{
		{ID: 1, SpecNodeID: "both", BeadID: "br-stale", BeadStatus: "open"},
	}
	r := &Resolver{
		MappingStore: store,
		Batch:        map[string]string{"both": "op-9"},
	}

	got, err := r.ResolveDeps([]string{"both"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(got) != 1 || got[0].Kind != RefOp || got[0].OpID != "op-9" {
		t.Fatalf("want ref:op op-9 (batch wins), got %+v", got)
	}
}

// TestResolveDeps_EmptyStatusTreatedAsOpen covers the conservative rule
// from the impl spec: a record with empty BeadStatus is treated as open
// rather than silently dropped.
func TestResolveDeps_EmptyStatusTreatedAsOpen(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["x"] = []mapping.Record{
		{ID: 1, SpecNodeID: "x", BeadID: "br-x", BeadStatus: ""},
	}
	r := &Resolver{MappingStore: store, Batch: map[string]string{}}

	got, err := r.ResolveDeps([]string{"x"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(got) != 1 || got[0].Kind != RefBead || got[0].BeadID != "br-x" {
		t.Fatalf("empty status must be treated as open: got %+v", got)
	}
}

// TestResolveDeps_HighestOpenRecordWins covers pickOpenRecord's tiebreak:
// when two open records exist for one spec_node_id, the highest-ID record's
// bead is chosen — the latest re-implementation supersedes earlier records.
func TestResolveDeps_HighestOpenRecordWins(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["dup"] = []mapping.Record{
		{ID: 3, SpecNodeID: "dup", BeadID: "br-newer", BeadStatus: "open"},
		{ID: 2, SpecNodeID: "dup", BeadID: "br-older", BeadStatus: "open"},
		{ID: 9, SpecNodeID: "dup", BeadID: "br-closed", BeadStatus: "closed"},
	}
	r := &Resolver{MappingStore: store, Batch: map[string]string{}}

	got, err := r.ResolveDeps([]string{"dup"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(got) != 1 || got[0].Kind != RefBead || got[0].BeadID != "br-newer" {
		t.Fatalf("want ref:bead br-newer (highest open ID wins, closed ignored), got %+v", got)
	}
}

// TestResolveDeps_PreservesOrder asserts the spec's determinism property:
// output order matches input order regardless of classification path.
func TestResolveDeps_PreservesOrder(t *testing.T) {
	store := newFakeStore()
	store.bySpecNode["beta"] = []mapping.Record{
		{ID: 1, SpecNodeID: "beta", BeadID: "br-b", BeadStatus: "open"},
	}
	r := &Resolver{
		MappingStore: store,
		Batch:        map[string]string{"alpha": "op-1"},
	}

	got, err := r.ResolveDeps([]string{"gamma", "alpha", "beta"})
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if got[0].Kind != RefSpecNode || got[0].SpecNodeID != "gamma" {
		t.Errorf("dep[0]: want ref:spec_node gamma, got %+v", got[0])
	}
	if got[1].Kind != RefOp || got[1].OpID != "op-1" {
		t.Errorf("dep[1]: want ref:op op-1, got %+v", got[1])
	}
	if got[2].Kind != RefBead || got[2].BeadID != "br-b" {
		t.Errorf("dep[2]: want ref:bead br-b, got %+v", got[2])
	}
}

// TestPriority_WalksImplementsToProjectRequirement covers the spec
// scenario: component implements R1 (preq → P1, priority 2) and R2
// (preq → P2, priority 1). Resolved priority is the minimum: 1.
func TestPriority_WalksImplementsToProjectRequirement(t *testing.T) {
	g := newFakeSpecGraph()
	g.components["comp1"] = Component{Implements: []string{"R1", "R2"}}
	g.moduleReqs["R1"] = ModuleRequirement{PreqID: "P1"}
	g.moduleReqs["R2"] = ModuleRequirement{PreqID: "P2"}
	g.projectReqs["P1"] = ProjectRequirement{Priority: intPtr(2)}
	g.projectReqs["P2"] = ProjectRequirement{Priority: intPtr(1)}

	r := &Resolver{SpecGraph: g}
	got := r.Priority("comp1")
	if got != 1 {
		t.Errorf("priority: want min=1, got %d", got)
	}
}

// TestPriority_FallbackWhenChainBroken covers the spec's fallback
// requirement: when the chain cannot be walked, FallbackPriority is
// returned. The validator is the authoritative gate for chain completeness;
// emit must not fail here.
func TestPriority_FallbackWhenChainBroken(t *testing.T) {
	g := newFakeSpecGraph()
	g.components["comp1"] = Component{Implements: []string{"R1"}}
	// R1 exists but its PreqID points nowhere.
	g.moduleReqs["R1"] = ModuleRequirement{PreqID: "Pmissing"}
	// Pmissing is not in projectReqs.

	r := &Resolver{SpecGraph: g}
	got := r.Priority("comp1")
	if got != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d, got %d", FallbackPriority, got)
	}
}

// TestPriority_FallbackWhenComponentMissing exercises the case where the
// component itself is not in the spec graph — same fallback applies.
func TestPriority_FallbackWhenComponentMissing(t *testing.T) {
	g := newFakeSpecGraph()
	r := &Resolver{SpecGraph: g}
	got := r.Priority("nonexistent")
	if got != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d for missing component, got %d", FallbackPriority, got)
	}
}

// TestPriority_FallbackWhenPreqIDEmpty covers the implements entry that
// resolves to a module requirement carrying no preq_id.
func TestPriority_FallbackWhenPreqIDEmpty(t *testing.T) {
	g := newFakeSpecGraph()
	g.components["comp1"] = Component{Implements: []string{"R1"}}
	g.moduleReqs["R1"] = ModuleRequirement{PreqID: ""}

	r := &Resolver{SpecGraph: g}
	got := r.Priority("comp1")
	if got != FallbackPriority {
		t.Errorf("priority: want FallbackPriority=%d, got %d", FallbackPriority, got)
	}
}

// TestPriority_SkipsBrokenChainPicksMinFromValidOnes ensures one bad
// implements entry does not prevent picking up a valid one.
func TestPriority_SkipsBrokenChainPicksMinFromValidOnes(t *testing.T) {
	g := newFakeSpecGraph()
	g.components["comp1"] = Component{Implements: []string{"Rgood", "Rbad"}}
	g.moduleReqs["Rgood"] = ModuleRequirement{PreqID: "Pgood"}
	g.moduleReqs["Rbad"] = ModuleRequirement{PreqID: "Pmissing"}
	g.projectReqs["Pgood"] = ProjectRequirement{Priority: intPtr(2)}

	r := &Resolver{SpecGraph: g}
	got := r.Priority("comp1")
	if got != 2 {
		t.Errorf("priority: want 2 (from Rgood), got %d", got)
	}
}

// TestPriority_FallbackWhenProjectReqHasNilPriority covers a project
// requirement record that exists but carries no priority field.
func TestPriority_FallbackWhenProjectReqHasNilPriority(t *testing.T) {
	g := newFakeSpecGraph()
	g.components["comp1"] = Component{Implements: []string{"R1"}}
	g.moduleReqs["R1"] = ModuleRequirement{PreqID: "P1"}
	g.projectReqs["P1"] = ProjectRequirement{Priority: nil}

	r := &Resolver{SpecGraph: g}
	got := r.Priority("comp1")
	if got != FallbackPriority {
		t.Errorf("priority: want FallbackPriority, got %d", got)
	}
}

// TestResolveParent_NewEpicInBatch covers the new-proposal path: there is
// no existing epic in the mapping store; ChangesetBuilder injected the
// synthetic "proposal/<ref>/epic" key into r.Batch pointing at the epic op_id.
func TestResolveParent_NewEpicInBatch(t *testing.T) {
	store := newFakeStore()
	r := &Resolver{
		MappingStore: store,
		Batch: map[string]string{
			"proposal/2026-04-foo/epic": "op-001",
		},
	}

	got, err := r.ResolveParent("2026-04-foo")
	if err != nil {
		t.Fatalf("ResolveParent: %v", err)
	}
	if got.Kind != RefOp || got.OpID != "op-001" {
		t.Errorf("parent: want ref:op op-001, got %+v", got)
	}
}

// TestResolveParent_ExistingEpicInMappingStore covers the re-run path:
// the mapping store already has an open epic record for the proposal, so
// the parent ref points at that bead instead of an in-batch op.
func TestResolveParent_ExistingEpicInMappingStore(t *testing.T) {
	store := newFakeStore()
	store.epic["2026-04-foo"] = mapping.Record{
		ID:         42,
		SpecNodeID: "2026-04-foo",
		BeadID:     "spexmachina-existing-epic",
		NodeType:   "proposal",
		BeadStatus: "open",
	}
	r := &Resolver{
		MappingStore: store,
		Batch:        map[string]string{},
	}

	got, err := r.ResolveParent("2026-04-foo")
	if err != nil {
		t.Fatalf("ResolveParent: %v", err)
	}
	if got.Kind != RefBead || got.BeadID != "spexmachina-existing-epic" {
		t.Errorf("parent: want ref:bead spexmachina-existing-epic, got %+v", got)
	}
}

// TestResolveParent_ErrorWhenNeither covers the impossible-state guard:
// neither an existing epic nor an in-batch synthetic key exists, so the
// caller has misused the Resolver. Surface a clear error.
func TestResolveParent_ErrorWhenNeither(t *testing.T) {
	store := newFakeStore()
	r := &Resolver{
		MappingStore: store,
		Batch:        map[string]string{},
	}

	_, err := r.ResolveParent("missing-proposal")
	if err == nil {
		t.Fatal("ResolveParent: want error for missing epic, got nil")
	}
}

// TestResolveParent_ExistingEpicBeatsBatchKey covers the re-run case:
// even if a synthetic batch key is present (defensive builder behavior),
// an existing epic record takes precedence so the run does not double-create.
func TestResolveParent_ExistingEpicBeatsBatchKey(t *testing.T) {
	store := newFakeStore()
	store.epic["proposal-X"] = mapping.Record{
		ID:         5,
		SpecNodeID: "proposal-X",
		BeadID:     "br-existing",
		NodeType:   "proposal",
		BeadStatus: "open",
	}
	r := &Resolver{
		MappingStore: store,
		Batch: map[string]string{
			"proposal/proposal-X/epic": "op-001",
		},
	}

	got, err := r.ResolveParent("proposal-X")
	if err != nil {
		t.Fatalf("ResolveParent: %v", err)
	}
	if got.Kind != RefBead || got.BeadID != "br-existing" {
		t.Errorf("parent: want ref:bead br-existing (mapping store wins), got %+v", got)
	}
}
