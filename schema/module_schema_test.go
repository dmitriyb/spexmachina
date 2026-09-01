package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileModuleSchema compiles the embedded module schema for validation.
func compileModuleSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	return compileSchemaFromBytes(t, data)
}

// compileSchemaFromBytes compiles a raw module schema document (static or
// composed) for validation.
func compileSchemaFromBytes(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal module schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("module.schema.json", doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("module.schema.json")
	if err != nil {
		t.Fatalf("compile module schema: %v", err)
	}
	return sch
}

// validateModule validates a JSON document against the module schema.
// Returns nil on success, error on validation failure.
func validateModule(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

func TestFR2_S3_MinimalModulePasses(t *testing.T) {
	sch := compileModuleSchema(t)
	data := readTestdata(t, "minimal_module.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("minimal module should pass validation: %v", err)
	}
}

func TestFR2_S4_FullModulePasses(t *testing.T) {
	sch := compileModuleSchema(t)
	data := readTestdata(t, "valid_module.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("full module should pass validation: %v", err)
	}
}

func TestFR2_S8_ModuleMissingNameFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md"}]}`)
	if err == nil {
		t.Fatal("expected validation error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("error should reference 'name', got: %v", err)
	}
}

func TestFR2_S9_RequirementMissingRequiredFields(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			"missing type",
			`{"name": "bad-req", "requirements": [{"id": "aabbccddeeff", "title": "No type field", "preq_id": "112233445566"}]}`,
			"type",
		},
		{
			"missing id",
			`{"name": "bad-req", "requirements": [{"type": "functional", "title": "No id", "preq_id": "112233445566"}]}`,
			"id",
		},
		{
			"missing preq_id",
			`{"name": "bad-req", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "No preq_id"}]}`,
			"preq_id",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error should reference %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestNFR4_S10_WrongTypeForID(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"integer ID in component",
			`{"name": "m", "components": [{"id": 1, "name": "C", "content": "arch_c.md"}]}`,
		},
		{
			"integer ID in requirement",
			`{"name": "m", "requirements": [{"id": 1, "type": "functional", "title": "R", "preq_id": "aabbccddeeff"}]}`,
		},
		{
			"float ID in component",
			`{"name": "m", "components": [{"id": 1.5, "name": "C", "content": "arch_c.md"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if err == nil {
				t.Fatal("expected validation error for wrong ID type, got nil")
			}
		})
	}
}

// TestNFR4_S13_IDBelowMinimumModule pins the module-side half of S13: a
// numeric zero ID fails the same way any other number does, since the field
// is a string matching the identity-hash pattern with no numeric range to
// violate.
func TestNFR4_S13_IDBelowMinimumModule(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "components": [{"id": 0, "name": "C", "content": "arch_c.md"}]}`)
	if err == nil {
		t.Fatal("expected validation error for id: 0, got nil")
	}
}

// TestNFR4_E2_BoundaryIDValueModule mirrors the project-side boundary test:
// module.schema.json spells the identity-hash pattern as ^[a-f0-9]{12}$, and
// the 12-vs-11-character boundary must hold there too.
func TestNFR4_E2_BoundaryIDValueModule(t *testing.T) {
	sch := compileModuleSchema(t)

	t.Run("12-char hex passes", func(t *testing.T) {
		err := validateModule(t, sch, `{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md"}]}`)
		if err != nil {
			t.Fatalf("12-char hex should pass: %v", err)
		}
	})

	t.Run("11-char hex fails", func(t *testing.T) {
		err := validateModule(t, sch, `{"name": "m", "components": [{"id": "aabbccddeef", "name": "C", "content": "arch_c.md"}]}`)
		if err == nil {
			t.Fatal("11-char hex should fail")
		}
	})
}

// TestNFR4_E3_NumericIDAnyMagnitudeFailsModule pins that a component ID is a
// string matching the identity-hash pattern, so a number fails on type
// regardless of magnitude — there is no integer-id path in the format for a
// bound to apply to.
func TestNFR4_E3_NumericIDAnyMagnitudeFailsModule(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "components": [{"id": 2147483647, "name": "MaxInt", "content": "arch_maxint.md"}]}`)
	if err == nil {
		t.Fatal("expected validation error for large numeric id, got nil")
	}
}

// TestNFR4_E4_WholeNumberFloatIDFailsModule pins that draft 2020-12's
// integer-vs-float leniency for whole-number floats like 1.0 never becomes
// reachable here: the id field's type is string, so any number fails before
// that judgement would apply.
func TestNFR4_E4_WholeNumberFloatIDFailsModule(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "components": [{"id": 1.0, "name": "C", "content": "arch_c.md"}]}`)
	if err == nil {
		t.Fatal("expected validation error for whole-number float id, got nil")
	}
}

