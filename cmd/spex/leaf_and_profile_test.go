package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/author"
	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// This file is cmd/spex's half of spec/author/test_leaf_and_profile.md — the
// "Leaf and profile tests" test section, which describes both LeafScaffolder
// and ProfileInspector: L5 and L6 exercise `spex profile show` alone; L1-L4,
// L7 and L8 drive `spex leaf scaffold` (L8 through `spex node add` first),
// now that AuthorCommands and NodeEditor exist to give those surfaces a
// command tree to run against.

// runProfileSpex is like runSpex (helpers_test.go) but carries the profile
// and validate command trees, mirroring runRenderSpex's pattern
// (render_test.go) — the leaf's Setup says every scenario assembles the
// command tree in place rather than spawning a process.
func runProfileSpex(t *testing.T, args ...string) (stdout string, execErr error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newProfileCmd(), newValidateCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, execErr
}

// runLeafSpex is runProfileSpex's counterpart for L1-L4, L7 and L8: it also
// carries the node and diff command trees, since those scenarios drive
// `spex leaf scaffold`, `spex node add` and (L1) `spex diff --json` over the
// same fixture.
func runLeafSpex(t *testing.T, args ...string) (stdout string, execErr error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newNodeCmd(), newLeafCmd(), newProfileCmd(), newDiffCmd(), newValidateCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, execErr
}

// leafRepoRoot resolves this repository's root from cmd/spex, the same way
// delivery/release_build_test.go's repoRootDir resolves it from delivery —
// so scripts/link-check.sh can be found regardless of the test binary's
// working directory.
func leafRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return dir
}

// runLinkCheck runs scripts/link-check.sh over specDir and diffJSONPath,
// returning its combined output and exit error — L1's parity oracle that
// what LeafScaffolder wrote actually links the node's declared edges, run
// the same way a human reviewer would run it.
func runLinkCheck(t *testing.T, specDir, diffJSONPath string) (string, error) {
	t.Helper()
	script := filepath.Join(leafRepoRoot(t), "scripts", "link-check.sh")
	cmd := exec.Command("bash", script, specDir, diffJSONPath)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// decodeJSONDoc unmarshals a profile-show document into a generic value for
// structural (JSON-value) comparison, since the acceptance criteria compare
// "equal as a JSON value" rather than byte-identical text.
func decodeJSONDoc(t *testing.T, label, doc string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("%s: not valid JSON: %v\n%s", label, err, doc)
	}
	return v
}

// contentBearingNodeTypes returns the node_types entries of a decoded
// profile document whose requires_content is true.
func contentBearingNodeTypes(t *testing.T, doc map[string]any) []map[string]any {
	t.Helper()
	raw, ok := doc["node_types"].([]any)
	if !ok {
		t.Fatalf("profile document carries no node_types array: %v", doc)
	}
	var out []map[string]any
	for _, nt := range raw {
		m, ok := nt.(map[string]any)
		if !ok {
			t.Fatalf("node_types entry is not an object: %v", nt)
		}
		if rc, _ := m["requires_content"].(bool); rc {
			out = append(out, m)
		}
	}
	return out
}

