package validator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// REQ-6f8284df92a2 (merkle REQ-8): Change completeness validation. The
// removal-time name check is the diff-time half of it — a removal is not
// complete while the corpus still names the node that went away. It lives in
// validator/ because that is where corpus-reading checkers live, and is
// wired into `spex diff`, which is the only command that knows what was
// removed.

// removalFixture builds a spec directory: one module per entry in
// liveComponents/liveAPIs, plus the given corpus files (paths relative to the
// spec dir, parent directories created as needed).
type removalFixture struct {
	modules map[string]*schema.ModuleSpec
	files   map[string]string
	beads   []schema.BeadMapRecord
	hasMap  bool
}

func newRemovalFixture() *removalFixture {
	return &removalFixture{modules: map[string]*schema.ModuleSpec{}, files: map[string]string{}}
}

// withBeadRecord adds one component bead record, the shape ingest writes: the
// module and component names alongside the spec node id they hash into.
func (f *removalFixture) withBeadRecord(module, component string) *removalFixture {
	f.hasMap = true
	f.beads = append(f.beads, schema.BeadMapRecord{
		ID:         len(f.beads) + 1,
		SpecNodeID: schema.IdentityHash(module, "component", component),
		BeadID:     "test-" + component,
		BeadType:   "task",
		Module:     module,
		Component:  component,
	})
	return f
}

// withStaleBeadRecord adds a record whose declared names do not hash into the
// spec node id it claims — the shape a rename leaves behind. It must never be
// believed, only quoted.
func (f *removalFixture) withStaleBeadRecord(module, component, specNodeID string) *removalFixture {
	f.hasMap = true
	f.beads = append(f.beads, schema.BeadMapRecord{
		ID:         len(f.beads) + 1,
		SpecNodeID: specNodeID,
		BeadID:     "test-stale-" + component,
		BeadType:   "task",
		Module:     module,
		Component:  component,
	})
	return f
}

// withEmptyBeadMap writes a bead map with no records, so a test can tell "no
// map on disk" apart from "a map that proves nothing".
func (f *removalFixture) withEmptyBeadMap() *removalFixture {
	f.hasMap = true
	return f
}

// beadMapPath is where build writes the bead map. walkCorpus skips dot-files,
// so keeping it inside the fixture dir does not put it in the corpus.
func beadMapPath(dir string) string { return filepath.Join(dir, ".bead-map.json") }

func (f *removalFixture) module(name string) *schema.ModuleSpec {
	if m, ok := f.modules[name]; ok {
		return m
	}
	m := &schema.ModuleSpec{Name: name}
	f.modules[name] = m
	return m
}

func (f *removalFixture) withComponent(module, name string) *removalFixture {
	m := f.module(module)
	m.Components = append(m.Components, schema.Component{
		ID:   schema.IdentityHash(module, "component", name),
		Name: name,
	})
	return f
}

func (f *removalFixture) withAPI(module, name string) *removalFixture {
	m := f.module(module)
	m.APIs = append(m.APIs, schema.API{
		ID:   schema.IdentityHash(module, "api", name),
		Name: name,
	})
	return f
}

func (f *removalFixture) withFile(path, content string) *removalFixture {
	f.files[path] = content
	return f
}

func (f *removalFixture) build(t *testing.T, extraModules ...string) string {
	t.Helper()
	dir := t.TempDir()

	for _, name := range extraModules {
		f.module(name)
	}

	names := make([]string, 0, len(f.modules))
	for name := range f.modules {
		names = append(names, name)
	}
	slices.Sort(names)

	proj := schema.Project{Name: "removal-test"}
	for _, name := range names {
		proj.Modules = append(proj.Modules, schema.Module{
			ID: schema.IdentityHash("module", name), Name: name, Path: name,
		})
		modDir := filepath.Join(dir, name)
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		data, err := json.Marshal(f.modules[name])
		if err != nil {
			t.Fatalf("marshal module: %v", err)
		}
		writeFile(t, modDir, "module.json", string(data))
	}
	projData, err := json.Marshal(proj)
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}
	writeProject(t, dir, string(projData))

	for path, content := range f.files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if f.hasMap {
		bm := schema.BeadMap{NextID: len(f.beads) + 1, Records: f.beads}
		data, err := json.Marshal(bm)
		if err != nil {
			t.Fatalf("marshal bead map: %v", err)
		}
		if err := os.WriteFile(beadMapPath(dir), data, 0644); err != nil {
			t.Fatalf("write bead map: %v", err)
		}
	}

	return dir
}

