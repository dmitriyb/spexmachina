package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-1: JSON schema conformance — validate project.json and module.json
// against their JSON Schemas. Report all violations, not just the first.

func TestREQ1_ValidSpecReturnsNoErrors(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors for valid spec, got %d: %v", len(errs), errs)
	}
}

func TestREQ1_MissingRequiredProjectField(t *testing.T) {
	// project.json is missing required "name" field.
	errs := CheckSchema(filepath.Join("testdata", "missing_name"))
	if len(errs) == 0 {
		t.Fatal("expected errors for missing project name, got none")
	}
	found := false
	for _, e := range errs {
		if e.Check != "schema" {
			t.Fatalf("expected check=schema, got %q", e.Check)
		}
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
		if strings.Contains(e.Path, "project.json") && strings.Contains(e.Message, "name") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an error about missing 'name' in project.json, got: %v", errs)
	}
}

func TestREQ1_InvalidModuleJSON(t *testing.T) {
	// module.json has a component missing required "id" field.
	errs := CheckSchema(filepath.Join("testdata", "bad_module"))
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid module.json, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error referencing core/module.json, got: %v", errs)
	}
}

func TestREQ1_MissingModuleJSONFile(t *testing.T) {
	// project.json references a module whose module.json does not exist.
	errs := CheckSchema(filepath.Join("testdata", "missing_module_json"))
	if len(errs) == 0 {
		t.Fatal("expected errors for missing module.json file, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json") && strings.Contains(e.Message, "read file") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected read error for core/module.json, got: %v", errs)
	}
}

func TestREQ1_MultipleViolationsReported(t *testing.T) {
	// project.json has multiple violations: missing "name", bad id type, missing module name.
	errs := CheckSchema(filepath.Join("testdata", "multi_error"))
	if len(errs) < 2 {
		t.Fatalf("expected multiple errors, got %d: %v", len(errs), errs)
	}
}

func TestREQ1_MissingProjectJSON(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "nonexistent"))
	if len(errs) == 0 {
		t.Fatal("expected errors for missing project.json, got none")
	}
	if errs[0].Check != "schema" {
		t.Fatalf("expected check=schema, got %q", errs[0].Check)
	}
	if !strings.Contains(errs[0].Message, "read file") {
		t.Fatalf("expected read file error, got: %s", errs[0].Message)
	}
}

func TestREQ1_AllErrorsHaveSchemaCheck(t *testing.T) {
	// Verify all errors from CheckSchema are tagged with check="schema".
	dirs := []string{"missing_name", "bad_module", "multi_error"}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			errs := CheckSchema(filepath.Join("testdata", dir))
			for _, e := range errs {
				if e.Check != "schema" {
					t.Fatalf("expected check=schema, got %q for error: %v", e.Check, e)
				}
				if e.Severity != "error" {
					t.Fatalf("expected severity=error, got %q for error: %v", e.Severity, e)
				}
			}
		})
	}
}

func TestREQ9_SelfValidate(t *testing.T) {
	// Validate spex-machina's own spec directory.
	specDir := filepath.Join("..", "spec")
	errs := CheckSchema(specDir)
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should be valid, got %d errors: %v", len(errs), errs)
	}
}
