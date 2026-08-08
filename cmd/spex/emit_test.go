package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/emit"
	"github.com/dmitriyb/spexmachina/impact"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/schema"
)

// runEmit executes spex emit via cobra and returns stdout, stderr, and the
// command-level error. stdinData feeds the command's stdin (cobra InOrStdin).
func runEmit(t *testing.T, stdinData string, args ...string) (string, string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newEmitCmd())

	in := strings.NewReader(stdinData)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	rootCmd.SetIn(in)
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"emit"}, args...))

	err := rootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// emitFixture is the on-disk state required to drive `spex emit`.
type emitFixture struct {
	specDir    string
	mapPath    string
	impactJSON string
	compID     string
}

// setupEmitFixture writes a minimal but complete spec tree and bead-map to
// a temp dir, plus an impact report containing a single create action for
// a brand-new component. The fixture exercises the new-proposal path
// (synthesized epic op), single tier-1 create, no obsoletes.
func setupEmitFixture(t *testing.T) emitFixture {
	t.Helper()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}

	proj := `{
		"name": "test-emit",
		"modules": [{"id": "` + modID + `", "name": "alpha", "path": "alpha"}]
	}`
	writeTestFile(t, specDir, "project.json", proj)

	mod := `{
		"name": "alpha",
		"components": [{"id": "` + compID + `", "name": "Comp1", "content": "arch_comp1.md"}]
	}`
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", mod)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Comp1\n")

	mapPath := filepath.Join(dir, ".bead-map.json")
	if err := os.WriteFile(mapPath, []byte(`{"next_id":1,"records":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "alpha",
				Node:       "Comp1",
				NodeType:   "component",
				SpecNodeID: compID,
				SpecHash:   "h1",
				Reason:     "New spec node: alpha/Comp1",
			},
		},
		Obsoletes: []impact.Action{},
		Summary:   impact.Summary{CreateCount: 1, ObsoleteCount: 0},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	return emitFixture{
		specDir:    specDir,
		mapPath:    mapPath,
		impactJSON: string(data),
		compID:     compID,
	}
}

func TestEmitCommand_HappyPath_StdinStdout(t *testing.T) {
	f := setupEmitFixture(t)

	stdout, stderr, err := runEmit(t,
		f.impactJSON,
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}

	var cs emit.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}
	if cs.Version != emit.ChangesetVersion {
		t.Errorf("want version %d, got %d", emit.ChangesetVersion, cs.Version)
	}
	if cs.GitHead != "deadbeefcafe" {
		t.Errorf("want git_head deadbeefcafe, got %s", cs.GitHead)
	}
	if cs.Proposal != "2026-04-18-decouple-spex-from-br" {
		t.Errorf("want proposal ref, got %s", cs.Proposal)
	}
	if len(cs.Ops) < 2 {
		t.Fatalf("want at least 2 ops (epic + comp create), got %d", len(cs.Ops))
	}
	// First op must be the synthesized proposal_epic create.
	if cs.Ops[0].Type != emit.OpCreate || cs.Ops[0].SpecNodeKind != "proposal_epic" {
		t.Errorf("want first op to be proposal_epic create, got %+v", cs.Ops[0])
	}
}

func TestEmitCommand_ImpactFlag_ReadsFile(t *testing.T) {
	f := setupEmitFixture(t)

	impactPath := filepath.Join(t.TempDir(), "impact.json")
	if err := os.WriteFile(impactPath, []byte(f.impactJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runEmit(t, "",
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--impact", impactPath,
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, `"version": 2`) {
		t.Fatalf("expected v2 changeset on stdout, got: %s", stdout)
	}
}

func TestEmitCommand_OutFlag_WritesFileEmptyStdout(t *testing.T) {
	f := setupEmitFixture(t)

	impactPath := filepath.Join(t.TempDir(), "impact.json")
	if err := os.WriteFile(impactPath, []byte(f.impactJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "changeset.json")

	stdout, stderr, err := runEmit(t, "",
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--impact", impactPath,
		"--out", outPath,
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("want empty stdout when --out is set, got: %s", stdout)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read --out file: %v", err)
	}
	var cs emit.Changeset
	if err := json.Unmarshal(data, &cs); err != nil {
		t.Fatalf("invalid changeset in --out file: %v\ndata: %s", err, data)
	}
	if cs.Version != emit.ChangesetVersion {
		t.Errorf("want version %d, got %d", emit.ChangesetVersion, cs.Version)
	}

	// Compare against the stdout form: must be byte-identical.
	stdoutForm, _, err := runEmit(t, "",
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--impact", impactPath,
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("stdout-form emit failed: %v", err)
	}
	if string(data) != stdoutForm {
		t.Fatalf("--out file does not match stdout form\nfile: %s\nstdout: %s", data, stdoutForm)
	}
}

func TestEmitCommand_MissingGitHead_Errors(t *testing.T) {
	f := setupEmitFixture(t)
	_, stderr, err := runEmit(t, f.impactJSON,
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for missing --git-head, got nil")
	}
	if !strings.Contains(err.Error()+stderr, "git-head") {
		t.Fatalf("want error mentioning git-head, got err=%v stderr=%s", err, stderr)
	}
}

func TestEmitCommand_MissingProposal_Errors(t *testing.T) {
	f := setupEmitFixture(t)
	_, stderr, err := runEmit(t, f.impactJSON,
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for missing --proposal, got nil")
	}
	if !strings.Contains(err.Error()+stderr, "proposal") {
		t.Fatalf("want error mentioning proposal, got err=%v stderr=%s", err, stderr)
	}
}

func TestEmitCommand_MalformedImpactJSON_Errors(t *testing.T) {
	f := setupEmitFixture(t)
	_, _, err := runEmit(t, "{not json",
		"--proposal", "p",
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed impact JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode impact") {
		t.Fatalf("want decode-impact error, got: %v", err)
	}
}

func TestEmitCommand_ImpactErrorsArray_RefusesToProceed(t *testing.T) {
	f := setupEmitFixture(t)

	// Wrap the report with an errors array. Emit must reject.
	withErrors := map[string]any{
		"creates":   []any{},
		"obsoletes": []any{},
		"summary":   map[string]int{"create_count": 0, "obsolete_count": 0},
		"errors": []map[string]any{
			{"type": "incomplete_change", "message": "upstream gate caught me"},
		},
	}
	data, err := json.Marshal(withErrors)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runEmit(t, string(data),
		"--proposal", "p",
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error when impact report carries errors, got nil")
	}
	if !strings.Contains(err.Error(), "errors") {
		t.Fatalf("want error mentioning errors, got: %v", err)
	}
}

func TestEmitCommand_HelpExits0(t *testing.T) {
	stdout, _, err := runEmit(t, "", "--help")
	if err != nil {
		t.Fatalf("--help should succeed, got: %v", err)
	}
	for _, flag := range []string{"--proposal", "--git-head", "--impact", "--out"} {
		if !strings.Contains(stdout, flag) {
			t.Errorf("want help output to mention %s, got:\n%s", flag, stdout)
		}
	}
}

func TestEmitCommand_Determinism(t *testing.T) {
	f := setupEmitFixture(t)

	out1, _, err := runEmit(t, f.impactJSON,
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("first emit failed: %v", err)
	}
	out2, _, err := runEmit(t, f.impactJSON,
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("second emit failed: %v", err)
	}
	if out1 != out2 {
		t.Fatalf("determinism: outputs differ\nrun1: %s\nrun2: %s", out1, out2)
	}
}

func TestEmitCommand_CycleInBatchDeps_Errors(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	aID := schema.IdentityHash("alpha", "component", "A")
	bID := schema.IdentityHash("alpha", "component", "B")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "cycle-test",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+aID+`", "name": "A", "content": "arch_a.md"},
			{"id": "`+bID+`", "name": "B", "content": "arch_b.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_a.md", "# A\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_b.md", "# B\n")

	mapPath := filepath.Join(dir, ".bead-map.json")
	if err := os.WriteFile(mapPath, []byte(`{"next_id":1,"records":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{Type: "create", Module: "alpha", Node: "A", NodeType: "component", SpecNodeID: aID, DepSpecNodeIDs: []string{bID}},
			{Type: "create", Module: "alpha", Node: "B", NodeType: "component", SpecNodeID: bID, DepSpecNodeIDs: []string{aID}},
		},
		Summary: impact.Summary{CreateCount: 2},
	}
	data, _ := json.Marshal(report)

	outPath := filepath.Join(t.TempDir(), "changeset.json")
	_, _, err := runEmit(t, string(data),
		"--proposal", "p",
		"--git-head", "deadbeefcafe",
		"--map", mapPath,
		"--spec-dir", specDir,
		"--out", outPath,
	)
	if err == nil {
		t.Fatal("want error for in-batch cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("want error mentioning cycle, got: %v", err)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Fatalf("want no --out file written when build fails, but %s exists", outPath)
	}
}

func TestEmitCommand_BadGitHead_Errors(t *testing.T) {
	f := setupEmitFixture(t)
	_, _, err := runEmit(t, f.impactJSON,
		"--proposal", "p",
		"--git-head", "not-a-sha",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed --git-head, got nil")
	}
	if !strings.Contains(err.Error(), "git-head") {
		t.Fatalf("want error mentioning git-head, got: %v", err)
	}
}

// TestEmitCommand_ClosedProposalEpicRecord_SynthesizesFreshEpicParent is an
// end-to-end regression test for the PR #217 closed-epic-record path: with a
// closed NodeType=="proposal" record for the proposal slug in the map file,
// `spex emit` still synthesizes a fresh proposal_epic op that every other
// create parents under, rather than wiring them to the closed epic's dead
// bead_id. It runs through the whole command (Resolver, Builder,
// legacyMapStoreFold together) and does not by itself pin any single rule
// inside legacyMapStoreFold.Entry — see TestLegacyMapStoreFold_Entry for
// unit coverage of the bridge's NodeType != "proposal" filter and the other
// three rules (highest-ID tiebreak, closed-skip, proposal-epic-first
// lookup).
func TestEmitCommand_ClosedProposalEpicRecord_SynthesizesFreshEpicParent(t *testing.T) {
	f := setupEmitFixture(t)
	proposal := "2026-04-18-decouple-spex-from-br"

	closedEpicMap := `{"next_id":2,"records":[{
		"id": 1,
		"spec_node_id": "` + proposal + `",
		"bead_id": "br-1",
		"bead_type": "epic",
		"node_type": "proposal",
		"module": "",
		"component": "` + proposal + `",
		"content_file": "",
		"spec_hash": "",
		"bead_status": "closed"
	}]}`
	if err := os.WriteFile(f.mapPath, []byte(closedEpicMap), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runEmit(t,
		f.impactJSON,
		"--proposal", proposal,
		"--git-head", "deadbeefcafe",
		"--map", f.mapPath,
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}

	var cs emit.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}

	var epic *emit.Op
	for i := range cs.Ops {
		if cs.Ops[i].SpecNodeKind == "proposal_epic" {
			epic = &cs.Ops[i]
			break
		}
	}
	if epic == nil {
		t.Fatalf("want a synthesized proposal_epic create op despite the closed record, got ops: %+v", cs.Ops)
	}

	var compOp *emit.Op
	for i := range cs.Ops {
		if cs.Ops[i].SpecNodeKind == "component" {
			compOp = &cs.Ops[i]
			break
		}
	}
	if compOp == nil {
		t.Fatalf("want a component create op, got ops: %+v", cs.Ops)
	}
	if compOp.Parent == nil || compOp.Parent.Kind != emit.RefOp || compOp.Parent.OpID != epic.OpID {
		t.Errorf("component parent: want ref:op %s (fresh epic), got %+v", epic.OpID, compOp.Parent)
	}
}

// writeLegacyMapFile writes a .bead-map.json fixture containing exactly the
// given records and returns its path.
func writeLegacyMapFile(t *testing.T, records []mapping.Record) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".bead-map.json")
	data, err := json.Marshal(struct {
		NextID  int              `json:"next_id"`
		Records []mapping.Record `json:"records"`
	}{NextID: len(records) + 1, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLegacyMapStoreFold_Entry pins each of legacyMapStoreFold.Entry's four
// bridge rules (cmd/spex/emit.go) directly against mapping.Store, rather
// than through emit.Build — see the comment on
// TestEmitCommand_ClosedProposalEpicRecord_SynthesizesFreshEpicParent for
// why routing through Build washes out the NodeType != "proposal" filter.
func TestLegacyMapStoreFold_Entry(t *testing.T) {
	const slug = "2026-04-18-decouple-spex-from-br"
	const key = "abc123def456"

	tests := []struct {
		name      string
		key       string
		records   []mapping.Record
		wantEntry emit.FoldEntry
		wantOK    bool
	}{
		{
			name: "open proposal record for the slug resolves via GetByProposalEpic",
			key:  slug,
			records: []mapping.Record{
				{ID: 1, SpecNodeID: slug, BeadID: "br-1", BeadType: "epic", NodeType: "proposal", Component: slug, BeadStatus: "open"},
			},
			wantEntry: emit.FoldEntry{TaskID: "br-1"},
			wantOK:    true,
		},
		{
			// GetByProposalEpic skips the closed record (not found), and the
			// NodeType != "proposal" filter then excludes it from the
			// GetBySpecNode fallback too, leaving no record at all: ok ==
			// false. Without the filter the closed proposal record would be
			// admitted into pickOpenMapRecord, collapse to Removed: true,
			// and return ok == true instead — this case is what
			// distinguishes the two.
			name: "closed proposal record for the slug is not re-admitted as a live epic",
			key:  slug,
			records: []mapping.Record{
				{ID: 1, SpecNodeID: slug, BeadID: "br-1", BeadType: "epic", NodeType: "proposal", Component: slug, BeadStatus: "closed"},
			},
			wantEntry: emit.FoldEntry{},
			wantOK:    false,
		},
		{
			name: "two open non-proposal records for one key: higher ID wins",
			key:  key,
			records: []mapping.Record{
				{ID: 1, SpecNodeID: key, BeadID: "br-1", BeadType: "task", NodeType: "component", Module: "m", Component: "c1", ContentFile: "c1.md", SpecHash: "h1", BeadStatus: "open"},
				{ID: 2, SpecNodeID: key, BeadID: "br-2", BeadType: "task", NodeType: "component", Module: "m", Component: "c2", ContentFile: "c2.md", SpecHash: "h2", BeadStatus: "open"},
			},
			wantEntry: emit.FoldEntry{TaskID: "br-2"},
			wantOK:    true,
		},
		{
			name: "all non-proposal records for one key closed collapses onto Removed",
			key:  key,
			records: []mapping.Record{
				{ID: 1, SpecNodeID: key, BeadID: "br-1", BeadType: "task", NodeType: "component", Module: "m", Component: "c1", ContentFile: "c1.md", SpecHash: "h1", BeadStatus: "closed"},
				{ID: 2, SpecNodeID: key, BeadID: "br-2", BeadType: "task", NodeType: "component", Module: "m", Component: "c2", ContentFile: "c2.md", SpecHash: "h2", BeadStatus: "closed"},
			},
			wantEntry: emit.FoldEntry{Removed: true},
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeLegacyMapFile(t, tt.records)
			fold := legacyMapStoreFold{store: mapping.NewFileStore(path)}

			gotEntry, gotOK := fold.Entry(tt.key)
			if gotOK != tt.wantOK {
				t.Fatalf("Entry(%q) ok = %v, want %v (entry: %+v)", tt.key, gotOK, tt.wantOK, gotEntry)
			}
			if gotOK && gotEntry != tt.wantEntry {
				t.Fatalf("Entry(%q) = %+v, want %+v", tt.key, gotEntry, tt.wantEntry)
			}
		})
	}
}

// exitCodeOf extracts the process exit code an error carries via the
// ExitCode interface main.go honors. Zero means no code attached.
func exitCodeOf(err error) int {
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 0
}

// TestEmitCommand_ValidationErrorsExit1 pins the arch_emit_command.md exit
// code contract: input validation failures (bad --git-head, malformed
// impact JSON, impact errors[] gate) carry exit code 1.
func TestEmitCommand_ValidationErrorsExit1(t *testing.T) {
	f := setupEmitFixture(t)

	cases := []struct {
		name  string
		stdin string
		head  string
	}{
		{"bad git-head", f.impactJSON, "not-a-sha"},
		{"malformed impact JSON", "{not json", "deadbeefcafe"},
		{"impact errors gate", `{"creates":[],"obsoletes":[],"errors":[{"type":"x"}]}`, "deadbeefcafe"},
	}
	for _, tc := range cases {
		_, _, err := runEmit(t, tc.stdin,
			"--proposal", "p",
			"--git-head", tc.head,
			"--map", f.mapPath,
			"--spec-dir", f.specDir,
		)
		if err == nil {
			t.Fatalf("%s: want error, got nil", tc.name)
		}
		if code := exitCodeOf(err); code != 1 {
			t.Errorf("%s: want exit code 1, got %d (err: %v)", tc.name, code, err)
		}
	}
}

// TestEmitCommand_BuilderErrorsExit2 pins the other half of the exit code
// contract: builder failures (in-batch dep cycle) carry exit code 2.
func TestEmitCommand_BuilderErrorsExit2(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	aID := schema.IdentityHash("alpha", "component", "A")
	bID := schema.IdentityHash("alpha", "component", "B")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "cycle-test",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+aID+`", "name": "A", "content": "arch_a.md"},
			{"id": "`+bID+`", "name": "B", "content": "arch_b.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_a.md", "# A\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_b.md", "# B\n")

	mapPath := filepath.Join(dir, ".bead-map.json")
	if err := os.WriteFile(mapPath, []byte(`{"next_id":1,"records":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{Type: "create", Module: "alpha", Node: "A", NodeType: "component", SpecNodeID: aID, DepSpecNodeIDs: []string{bID}},
			{Type: "create", Module: "alpha", Node: "B", NodeType: "component", SpecNodeID: bID, DepSpecNodeIDs: []string{aID}},
		},
		Summary: impact.Summary{CreateCount: 2},
	}
	data, _ := json.Marshal(report)

	_, _, err := runEmit(t, string(data),
		"--proposal", "p",
		"--git-head", "deadbeefcafe",
		"--map", mapPath,
		"--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want builder error for in-batch cycle, got nil")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit code 2 for builder error, got %d (err: %v)", code, err)
	}
}
