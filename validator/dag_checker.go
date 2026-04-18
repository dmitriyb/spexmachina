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

// CheckDAG builds dependency graphs from the spec and checks each for cycles.
// It checks three graph types:
//  1. Module dependency graph (project-wide): edges from requires_module
//  2. Requirement dependency graph (per module): edges from depends_on
//  3. Component dependency graph (per module): edges from uses
func CheckDAG(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "dag")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError
	result = append(result, checkModuleDAG(project)...)

	modNames := make([]string, 0, len(modules))
	for name := range modules {
		modNames = append(modNames, name)
	}
	slices.Sort(modNames)

	for _, modName := range modNames {
		mod := modules[modName]
		result = append(result, checkRequirementDAG(modName, mod)...)
		result = append(result, checkComponentDAG(modName, mod)...)
	}

	return result
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
