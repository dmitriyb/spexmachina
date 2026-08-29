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

// S5: module.json with malformed identity-hash id — every id field is a
// string constrained by the identity-hash pattern, so a value that doesn't
// match it is reported as a pattern violation, not a type mismatch.

func TestS5_MalformedIdentityHashID(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "malformed_id"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if !strings.Contains(e.Path, "requirements/0/id") {
		t.Fatalf("expected path referencing requirements/0/id, got %q", e.Path)
	}
}

// S6: project.json with an unknown extra field — the schema forbids
// additional properties at the root, so the extra field is one violation.

func TestS6_UnknownExtraFieldRejected(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "extra_field"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if e.Path != "project.json" {
		t.Fatalf("expected path %q, got %q", "project.json", e.Path)
	}
}

// E2: project.json is not valid JSON — a parse failure, not a panic, and no
// module.json is opened since the module list can't be discovered.

func TestE2_ProjectJSONParseFailure(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "invalid_json"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if e.Path != "project.json" {
		t.Fatalf("expected path %q, got %q", "project.json", e.Path)
	}
	if !strings.Contains(e.Message, "invalid JSON") {
		t.Fatalf("expected a JSON parse failure message, got %q", e.Message)
	}
}

// S16: conformance runs against the profile-composed schemas — a profile
// declaring an additional module-scoped type is accepted, and the same
// module.json fails under the default profile once the profile is removed.

func TestS16_ConformanceAgainstProfileComposedSchema(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "schema_profile_endpoint"))
	if len(errs) != 0 {
		t.Fatalf("expected no errors with the profile declaring 'endpoint', got %d: %v", len(errs), errs)
	}
}

func TestS16_SameModuleFailsUnderDefaultProfile(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "schema_profile_endpoint_no_profile"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error without the profile, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if !strings.Contains(e.Path, "alpha/module.json") {
		t.Fatalf("expected path referencing alpha/module.json, got %q", e.Path)
	}
}

// S17: a malformed profile fails before any conformance check runs — one
// error naming profile.json, and zero schema-conformance errors even though
// the baseline project.json and module.json are otherwise valid.

func TestS17_MalformedProfileFailsBeforeConformance(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "schema_profile_malformed"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if e.Path != "profile.json" {
		t.Fatalf("expected path %q, got %q", "profile.json", e.Path)
	}
}

// S4: violations in project.json and in a module.json are both reported —
// not just the first file examined.

func TestS4_MultipleViolationsAcrossFiles(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "s4_multi_file_errors"))
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, one per file, got %d: %v", len(errs), errs)
	}
	var sawProject, sawModule bool
	for _, e := range errs {
		if e.Path == "project.json" {
			sawProject = true
		}
		if strings.Contains(e.Path, "alpha/module.json") {
			sawModule = true
		}
	}
	if !sawProject {
		t.Fatalf("expected an error referencing project.json, got: %v", errs)
	}
	if !sawModule {
		t.Fatalf("expected an error referencing alpha/module.json, got: %v", errs)
	}
}

// S15: schema checks run across multiple modules with per-module error
// attribution — alpha is fully valid, beta carries a schema violation, and
// the errors returned name only the module that actually violates the schema.

func TestS15_SchemaChecksAcrossMultipleModules(t *testing.T) {
	errs := CheckSchema(filepath.Join("testdata", "schema_multi_module"))
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "schema" {
		t.Fatalf("expected check=schema, got %q", e.Check)
	}
	if !strings.Contains(e.Path, "beta/module.json") {
		t.Fatalf("expected error referencing beta/module.json, got %q", e.Path)
	}
	for _, e := range errs {
		if strings.Contains(e.Path, "alpha") {
			t.Fatalf("expected no error referencing alpha, got: %v", errs)
		}
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