func TestFR2_S11_InvalidRequirementTypeEnum(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "performance", "title": "R", "preq_id": "112233445566"}]}`)
	if err == nil {
		t.Fatal("expected validation error for invalid requirement type enum, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Fatalf("error should reference 'type', got: %v", err)
	}
}

func TestFR2_S12_ExtraFieldsRejected(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"extra field at component level",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md", "status": "done"}]}`,
		},
		{
			"extra field at test_section level",
			`{"name": "m", "test_sections": [{"id": "aabbccddeeff", "name": "T", "content": "test_t.md", "priority": "P1"}]}`,
		},
		{
			"extra field at module root",
			`{"name": "m", "author": "unknown"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for extra field in %s, got nil", tt.name)
			}
		})
	}
}

func TestNFR4_IDPatternValidation(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{
			"valid 12-char hex ID",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md"}]}`,
			false,
		},
		{
			"too short ID",
			`{"name": "m", "components": [{"id": "aabbcc", "name": "C", "content": "arch_c.md"}]}`,
			true,
		},
		{
			"too long ID",
			`{"name": "m", "components": [{"id": "aabbccddeeff00", "name": "C", "content": "arch_c.md"}]}`,
			true,
		},
		{
			"uppercase hex rejected",
			`{"name": "m", "components": [{"id": "AABBCCDDEEFF", "name": "C", "content": "arch_c.md"}]}`,
			true,
		},
		{
			"non-hex characters rejected",
			`{"name": "m", "components": [{"id": "aabbccddeegg", "name": "C", "content": "arch_c.md"}]}`,
			true,
		},
		{
			"empty string rejected",
			`{"name": "m", "components": [{"id": "", "name": "C", "content": "arch_c.md"}]}`,
			true,
		},
		{
			"valid preq_id pattern",
			`{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R", "preq_id": "112233445566"}]}`,
			false,
		},
		{
			"invalid preq_id pattern",
			`{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R", "preq_id": "short"}]}`,
			true,
		},
		{
			"valid depends_on hashes",
			`{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R", "preq_id": "112233445566", "depends_on": ["ffeeddccbbaa"]}]}`,
			false,
		},
		{
			"invalid depends_on item pattern",
			`{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R", "preq_id": "112233445566", "depends_on": ["bad"]}]}`,
			true,
		},
		{
			"valid implements hashes",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md", "implements": ["112233445566"]}]}`,
			false,
		},
		{
			"invalid implements item pattern",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md", "implements": ["xyz"]}]}`,
			true,
		},
		{
			"valid uses hashes",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md", "uses": ["112233445566"]}]}`,
			false,
		},
		{
			"valid describes hashes",
			`{"name": "m", "test_sections": [{"id": "aabbccddeeff", "name": "S", "content": "test_s.md", "describes": ["112233445566"]}]}`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestFR2_S14_EmptyStringNameFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": ""}`)
	if err == nil {
		t.Fatal("expected validation error for empty name, got nil")
	}
}

