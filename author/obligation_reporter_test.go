package author

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// obligationFixture is the "Node editing tests" fixture test_obligation.md's
// Setup reuses: module alpha, project requirement P1 (derived into module
// requirement R1), Comp1 (implements R1), Comp2 (uses Comp1), test section
// T1 (describes Comp1 and Comp2), and api "demo run" (provided_by Comp1).
// It returns the directory and the identity hashes a scenario needs to
// build its own change on top of it.
type obligationFixture struct {
	dir              string
	p1ID, p2ID       string
	r1ID             string
	comp1ID, comp2ID string
	t1ID             string
	apiID            string
}

func buildObligationFixture(t *testing.T) obligationFixture {
	t.Helper()
	dir := t.TempDir()

	f := obligationFixture{
		dir:     dir,
		p1ID:    schema.IdentityHash("project", "requirement", "P1"),
		p2ID:    schema.IdentityHash("project", "requirement", "P2"),
		r1ID:    schema.IdentityHash("alpha", "requirement", "R1"),
		comp1ID: schema.IdentityHash("alpha", "component", "Comp1"),
		comp2ID: schema.IdentityHash("alpha", "component", "Comp2"),
		t1ID:    schema.IdentityHash("alpha", "test_section", "T1"),
		apiID:   schema.IdentityHash("alpha", "api", "demo run"),
	}

	proj := schema.Project{
		Name: "test-project",
		Requirements: []schema.Requirement{
			{ID: f.p1ID, Type: "functional", Title: "P1"},
			{ID: f.p2ID, Type: "functional", Title: "P2", Derivation: "pending"},
		},
		Modules: []schema.Module{
			{ID: "000000000001", Name: "alpha", Path: "alpha"},
		},
	}
	writeJSON(t, filepath.Join(dir, "project.json"), proj)

	alphaDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(alphaDir, 0755); err != nil {
		t.Fatal(err)
	}

	mod := schema.ModuleSpec{
		Name: "alpha",
		Requirements: []schema.ModuleRequirement{
			{ID: f.r1ID, PreqID: f.p1ID, Type: "functional", Title: "R1"},
		},
		Components: []schema.Component{
			{ID: f.comp1ID, Name: "Comp1", Content: "arch_comp1.md", Implements: []string{f.r1ID}},
			{ID: f.comp2ID, Name: "Comp2", Content: "arch_comp2.md", Uses: []string{f.comp1ID}},
		},
		TestSections: []schema.TestSection{
			{ID: f.t1ID, Name: "T1", Content: "test_t1.md", Describes: []string{f.comp1ID, f.comp2ID}},
		},
		APIs: []schema.API{
			{ID: f.apiID, Name: "demo run", ProvidedBy: []string{f.comp1ID}},
		},
	}
	writeJSON(t, filepath.Join(alphaDir, "module.json"), mod)

	writeFile(t, filepath.Join(alphaDir, "arch_comp1.md"), "# Comp1\n\nImplements [["+f.r1ID+"|R1]].\n")
	writeFile(t, filepath.Join(alphaDir, "arch_comp2.md"), "# Comp2\n\nUses [["+f.comp1ID+"|Comp1]].\n")
	writeFile(t, filepath.Join(alphaDir, "test_t1.md"), "# T1\n\nDescribes [["+f.comp1ID+"|Comp1]] and [["+f.comp2ID+"|Comp2]].\n")

	return f
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// memFSFromDir snapshots a real directory into a validator.MemFS — the
// "tree as the worker left it in memory" a scenario builds by starting from
// the fixture's on-disk state and mutating the copy, never the fixture
// itself and never disk (flow_authoring.md, "Every write": "The worker
// applies the change to an in-memory copy and hands the before-and-after
// pair to ObligationReporter").
func memFSFromDir(t *testing.T, dir string) validator.MemFS {
	t.Helper()
	m := validator.MemFS{}
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
		m[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("memFS from dir: %v", err)
	}
	return m
}

// cloneMemFS makes a fresh top-level copy of m, so a scenario chained on
// top of another (O3 on O1) can mutate its own copy without perturbing the
// state the first scenario built.
func cloneMemFS(m validator.MemFS) validator.MemFS {
	out := make(validator.MemFS, len(m))
	for k, v := range m {
		data := make([]byte, len(v))
		copy(data, v)
		out[k] = data
	}
	return out
}

func readModuleSpecMem(t *testing.T, m validator.MemFS) schema.ModuleSpec {
	t.Helper()
	var mod schema.ModuleSpec
	if err := json.Unmarshal(m["alpha/module.json"], &mod); err != nil {
		t.Fatalf("parse alpha/module.json: %v", err)
	}
	return mod
}

func writeModuleSpecMem(t *testing.T, m validator.MemFS, mod schema.ModuleSpec) {
	t.Helper()
	data, err := json.MarshalIndent(mod, "", "  ")
	if err != nil {
		t.Fatalf("marshal alpha/module.json: %v", err)
	}
	m["alpha/module.json"] = data
}

func readProjectMem(t *testing.T, m validator.MemFS) schema.Project {
	t.Helper()
	var proj schema.Project
	if err := json.Unmarshal(m["project.json"], &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	return proj
}

func writeProjectMem(t *testing.T, m validator.MemFS, proj schema.Project) {
	t.Helper()
	data, err := json.MarshalIndent(proj, "", "  ")
	if err != nil {
		t.Fatalf("marshal project.json: %v", err)
	}
	m["project.json"] = data
}

func mustProfile(t *testing.T, dir string) *schema.Profile {
	t.Helper()
	p, err := schema.ResolveProfile(dir)
	if err != nil {
		t.Fatalf("resolve profile: %v", err)
	}
	return p
}

// obligationKeys returns the Message of each entry in obligations, in the
// order Report produced them (completeness entries first, then validator
// findings) — used for order-independent substring checks via
// containsSubstring, not sorted.
func obligationKeys(t *testing.T, obligations []merkle.DiffError) []string {
	t.Helper()
	var keys []string
	for _, o := range obligations {
		keys = append(keys, o.Message)
	}
	return keys
}

// obligationsOfType returns the entries in obligations whose Type equals
// typ — e.g. "requirement_coverage" for a validator finding, as opposed to
// "incomplete_change" for a merkle.CheckCompleteness entry.
func obligationsOfType(obligations []merkle.DiffError, typ string) []merkle.DiffError {
	var out []merkle.DiffError
	for _, o := range obligations {
		if o.Type == typ {
			out = append(out, o)
		}
	}
	return out
}

func containsSubstring(list []string, sub string) bool {
	for _, s := range list {
		if bytes.Contains([]byte(s), []byte(sub)) {
			return true
		}
	}
	return false
}

// O1: An added requirement prints the implementation it now owes.
func TestO1_AddedRequirement_PrintsUnimplemented(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	after := memFSFromDir(t, f.dir)
	mod := readModuleSpecMem(t, after)
	r2ID := schema.IdentityHash("alpha", "requirement", "R2")
	mod.Requirements = append(mod.Requirements, schema.ModuleRequirement{
		ID: r2ID, PreqID: f.p1ID, Type: "functional", Title: "R2",
	})
	writeModuleSpecMem(t, after, mod)

	refusals, obligations, err := Report(os.DirFS(f.dir), after, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(refusals) != 0 {
		t.Fatalf("want no refusals, got %+v", refusals)
	}

	completeness := obligationsOfType(obligations, "incomplete_change")
	if len(completeness) != 1 {
		t.Fatalf("want exactly one completeness obligation (the meta-obligation is suppressed by the requirement change), got %+v", completeness)
	}
	want := "requirement 'R2' (alpha) added but not implemented by any component"
	if completeness[0].Message != want {
		t.Fatalf("want obligation %q, got %q", want, completeness[0].Message)
	}

	// R2 has no implementor, so the validator's own requirement_coverage
	// check on the after-state finds the same gap; the change the validator
	// would accept still surfaces its finding as an obligation, not a
	// refusal (spec/author/arch_obligation_reporter.md, "Obligations are
	// printed, not discovered").
	reqCoverage := obligationsOfType(obligations, "requirement_coverage")
	if len(reqCoverage) == 0 {
		t.Fatalf("want a requirement_coverage obligation for R2's missing implementor, got %+v", obligations)
	}

	// Parity: spex diff --json against a snapshot of the before-state,
	// taken after the same change lands on disk, reports the completeness
	// entry and no other — spex diff never runs the full validator suite,
	// only the completeness rules Report's own completenessObligations
	// runs (merkle.CheckCompleteness).
	beforeTree, err := merkle.BuildTree(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	afterTree, err := merkle.BuildTreeFS(after)
	if err != nil {
		t.Fatal(err)
	}
	changes := merkle.Diff(afterTree, beforeTree)
	classified := merkle.Classify(changes, merkle.ModuleNames(afterTree), profile)
	diffErrors := merkle.CheckCompletenessFS(classified, after, profile)
	if len(diffErrors) != 1 || diffErrors[0].Message != want {
		t.Fatalf("spex diff parity: want exactly [%q], got %+v", want, diffErrors)
	}
}

// O2: A module.json edit without a requirement change obliges every
// component in the module.
func TestO2_EdgeAddWithoutRequirementChange_ObligesEveryComponent(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	after := memFSFromDir(t, f.dir)
	mod := readModuleSpecMem(t, after)
	for i := range mod.Components {
		if mod.Components[i].ID == f.comp2ID {
			mod.Components[i].Implements = append(mod.Components[i].Implements, f.r1ID)
		}
	}
	writeModuleSpecMem(t, after, mod)

	refusals, obligations, err := Report(os.DirFS(f.dir), after, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(refusals) != 0 {
		t.Fatalf("want no refusals, got %+v", refusals)
	}

	completeness := obligationsOfType(obligations, "incomplete_change")
	keys := obligationKeys(t, completeness)
	if len(completeness) != 2 {
		t.Fatalf("want two completeness obligations (one per component in alpha), got %+v", completeness)
	}
	for _, comp := range []string{"Comp1", "Comp2"} {
		if !containsSubstring(keys, comp) {
			t.Fatalf("want an obligation naming %s, got %+v", comp, keys)
		}
	}
}

// O3: A requirement change in the same module suppresses the meta
// obligation. Chained on top of O1's change, with no fresh snapshot taken
// in between — see the note on this test below.
func TestO3_EdgeAddToRecentlyAddedRequirement(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	// O1's own change, held only in memory — mirrors O1 exactly and gives
	// O3 a real "state after O1" to build its own before/after pair from,
	// without either scenario ever touching disk (ObligationReporter
	// itself never writes; see Report's doc comment).
	afterO1 := memFSFromDir(t, f.dir)
	mod := readModuleSpecMem(t, afterO1)
	r2ID := schema.IdentityHash("alpha", "requirement", "R2")
	mod.Requirements = append(mod.Requirements, schema.ModuleRequirement{
		ID: r2ID, PreqID: f.p1ID, Type: "functional", Title: "R2",
	})
	writeModuleSpecMem(t, afterO1, mod)
	if refusals, _, err := Report(os.DirFS(f.dir), afterO1, profile); err != nil || len(refusals) != 0 {
		t.Fatalf("setup: O1's own write must be accepted, got refusals=%+v err=%v", refusals, err)
	}

	// O3: spex edge add source Comp2 field implements target R2, run
	// against the state O1 left behind — ObligationReporter reads no
	// .spex/ baseline (spec/author/arch_obligation_reporter.md, "What it
	// does not do"), so its own before-state is afterO1's contents, not
	// the pristine fixture.
	afterO3 := cloneMemFS(afterO1)
	mod = readModuleSpecMem(t, afterO3)
	for i := range mod.Components {
		if mod.Components[i].ID == f.comp2ID {
			mod.Components[i].Implements = append(mod.Components[i].Implements, r2ID)
		}
	}
	writeModuleSpecMem(t, afterO3, mod)

	refusals, obligations, err := Report(afterO1, afterO3, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(refusals) != 0 {
		t.Fatalf("want no refusals, got %+v", refusals)
	}

	// NOTE on divergence from test_obligation.md's O3 prose: the scenario
	// as written expects this command's own obligations to name Comp2's
	// leaf only, on the theory that "R2's leaf is still an added
	// requirement in this diff". Read literally against
	// arch_obligation_reporter.md's stated mechanism — before-state is
	// whatever is on disk right now, and R2 was already fully written to
	// disk by O1's own prior command — R2 carries an identical hash on
	// both sides of *this* command's diff, so it cannot be classified
	// Added here: only module alpha's meta leaf changed, with no
	// requirement change in this diff, which is exactly O2's shape and
	// obliges every component in the module. Reproducing the scenario's
	// literal expectation would require ObligationReporter to know that
	// R2 is "recently added" across process boundaries — state the arch
	// leaf's "What it does not do" section explicitly rules out reading
	// (there is also no .spex/ baseline to read it from: the fixture is
	// deliberately uninitialised). This is filed as
	// drifts/drift-spexmachina-yih0.6.json rather than silently
	// special-cased here. The assertion below is what the documented,
	// self-consistent mechanism actually — and correctly — produces.
	completeness := obligationsOfType(obligations, "incomplete_change")
	keys := obligationKeys(t, completeness)
	if len(completeness) != 2 {
		t.Fatalf("per the documented before-state-is-disk-now mechanism, want two completeness obligations (module alpha's meta obligation, R2 unchanged in this diff), got %+v", completeness)
	}
	for _, comp := range []string{"Comp1", "Comp2"} {
		if !containsSubstring(keys, comp) {
			t.Fatalf("want an obligation naming %s, got %+v", comp, keys)
		}
	}
}

// O4: A refusal is the validator's error with a fix attached.
func TestO4_DuplicateNode_RefusedWithFix(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	after := memFSFromDir(t, f.dir)
	mod := readModuleSpecMem(t, after)
	// A second "Comp1": same module, type and name, so it derives the
	// exact same id as the existing one — the shape `spex node add`
	// would apply in-memory before ObligationReporter ever runs.
	mod.Components = append(mod.Components, schema.Component{
		ID: f.comp1ID, Name: "Comp1", Content: "arch_comp1_2.md",
	})
	writeModuleSpecMem(t, after, mod)
	after["alpha/arch_comp1_2.md"] = []byte("# Comp1\n")

	refusals, obligations, err := Report(os.DirFS(f.dir), after, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if obligations != nil {
		t.Fatalf("want no obligations on a refusal, got %+v", obligations)
	}
	if len(refusals) == 0 {
		t.Fatal("want at least one refusal for the duplicate id")
	}

	var found bool
	for _, r := range refusals {
		if r.Check != "id" {
			continue
		}
		if r.Message == "duplicate ID "+f.comp1ID {
			found = true
			if r.Fix == "" {
				t.Fatal("want a non-empty fix")
			}
			if !bytes.Contains([]byte(r.Fix), []byte(f.comp1ID)) {
				t.Fatalf("want the fix to name the existing node's id %s, got %q", f.comp1ID, r.Fix)
			}
			if !bytes.Contains([]byte(r.Fix), []byte("alpha/module.json")) {
				t.Fatalf("want the fix to name the existing node's file, got %q", r.Fix)
			}
		}
	}
	if !found {
		t.Fatalf("want a refusal carrying the validator's duplicate-ID message, got %+v", refusals)
	}

	// Parity: the same entry written by hand fails spex validate with the
	// same check and message.
	hand := copyTree(t, f.dir)
	handMod := readModuleSpec(t, hand)
	handMod.Components = append(handMod.Components, schema.Component{
		ID: f.comp1ID, Name: "Comp1", Content: "arch_comp1_2.md",
	})
	writeModuleSpec(t, hand, handMod)
	writeFile(t, filepath.Join(hand, "alpha", "arch_comp1_2.md"), "# Comp1\n")

	handErrs := refusalCheckers(os.DirFS(hand))
	var handFound bool
	for _, e := range handErrs {
		if e.Check == "id" && e.Message == "duplicate ID "+f.comp1ID {
			handFound = true
		}
	}
	if !handFound {
		t.Fatal("parity: hand copy should fail spex validate with the same duplicate-ID message")
	}
}

// O5: A change the validator would accept is never refused.
func TestO5_UnderivedProjectRequirement_AcceptedAsObligation(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	after := memFSFromDir(t, f.dir)
	proj := readProjectMem(t, after)
	priority := 1
	unfulfilledID := schema.IdentityHash("project", "requirement", "Unfulfilled")
	proj.Requirements = append(proj.Requirements, schema.Requirement{
		ID: unfulfilledID, Type: "functional", Title: "Unfulfilled", Priority: &priority,
	})
	writeProjectMem(t, after, proj)

	refusals, obligations, err := Report(os.DirFS(f.dir), after, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if len(refusals) != 0 {
		t.Fatalf("an accepted change must never be refused, got %+v", refusals)
	}

	// The validator's own requirement_coverage finding — "project
	// requirement 'Unfulfilled' is not derived into any module
	// requirement" — is a validation error on the hand copy, not a
	// refusal at write, and it appears under obligations as the finding
	// the change left open: this is the half of O5 that distinguishes it
	// from O1, where only the completeness checker's entry is at stake.
	reqCoverage := obligationsOfType(obligations, "requirement_coverage")
	if len(reqCoverage) == 0 {
		t.Fatalf("want a requirement_coverage obligation naming the underived requirement, got %+v", obligations)
	}
	reqKeys := obligationKeys(t, reqCoverage)
	if !containsSubstring(reqKeys, "Unfulfilled") {
		t.Fatalf("want the requirement_coverage obligation to name Unfulfilled, got %+v", reqKeys)
	}

	// Parity: the validator would indeed flag this on a hand copy, as a
	// validation error rather than a refusal.
	hand := copyTree(t, f.dir)
	handProj := readProject(t, hand)
	handProj.Requirements = append(handProj.Requirements, schema.Requirement{
		ID: unfulfilledID, Type: "functional", Title: "Unfulfilled", Priority: &priority,
	})
	writeProject(t, hand, handProj)
	handErrs, _ := validator.CheckRequirementCoverage(hand)
	var handFound bool
	for _, e := range handErrs {
		if e.Check == "requirement_coverage" && bytes.Contains([]byte(e.Message), []byte("Unfulfilled")) {
			handFound = true
		}
	}
	if !handFound {
		t.Fatal("parity: hand copy should fail spex validate's requirement_coverage check naming Unfulfilled")
	}
}

// O6: Nothing is written when any checker refuses — modeled here as
// "nothing is accepted": a valid addition alongside a schema violation in
// the same after-state refuses as a whole, so a caller following Report's
// answer writes none of it.
func TestO6_PartialRefusal_RefusesWhole(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	after := memFSFromDir(t, f.dir)
	mod := readModuleSpecMem(t, after)
	widgetID := schema.IdentityHash("alpha", "component", "Widget")
	mod.Components = append(mod.Components, schema.Component{
		ID: widgetID, Name: "Widget", Content: "arch_widget.md",
	})
	writeModuleSpecMem(t, after, mod)
	after["alpha/arch_widget.md"] = []byte("# Widget\n")

	// Corrupt Comp1's "content" field to a non-string value: a schema
	// violation the after-state introduces that the before-state did not
	// carry, alongside the otherwise-valid Widget addition above.
	corruptFieldToNumberMem(t, after, f.comp1ID, "content")

	refusals, obligations, err := Report(os.DirFS(f.dir), after, profile)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if obligations != nil {
		t.Fatalf("want no obligations on a refusal, got %+v", obligations)
	}
	if len(refusals) == 0 {
		t.Fatal("want a refusal for the corrupted field")
	}
	var sawSchema bool
	for _, r := range refusals {
		if r.Check == "schema" {
			sawSchema = true
		}
	}
	if !sawSchema {
		t.Fatalf("want a schema refusal, got %+v", refusals)
	}
}

// corruptFieldToNumberMem rewrites one entry's named string field to a JSON
// number in the given MemFS's alpha/module.json — a shape no worker would
// ever produce, used only to manufacture a schema violation for O6.
func corruptFieldToNumberMem(t *testing.T, m validator.MemFS, id, field string) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(m["alpha/module.json"], &raw); err != nil {
		t.Fatal(err)
	}
	var comps []map[string]json.RawMessage
	if err := json.Unmarshal(raw["components"], &comps); err != nil {
		t.Fatal(err)
	}
	for i, c := range comps {
		var cid string
		if err := json.Unmarshal(c["id"], &cid); err != nil {
			continue
		}
		if cid == id {
			comps[i][field] = json.RawMessage("12345")
		}
	}
	comData, err := json.Marshal(comps)
	if err != nil {
		t.Fatal(err)
	}
	raw["components"] = comData
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	m["alpha/module.json"] = out
}

// O7: A read command prints no obligations. ShowProfile (ProfileInspector)
// never calls Report and never writes; its output has no `obligations` key
// because *schema.Profile carries no such field.
func TestO7_ReadCommand_PrintsNoObligations(t *testing.T) {
	f := buildObligationFixture(t)

	before, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if _, err := ShowProfile(f.dir, &buf, false); err != nil {
		t.Fatalf("ShowProfile: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := doc["obligations"]; ok {
		t.Fatal("want no obligations key in a read command's output")
	}

	after, err := os.ReadFile(filepath.Join(f.dir, "alpha", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("want no file changed by a read command")
	}
}

// --- Unit tests for computeFix and its helpers ---

// copyTree copies src to a fresh temp directory and returns it — used only
// where a scenario needs a real hand-edited copy on disk for parity against
// `spex validate`'s own on-disk behavior (O4, O5), not as ObligationReporter's
// own after-state shape.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy tree: %v", err)
	}
	return dst
}

func readModuleSpec(t *testing.T, dir string) schema.ModuleSpec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "alpha", "module.json"))
	if err != nil {
		t.Fatalf("read module.json: %v", err)
	}
	var mod schema.ModuleSpec
	if err := json.Unmarshal(data, &mod); err != nil {
		t.Fatalf("parse module.json: %v", err)
	}
	return mod
}

func writeModuleSpec(t *testing.T, dir string, mod schema.ModuleSpec) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "alpha", "module.json"), mod)
}

func readProject(t *testing.T, dir string) schema.Project {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatalf("read project.json: %v", err)
	}
	var proj schema.Project
	if err := json.Unmarshal(data, &proj); err != nil {
		t.Fatalf("parse project.json: %v", err)
	}
	return proj
}