// L5: The printed profile is the resolved profile.
func TestREQ_7f193910f7ef_L5_PrintedProfileIsResolvedProfile(t *testing.T) {
	specDir := setupTestSpec(t)

	out, err := runProfileSpex(t, "profile", "show", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("profile show: unexpected error: %v", err)
	}

	got := decodeJSONDoc(t, "profile show output", out)

	def := schema.DefaultProfile()
	defBytes, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal default profile: %v", err)
	}
	want := decodeJSONDoc(t, "default profile", string(defBytes))

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("printed profile does not equal the built-in default profile as a JSON value\ngot:  %s\nwant: %s", out, defBytes)
	}

	if v, ok := got["profile_version"]; !ok || v != float64(2) {
		t.Fatalf("want profile_version 2, got %v (present: %v)", v, ok)
	}

	for _, nt := range contentBearingNodeTypes(t, got) {
		if _, ok := nt["content_prefix"]; !ok {
			t.Errorf("content-bearing type %v: missing content_prefix", nt["name"])
		}
		if _, ok := nt["leaf_sections"]; !ok {
			t.Errorf("content-bearing type %v: missing leaf_sections", nt["name"])
		}
	}

	// Writing that document to spec/profile.json and resolving again prints
	// an equal document.
	if err := os.WriteFile(filepath.Join(specDir, "profile.json"), []byte(out), 0644); err != nil {
		t.Fatalf("write profile.json: %v", err)
	}
	out2, err := runProfileSpex(t, "profile", "show", "--spec-dir", specDir)
	if err != nil {
		t.Fatalf("profile show over the written profile.json: unexpected error: %v", err)
	}
	got2 := decodeJSONDoc(t, "second profile show output", out2)
	if !reflect.DeepEqual(got, got2) {
		t.Fatalf("profile show does not round-trip through resolution\nfirst:  %s\nsecond: %s", out, out2)
	}

	// spex validate over the fixture with that file in place is green.
	if _, err := runProfileSpex(t, "validate", "--spec-dir", specDir); err != nil {
		t.Fatalf("validate over the fixture with the written profile.json should be green: %v", err)
	}
}

// buildProfileDoc decodes the default profile to a mutable JSON document
// and applies edit to it, for constructing the version-1 and version-3
// fixtures L6 needs without hand-authoring the default vocabulary twice.
func buildProfileDoc(t *testing.T, edit func(doc map[string]any)) string {
	t.Helper()
	defBytes, err := json.Marshal(schema.DefaultProfile())
	if err != nil {
		t.Fatalf("marshal default profile: %v", err)
	}
	doc := decodeJSONDoc(t, "default profile", string(defBytes))
	edit(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal edited profile doc: %v", err)
	}
	return string(out)
}

// L6: A version 1 profile still resolves, a version 3 one is refused.
func TestREQ_7f193910f7ef_L6_VersionOneResolvesVersionThreeRefused(t *testing.T) {
	t.Run("version 1 resolves with default conventions filled in", func(t *testing.T) {
		specDir := setupTestSpec(t)
		v1 := buildProfileDoc(t, func(doc map[string]any) {
			doc["profile_version"] = 1
			for _, nt := range doc["node_types"].([]any) {
				m := nt.(map[string]any)
				delete(m, "content_prefix")
				delete(m, "leaf_sections")
			}
		})
		writeTestFile(t, specDir, "profile.json", v1)

		out, err := runProfileSpex(t, "profile", "show", "--spec-dir", specDir)
		if err != nil {
			t.Fatalf("profile show over a version 1 profile: unexpected error: %v", err)
		}

		got := decodeJSONDoc(t, "profile show output", out)
		wantPrefix := map[string]string{
			"component":    "arch_",
			"data_flow":    "flow_",
			"test_section": "test_",
		}
		wantSections := map[string][]string{
			"component":    {"Responsibilities", "Interface"},
			"data_flow":    {"Data Shapes"},
			"test_section": {"Setup", "Scenarios", "Edge Cases"},
		}
		for _, nt := range contentBearingNodeTypes(t, got) {
			name, _ := nt["name"].(string)
			if wantPrefix[name] == "" {
				continue
			}
			if prefix, _ := nt["content_prefix"].(string); prefix != wantPrefix[name] {
				t.Errorf("type %q: content_prefix = %q, want %q", name, prefix, wantPrefix[name])
			}
			sections, _ := nt["leaf_sections"].([]any)
			var got []string
			for _, s := range sections {
				got = append(got, s.(string))
			}
			if !reflect.DeepEqual(got, wantSections[name]) {
				t.Errorf("type %q: leaf_sections = %v, want %v", name, got, wantSections[name])
			}
		}
	})

	t.Run("version 3 is refused with the file, the version and the supported range", func(t *testing.T) {
		specDir := setupTestSpec(t)
		v3 := buildProfileDoc(t, func(doc map[string]any) {
			doc["profile_version"] = 3
		})
		writeTestFile(t, specDir, "profile.json", v3)

		out, err := runProfileSpex(t, "profile", "show", "--spec-dir", specDir)
		if err == nil {
			t.Fatalf("want a non-zero exit for a version 3 profile, got output: %s", out)
		}
		if out != "" {
			t.Fatalf("want no stdout on refusal, got: %s", out)
		}

		profilePath := filepath.Join(specDir, "profile.json")
		msg := err.Error()
		for _, want := range []string{profilePath, "3", "1-2"} {
			if !strings.Contains(msg, want) {
				t.Errorf("profile show error %q does not name %q", msg, want)
			}
		}

		// spex validate over the same fixture fails with the same message —
		// both surfaces pass the schema module's own error through
		// unrewritten, differing only in their own command prefix.
		_, verr := runProfileSpex(t, "validate", "--spec-dir", specDir)
		if verr == nil {
			t.Fatal("want validate to fail over the same fixture")
		}
		profileCore := strings.TrimPrefix(msg, "profile show: ")
		validateCore := strings.TrimPrefix(verr.Error(), "validate: ")
		if profileCore != validateCore {
			t.Fatalf("profile show and validate disagree on the message:\nprofile show: %s\nvalidate:     %s", profileCore, validateCore)
		}
	})
}

