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
	err := validateProject(t, sch, `{"modules": [{"id": 1, "name": "m", "path": "m/"}]}`)
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
			`{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/"}], "requirements": [{"id": 1, "title": "No type field"}]}`,
			"type",
		},
		{
			"missing id",
			`{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/"}], "requirements": [{"type": "functional", "title": "No id"}]}`,
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
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"requirements": [{"id": 1, "type": "performance", "title": "R"}]
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
			`{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/"}], "author": "unknown"}`,
		},
		{
			"extra field in module declaration",
			`{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/", "priority": "high"}]}`,
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

func TestNFR4_S13_IDBelowMinimumProject(t *testing.T) {
	sch := compileProjectSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"id zero in milestone",
			`{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/"}], "milestones": [{"id": 0, "title": "M"}]}`,
		},
		{
			"negative id in module",
			`{"name": "p", "modules": [{"id": -1, "name": "m", "path": "m/"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProject(t, sch, tt.doc)
			if err == nil {
				t.Fatal("expected validation error for ID below minimum, got nil")
			}
		})
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
			`{"name": "", "modules": [{"id": 1, "name": "m", "path": "m/"}]}`,
		},
		{
			"empty module name",
			`{"name": "p", "modules": [{"id": 1, "name": "", "path": "m/"}]}`,
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
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"requirements": [
			{"id": 1, "type": "functional", "title": "R1"},
			{"id": 2, "type": "functional", "title": "R2", "depends_on": [1, 1]}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for duplicate depends_on items, got nil")
	}
}

func TestFR6_S16_TestPlanValidation(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("valid test_plan with minimal scenario", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": 1, "name": "m", "path": "m/"}],
			"test_plan": {
				"scenarios": [{"id": 1, "name": "Smoke test"}]
			}
		}`)
		if err != nil {
			t.Fatalf("valid test_plan should pass: %v", err)
		}
	})

	t.Run("test_plan with extra property fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": 1, "name": "m", "path": "m/"}],
			"test_plan": {
				"strategy": "risk-based",
				"scenarios": []
			}
		}`)
		if err == nil {
			t.Fatal("expected validation error for extra property in test_plan, got nil")
		}
	})
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
	if len(proj.Modules) > 1 {
		if len(proj.Modules[1].RequiresModule) != len(proj2.Modules[1].RequiresModule) {
			t.Fatal("requires_module length mismatch")
		}
		for i, v := range proj.Modules[1].RequiresModule {
			if v != proj2.Modules[1].RequiresModule[i] {
				t.Fatalf("requires_module[%d] mismatch: want %d, got %d", i, v, proj2.Modules[1].RequiresModule[i])
			}
		}
	}
	if len(proj.Requirements) > 1 {
		if len(proj.Requirements[1].DependsOn) != len(proj2.Requirements[1].DependsOn) {
			t.Fatal("depends_on length mismatch")
		}
		for i, v := range proj.Requirements[1].DependsOn {
			if v != proj2.Requirements[1].DependsOn[i] {
				t.Fatalf("depends_on[%d] mismatch: want %d, got %d", i, v, proj2.Requirements[1].DependsOn[i])
			}
		}
	}
	if len(proj.Requirements) > 0 && proj.Requirements[0].Priority != nil {
		if proj2.Requirements[0].Priority == nil || *proj.Requirements[0].Priority != *proj2.Requirements[0].Priority {
			t.Fatal("priority mismatch")
		}
	}
	if len(proj.Milestones) > 0 {
		if len(proj.Milestones[0].Groups) != len(proj2.Milestones[0].Groups) {
			t.Fatal("groups length mismatch")
		}
		for i, v := range proj.Milestones[0].Groups {
			if v != proj2.Milestones[0].Groups[i] {
				t.Fatalf("groups[%d] mismatch: want %d, got %d", i, v, proj2.Milestones[0].Groups[i])
			}
		}
	}
}

func TestFR1_S19_PriorityFieldOnProjectRequirements(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("priority accepted", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": 1, "name": "m", "path": "m/"}],
			"requirements": [
				{"id": 1, "type": "functional", "title": "R", "priority": 1}
			]
		}`)
		if err != nil {
			t.Fatalf("priority field should be accepted: %v", err)
		}
	})

	t.Run("priority out of range fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": 1, "name": "m", "path": "m/"}],
			"requirements": [
				{"id": 1, "type": "functional", "title": "R", "priority": 5}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for priority > 4, got nil")
		}
	})

	t.Run("negative priority fails", func(t *testing.T) {
		err := validateProject(t, sch, `{
			"name": "p",
			"modules": [{"id": 1, "name": "m", "path": "m/"}],
			"requirements": [
				{"id": 1, "type": "functional", "title": "R", "priority": -1}
			]
		}`)
		if err == nil {
			t.Fatal("expected validation error for priority < 0, got nil")
		}
	})
}

func TestFR1_E1_EmptyOptionalArraysValidProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"requirements": [],
		"milestones": [],
		"test_plan": {"scenarios": []}
	}`)
	if err != nil {
		t.Fatalf("empty optional arrays should pass: %v", err)
	}
}

func TestNFR4_E2_BoundaryIDValueProject(t *testing.T) {
	sch := compileProjectSchema(t)

	t.Run("id 1 passes", func(t *testing.T) {
		err := validateProject(t, sch, `{"name": "p", "modules": [{"id": 1, "name": "m", "path": "m/"}]}`)
		if err != nil {
			t.Fatalf("id=1 should pass: %v", err)
		}
	})

	t.Run("id 0 fails", func(t *testing.T) {
		err := validateProject(t, sch, `{"name": "p", "modules": [{"id": 0, "name": "m", "path": "m/"}]}`)
		if err == nil {
			t.Fatal("id=0 should fail")
		}
	})
}

func TestFR1_E5_NullOptionalFieldFailsProject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
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
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"requirements": [
			{"id": 1, "type": "functional", "title": "R1", "depends_on": [999]}
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
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"requirements": [
			{"id": 1, "type": "functional", "title": "R", "preq_id": 5}
		]
	}`)
	if err == nil {
		t.Fatal("expected validation error for preq_id in project requirement, got nil")
	}
}

func TestFR1_E3_LargeIDValuePasses(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 2147483647, "name": "m", "path": "m/"}],
		"requirements": [
			{"id": 2147483647, "type": "functional", "title": "R"}
		]
	}`)
	if err != nil {
		t.Fatalf("large ID value (2147483647) should pass validation: %v", err)
	}
}

func TestFR6_E9_TestPlanEmptyScenariosObject(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"test_plan": {}
	}`)
	if err != nil {
		t.Fatalf("empty test_plan object should pass: %v", err)
	}
}

func TestFR6_E10_TestScenarioModulesDuplicatesFails(t *testing.T) {
	sch := compileProjectSchema(t)
	err := validateProject(t, sch, `{
		"name": "p",
		"modules": [{"id": 1, "name": "m", "path": "m/"}],
		"test_plan": {
			"scenarios": [{"id": 1, "name": "S", "modules": [1, 1]}]
		}
	}`)
	if err == nil {
		t.Fatal("expected validation error for duplicate modules in test_scenario, got nil")
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
	for _, key := range []string{"name", "description", "version", "requirements", "modules", "milestones", "test_plan"} {
		if props[key] == nil {
			t.Fatalf("project schema missing property %q", key)
		}
	}

	// Check $defs keys
	defs := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "module", "milestone", "test_scenario"} {
		if defs[key] == nil {
			t.Fatalf("project schema missing $def %q", key)
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
