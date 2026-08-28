package validator

import (
	"path/filepath"
	"strings"
	"testing"
)

// REQ-5: ID uniqueness — no duplicate IDs within any array.
// REQ-6: Cross-reference integrity — all reference targets exist.
// REQ-13: Priority on project requirements.

// --- I1: All IDs unique and references valid ---

func TestREQ5_ValidIDsReturnsEmpty(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_valid"))
	if len(errs) > 0 {
		t.Fatalf("expected no errors, got %d: %v", len(errs), errs)
	}
}

// --- I2: Duplicate requirement IDs within a module ---

func TestREQ5_DuplicateReqIDsInModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/requirements") &&
			strings.Contains(e.Message, "duplicate ID 000000000001") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate requirement ID error in module, got none")
	}
}

// --- I3: Duplicate component IDs within a module ---

func TestREQ5_DuplicateCompIDsInModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/components") &&
			strings.Contains(e.Message, "duplicate ID 000000000001") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate component ID error in module, got none")
	}
}

// --- I4: Duplicate test_section IDs within a module ---

func TestREQ5_DuplicateTestSectionIDs(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup_testsec"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/test_sections") &&
			strings.Contains(e.Message, "duplicate ID 000000000001") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate test_section ID error, got none")
	}
}

// --- I5: Duplicate module IDs in project.json ---

func TestREQ5_DuplicateModuleIDs(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup_module"))
	found := false
	for _, e := range errs {
		if e.Path == "project.json:/modules" &&
			strings.Contains(e.Message, "duplicate ID 000000000001") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected duplicate module ID error in project.json, got none")
	}
}

// --- I6: Component implements references non-existent requirement ---

func TestREQ6_DanglingImplements(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "implements references non-existent requirement 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling implements error, got none")
	}
}

// --- I7: Component uses references non-existent component ---

func TestREQ6_DanglingUses(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "components") &&
			strings.Contains(e.Message, "uses references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling component uses error, got none")
	}
}

// --- I8: test_section describes references non-existent component ---

func TestREQ6_DanglingDescribes(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "test_sections") &&
			strings.Contains(e.Message, "describes references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling test_section describes error, got none")
	}
}

// --- I9: Requirement depends_on references non-existent requirement ---

func TestREQ6_DanglingDependsOn(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "depends_on references non-existent requirement 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling depends_on error, got none")
	}
}

// --- I10: Module requires_module references non-existent module ---

func TestREQ6_DanglingRequiresModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "requires_module references non-existent module 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling requires_module error, got none")
	}
}

// --- I12: Requirement preq_id references non-existent project requirement ---

func TestREQ6_DanglingPreqID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "preq_id references non-existent project requirement 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling preq_id error, got none")
	}
}

// --- I13: Multiple dangling references in one module ---

func TestREQ6_MultipleDanglingRefsReported(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))

	// Should have at least 3 module-level errors: implements, uses, describes.
	moduleErrs := 0
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:") {
			moduleErrs++
		}
	}
	if moduleErrs < 3 {
		t.Fatalf("expected at least 3 module-level errors, got %d", moduleErrs)
	}
}

// --- I14: data_flow uses references non-existent component ---

func TestREQ6_DanglingDataFlowUses(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "data_flows") &&
			strings.Contains(e.Message, "uses references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling data_flow uses error, got none")
	}
}

// --- I15: Module requirement missing preq_id fails validation ---

func TestREQ6_MissingPreqID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_missing_preq"))
	if len(errs) == 0 {
		t.Fatal("expected error for missing preq_id, got none")
	}

	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "missing preq_id") {
			found = true
			if e.Check != "id" {
				t.Fatalf("expected check=id, got %q", e.Check)
			}
			if e.Severity != "error" {
				t.Fatalf("expected severity=error, got %q", e.Severity)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'missing preq_id' error, got: %v", errs)
	}
}

// --- I16: Project requirement missing priority field ---