// removedChange is what `spex diff` hands the checker for a node that is in
// the snapshot and not in the current spec.
func removedChange(module, nodeType, name string) merkle.ClassifiedChange {
	return merkle.ClassifiedChange{
		Change: merkle.Change{
			Key:      schema.IdentityHash(module, nodeType, name),
			Type:     merkle.Removed,
			NodeType: nodeType,
			Module:   schema.IdentityHash("module", module),
		},
		Module: module,
	}
}

func mustReport(t *testing.T, dir string, changes ...merkle.ClassifiedChange) RemovedNameReport {
	t.Helper()
	report, err := CheckRemovedNames(dir, beadMapPath(dir), changes)
	if err != nil {
		t.Fatalf("CheckRemovedNames: %v", err)
	}
	return report
}

func mustCheck(t *testing.T, dir string, changes ...merkle.ClassifiedChange) []SurvivingName {
	t.Helper()
	return mustReport(t, dir, changes...).Survivors
}

// wantOneNote asserts exactly one note of the given kind whose message says
// what happened.
func wantOneNote(t *testing.T, notes []RemovedNameNote, kind string, substrs ...string) {
	t.Helper()
	if len(notes) != 1 {
		t.Fatalf("want 1 note, got %d: %+v", len(notes), notes)
	}
	if notes[0].Kind != kind {
		t.Fatalf("want note kind %q, got %q", kind, notes[0].Kind)
	}
	for _, s := range substrs {
		if !strings.Contains(notes[0].Message, s) {
			t.Fatalf("want note message containing %q, got %q", s, notes[0].Message)
		}
	}
}

// TestREQ_6f8284df92a2_RemovedComponentNameSurvives is the core case: the
// name is not recorded anywhere after the removal, and is recovered from the
// corpus by hashing candidate phrases against the removed key.
func TestREQ_6f8284df92a2_RemovedComponentNameSurvives(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "DAGChecker").
		withFile("validator/arch_dag_checker.md", "# DAGChecker\n\nThe OrphanDetector used to run here.\n").
		withFile("validator/test_pipeline.md", "OrphanDetector scenarios.\n").
		build(t)

	found := mustCheck(t, dir, removedChange("validator", "component", "OrphanDetector"))

	if len(found) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(found), found)
	}
	if found[0].Name != "OrphanDetector" {
		t.Fatalf("want recovered name %q, got %q", "OrphanDetector", found[0].Name)
	}
	if found[0].Key != schema.IdentityHash("validator", "component", "OrphanDetector") {
		t.Fatalf("wrong key: %q", found[0].Key)
	}
	if found[0].NodeType != "component" || found[0].Module != "validator" {
		t.Fatalf("wrong node identity: %+v", found[0])
	}
	want := []string{"validator/arch_dag_checker.md:3", "validator/test_pipeline.md:1"}
	if strings.Join(found[0].Sites, ",") != strings.Join(want, ",") {
		t.Fatalf("want sites %v, got %v", want, found[0].Sites)
	}
}

// TestREQ_6f8284df92a2_SweptCorpusPasses: once the mentions are gone the
// check is silent, which is the state every removal must reach.
func TestREQ_6f8284df92a2_SweptCorpusPasses(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "DAGChecker").
		withFile("validator/arch_dag_checker.md", "# DAGChecker\n\nNothing here names the removed node.\n").
		build(t)

	if found := mustCheck(t, dir, removedChange("validator", "component", "OrphanDetector")); len(found) != 0 {
		t.Fatalf("want 0 findings for a swept corpus, got %+v", found)
	}
}