// TestFR2_S14_EmptyRequirementTitleFails pins the module-side half of S14:
// a requirement's title carries the same minLength: 1 constraint the name
// fields do, which is the promise IH6 in the schema-loading tests rests on
// — no reachable node hashes an empty identity part.
func TestFR2_S14_EmptyRequirementTitleFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "", "preq_id": "112233445566"}]}`)
	if err == nil {
		t.Fatal("expected validation error for empty requirement title, got nil")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Fatalf("error should reference 'title', got: %v", err)
	}
}

func TestFR2_ContentRequiredOnContentBearingNodes(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"component missing content",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C"}]}`,
		},
		{
			"component empty content",
			`{"name": "m", "components": [{"id": "aabbccddeeff", "name": "C", "content": ""}]}`,
		},
		{
			"test_section missing content",
			`{"name": "m", "test_sections": [{"id": "aabbccddeeff", "name": "T"}]}`,
		},
		{
			"test_section empty content",
			`{"name": "m", "test_sections": [{"id": "aabbccddeeff", "name": "T", "content": ""}]}`,
		},
		{
			"data_flow missing content",
			`{"name": "m", "data_flows": [{"id": "aabbccddeeff", "name": "F"}]}`,
		},
		{
			"data_flow empty content",
			`{"name": "m", "data_flows": [{"id": "aabbccddeeff", "name": "F", "content": ""}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
			if !strings.Contains(err.Error(), "content") {
				t.Fatalf("error should reference 'content', got: %v", err)
			}
		})
	}
}

func TestFR2_S15_DependsOnDuplicatesFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "R1", "preq_id": "112233445566"},
			{"id": "112233445566", "type": "functional", "title": "R2", "preq_id": "112233445566", "depends_on": ["aabbccddeeff", "aabbccddeeff"]}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for duplicate depends_on items, got nil")
	}
}

func TestFR5_S17_TestSectionsValidation(t *testing.T) {
	sch := compileModuleSchema(t)

	t.Run("valid test_sections with full fields", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"test_sections": [
				{"id": "aabbccddeeff", "name": "Unit tests", "content": "test_unit.md", "describes": ["112233445566", "ffeeddccbbaa"]}
			]
		}`)
		if err != nil {
			t.Fatalf("valid test_sections should pass: %v", err)
		}
	})

	t.Run("test_section missing required name", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"test_sections": [{"id": "aabbccddeeff", "content": "test_t.md"}]
		}`)
		if err == nil {
			t.Fatal("expected validation error for test_section missing name, got nil")
		}
		if !strings.Contains(err.Error(), "name") {
			t.Fatalf("error should reference 'name', got: %v", err)
		}
	})
}

func TestFR2_S18_GoTypeRoundTrip(t *testing.T) {
	data := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	out, err := json.Marshal(&mod)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var mod2 ModuleSpec
	if err := json.Unmarshal(out, &mod2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if mod.Name != mod2.Name {
		t.Fatalf("name mismatch: want %q, got %q", mod.Name, mod2.Name)
	}
	if len(mod.Components) != len(mod2.Components) {
		t.Fatalf("components length mismatch: want %d, got %d", len(mod.Components), len(mod2.Components))
	}
	if len(mod.Components[0].Implements) != len(mod2.Components[0].Implements) {
		t.Fatalf("implements length mismatch")
	}
	for i, v := range mod.Components[0].Implements {
		if v != mod2.Components[0].Implements[i] {
			t.Fatalf("implements[%d] mismatch: want %s, got %s", i, v, mod2.Components[0].Implements[i])
		}
	}
	if len(mod.TestSections[0].Describes) != len(mod2.TestSections[0].Describes) {
		t.Fatalf("describes length mismatch")
	}
	for i, v := range mod.TestSections[0].Describes {
		if v != mod2.TestSections[0].Describes[i] {
			t.Fatalf("describes[%d] mismatch: want %s, got %s", i, v, mod2.TestSections[0].Describes[i])
		}
	}
	if len(mod.DataFlows[0].Uses) != len(mod2.DataFlows[0].Uses) {
		t.Fatalf("data_flow uses length mismatch")
	}
	for i, v := range mod.DataFlows[0].Uses {
		if v != mod2.DataFlows[0].Uses[i] {
			t.Fatalf("uses[%d] mismatch: want %s, got %s", i, v, mod2.DataFlows[0].Uses[i])
		}
	}
	if len(mod.TestSections) != len(mod2.TestSections) {
		t.Fatalf("test_sections length mismatch")
	}
}

func TestFR2_S20_PreqIDRequiredOnModuleRequirements(t *testing.T) {
	sch := compileModuleSchema(t)

	t.Run("missing preq_id fails", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R"}]
		}`)
		if err == nil {
			t.Fatal("expected validation error for missing preq_id, got nil")
		}
		if !strings.Contains(err.Error(), "preq_id") {
			t.Fatalf("error should reference 'preq_id', got: %v", err)
		}
	})

	t.Run("with preq_id passes", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"requirements": [{"id": "aabbccddeeff", "type": "functional", "title": "R", "preq_id": "112233445566"}]
		}`)
		if err != nil {
			t.Fatalf("requirement with preq_id should pass: %v", err)
		}
	})
}

func TestFR2_S26_DerivationFieldRejected(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [
			{"id": "aabbccddee01", "type": "functional", "title": "R", "preq_id": "aabbccddee00", "derivation": "pending"}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for derivation on a module requirement, got nil")
	}
}

func TestFR2_E1_EmptyOptionalArraysValid(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [],
		"components": [],
		"data_flows": [],
		"test_sections": []
	}`)
	if err != nil {
		t.Fatalf("empty optional arrays should pass: %v", err)
	}
}

