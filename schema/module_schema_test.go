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
	err := validateModule(t, sch, `{"components": [{"id": 1, "name": "C"}]}`)
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
			`{"name": "bad-req", "requirements": [{"id": 1, "title": "No type field", "preq_id": 1}]}`,
			"type",
		},
		{
			"missing id",
			`{"name": "bad-req", "requirements": [{"type": "functional", "title": "No id", "preq_id": 1}]}`,
			"id",
		},
		{
			"missing preq_id",
			`{"name": "bad-req", "requirements": [{"id": 1, "type": "functional", "title": "No preq_id"}]}`,
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
			"string ID in component",
			`{"name": "m", "components": [{"id": "one", "name": "C"}]}`,
		},
		{
			"float ID in component",
			`{"name": "m", "components": [{"id": 1.5, "name": "C"}]}`,
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

func TestFR2_S11_InvalidRequirementTypeEnum(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "requirements": [{"id": 1, "type": "performance", "title": "R", "preq_id": 1}]}`)
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
			`{"name": "m", "components": [{"id": 1, "name": "C", "status": "done"}]}`,
		},
		{
			"extra field at test_section level",
			`{"name": "m", "test_sections": [{"id": 1, "name": "T", "priority": "P1"}]}`,
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

func TestNFR4_S13_IDBelowMinimum(t *testing.T) {
	sch := compileModuleSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"id zero in component",
			`{"name": "m", "components": [{"id": 0, "name": "C"}]}`,
		},
		{
			"negative id in requirement",
			`{"name": "m", "requirements": [{"id": -1, "type": "functional", "title": "R", "preq_id": 1}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModule(t, sch, tt.doc)
			if err == nil {
				t.Fatal("expected validation error for ID below minimum, got nil")
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

func TestFR2_S15_DependsOnDuplicatesFails(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [
			{"id": 1, "type": "functional", "title": "R1", "preq_id": 1},
			{"id": 2, "type": "functional", "title": "R2", "preq_id": 1, "depends_on": [1, 1]}
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
				{"id": 1, "name": "Unit tests", "content": "test_unit.md", "describes": [1, 2]}
			]
		}`)
		if err != nil {
			t.Fatalf("valid test_sections should pass: %v", err)
		}
	})

	t.Run("test_section missing required name", func(t *testing.T) {
		err := validateModule(t, sch, `{
			"name": "m",
			"test_sections": [{"id": 1}]
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
			t.Fatalf("implements[%d] mismatch: want %d, got %d", i, v, mod2.Components[0].Implements[i])
		}
	}
	if len(mod.ImplSections[0].Describes) != len(mod2.ImplSections[0].Describes) {
		t.Fatalf("describes length mismatch")
	}
	for i, v := range mod.ImplSections[0].Describes {
		if v != mod2.ImplSections[0].Describes[i] {
			t.Fatalf("describes[%d] mismatch: want %d, got %d", i, v, mod2.ImplSections[0].Describes[i])
		}
	}
	if len(mod.DataFlows[0].Uses) != len(mod2.DataFlows[0].Uses) {
		t.Fatalf("data_flow uses length mismatch")
	}
	for i, v := range mod.DataFlows[0].Uses {
		if v != mod2.DataFlows[0].Uses[i] {
			t.Fatalf("uses[%d] mismatch: want %d, got %d", i, v, mod2.DataFlows[0].Uses[i])
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
			"requirements": [{"id": 1, "type": "functional", "title": "R"}]
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
			"requirements": [{"id": 1, "type": "functional", "title": "R", "preq_id": 1}]
		}`)
		if err != nil {
			t.Fatalf("requirement with preq_id should pass: %v", err)
		}
	})
}

func TestFR2_E1_EmptyOptionalArraysValid(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{
		"name": "m",
		"requirements": [],
		"components": [],
		"impl_sections": [],
		"data_flows": [],
		"test_sections": []
	}`)
	if err != nil {
		t.Fatalf("empty optional arrays should pass: %v", err)
	}
}

func TestNFR4_E2_BoundaryIDValue(t *testing.T) {
	sch := compileModuleSchema(t)

	t.Run("id 1 passes", func(t *testing.T) {
		err := validateModule(t, sch, `{"name": "m", "components": [{"id": 1, "name": "C"}]}`)
		if err != nil {
			t.Fatalf("id=1 should pass: %v", err)
		}
	})

	t.Run("id 0 fails", func(t *testing.T) {
		err := validateModule(t, sch, `{"name": "m", "components": [{"id": 0, "name": "C"}]}`)
		if err == nil {
			t.Fatal("id=0 should fail")
		}
	})
}

func TestNFR4_E3_LargeIDValue(t *testing.T) {
	sch := compileModuleSchema(t)
	err := validateModule(t, sch, `{"name": "m", "components": [{"id": 2147483647, "name": "MaxInt"}]}`)
	if err != nil {
		t.Fatalf("large ID should pass: %v", err)
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
			{"id": 1, "type": "functional", "title": "R1", "preq_id": 1, "depends_on": [999]}
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
		name string
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
	for _, key := range []string{"name", "description", "requirements", "components", "impl_sections", "data_flows", "test_sections"} {
		if props[key] == nil {
			t.Fatalf("module schema missing property %q", key)
		}
	}

	// Check $defs keys
	defs := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "component", "impl_section", "data_flow", "test_section"} {
		if defs[key] == nil {
			t.Fatalf("module schema missing $def %q", key)
		}
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
