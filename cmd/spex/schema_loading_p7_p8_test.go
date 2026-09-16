package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// This file covers the two test_schema_loading.md scenarios (8719672c7580)
// that name commands in their own `When` clause — P7 ("spex validate" then
// "spex profile show") and P8 ("spex validate", "spex diff", "spex profile
// show" and "spex node add") — at the command level those scenarios are
// written at. schema/schema_loader_test.go's TestFR9_P7_* and TestFR9_P8_*
// pin the same conventions at ResolveProfile's level; neither runs a
// command, so they do not stand in for the coverage this file adds.

// runSchemaLoadingSpex runs the four command trees P7 and P8 name, mirroring
// runAuthorSpex/runLeafSpex's pattern (author_commands_test.go,
// leaf_and_profile_test.go) of assembling the tree in place rather than
// spawning a process.
func runSchemaLoadingSpex(t *testing.T, args ...string) (stdout string, execErr error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newNodeCmd(), newDiffCmd(), newProfileCmd(), newValidateCmd())

	rootCmd.SetArgs(args)
	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})
	return stdout, execErr
}

// TestFR9_P7_CommandLevelConventions covers test_schema_loading.md's P7 at
// the command level the scenario names: a version 2 document's own
// content_prefix/leaf_sections on a custom content-bearing type resolve as
// declared, while a version 1 document gets the built-in conventions filled
// in — both reachable through `spex validate` (exit 0) and `spex profile
// show` (the printed document), not only through ResolveProfile directly.
func TestFR9_P7_CommandLevelConventions(t *testing.T) {
	t.Run("version 2 profile.json: validate exits 0, profile show prints its own conventions in order plus the built-in defaults", func(t *testing.T) {
		specDir := setupTestSpec(t)
		// A file-backed profile.json replaces node_types wholesale (P4's
		// fixture shape), so the default ontology has to be restated
		// alongside the new endpoint type for alpha/module.json's existing
		// components/test_sections arrays to stay declared.
		profile := schema.DefaultProfile()
		contentPrefix := "ep_"
		profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
			Name: "endpoint", PluralKey: "endpoints", Scope: "module", RequiresContent: true,
			ContentPrefix: &contentPrefix, LeafSections: []string{"Contract", "Errors"},
		})
		data, err := json.Marshal(profile)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		writeTestFile(t, specDir, "profile.json", string(data))

		if _, err := runSchemaLoadingSpex(t, "validate", "--spec-dir", specDir); err != nil {
			t.Fatalf("validate: want exit 0 over a valid version 2 profile.json, got: %v", err)
		}

		out, err := runSchemaLoadingSpex(t, "profile", "show", "--spec-dir", specDir)
		if err != nil {
			t.Fatalf("profile show: unexpected error: %v", err)
		}
		doc := decodeJSONDoc(t, "profile show output", out)

		endpoint := findNodeType(t, doc, "endpoint")
		if prefix, _ := endpoint["content_prefix"].(string); prefix != "ep_" {
			t.Errorf("endpoint content_prefix = %v, want \"ep_\"", endpoint["content_prefix"])
		}
		assertStringSliceField(t, endpoint, "leaf_sections", []string{"Contract", "Errors"})

		// "in that order": content_prefix precedes leaf_sections in the raw
		// printed text — the JSON key order a Go struct's field order fixes,
		// not just their decoded values.
		endpointText := extractObjectText(t, out, `"name":"endpoint"`)
		prefixIdx := strings.Index(endpointText, `"content_prefix"`)
		sectionsIdx := strings.Index(endpointText, `"leaf_sections"`)
		if prefixIdx < 0 || sectionsIdx < 0 || prefixIdx > sectionsIdx {
			t.Fatalf("want content_prefix before leaf_sections in printed order, got: %s", endpointText)
		}

		assertBuiltinConventions(t, doc)
	})

	t.Run("version 1 profile.json: validate exits 0, profile show fills in the defaults and round-trips", func(t *testing.T) {
		specDir := setupTestSpec(t)
		profile := schema.DefaultProfile()
		profile.ProfileVersion = nil
		for i := range profile.NodeTypes {
			profile.NodeTypes[i].ContentPrefix = nil
			profile.NodeTypes[i].LeafSections = nil
		}
		data, err := json.Marshal(profile)
		if err != nil {
			t.Fatalf("marshal profile: %v", err)
		}
		writeTestFile(t, specDir, "profile.json", string(data))

		if _, err := runSchemaLoadingSpex(t, "validate", "--spec-dir", specDir); err != nil {
			t.Fatalf("validate: want exit 0 over a valid version 1 profile.json, got: %v", err)
		}

		out, err := runSchemaLoadingSpex(t, "profile", "show", "--spec-dir", specDir)
		if err != nil {
			t.Fatalf("profile show: unexpected error: %v", err)
		}
		doc := decodeJSONDoc(t, "profile show output", out)
		assertBuiltinConventions(t, doc)

		// P7's round-trip clause: the printed document, written back as
		// spec/profile.json, resolves to an equal document on the next run.
		writeTestFile(t, specDir, "profile.json", out)
		out2, err := runSchemaLoadingSpex(t, "profile", "show", "--spec-dir", specDir)
		if err != nil {
			t.Fatalf("profile show over the written-back profile.json: unexpected error: %v", err)
		}
		doc2 := decodeJSONDoc(t, "second profile show output", out2)
		if !reflect.DeepEqual(doc, doc2) {
			t.Fatalf("profile show does not round-trip through resolution\nfirst:  %s\nsecond: %s", out, out2)
		}
	})
}

