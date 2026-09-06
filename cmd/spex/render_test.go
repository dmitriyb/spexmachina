package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/internal/perf"
	"github.com/dmitriyb/spexmachina/schema"
)

// setupRenderSpec creates a multi-module fixture for render CLI tests.
func setupRenderSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"description": "A test spec",
		"requirements": [
			{"id": "000000000001", "type": "functional", "title": "Parse input", "description": "Accept structured input."},
			{"id": "000000000002", "type": "functional", "title": "Build output", "description": "Build output."},
			{"id": "000000000003", "type": "non_functional", "title": "Performance", "description": "Fast."}
		],
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha", "description": "Alpha module"},
			{"id": "000000000002", "name": "beta", "path": "beta", "description": "Beta module", "requires_module": ["000000000001"]}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	os.MkdirAll(alphaDir, 0755)
	alphaMod := `{
		"name": "alpha",
		"description": "Alpha module description",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "Parse", "preq_id": "aabbccddeeff"},
			{"id": "ffeeddccbbaa", "type": "functional", "title": "Build", "preq_id": "ffeeddccbbaa"}
		],
		"components": [
			{"id": "aabbccddeeff", "name": "Parser", "content": "arch_parser.md", "implements": ["aabbccddeeff"]},
			{"id": "ffeeddccbbaa", "name": "Builder", "content": "arch_builder.md", "implements": ["ffeeddccbbaa"], "uses": ["aabbccddeeff"]}
		],
		"data_flows": [
			{"id": "aabbccddeeff", "name": "Build Pipeline", "content": "flow_build.md", "uses": ["aabbccddeeff", "ffeeddccbbaa"]}
		],
		"test_sections": [
			{"id": "112233445566", "name": "Parser Tests", "content": "test_parser.md", "describes": ["aabbccddeeff"]}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_parser.md", "# Parser\n\nParses input.\n")
	writeTestFile(t, alphaDir, "arch_builder.md", "# Builder\n\nBuilds output.\n")
	writeTestFile(t, alphaDir, "flow_build.md", "# Build Pipeline\n\nData flow.\n")
	writeTestFile(t, alphaDir, "test_parser.md", "# Parser Tests\n\nTests the parser.\n")

	betaDir := filepath.Join(dir, "beta")
	os.MkdirAll(betaDir, 0755)
	betaMod := `{
		"name": "beta",
		"description": "Beta module description",
		"requirements": [
			{"id": "aabbccddeeff", "type": "functional", "title": "Consume", "preq_id": "aabbccddeeff"}
		],
		"components": [
			{"id": "aabbccddeeff", "name": "Consumer", "content": "arch_consumer.md", "implements": ["aabbccddeeff"]}
		],
		"test_sections": [
			{"id": "223344556677", "name": "Consumer Tests", "content": "test_consumer.md", "describes": ["aabbccddeeff"]}
		]
	}`
	writeTestFile(t, betaDir, "module.json", betaMod)
	writeTestFile(t, betaDir, "arch_consumer.md", "# Consumer\n\nConsumes output.\n")
	writeTestFile(t, betaDir, "test_consumer.md", "# Consumer Tests\n\nTests the consumer.\n")

	return dir
}

// runRenderSpex is like runSpex but includes the render command.
func runRenderSpex(t *testing.T, args ...string) (stdout string, stderr string, execErr error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newRenderCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	stderr = errBuf.String()
	return
}

// S1: Default format produces markdown
func TestFR1_S1_DefaultFormatMarkdown(t *testing.T) {
	dir := setupRenderSpec(t)
	out, _, err := runRenderSpex(t, "render", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !strings.HasPrefix(out, "# ") {
		t.Fatalf("default format should produce markdown starting with '# ', got: %.60s", out)
	}

	// Should match explicit markdown output
	mdOut, _, _ := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "markdown")
	if out != mdOut {
		t.Fatal("default format should match --format markdown")
	}
}

// S2: Explicit markdown format
func TestFR1_S2_ExplicitMarkdown(t *testing.T) {
	dir := setupRenderSpec(t)
	out, stderr, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "markdown")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty, got: %s", stderr)
	}
	if !strings.Contains(out, "# test-project") {
		t.Fatal("should contain project heading")
	}
	if !strings.Contains(out, "## Module: alpha") {
		t.Fatal("should contain module sections")
	}
	if !strings.Contains(out, "Parses input.") {
		t.Fatal("should contain inlined content")
	}
}

// S3: DOT format output
func TestFR2_S3_DOTFormat(t *testing.T) {
	dir := setupRenderSpec(t)
	out, stderr, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "dot")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty, got: %s", stderr)
	}
	if !strings.HasPrefix(out, "digraph spec {") {
		t.Fatalf("DOT output should start with 'digraph spec {', got: %.60s", out)
	}
	if !strings.Contains(out, "subgraph cluster_alpha") {
		t.Fatal("should contain cluster_alpha")
	}
	if !strings.Contains(out, "subgraph cluster_beta") {
		t.Fatal("should contain cluster_beta")
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "}") {
		t.Fatal("should end with '}'")
	}
}

// S4: JSON format output
func TestFR3_S4_JSONFormat(t *testing.T) {
	dir := setupRenderSpec(t)
	out, stderr, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty, got: %s", stderr)
	}

	var result struct {
		Nodes []json.RawMessage `json:"nodes"`
		Edges []json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("nodes should not be empty")
	}

	// Check node types present
	types := make(map[string]bool)
	for _, raw := range result.Nodes {
		var n struct{ Type string }
		json.Unmarshal(raw, &n)
		types[n.Type] = true
	}
	for _, typ := range []string{"project", "module", "requirement", "component", "data_flow", "test_section"} {
		if !types[typ] {
			t.Errorf("missing node type %q", typ)
		}
	}
}

// S5: Output is written to stdout only (composable)
func TestNFR4_S5_StdoutOnly(t *testing.T) {
	dir := setupRenderSpec(t)

	// List files before
	before, _ := os.ReadDir(dir)
	beforeNames := make(map[string]bool)
	for _, e := range before {
		beforeNames[e.Name()] = true
	}

	_, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// List files after
	after, _ := os.ReadDir(dir)
	for _, e := range after {
		if !beforeNames[e.Name()] {
			t.Fatalf("render created file %q — should not create any files", e.Name())
		}
	}
}

// S7: Positional arguments are rejected — the spec directory comes from --spec-dir only
func TestFR1_S7_PositionalArgumentRejected(t *testing.T) {
	dir := setupRenderSpec(t)
	_, _, err := runRenderSpex(t, "render", dir)
	if err == nil {
		t.Fatal("positional argument should be rejected, got no error")
	}
}

// S8: Current directory as implicit spec root
func TestFR1_S8_CurrentDirectory(t *testing.T) {
	dir := setupRenderSpec(t)

	// Change to spec dir
	old, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(old) })

	out, _, err := runRenderSpex(t, "render", "--spec-dir", ".")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !strings.Contains(out, "# test-project") {
		t.Fatal("should render from current directory")
	}
}

// S9: Markdown output round-trip consistency
func TestNFR4_S9_MarkdownDeterminism(t *testing.T) {
	dir := setupRenderSpec(t)

	out1, _, _ := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "markdown")
	out2, _, _ := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "markdown")
	if out1 != out2 {
		t.Fatal("markdown output should be deterministic")
	}
}

// S10: JSON output determinism
func TestNFR4_S10_JSONDeterminism(t *testing.T) {
	dir := setupRenderSpec(t)

	out1, _, _ := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	out2, _, _ := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	if out1 != out2 {
		t.Fatal("JSON output should be byte-identical across runs")
	}
}

// E1: Invalid format flag
func TestFR1_E1_InvalidFormat(t *testing.T) {
	dir := setupRenderSpec(t)
	out, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "xml")
	if err == nil {
		t.Fatal("want error for invalid format")
	}
	if out != "" {
		t.Fatal("stdout should be empty on error")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "xml") {
		t.Fatalf("error should mention 'xml', got: %v", err)
	}
	if !strings.Contains(errMsg, "markdown") || !strings.Contains(errMsg, "dot") || !strings.Contains(errMsg, "json") {
		t.Fatalf("error should list valid formats, got: %v", err)
	}
}

// E2: Non-existent spec directory
func TestFR1_E2_NonExistentDir(t *testing.T) {
	out, _, err := runRenderSpex(t, "render", "--spec-dir", "/tmp/nonexistent-spec-xyz")
	if err == nil {
		t.Fatal("want error for non-existent dir")
	}
	if out != "" {
		t.Fatal("stdout should be empty on error")
	}
	if !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("error should indicate the directory does not exist, got: %v", err)
	}
}

// E3: Spec directory missing project.json
func TestFR1_E3_MissingProjectJSON(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runRenderSpex(t, "render", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for missing project.json")
	}
	if out != "" {
		t.Fatal("stdout should be empty on error")
	}
	if !strings.Contains(err.Error(), "project.json") {
		t.Fatalf("error should mention project.json, got: %v", err)
	}
}

// E4: Spec with broken content reference
func TestFR1_E4_BrokenContentRef(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": "000000000001", "name": "m", "path": "m"}]
	}`)
	modDir := filepath.Join(dir, "m")
	os.MkdirAll(modDir, 0755)
	writeTestFile(t, modDir, "module.json", `{
		"name": "m",
		"components": [{"id": "aabbccddeeff", "name": "C", "content": "arch_missing.md"}]
	}`)

	out, _, err := runRenderSpex(t, "render", "--spec-dir", dir)
	if err == nil {
		t.Fatal("want error for missing content file")
	}
	if out != "" {
		t.Fatal("stdout should be empty on error")
	}
	if !strings.Contains(err.Error(), "arch_missing.md") {
		t.Fatalf("error should mention missing file, got: %v", err)
	}
}

