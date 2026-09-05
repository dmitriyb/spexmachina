package proposal

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// placeholderPattern matches an angle-bracket placeholder marker like
// "<Describe the project vision and motivation>".
var placeholderPattern = regexp.MustCompile(`<[^>\n]+>`)

// sectionBody returns the text of the H2 section named heading, up to (but
// not including) the next H1/H2 heading or the end of content. Sub-headings
// (H3 and deeper) inside the section stay part of the body.
func sectionBody(t *testing.T, content, heading string) string {
	t.Helper()
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if line == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatalf("heading %q not found in template", heading)
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "# ") || strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

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
			continue
		}
		if body := sectionBody(t, out, heading); !placeholderPattern.MatchString(body) {
			t.Errorf("project template section %q missing placeholder text (e.g. <...>), got: %q", heading, body)
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
			continue
		}
		if body := sectionBody(t, out, heading); !placeholderPattern.MatchString(body) {
			t.Errorf("change template section %q missing placeholder text (e.g. <...>), got: %q", heading, body)
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

// TestFR3_Template_EmptyType covers E6: an empty template type is refused the
// same way an unrecognized one is, and nothing is written to the buffer.
func TestFR3_Template_EmptyType(t *testing.T) {
	var buf bytes.Buffer
	err := Template("", &buf)
	if err == nil {
		t.Fatal("want error for empty type, got nil")
	}
	if !strings.Contains(err.Error(), `unknown template type: ""`) {
		t.Errorf("want error about empty type, got: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("want nothing written to buf, got %q", buf.String())
	}
}

// TestFR3_Template_Deterministic covers E4: templates are embedded constants,
// so two consecutive calls for the same type must emit byte-identical output.
func TestFR3_Template_Deterministic(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	if err := Template("project", &buf1); err != nil {
		t.Fatalf("Template(project) #1: %v", err)
	}
	if err := Template("project", &buf2); err != nil {
		t.Fatalf("Template(project) #2: %v", err)
	}
	if buf1.String() != buf2.String() {
		t.Errorf("want identical output across calls, got:\n%q\nvs\n%q", buf1.String(), buf2.String())
	}
}

// TestFR3_Template_ProjectSectionsMatchRegistrar covers S11: the project
// template's own output must round-trip through detectType and
// validateSections — the same checks Register runs — confirming the
// template and the registrar agree on what a project proposal requires.
func TestFR3_Template_ProjectSectionsMatchRegistrar(t *testing.T) {
	var buf bytes.Buffer
	if err := Template("project", &buf); err != nil {
		t.Fatalf("Template(project): %v", err)
	}
	content := buf.String()

	ptype, err := detectType(content)
	if err != nil {
		t.Fatalf("detectType: %v", err)
	}
	if ptype != "project" {
		t.Errorf("want detected type %q, got %q", "project", ptype)
	}
	if err := validateSections(content, "project"); err != nil {
		t.Errorf("validateSections: %v", err)
	}
}

// TestFR3_Template_ChangeSectionsMatchRegistrar covers S12: same round-trip
// as S11, for the change template.
func TestFR3_Template_ChangeSectionsMatchRegistrar(t *testing.T) {
	var buf bytes.Buffer
	if err := Template("change", &buf); err != nil {
		t.Fatalf("Template(change): %v", err)
	}
	content := buf.String()

	ptype, err := detectType(content)
	if err != nil {
		t.Fatalf("detectType: %v", err)
	}
	if ptype != "change" {
		t.Errorf("want detected type %q, got %q", "change", ptype)
	}
	if err := validateSections(content, "change"); err != nil {
		t.Errorf("validateSections: %v", err)
	}
}
