package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// --- IdentityHash tests (IH1-IH6) ---

func TestFR10_IH1_IdentityHashDeterministic(t *testing.T) {
	var results [10]string
	for i := range results {
		results[i] = IdentityHash("impact", "component", "NodeMatcher")
	}
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Fatalf("call %d returned %q, call 0 returned %q", i, results[i], results[0])
		}
	}
}

func TestFR10_IH2_IdentityHashMatchesSchemaPattern(t *testing.T) {
	pat := regexp.MustCompile(`^[a-f0-9]{12}$`)
	inputs := [][]string{
		{"a"},
		{"a", "b"},
		{"a", "b", "c"},
		{"project", "requirement", "Define project schema"},
		{"module", "schema"},
		{"schema", "requirement", "Embed schemas in binary"},
		{"schema", "component", "ProjectSchema"},
		{"schema", "component", "ModuleSchema"},
		{"schema", "component", "SchemaLoader"},
		{"schema", "impl_section", "Schema definitions"},
		{"schema", "data_flow", "ValidateSpec"},
		{"schema", "test_section", "Schema validation tests"},
		{"milestone", "Bootstrap"},
		{"test_plan", "scenario", "End-to-end validation"},
		{"validator", "component", "DAGChecker"},
		{"merkle", "component", "TreeBuilder"},
		{"impact", "component", "NodeMatcher"},
		{"apply", "component", "SnapshotSaver"},
		{"map", "component", "MappingStore"},
		{"render", "component", "DOTRenderer"},
		{"x"},
		{"x", "y"},
		{"x", "y", "z"},
		{"long-module-name", "component", "LongComponentNameThatIsVeryDescriptive"},
		{"a", "b", "c", "d", "e"},
		{"unicode", "component", "Ünïcödë"},
		{"spaces", "component", "Has Spaces"},
		{"special", "chars!@#$%"},
		{"", "nonempty"},
		{"nonempty", ""},
	}
	for _, parts := range inputs {
		h := IdentityHash(parts...)
		if !pat.MatchString(h) {
			t.Errorf("IdentityHash(%v) = %q, does not match ^[a-f0-9]{12}$", parts, h)
		}
	}
}

func TestFR10_IH3_DifferentPartsProduceDifferentHashes(t *testing.T) {
	hashes := map[string]string{}
	inputs := [][]string{
		{"a", "b", "c"},
		{"a", "b", "d"},
		{"a", "c", "b"},
		{"d", "b", "c"},
	}
	for _, parts := range inputs {
		h := IdentityHash(parts...)
		key := strings.Join(parts, "/")
		for prevKey, prevHash := range hashes {
			if h == prevHash {
				t.Fatalf("collision: %q and %q both produce %q", key, prevKey, h)
			}
		}
		hashes[key] = h
	}
}

func TestFR10_IH4_SameNameDifferentModulesDiffer(t *testing.T) {
	h1 := IdentityHash("validator", "component", "Foo")
	h2 := IdentityHash("merkle", "component", "Foo")
	if h1 == h2 {
		t.Fatalf("expected different hashes for different modules, both got %q", h1)
	}
}

func TestFR10_IH5_JoinSeparatorIsSlash(t *testing.T) {
	h := IdentityHash("a", "b")
	sum := sha256.Sum256([]byte("a/b"))
	want := hex.EncodeToString(sum[:6])
	if h != want {
		t.Fatalf("IdentityHash(\"a\",\"b\") = %q, manual sha256(\"a/b\")[:6] = %q", h, want)
	}
}

func TestFR10_IH6_EmptyPartsAndEmptyInput(t *testing.T) {
	pat := regexp.MustCompile(`^[a-f0-9]{12}$`)
	h1 := IdentityHash()
	h2 := IdentityHash("")
	h3 := IdentityHash("a", "", "b")

	for _, h := range []string{h1, h2, h3} {
		if !pat.MatchString(h) {
			t.Fatalf("degenerate input produced invalid hash %q", h)
		}
	}

	// IdentityHash() and IdentityHash("") both join to "" per the algorithm
	// spec (strings.Join), so they produce the same hash. The important
	// property is that neither panics and both produce schema-valid output.
	if h1 == h3 {
		t.Fatal("IdentityHash() and IdentityHash(\"a\",\"\",\"b\") should differ")
	}
	if h2 == h3 {
		t.Fatal("IdentityHash(\"\") and IdentityHash(\"a\",\"\",\"b\") should differ")
	}
}

// --- Schema loading integration tests (S1-S13, E1-E6, BM1-BM2) ---

func TestFR3_S1_ProjectSchemaLoads(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR3_S2_ModuleSchemaLoads(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR3_S3_ProjectSchemaStructure(t *testing.T) {
	data, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://spexmachina.dev/schema/project.json",
		"title":   "Spex Machina Project",
		"type":    "object",
	}
	for key, want := range checks {
		got, _ := raw[key].(string)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	required, _ := raw["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range required {
		reqSet[r.(string)] = true
	}
	for _, need := range []string{"name", "modules"} {
		if !reqSet[need] {
			t.Errorf("required missing %q", need)
		}
	}

	props, _ := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "version", "requirements", "modules", "milestones", "test_plan"} {
		if props[key] == nil {
			t.Errorf("properties missing %q", key)
		}
	}

	defs, _ := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "module", "milestone", "test_scenario"} {
		if defs[key] == nil {
			t.Errorf("$defs missing %q", key)
		}
	}

	if ap, ok := raw["additionalProperties"].(bool); !ok || ap {
		t.Error("additionalProperties should be false")
	}
}