func TestREQ13_MissingPriority(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_missing_priority"))
	if len(errs) == 0 {
		t.Fatal("expected error for missing priority, got none")
	}

	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "missing priority") {
			found = true
			if e.Check != "id" {
				t.Fatalf("expected check=id, got %q", e.Check)
			}
			if e.Severity != "error" {
				t.Fatalf("expected severity=error, got %q", e.Severity)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'missing priority' error, got: %v", errs)
	}
}

// --- I17: Project requirement with out-of-range priority ---

func TestREQ13_PriorityOutOfRange(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_priority_range"))
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 priority range errors (5 and -1), got %d: %v", len(errs), errs)
	}

	for _, e := range errs {
		if !strings.Contains(e.Message, "out of range") {
			t.Fatalf("expected 'out of range' in message, got: %s", e.Message)
		}
		if e.Check != "id" {
			t.Fatalf("expected check=id, got %q", e.Check)
		}
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

// --- E2: Same numeric ID reused across different array types ---

func TestREQ5_SameIDacrossTypesIsValid(t *testing.T) {
	// The id_valid fixture already uses ID 1 for requirements, components,
	// data_flows and test_sections. This is valid because
	// IDs only need to be unique within their own array type.
	errs := CheckIDs(filepath.Join("testdata", "id_valid"))
	if len(errs) > 0 {
		t.Fatalf("same ID across different array types should be valid, got errors: %v", errs)
	}
}

// --- E5: test_section describes references non-existent component ---

func TestREQ6_DanglingTestSectionDescribes(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "test_sections") &&
			strings.Contains(e.Message, "describes references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected dangling test_section describes error, got none")
	}
}

// --- A1: Duplicate api IDs within one apis array ---

// TestREQ5_DuplicateAPIIDs closes the gap the api node type shipped with:
// checkModuleUniqueness covered every module array except apis, so two api
// entries could share an id while module.schema.json's own $comment claimed
// the id was "unique within the apis array".
func TestREQ5_DuplicateAPIIDs(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_dup_id"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/apis") &&
			strings.Contains(e.Message, "duplicate ID 0000000000a1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate api ID error, got %v", errs)
	}
}

// --- A2: API names are globally unique across modules ---

// TestREQ5_DuplicateAPINameAcrossModules pins the one uniqueness rule that is
// not per-array: an api name is the external surface string callers type, so
// two modules cannot both claim it.
func TestREQ5_DuplicateAPINameAcrossModules(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_dup_name"))
	found := false
	for _, e := range errs {
		if e.Check == "id" && strings.Contains(e.Message, `duplicate api name "spex diff"`) &&
			strings.Contains(e.Message, "core, edge") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cross-module duplicate api name error, got %v", errs)
	}
}

func TestREQ5_DuplicateAPINameWithinModule(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_dup_name_same_module"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, `duplicate api name "spex diff"`) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate api name error within one module, got %v", errs)
	}
}

func TestREQ5_DistinctAPINamesValid(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_provided_by"))
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate api name") {
			t.Fatalf("distinct api names must not collide: %v", e)
		}
	}
}

// --- A3: api.provided_by is module-local ---

// TestREQ6_DanglingProvidedBy: provided_by got no referential-integrity check
// at all when apis shipped, while component.uses, test_section.describes,
// test_section.describes and data_flow.uses were all checked against the
// module's component set.
func TestREQ6_DanglingProvidedBy(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_provided_by"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/apis/0000000000a2") &&
			strings.Contains(e.Message, "provided_by references non-existent component 000000000000") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dangling provided_by error, got %v", errs)
	}
}

// TestREQ6_ProvidedByIsModuleLocal: a real component id belonging to another
// module is as wrong as an id that exists nowhere — the api belongs to the
// module owning its entry point, and other modules' involvement is carried by
// component `uses` edges.
func TestREQ6_ProvidedByIsModuleLocal(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_provided_by"))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Path, "core/module.json:/apis/0000000000a3") &&
			strings.Contains(e.Message, "provided_by references non-existent component 0000000000b1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cross-module provided_by error, got %v", errs)
	}
	for _, e := range errs {
		if strings.Contains(e.Path, "0000000000a1") {
			t.Fatalf("a module-local provided_by must pass, got %v", e)
		}
	}
}

