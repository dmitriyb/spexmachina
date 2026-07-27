package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFR1_ProjectSchemaLoads(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ProjectSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("ProjectSchema() is not valid JSON: %v", err)
	}
	if raw["$schema"] == nil {
		t.Fatal("ProjectSchema() missing $schema field")
	}
}

func TestFR1_ModuleSchemaLoads(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ModuleSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("ModuleSchema() is not valid JSON: %v", err)
	}
	if raw["$schema"] == nil {
		t.Fatal("ModuleSchema() missing $schema field")
	}
}

func TestFR7_BeadMapSchemaLoads(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("BeadMapSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("BeadMapSchema() is not valid JSON: %v", err)
	}
	if raw["$schema"] == nil {
		t.Fatal("BeadMapSchema() missing $schema field")
	}
}

func TestFR7_BeadMapSchemaDefinesFields(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal bead-map schema: %v", err)
	}

	// Verify top-level properties
	props := raw["properties"].(map[string]any)
	for _, key := range []string{"next_id", "records"} {
		if props[key] == nil {
			t.Fatalf("bead-map schema missing top-level property %q", key)
		}
	}

	// Verify record definition has all required fields
	defs := raw["$defs"].(map[string]any)
	recordDef := defs["record"].(map[string]any)
	recordProps := recordDef["properties"].(map[string]any)
	for _, key := range []string{"id", "spec_node_id", "bead_id", "bead_type", "module", "component", "content_file", "spec_hash"} {
		if recordProps[key] == nil {
			t.Fatalf("bead-map record definition missing property %q", key)
		}
	}

	// Verify bead_status is defined (optional field)
	if recordProps["bead_status"] == nil {
		t.Fatal("bead-map record definition missing optional property \"bead_status\"")
	}

	// Verify spec_node_id enforces a non-empty string (pattern was relaxed
	// to allow both identity-hash and proposal-reference values).
	specNodeDef := recordProps["spec_node_id"].(map[string]any)
	if _, ok := specNodeDef["minLength"]; !ok {
		t.Fatal("spec_node_id missing minLength constraint")
	}
}

func TestFR7_BeadMapSchemaRejectsAdditionalProperties(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Top level
	if raw["additionalProperties"] != false {
		t.Fatal("bead-map schema should have additionalProperties: false at top level")
	}

	// Record level
	defs := raw["$defs"].(map[string]any)
	recordDef := defs["record"].(map[string]any)
	if recordDef["additionalProperties"] != false {
		t.Fatal("bead-map record definition should have additionalProperties: false")
	}
}

func TestFR2_ProjectRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"full project", "valid_project.json"},
		{"minimal project", "minimal_project.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := readTestdata(t, tt.file)
			var proj Project
			if err := json.Unmarshal(data, &proj); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.file, err)
			}
			if proj.Name == "" {
				t.Fatal("project name is empty after unmarshal")
			}
			if len(proj.Modules) == 0 {
				t.Fatal("project modules is empty after unmarshal")
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
				t.Fatalf("round-trip name mismatch: want %q, got %q", proj.Name, proj2.Name)
			}
			if len(proj.Modules) != len(proj2.Modules) {
				t.Fatalf("round-trip modules length mismatch: want %d, got %d", len(proj.Modules), len(proj2.Modules))
			}
		})
	}
}

func TestFR2_ModuleSpecRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"full module", "valid_module.json"},
		{"minimal module", "minimal_module.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := readTestdata(t, tt.file)
			var mod ModuleSpec
			if err := json.Unmarshal(data, &mod); err != nil {
				t.Fatalf("unmarshal %s: %v", tt.file, err)
			}
			if mod.Name == "" {
				t.Fatal("module name is empty after unmarshal")
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
				t.Fatalf("round-trip name mismatch: want %q, got %q", mod.Name, mod2.Name)
			}
		})
	}
}

func TestFR3_AllNodeTypes(t *testing.T) {
	// Verify the full module fixture exercises all node types.
	data := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tests := []struct {
		name  string
		count int
	}{
		{"requirements", len(mod.Requirements)},
		{"components", len(mod.Components)},
		{"impl_sections", len(mod.ImplSections)},
		{"data_flows", len(mod.DataFlows)},
		{"test_sections", len(mod.TestSections)},
		{"apis", len(mod.APIs)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.count == 0 {
				t.Fatalf("expected at least one %s in fixture, got 0", tt.name)
			}
		})
	}

	// Verify module IDs are identity hash strings (12-char hex)
	if mod.Components[0].ID == "" || len(mod.Components[0].ID) != 12 {
		t.Fatalf("component ID should be 12-char hex, got %q", mod.Components[0].ID)
	}
	if mod.Requirements[0].ID == "" || len(mod.Requirements[0].ID) != 12 {
		t.Fatalf("requirement ID should be 12-char hex, got %q", mod.Requirements[0].ID)
	}

	// Project-level node types: requirement, module, milestone.
	projData := readTestdata(t, "valid_project.json")
	var proj Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}
	if len(proj.Requirements) == 0 {
		t.Fatal("expected project requirements in fixture")
	}
	if len(proj.Modules) == 0 {
		t.Fatal("expected project modules in fixture")
	}
	if len(proj.Milestones) == 0 {
		t.Fatal("expected project milestones in fixture")
	}
	if proj.TestPlan == nil || len(proj.TestPlan.Scenarios) == 0 {
		t.Fatal("expected project test_plan with scenarios in fixture")
	}
}