// assertLeafInOrder fails the test unless each of substrs appears in s, in
// the given order (each search starts after the previous match) — the
// shape L1 and L4 need for "carries in order the ## headings", mirroring
// author/leaf_scaffolder_test.go's assertInOrder for the CLI-level runs
// this file drives.
func assertLeafInOrder(t *testing.T, s string, substrs ...string) {
	t.Helper()
	pos := 0
	for _, sub := range substrs {
		idx := strings.Index(s[pos:], sub)
		if idx < 0 {
			t.Fatalf("want %q after position %d in:\n%s", sub, pos, s)
		}
		pos += idx + len(sub)
	}
}

// L1: The skeleton carries the profile's headings and one placeholder per
// owed link. Comp2's leaf is deleted from the node-editing fixture and
// rewritten by `spex leaf scaffold`; `spex validate` must stay green and
// `scripts/link-check.sh` must pass over the diff the write produced.
func TestREQ_1631cb19fac3_L1_SkeletonCarriesHeadingsAndOwedPlaceholder(t *testing.T) {
	dir := t.TempDir()
	f := buildAuthorCmdFixture(t, dir)

	// Snapshot the pristine fixture before Comp2's leaf is deleted, so
	// `spex diff` has a baseline to compare the scaffolded write against.
	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatalf("build snapshot tree: %v", err)
	}
	seedProjectState(t, dir, tree, time.Now())

	comp2Path := filepath.Join(dir, "alpha", "arch_comp2.md")
	if err := os.Remove(comp2Path); err != nil {
		t.Fatalf("remove arch_comp2.md: %v", err)
	}

	out, err := runLeafSpex(t, "leaf", "scaffold", f.comp2ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("leaf scaffold: unexpected error: %v\n%s", err, out)
	}

	data, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "# Comp2\n") {
		t.Fatalf("arch_comp2.md does not open with '# Comp2': %s", content)
	}
	assertLeafInOrder(t, content, "## Responsibilities", "## Interface")
	wantLink := "[[" + f.comp1ID + "|Comp1]]"
	if strings.Count(content, wantLink) != 1 {
		t.Fatalf("arch_comp2.md wants exactly one placeholder link %s, got:\n%s", wantLink, content)
	}

	if _, err := runLeafSpex(t, "validate", "--spec-dir", dir); err != nil {
		t.Fatalf("validate over the scaffolded fixture should be green: %v", err)
	}

	diffOut, err := runLeafSpex(t, "diff", "--json", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("diff over the scaffolded fixture: unexpected error: %v\n%s", err, diffOut)
	}
	diffPath := filepath.Join(t.TempDir(), "diff.json")
	if err := os.WriteFile(diffPath, []byte(diffOut), 0644); err != nil {
		t.Fatalf("write diff.json: %v", err)
	}

	lcOut, err := runLinkCheck(t, dir, diffPath)
	if err != nil {
		t.Fatalf("scripts/link-check.sh: want exit 0, got %v\n%s", err, lcOut)
	}
}