func TestFR2_E5_NullOptionalFieldFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "description": null}`)
	if err == nil {
		t.Fatal("expected validation error for null description, got nil")
	}
}

func TestFR2_E6_WrongTopLevelType(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{"array", `[]`},
		{"string", `"just a string"`},
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

func TestFR2_E7_DependsOnNonExistentIDPasses(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "R1", "preq_id": "112233445566", "depends_on": ["ffeeddccbbaa"]}
		]
	}`)
	if err != nil {
		t.Fatalf("depends_on with non-existent ID should pass schema validation: %v", err)
	}
}

func TestFR2_ModuleSchemaMetaProperties(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
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
		{"$id is correct", func() bool { return raw["$id"] == "https://spexmachina.dev/schema/module.json" }},
		{"title is correct", func() bool { return raw["title"] == "Spex Machina Module" }},
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

	// Check required array contains "name"
	req := raw["required"].([]any)
	found := false
	for _, v := range req {
		if v == "name" {
			found = true
		}
	}
	if !found {
		t.Fatal("required array should contain 'name'")
	}

	// Check properties keys
	props := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "requirements", "components", "data_flows", "test_sections"} {
		if props[key] == nil {
			t.Fatalf("module schema missing property %q", key)
		}
	}

	// Check $defs keys
	defs := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "component", "data_flow", "test_section"} {
		if defs[key] == nil {
			t.Fatalf("module schema missing $def %q", key)
		}
	}
}

func TestNFR4_IDFieldsUseIdentityHashPattern(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	defs := raw["$defs"].(map[string]any)
	wantPattern := "^[a-f0-9]{12}$"

	// Check all $def ID fields use identity hash pattern
	for _, defName := range []string{"requirement", "component", "data_flow", "test_section"} {
		def := defs[defName].(map[string]any)
		props := def["properties"].(map[string]any)
		idProp := props["id"].(map[string]any)
		if idProp["type"] != "string" {
			t.Fatalf("%s.id should be type string, got %v", defName, idProp["type"])
		}
		if idProp["pattern"] != wantPattern {
			t.Fatalf("%s.id should have pattern %q, got %v", defName, wantPattern, idProp["pattern"])
		}
	}

	// Check preq_id uses identity hash pattern
	reqDef := defs["requirement"].(map[string]any)
	reqProps := reqDef["properties"].(map[string]any)
	preqProp := reqProps["preq_id"].(map[string]any)
	if preqProp["type"] != "string" {
		t.Fatalf("requirement.preq_id should be type string, got %v", preqProp["type"])
	}
	if preqProp["pattern"] != wantPattern {
		t.Fatalf("requirement.preq_id should have pattern %q, got %v", wantPattern, preqProp["pattern"])
	}
}

func TestFR2_ModuleSchemaIdempotent(t *testing.T) {
	data1, err := ModuleSchema()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	data2, err := ModuleSchema()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("ModuleSchema() not idempotent")
	}
}

func TestFR2_ModuleSchemaConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ModuleSchema()
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

func TestFR2_ModuleSchemaE1_NonTrivialSize(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	if len(data) <= 100 {
		t.Fatalf("module schema too small (%d bytes), expected > 100", len(data))
	}
}

