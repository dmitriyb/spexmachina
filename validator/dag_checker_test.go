package validator

import (
	"path/filepath"
	"strings"
	"testing"
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