// --- I18: Cross-reference integrity over profile-declared edges ---

// TestREQ6_DanglingProfileDeclaredEdge: spec/profile.json declares a custom
// "endpoint" type carrying a "serves" edge to components — a reference field
// no built-in type has. The dangling target is still caught, resolved by
// the same set-membership machinery as implements/uses/describes/
// provided_by, because a profile-declared edge is not a fixed set of seven
// field names; it is whatever the resolved profile lists.
func TestREQ6_DanglingProfileDeclaredEdge(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_profile_serves"))
	found := false
	for _, e := range errs {
		if e.Check == "id" &&
			e.Path == "alpha/module.json:/endpoints/0000000000e1" &&
			strings.Contains(e.Message, "serves references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dangling serves error for profile-declared endpoint type, got %v", errs)
	}
}

// TestREQ6_DanglingProfileDeclaredEdgeSharesBuiltinKind: a profile can
// declare a second "uses" edge from a profile-declared type ("endpoint")
// alongside the built-in "uses" edge from component/data_flow. Partitioning
// coverage on kind name alone would treat "uses" as fully built-in and skip
// this from-type entirely, leaving the dangling reference unchecked by both
// the hardcoded path and the generic one. Coverage must be per (kind,
// from-type) pair.
func TestREQ6_DanglingProfileDeclaredEdgeSharesBuiltinKind(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_profile_uses_second_from"))
	found := false
	for _, e := range errs {
		if e.Check == "id" &&
			e.Path == "alpha/module.json:/endpoints/0000000000e1" &&
			strings.Contains(e.Message, "uses references non-existent component 000000000099") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dangling uses error for profile-declared endpoint from-type sharing the built-in \"uses\" kind, got %v", errs)
	}
}

// TestREQ5_ProfileDeclaredNameDeclarableTypeChecked: checkNameRecoverability
// used to iterate a hardcoded component/api pair. A profile that marks a
// third module-scoped type ("endpoint") name-declarable via NodeType.
// NameDeclarable gets that type's names shape-checked too, using the same
// per-type flag CheckRemovedNames already reads for the removal sweep
// (nameDeclarableNodeTypes) — the two can no longer drift apart.
func TestREQ5_ProfileDeclaredNameDeclarableTypeChecked(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_profile_name_declarable"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Check != "id" || errs[0].Severity != "error" {
		t.Fatalf("want check=id severity=error, got %+v", errs[0])
	}
	if !strings.Contains(errs[0].Message, `endpoint name "Widget (v2)" is not its own corpus tokenization`) {
		t.Fatalf("want the endpoint name-shape error, got %q", errs[0].Message)
	}
	if errs[0].Path != "alpha/module.json:/endpoints/0000000000e1" {
		t.Fatalf("want the declaring entry in the path, got %q", errs[0].Path)
	}
}

// TestREQ5_ProfileDeclaredTypeUniquenessChecked: ID uniqueness used to be
// checked only for the five arrays schema.ModuleSpec has typed fields for. A
// profile-declared type beyond those five (here "endpoint", read generically
// off raw JSON) still gets its array checked for duplicate ids.
func TestREQ5_ProfileDeclaredTypeUniquenessChecked(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_profile_extra_dup"))
	found := false
	for _, e := range errs {
		if e.Check == "id" && e.Path == "alpha/module.json:/endpoints" &&
			strings.Contains(e.Message, "duplicate ID 0000000000e1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate ID error for profile-declared endpoint type, got %v", errs)
	}
}

// --- Structural tests ---

