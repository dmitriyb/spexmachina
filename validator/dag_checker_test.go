package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/internal/perf"
	"github.com/dmitriyb/spexmachina/schema"
)

// REQ-3: DAG checking — build dependency graphs from module, requirement, and
// component references, detect cycles, report full cycle paths.

func TestREQ3_ValidDAGReturnsNoErrors(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid DAG, got %d: %v", len(errs), errs)
	}
}

func TestREQ3_ModuleDependencyCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_module_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected cycle error for module dependencies, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "module dependency cycle") {
			found = true
			if !strings.Contains(e.Message, "alpha") || !strings.Contains(e.Message, "beta") {
				t.Fatalf("cycle path should mention both modules, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected module dependency cycle error, got: %v", errs)
	}
}

func TestREQ3_RequirementDependencyCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_req_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected cycle error for requirement dependencies, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "requirement dependency cycle") {
			found = true
			if !strings.Contains(e.Path, "core/module.json") {
				t.Fatalf("expected path to reference core/module.json, got: %s", e.Path)
			}
		}
	}
	if !found {
		t.Fatalf("expected requirement dependency cycle error, got: %v", errs)
	}
}

func TestREQ3_ComponentDependencyCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_comp_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected cycle error for component dependencies, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "component dependency cycle") {
			found = true
			if !strings.Contains(e.Message, "Parser") || !strings.Contains(e.Message, "Lexer") {
				t.Fatalf("cycle path should mention both components, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected component dependency cycle error, got: %v", errs)
	}
}

func TestREQ3_CyclePathIncludesAllNodes(t *testing.T) {
	// The 3-node requirement cycle should include all three titles in the path.
	errs := CheckDAG(filepath.Join("testdata", "dag_req_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected cycle errors, got none")
	}
	msg := errs[0].Message
	for _, title := range []string{"Feature A", "Feature B", "Feature C"} {
		if !strings.Contains(msg, title) {
			t.Fatalf("cycle path should include %q, got: %s", title, msg)
		}
	}
}

func TestREQ3_AllDAGErrorsTagged(t *testing.T) {
	dirs := []string{"dag_module_cycle", "dag_req_cycle", "dag_comp_cycle"}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			errs := CheckDAG(filepath.Join("testdata", dir))
			for _, e := range errs {
				if e.Check != "dag" {
					t.Fatalf("expected check=dag, got %q for error: %v", e.Check, e)
				}
				if e.Severity != "error" {
					t.Fatalf("expected severity=error, got %q for error: %v", e.Severity, e)
				}
			}
		})
	}
}

// D9: Acyclicity over profile-declared edges — the DAG check enforces
// acyclicity over every edge kind the resolved profile declares that is not
// marked cyclic: true, not over a fixed edge set. A profile declaring an
// "endpoint" type carrying a "serves" edge to other endpoints exercises that
// with a graph the three built-in fast paths never walk.
func TestREQ3_ProfileDeclaredEdgeCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_profile_edge_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected a cycle error for the profile-declared serves edge, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "serves cycle") {
			found = true
			if !strings.Contains(e.Message, "Get widget") || !strings.Contains(e.Message, "Give widget") {
				t.Fatalf("cycle path should name both endpoints, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected a serves cycle error, got: %v", errs)
	}
}

// D9 variant: the same loop, but the profile marks the serves edge
// cyclic: true — a cyclic edge is descriptive, exempt from the cycle check,
// so the same data validates with zero DAG errors.
func TestREQ3_ProfileDeclaredEdgeCyclicExempt(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_profile_edge_cyclic_exempt"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for a cyclic-exempt profile-declared edge, got %d: %v", len(errs), errs)
	}
}

// A built-in edge kind (requires_module) marked cyclic: true in the resolved
// profile is exempt from the cycle check the same way a profile-declared
// edge is: the flag applies uniformly, not only to graphs built generically.
func TestREQ3_BuiltinEdgeCyclicExempt(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_profile_builtin_exempt"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors when requires_module is marked cyclic: true, got %d: %v", len(errs), errs)
	}
}

// A profile that declares none of requires_module, depends_on or uses must
// not have the built-in module/requirement/component graphs built and
// walked anyway: the checker holds no fixed edge list of its own, so an
// edge kind the resolved profile omits entirely gets no graph, the same as
// one it declares cyclic: true.
func TestREQ3_BuiltinEdgeUndeclaredNotChecked(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_profile_omits_builtin_edges"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors when the profile declares none of requires_module, depends_on or uses, got %d: %v", len(errs), errs)
	}
}

// D9 variant: a profile-declared uses edge sourced from data_flow, not
// component. checkComponentDAG's built-in fast path only walks uses from
// components, so builtinDAGEdgeCoverage must not claim data_flow as covered
// by it — otherwise a data_flow-to-data_flow uses cycle is excluded from
// the generic path and walked by nothing at all.
func TestREQ3_ProfileDeclaredDataFlowUsesEdgeCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_profile_data_flow_uses_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected a cycle error for the data_flow-sourced uses edge, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "uses cycle") {
			found = true
			if !strings.Contains(e.Path, "alpha/module.json:/data_flows") {
				t.Fatalf("expected path to reference alpha/module.json:/data_flows, got: %s", e.Path)
			}
			if !strings.Contains(e.Message, "Flow A") || !strings.Contains(e.Message, "Flow B") {
				t.Fatalf("cycle path should name both data flows, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected a uses cycle error, got: %v", errs)
	}
}