// findNodeType returns the node_types entry named name from a decoded
// profile-show document, failing the test if absent.
func findNodeType(t *testing.T, doc map[string]any, name string) map[string]any {
	t.Helper()
	raw, ok := doc["node_types"].([]any)
	if !ok {
		t.Fatalf("profile document carries no node_types array: %v", doc)
	}
	for _, nt := range raw {
		m, ok := nt.(map[string]any)
		if !ok {
			continue
		}
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("profile document carries no node type named %q: %v", name, doc)
	return nil
}

// assertStringSliceField checks that doc[field] decodes to exactly want, in
// order.
func assertStringSliceField(t *testing.T, doc map[string]any, field string, want []string) {
	t.Helper()
	raw, _ := doc[field].([]any)
	var got []string
	for _, v := range raw {
		s, _ := v.(string)
		got = append(got, s)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", field, got, want)
	}
}

// extractObjectText returns the smallest text window in s that starts at
// marker and runs to the next top-level "},{" boundary (or the end of the
// array) — enough to isolate one node_types entry's own key order without a
// full JSON tokenizer, since the printed document is one compact line.
func extractObjectText(t *testing.T, s, marker string) string {
	t.Helper()
	start := strings.Index(s, marker)
	if start < 0 {
		t.Fatalf("marker %q not found in: %s", marker, s)
	}
	rest := s[start:]
	end := strings.Index(rest, "},{")
	if end < 0 {
		end = strings.Index(rest, "}]")
	}
	if end < 0 {
		t.Fatalf("could not bound object text starting at %q in: %s", marker, s)
	}
	return rest[:end]
}

// assertBuiltinConventions checks that the three built-in content-bearing
// types carry their conventional content_prefix and leaf_sections, which
// every P7 fixture leaves unrestated.
func assertBuiltinConventions(t *testing.T, doc map[string]any) {
	t.Helper()
	want := map[string]struct {
		prefix   string
		sections []string
	}{
		"component":    {"arch_", []string{"Responsibilities", "Interface"}},
		"data_flow":    {"flow_", []string{"Data Shapes"}},
		"test_section": {"test_", []string{"Setup", "Scenarios", "Edge Cases"}},
	}
	for name, w := range want {
		nt := findNodeType(t, doc, name)
		if prefix, _ := nt["content_prefix"].(string); prefix != w.prefix {
			t.Errorf("%s content_prefix = %v, want %q", name, nt["content_prefix"], w.prefix)
		}
		assertStringSliceField(t, nt, "leaf_sections", w.sections)
	}
}

// TestFR9_P8_CommandLevelUniformFailure covers test_schema_loading.md's P8
// at the command level the scenario names: an out-of-range profile_version,
// or a version 2 document with a defective declaration (leaf_sections on a
// content-less built-in type, plus an undeclared top-level key), fails
// every one of `spex validate`, `spex diff`, `spex profile show` and `spex
// node add` the same way — exit 1, one message, no other output.
func TestFR9_P8_CommandLevelUniformFailure(t *testing.T) {
	t.Run("profile_version 3 is out of the supported range", func(t *testing.T) {
		specDir, profilePath := setupP8Fixture(t, `{"profile_version": 3, "node_types": []}`)
		assertEveryCommandFailsUniformly(t, specDir, profilePath, "3", "1-2")
	})

	t.Run("leaf_sections on a content-less built-in type, plus an undeclared top-level key", func(t *testing.T) {
		specDir, profilePath := setupP8Fixture(t, `{
			"profile_version": 2,
			"node_types": [
				{"name": "api", "plural_key": "apis", "scope": "module", "leaf_sections": ["Notes"]}
			],
			"not_a_declared_key": true
		}`)
		// Decoding is strict: the undeclared top-level key fails the whole
		// document at decode, before the leaf_sections-on-a-content-less-type
		// check ever runs — so the one message every surface gives names the
		// undeclared key, not the leaf_sections declaration underneath it.
		assertEveryCommandFailsUniformly(t, specDir, profilePath, "not_a_declared_key")
	})
}

// setupP8Fixture builds an otherwise-valid, initialised spec (a snapshot in
// place so `spex diff` reaches its own profile-resolution pre-flight rather
// than refusing earlier as an uninitialised project) with profileDoc written
// as spec/profile.json. Returns the spec directory and the profile.json
// path the expected error messages name.
func setupP8Fixture(t *testing.T, profileDoc string) (specDir, profilePath string) {
	t.Helper()
	specDir = setupTestSpec(t)
	writeTestFile(t, specDir, "profile.json", profileDoc)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Unix(0, 0))
	return specDir, filepath.Join(specDir, "profile.json")
}

