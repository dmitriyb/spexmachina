package emit

import (
	"testing"
)

// fakeSpecGraph is an in-memory SpecGraph for Resolver priority tests.
// Lookups miss when the requested key is not pre-loaded so callers can
// exercise the fallback paths explicitly.
type fakeSpecGraph struct {
	components  map[string]Component
	moduleReqs  map[string]ModuleRequirement
	projectReqs map[string]ProjectRequirement
	paths       map[string]NodePaths
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

// TODO(bead:spexmachina-y0wc.29): TestResolveDeps_*/TestResolveParent_*
// drove Resolver.ResolveDeps/ResolveParent against a fakeStore double for
// mapping.Store, retired by spexmachina-y0wc.19's migration of MappingStore
// onto the journal. Rewrite against the journal-backed
// MappingStore/FoldEntry per spec/emit/test_changeset_builder.md.

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