func writeProject(t *testing.T, dir string, proj schema.Project) {
	t.Helper()
	writeJSON(t, filepath.Join(dir, "project.json"), proj)
}

func TestComputeFix_DuplicateID_NamesExistingFile(t *testing.T) {
	f := buildObligationFixture(t)
	file, ok := findDeclaringFile(os.DirFS(f.dir), f.comp1ID)
	if !ok {
		t.Fatal("want the existing Comp1 to be found")
	}
	if file != "alpha/module.json" {
		t.Fatalf("want alpha/module.json, got %q", file)
	}
}

func TestComputeFix_UnknownID_NotFound(t *testing.T) {
	f := buildObligationFixture(t)
	if _, ok := findDeclaringFile(os.DirFS(f.dir), "000000000000"); ok {
		t.Fatal("want an unknown id not to be found")
	}
}

func TestComputeFix_IDDerivationMismatch_SetsWantHash(t *testing.T) {
	want := schema.IdentityHash("alpha", "component", "Comp1")
	msg := `component "Comp1" declares id 000000000000 but its identity hash is ` + want + `; a module-scoped id must equal IdentityHash("alpha", "component", "Comp1") or the node cannot be recovered from its hash after removal`
	got := computeFix(validatorError("id_derivation", msg), emptyFS(), schema.DefaultProfile())
	if got != "set id to "+want {
		t.Fatalf("got %q", got)
	}
}