// TestREQ_6f8284df92a2_ProposalsOutsideGate: a proposal describes the change
// that removed the node and must go on naming it.
func TestREQ_6f8284df92a2_ProposalsOutsideGate(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "DAGChecker").
		withFile("proposals/2026-07-25-retire.md", "This proposal removes OrphanDetector.\n").
		build(t)

	if found := mustCheck(t, dir, removedChange("validator", "component", "OrphanDetector")); len(found) != 0 {
		t.Fatalf("proposals are outside the gate, got %+v", found)
	}
}

// TestREQ_6f8284df92a2_GeneratedStateSkipped: .snapshot.json is machine state,
// not prose.
func TestREQ_6f8284df92a2_GeneratedStateSkipped(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "DAGChecker").
		withFile(".snapshot.json", `{"note": "OrphanDetector", "kind": "component"}`).
		build(t)

	if found := mustCheck(t, dir, removedChange("validator", "component", "OrphanDetector")); len(found) != 0 {
		t.Fatalf("generated state is not corpus, got %+v", found)
	}
}

// TestREQ_6f8284df92a2_LongestMatchFirst reproduces the measured case: "spex
// map" occurs 34 times in this project's corpus, 29 of them inside "spex map
// get", "spex map list" or "spex map context". Without the subtraction,
// removing the bare api reports every one of them.
func TestREQ_6f8284df92a2_LongestMatchFirst(t *testing.T) {
	body := "Run `spex map get` to read one record.\n" +
		"`spex map list` prints all of them, and `spex map context` the spec.\n" +
		"The bare spex map subcommand is gone.\n"
	dir := newRemovalFixture().
		withAPI("map", "spex map get").
		withAPI("map", "spex map list").
		withAPI("map", "spex map context").
		withFile("map/arch_map_command.md", body).
		build(t)

	found := mustCheck(t, dir, removedChange("map", "api", "spex map"))

	if len(found) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(found), found)
	}
	if len(found[0].Sites) != 1 || found[0].Sites[0] != "map/arch_map_command.md:3" {
		t.Fatalf("want only the bare mention on line 3, got %v", found[0].Sites)
	}
}

// TestREQ_6f8284df92a2_LongerLiveNameAloneIsSilent: when every mention is
// consumed there is nothing to report.
func TestREQ_6f8284df92a2_LongerLiveNameAloneIsSilent(t *testing.T) {
	dir := newRemovalFixture().
		withAPI("map", "spex map get").
		withFile("map/arch_map_command.md", "Call `spex map get` twice; `spex map get` is idempotent.\n").
		build(t)

	if found := mustCheck(t, dir, removedChange("map", "api", "spex map")); len(found) != 0 {
		t.Fatalf("every hit is consumed by a longer live name, got %+v", found)
	}
}

// TestREQ_6f8284df92a2_OnlyAPIAndComponentNamesSearched: the 56 impl_section
// names are generic noun phrases — "Hash computation" alone survives sixteen
// times in another module's test leaves.
func TestREQ_6f8284df92a2_OnlyAPIAndComponentNamesSearched(t *testing.T) {
	corpus := "Hash computation is described here.\nThe Snapshot storage requirement too.\n"
	tests := []struct {
		nodeType string
		name     string
	}{
		{"impl_section", "Hash computation"},
		{"requirement", "Snapshot storage"},
		{"data_flow", "Hash computation"},
		{"test_section", "Hash computation"},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			dir := newRemovalFixture().
				withFile("merkle/impl_hashing.md", corpus).
				build(t, "merkle")

			if found := mustCheck(t, dir, removedChange("merkle", tt.nodeType, tt.name)); len(found) != 0 {
				t.Fatalf("%s names are not searched, got %+v", tt.nodeType, found)
			}
		})
	}
}

