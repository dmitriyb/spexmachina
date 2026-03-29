package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
)

func TestFR2_S1_VersionWithInjectedValues(t *testing.T) {
	// Save originals and restore after test.
	origVersion, origCommit, origDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = origVersion, origCommit, origDate
	})

	version = "v1.2.3"
	commit = "abc1234"
	date = "2026-01-01T00:00:00Z"

	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	for _, want := range []string{"v1.2.3", "abc1234", "2026-01-01T00:00:00Z", runtime.Version()} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
}

func TestFR2_S2_VersionWithDevDefaults(t *testing.T) {
	// Save originals and restore after test.
	origVersion, origCommit, origDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = origVersion, origCommit, origDate
	})

	version = "dev"
	commit = "unknown"
	date = "unknown"

	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	for _, want := range []string{"dev", "unknown"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	if !strings.Contains(output, runtime.Version()) {
		t.Errorf("output missing Go version %q:\n%s", runtime.Version(), output)
	}
}

func TestFR2_S3_VersionHelp(t *testing.T) {
	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"version", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "version") {
		t.Errorf("help output missing 'version':\n%s", output)
	}
}

func TestFR2_S4_VersionExitsCleanly(t *testing.T) {
	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var stdout, stderr strings.Builder
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stderr.Len() > 0 {
		t.Errorf("unexpected stderr output: %q", stderr.String())
	}
}

func TestFR2_S5_VersionNoArgs(t *testing.T) {
	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var stderr strings.Builder
	root.SetErr(&stderr)
	root.SetArgs([]string{"version", "foo"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for extra args, got nil")
	}
}

func TestFR2_OutputFormat(t *testing.T) {
	origVersion, origCommit, origDate := version, commit, date
	t.Cleanup(func() {
		version, commit, date = origVersion, origCommit, origDate
	})

	version = "v0.1.0"
	commit = "abc1234"
	date = "2026-03-10T12:00:00Z"

	root := cli.NewRootCmd()
	root.AddCommand(newVersionCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), output)
	}
	if !strings.HasPrefix(lines[0], "spex v0.1.0") {
		t.Errorf("line 1: want prefix 'spex v0.1.0', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "commit:") {
		t.Errorf("line 2: want prefix 'commit:', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "built:") {
		t.Errorf("line 3: want prefix 'built:', got %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "go:") {
		t.Errorf("line 4: want prefix 'go:', got %q", lines[3])
	}
}