func TestFR2_ModuleSchemaE6_SelfContainedRefs(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check that components.items.$ref points to #/$defs/component
	props := raw["properties"].(map[string]any)
	components := props["components"].(map[string]any)
	items := components["items"].(map[string]any)
	ref := items["$ref"].(string)
	if ref != "#/$defs/component" {
		t.Fatalf("components $ref should be #/$defs/component, got %q", ref)
	}

	// Verify $defs/component exists
	defs := raw["$defs"].(map[string]any)
	if defs["component"] == nil {
		t.Fatal("$defs/component missing")
	}
}

// TestFR2_S27_ProfileDeclaredArrayAccepted composes the module schema from
// the default profile plus a novel "endpoint" type and checks both halves
// of S27: the profile-supplied array validates with the generic envelope,
// and an array no profile declares is still rejected by the frame's
// root-level additionalProperties:false.
func TestFR2_S27_ProfileDeclaredArrayAccepted(t *testing.T) {
	types := append(DefaultModuleNodeTypes(), ModuleNodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		RequiresContent: true,
	})
	data, err := ComposeModuleSchema(types, nil)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	t.Run("declared array passes", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"endpoints": [
				{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md"}
			]
		}`)
		if err != nil {
			t.Fatalf("profile-declared endpoints array should pass: %v", err)
		}
	})

	t.Run("undeclared array still rejected", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"endpoints": [
				{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md"}
			],
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
			{"missing id", `{"name": "m", "endpoints": [{"name": "GET /things", "content": "endpoint_things.md"}]}`},
			{"missing name", `{"name": "m", "endpoints": [{"id": "aabbccddeeff", "content": "endpoint_things.md"}]}`},
			{"missing content", `{"name": "m", "endpoints": [{"id": "aabbccddeeff", "name": "GET /things"}]}`},
			{"non-identity-hash id", `{"name": "m", "endpoints": [{"id": "one", "name": "GET /things", "content": "endpoint_things.md"}]}`},
			{"extra field", `{"name": "m", "endpoints": [{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md", "verb": "GET"}]}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := validateModule(t, sch, tt.doc); err == nil {
					t.Fatalf("expected validation error, got nil")
				}
			})
		}
	})
}

