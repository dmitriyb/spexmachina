package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileProjectSchema compiles the embedded project schema for validation.
func compileProjectSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal project schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("project.schema.json", doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("project.schema.json")
	if err != nil {
		t.Fatalf("compile project schema: %v", err)
	}
	return sch
}

// validateProject validates a JSON document against the project schema.
// Returns nil on success, error on validation failure.
func validateProject(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

func TestFR1_S1_MinimalProjectPasses(t *testing.T) {
	sch := compileProjectSchema(t)
	data := readTestdata(t, "minimal_project.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("minimal project should pass validation: %v", err)
	}
}

func TestFR1_S2_FullProjectPasses(t *testing.T) {
	sch := compileProjectSchema(t)
	data := readTestdata(t, "valid_project.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("full project should pass validation: %v", err)
	}
}

func TestFR1_S5_ProjectMissingNameFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{"modules": [{"id": "000000000001", "name": "m", "path": "m/"}]}`)
	if err == nil {
		t.Fatal("expected validation error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should reference 'name', got: %v", err)
	}
}

func TestFR1_S6_ProjectMissingModulesFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{"name": "orphan"}`)
	if err == nil {
		t.Fatal("expected validation error for missing modules, got nil")
	}
	if !strings.Contains(err.Error(), "modules") {
		t.Fatalf("error should reference 'modules', got: %v", err)
	}
}

func TestFR1_S7_ProjectEmptyModulesFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{"name": "empty-modules", "modules": []}`)
	if err == nil {
		t.Fatal("expected validation error for empty modules array, got nil")
	}
}

func TestFR1_S9_RequirementMissingRequiredFields(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			"missing type",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "requirements": [{"id": "000000000001", "name": "No type field"}]}`,
			"type",
		},
		{
			"missing id",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "requirements": [{"type": "functional", "name": "No id"}]}`,
			"id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error should reference %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestNFR4_S10_WrongTypeForIDProject(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"string ID in module declaration",
			`{"name": "p", "modules": [{"id": "one", "name": "m", "path": "m/"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatal("expected validation error for wrong ID type, got nil")
			}
		})
	}
}

func TestFR1_S11_InvalidRequirementTypeEnum(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [{"id": "000000000001", "type": "performance", "name": "R"}]
	}`)
	if err == nil {
		t.Fatal("expected validation error for invalid requirement type enum, got nil")
	}
}

func TestFR1_S12_ExtraFieldsRejected(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"extra field at project level",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "author": "unknown"}`,
		},
		{
			"extra field in module declaration",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/", "priority": "high"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for extra field in %s, got nil", tt.name)
			}
		})
	}
}

func TestNFR4_S13_InvalidIDPatternProject(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"short id in requirement",
			`{"name": "p", "modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}], "requirements": [{"id": "abc", "type": "functional", "name": "R"}]}`,
		},
		{
			"non-hex id in module",
			`{"name": "p", "modules": [{"id": "ZZZ000000000", "name": "m", "path": "m/"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatal("expected validation error for invalid ID pattern, got nil")
			}
		})
	}
}

// TestNFR4_S13_NegativeIDProject pins the project-side half of S13: a
// negative numeric ID fails the same way as any other number, since
// project.schema.json constrains module ids by pattern (a string), not by
// numeric range.
func TestNFR4_S13_NegativeIDProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{"name": "p", "modules": [{"id": -1, "name": "m", "path": "m/"}]}`)
	if err == nil {
		t.Fatal("expected validation error for id: -1, got nil")
	}
}

func TestFR1_S14_EmptyStringNameFails(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"empty project name",
			`{"name": "", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}]}`,
		},
		{
			"empty module name",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "", "path": "m/"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for empty name in %s, got nil", tt.name)
			}
		})
	}
}

func TestFR1_S15_DependsOnDuplicatesFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [
			{"id": "000000000001", "type": "functional", "name": "R1"},
			{"id": "000000000002", "type": "functional", "name": "R2", "depends_on": ["000000000001", "000000000001"]}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for duplicate depends_on items, got nil")
	}
}

