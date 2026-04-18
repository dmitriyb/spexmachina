package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// setupMultiModuleSpec creates a two-module fixture matching the test spec.
func setupMultiModuleSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"description": "A test spec",
		"requirements": [
			{"id": "112233445566", "type": "functional", "title": "Parse input", "description": "Accept structured input and parse it."},
			{"id": "665544332211", "type": "functional", "title": "Build output", "description": "Build output from parsed input."},
			{"id": "778899aabbcc", "type": "non_functional", "title": "Performance", "description": "Complete within 2 seconds."}
		],
		"modules": [
			{"id": "111111111111", "name": "alpha", "path": "alpha", "description": "Alpha module"},
			{"id": "222222222222", "name": "beta", "path": "beta", "description": "Beta module", "requires_module": ["111111111111"]}
		],
		"milestones": [
			{"id": "ccbbaa998877", "title": "MVP", "description": "First release", "groups": ["111111111111", "222222222222"]}
		]
	}`
	writeFile(t, dir, "project.json", proj)

	// Alpha module
	alphaDir := filepath.Join(dir, "alpha")
	os.MkdirAll(alphaDir, 0755)
	alphaMod := `{
		"name": "alpha",
		"description": "Alpha module description",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "Parse", "preq_id": "aabbccddeeff", "depends_on": []},
			{"id": "ffeeddccbbaa", "type": "functional", "title": "Build", "preq_id": "ffeeddccbbaa"}
		],
		"components": [
			{"id": "aabbccddeeff", "name": "Parser", "description": "Parses input into AST.", "content": "arch_parser.md", "implements": ["aabbccddeeff"], "uses": []},
			{"id": "ffeeddccbbaa", "name": "Builder", "description": "Builds output from AST.", "content": "arch_builder.md", "implements": ["ffeeddccbbaa"], "uses": ["aabbccddeeff"]}
		],
		"impl_sections": [
			{"id": "aabbccddeeff", "name": "Parsing Implementation", "content": "impl_parsing.md", "describes": ["aabbccddeeff"]}
		],
		"data_flows": [
			{"id": "aabbccddeeff", "name": "Build Pipeline", "description": "Parse then build.", "content": "flow_build_pipeline.md", "uses": ["aabbccddeeff", "ffeeddccbbaa"]}
		]
	}`
	writeFile(t, alphaDir, "module.json", alphaMod)
	writeFile(t, alphaDir, "arch_parser.md", "# Parser\n\nParses input into AST.\n")
	writeFile(t, alphaDir, "arch_builder.md", "# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first.\n")
	writeFile(t, alphaDir, "impl_parsing.md", "# Parsing Implementation\n\nUse recursive descent.\n")
	writeFile(t, alphaDir, "flow_build_pipeline.md", "# Build Pipeline\n\nParse then build.\n")

	// Beta module
	betaDir := filepath.Join(dir, "beta")
	os.MkdirAll(betaDir, 0755)
	betaMod := `{
		"name": "beta",
		"description": "Beta module description",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "Consume", "preq_id": "aabbccddeeff"}
		],
		"components": [
			{"id": "aabbccddeeff", "name": "Consumer", "description": "Consumes built output.", "content": "arch_consumer.md", "implements": ["aabbccddeeff"]}
		],
		"impl_sections": [
			{"id": "aabbccddeeff", "name": "Consumption Implementation", "content": "impl_consumption.md", "describes": ["aabbccddeeff"]}
		]
	}`
	writeFile(t, betaDir, "module.json", betaMod)
	writeFile(t, betaDir, "arch_consumer.md", "# Consumer\n\nConsumes built output.\n")
	writeFile(t, betaDir, "impl_consumption.md", "# Consumption Implementation\n\nConsume the output.\n")

	return dir
}

// S1: Parse minimal valid spec into SpecGraph
func TestFR1_S1_ParseMinimalSpec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "minimal",
		"modules": [{"id": "mod1mod1mod1", "name": "mod1", "path": "mod1"}]
	}`)
	modDir := filepath.Join(dir, "mod1")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{
		"name": "mod1",
		"components": [{"id": "aabbccddeeff", "name": "Comp1", "content": "arch_comp1.md"}]
	}`)
	writeFile(t, modDir, "arch_comp1.md", "# Comp1\n")

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if graph.Project.Name != "minimal" {
		t.Fatalf("want project name 'minimal', got %q", graph.Project.Name)
	}
	if len(graph.Modules) != 1 {
		t.Fatalf("want 1 module, got %d", len(graph.Modules))
	}
	if graph.Modules[0].Spec.Name != "mod1" {
		t.Fatalf("want module name 'mod1', got %q", graph.Modules[0].Spec.Name)
	}
}

// S2: Content map populated with all markdown leaves
func TestFR1_S2_ContentMapPopulated(t *testing.T) {
	dir := setupMultiModuleSpec(t)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	alpha := graph.Modules[0]
	if len(alpha.Content) != 4 {
		t.Fatalf("want 4 content entries for alpha, got %d", len(alpha.Content))
	}

	for _, key := range []string{"arch_parser.md", "arch_builder.md", "impl_parsing.md", "flow_build_pipeline.md"} {
		if _, ok := alpha.Content[key]; !ok {
			t.Fatalf("missing content key %q", key)
		}
	}

	// Verify content is byte-identical to source
	parserContent, _ := os.ReadFile(filepath.Join(dir, "alpha", "arch_parser.md"))
	if alpha.Content["arch_parser.md"] != string(parserContent) {
		t.Fatal("content not byte-identical to source file")
	}
}

// S3: Multi-module spec with cross-module dependency
func TestFR1_S3_MultiModule(t *testing.T) {
	dir := setupMultiModuleSpec(t)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Modules) != 2 {
		t.Fatalf("want 2 modules, got %d", len(graph.Modules))
	}

	// Module ordering matches declaration order
	if graph.Modules[0].Module.Name != "alpha" {
		t.Fatalf("want first module 'alpha', got %q", graph.Modules[0].Module.Name)
	}
	if graph.Modules[1].Module.Name != "beta" {
		t.Fatalf("want second module 'beta', got %q", graph.Modules[1].Module.Name)
	}

	// Beta requires_module preserved
	if len(graph.Modules[1].Module.RequiresModule) != 1 || graph.Modules[1].Module.RequiresModule[0] != "111111111111" {
		t.Fatalf("beta should require alpha, got %v", graph.Modules[1].Module.RequiresModule)
	}
}

// S4: Project-level requirements and milestones preserved
func TestFR1_S4_ProjectRequirementsAndMilestones(t *testing.T) {
	dir := setupMultiModuleSpec(t)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Project.Requirements) != 3 {
		t.Fatalf("want 3 project requirements, got %d", len(graph.Project.Requirements))
	}

	if len(graph.Project.Milestones) != 1 {
		t.Fatalf("want 1 milestone, got %d", len(graph.Project.Milestones))
	}
	if len(graph.Project.Milestones[0].Groups) != 2 {
		t.Fatalf("want 2 groups in milestone, got %d", len(graph.Project.Milestones[0].Groups))
	}
}

// S5: All module-level edge types preserved
func TestFR1_S5_EdgeTypesPreserved(t *testing.T) {
	dir := setupMultiModuleSpec(t)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	alpha := graph.Modules[0].Spec
	// Component implements
	if len(alpha.Components[0].Implements) != 1 || alpha.Components[0].Implements[0] != "aabbccddeeff" {
		t.Fatalf("Parser should implement [aabbccddeeff], got %v", alpha.Components[0].Implements)
	}
	// Component uses
	if len(alpha.Components[1].Uses) != 1 || alpha.Components[1].Uses[0] != "aabbccddeeff" {
		t.Fatalf("Builder should use [aabbccddeeff], got %v", alpha.Components[1].Uses)
	}
	// Impl describes
	if len(alpha.ImplSections[0].Describes) != 1 || alpha.ImplSections[0].Describes[0] != "aabbccddeeff" {
		t.Fatalf("ImplSection should describe [aabbccddeeff], got %v", alpha.ImplSections[0].Describes)
	}
	// DataFlow uses
	if len(alpha.DataFlows[0].Uses) != 2 {
		t.Fatalf("DataFlow should use 2 components, got %v", alpha.DataFlows[0].Uses)
	}
	// Requirement preq_id
	if alpha.Requirements[0].PreqID != "aabbccddeeff" {
		t.Fatalf("alpha req 1 should have preq_id aabbccddeeff, got %s", alpha.Requirements[0].PreqID)
	}
}

// S6: Content with special characters and unicode
func TestFR1_S6_UnicodeContent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "unicode-test",
		"modules": [{"id": "u00000000001", "name": "u", "path": "u"}]
	}`)
	modDir := filepath.Join(dir, "u")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{
		"name": "u",
		"components": [{"id": "aabbccddeeff", "name": "Comp", "content": "arch_comp.md"}]
	}`)

	unicodeContent := "# Comp\n\nUnicode: 日本語 🎉\n\n```go\nfunc main() { fmt.Println(\"hello\") }\n```\n\n" + strings.Repeat("x", 1100) + "\n"
	writeFile(t, modDir, "arch_comp.md", unicodeContent)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if graph.Modules[0].Content["arch_comp.md"] != unicodeContent {
		t.Fatal("unicode content not preserved verbatim")
	}
}

// E1: Missing project.json
func TestFR1_E1_MissingProjectJSON(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadSpec(dir)
	if err == nil {
		t.Fatal("want error for missing project.json")
	}
	if !strings.Contains(err.Error(), "project.json") {
		t.Fatalf("error should mention project.json, got: %v", err)
	}
}

// E2: Missing module.json for declared module
func TestFR1_E2_MissingModuleJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "gamma0000001", "name": "gamma", "path": "gamma"}]
	}`)
	os.MkdirAll(filepath.Join(dir, "gamma"), 0755)

	_, err := ReadSpec(dir)
	if err == nil {
		t.Fatal("want error for missing module.json")
	}
	if !strings.Contains(err.Error(), "gamma") {
		t.Fatalf("error should mention module name 'gamma', got: %v", err)
	}
}

// E3: Missing content leaf file
func TestFR1_E3_MissingContentFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "m00000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{
		"name": "m",
		"components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_missing.md"}]
	}`)

	_, err := ReadSpec(dir)
	if err == nil {
		t.Fatal("want error for missing content file")
	}
	if !strings.Contains(err.Error(), "arch_missing.md") {
		t.Fatalf("error should mention missing file, got: %v", err)
	}
}