// TestFR2_S27_ProfileDeclaredEdgeFieldAccepted covers S27's edge half: a
// profile-declared edge whose source names a custom type opens a reference
// field on that type's composed entry, and the same field is rejected when
// composed from a profile that never declared the edge — the two documents
// are byte-identical, only the profile differs, which is what pins the
// rejection to the missing declaration rather than to document shape.
func TestFR2_S27_ProfileDeclaredEdgeFieldAccepted(t *testing.T) {
	types := append(DefaultModuleNodeTypes(), ModuleNodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		RequiresContent: true,
	})
	doc := `{
		"name": "m",
		"endpoints": [
			{"id": "aabbccddeeff", "name": "GET /things", "content": "endpoint_things.md", "serves": ["112233445566"]}
		]
	}`

	t.Run("declared edge field passes", func(t *testing.T) {
		edges := []Edge{{Kind: "serves", From: []string{"endpoint"}, To: []string{"component"}}}
		data, err := ComposeModuleSchema(types, edges)
		if err != nil {
			t.Fatalf("ComposeModuleSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateModule(t, sch, doc); err != nil {
			t.Fatalf("profile-declared serves edge should pass: %v", err)
		}
	})

	t.Run("undeclared edge field still rejected", func(t *testing.T) {
		data, err := ComposeModuleSchema(types, nil)
		if err != nil {
			t.Fatalf("ComposeModuleSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateModule(t, sch, doc); err == nil {
			t.Fatal("expected validation error for undeclared serves field, got nil")
		}
	})

	t.Run("edge sourced at a different type does not leak in", func(t *testing.T) {
		edges := []Edge{{Kind: "serves", From: []string{"component"}, To: []string{"endpoint"}}}
		data, err := ComposeModuleSchema(types, edges)
		if err != nil {
			t.Fatalf("ComposeModuleSchema: %v", err)
		}
		sch := compileSchemaFromBytes(t, data)
		if err := validateModule(t, sch, doc); err == nil {
			t.Fatal("expected validation error: serves is not declared with endpoint as its source")
		}
	})
}

// TestFR2_ComposeModuleSchemaEdgeFieldShape pins the shape of a
// profile-declared edge field on a synthesized entry definition: an
// optional array of identity-hash strings with uniqueItems, the same
// constraints every built-in edge field (implements, uses, describes,
// provided_by) already carries.
func TestFR2_ComposeModuleSchemaEdgeFieldShape(t *testing.T) {
	types := []ModuleNodeType{{Name: "endpoint", PluralKey: "endpoints"}}
	edges := []Edge{
		{Kind: "serves", From: []string{"endpoint"}, To: []string{"component"}},
		{Kind: "depends_on_endpoint", From: []string{"endpoint"}, To: []string{"endpoint"}},
	}
	data, err := ComposeModuleSchema(types, edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	def := raw["$defs"].(map[string]any)["endpoint"].(map[string]any)
	props := def["properties"].(map[string]any)

	for _, kind := range []string{"serves", "depends_on_endpoint"} {
		field, ok := props[kind].(map[string]any)
		if !ok {
			t.Fatalf("endpoint definition missing edge property %q", kind)
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
		if r == "serves" || r == "depends_on_endpoint" {
			t.Fatalf("declared edge fields must not be required, got required=%v", required)
		}
	}

	if def["additionalProperties"] != false {
		t.Fatalf("entry definition should still reject undeclared fields")
	}
}

// TestFR2_ComposeModuleSchemaBuiltinTypeUnaffectedByEdges pins that a
// built-in type's frame $def is reused unchanged even when the caller
// passes an edge sourced at that type's name: built-in definitions carry
// only the reference fields the frame already gives them.
func TestFR2_ComposeModuleSchemaBuiltinTypeUnaffectedByEdges(t *testing.T) {
	edges := []Edge{{Kind: "serves", From: []string{"component"}, To: []string{"component"}}}
	data, err := ComposeModuleSchema(DefaultModuleNodeTypes(), edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	err = validateModule(t, sch, `{
		"name": "m",
		"components": [
			{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md", "serves": ["112233445566"]}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error: built-in component definition should not gain a serves field")
	}
}

// TestFR2_ComposeModuleSchemaDefaultAcceptsKnownGoodFixtures checks that
// composing from DefaultModuleNodeTypes yields a schema that still accepts
// the same known-good module fixtures the static schema accepts (S3, S4),
// since every default type name resolves to the frame's existing $defs
// entry. It validates fixtures only — it does not compare the composed
// document against the shipped static module.schema.json byte-for-byte;
// that golden comparison is scenario P2 in test_schema_loading.md, owned by
// SchemaLoader.
func TestFR2_ComposeModuleSchemaDefaultAcceptsKnownGoodFixtures(t *testing.T) {
	data, err := ComposeModuleSchema(DefaultModuleNodeTypes(), DefaultProfile().Edges)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	full := readTestdata(t, "valid_module.json")
	var v any
	if err := json.Unmarshal(full, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("default-profile composed schema should accept valid_module.json: %v", err)
	}

	minimal := readTestdata(t, "minimal_module.json")
	if err := json.Unmarshal(minimal, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("default-profile composed schema should accept minimal_module.json: %v", err)
	}
}

// TestFR2_ComposeModuleSchemaNoContentPropertyWhenNotRequired pins the
// other half of the generic envelope: a type declared without
// RequiresContent gets no content property at all, the same shape an api
// entry has, rather than an optional one.
func TestFR2_ComposeModuleSchemaNoContentPropertyWhenNotRequired(t *testing.T) {
	types := []ModuleNodeType{{Name: "milestone", PluralKey: "milestones"}}
	data, err := ComposeModuleSchema(types, nil)
	if err != nil {
		t.Fatalf("ComposeModuleSchema: %v", err)
	}
	sch := compileSchemaFromBytes(t, data)

	if err := validateModule(t, sch, `{"name": "m", "milestones": [{"id": "aabbccddeeff", "name": "M1"}]}`); err != nil {
		t.Fatalf("node type without RequiresContent should pass without content: %v", err)
	}

	if err := validateModule(t, sch, `{"name": "m", "milestones": [{"id": "aabbccddeeff", "name": "M1", "content": "m1.md"}]}`); err == nil {
		t.Fatal("expected validation error: content is not a declared property for this type")
	}
}
