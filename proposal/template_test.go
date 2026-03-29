package proposal

import (
	"bytes"
	"strings"
	"testing"
)

func TestFR3_Template_Project(t *testing.T) {
	var buf bytes.Buffer
	err := Template("project", &buf)
	if err != nil {
		t.Fatalf("Template(project): %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "# Project Proposal: <Project Name>") {
		t.Error("project template should start with '# Project Proposal: <Project Name>'")
	}
	for _, heading := range []string{"## Vision", "## Modules", "## Key requirements", "## Design decisions"} {
		if !strings.Contains(out, heading) {
			t.Errorf("project template missing heading %q", heading)
		}
	}
}

func TestFR3_Template_Change(t *testing.T) {
	var buf bytes.Buffer
	err := Template("change", &buf)
	if err != nil {
		t.Fatalf("Template(change): %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "# Change Proposal: <Title>") {
		t.Error("change template should start with '# Change Proposal: <Title>'")
	}
	for _, heading := range []string{"## Context", "## Proposed change", "## Impact expectation"} {
		if !strings.Contains(out, heading) {
			t.Errorf("change template missing heading %q", heading)
		}
	}
}

func TestFR3_Template_UnknownType(t *testing.T) {
	var buf bytes.Buffer
	err := Template("unknown", &buf)
	if err == nil {
		t.Fatal("want error for unknown type, got nil")
	}
	if !strings.Contains(err.Error(), `unknown template type: "unknown"`) {
		t.Errorf("want error about unknown type, got: %v", err)
	}
}
