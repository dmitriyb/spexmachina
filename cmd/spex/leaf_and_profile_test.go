package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/schema"
)

// This file is cmd/spex's half of spec/author/test_leaf_and_profile.md — the
// "Leaf and profile tests" test section, which describes both LeafScaffolder
// and ProfileInspector. Only the ProfileInspector scenarios that exercise
// `spex profile show` alone (L5, L6) are implemented here; L1-L4, L7 and L8
// all drive `spex leaf scaffold`, which belongs to LeafScaffolder's own bead.

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