// TestREQ_6f8284df92a2_RemovedAPINameSurvives: apis produce no beads and so
// never appear in .bead-map.json, the one other hash → name table. Hashing
// corpus phrases is the only way their names can be recovered at all.
func TestREQ_6f8284df92a2_RemovedAPINameSurvives(t *testing.T) {
	dir := newRemovalFixture().
		withAPI("emit", "spex emit").
		withFile("emit/arch_emit_command.md", "Legacy callers still run spex apply after emit.\n").
		build(t)

	found := mustCheck(t, dir, removedChange("emit", "api", "spex apply"))

	if len(found) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(found), found)
	}
	if found[0].Name != "spex apply" || found[0].NodeType != "api" {
		t.Fatalf("wrong finding: %+v", found[0])
	}
}

// TestREQ_6f8284df92a2_MentionsFoundThroughMarkup: the name has to be found
// through the punctuation prose wraps it in, since a hit is only ever
// confirmed by an exact hash match on the stripped phrase.
func TestREQ_6f8284df92a2_MentionsFoundThroughMarkup(t *testing.T) {
	tests := []struct{ name, body string }{
		{"code span", "The `OrphanDetector` ran last.\n"},
		{"bold", "The **OrphanDetector** ran last.\n"},
		{"parenthesised", "(OrphanDetector) ran last.\n"},
		{"sentence end", "Nothing calls OrphanDetector.\n"},
		{"possessive", "The OrphanDetector's pass ran last.\n"},
		{"json value", "{\n  \"component\": \"OrphanDetector\",\n  \"id\": \"x\"\n}\n"},
		{"line wrapped api", "Callers run spex\nmap today.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ".md"
			if tt.name == "json value" {
				ext = ".json"
			}
			change := removedChange("validator", "component", "OrphanDetector")
			if tt.name == "line wrapped api" {
				change = removedChange("validator", "api", "spex map")
			}
			dir := newRemovalFixture().
				withFile("validator/leaf"+ext, tt.body).
				build(t, "validator")

			if found := mustCheck(t, dir, change); len(found) != 1 {
				t.Fatalf("want 1 finding, got %d: %+v", len(found), found)
			}
		})
	}
}

// TestREQ_6f8284df92a2_RemovedModuleNameRecovered: retiring a whole module is
// the highest-volume removal there is, and it is the one case where `spex
// diff` reports the module as a hash because project.json no longer names it.
// The module name is recovered from the corpus the same way node names are,
// and the sweep then runs normally.
func TestREQ_6f8284df92a2_RemovedModuleNameRecovered(t *testing.T) {
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator") // unresolved: module gone

	dir := newRemovalFixture().
		withFile("other/leaf.md", "The validator module is gone, but OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 1 {
		t.Fatalf("want 1 finding once the module name is recovered, got %+v", report.Survivors)
	}
	if report.Survivors[0].Module != "validator" || report.Survivors[0].Name != "OrphanDetector" {
		t.Fatalf("wrong recovered identity: %+v", report.Survivors[0])
	}
	if len(report.Notes) != 0 {
		t.Fatalf("a recovered module needs no note, got %+v", report.Notes)
	}
}

// TestREQ_6f8284df92a2_UnverifiableModuleReported: when the removed module's
// name is nowhere in the corpus there is no identity to hash candidate
// phrases against, and the group genuinely cannot be checked. Exiting 0 in
// silence is the one unacceptable outcome, so it is disclosed instead.
func TestREQ_6f8284df92a2_UnverifiableModuleReported(t *testing.T) {
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator")

	dir := newRemovalFixture().
		withFile("other/leaf.md", "OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 0 {
		t.Fatalf("nothing can be proven without the module name, got %+v", report.Survivors)
	}
	wantOneNote(t, report.Notes, NoteUnverifiableModule,
		schema.IdentityHash("module", "validator"),
		schema.IdentityHash("validator", "component", "OrphanDetector"),
		"could not be checked")
}