// E7: Exit code contract (error returns non-nil error, success returns nil)
func TestFR1_E7_ExitCodeContract(t *testing.T) {
	dir := setupRenderSpec(t)

	// Success case
	_, _, err := runRenderSpex(t, "render", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("success case should return nil error, got: %v", err)
	}

	// Error case
	_, _, err = runRenderSpex(t, "render", "--spec-dir", "/tmp/nonexistent-xyz")
	if err == nil {
		t.Fatal("error case should return non-nil error")
	}
}

// E8: Empty flag value
func TestFR1_E8_EmptyFlagValue(t *testing.T) {
	dir := setupRenderSpec(t)
	_, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "")
	if err == nil {
		// If no error, it should default to markdown (either behavior acceptable per spec)
		return
	}
	// If error, should mention invalid format
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("error should mention format, got: %v", err)
	}
}

// E9: Help flag
func TestFR1_E9_HelpFlag(t *testing.T) {
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newRenderCmd())

	var helpBuf bytes.Buffer
	rootCmd.SetOut(&helpBuf)
	rootCmd.SetArgs([]string{"render", "--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("help should not error, got: %v", err)
	}

	helpOut := helpBuf.String()
	if !strings.Contains(helpOut, "render") {
		t.Fatal("help should describe the render command")
	}
	for _, format := range []string{"markdown", "dot", "json"} {
		if !strings.Contains(helpOut, format) {
			t.Fatalf("help should list %q as an available --format option, got: %s", format, helpOut)
		}
	}
	if !strings.Contains(helpOut, "Usage:\n  spex render [flags]") {
		t.Fatalf("usage line should name the command and flags only, with no positional argument, got: %s", helpOut)
	}
}