// L2: A non-empty leaf is never overwritten; a leaf present but zero bytes
// long is written.
func TestREQ_1631cb19fac3_L2_NonEmptyLeafNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	f := buildAuthorCmdFixture(t, dir)
	comp2Path := filepath.Join(dir, "alpha", "arch_comp2.md")

	before, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}

	out, err := runLeafSpex(t, "leaf", "scaffold", f.comp2ID, "--spec-dir", dir)
	if err == nil {
		t.Fatalf("leaf scaffold over a non-empty leaf: want a non-zero exit, got output: %s", out)
	}
	if exitCodeOf(err) != author.ExitRefusal {
		t.Errorf("want exit code %d, got %d", author.ExitRefusal, exitCodeOf(err))
	}

	after, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("arch_comp2.md changed:\nbefore: %s\nafter:  %s", before, after)
	}

	var refusals []author.RefusalEntry
	if jerr := json.Unmarshal([]byte(out), &refusals); jerr != nil {
		t.Fatalf("stdout is not a refusal document: %v\n%s", jerr, out)
	}
	if len(refusals) != 1 {
		t.Fatalf("want exactly one refusal, got %+v", refusals)
	}
	if !strings.Contains(refusals[0].Fix, "arch_comp2.md") {
		t.Errorf("refusal fix does not name the file: %+v", refusals[0])
	}
	if !strings.Contains(refusals[0].Fix, "empty") || !strings.Contains(refusals[0].Fix, "move") {
		t.Errorf("refusal fix does not say to empty or move the file: %q", refusals[0].Fix)
	}

	// A leaf present but zero bytes long is written.
	if err := os.WriteFile(comp2Path, nil, 0644); err != nil {
		t.Fatalf("truncate arch_comp2.md: %v", err)
	}
	out2, err := runLeafSpex(t, "leaf", "scaffold", f.comp2ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("leaf scaffold over a zero-byte leaf: unexpected error: %v\n%s", err, out2)
	}
	data, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	if len(data) == 0 || !strings.HasPrefix(string(data), "# Comp2\n") {
		t.Fatalf("arch_comp2.md should carry the written skeleton, got: %q", data)
	}
}

// L3: Scaffolding is idempotent — a second run over an already-scaffolded
// leaf changes nothing and reports it in the write report.
func TestREQ_1631cb19fac3_L3_ScaffoldingIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	f := buildAuthorCmdFixture(t, dir)
	comp2Path := filepath.Join(dir, "alpha", "arch_comp2.md")
	if err := os.Remove(comp2Path); err != nil {
		t.Fatalf("remove arch_comp2.md: %v", err)
	}

	if out, err := runLeafSpex(t, "leaf", "scaffold", f.comp2ID, "--spec-dir", dir); err != nil {
		t.Fatalf("first scaffold: unexpected error: %v\n%s", err, out)
	}
	written, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}

	out, err := runLeafSpex(t, "leaf", "scaffold", f.comp2ID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("second scaffold: unexpected error: %v\n%s", err, out)
	}
	var report author.WriteReport
	if jerr := json.Unmarshal([]byte(out), &report); jerr != nil {
		t.Fatalf("stdout is not a write report: %v\n%s", jerr, out)
	}
	if len(report.Written) != 0 {
		t.Errorf("second scaffold: want an empty written list (already scaffolded), got %+v", report.Written)
	}

	again, err := os.ReadFile(comp2Path)
	if err != nil {
		t.Fatalf("read arch_comp2.md: %v", err)
	}
	if string(written) != string(again) {
		t.Fatalf("second scaffold changed the file:\nfirst:  %s\nsecond: %s", written, again)
	}
}

// customProfileFixture is tmp/custom/: the node-editing fixture's layout,
// but under a profile.json whose types share no name with the default —
// a module-scoped content-bearing endpoint (content prefix ep_, leaf
// sections Contract and Errors) and a module-scoped resource with no leaf
// sections, which owes a placeholder link to the endpoint it supports.
type customProfileFixture struct {
	dir                    string
	endpointID, resourceID string
}

