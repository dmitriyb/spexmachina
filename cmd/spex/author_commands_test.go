package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/author"
	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/spf13/cobra"
)

// This file is spec/author/test_author_commands.md, the "Author command
// tests" test section (0555a6152590) — the surface AuthorCommands (1f9fec7c)
// itself presents: exit codes, stdout shape, flag handling, as opposed to
// what each worker does with the values, which the other test leaves in
// this module (test_node_editing.md, test_leaf_and_profile.md,
// test_migration.md, test_obligation.md) cover from inside the author
// package directly.

// authorCmdFixture is the Setup fixture test_author_commands.md names: "a
// tmp/spec/ with module alpha, components Comp1 and Comp2, test section T1
// and api demo run" — the same shape author.buildObligationFixture builds,
// reconstructed here from schema's exported types since this package
// cannot reach that unexported test helper.
type authorCmdFixture struct {
	dir              string
	comp1ID, comp2ID string
	t1ID             string
	apiID            string
}

func buildAuthorCmdFixture(t *testing.T, dir string) authorCmdFixture {
	t.Helper()

	f := authorCmdFixture{
		dir:     dir,
		comp1ID: schema.IdentityHash("alpha", "component", "Comp1"),
		comp2ID: schema.IdentityHash("alpha", "component", "Comp2"),
		t1ID:    schema.IdentityHash("alpha", "test_section", "T1"),
		apiID:   schema.IdentityHash("alpha", "api", "demo run"),
	}

	proj := schema.Project{
		Name: "test-project",
		Modules: []schema.Module{
			{ID: "000000000001", Name: "alpha", Path: "alpha"},
		},
	}
	writeAuthorJSON(t, filepath.Join(dir, "project.json"), proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}

	mod := schema.ModuleSpec{
		Name: "alpha",
		Components: []schema.Component{
			{ID: f.comp1ID, Name: "Comp1", Content: "arch_comp1.md"},
			{ID: f.comp2ID, Name: "Comp2", Content: "arch_comp2.md", Uses: []string{f.comp1ID}},
		},
		TestSections: []schema.TestSection{
			{ID: f.t1ID, Name: "T1", Content: "test_t1.md", Describes: []string{f.comp1ID, f.comp2ID}},
		},
		APIs: []schema.API{
			{ID: f.apiID, Name: "demo run", ProvidedBy: []string{f.comp1ID}},
		},
	}
	writeAuthorJSON(t, filepath.Join(alphaDir, "module.json"), mod)

	writeTestFile(t, alphaDir, "arch_comp1.md", "# Comp1\n")
	writeTestFile(t, alphaDir, "arch_comp2.md", "# Comp2\n\nUses [["+f.comp1ID+"|Comp1]].\n")
	writeTestFile(t, alphaDir, "test_t1.md", "# T1\n\nDescribes [["+f.comp1ID+"|Comp1]] and [["+f.comp2ID+"|Comp2]].\n")

	return f
}

func writeAuthorJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// newAuthorRootCmd assembles the eight AuthorCommands surfaces plus
// validate, the tree every scenario in this file drives.
func newAuthorRootCmd() *cobra.Command {
	root := cli.NewRootCmd()
	root.AddCommand(newNodeCmd(), newEdgeCmd(), newLeafCmd(), newProfileCmd(), newMigrateCmd(), newValidateCmd())
	return root
}

// runAuthorSpex runs the AuthorCommands tree with args, capturing stdout
// and stderr separately — A2 asserts on both.
func runAuthorSpex(t *testing.T, args ...string) (stdout, stderr string, execErr error) {
	t.Helper()
	root := newAuthorRootCmd()

	errBuf := new(bytes.Buffer)
	root.SetErr(errBuf)
	root.SetArgs(args)

	stdout = captureStdout(t, func() {
		execErr = root.Execute()
	})
	stderr = errBuf.String()
	return
}

