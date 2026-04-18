package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// REQ-d99ef6b9b776: Validate sections and coupled modules.
// Scenarios from spec/validator/test_coupled_sections.md.

const validSectionSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["versioning"],
  "properties": {
    "versioning": {
      "type": "object",
      "required": ["scheme"],
      "properties": {
        "scheme": { "type": "string" }
      }
    }
  }
}`

// writeFile writes a file at dir/name with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeModule creates a module directory with a module.json containing only the name.
func writeModule(t *testing.T, specDir, modPath, modName string) {
	t.Helper()
	dir := filepath.Join(specDir, modPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeFile(t, dir, "module.json", `{"name": "`+modName+`"}`)
}

// writeProject writes a project.json file at the given spec directory.
func writeProject(t *testing.T, specDir, content string) {
	t.Helper()
	writeFile(t, specDir, "project.json", content)
}

// TestS1_ValidCoupledSectionPasses: zero errors when section, module, and schema
// all align and section content satisfies the schema.
func TestS1_ValidCoupledSectionPasses(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled", "versioning": {"scheme": "semver"}}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// TestS2_NonCoupledSectionSkipped: a section with type other than "coupled"
// only needs envelope fields — no module match required.
func TestS2_NonCoupledSectionSkipped(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": [
    {"id": 1, "name": "notes", "type": "informational"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// TestS3_CoupledNoMatchingModule: coupled section with no module of the same name
// must report an actionable error suggesting the module be added.
func TestS3_CoupledNoMatchingModule(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "coupled_section" {
		t.Fatalf("expected check=coupled_section, got %q", e.Check)
	}
	if !strings.Contains(e.Message, "delivery") {
		t.Fatalf("expected section name in message, got: %s", e.Message)
	}
	if !strings.Contains(strings.ToLower(e.Message), "module") {
		t.Fatalf("expected guidance about adding a module, got: %s", e.Message)
	}
}

// TestS4_MissingSectionSchemaFile: coupled module exists but section.schema.json
// is absent on disk — must be reported with the expected path.
func TestS4_MissingSectionSchemaFile(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if !strings.Contains(e.Message, "section.schema.json") {
		t.Fatalf("expected file name in message, got: %s", e.Message)
	}
	if !strings.Contains(e.Message, "delivery") {
		t.Fatalf("expected module name in message, got: %s", e.Message)
	}
}

// TestS5_ContentFailsSchema: section content violates the module's section schema —
// the JSON Schema validation error must be surfaced with path detail.
func TestS5_ContentFailsSchema(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled", "versioning": {"scheme": 123}}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected at least 1 error, got 0")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "string") || strings.Contains(strings.ToLower(e.Message), "scheme") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error referencing scheme/string violation, got: %v", errs)
	}
}

// TestS6_DuplicateSectionIDs: two sections with the same id must be reported.
func TestS6_DuplicateSectionIDs(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": [
    {"id": 1, "name": "alpha", "type": "informational"},
    {"id": 1, "name": "beta", "type": "informational"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	dupCount := 0
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "duplicate") && strings.Contains(e.Message, "id") {
			dupCount++
		}
	}
	if dupCount == 0 {
		t.Fatalf("expected duplicate ID error, got: %v", errs)
	}
}

// TestS7_MissingEnvelopeName: a section without name must be reported.
func TestS7_MissingEnvelopeName(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": [
    {"id": 1, "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected error for missing name, got none")
	}
	found := false
	for _, e := range errs {
		m := strings.ToLower(e.Message)
		if strings.Contains(m, "name") && (strings.Contains(m, "missing") || strings.Contains(m, "required")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected message about missing name, got: %v", errs)
	}
}

// TestS8_MultipleCoupledIndependent: two coupled sections, one valid one missing
// its module — only the broken one must report an error.
func TestS8_MultipleCoupledIndependent(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled", "versioning": {"scheme": "semver"}},
    {"id": 2, "name": "performance", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "performance") {
		t.Fatalf("expected error to reference performance, got: %s", errs[0].Message)
	}
}

// TestS9_EmptySectionsArray: zero errors when sections is an empty array.
func TestS9_EmptySectionsArray(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": []
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// TestS10_NoSectionsField: zero errors when project.json has no sections key.
func TestS10_NoSectionsField(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d: %v", len(errs), errs)
	}
}

// TestE1_SectionNameCaseSensitive: section "Delivery" with module "delivery"
// must fail — name match is case-sensitive.
func TestE1_SectionNameCaseSensitive(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "Delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected error for case-mismatched section name, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "Delivery") && strings.Contains(e.Message, "delivery") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error mentioning both names with case difference, got: %v", errs)
	}
}

// TestE2_InvalidSectionSchema: section.schema.json is structurally invalid as JSON Schema.
func TestE2_InvalidSectionSchema(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json",
		`{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "invalid_type"}`)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected schema compilation error, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "compile") || strings.Contains(strings.ToLower(e.Message), "schema") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected message about schema compilation, got: %v", errs)
	}
}

// TestE3_NotValidJSON: section.schema.json is malformed JSON.
func TestE3_NotValidJSON(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", `{broken json`)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected JSON parse error, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "parse") || strings.Contains(strings.ToLower(e.Message), "json") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected parse error message, got: %v", errs)
	}
}

// TestE4_EmptyContentFailsRequired: envelope-only section against schema that
// requires extra content must fail content validation.
func TestE4_EmptyContentFailsRequired(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)

	errs := CheckCoupledSections(dir)
	if len(errs) == 0 {
		t.Fatalf("expected content validation error for missing required field, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "versioning") || strings.Contains(strings.ToLower(e.Message), "required") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected message referencing missing versioning/required, got: %v", errs)
	}
}

// TestE5_DuplicateSectionName: two coupled sections sharing a name must be reported.
func TestE5_DuplicateSectionName(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "delivery", "delivery")
	writeFile(t, filepath.Join(dir, "delivery"), "section.schema.json", validSectionSchema)
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "delivery", "path": "delivery"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled", "versioning": {"scheme": "semver"}},
    {"id": 2, "name": "delivery", "type": "coupled", "versioning": {"scheme": "semver"}}
  ]
}`)

	errs := CheckCoupledSections(dir)
	dupCount := 0
	for _, e := range errs {
		m := strings.ToLower(e.Message)
		if strings.Contains(m, "duplicate") && strings.Contains(m, "name") {
			dupCount++
		}
	}
	if dupCount == 0 {
		t.Fatalf("expected duplicate name error, got: %v", errs)
	}
}

// TestREQ_d99ef6b9b776_AllSeverityError: every emitted error has severity=error.
func TestREQ_d99ef6b9b776_AllSeverityError(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "core", "core")
	writeProject(t, dir, `{
  "name": "p",
  "modules": [{"id": 1, "name": "core", "path": "core"}],
  "sections": [
    {"id": 1, "name": "delivery", "type": "coupled"}
  ]
}`)
	errs := CheckCoupledSections(dir)
	for _, e := range errs {
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
		if e.Check != "coupled_section" {
			t.Fatalf("expected check=coupled_section, got %q", e.Check)
		}
	}
}
