package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
)

// setupRenderSpec creates a multi-module fixture for render CLI tests.
func setupRenderSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"description": "A test spec",
		"requirements": [
			{"id": 1, "type": "functional", "title": "Parse input", "description": "Accept structured input."},
			{"id": 2, "type": "functional", "title": "Build output", "description": "Build output."},
			{"id": 3, "type": "non_functional", "title": "Performance", "description": "Fast."}
		],
		"modules": [
			{"id": 1, "name": "alpha", "path": "alpha", "description": "Alpha module"},
			{"id": 2, "name": "beta", "path": "beta", "description": "Beta module", "requires_module": [1]}
		],
		"milestones": [{"id": 1, "title": "MVP", "groups": [1, 2]}]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	os.MkdirAll(alphaDir, 0755)
	// TODO(bead:spexmachina-ir6): fix after spexmachina-e8t changed module IDs from int to identity hash strings
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
		"impl_sections": [
			{"id": "aabbccddeeff", "name": "Parsing Impl", "content": "impl_parsing.md", "describes": ["aabbccddeeff"]}
		],
		"data_flows": [
			{"id": "aabbccddeeff", "name": "Build Pipeline", "content": "flow_build.md", "uses": ["aabbccddeeff", "ffeeddccbbaa"]}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_parser.md", "# Parser\n\nParses input.\n")
	writeTestFile(t, alphaDir, "arch_builder.md", "# Builder\n\nBuilds output.\n")
	writeTestFile(t, alphaDir, "impl_parsing.md", "# Parsing\n\nImpl details.\n")
	writeTestFile(t, alphaDir, "flow_build.md", "# Build Pipeline\n\nData flow.\n")

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
		"impl_sections": [
			{"id": "aabbccddeeff", "name": "Consumption Impl", "content": "impl_consumption.md", "describes": ["aabbccddeeff"]}
		]
	}`
	writeTestFile(t, betaDir, "module.json", betaMod)
	writeTestFile(t, betaDir, "arch_consumer.md", "# Consumer\n\nConsumes output.\n")
	writeTestFile(t, betaDir, "impl_consumption.md", "# Consumption\n\nConsume impl.\n")

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
	for _, typ := range []string{"project", "module", "requirement", "component", "impl_section", "data_flow"} {
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

// S7: Spec directory as positional argument
func TestFR1_S7_PositionalArgument(t *testing.T) {
	dir := setupRenderSpec(t)
	out, _, err := runRenderSpex(t, "render", dir)
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !strings.Contains(out, "# test-project") {
		t.Fatal("positional arg should work")
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
	// TODO(bead:spexmachina-ir6): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	dir := t.TempDir()
	writeTestFile(t, dir, "project.json", `{
		"name": "test",
		"modules": [{"id": 1, "name": "m", "path": "m"}]
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
	if !strings.Contains(helpOut, "format") {
		t.Fatal("help should mention --format flag")
	}
}
