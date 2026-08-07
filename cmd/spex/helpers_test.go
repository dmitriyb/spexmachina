package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/schema"
)

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
		newImpactCmd(),
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