// The default profile declares "requirement" at both scopes under one
// shared depends_on edge: checkRequirementDAG only walks each module's own
// requirements, so a cycle among project.json's top-level requirements must
// fall through to the generic path rather than be dropped alongside the
// module-scoped occurrence builtinDAGEdgeCoverage marks covered.
func TestREQ3_ProjectRequirementDependencyCycle(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "dag_project_req_cycle"))
	if len(errs) == 0 {
		t.Fatal("expected a cycle error for project-scoped requirement dependencies, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "dag" {
			t.Fatalf("expected check=dag, got %q", e.Check)
		}
		if strings.Contains(e.Message, "depends_on cycle") {
			found = true
			if e.Path != "project.json:/requirements" {
				t.Fatalf("expected path project.json:/requirements, got: %s", e.Path)
			}
			if !strings.Contains(e.Message, "Project requirement A") || !strings.Contains(e.Message, "Project requirement B") {
				t.Fatalf("cycle path should name both project requirement titles, got: %s", e.Message)
			}
		}
	}
	if !found {
		t.Fatalf("expected a depends_on cycle error, got: %v", errs)
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly one cycle error (no double-report from a module-scope fast path), got %d: %v", len(errs), errs)
	}
}

// E1: Module with empty arrays — no requirements, components or
// test_sections means no edges to form cycles. The IDValidator half of the
// same Given is TestREQ5_EmptyArraysReturnsEmpty in id_validator_test.go.
func TestREQ3_EmptyArraysNoErrors(t *testing.T) {
	errs := CheckDAG(filepath.Join("testdata", "empty_arrays"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for module with empty arrays, got %d: %v", len(errs), errs)
	}
}

// E3: Large graph performance — 50 modules, each with 20 requirements and 10
// components, forming a deep but acyclic dependency chain, completes in
// under 100ms and returns zero errors.
func TestREQ3_LargeGraphPerformance(t *testing.T) {
	const numModules = 50
	const numRequirements = 20
	const numComponents = 10

	dir := t.TempDir()
	proj := schema.Project{Name: "perf-dag-test"}

	var prevModuleID string
	for m := 0; m < numModules; m++ {
		modName := fmt.Sprintf("module%03d", m)
		modID := schema.IdentityHash("module", modName)
		mod := schema.Module{
			ID:   modID,
			Name: modName,
			Path: modName,
		}
		if prevModuleID != "" {
			mod.RequiresModule = []string{prevModuleID}
		}
		proj.Modules = append(proj.Modules, mod)
		prevModuleID = modID

		spec := schema.ModuleSpec{Name: modName}

		var prevReqID string
		for r := 0; r < numRequirements; r++ {
			reqName := fmt.Sprintf("req%03d", r)
			reqID := schema.IdentityHash(modName, "requirement", reqName)
			req := schema.ModuleRequirement{
				ID:     reqID,
				PreqID: schema.IdentityHash("project", "requirement", "root"),
				Type:   "functional",
				Title:  reqName,
			}
			if prevReqID != "" {
				req.DependsOn = []string{prevReqID}
			}
			spec.Requirements = append(spec.Requirements, req)
			prevReqID = reqID
		}

		var prevCompID string
		for c := 0; c < numComponents; c++ {
			compName := fmt.Sprintf("comp%03d", c)
			compID := schema.IdentityHash(modName, "component", compName)
			comp := schema.Component{
				ID:   compID,
				Name: compName,
			}
			if prevCompID != "" {
				comp.Uses = []string{prevCompID}
			}
			spec.Components = append(spec.Components, comp)
			prevCompID = compID
		}

		modDir := filepath.Join(dir, modName)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		specData, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal module: %v", err)
		}
		writeFile(t, modDir, "module.json", string(specData))
	}

	proj.Requirements = []schema.Requirement{{
		ID:    schema.IdentityHash("project", "requirement", "root"),
		Type:  "functional",
		Title: "root",
	}}

	projData, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}
	writeProject(t, dir, string(projData))

	var errs []ValidationError
	perf.Within(t, 100*time.Millisecond, func() {
		errs = CheckDAG(dir)
	})

	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ3_SelfValidateDAG(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckDAG(specDir)
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should have no DAG cycles, got %d errors: %v", len(errs), errs)
	}
}

func TestREQ3_DetectCyclesUnit(t *testing.T) {
	tests := []struct {
		name      string
		adj       map[string][]string
		wantCount int
	}{
		{
			name:      "no edges",
			adj:       map[string][]string{"a": {}, "b": {}, "c": {}},
			wantCount: 0,
		},
		{
			name:      "linear chain",
			adj:       map[string][]string{"a": {"b"}, "b": {"c"}, "c": {}},
			wantCount: 0,
		},
		{
			name:      "self loop",
			adj:       map[string][]string{"a": {"a"}},
			wantCount: 1,
		},
		{
			name:      "two node cycle",
			adj:       map[string][]string{"a": {"b"}, "b": {"a"}},
			wantCount: 1,
		},
		{
			name:      "diamond no cycle",
			adj:       map[string][]string{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}, "d": {}},
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycles := detectStringCycles(tt.adj)
			if len(cycles) != tt.wantCount {
				t.Fatalf("want %d cycles, got %d: %v", tt.wantCount, len(cycles), cycles)
			}
		})
	}
}