func TestComputeFix_DAGCycle_NamesTheCycle(t *testing.T) {
	msg := "uses cycle: Comp1 -> Comp2 -> Comp1"
	got := computeFix(validatorError("dag", msg), emptyFS(), schema.DefaultProfile())
	if got != "remove one edge from the cycle Comp1 -> Comp2 -> Comp1" {
		t.Fatalf("got %q", got)
	}
}

func TestComputeFix_UnrecognizedMessage_FallsBackToMessage(t *testing.T) {
	msg := "some future validator message this reporter does not parse"
	got := computeFix(validatorError("schema", msg), emptyFS(), schema.DefaultProfile())
	if got != msg {
		t.Fatalf("want the fix to fall back to the message, got %q", got)
	}
	if got == "" {
		t.Fatal("fix must never be empty")
	}
}

// TestComputeFix_MissingReferenceTarget_NamesSearchLocation reproduces the
// review's repro for arch_obligation_reporter.md's "a reference target that
// does not exist" row: a target id that never existed in either state names
// the array searched, by file and key, not a verbatim copy of the message.
func TestComputeFix_MissingReferenceTarget_NamesSearchLocation(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	msg := "implements references non-existent requirement aaaaaaaaaaaa"
	e := validatorError("id", msg)
	e.Path = "alpha/module.json:/components/" + f.comp1ID
	got := computeFix(e, os.DirFS(f.dir), profile)

	if got == msg {
		t.Fatalf("want a fix distinct from the verbatim message, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("alpha/module.json")) {
		t.Fatalf("want the fix to name the file searched, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("requirements")) {
		t.Fatalf("want the fix to name the array key searched, got %q", got)
	}
}

// TestComputeFix_RemovedReferenceTarget_NamesRetargetCommand covers "an
// inbound reference blocking a removal": when the missing target did exist
// in the before-state, the fix names the spex edge remove invocation that
// retargets it, and --force.
func TestComputeFix_RemovedReferenceTarget_NamesRetargetCommand(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	msg := "implements references non-existent requirement " + f.r1ID
	e := validatorError("id", msg)
	e.Path = "alpha/module.json:/components/" + f.comp1ID
	got := computeFix(e, os.DirFS(f.dir), profile)

	if !bytes.Contains([]byte(got), []byte("spex edge remove")) {
		t.Fatalf("want the fix to name spex edge remove, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("--force")) {
		t.Fatalf("want the fix to name --force, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte(f.comp1ID)) {
		t.Fatalf("want the fix to name the source id %s, got %q", f.comp1ID, got)
	}
}

// TestComputeFix_UndeclaredType_ListsDeclaredTypes covers "a type the
// profile does not declare": a root-level schema additionalProperties
// violation names the declared types, and module.
func TestComputeFix_UndeclaredType_ListsDeclaredTypes(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	e := validatorError("schema", `additional properties 'widgets' not allowed`)
	e.Path = "alpha/module.json"
	got := computeFix(e, os.DirFS(f.dir), profile)

	for _, want := range []string{"component", "module"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want the fix to list %q among the declared types, got %q", want, got)
		}
	}
}

// TestComputeFix_DisallowedField_ListsDeclaredFields covers "a field the
// source type may not carry": a nested schema additionalProperties
// violation names the fields the profile declares on that type.
func TestComputeFix_DisallowedField_ListsDeclaredFields(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	e := validatorError("schema", `additional properties 'reviewed_by' not allowed`)
	e.Path = "alpha/module.json:/components/" + f.comp1ID
	got := computeFix(e, os.DirFS(f.dir), profile)

	for _, want := range []string{"id", "name", "content"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want the fix to list %q among the declared fields, got %q", want, got)
		}
	}
}

// TestComputeFix_MissingRequiredField_NamesFieldAndKind covers "a required
// field absent": the fix names the field's declared kind alongside its
// name, not just the name — a text field constrained to an enum, as
// requirement's "type" is, tells the agent which values are legal.
func TestComputeFix_MissingRequiredField_NamesFieldAndKind(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	e := validatorError("schema", "missing required property 'type'")
	e.Path = "alpha/module.json:/requirements/0"
	got := computeFix(e, os.DirFS(f.dir), profile)

	for _, want := range []string{"type", "functional", "non_functional"} {
		if !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("want the fix to name %q, got %q", want, got)
		}
	}
}

// TestComputeFix_ModuleReferenceTarget_NamesModulesArray covers
// requires_module's target type, "module" — the frame's fixed interior-node
// concept and never a profile-declared NodeType — which used to leave the
// search-location clause empty and the fix's sentence dangling.
func TestComputeFix_ModuleReferenceTarget_NamesModulesArray(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	e := validatorError("id", "requires_module references non-existent module 000000000099")
	e.Path = "project.json:/modules/000000000001"
	got := computeFix(e, os.DirFS(f.dir), profile)

	if !bytes.Contains([]byte(got), []byte("project.json:/modules")) {
		t.Fatalf("want the fix to name project.json's modules array, got %q", got)
	}
	if bytes.HasSuffix([]byte(got), []byte("searched ")) {
		t.Fatalf("fix must not end mid-clause on a dangling 'searched ', got %q", got)
	}
}

// TestComputeFix_ReferenceTargetFix_NoLocations_NoDanglingSearched guards
// the empty-target-list case the review asked for: an edge kind the profile
// does not declare (so no location is ever found to search) must not leave
// a trailing "searched " with nothing after it.
func TestComputeFix_ReferenceTargetFix_NoLocations_NoDanglingSearched(t *testing.T) {
	f := buildObligationFixture(t)
	profile := mustProfile(t, f.dir)

	e := validatorError("id", "fake_edge references non-existent widget 000000000099")
	e.Path = "alpha/module.json:/components/" + f.comp1ID
	got := computeFix(e, os.DirFS(f.dir), profile)

	if bytes.HasSuffix([]byte(got), []byte("searched ")) || bytes.HasSuffix([]byte(got), []byte("searched")) {
		t.Fatalf("fix must not end with a dangling 'searched', got %q", got)
	}
}

func validatorError(check, message string) validator.ValidationError {
	return validator.ValidationError{Check: check, Message: message}
}

func emptyFS() fs.FS {
	return validator.MemFS{}
}