func TestFR4_AllEdgeTypes(t *testing.T) {
	modData := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("unmarshal module: %v", err)
	}

	projData := readTestdata(t, "valid_project.json")
	var proj Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}

	tests := []struct {
		name  string
		found bool
	}{
		{"implements", len(mod.Components) > 0 && len(mod.Components[0].Implements) > 0},
		{"uses (component)", len(mod.Components) > 1 && len(mod.Components[1].Uses) > 0},
		{"describes", len(mod.ImplSections) > 0 && len(mod.ImplSections[0].Describes) > 0},
		{"depends_on", len(mod.Requirements) > 2 && len(mod.Requirements[2].DependsOn) > 0},
		{"preq_id", len(mod.Requirements) > 0 && mod.Requirements[0].PreqID != ""},
		{"uses (data_flow)", len(mod.DataFlows) > 0 && len(mod.DataFlows[0].Uses) > 0},
		{"groups", len(proj.Milestones) > 0 && len(proj.Milestones[0].Groups) > 0},
		{"requires_module", len(proj.Modules) > 1 && len(proj.Modules[1].RequiresModule) > 0},
		{"modules (test_scenario)", proj.TestPlan != nil && len(proj.TestPlan.Scenarios) > 0 && len(proj.TestPlan.Scenarios[0].Modules) > 0},
		{"describes (test_section)", len(mod.TestSections) > 0 && len(mod.TestSections[0].Describes) > 0},
		{"provided_by (api)", len(mod.APIs) > 0 && len(mod.APIs[0].ProvidedBy) > 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.found {
				t.Fatalf("edge type %q not exercised in test fixtures", tt.name)
			}
		})
	}
}

func TestFR5_ModuleIDsAreIdentityHashes(t *testing.T) {
	modData := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("unmarshal module: %v", err)
	}

	checkHash := func(label, id string) {
		t.Helper()
		if len(id) != 12 {
			t.Fatalf("%s ID must be 12 hex chars, got %q (len %d)", label, id, len(id))
		}
	}

	for _, r := range mod.Requirements {
		checkHash("requirement", r.ID)
		checkHash("requirement.preq_id", r.PreqID)
	}
	for _, c := range mod.Components {
		checkHash("component", c.ID)
	}
	for _, s := range mod.ImplSections {
		checkHash("impl_section", s.ID)
	}
	for _, d := range mod.DataFlows {
		checkHash("data_flow", d.ID)
	}
	for _, ts := range mod.TestSections {
		checkHash("test_section", ts.ID)
	}
	for _, a := range mod.APIs {
		checkHash("api", a.ID)
		for _, p := range a.ProvidedBy {
			checkHash("api.provided_by", p)
		}
	}
}

func TestFR5_ProjectIDsAreIdentityHashes(t *testing.T) {
	projData := readTestdata(t, "valid_project.json")
	var proj Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}
	checkHash := func(label, id string) {
		t.Helper()
		if len(id) != 12 {
			t.Fatalf("%s ID must be 12 hex chars, got %q (len %d)", label, id, len(id))
		}
	}
	for _, r := range proj.Requirements {
		checkHash("requirement", r.ID)
	}
	for _, m := range proj.Modules {
		checkHash("module", m.ID)
	}
	for _, ms := range proj.Milestones {
		checkHash("milestone", ms.ID)
	}
	if proj.TestPlan != nil {
		for _, s := range proj.TestPlan.Scenarios {
			checkHash("test_scenario", s.ID)
		}
	}
}

func TestFR6_ContentPaths(t *testing.T) {
	modData := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(modData, &mod); err != nil {
		t.Fatalf("unmarshal module: %v", err)
	}

	// The schema requires a non-empty content on components, impl_sections,
	// data_flows and test_sections; test_plan scenarios below may still omit
	// it, so the non-empty guard stays on every loop.
	var found int
	for _, c := range mod.Components {
		if c.Content != "" {
			found++
		}
	}
	for _, s := range mod.ImplSections {
		if s.Content != "" {
			found++
		}
	}
	for _, d := range mod.DataFlows {
		if d.Content != "" {
			found++
		}
	}
	for _, ts := range mod.TestSections {
		if ts.Content != "" {
			found++
		}
	}
	projData := readTestdata(t, "valid_project.json")
	var proj Project
	if err := json.Unmarshal(projData, &proj); err != nil {
		t.Fatalf("unmarshal project: %v", err)
	}
	if proj.TestPlan != nil {
		for _, s := range proj.TestPlan.Scenarios {
			if s.Content != "" {
				found++
			}
		}
	}
	if found == 0 {
		t.Fatal("expected at least one node with a content path in fixture")
	}
}