// E4: Malformed JSON in project.json
func TestFR1_E4_MalformedProjectJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{"name": "test",}`)

	_, err := ReadSpec(dir)
	if err == nil {
		t.Fatal("want error for malformed project.json")
	}
	if !strings.Contains(err.Error(), "project.json") {
		t.Fatalf("error should mention project.json, got: %v", err)
	}
}

// E5: Malformed JSON in module.json
func TestFR1_E5_MalformedModuleJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "m00000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{bad json`)

	_, err := ReadSpec(dir)
	if err == nil {
		t.Fatal("want error for malformed module.json")
	}
	if !strings.Contains(err.Error(), "m") {
		t.Fatalf("error should identify module, got: %v", err)
	}
}

// E6: Empty content file
func TestFR1_E6_EmptyContentFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "m00000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{
		"name": "m",
		"components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_c.md"}]
	}`)
	writeFile(t, modDir, "arch_c.md", "")

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if graph.Modules[0].Content["arch_c.md"] != "" {
		t.Fatal("empty content file should produce empty string")
	}
}

// E7: Spec directory does not exist
func TestFR1_E7_NonExistentDir(t *testing.T) {
	_, err := ReadSpec("/tmp/nonexistent-spec-dir-xyz")
	if err == nil {
		t.Fatal("want error for non-existent directory")
	}
}

// S7: Sections array parsed into SpecGraph
func TestFR1_S7_SectionsParsed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "sections-test",
		"modules": [{"id": "delivery0001", "name": "delivery", "path": "delivery"}],
		"sections": [
			{
				"id": "section00001",
				"name": "delivery",
				"type": "coupled",
				"versioning": {"scheme": "semver", "source": "git-tag"},
				"artifacts": [{"id": 1, "name": "app", "type": "binary"}]
			}
		]
	}`)
	modDir := filepath.Join(dir, "delivery")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{"name": "delivery"}`)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Project.Sections) != 1 {
		t.Fatalf("want 1 section, got %d", len(graph.Project.Sections))
	}

	sec := graph.Project.Sections[0]
	if sec.ID != "section00001" {
		t.Fatalf("want section ID section00001, got %s", sec.ID)
	}
	if sec.Name != "delivery" {
		t.Fatalf("want section name 'delivery', got %q", sec.Name)
	}
	if sec.Type != "coupled" {
		t.Fatalf("want section type 'coupled', got %q", sec.Type)
	}

	// Raw content preserved for renderer access without knowing schema
	if len(sec.Raw) == 0 {
		t.Fatal("section Raw should be populated")
	}
	var freeform map[string]any
	if err := json.Unmarshal(sec.Raw, &freeform); err != nil {
		t.Fatalf("section Raw not valid JSON: %v", err)
	}
	if _, ok := freeform["versioning"]; !ok {
		t.Fatal("section Raw should contain 'versioning' freeform field")
	}
	if _, ok := freeform["artifacts"]; !ok {
		t.Fatal("section Raw should contain 'artifacts' freeform field")
	}
}

// S8: Sections absent from project.json
func TestFR1_S8_SectionsAbsent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "no-sections",
		"modules": [{"id": "m00000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{"name": "m"}`)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Project.Sections) != 0 {
		t.Fatalf("want no sections, got %d", len(graph.Project.Sections))
	}
}

// E8: Module with no content fields
func TestFR1_E8_ModuleWithNoContent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "m00000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeFile(t, modDir, "module.json", `{
		"name": "m",
		"components": [{"id": "aabbccddeeff", "name": "C"}],
		"impl_sections": [{"id": "aabbccddeeff", "name": "I"}]
	}`)

	graph, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Modules[0].Content) != 0 {
		t.Fatalf("want empty content map, got %d entries", len(graph.Modules[0].Content))
	}
}