// A1: Every surface is registered and has help.
func TestA1_EverySurfaceRegisteredAndHasHelp(t *testing.T) {
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		out, _, err := runAuthorSpex(t, args...)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", args, err)
		}
		return out
	}

	rootHelp := run(t, "--help")
	for _, name := range []string{"node", "edge", "leaf", "profile", "migrate"} {
		if !strings.Contains(rootHelp, name) {
			t.Errorf("spex --help does not list %q:\n%s", name, rootHelp)
		}
	}

	nodeHelp := run(t, "node", "--help")
	for _, name := range []string{"add", "remove", "rename"} {
		if !strings.Contains(nodeHelp, name) {
			t.Errorf("spex node --help does not list %q:\n%s", name, nodeHelp)
		}
	}

	edgeHelp := run(t, "edge", "--help")
	for _, name := range []string{"add", "remove"} {
		if !strings.Contains(edgeHelp, name) {
			t.Errorf("spex edge --help does not list %q:\n%s", name, edgeHelp)
		}
	}

	leafHelp := run(t, "leaf", "--help")
	if !strings.Contains(leafHelp, "scaffold") {
		t.Errorf("spex leaf --help does not list \"scaffold\":\n%s", leafHelp)
	}

	profileHelp := run(t, "profile", "--help")
	if !strings.Contains(profileHelp, "show") {
		t.Errorf("spex profile --help does not list \"show\":\n%s", profileHelp)
	}

	surfaces := [][]string{
		{"node", "add", "--help"},
		{"node", "remove", "--help"},
		{"node", "rename", "--help"},
		{"edge", "add", "--help"},
		{"edge", "remove", "--help"},
		{"leaf", "scaffold", "--help"},
		{"profile", "show", "--help"},
		{"migrate", "--help"},
	}
	for _, args := range surfaces {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			out, _, err := runAuthorSpex(t, args...)
			if err != nil {
				t.Fatalf("%v: want exit 0, got error: %v", args, err)
			}
			if !strings.Contains(out, "Usage:") {
				t.Errorf("%v: want usage in help output, got:\n%s", args, out)
			}
		})
	}

	// A bare grouping prints that grouping's help and exits 0.
	for _, name := range []string{"node", "edge", "leaf", "profile"} {
		t.Run("bare_"+name, func(t *testing.T) {
			out, _, err := runAuthorSpex(t, name)
			if err != nil {
				t.Fatalf("bare spex %s: want exit 0, got error: %v", name, err)
			}
			if !strings.Contains(out, "Usage:") {
				t.Errorf("bare spex %s: want its grouping help, got:\n%s", name, out)
			}
		})
	}
}

// A2: Exit codes are the documented set — 0, 2, 1, 1 across a successful
// node add, a refused one (undeclared type), a missing required flag, and
// a directory holding no project.json; a malformed spec/profile.json is a
// fifth run and also exits 1.
func TestA2_ExitCodesAreDocumentedSet(t *testing.T) {
	t.Run("successful write exits 0", func(t *testing.T) {
		dir := t.TempDir()
		buildAuthorCmdFixture(t, dir)

		out, errOut, err := runAuthorSpex(t, "node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", dir)
		if err != nil {
			t.Fatalf("want exit 0, got error: %v (stderr: %s)", err, errOut)
		}
		if exitCodeOf(err) != 0 {
			t.Errorf("want exit code 0, got %d", exitCodeOf(err))
		}
		var report author.WriteReport
		if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
			t.Fatalf("stdout is not a write report: %v\n%s", jerr, out)
		}
		if len(report.Written) == 0 {
			t.Errorf("want a non-empty written list, got %+v", report)
		}
	})

	t.Run("refusal exits 2 with the error document on stdout", func(t *testing.T) {
		dir := t.TempDir()
		buildAuthorCmdFixture(t, dir)

		out, errOut, err := runAuthorSpex(t, "node", "add", "Nope", "--type", "bogus_type", "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for an undeclared type")
		}
		if exitCodeOf(err) != author.ExitRefusal {
			t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
		}
		if strings.Contains(errOut, "Usage:") {
			t.Errorf("want no usage block on stderr, got:\n%s", errOut)
		}
		if strings.Count(strings.TrimRight(errOut, "\n"), "\n") != 0 {
			t.Errorf("want exactly one stderr line, got:\n%s", errOut)
		}

		var refusals []author.RefusalEntry
		if jerr := json.Unmarshal([]byte(out), &refusals); jerr != nil {
			t.Fatalf("stdout is not a refusal document: %v\n%s", jerr, out)
		}
		if len(refusals) == 0 {
			t.Fatal("want at least one refusal entry")
		}
		for _, r := range refusals {
			if r.Fix == "" {
				t.Errorf("refusal entry carries no fix: %+v", r)
			}
		}
	})

	t.Run("missing required flag exits 1", func(t *testing.T) {
		dir := t.TempDir()
		buildAuthorCmdFixture(t, dir)

		out, errOut, err := runAuthorSpex(t, "node", "add", "Widget", "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for a missing --type")
		}
		if exitCodeOf(err) != 0 {
			t.Errorf("want the default exit code (1), got explicit code %d", exitCodeOf(err))
		}
		if out != "" {
			t.Errorf("want no stdout for an input error, got: %s", out)
		}
		if strings.Contains(errOut, "Usage:") {
			t.Errorf("want no usage block on stderr, got:\n%s", errOut)
		}
	})

	t.Run("directory holding no project.json exits 1", func(t *testing.T) {
		dir := t.TempDir()

		out, errOut, err := runAuthorSpex(t, "node", "add", "New", "--type", "requirement", "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for a spec dir with no project.json")
		}
		if exitCodeOf(err) != 0 {
			t.Errorf("want the default exit code (1), got explicit code %d", exitCodeOf(err))
		}
		if out != "" {
			t.Errorf("want no stdout for an input error, got: %s", out)
		}
		if strings.Contains(errOut, "Usage:") {
			t.Errorf("want no usage block on stderr, got:\n%s", errOut)
		}
	})

	t.Run("malformed spec/profile.json exits 1", func(t *testing.T) {
		dir := t.TempDir()
		buildAuthorCmdFixture(t, dir)
		writeTestFile(t, dir, "profile.json", `{"profile_version": 99, "node_types": []}`)

		out, _, err := runAuthorSpex(t, "profile", "show", "--spec-dir", dir)
		if err == nil {
			t.Fatal("want a non-zero exit for a malformed profile.json")
		}
		if exitCodeOf(err) != 0 {
			t.Errorf("want the default exit code (1), got explicit code %d", exitCodeOf(err))
		}
		if out != "" {
			t.Errorf("want no stdout for an input error, got: %s", out)
		}
	})
}

