package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// builtinDAGEdgeCoverage is the (kind, from-type) pairs checkModuleDAG,
// checkRequirementDAG and checkComponentDAG already resolve by name, each
// mapped to the scope its fast path actually walks — not every scope the
// from-type name might be declared at. "uses" carries two from-types under
// the default profile (component, data_flow), but checkComponentDAG walks
// uses from components only, so this map names component alone; a
// data_flow-sourced uses edge is left to the generic path even under the
// default profile, where it is vacuous (data_flow.uses targets component,
// which cannot close a loop back to a data_flow) but is not vacuous under a
// profile that declares a data_flow-to-data_flow uses edge. "requirement" is
// scoped to "module" because checkRequirementDAG walks each module's own
// requirements only: the default profile declares "requirement" at project
// scope too (project.json's top-level requirements, see I11), and that
// occurrence's depends_on cycles are walked by nothing built-in, so they
// must fall through to the generic path rather than be silently dropped
// alongside the module-scoped occurrence that shares the same name. A
// profile-declared edge covers a (kind, from-type, scope) triple outside
// this set — a new edge kind entirely, a from-type added to a kind these
// three don't already walk, or a from-type's occurrence at a scope the fast
// path doesn't reach — and is checked generically instead, via
// checkExtraModuleDAGEdges or checkExtraProjectDAGEdges depending on the
// from-type's scope. This mirrors builtinEdgeCoverage in id_validator.go,
// which partitions the same way for reference-integrity checking; the DAG
// checker's own set is narrower because only three of the default profile's
// seven edge kinds can close a loop (the other four connect node types that
// never point back at each other) — see arch_dag_checker.md "Graphs
// Checked".
var builtinDAGEdgeCoverage = map[string]map[string]string{
	"requires_module": {"module": "project"},
	"depends_on":      {"requirement": "module"},
	"uses":            {"component": "module"},
}

// CheckDAG builds dependency graphs from the spec and checks each for
// cycles. The graphs checked are the edge kinds the resolved profile
// declares, minus those marked cyclic: true. Three built-in graphs cover the
// edge kinds that can actually close a loop under the default profile:
//  1. Module dependency graph (project-wide): edges from requires_module
//  2. Requirement dependency graph (per module): edges from depends_on
//  3. Component dependency graph (per module): edges from uses
//
// Any other edge kind the resolved profile declares — new, or an added
// from-type on one of the three kinds above — is built and walked by the
// same cycle-detection machinery via checkExtraModuleDAGEdges and
// checkExtraProjectDAGEdges; the checker holds no fixed edge list of its
// own beyond these three fast paths.
func CheckDAG(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "dag")
	if len(errs) > 0 {
		return errs
	}

	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "dag",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}
	}

	var result []ValidationError
	if edgeActive(profile, "requires_module", "module") {
		result = append(result, checkModuleDAG(project)...)
	}

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	reqActive := edgeActive(profile, "depends_on", "requirement")
	usesActive := edgeActive(profile, "uses", "component")
	for _, modName := range modNames {
		mod := modules[modName]
		if reqActive {
			result = append(result, checkRequirementDAG(modName, mod)...)
		}
		if usesActive {
			result = append(result, checkComponentDAG(modName, mod)...)
		}
	}

	result = append(result, checkExtraModuleDAGEdges(specDir, modNames, project, modules, profile)...)
	result = append(result, checkExtraProjectDAGEdges(specDir, modNames, project, modules, profile)...)

	return result
}

// edgeActive reports whether a built-in fast path should build and walk its
// graph: the resolved profile must declare the given edge kind carried from
// the given from-type, and that from-type's own declaration must not be
// marked cyclic: true — checked per from-type, since two types declaring the
// same-named field can set the flag differently. An edge kind the profile
// does not declare for that from-type at all is inactive — the checker
// holds no fixed edge list of its own, so an undeclared (kind, from-type)
// pair is skipped here exactly as it would be by the generic
// checkExtra*DAGEdges path, rather than walked anyway using the built-in
// struct field regardless of what the profile says.
func edgeActive(profile *schema.Profile, kind, fromType string) bool {
	for _, e := range profile.Edges {
		if e.Kind != kind {
			continue
		}
		if !slices.Contains(e.From, fromType) {
			continue
		}
		return !e.CyclicForType(fromType)
	}
	return false
}