// TestFR6_S16_RetiredProjectKeysRejected pins the deletion of the two
// vestigial project-level node types. The project schema sets
// additionalProperties:false, so a spec that still declares either one is a
// hard validation failure rather than a silently ignored field.
func TestFR6_S16_RetiredProjectKeysRejected(t *testing.T) {
	sch := compileProjectSchema(t)

	for _, tt := range []struct {
		name string
		doc  string
	}{
		{
			"milestones",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "milestones": []}`,
		},
		{
			"test_plan",
			`{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "test_plan": {"scenarios": []}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateProject(t, sch, tt.doc); err == nil {
				t.Fatalf("%q was retired but still validates", tt.name)
			}
		})
	}
}

// TestFR6_S21_SectionsFullFieldsPasses pins that the sections array accepts
// an entry with additional freeform properties beyond the required envelope
// (id, name, type) — the envelope is validated, the rest is delegated to the
// coupled module's own section.schema.json.
func TestFR6_S21_SectionsFullFieldsPasses(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"sections": [
			{"id": "000000000001", "name": "delivery", "type": "coupled", "versioning": {"scheme": "semver"}}
		]
	}`)
	if err != nil {
		t.Fatalf("section with envelope + freeform content should pass: %v", err)
	}
}

// TestFR6_S22_SectionMissingNameFails pins that the section envelope's
// required fields (id, name, type) are enforced like any other node.
func TestFR6_S22_SectionMissingNameFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"sections": [{"id": "000000000001", "type": "coupled"}]
	}`)
	if err == nil {
		t.Fatal("expected validation error for section missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should reference 'name', got: %v", err)
	}
}

// TestFR6_S23_SectionInvalidIDTypeFails pins that section ids follow the
// same identity-hash pattern as every other node type.
func TestFR6_S23_SectionInvalidIDTypeFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"sections": [{"id": "one", "name": "delivery", "type": "coupled"}]
	}`)
	if err == nil {
		t.Fatal("expected validation error for section with non-hex id, got nil")
	}
}

// TestFR6_S24_EmptySectionsArrayPasses pins that sections, like every other
// optional array, has no minItems constraint.
func TestFR6_S24_EmptySectionsArrayPasses(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"sections": []
	}`)
	if err != nil {
		t.Fatalf("empty sections array should pass: %v", err)
	}
}