func TestFR3_S4_ModuleSchemaStructure(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://spexmachina.dev/schema/module.json",
		"title":   "Spex Machina Module",
		"type":    "object",
	}
	for key, want := range checks {
		got, _ := raw[key].(string)
		if got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	required, _ := raw["required"].([]any)
	reqSet := map[string]bool{}
	for _, r := range required {
		reqSet[r.(string)] = true
	}
	if !reqSet["name"] {
		t.Error("required missing \"name\"")
	}

	props, _ := raw["properties"].(map[string]any)
	for _, key := range []string{"name", "description", "requirements", "components", "impl_sections", "data_flows", "test_sections"} {
		if props[key] == nil {
			t.Errorf("properties missing %q", key)
		}
	}

	defs, _ := raw["$defs"].(map[string]any)
	for _, key := range []string{"requirement", "component", "impl_section", "data_flow", "test_section"} {
		if defs[key] == nil {
			t.Errorf("$defs missing %q", key)
		}
	}

	if ap, ok := raw["additionalProperties"].(bool); !ok || ap {
		t.Error("additionalProperties should be false")
	}
}

func TestFR3_S5_BothSchemasIndependent(t *testing.T) {
	proj, err1 := ProjectSchema()
	mod, err2 := ModuleSchema()
	if err1 != nil {
		t.Fatalf("ProjectSchema(): %v", err1)
	}
	if err2 != nil {
		t.Fatalf("ModuleSchema(): %v", err2)
	}
	if bytes.Equal(proj, mod) {
		t.Fatal("project and module schemas are identical — expected different content")
	}

	var projRaw, modRaw map[string]any
	json.Unmarshal(proj, &projRaw)
	json.Unmarshal(mod, &modRaw)
	if projRaw["$id"] == modRaw["$id"] {
		t.Fatalf("$id values should differ, both are %v", projRaw["$id"])
	}
}

func compileSchema(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", raw); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

func validateJSON(t *testing.T, sch *jsonschema.Schema, doc []byte) error {
	t.Helper()
	var inst any
	if err := json.Unmarshal(doc, &inst); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	return sch.Validate(inst)
}

func TestFR3_S6_ProjectSchemaValidatesFixture(t *testing.T) {
	schData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_project.json")
	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("valid_project.json should pass validation: %v", err)
	}
}

func TestFR3_S7_ModuleSchemaValidatesFixture(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_module.json")
	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("valid_module.json should pass validation: %v", err)
	}
}

func TestFR3_S8_ProjectSchemaRejectsInvalid(t *testing.T) {
	schData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	// Missing required "modules"
	invalid := []byte(`{"name": "p"}`)
	if err := validateJSON(t, sch, invalid); err == nil {
		t.Fatal("expected validation error for missing modules, got nil")
	}
}

func TestFR3_S9_ModuleSchemaRejectsInvalid(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	// Missing required "name"
	invalid := []byte(`{"components": [{"id": "aabbccddeeff", "name": "C"}]}`)
	if err := validateJSON(t, sch, invalid); err == nil {
		t.Fatal("expected validation error for missing name, got nil")
	}
}

func TestFR3_S10_MinimalFixturesValidate(t *testing.T) {
	projData, err := ProjectSchema()
	if err != nil {
		t.Fatalf("ProjectSchema(): %v", err)
	}
	modData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	projSch := compileSchema(t, projData)
	modSch := compileSchema(t, modData)

	minProj := readTestdata(t, "minimal_project.json")
	if err := validateJSON(t, projSch, minProj); err != nil {
		t.Fatalf("minimal_project.json should pass: %v", err)
	}

	minMod := readTestdata(t, "minimal_module.json")
	if err := validateJSON(t, modSch, minMod); err != nil {
		t.Fatalf("minimal_module.json should pass: %v", err)
	}
}

func TestFR3_S11_ProjectSchemaIdempotent(t *testing.T) {
	d1, err1 := ProjectSchema()
	d2, err2 := ProjectSchema()
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if !bytes.Equal(d1, d2) {
		t.Fatal("ProjectSchema() returned different content on two calls")
	}
}

func TestFR3_S12_ModuleSchemaIdempotent(t *testing.T) {
	d1, err1 := ModuleSchema()
	d2, err2 := ModuleSchema()
	if err1 != nil || err2 != nil {
		t.Fatalf("errors: %v, %v", err1, err2)
	}
	if !bytes.Equal(d1, d2) {
		t.Fatal("ModuleSchema() returned different content on two calls")
	}
}