// TestREQ_6f8284df92a2_SweepReachIndependentOfModuleNameSurvival is the
// coupling this check must not have. Two runs of the same module removal, with
// the same surviving mention of the same removed component, differing only in
// whether the module's own name is still written in the remaining prose. The
// corpus-only recovery answered "one survivor" for the first and "I could not
// check" for the second — so the sweep found less the more thoroughly the
// module name had been swept, which inverts the incentive the gate exists to
// create. The bead map decouples them: it recorded the module name while the
// node existed, so both runs now report the same survivor.
func TestREQ_6f8284df92a2_SweepReachIndependentOfModuleNameSurvival(t *testing.T) {
	const survivingMention = "Nothing calls OrphanDetector any more, but its name is still here.\n"
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator") // unresolved: module gone

	tests := []struct{ name, prose string }{
		{"module name survives in prose", "The validator module is gone. " + survivingMention},
		{"module name fully swept", survivingMention},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newRemovalFixture().
				withBeadRecord("validator", "OrphanDetector").
				withFile("other/leaf.md", tt.prose).
				build(t, "other")

			report := mustReport(t, dir, change)
			if len(report.Survivors) != 1 {
				t.Fatalf("want the same 1 survivor either way, got %+v / notes %+v", report.Survivors, report.Notes)
			}
			if report.Survivors[0].Module != "validator" || report.Survivors[0].Name != "OrphanDetector" {
				t.Fatalf("wrong recovered identity: %+v", report.Survivors[0])
			}
			if len(report.Notes) != 0 {
				t.Fatalf("a recovered module needs no note, got %+v", report.Notes)
			}
		})
	}
}

// TestREQ_6f8284df92a2_BeadMapProvesModuleFromSiblingRecord pins the first of
// the two bead-map routes on its own. The removed node has no record of its
// own — a node added and retired between two ingests never got a bead — but a
// sibling component in the same module did, and a module id is
// IdentityHash("module", name), so that sibling's record reproduces the module
// hash and proves the name.
func TestREQ_6f8284df92a2_BeadMapProvesModuleFromSiblingRecord(t *testing.T) {
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator")

	dir := newRemovalFixture().
		withBeadRecord("validator", "SchemaChecker").
		withFile("other/leaf.md", "OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 1 {
		t.Fatalf("want 1 survivor, got %+v / notes %+v", report.Survivors, report.Notes)
	}
	if report.Survivors[0].Module != "validator" || report.Survivors[0].Name != "OrphanDetector" {
		t.Fatalf("wrong recovered identity: %+v", report.Survivors[0])
	}
}

// TestREQ_6f8284df92a2_BeadMapProvesModuleFromNodeRecord pins the second of the
// two bead-map routes. Module ids in project.json are exempt from
// CheckIDDerivation, so a hand-written one derives from nothing and the
// IdentityHash("module", name) lookup cannot match it. A record for the removed
// key still proves the module name, because the key is IdentityHash(module,
// type, component) and the record carries all three.
func TestREQ_6f8284df92a2_BeadMapProvesModuleFromNodeRecord(t *testing.T) {
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = "000000000001" // hand-written module id: derives from nothing

	dir := newRemovalFixture().
		withBeadRecord("validator", "OrphanDetector").
		withFile("other/leaf.md", "OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 1 {
		t.Fatalf("want 1 survivor, got %+v / notes %+v", report.Survivors, report.Notes)
	}
	if report.Survivors[0].Module != "validator" {
		t.Fatalf("want the module name proved from the record, got %+v", report.Survivors[0])
	}
}

// TestREQ_6f8284df92a2_BeadMapRecordsAreProofsNotClaims: a record whose names
// no longer hash into its spec node id is stale, and a stale record must not be
// able to redirect the sweep at a module that never declared the node. It is
// quoted in the note as an unverified lead and nothing more.
func TestREQ_6f8284df92a2_BeadMapRecordsAreProofsNotClaims(t *testing.T) {
	key := schema.IdentityHash("validator", "component", "OrphanDetector")
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator")

	dir := newRemovalFixture().
		withStaleBeadRecord("ghost", "OrphanDetector", key).
		withFile("other/leaf.md", "OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 0 {
		t.Fatalf("a stale record proves nothing, got %+v", report.Survivors)
	}
	wantOneNote(t, report.Notes, NoteUnverifiableModule,
		key, `"OrphanDetector"`, `"ghost"`, "unverified", "could not be checked")
}