// A3: Output is machine-readable when piped — one compact JSON document,
// parseable with no prose around it.
func TestA3_OutputMachineReadableWhenPiped(t *testing.T) {
	dir := t.TempDir()
	buildAuthorCmdFixture(t, dir)

	out, _, err := runAuthorSpex(t, "profile", "show", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("profile show: unexpected error: %v", err)
	}
	assertCompactJSONDoc(t, "profile show", out)

	out, _, err = runAuthorSpex(t, "node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add: unexpected error: %v", err)
	}
	assertCompactJSONDoc(t, "node add", out)

	var report map[string]any
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
		t.Fatalf("node add output is not valid JSON: %v\n%s", jerr, out)
	}
	if _, ok := report["obligations"]; !ok {
		t.Errorf("write report carries no obligations key: %s", out)
	}
}

// assertCompactJSONDoc checks that s is exactly one line of valid JSON with
// no surrounding prose — jq's "one compact document" requirement.
func assertCompactJSONDoc(t *testing.T, label, s string) {
	t.Helper()
	trimmed := strings.TrimRight(s, "\n")
	if strings.Contains(trimmed, "\n") {
		t.Errorf("%s: want one compact JSON line, got multiple lines:\n%s", label, s)
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		t.Fatalf("%s: not valid JSON: %v\n%s", label, err, s)
	}
}

// A4: A run is deterministic — two byte-identical fixture copies, N8's
// rename run over each, produce byte-identical trees and byte-identical
// stdouts.
func TestA4_RunIsDeterministic(t *testing.T) {
	dir1 := t.TempDir()
	f1 := buildAuthorCmdFixture(t, dir1)
	dir2 := t.TempDir()
	buildAuthorCmdFixture(t, dir2)

	out1, _, err1 := runAuthorSpex(t, "node", "rename", f1.comp1ID, "Core", "--spec-dir", dir1)
	if err1 != nil {
		t.Fatalf("rename over dir1: unexpected error: %v", err1)
	}
	out2, _, err2 := runAuthorSpex(t, "node", "rename", f1.comp1ID, "Core", "--spec-dir", dir2)
	if err2 != nil {
		t.Fatalf("rename over dir2: unexpected error: %v", err2)
	}

	if out1 != out2 {
		t.Errorf("stdouts differ:\ndir1: %s\ndir2: %s", out1, out2)
	}
	assertTreesByteIdentical(t, dir1, dir2)
}