// TestFR6_E11_SectionArbitraryContentPasses pins the boundary between schema
// validation and the coupled module's own section.schema.json: the project
// schema validates only the envelope and allows any additional properties.
func TestFR6_E11_SectionArbitraryContentPasses(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"sections": [
			{
				"id": "000000000001",
				"name": "performance",
				"type": "coupled",
				"budgets": [{"metric": "p99_latency", "threshold_ms": 200}],
				"monitoring": {"dashboard": "grafana.internal/perf"}
			}
		]
	}`)
	if err != nil {
		t.Fatalf("section with arbitrary content properties should pass: %v", err)
	}
}

func TestFR1_S18_GoTypeRoundTripProject(t *testing.T) {
	data := readTestdata(t, "valid_project.json")
	var proj Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := json.Marshal(&proj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var proj2 Project
	if err := json.Unmarshal(out, &proj2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if proj.Name != proj2.Name {
		t.Fatalf("name mismatch: want %q, got %q", proj.Name, proj2.Name)
	}
	if len(proj.Modules) != len(proj2.Modules) {
		t.Fatalf("modules length mismatch: want %d, got %d", len(proj.Modules), len(proj2.Modules))
	}
	// requires_module is the project level's string-array field, and pinning it
	// through the round-trip is the point of this block. Iterate rather than
	// hard-code an index: the fixture's first module carries no requires_module,
	// so an indexed check is one fixture edit away from silently asserting
	// nothing. pinned counts elements actually compared and fails if the fixture
	// stops exercising the field at all.
	pinned := 0
	for i := range proj.Modules {
		if len(proj.Modules[i].RequiresModule) != len(proj2.Modules[i].RequiresModule) {
			t.Fatalf("modules[%d].requires_module length mismatch: want %d, got %d",
				i, len(proj.Modules[i].RequiresModule), len(proj2.Modules[i].RequiresModule))
		}
		for j, v := range proj.Modules[i].RequiresModule {
			if v != proj2.Modules[i].RequiresModule[j] {
				t.Fatalf("modules[%d].requires_module[%d] mismatch: want %s, got %s",
					i, j, v, proj2.Modules[i].RequiresModule[j])
			}
			pinned++
		}
	}
	if pinned == 0 {
		t.Fatal("fixture carries no requires_module: the string-array round-trip assertion is vacuous")
	}
	if len(proj.Requirements) > 1 {
		if len(proj.Requirements[1].DependsOn) != len(proj2.Requirements[1].DependsOn) {
			t.Fatal("depends_on length mismatch")
		}
		for i, v := range proj.Requirements[1].DependsOn {
			if v != proj2.Requirements[1].DependsOn[i] {
				t.Fatalf("depends_on[%d] mismatch: want %s, got %s", i, v, proj2.Requirements[1].DependsOn[i])
			}
		}
	}
	if len(proj.Requirements) > 0 && proj.Requirements[0].Priority != nil {
		if proj2.Requirements[0].Priority == nil || *proj.Requirements[0].Priority != *proj2.Requirements[0].Priority {
			t.Fatal("priority mismatch")
		}
	}
}

func TestFR1_S19_PriorityFieldOnProjectRequirements(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("priority accepted", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "000000000001", "type": "functional", "name": "R", "priority": 1}
			]
		}`)
		if err != nil {
			t.Fatalf("priority field should be accepted: %v", err)
		}
	})

	t.Run("priority out of range fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "000000000001", "type": "functional", "name": "R", "priority": 5}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for priority > 4, got nil")
		}
	})

	t.Run("negative priority fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "000000000001", "type": "functional", "name": "R", "priority": -1}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for priority < 0, got nil")
		}
	})
}

func TestFR1_S25_DerivationFieldPendingOnly(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("pending accepted", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "aabbccddee00", "type": "functional", "name": "R", "priority": 1, "derivation": "pending"}
			]
		}`)
		if err != nil {
			t.Fatalf("derivation: \"pending\" should be accepted: %v", err)
		}
	})

	t.Run("unknown value fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "aabbccddee00", "type": "functional", "name": "R", "priority": 1, "derivation": "done"}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for derivation: \"done\", got nil")
		}
	})

	t.Run("empty string fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "aabbccddee00", "type": "functional", "name": "R", "priority": 1, "derivation": ""}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for derivation: \"\", got nil")
		}
	})

	t.Run("wrong type fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "aabbccddee00", "type": "functional", "name": "R", "priority": 1, "derivation": true}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for derivation: true, got nil")
		}
	})

	t.Run("field is optional", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}],
			"requirements": [
				{"id": "aabbccddee00", "type": "functional", "name": "R", "priority": 1}
			]
		}`)
		if err != nil {
			t.Fatalf("requirement without derivation should pass: %v", err)
		}
	})
}

// TestFR1_S28_SpecVersionAcceptedInProject pins the project-side half of
// S28: spec_version is an optional integer accepted on project.json only —
// metadata outside the hashed payload, absent meaning version 1. The
// module-side half (spec_version rejected on a module document) is pinned
// by TestFR2_S28_SpecVersionRejectedOnModuleDocument.
func TestFR1_S28_SpecVersionAcceptedInProject(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("accepted", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"spec_version": 1
		}`)
		if err != nil {
			t.Fatalf("spec_version should be accepted: %v", err)
		}
	})

	t.Run("wrong type fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"spec_version": "1"
		}`)
		if err == nil {
			t.Fatal("expected validation error for spec_version as string, got nil")
		}
	})
}