// S11: --format json --slim emits nodes only, compact, without content
func TestFR3_S11_SlimJSONFlag(t *testing.T) {
	dir := setupRenderSpec(t)
	out, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json", "--slim")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var slim map[string]any
	if err := json.Unmarshal([]byte(out), &slim); err != nil {
		t.Fatalf("slim output is not valid JSON: %v\n%s", err, out)
	}
	if _, ok := slim["edges"]; ok {
		t.Error("--slim must not emit edges")
	}
	nodes, ok := slim["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		t.Fatalf("--slim should emit a non-empty nodes array, got: %s", out)
	}
	for _, n := range nodes {
		obj := n.(map[string]any)
		for _, banned := range []string{"content", "description"} {
			if _, present := obj[banned]; present {
				t.Errorf("--slim node still carries %q: %#v", banned, obj)
			}
		}
	}

	// The same spec rendered without --slim inlines content, so --slim must be
	// substantially smaller.
	full, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if len(out) >= len(full) {
		t.Errorf("--slim output (%d bytes) should be smaller than full JSON (%d bytes)", len(out), len(full))
	}
	if strings.Contains(out, "Parses input.") {
		t.Errorf("--slim must not inline content leaves, got:\n%s", out)
	}
}

// E5: Large spec performance — 15 modules, 8 components each, content
// leaves averaging 2KB, renders within the 2-second budget and every
// module, component and content leaf reaches the output.
func TestFR3_E5_LargeSpecPerformance(t *testing.T) {
	const modules = 15
	const compsPerModule = 8
	dir := buildLargeRenderSpec(t, modules, compsPerModule)

	var (
		out string
		err error
	)
	perf.Within(t, 2*time.Second, func() {
		out, _, err = runRenderSpex(t, "render", "--spec-dir", dir, "--format", "json")
	})
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	var result struct {
		Nodes []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	var moduleCount, compCount int
	for _, n := range result.Nodes {
		switch n.Type {
		case "module":
			moduleCount++
		case "component":
			compCount++
			if len(n.Content) < 2000 {
				t.Fatalf("component node missing its inlined ~2KB content: got %d bytes", len(n.Content))
			}
		}
	}
	if moduleCount != modules {
		t.Fatalf("want %d module nodes in the output, got %d", modules, moduleCount)
	}
	if compCount != modules*compsPerModule {
		t.Fatalf("want %d component nodes in the output, got %d", modules*compsPerModule, compCount)
	}
}

// buildLargeRenderSpec writes a spec with the given number of modules, each
// carrying compsPerModule components with a content leaf averaging 2KB, per
// test_render_command.md E5's fixture.
func buildLargeRenderSpec(t *testing.T, modules, compsPerModule int) string {
	t.Helper()
	dir := t.TempDir()

	proj := schema.Project{Name: "large-project"}
	for m := 0; m < modules; m++ {
		modName := fmt.Sprintf("module%d", m)
		modID := fmt.Sprintf("%012x", m+1)
		proj.Modules = append(proj.Modules, schema.Module{ID: modID, Name: modName, Path: modName})

		modDir := filepath.Join(dir, modName)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatal(err)
		}

		modSpec := schema.ModuleSpec{Name: modName}
		for c := 0; c < compsPerModule; c++ {
			name := fmt.Sprintf("Comp%d", c)
			id := schema.IdentityHash(modName, "component", name)
			content := fmt.Sprintf("arch_comp%d.md", c)
			modSpec.Components = append(modSpec.Components, schema.Component{
				ID:      id,
				Name:    name,
				Content: content,
			})
			writeTestFile(t, modDir, content, largeContentLeaf(name))
		}

		modBytes, err := json.Marshal(modSpec)
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, modDir, "module.json", string(modBytes))
	}

	projBytes, err := json.Marshal(proj)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "project.json", string(projBytes))

	return dir
}