// assertTreesByteIdentical walks both directories and fails t if either the
// file sets or any file's bytes differ.
func assertTreesByteIdentical(t *testing.T, dir1, dir2 string) {
	t.Helper()
	files1 := readTreeFiles(t, dir1)
	files2 := readTreeFiles(t, dir2)

	if len(files1) != len(files2) {
		t.Fatalf("file count differs: dir1 has %d, dir2 has %d", len(files1), len(files2))
	}
	for rel, data1 := range files1 {
		data2, ok := files2[rel]
		if !ok {
			t.Errorf("%s exists in dir1 but not dir2", rel)
			continue
		}
		if !bytes.Equal(data1, data2) {
			t.Errorf("%s differs between dir1 and dir2", rel)
		}
	}
	for rel := range files2 {
		if _, ok := files1[rel]; !ok {
			t.Errorf("%s exists in dir2 but not dir1", rel)
		}
	}
}

func readTreeFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// A5: The retired name of a rename reaches stdout as data.
func TestA5_RetiredNameReachesStdoutAsData(t *testing.T) {
	dir := t.TempDir()
	f := buildAuthorCmdFixture(t, dir)

	out, _, err := runAuthorSpex(t, "node", "rename", f.comp1ID, "Core", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("rename: unexpected error: %v", err)
	}

	var report author.WriteReport
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
		t.Fatalf("stdout is not a write report: %v\n%s", jerr, out)
	}
	if report.RetiredName != "Comp1" {
		t.Errorf("RetiredName = %q, want %q", report.RetiredName, "Comp1")
	}
}

// A6: --spec-dir is honoured on every surface — running each of the eight
// surfaces against a second fixture leaves a first fixture at a different
// path byte-identical before and after.
func TestA6_SpecDirIsolation(t *testing.T) {
	control := t.TempDir()
	buildAuthorCmdFixture(t, control)
	controlSnapshot := readTreeFiles(t, control)

	assertControlUnchanged := func(t *testing.T) {
		t.Helper()
		after := readTreeFiles(t, control)
		if len(after) != len(controlSnapshot) {
			t.Fatalf("control fixture file count changed: was %d, now %d", len(controlSnapshot), len(after))
		}
		for rel, data := range controlSnapshot {
			got, ok := after[rel]
			if !ok || !bytes.Equal(got, data) {
				t.Errorf("control fixture file %s changed after a run against the other --spec-dir", rel)
			}
		}
	}

	type surfaceCase struct {
		name string
		args func(f authorCmdFixture, dir string) []string
	}
	cases := []surfaceCase{
		{"node add", func(f authorCmdFixture, dir string) []string {
			return []string{"node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", dir}
		}},
		{"node remove", func(f authorCmdFixture, dir string) []string {
			return []string{"node", "remove", f.comp2ID, "--force", "--spec-dir", dir}
		}},
		{"node rename", func(f authorCmdFixture, dir string) []string {
			return []string{"node", "rename", f.comp1ID, "Core", "--spec-dir", dir}
		}},
		{"edge add", func(f authorCmdFixture, dir string) []string {
			return []string{"edge", "add", f.t1ID, "describes", f.comp1ID, "--spec-dir", dir}
		}},
		{"edge remove", func(f authorCmdFixture, dir string) []string {
			return []string{"edge", "remove", f.comp2ID, "uses", f.comp1ID, "--spec-dir", dir}
		}},
		{"leaf scaffold", func(f authorCmdFixture, dir string) []string {
			return []string{"leaf", "scaffold", f.comp1ID, "--spec-dir", dir}
		}},
		{"profile show", func(f authorCmdFixture, dir string) []string {
			return []string{"profile", "show", "--spec-dir", dir}
		}},
		{"migrate", func(f authorCmdFixture, dir string) []string {
			return []string{"migrate", "--spec-dir", dir}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			other := t.TempDir()
			f := buildAuthorCmdFixture(t, other)

			// The surface may succeed, no-op or refuse — A6 asserts only
			// that the control fixture at the other path is untouched,
			// regardless of the outcome against `other`.
			runAuthorSpex(t, c.args(f, other)...)

			assertControlUnchanged(t)
		})
	}
}