func buildCustomLeafProfileFixture(t *testing.T) customProfileFixture {
	t.Helper()
	dir := t.TempDir()

	profileDoc := `{
		"profile_version": 2,
		"node_types": [
			{"name": "endpoint", "plural_key": "endpoints", "scope": "module", "requires_content": true, "content_prefix": "ep_", "leaf_sections": ["Contract", "Errors"]},
			{"name": "resource", "plural_key": "resources", "scope": "module", "requires_content": true, "leaf_sections": [],
				"fields": [{"name": "supports", "kind": "reference", "targets": ["endpoint"], "cardinality": "many"}]}
		]
	}`
	writeTestFile(t, dir, "profile.json", profileDoc)

	proj := `{"name": "custom-project", "modules": [{"id": "000000000001", "name": "alpha", "path": "alpha"}]}`
	writeTestFile(t, dir, "project.json", proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}

	endpointID := schema.IdentityHash("alpha", "endpoint", "Get widget")
	resourceID := schema.IdentityHash("alpha", "resource", "Widget resource")
	mod := `{
		"name": "alpha",
		"endpoints": [{"id": "` + endpointID + `", "name": "Get widget", "content": "ep_get_widget.md"}],
		"resources": [{"id": "` + resourceID + `", "name": "Widget resource", "content": "res_widget_resource.md", "supports": ["` + endpointID + `"]}]
	}`
	writeTestFile(t, alphaDir, "module.json", mod)

	return customProfileFixture{dir: dir, endpointID: endpointID, resourceID: resourceID}
}

// L4: The headings come from the profile, not the command — a type
// declaring leaf_sections gets exactly those headings, in the order
// `spex profile show` prints them, and a type declaring none gets no "##"
// heading at all, just its title and its owed placeholder lines.
func TestREQ_1631cb19fac3_L4_HeadingsComeFromProfileNotCommand(t *testing.T) {
	f := buildCustomLeafProfileFixture(t)

	profileOut, err := runLeafSpex(t, "profile", "show", "--spec-dir", f.dir)
	if err != nil {
		t.Fatalf("profile show: unexpected error: %v", err)
	}
	profileDoc := decodeJSONDoc(t, "profile show output", profileOut)
	var endpointSections []string
	if nts, ok := profileDoc["node_types"].([]any); ok {
		for _, nt := range nts {
			m, ok := nt.(map[string]any)
			if !ok || m["name"] != "endpoint" {
				continue
			}
			if secs, ok := m["leaf_sections"].([]any); ok {
				for _, s := range secs {
					if str, ok := s.(string); ok {
						endpointSections = append(endpointSections, str)
					}
				}
			}
		}
	}
	if want := []string{"Contract", "Errors"}; !reflect.DeepEqual(endpointSections, want) {
		t.Fatalf("profile show's endpoint leaf_sections = %v, want %v", endpointSections, want)
	}

	out, err := runLeafSpex(t, "leaf", "scaffold", f.endpointID, "--spec-dir", f.dir)
	if err != nil {
		t.Fatalf("leaf scaffold(endpoint): unexpected error: %v\n%s", err, out)
	}
	epPath := filepath.Join(f.dir, "alpha", "ep_get_widget.md")
	epData, err := os.ReadFile(epPath)
	if err != nil {
		t.Fatalf("read %s: %v", epPath, err)
	}
	epContent := string(epData)
	if !strings.HasPrefix(epContent, "# Get widget\n") {
		t.Fatalf("endpoint leaf does not open with '# Get widget': %s", epContent)
	}
	assertLeafInOrder(t, epContent, "## Contract", "## Errors")
	if strings.Count(epContent, "##") != 2 {
		t.Fatalf("endpoint leaf wants exactly the two declared headings and nothing else: %s", epContent)
	}

	out, err = runLeafSpex(t, "leaf", "scaffold", f.resourceID, "--spec-dir", f.dir)
	if err != nil {
		t.Fatalf("leaf scaffold(resource): unexpected error: %v\n%s", err, out)
	}
	resPath := filepath.Join(f.dir, "alpha", "res_widget_resource.md")
	resData, err := os.ReadFile(resPath)
	if err != nil {
		t.Fatalf("read %s: %v", resPath, err)
	}
	resContent := string(resData)
	if !strings.HasPrefix(resContent, "# Widget resource\n") {
		t.Fatalf("resource leaf does not open with '# Widget resource': %s", resContent)
	}
	if strings.Contains(resContent, "##") {
		t.Fatalf("resource leaf declares no leaf_sections and wants no '##' heading: %s", resContent)
	}
	wantLink := "[[" + f.endpointID + "|Get widget]]"
	if !strings.Contains(resContent, wantLink) {
		t.Fatalf("resource leaf wants its owed placeholder link %s: %s", wantLink, resContent)
	}
}

