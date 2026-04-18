package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/schema"
)

func runHashID(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := cli.NewRootCmd()
	root.AddCommand(newHashIDCmd())

	var stdout, stderr strings.Builder
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"hash-id"}, args...))

	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestFR58_S1_ComponentHashMatchesSchema(t *testing.T) {
	stdout, _, err := runHashID(t, "--module", "impact", "--type", "component", "--name", "NodeMatcher")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("impact", "component", "NodeMatcher")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S2_ModuleHashNoModuleFlag(t *testing.T) {
	stdout, _, err := runHashID(t, "--type", "module", "--name", "schema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("module", "schema")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S3_ProjectRequirementNoModuleFlag(t *testing.T) {
	stdout, _, err := runHashID(t, "--type", "requirement", "--name", "Validate spec structure")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("project", "requirement", "Validate spec structure")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S4_ModuleRequirementWithModuleFlag(t *testing.T) {
	stdout, _, err := runHashID(t, "--module", "validator", "--type", "requirement", "--name", "ID uniqueness")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("validator", "requirement", "ID uniqueness")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S5_MilestoneHash(t *testing.T) {
	stdout, _, err := runHashID(t, "--type", "milestone", "--name", "Bootstrap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("milestone", "Bootstrap")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S6_ScenarioHash(t *testing.T) {
	stdout, _, err := runHashID(t, "--type", "scenario", "--name", "Cross-module mapping integration")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("test_plan", "scenario", "Cross-module mapping integration")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q, got %q", want, got)
	}
}

func TestFR58_S7_AllModuleScopedTypes(t *testing.T) {
	types := []string{"component", "impl_section", "data_flow", "test_section"}
	for _, typ := range types {
		t.Run(typ, func(t *testing.T) {
			stdout, _, err := runHashID(t, "--module", "alpha", "--type", typ, "--name", "Foo")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := schema.IdentityHash("alpha", typ, "Foo")
			if got := strings.TrimSpace(stdout); got != want {
				t.Errorf("type %q: want %q, got %q", typ, want, got)
			}
		})
	}
}

func TestFR58_S8_OutputMatchesHexPattern(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-f0-9]{12}$`)
	cases := [][]string{
		{"--module", "impact", "--type", "component", "--name", "A"},
		{"--module", "impact", "--type", "component", "--name", "B"},
		{"--module", "impact", "--type", "impl_section", "--name", "Build"},
		{"--module", "impact", "--type", "data_flow", "--name", "Flow"},
		{"--module", "impact", "--type", "test_section", "--name", "T"},
		{"--type", "module", "--name", "x"},
		{"--type", "milestone", "--name", "M"},
		{"--type", "scenario", "--name", "S"},
		{"--type", "requirement", "--name", "req a"},
		{"--module", "m", "--type", "requirement", "--name", "req b"},
	}
	for i, args := range cases {
		stdout, _, err := runHashID(t, args...)
		if err != nil {
			t.Fatalf("case %d: unexpected error: %v", i, err)
		}
		got := strings.TrimSpace(stdout)
		if !pattern.MatchString(got) {
			t.Errorf("case %d: output %q does not match %s", i, got, pattern)
		}
	}
}

func TestFR58_S9_Deterministic(t *testing.T) {
	args := []string{"--module", "impact", "--type", "component", "--name", "NodeMatcher"}
	out1, _, err := runHashID(t, args...)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	out2, _, err := runHashID(t, args...)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if out1 != out2 {
		t.Errorf("non-deterministic: %q != %q", out1, out2)
	}
}

func TestFR58_E1_MissingTypeFlag(t *testing.T) {
	_, _, err := runHashID(t, "--name", "Foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("expected error mentioning --type, got %q", err)
	}
}

func TestFR58_E2_MissingNameFlag(t *testing.T) {
	_, _, err := runHashID(t, "--type", "component")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error mentioning --name, got %q", err)
	}
}

func TestFR58_E3_ModuleScopedWithoutModule(t *testing.T) {
	_, _, err := runHashID(t, "--type", "component", "--name", "Foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := `--module is required for type "component"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("expected error containing %q, got %q", want, err)
	}
}

func TestFR58_E4_UnknownType(t *testing.T) {
	_, _, err := runHashID(t, "--type", "bogus", "--name", "Foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"requirement", "component", "module", "milestone", "scenario"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error listing valid types, missing %q in %q", want, msg)
		}
	}
}

func TestFR58_E5_ModuleIgnoredForProjectLevelTypes(t *testing.T) {
	stdout, _, err := runHashID(t, "--module", "ignored", "--type", "module", "--name", "schema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := schema.IdentityHash("module", "schema")
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("want %q (module flag ignored), got %q", want, got)
	}
}