func TestFR1_E1_EmptyOptionalArraysValidProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [],
		"sections": []
	}`)
	if err != nil {
		t.Fatalf("empty optional arrays should pass: %v", err)
	}
}

func TestNFR4_E2_BoundaryIDValueProject(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("valid 12-char hex passes", func(t *testing.T) {
		err := validateProject(t, sch, `{"name": "p", "modules": [{"id": "aabbccddeeff", "name": "m", "path": "m/"}]}`)
		if err != nil {
			t.Fatalf("12-char hex should pass: %v", err)
		}
	})

	t.Run("11-char hex fails", func(t *testing.T) {
		err := validateProject(t, sch, `{"name": "p", "modules": [{"id": "aabbccddeef", "name": "m", "path": "m/"}]}`)
		if err == nil {
			t.Fatal("11-char hex should fail")
		}
	})
}

func TestFR1_E5_NullOptionalFieldFailsProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"description": null
	}`)
	if err == nil {
		t.Fatal("expected validation error for null description, got nil")
	}
}

func TestFR1_E6_WrongTopLevelTypeProject(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{"string", `"just a string"`},
		{"array", `[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.doc), &v); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := sch.Validate(v); err == nil {
				t.Fatalf("expected validation error for %s top-level type, got nil", tt.name)
			}
		})
	}
}

func TestFR1_E7_DependsOnNonExistentIDPassesProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [
			{"id": "000000000001", "type": "functional", "name": "R1", "depends_on": ["000000000999"]}
		]
	}`)
	if err != nil {
		t.Fatalf("depends_on with non-existent ID should pass schema validation: %v", err)
	}
}

func TestFR1_E8_PreqIDInProjectRequirementFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [
			{"id": "000000000001", "type": "functional", "name": "R", "preq_id": "000000000005"}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for preq_id in project requirement, got nil")
	}
}

