package plan

import "github.com/dmitriyb/spexmachina/schema"

// SpecGraph is the current spec directory — project.json plus every
// module's module.json — indexed for the lookups ActionClassifier makes
// that the diff alone cannot answer: a test_section's current describes
// length, a component's uses edges, a module's requires_module edges, and
// a data_flow's uses edges (spec/plan/arch_action_classifier.md,
// "Node-Type Gate" and "DepSpecNodeIDs Collection"). PlanCommand loads it
// once per run from the spec directory and hands it down unchanged; it is
// otherwise a passive index, consulted, never mutated.
//
// A diff's ClassifiedChange.Module already carries the module's resolved
// name — spex diff joins it via merkle.ModuleNames before writing the
// document — so lookups from a change key off name. A module's own
// requires_module edges are identity hashes, so the transitive walk keys
// off id.
type SpecGraph struct {
	byName map[string]moduleEntry
	byID   map[string]moduleEntry
	// projectReqs indexes project.json's requirements by id — Resolver's
	// priority walk (implements -> preq_id -> priority) is the only reader;
	// nothing else in the package needs a project requirement's fields.
	projectReqs map[string]schema.Requirement
}

// moduleEntry pairs a module's project.json declaration (id, name,
// requires_module) with its parsed module.json contents.
type moduleEntry struct {
	Module schema.Module
	Spec   schema.ModuleSpec
}

// NewSpecGraph indexes a parsed project and its modules' specs, keyed by
// module identity hash exactly as the caller reads modules off disk.
func NewSpecGraph(proj schema.Project, specs map[string]schema.ModuleSpec) SpecGraph {
	g := SpecGraph{byName: map[string]moduleEntry{}, byID: map[string]moduleEntry{}, projectReqs: map[string]schema.Requirement{}}
	for _, mod := range proj.Modules {
		entry := moduleEntry{Module: mod, Spec: specs[mod.ID]}
		g.byID[mod.ID] = entry
		g.byName[mod.Name] = entry
	}
	for _, req := range proj.Requirements {
		g.projectReqs[req.ID] = req
	}
	return g
}

func (g SpecGraph) moduleByName(name string) (moduleEntry, bool) {
	e, ok := g.byName[name]
	return e, ok
}

func (g SpecGraph) moduleByID(id string) (moduleEntry, bool) {
	e, ok := g.byID[id]
	return e, ok
}

// projectRequirement resolves one project.json requirement by id — the
// last link in Resolver's implements -> preq_id -> priority chain.
func (g SpecGraph) projectRequirement(id string) (schema.Requirement, bool) {
	r, ok := g.projectReqs[id]
	return r, ok
}

func findComponent(spec schema.ModuleSpec, id string) (schema.Component, bool) {
	for _, c := range spec.Components {
		if c.ID == id {
			return c, true
		}
	}
	return schema.Component{}, false
}

func findDataFlow(spec schema.ModuleSpec, id string) (schema.DataFlow, bool) {
	for _, f := range spec.DataFlows {
		if f.ID == id {
			return f, true
		}
	}
	return schema.DataFlow{}, false
}

func findTestSection(spec schema.ModuleSpec, id string) (schema.TestSection, bool) {
	for _, ts := range spec.TestSections {
		if ts.ID == id {
			return ts, true
		}
	}
	return schema.TestSection{}, false
}

func findModuleRequirement(spec schema.ModuleSpec, id string) (schema.ModuleRequirement, bool) {
	for _, r := range spec.Requirements {
		if r.ID == id {
			return r, true
		}
	}
	return schema.ModuleRequirement{}, false
}