// extraCycleEdgesForScope returns the resolved profile's declared edges,
// reduced to the (kind, from-type) pairs not already covered, at the given
// scope, by one of the three built-in graphs — with any from-type whose own
// declaration is marked cyclic: true dropped from that edge's From list,
// since that occurrence closes no graph at all, built-in coverage aside; a
// sibling from-type sharing the edge kind but not marked cyclic is kept, so
// one type's exemption never silently drops another's cycle check. Scoping
// the reduction matters because a from-type name like "requirement" can be
// declared at both scopes under one shared edge declaration: filtering on
// name alone would drop the project-scoped occurrence too, even though only
// the module-scoped one is actually walked by checkRequirementDAG. Called
// once per scope by checkExtraModuleDAGEdges and checkExtraProjectDAGEdges;
// a from-type this leaves in that has no occurrence at the caller's scope is
// simply skipped downstream by findModuleNodeType/projectEdgeSourceKey, so
// over-including here is safe.
func extraCycleEdgesForScope(profile *schema.Profile, scope string) []schema.Edge {
	var out []schema.Edge
	for _, e := range profile.Edges {
		covered := builtinDAGEdgeCoverage[e.Kind]
		var from []string
		for _, f := range e.From {
			if e.CyclicForType(f) {
				continue
			}
			if covered[f] == scope {
				continue
			}
			from = append(from, f)
		}
		if len(from) == 0 {
			continue
		}
		extra := e
		extra.From = from
		out = append(out, extra)
	}
	return out
}

// checkExtraModuleDAGEdges builds and walks a cycle graph for each
// (edge kind, from-type) pair extraCycleEdgesForScope returns for module
// scope, one graph per module — mirroring checkRequirementDAG and
// checkComponentDAG's own per-module scoping. Nodes and edges are read
// generically off module.json, since a profile-declared type carries no
// dedicated Go field; an edge target that does not name another node in the
// same array is dropped rather than reported here, because it cannot close
// a loop within this graph and cross-reference integrity is CheckIDs' job,
// not this checker's.
func checkExtraModuleDAGEdges(specDir string, modNames []string, project *schema.Project, modules map[string]*schema.ModuleSpec, profile *schema.Profile) []ValidationError {
	edges := extraCycleEdgesForScope(profile, "module")
	if len(edges) == 0 {
		return nil
	}

	var errs []ValidationError
	for _, modName := range modNames {
		mod := modules[modName]
		if mod == nil {
			continue
		}
		modPath := modulePathByName(project, modName)

		// One read+decode of module.json per module, however many
		// (edge, from-type) pairs below ask it for entries: the large-graph
		// perf budget doesn't survive one full file re-read per pair.
		doc, err := loadRawDoc(filepath.Join(specDir, modPath, "module.json"))
		if err != nil {
			continue
		}

		for _, edge := range edges {
			for _, fromName := range edge.From {
				nt, ok := findModuleNodeType(profile, fromName)
				if !ok {
					continue
				}
				entries, err := doc.entries(nt.PluralKey)
				if err != nil || len(entries) == 0 {
					continue
				}
				adj, labels := genericCycleAdjacency(entries, edge.Kind)
				path := modName + "/module.json:/" + nt.PluralKey
				errs = append(errs, reportCycles(detectStringCycles(adj), labels, edge.Kind, path)...)
			}
		}
	}
	return errs
}

// checkExtraProjectDAGEdges is checkExtraModuleDAGEdges' project-scoped
// counterpart: it builds and walks a cycle graph, once for the whole
// project, for each (edge kind, from-type) pair extraCycleEdgesForScope
// returns for project scope — a profile-declared project-scoped type, or a
// project-scoped occurrence of a from-type name whose module-scoped
// occurrence one of the three built-in fast paths already covers (e.g.
// "requirement": module-scoped requirements are checkRequirementDAG's, but
// the default profile's project-scoped requirements, with their own
// depends_on edges, are not). projectEdgeSourceKey (declared in
// id_validator.go) resolves both the same way checkExtraProjectEdges does
// for reference-integrity checking.
func checkExtraProjectDAGEdges(specDir string, modNames []string, project *schema.Project, modules map[string]*schema.ModuleSpec, profile *schema.Profile) []ValidationError {
	edges := extraCycleEdgesForScope(profile, "project")
	if len(edges) == 0 {
		return nil
	}

	doc, err := loadRawDoc(filepath.Join(specDir, "project.json"))
	if err != nil {
		return nil
	}

	var errs []ValidationError
	for _, edge := range edges {
		for _, fromName := range edge.From {
			pluralKey, ok := projectEdgeSourceKey(profile, fromName)
			if !ok {
				continue
			}
			entries, err := doc.entries(pluralKey)
			if err != nil || len(entries) == 0 {
				continue
			}
			adj, labels := genericCycleAdjacency(entries, edge.Kind)
			path := "project.json:/" + pluralKey
			errs = append(errs, reportCycles(detectStringCycles(adj), labels, edge.Kind, path)...)
		}
	}
	return errs
}

