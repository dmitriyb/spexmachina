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
		adj       map[int][]int
		wantCount int
	}{
		{
			name:      "no edges",
			adj:       map[int][]int{1: {}, 2: {}, 3: {}},
			wantCount: 0,
		},
		{
			name:      "linear chain",
			adj:       map[int][]int{1: {2}, 2: {3}, 3: {}},
			wantCount: 0,
		},
		{
			name:      "self loop",
			adj:       map[int][]int{1: {1}},
			wantCount: 1,
		},
		{
			name:      "two node cycle",
			adj:       map[int][]int{1: {2}, 2: {1}},
			wantCount: 1,
		},
		{
			name:      "diamond no cycle",
			adj:       map[int][]int{1: {2, 3}, 2: {4}, 3: {4}, 4: {}},
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycles := detectCycles(tt.adj)
			if len(cycles) != tt.wantCount {
				t.Fatalf("want %d cycles, got %d: %v", tt.wantCount, len(cycles), cycles)
			}
		})
	}
}
