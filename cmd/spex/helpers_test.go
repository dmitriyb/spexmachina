package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// The wire spellings `spex diff --json` writes map onto merkle's enums one
// for one. A swapped pair here would misclassify every change of that kind
// downstream without any parse error, so each arm is pinned by value rather
// than merely exercised.
func TestParseDiffJSON_EnumSpellings(t *testing.T) {
	changeTypes := map[string]merkle.ChangeType{
		"added":    merkle.Added,
		"removed":  merkle.Removed,
		"modified": merkle.Modified,
	}
	for spelling, want := range changeTypes {
		got, err := parseChangeType(spelling)
		if err != nil {
			t.Fatalf("parseChangeType(%q): %v", spelling, err)
		}
		if got != want {
			t.Errorf("parseChangeType(%q) = %v, want %v", spelling, got, want)
		}
	}

	impactLevels := map[string]merkle.ImpactLevel{
		"impl_only":  merkle.ImplOnly,
		"contract":   merkle.Contract,
		"arch_impl":  merkle.ArchImpl,
		"structural": merkle.Structural,
	}
	for spelling, want := range impactLevels {
		got, err := parseImpactLevel(spelling)
		if err != nil {
			t.Fatalf("parseImpactLevel(%q): %v", spelling, err)
		}
		if got != want {
			t.Errorf("parseImpactLevel(%q) = %v, want %v", spelling, got, want)
		}
	}
}

// parseDiffJSON rejects a change type outside merkle's enum rather than
// defaulting it. The default arm is the only thing standing between a
// typo'd diff document and a changeset built over a silently wrong change
// type, so it is pinned directly.
func TestParseDiffJSON_UnknownChangeType(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "bogus", "impact": "impl_only", "module": "m"}]}`
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown change type") {
		t.Fatalf("want error about unknown change type, got %v", err)
	}
}

// parseDiffJSON rejects an impact level outside merkle's enum, for the same
// reason as the change type above.
func TestParseDiffJSON_UnknownImpactLevel(t *testing.T) {
	input := `{"changes": [{"path": "x", "type": "added", "impact": "bogus", "module": "m"}]}`
	_, _, err := parseDiffJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "unknown impact level") {
		t.Fatalf("want error about unknown impact level, got %v", err)
	}
}

// exitCodeOf extracts the process exit code an error carries via the
// ExitCode interface main.go honors. Zero means no code attached.
func exitCodeOf(err error) int {
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 0
}

func setupTestSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	proj := `{
		"name": "test-project",
		"modules": [
			{"id": "000000000001", "name": "alpha", "path": "alpha"}
		]
	}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}
	comp1Hash := schema.IdentityHash("alpha", "component", "Comp1")
	test1Hash := schema.IdentityHash("alpha", "test_section", "Comp1 tests")
	alphaMod := `{
		"name": "alpha",
		"components": [
			{"id": "` + comp1Hash + `", "name": "Comp1", "content": "arch_comp1.md"}
		],
		"test_sections": [
			{"id": "` + test1Hash + `", "name": "Comp1 tests", "content": "test_comp1.md", "describes": ["` + comp1Hash + `"]}
		]
	}`
	writeTestFile(t, alphaDir, "module.json", alphaMod)
	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1 architecture\n")
	writeTestFile(t, alphaDir, "test_comp1.md", "# Comp1 tests\n")

	return dir
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// captureStdout runs fn with stdout redirected and returns what was written.
// The read side drains concurrently with fn: an OS pipe's buffer is finite
// (typically 64KB), and a large write (e.g. a big JSON report) would
// otherwise block forever waiting for a reader that only starts after fn
// returns.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	outCh := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		outCh <- string(out)
	}()

	fn()

	w.Close()
	os.Stdout = old

	return <-outCh
}

// runSpex executes the full spex CLI with the given args, capturing stdout.
// Returns captured stdout and any error from cobra.
func runSpex(t *testing.T, args ...string) (string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(
		newDiffCmd(),
		newValidateCmd(),
		newMapCmd(),
		newRenderCmd(),
	)

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	var execErr error
	stdout := captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, execErr
}