func TestREQ5_DuplicatesBlockRefChecks(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	for _, e := range errs {
		if !strings.Contains(e.Message, "duplicate ID") {
			t.Fatalf("expected only duplicate errors when duplicates exist, got: %s", e.Message)
		}
	}
}

func TestREQ5_AllDuplicateErrorsTaggedID(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dup"))
	for _, e := range errs {
		if e.Check != "id" {
			t.Fatalf("expected check=id, got %q", e.Check)
		}
		if e.Severity != "error" {
			t.Fatalf("expected severity=error, got %q", e.Severity)
		}
	}
}

func TestREQ6_DanglingRefPathIncludesSource(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_dangling"))

	hasProject := false
	hasModule := false
	for _, e := range errs {
		if strings.HasPrefix(e.Path, "project.json:") {
			hasProject = true
		}
		if strings.HasPrefix(e.Path, "core/module.json:") {
			hasModule = true
		}
	}
	if !hasProject {
		t.Fatal("expected at least one error with project.json path")
	}
	if !hasModule {
		t.Fatal("expected at least one error with module path")
	}
}

func TestREQ5_SelfValidateIDs(t *testing.T) {
	specDir := filepath.Join("..", "spec")
	errs := CheckIDs(specDir)
	for _, e := range errs {
		t.Fatalf("unexpected error in own spec: %v", e)
	}
}

// --- A3: api and component names must stay recoverable ---

// TestREQ5_NameWordBoundAcceptsExactly pins the lower edge of the shared
// maxNameWords bound: a name of exactly the bound is legal to declare, and the
// removal sweep can build a phrase that long (see
// TestREQ_6f8284df92a2_NameWordBoundIsExact). Lowering the constant breaks
// this fixture; raising it breaks TestREQ5_NameLongerThanBoundRejected.
func TestREQ5_NameWordBoundAcceptsExactly(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_name_shape_ok"))
	if len(errs) != 0 {
		t.Fatalf("names of exactly maxNameWords words are legal, got %v", errs)
	}
}

// TestREQ5_NameLongerThanBoundRejected: the sweep never builds a phrase longer
// than maxNameWords, so a longer name is unsweepable by construction. The
// bound is enforced where names are declared rather than raised in the scan,
// which is what keeps "declarable" and "sweepable" the same set.
func TestREQ5_NameLongerThanBoundRejected(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_name_too_long"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Check != "id" || errs[0].Severity != "error" {
		t.Fatalf("want check=id severity=error, got %+v", errs[0])
	}
	if !strings.Contains(errs[0].Message, "has 7 words") ||
		!strings.Contains(errs[0].Message, "at most 6 are allowed") {
		t.Fatalf("message must name both counts, got %q", errs[0].Message)
	}
	if !strings.Contains(errs[0].Path, "core/module.json:/apis/") {
		t.Fatalf("want the declaring array in the path, got %q", errs[0].Path)
	}
}

// TestREQ5_APINameSurroundingWhitespaceRejected: api name uniqueness is
// byte-exact, so "spex six " and "spex six" are two surfaces that render
// identically and hash differently. Case-sensitivity is defensible for an
// exact external surface string; invisible whitespace is not.
func TestREQ5_APINameSurroundingWhitespaceRejected(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_api_name_whitespace"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, `api name "spex six " is not its own corpus tokenization`) ||
		!strings.Contains(errs[0].Message, `reduces to "spex six"`) {
		t.Fatalf("message must show both forms, got %q", errs[0].Message)
	}
}