// rawDocCache holds one JSON document's top-level fields, decoded once, so
// repeated lookups of different array keys (one per (edge, from-type) pair
// checkExtraModuleDAGEdges or checkExtraProjectDAGEdges is asked about) cost
// a cheap map lookup plus a small array unmarshal instead of a fresh
// os.ReadFile and full-document json.Unmarshal apiece.
type rawDocCache struct {
	top map[string]json.RawMessage
}

// loadRawDoc reads and top-level-decodes the JSON document at path.
func loadRawDoc(path string) (*rawDocCache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	return &rawDocCache{top: top}, nil
}

// entries decodes the array at pluralKey, if present. A document carrying
// no such array yields no entries and no error, matching rawArrayEntries.
func (c *rawDocCache) entries(pluralKey string) ([]rawEntry, error) {
	arr, ok := c.top[pluralKey]
	if !ok {
		return nil, nil
	}
	var entries []rawEntry
	if err := json.Unmarshal(arr, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// genericCycleAdjacency turns a slice of generically-read entries into an
// adjacency map keyed by identity hash, and a parallel id-to-label map used
// to spell a found cycle out by name. An edge whose target id is not itself
// an entry in the same slice is dropped: it cannot participate in a cycle
// confined to this graph. A node's declared-identity field is "name" for
// every built-in type now, project requirement included —
// project.schema.json's required ["id", "type", "name"] carries no "title"
// property at all since the title-to-name rename — so the "title" fallback
// below no longer has a built-in type to serve; it stays only as tolerant
// handling of a node entry that carries "title" anyway, ahead of the final
// fallback to the raw id.
func genericCycleAdjacency(entries []rawEntry, edgeKind string) (map[string][]string, map[string]string) {
	ids := make(map[string]bool, len(entries))
	labels := make(map[string]string, len(entries))
	for _, e := range entries {
		id := e.str("id")
		ids[id] = true
		switch {
		case e.str("name") != "":
			labels[id] = e.str("name")
		case e.str("title") != "":
			labels[id] = e.str("title")
		default:
			labels[id] = id
		}
	}

	adj := make(map[string][]string, len(entries))
	for _, e := range entries {
		id := e.str("id")
		var targets []string
		for _, target := range e.strSlice(edgeKind) {
			if ids[target] {
				targets = append(targets, target)
			}
		}
		adj[id] = targets
	}
	return adj, labels
}

// reportCycles turns detectStringCycles' output into validation entries,
// naming the edge kind the graph was built from and spelling the cycle out
// by the labels map — the same shape checkModuleDAG, checkRequirementDAG
// and checkComponentDAG produce for the three built-in graphs.
func reportCycles(cycles [][]string, labels map[string]string, edgeKind, path string) []ValidationError {
	var errs []ValidationError
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, id := range cycle {
			names[i] = labels[id]
		}
		errs = append(errs, ValidationError{
			Check:    "dag",
			Severity: "error",
			Path:     path,
			Message:  fmt.Sprintf("%s cycle: %s", edgeKind, strings.Join(names, " -> ")),
		})
	}
	return errs
}

// loadSpec reads project.json and all referenced module.json files, returning
// typed structures for validation. The check parameter is used to tag any
// load/parse errors with the correct checker name.
func loadSpec(specDir, check string) (*schema.Project, map[string]*schema.ModuleSpec, []ValidationError) {
	projPath := filepath.Join(specDir, "project.json")
	projData, err := os.ReadFile(projPath)
	if err != nil {
		return nil, nil, []ValidationError{{
			Check:    check,
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("read file: %s", err),
		}}
	}

	var project schema.Project
	if err := json.Unmarshal(projData, &project); err != nil {
		return nil, nil, []ValidationError{{
			Check:    check,
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("parse JSON: %s", err),
		}}
	}

	modules := make(map[string]*schema.ModuleSpec, len(project.Modules))
	var errs []ValidationError
	for _, mod := range project.Modules {
		modPath := filepath.Join(specDir, mod.Path, "module.json")
		modData, err := os.ReadFile(modPath)
		if err != nil {
			errs = append(errs, ValidationError{
				Check:    check,
				Severity: "error",
				Path:     mod.Path + "/module.json",
				Message:  fmt.Sprintf("read file: %s", err),
			})
			continue
		}
		var modSpec schema.ModuleSpec
		if err := json.Unmarshal(modData, &modSpec); err != nil {
			errs = append(errs, ValidationError{
				Check:    check,
				Severity: "error",
				Path:     mod.Path + "/module.json",
				Message:  fmt.Sprintf("parse JSON: %s", err),
			})
			continue
		}
		modules[mod.Name] = &modSpec
	}
	if len(errs) > 0 {
		return nil, nil, errs
	}

	return &project, modules, nil
}

// checkModuleDAG checks the module dependency graph for cycles.
// Nodes are module identity hashes, edges come from requires_module.
func checkModuleDAG(project *schema.Project) []ValidationError {
	idToName := make(map[string]string, len(project.Modules))
	adj := make(map[string][]string, len(project.Modules))
	for _, mod := range project.Modules {
		idToName[mod.ID] = mod.Name
		adj[mod.ID] = mod.RequiresModule
	}

	cycles := detectStringCycles(adj)
	var errs []ValidationError
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, id := range cycle {
			names[i] = idToName[id]
		}
		errs = append(errs, ValidationError{
			Check:    "dag",
			Severity: "error",
			Path:     "project.json:/modules",
			Message:  fmt.Sprintf("module dependency cycle: %s", strings.Join(names, " -> ")),
		})
	}
	return errs
}