func TestFR3_S13_GoTypesUnmarshalFromValidatedFixture(t *testing.T) {
	schData, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	sch := compileSchema(t, schData)
	fixture := readTestdata(t, "valid_module.json")

	if err := validateJSON(t, sch, fixture); err != nil {
		t.Fatalf("fixture should validate: %v", err)
	}

	var mod ModuleSpec
	if err := json.Unmarshal(fixture, &mod); err != nil {
		t.Fatalf("unmarshal into Go type: %v", err)
	}
	if mod.Name == "" {
		t.Fatal("mod.Name is empty after unmarshal")
	}
	if len(mod.Components) == 0 {
		t.Fatal("mod.Components is empty after unmarshal")
	}
	if len(mod.TestSections) == 0 {
		t.Fatal("mod.TestSections is empty after unmarshal")
	}
}

// --- Edge cases (E1-E6) ---

func TestFR3_E1_SchemasNonTrivialSize(t *testing.T) {
	proj, _ := ProjectSchema()
	mod, _ := ModuleSchema()
	if len(proj) <= 100 {
		t.Fatalf("ProjectSchema() too small: %d bytes", len(proj))
	}
	if len(mod) <= 100 {
		t.Fatalf("ModuleSchema() too small: %d bytes", len(mod))
	}
}

func TestFR3_E2_SchemaStartsWithJSONObject(t *testing.T) {
	for _, load := range []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"project", ProjectSchema},
		{"module", ModuleSchema},
		{"bead-map", BeadMapSchema},
	} {
		t.Run(load.name, func(t *testing.T) {
			data, err := load.fn()
			if err != nil {
				t.Fatalf("%s: %v", load.name, err)
			}
			trimmed := bytes.TrimLeft(data, " \t\n\r")
			if len(trimmed) == 0 || trimmed[0] != '{' {
				t.Fatalf("%s schema does not start with '{'", load.name)
			}
		})
	}
}

func TestFR3_E4_ConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 20)
	errs := make([]error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ProjectSchema()
		}(i * 2)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ModuleSchema()
		}(i*2 + 1)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}
	// All project results identical
	for i := 2; i < 20; i += 2 {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatalf("project schema differs between goroutines 0 and %d", i)
		}
	}
	// All module results identical
	for i := 3; i < 20; i += 2 {
		if !bytes.Equal(results[1], results[i]) {
			t.Fatalf("module schema differs between goroutines 1 and %d", i)
		}
	}
}

func TestFR3_E5_NonExistentSchemaFile(t *testing.T) {
	_, err := schemaFS.ReadFile("nonexistent.schema.json")
	if err == nil {
		t.Fatal("expected error reading nonexistent schema file, got nil")
	}
}

func TestFR3_E6_SchemaRefsResolveToDefs(t *testing.T) {
	data, err := ModuleSchema()
	if err != nil {
		t.Fatalf("ModuleSchema(): %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)

	defs, _ := raw["$defs"].(map[string]any)
	props, _ := raw["properties"].(map[string]any)

	// Check that components.items.$ref resolves to a $defs entry
	comps, _ := props["components"].(map[string]any)
	items, _ := comps["items"].(map[string]any)
	ref, _ := items["$ref"].(string)
	if ref != "#/$defs/component" {
		t.Fatalf("components.$ref = %q, want #/$defs/component", ref)
	}
	if defs["component"] == nil {
		t.Fatal("$defs/component missing")
	}
}

// --- BeadMapSchema tests (BM1-BM2) ---

func TestFR7_BM1_BeadMapSchemaLoads(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestFR7_BM2_BeadMapAcceptsIdentityHashAndProposalRef(t *testing.T) {
	schData, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	sch := compileSchema(t, schData)

	// Identity-hash spec_node_id (component/data_flow/test_section records).
	identity := fmt.Sprintf(`{"next_id":2,"records":[{"id":1,"spec_node_id":"a1b2c3d4e5f6","bead_id":"test-abc","bead_type":"task","module":"schema","component":"Foo","content_file":"arch_foo.md","spec_hash":"%s"}]}`, strings.Repeat("a", 64))
	if err := validateJSON(t, sch, []byte(identity)); err != nil {
		t.Fatalf("identity hash spec_node_id should pass: %v", err)
	}

	// Proposal reference spec_node_id (proposal epic records).
	proposal := `{"next_id":2,"records":[{"id":1,"spec_node_id":"2026-04-12-data-flow-contract-layer","bead_id":"test-epic","bead_type":"epic","node_type":"proposal","module":"","component":"2026-04-12-data-flow-contract-layer","content_file":"","spec_hash":""}]}`
	if err := validateJSON(t, sch, []byte(proposal)); err != nil {
		t.Fatalf("proposal epic record should pass: %v", err)
	}

	// Empty spec_node_id is always rejected.
	empty := `{"next_id":2,"records":[{"id":1,"spec_node_id":"","bead_id":"test-abc","bead_type":"task","module":"schema","component":"Foo","content_file":"arch_foo.md","spec_hash":"abc"}]}`
	if err := validateJSON(t, sch, []byte(empty)); err == nil {
		t.Fatal("empty spec_node_id should fail validation")
	}
}