// TestREQ5_DeclarableIsExactlySweepable is the claim the two halves used to
// assert and not hold. The validator enforced strings.Fields normalization and
// the word bound; the sweep additionally ran normalizeToken over every token
// with tokenTrimCutset. Everything in that gap validated clean and could never
// be rebuilt from its hash, so the removal was unsweepable from the moment the
// name was written — silently, and only at removal time would anyone find out.
//
// Every rejected case here was measured as accepted-and-unbuildable before
// declarableName replaced the two-rule check. The realism is the point: an api
// name is "the exact surface string as callers write it" (schema.go), and
// `[…]` is how an optional argument is written; a parenthetical is how a
// component name is disambiguated.
func TestREQ5_DeclarableIsExactlySweepable(t *testing.T) {
	tests := []struct {
		name       string
		declarable bool
		phrase     string
	}{
		{"spex validate [--json]", false, "spex validate --json"},
		{"Validator (core)", false, "Validator core"},
		{"Widget.", false, "Widget"},
		{"Note: thing", false, "Note thing"},
		{"_private", false, "private"},
		{"name,", false, "name"},
		{"***", false, ""},
		{"Bob's", false, "Bob"},
		{`""`, false, ""},
		{"", false, ""},
		{"spex six ", false, "spex six"},
		{"spex render --format json --slim --bare --extra", false, "spex render --format json --slim --bare --extra"},

		{"spex validate --json", true, "spex validate --json"},
		{"OrphanDetector", true, "OrphanDetector"},
		{"GET /v1/specs/{id}", true, "GET /v1/specs/{id}"},
		{"schema.IdentityHash", true, "schema.IdentityHash"},
		{"spex render --format json --slim --bare", true, "spex render --format json --slim --bare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phrase, ok := declarableName(tt.name)
			if ok != tt.declarable {
				t.Fatalf("declarableName(%q) = %v, want %v (phrase %q)", tt.name, ok, tt.declarable, phrase)
			}
			if phrase != tt.phrase {
				t.Fatalf("declarableName(%q) phrase = %q, want %q", tt.name, phrase, tt.phrase)
			}
		})
	}
}

// TestREQ5_DistinctDeclarableNamesNeverCollide: `spex validate [--json]` and
// `spex validate --json` used to be two declarable names that tokenized to one
// phrase, so a hit on either was mutually mis-attributable — the sweep could
// blame the wrong node for a mention, or clear the wrong one. A declarable name
// is its own tokenization, so the mapping is the identity and cannot collide.
func TestREQ5_DistinctDeclarableNamesNeverCollide(t *testing.T) {
	names := []string{
		"spex validate --json",
		"spex validate",
		"Validator core",
		"Validator",
		"OrphanDetector",
		"spex render --format json --slim --bare",
	}
	seen := map[string]string{}
	for _, n := range names {
		phrase, ok := declarableName(n)
		if !ok {
			t.Fatalf("fixture %q must be declarable", n)
		}
		if prev, dup := seen[phrase]; dup {
			t.Fatalf("names %q and %q collapse to the same phrase %q", prev, n, phrase)
		}
		seen[phrase] = n
	}
}

// TestREQ5_ComponentNameShapeChecked pins the half of checkNameRecoverability
// that has live subjects. The corpus declares 51 component names and zero apis,
// so a check that only covered the api loop covered nothing that exists —
// removing `range mod.Components` used to leave every test green.
func TestREQ5_ComponentNameShapeChecked(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_component_name_shape"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0].Check != "id" || errs[0].Severity != "error" {
		t.Fatalf("want check=id severity=error, got %+v", errs[0])
	}
	if !strings.Contains(errs[0].Message, `component name "Validator (core)" is not its own corpus tokenization`) ||
		!strings.Contains(errs[0].Message, `reduces to "Validator core"`) {
		t.Fatalf("message must name the component and its tokenization, got %q", errs[0].Message)
	}
	if errs[0].Path != "core/module.json:/components/0000000000c1" {
		t.Fatalf("want the declaring entry in the path, got %q", errs[0].Path)
	}
}

// TestREQ5_ComponentNameLongerThanBoundRejected: the word bound applies to
// components too, not just to the long api names it was written for.
func TestREQ5_ComponentNameLongerThanBoundRejected(t *testing.T) {
	errs := CheckIDs(filepath.Join("testdata", "id_component_name_too_long"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Message, "component") || !strings.Contains(errs[0].Message, "has 7 words") {
		t.Fatalf("want the component word-bound error, got %q", errs[0].Message)
	}
}