// L7: A node the profile marks as not content-bearing has no leaf to
// scaffold — the default profile's api.
func TestREQ_1631cb19fac3_L7_NotContentBearingRefused(t *testing.T) {
	dir := t.TempDir()
	f := buildAuthorCmdFixture(t, dir)

	entriesBefore, err := os.ReadDir(filepath.Join(dir, "alpha"))
	if err != nil {
		t.Fatalf("read alpha dir: %v", err)
	}

	out, err := runLeafSpex(t, "leaf", "scaffold", f.apiID, "--spec-dir", dir)
	if err == nil {
		t.Fatalf("leaf scaffold(api): want a non-zero exit, got output: %s", out)
	}
	var refusals []author.RefusalEntry
	if jerr := json.Unmarshal([]byte(out), &refusals); jerr != nil {
		t.Fatalf("stdout is not a refusal document: %v\n%s", jerr, out)
	}
	if len(refusals) != 1 {
		t.Fatalf("want exactly one refusal, got %+v", refusals)
	}
	if !strings.Contains(refusals[0].Message, "api") {
		t.Errorf("refusal does not name the type api: %+v", refusals[0])
	}
	if !strings.Contains(refusals[0].Fix, "component") {
		t.Errorf("refusal fix does not list the content-bearing types: %+v", refusals[0])
	}

	entriesAfter, err := os.ReadDir(filepath.Join(dir, "alpha"))
	if err != nil {
		t.Fatalf("read alpha dir: %v", err)
	}
	if len(entriesBefore) != len(entriesAfter) {
		t.Fatalf("leaf scaffold(api) wrote a file: before %v, after %v", entriesBefore, entriesAfter)
	}
}

// L8: Adding a node scaffolds through the same path — `spex node add`
// writes the new node's leaf skeleton itself, and a following
// `spex leaf scaffold` over the same id is a no-op.
func TestREQ_1631cb19fac3_L8_NodeAddScaffoldsThroughSamePath(t *testing.T) {
	dir := t.TempDir()
	buildAuthorCmdFixture(t, dir)

	addOut, err := runLeafSpex(t, "node", "add", "Demo flow", "--type", "data_flow", "--module", "alpha", "--spec-dir", dir)
	if err != nil {
		t.Fatalf("node add: unexpected error: %v\n%s", err, addOut)
	}

	flowID := schema.IdentityHash("alpha", "data_flow", "Demo flow")
	flowPath := filepath.Join(dir, "alpha", "flow_demo_flow.md")
	data, err := os.ReadFile(flowPath)
	if err != nil {
		t.Fatalf("read %s: %v", flowPath, err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "# Demo flow\n") {
		t.Fatalf("flow_demo_flow.md does not open with '# Demo flow': %s", content)
	}
	if !strings.Contains(content, "## Data Shapes") {
		t.Fatalf("flow_demo_flow.md does not carry the data_flow heading: %s", content)
	}

	scaffoldOut, err := runLeafSpex(t, "leaf", "scaffold", flowID, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("leaf scaffold over the just-added node: unexpected error: %v\n%s", err, scaffoldOut)
	}
	var report author.WriteReport
	if jerr := json.Unmarshal([]byte(scaffoldOut), &report); jerr != nil {
		t.Fatalf("stdout is not a write report: %v\n%s", jerr, scaffoldOut)
	}
	if len(report.Written) != 0 {
		t.Errorf("leaf scaffold over the already-scaffolded node: want an empty written list, got %+v", report.Written)
	}

	again, err := os.ReadFile(flowPath)
	if err != nil {
		t.Fatalf("read %s: %v", flowPath, err)
	}
	if string(data) != string(again) {
		t.Fatalf("leaf scaffold changed the just-added flow leaf:\nbefore: %s\nafter:  %s", data, again)
	}
}