// checkRequirementDAG checks the requirement dependency graph for a single module.
// Nodes are requirement IDs, edges come from depends_on.
func checkRequirementDAG(modName string, mod *schema.ModuleSpec) []ValidationError {
	adj := make(map[string][]string, len(mod.Requirements))
	idToTitle := make(map[string]string, len(mod.Requirements))
	for _, req := range mod.Requirements {
		idToTitle[req.ID] = req.Title
		adj[req.ID] = req.DependsOn
	}

	cycles := detectStringCycles(adj)
	var errs []ValidationError
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, id := range cycle {
			names[i] = idToTitle[id]
		}
		errs = append(errs, ValidationError{
			Check:    "dag",
			Severity: "error",
			Path:     modName + "/module.json:/requirements",
			Message:  fmt.Sprintf("requirement dependency cycle: %s", strings.Join(names, " -> ")),
		})
	}
	return errs
}

// checkComponentDAG checks the component uses graph for a single module.
// Nodes are component IDs, edges come from uses.
func checkComponentDAG(modName string, mod *schema.ModuleSpec) []ValidationError {
	adj := make(map[string][]string, len(mod.Components))
	idToName := make(map[string]string, len(mod.Components))
	for _, comp := range mod.Components {
		idToName[comp.ID] = comp.Name
		adj[comp.ID] = comp.Uses
	}

	cycles := detectStringCycles(adj)
	var errs []ValidationError
	for _, cycle := range cycles {
		names := make([]string, len(cycle))
		for i, id := range cycle {
			names[i] = idToName[id]
		}
		errs = append(errs, ValidationError{
			Check:    "dag",
			Severity: "error",
			Path:     modName + "/module.json:/components",
			Message:  fmt.Sprintf("component dependency cycle: %s", strings.Join(names, " -> ")),
		})
	}
	return errs
}

// color constants for three-color DFS marking.
const (
	white = 0 // unvisited
	gray  = 1 // in current DFS stack
	black = 2 // fully explored
)

// detectStringCycles finds all cycles in a directed graph of identity-hash
// string IDs using DFS with three-color marking. Returns each cycle as a
// slice of IDs forming the cycle path (ending with the repeated start node).
func detectStringCycles(adj map[string][]string) [][]string {
	color := make(map[string]int, len(adj))
	parent := make(map[string]string, len(adj))
	var cycles [][]string

	keys := make([]string, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		for _, neighbor := range adj[node] {
			switch color[neighbor] {
			case white:
				parent[neighbor] = node
				dfs(neighbor)
			case gray:
				var path []string
				for n := node; n != neighbor; n = parent[n] {
					path = append(path, n)
				}
				path = append(path, neighbor)
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				path = append(path, neighbor)
				cycles = append(cycles, path)
			}
		}
		color[node] = black
	}

	for _, node := range keys {
		if color[node] == white {
			dfs(node)
		}
	}

	return cycles
}