// TestREQ_6f8284df92a2_EmptyBeadMapStillNotes: the fallback is a second source
// of proof, not a second reason to fall silent. A map that records nothing
// leaves the group exactly as unverifiable as no map at all.
func TestREQ_6f8284df92a2_EmptyBeadMapStillNotes(t *testing.T) {
	change := removedChange("validator", "component", "OrphanDetector")
	change.Module = schema.IdentityHash("module", "validator")

	dir := newRemovalFixture().
		withEmptyBeadMap().
		withFile("other/leaf.md", "OrphanDetector is still named.\n").
		build(t, "other")

	report := mustReport(t, dir, change)
	if len(report.Survivors) != 0 {
		t.Fatalf("nothing can be proven, got %+v", report.Survivors)
	}
	wantOneNote(t, report.Notes, NoteUnverifiableModule, "could not be checked")
}

// TestREQ_6f8284df92a2_MissingBeadMapIsNotAnError: `spex diff` runs in trees
// that have never been ingested, and in tests whose working directory has no
// map at all.
func TestREQ_6f8284df92a2_MissingBeadMapIsNotAnError(t *testing.T) {
	dir := newRemovalFixture().
		withFile("validator/leaf.md", "OrphanDetector is still named.\n").
		build(t, "validator")

	for _, path := range []string{"", filepath.Join(dir, "nope.json")} {
		report, err := CheckRemovedNames(dir, path, []merkle.ClassifiedChange{
			removedChange("validator", "component", "OrphanDetector"),
		})
		if err != nil {
			t.Fatalf("map path %q: %v", path, err)
		}
		if len(report.Survivors) != 1 {
			t.Fatalf("map path %q: the corpus sweep must still run, got %+v", path, report.Survivors)
		}
	}
}

func TestREQ_6f8284df92a2_NoRemovalsNoFindings(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "DAGChecker").
		withFile("validator/arch_dag_checker.md", "DAGChecker docs.\n").
		build(t)

	modified := merkle.ClassifiedChange{
		Change: merkle.Change{
			Key:      schema.IdentityHash("validator", "component", "DAGChecker"),
			Type:     merkle.Modified,
			NodeType: "component",
		},
		Module: "validator",
	}
	if found := mustCheck(t, dir, modified); len(found) != 0 {
		t.Fatalf("only removals are checked, got %+v", found)
	}
	if found := mustCheck(t, dir); len(found) != 0 {
		t.Fatalf("no changes means no work, got %+v", found)
	}
}

// TestREQ_6f8284df92a2_SelfCheckRealCorpus is the live end-to-end case: the
// working tree really has removed component OrphanDetector relative to the
// snapshot, and its 24 prose mentions really were swept.
func TestREQ_6f8284df92a2_SelfCheckRealCorpus(t *testing.T) {
	specDir := filepath.Join("..", "spec")

	current, err := merkle.BuildTree(specDir)
	if err != nil {
		t.Fatalf("build tree: %v", err)
	}
	snapshot, err := merkle.Load(filepath.Join(specDir, ".snapshot.json"))
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	classified := merkle.Classify(merkle.Diff(current, snapshot), merkle.ModuleNames(current))

	report, err := CheckRemovedNames(specDir, filepath.Join("..", ".bead-map.json"), classified)
	if err != nil {
		t.Fatalf("CheckRemovedNames: %v", err)
	}
	if len(report.Survivors) != 0 {
		t.Fatalf("spex-machina's own corpus should name no removed node, got %+v", report.Survivors)
	}
	for _, n := range report.Notes {
		if n.Kind == NoteUnverifiableModule {
			t.Fatalf("no module has been retired, so nothing should be unverifiable: %+v", n)
		}
	}
}