func TestFR7_SchemaDefinesNodeTypes(t *testing.T) {
	// Verify both schemas define the expected node types via $defs or properties.
	projSchema, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var projRaw map[string]any
	if err := json.Unmarshal(projSchema, &projRaw); err != nil {
		t.Fatalf("unmarshal project schema: %v", err)
	}
	props := projRaw["properties"].(map[string]any)
	for _, key := range []string{"requirements", "modules", "milestones", "test_plan"} {
		if props[key] == nil {
			t.Fatalf("project schema missing property %q", key)
		}
	}

	modSchema, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var modRaw map[string]any
	if err := json.Unmarshal(modSchema, &modRaw); err != nil {
		t.Fatalf("unmarshal module schema: %v", err)
	}
	modProps := modRaw["properties"].(map[string]any)
	for _, key := range []string{"requirements", "components", "impl_sections", "data_flows", "test_sections", "apis"} {
		if modProps[key] == nil {
			t.Fatalf("module schema missing property %q", key)
		}
	}
}

func TestFR5_TestSectionsRoundTrip(t *testing.T) {
	data := readTestdata(t, "valid_module.json")
	var mod ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(mod.TestSections) == 0 {
		t.Fatal("expected test_sections in fixture, got 0")
	}

	ts := mod.TestSections[0]
	if len(ts.ID) != 12 {
		t.Fatalf("test_section ID must be 12-char hex, got %q", ts.ID)
	}
	if ts.Name == "" {
		t.Fatal("test_section name is empty")
	}
	if ts.Content == "" {
		t.Fatal("test_section content is empty")
	}
	if len(ts.Describes) == 0 {
		t.Fatal("test_section describes is empty")
	}

	out, err := json.Marshal(&mod)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var mod2 ModuleSpec
	if err := json.Unmarshal(out, &mod2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if len(mod2.TestSections) != len(mod.TestSections) {
		t.Fatalf("round-trip test_sections length mismatch: want %d, got %d", len(mod.TestSections), len(mod2.TestSections))
	}
	if mod2.TestSections[0].ID != ts.ID {
		t.Fatalf("round-trip test_section ID mismatch: want %s, got %s", ts.ID, mod2.TestSections[0].ID)
	}
	if mod2.TestSections[0].Name != ts.Name {
		t.Fatalf("round-trip test_section name mismatch: want %q, got %q", ts.Name, mod2.TestSections[0].Name)
	}
}

func TestNegative_TypeMismatch(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		target any
	}{
		{
			"integer ID in module component fails Go unmarshal",
			`{"name":"m","components":[{"id":1,"name":"c"}]}`,
			new(ModuleSpec),
		},
		{
			"string ID in project requirement fails Go unmarshal",
			`{"name":"p","modules":[{"id":1,"name":"m","path":"m/"}],"requirements":[{"id":"x","type":"functional","title":"t"}]}`,
			new(Project),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tt.json), tt.target)
			if err == nil {
				t.Fatal("expected unmarshal error for type mismatch, got nil")
			}
		})
	}
}

func TestNegative_MissingRequired(t *testing.T) {
	// Go unmarshal doesn't enforce "required" — those are JSON Schema constraints
	// for the validator module. Verify zero-value behavior so type changes don't
	// silently pass.
	t.Run("project missing name", func(t *testing.T) {
		var proj Project
		err := json.Unmarshal([]byte(`{"modules":[{"id":"000000000001","name":"m","path":"m/"}]}`), &proj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if proj.Name != "" {
			t.Fatalf("expected empty Name, got %q", proj.Name)
		}
	})
	t.Run("module missing name", func(t *testing.T) {
		var mod ModuleSpec
		err := json.Unmarshal([]byte(`{}`), &mod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mod.Name != "" {
			t.Fatalf("expected empty Name, got %q", mod.Name)
		}
	})
	t.Run("module requirement ID is string zero-value", func(t *testing.T) {
		var mod ModuleSpec
		err := json.Unmarshal([]byte(`{"name":"m","requirements":[{"type":"functional","title":"R","preq_id":"aabbccddeeff"}]}`), &mod)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mod.Requirements[0].ID != "" {
			t.Fatalf("expected empty ID, got %q", mod.Requirements[0].ID)
		}
	})
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return data
}