// TestNFR4_E3_NumericIDAnyMagnitudeFailsProject pins that a module ID is a
// string matching the identity-hash pattern, so a number fails on type
// regardless of magnitude — there is no integer-id path in the format for a
// bound to apply to.
func TestNFR4_E3_NumericIDAnyMagnitudeFailsProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 2147483647, "name": "m", "path": "m/"}]
	}`)
	if err == nil {
		t.Fatal("expected validation error for large numeric id, got nil")
	}
}

func TestFR1_ProjectSchemaMetaProperties(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tests := []struct {
		name  string
		check func() bool
	}{
		{"$schema is 2020-12", func() bool { return raw["$schema"] == "https://json-schema.org/draft/2020-12/schema" }},
		{"$id is correct", func() bool { return raw["$id"] == "https://spexmachina.dev/schema/project.json" }},
		{"title is correct", func() bool { return raw["title"] == "Spex Machina Project" }},
		{"type is object", func() bool { return raw["type"] == "object" }},
		{"additionalProperties is false", func() bool { return raw["additionalProperties"] == false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check() {
				t.Fatal("check failed")
			}
		})
	}

	// Check required array contains "name" and "modules"
	req := raw["required"].([]any)
	wantRequired := map[string]bool{"name": false, "modules": false}
	for _, v := range req {
		if _, ok := wantRequired[v.(string)]; ok {
			wantRequired[v.(string)] = true
		}
	}
	for k, found := range wantRequired {
		if !found {
			t.Fatalf("required array should contain %q", k)
		}
	}

	// Check properties keys
	props := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "version", "requirements", "modules", "sections", "spec_version"} {
		if props[key] == nil {
			t.Fatalf("project schema missing property %q", key)
		}
	}
	for _, key := range []string{"milestones", "test_plan"} {
		if props[key] != nil {
			t.Fatalf("project schema still declares retired property %q", key)
		}
	}

	// Check $defs keys
	defs := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "module", "section"} {
		if defs[key] == nil {
			t.Fatalf("project schema missing $def %q", key)
		}
	}
	for _, key := range []string{"milestone", "test_scenario"} {
		if defs[key] != nil {
			t.Fatalf("project schema still declares retired $def %q", key)
		}
	}
}

func TestFR1_ProjectSchemaIdempotent(t *testing.T) {
	data1, err := ProjectSchema()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	data2, err := ProjectSchema()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("ProjectSchema() not idempotent")
	}
}

func TestFR1_ProjectSchemaConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ProjectSchema()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}
	for i := 1; i < 10; i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatalf("goroutine %d returned different content", i)
		}
	}
}

func TestFR1_ProjectSchemaNonTrivialSize(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	if len(data) <= 100 {
		t.Fatalf("project schema too small (%d bytes), expected > 100", len(data))
	}
}

// TestFR1_S27_ProjectProfileDeclaredArrayAccepted mirrors the module side's
// S27 (TestFR2_S27_ProfileDeclaredArrayAccepted) for the project schema: a
// profile-declared project-scoped type gets an array validated with the
// generic envelope, and additionalProperties:false still rejects any array
// no profile declares.
func TestFR1_S27_ProjectProfileDeclaredArrayAccepted(t *testing.T) {
	types := append(DefaultProjectNodeTypes(), ProjectNodeType{
		Name:            "milestone",
		PluralKey:       "milestones",
		RequiresContent: false,
	})
	data, err := ComposeProjectSchema(types, nil)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	t.Run("declared array passes", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"milestones": [{"id": "000000000002", "name": "M1"}]
		}`)
		if err != nil {
			t.Fatalf("profile-declared milestones array should pass: %v", err)
		}
	})

	t.Run("undeclared array still rejected", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
			"milestones": [{"id": "000000000002", "name": "M1"}],
			"widgets": []
		}`)
		if err == nil {
			t.Fatal("expected validation error for undeclared widgets array, got nil")
		}
	})

	t.Run("declared array enforces envelope constraints", func(t *testing.T) {
		tests := []struct {
			name string
			doc  string
		}{
			{"missing id", `{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "milestones": [{"name": "M1"}]}`},
			{"missing name", `{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "milestones": [{"id": "000000000002"}]}`},
			{"non-identity-hash id", `{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "milestones": [{"id": "one", "name": "M1"}]}`},
			{"extra field", `{"name": "p", "modules": [{"id": "000000000001", "name": "m", "path": "m/"}], "milestones": [{"id": "000000000002", "name": "M1", "status": "done"}]}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := validateProject(t, sch, tt.doc); err == nil {
					t.Fatalf("expected validation error, got nil")
				}
			})
		}
	})
}