// largeContentLeaf returns a ~2KB markdown body for name, matching E5's
// "content leaves averaging 2KB" fixture.
func largeContentLeaf(name string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s architecture\n\n", name)
	const line = "Lorem ipsum dolor sit amet, consectetur adipiscing elit.\n"
	for b.Len() < 2000 {
		b.WriteString(line)
	}
	return b.String()
}

// E10: --slim is rejected for non-JSON formats
func TestFR3_E10_SlimRequiresJSON(t *testing.T) {
	dir := setupRenderSpec(t)
	for _, format := range []string{"markdown", "dot"} {
		_, _, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", format, "--slim")
		if err == nil {
			t.Fatalf("--slim with --format %s should fail", format)
		}
		if !strings.Contains(err.Error(), "--slim requires --format json") {
			t.Errorf("want a --slim/--format json error, got %v", err)
		}
	}
}

// TestFR2_S3b_DOTShapesAndEdgeLabels pins the DOT contract beyond the
// digraph wrapper S3 asserts: every declared kind is drawn with its
// arch_dot_renderer.md shape, every node's label is its declared name,
// every edge carries a label naming its kind, and the graph is laid out
// left to right.
func TestFR2_S3b_DOTShapesAndEdgeLabels(t *testing.T) {
	dir := setupRenderSpec(t)
	out, stderr, err := runRenderSpex(t, "render", "--spec-dir", dir, "--format", "dot")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if stderr != "" {
		t.Fatalf("stderr should be empty, got: %s", stderr)
	}
	if strings.Count(out, "rankdir=LR;") != 1 {
		t.Fatalf("want rankdir=LR exactly once, got %d in:\n%s", strings.Count(out, "rankdir=LR;"), out)
	}
	wantNodes := []string{
		`[label="alpha", shape=folder`,
		`[label="beta", shape=folder`,
		`[label="Parse", shape=box`,
		`[label="Build", shape=box`,
		`[label="Parser", shape=component`,
		`[label="Builder", shape=component`,
		`[label="Build Pipeline", shape=ellipse`,
	}
	for _, w := range wantNodes {
		if !strings.Contains(out, w) {
			t.Errorf("DOT output should declare %s", w)
		}
	}
	wantEdges := []string{
		`[label="implements"`,
		`[label="uses", style=dotted`,
		`[label="requires_module"]`,
		`[label="preq_id", style=dashed`,
	}
	for _, w := range wantEdges {
		if !strings.Contains(out, w) {
			t.Errorf("DOT output should carry an edge %s", w)
		}
	}
	if t.Failed() {
		t.Logf("output was:\n%s", out)
	}
}