// TestREQ_6f8284df92a2_LiveNameSuppressionIsDisclosed: the live-name set is
// global, so an identically named node in another module consumes every hit
// and the sweep reports zero. That is the right answer — the text denotes the
// live node — but "zero findings because nothing was found" and "zero
// findings because a live node absorbed them all" must not look the same to
// the author, so the second one says which node did it.
func TestREQ_6f8284df92a2_LiveNameSuppressionIsDisclosed(t *testing.T) {
	dir := newRemovalFixture().
		withComponent("validator", "SchemaChecker").
		withFile("validator/arch_schema_checker.md", "SchemaChecker validates the spec.\n").
		build(t, "merkle")

	report := mustReport(t, dir, removedChange("merkle", "component", "SchemaChecker"))

	if len(report.Survivors) != 0 {
		t.Fatalf("the live component covers every mention, got %+v", report.Survivors)
	}
	wantOneNote(t, report.Notes, NoteSuppressedByLiveName,
		schema.IdentityHash("merkle", "component", "SchemaChecker"),
		`component "SchemaChecker" in module validator`,
		"1 mention(s) not reported",
		"0 site(s) reported")
}

// TestREQ_6f8284df92a2_EqualLengthLiveNameCovers pins the "at least as long"
// half of the subtraction: an identically named live node covers a hit of the
// same width. Narrowing the comparison to strictly longer would report every
// mention of a name that two modules share.
func TestREQ_6f8284df92a2_EqualLengthLiveNameCovers(t *testing.T) {
	dir := newRemovalFixture().
		withAPI("beta", "spex twin").
		withFile("beta/arch_twin.md", "Callers run spex twin every day.\n").
		build(t, "alpha")

	report := mustReport(t, dir, removedChange("alpha", "api", "spex twin"))

	if len(report.Survivors) != 0 {
		t.Fatalf("an equally long live name covers the hit, got %+v", report.Survivors)
	}
	wantOneNote(t, report.Notes, NoteSuppressedByLiveName, `api "spex twin" in module beta`)
}

// TestREQ_6f8284df92a2_OnlyRemovalsAreSearched: an added or modified node is
// still declared, so its name is supposed to be in the corpus. Searching for
// those names would report every live node the diff touched.
func TestREQ_6f8284df92a2_OnlyRemovalsAreSearched(t *testing.T) {
	for _, changeType := range []merkle.ChangeType{merkle.Added, merkle.Modified} {
		t.Run(changeType.String(), func(t *testing.T) {
			dir := newRemovalFixture().
				withComponent("validator", "DAGChecker").
				withFile("validator/arch_dag_checker.md", "DAGChecker walks the graph.\n").
				build(t)

			change := removedChange("validator", "component", "DAGChecker")
			change.Type = changeType

			report := mustReport(t, dir, change)
			if len(report.Survivors) != 0 || len(report.Notes) != 0 {
				t.Fatalf("only removals are searched, got %+v / %+v", report.Survivors, report.Notes)
			}
		})
	}
}

// TestREQ_6f8284df92a2_NameWordBoundIsExact pins maxNameWords from the scan
// side: a name of exactly the bound is found. checkNameRecoverability pins the
// other side by refusing to let a longer one be declared, so the two move
// together or not at all.
func TestREQ_6f8284df92a2_NameWordBoundIsExact(t *testing.T) {
	const atBound = "spex render --format json --slim --bare"
	if got := len(strings.Fields(atBound)); got != maxNameWords {
		t.Fatalf("fixture must be exactly maxNameWords words, got %d", got)
	}

	dir := newRemovalFixture().
		withFile("render/arch_render.md", "Legacy scripts still call "+atBound+" here.\n").
		build(t, "render")

	found := mustCheck(t, dir, removedChange("render", "api", atBound))
	if len(found) != 1 {
		t.Fatalf("a name of exactly maxNameWords words must be reachable, got %+v", found)
	}
}