// TestFR1_S27_ProjectProfileDeclaredEdgeFieldAccepted mirrors the module
// side's S27 edge coverage (TestFR2_S27_ProfileDeclaredEdgeFieldAccepted)
// for the project schema: a profile-declared edge whose source names a
// custom project-scoped type opens a reference field on that type's
// composed entry, and the same field is rejected when composed from a
// profile that never declared the edge — the two documents are byte-
// identical, only the profile differs, which is what pins the rejection to
// the missing declaration rather than to document shape.
func TestFR1_S27_ProjectProfileDeclaredEdgeFieldAccepted(t *testing.T) {
	types := append(DefaultProjectNodeTypes(), ProjectNodeType{
		Name:      "milestone",
		PluralKey: "milestones",
	})
	doc := `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"milestones": [
			{"id": "aabbccddeeff", "name": "M1", "tracks": ["112233445566"]}
		]
	}`

	t.Run("declared edge field passes", func(t *testing.T) {
		edges := []Edge{{Kind: "tracks", From: []string{"milestone"}, To: []string{"requirement"}}}
		data, err := ComposeProjectSchema(types, edges)
		if err != nil {
			t.Fatalf("ComposeProjectSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateProject(t, sch, doc); err != nil {
			t.Fatalf("profile-declared tracks edge should pass: %v", err)
		}
	})

	t.Run("undeclared edge field still rejected", func(t *testing.T) {
		data, err := ComposeProjectSchema(types, nil)
		if err != nil {
			t.Fatalf("ComposeProjectSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateProject(t, sch, doc); err == nil {
			t.Fatal("expected validation error for undeclared tracks field, got nil")
		}
	})

	t.Run("edge sourced at a different type does not leak in", func(t *testing.T) {
		edges := []Edge{{Kind: "tracks", From: []string{"requirement"}, To: []string{"milestone"}}}
		data, err := ComposeProjectSchema(types, edges)
		if err != nil {
			t.Fatalf("ComposeProjectSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateProject(t, sch, doc); err == nil {
			t.Fatal("expected validation error: tracks is not declared with milestone as its source")
		}
	})
}

// TestFR1_ComposeProjectSchemaEdgeFieldShape pins the shape of a
// profile-declared edge field on a synthesized entry definition: an
// optional array of identity-hash strings with uniqueItems, the same
// constraints every built-in edge field (depends_on, requires_module)
// already carries.
func TestFR1_ComposeProjectSchemaEdgeFieldShape(t *testing.T) {
	types := []ProjectNodeType{{Name: "milestone", PluralKey: "milestones"}}
	edges := []Edge{
		{Kind: "tracks", From: []string{"milestone"}, To: []string{"requirement"}},
		{Kind: "blocks_milestone", From: []string{"milestone"}, To: []string{"milestone"}},
	}
	data, err := ComposeProjectSchema(types, edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	def := raw["$defs"].(map[string]any)["milestone"].(map[string]any)
	props := def["properties"].(map[string]any)

	for _, kind := range []string{"tracks", "blocks_milestone"} {
		field, ok := props[kind].(map[string]any)
		if !ok {
			t.Fatalf("milestone definition missing edge property %q", kind)
		}
		if field["type"] != "array" {
			t.Fatalf("%s: want type array, got %v", kind, field["type"])
		}
		if field["uniqueItems"] != true {
			t.Fatalf("%s: want uniqueItems true, got %v", kind, field["uniqueItems"])
		}
		items, ok := field["items"].(map[string]any)
		if !ok {
			t.Fatalf("%s: items missing", kind)
		}
		if items["type"] != "string" || items["pattern"] != identityHashPattern {
			t.Fatalf("%s: items should be identity-hash strings, got %v", kind, items)
		}
	}

	required, _ := def["required"].([]any)
	for _, r := range required {
		if r == "tracks" || r == "blocks_milestone" {
			t.Fatalf("declared edge fields must not be required, got required=%v", required)
		}
	}

	if def["additionalProperties"] != false {
		t.Fatalf("entry definition should still reject undeclared fields")
	}
}

// TestFR1_ComposeProjectSchemaBuiltinRequirementGainsDeclaredEdges pins the
// project-scope half of "no built-in $defs remain in the frame"
// (arch_project_schema.md) at the composition layer directly: unlike the
// module side, no project-scoped type's $defs entry is frame-fixed, the
// built-in requirement type included, so ComposeProjectSchema composes a
// reference field onto it for an edge sourced at "requirement" exactly as
// it would for a profile-declared type — the same path
// TestFR1_ComposeProjectSchemaEdgeFieldShape exercises for a custom type.
// It calls ComposeProjectSchema directly rather than through a resolved
// Profile: Profile.Validate still refuses a "requirement"-sourced edge the
// frame does not already carry, because the same bare type name also
// reaches module.json's frame-fixed requirement $defs entry, which would
// silently drop it (builtinTypeNames) — that guard is scope-blind by
// construction and stays in force regardless of which scope's composition
// could actually carry the edge.
func TestFR1_ComposeProjectSchemaBuiltinRequirementGainsDeclaredEdges(t *testing.T) {
	edges := []Edge{{Kind: "tracks", From: []string{"requirement"}, To: []string{"requirement"}}}
	data, err := ComposeProjectSchema(DefaultProjectNodeTypes(), edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	err = validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "name": "R", "tracks": ["112233445566"]}
		]
	}`)
	if err != nil {
		t.Fatalf("declared tracks edge should compose onto the built-in requirement type: %v", err)
	}
}

// TestFR1_ComposeProjectSchemaPreqIDExcludedFromRequirement pins
// projectScopedEdges: the preq_id edge's From names "requirement" the same
// bare way depends_on's does, but preq_id links a module requirement to its
// parent project requirement — module-scope-only, the one property
// project.schema.json's requirement definition has always rejected (E8).
// Composing with the full default edge list must not let it leak onto the
// project-scoped requirement type now that requirement composes generically.
func TestFR1_ComposeProjectSchemaPreqIDExcludedFromRequirement(t *testing.T) {
	data, err := ComposeProjectSchema(DefaultProjectNodeTypes(), DefaultProfile().Edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	err = validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "name": "R", "preq_id": "112233445566"}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error: preq_id must not compose onto the project-scoped requirement type")
	}
}

// TestFR1_ComposeProjectSchemaDefaultAcceptsKnownGoodFixtures checks that
// composing from DefaultProjectNodeTypes yields a schema that still accepts
// the same known-good project fixtures the static schema accepts (S1, S2) —
// the requirement type's own fields (type, priority, derivation, depends_on)
// are declared on DefaultProjectNodeTypes and materialize through the same
// generic composition a profile-declared type goes through. It validates
// fixtures only — it does not compare the composed document against the
// shipped static project.schema.json byte-for-byte; that golden comparison
// is scenario P2 in test_schema_loading.md, owned by SchemaLoader.
func TestFR1_ComposeProjectSchemaDefaultAcceptsKnownGoodFixtures(t *testing.T) {
	data, err := ComposeProjectSchema(DefaultProjectNodeTypes(), DefaultProfile().Edges)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	full := readTestdata(t, "valid_project.json")
	var v any
	if err := json.Unmarshal(full, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("default-profile composed schema should accept valid_project.json: %v", err)
	}

	minimal := readTestdata(t, "minimal_project.json")
	if err := json.Unmarshal(minimal, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("default-profile composed schema should accept minimal_project.json: %v", err)
	}
}

// TestFR1_ComposeProjectSchemaNoContentPropertyWhenNotRequired pins the
// other half of the generic envelope: a project-scoped type declared
// without RequiresContent gets no content property at all, mirroring the
// module side's TestFR2_ComposeModuleSchemaNoContentPropertyWhenNotRequired.
func TestFR1_ComposeProjectSchemaNoContentPropertyWhenNotRequired(t *testing.T) {
	types := []ProjectNodeType{{Name: "milestone", PluralKey: "milestones"}}
	data, err := ComposeProjectSchema(types, nil)
	if err != nil {
		t.Fatalf("ComposeProjectSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	err = validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"milestones": [{"id": "000000000002", "name": "M1"}]
	}`)
	if err != nil {
		t.Fatalf("node type without RequiresContent should pass without content: %v", err)
	}

	err = validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": "000000000001", "name": "m", "path": "m/"}],
		"milestones": [{"id": "000000000002", "name": "M1", "content": "m1.md"}]
	}`)
	if err == nil {
		t.Fatal("expected validation error: content is not a declared property for this type")
	}
}

func TestFR1_ProjectSchemaSelfContainedRefs(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check that modules.items.$ref points to #/$defs/module
	props := raw["properties"].(map[string]any)
	modules := props["modules"].(map[string]any)
	items := modules["items"].(map[string]any)
	ref := items["$ref"].(string)
	if ref != "#/$defs/module" {
		t.Fatalf("modules $ref should be #/$defs/module, got %q", ref)
	}

	// Verify $defs/module exists
	defs := raw["$defs"].(map[string]any)
	if defs["module"] == nil {
		t.Fatal("$defs/module missing")
	}
}