// assertEveryCommandFailsUniformly runs validate, diff, profile show and
// node add over specDir and checks each exits non-zero with no stdout and
// an error message naming every string in wantSubstrs.
func assertEveryCommandFailsUniformly(t *testing.T, specDir, profilePath string, wantSubstrs ...string) {
	t.Helper()

	cases := []struct {
		name string
		args []string
	}{
		{"validate", []string{"validate", "--spec-dir", specDir}},
		{"diff", []string{"diff", "--spec-dir", specDir}},
		{"profile show", []string{"profile", "show", "--spec-dir", specDir}},
		{"node add", []string{"node", "add", "Widget", "--type", "component", "--module", "alpha", "--spec-dir", specDir}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := runSchemaLoadingSpex(t, c.args...)
			if err == nil {
				t.Fatalf("%s: want a non-zero exit over a malformed profile.json, got output: %s", c.name, out)
			}
			if exitCodeOf(err) != 0 {
				t.Errorf("%s: want the default exit code (1), got explicit code %d", c.name, exitCodeOf(err))
			}
			if out != "" {
				t.Errorf("%s: want no stdout on refusal, got: %s", c.name, out)
			}
			msg := err.Error()
			if strings.Contains(msg, "\n") {
				t.Errorf("%s: want one message with no conformance error following it, got multi-line: %q", c.name, msg)
			}
			for _, want := range append([]string{profilePath}, wantSubstrs...) {
				if !strings.Contains(msg, want) {
					t.Errorf("%s: error %q does not name %q", c.name, msg, want)
				}
			}
		})
	}
}
