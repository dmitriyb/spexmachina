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
	impactJSON string
	compID     string
}

// setupEmitFixture writes a minimal but complete spec tree and a seeded
// spec/.history.jsonl journal to a temp dir, plus an impact report
// containing a single create action for a brand-new component. The journal
// carries an unrelated already-paired node so the fixture exercises a real
// fold read without affecting the new-proposal path (synthesized epic op,
// single tier-1 create, no obsoletes) the happy-path scenarios assert on.
// It also carries a registered event for the fixture's own proposal
// (2026-04-18-decouple-spex-from-br) — without one, EmitCommand's
// registration read finds nothing and the builder refuses to synthesize
// an epic.
func setupEmitFixture(t *testing.T) emitFixture {
	t.Helper()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")
	otherID := schema.IdentityHash("alpha", "component", "Other")

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

	writeTestJournal(t, specDir, []string{
		`{"event":"added","eid":"e-other","node":"` + otherID + `","name":"Other","node_type":"component","module":"alpha","before":null,"after":"h-other","git_head":"cafe0000","proposal":"earlier-proposal"}`,
		`{"event":"task_created","for":"e-other","task_id":"spex-other"}`,
		`{"event":"registered","eid":"cafe0000:2026-04-18-decouple-spex-from-br","proposal":"2026-04-18-decouple-spex-from-br","git_head":"cafe0000"}`,
	})

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
		"--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("first emit failed: %v", err)
	}
	out2, _, err := runEmit(t, f.impactJSON,
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
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
		"--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed --git-head, got nil")
	}
	if !strings.Contains(err.Error(), "git-head") {
		t.Fatalf("want error mentioning git-head, got: %v", err)
	}
}

// TestEmitCommand_ExistingEpic_ParentsUnderExistingBead pins the journal
// read as load-bearing: a pre-existing task_created receipt keyed by the
// proposal slug makes Builder.hasExistingEpic true, so no proposal_epic
// op is synthesized and the lone create parents directly under the
// journal's bead. Mutating the wiring to discard the fold would leave
// this test asserting a synthesized epic parent instead, and fail.
func TestEmitCommand_ExistingEpic_ParentsUnderExistingBead(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-emit",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [{"id": "`+compID+`", "name": "Comp1", "content": "arch_comp1.md"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Comp1\n")

	writeTestJournal(t, specDir, []string{
		`{"event":"task_created","proposal":"2026-04-18-decouple-spex-from-br","task_id":"br-epic"}`,
	})

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{Type: "create", Module: "alpha", Node: "Comp1", NodeType: "component", SpecNodeID: compID, SpecHash: "h1", Reason: "New spec node: alpha/Comp1"},
		},
		Obsoletes: []impact.Action{},
		Summary:   impact.Summary{CreateCount: 1, ObsoleteCount: 0},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runEmit(t, string(data),
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}

	var cs emit.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}
	if len(cs.Ops) != 1 {
		t.Fatalf("want exactly 1 op (existing epic found in journal, no synthesized proposal_epic), got %d: %+v", len(cs.Ops), cs.Ops)
	}
	op := cs.Ops[0]
	if op.SpecNodeKind == "proposal_epic" {
		t.Fatalf("want no synthesized proposal_epic when journal already carries a live epic, got %+v", op)
	}
	if op.Parent == nil || op.Parent.Kind != emit.RefBead || op.Parent.BeadID != "br-epic" {
		t.Fatalf("want parent ref:bead br-epic (from journal), got %+v", op.Parent)
	}
}

// TestEmitCommand_OutOfBatchDep_ResolvesToRefBead pins the journal read's
// second load-bearing role: a dep naming a spec node that is not in this
// batch, but is paired to a live task in the journal fold, must resolve
// to ref:bead rather than erroring or being dropped. Discarding the fold
// would turn this dep unresolvable and fail the build outright.
func TestEmitCommand_OutOfBatchDep_ResolvesToRefBead(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")
	otherID := schema.IdentityHash("alpha", "component", "Other")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-emit",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [{"id": "`+compID+`", "name": "Comp1", "content": "arch_comp1.md"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Comp1\n")

	writeTestJournal(t, specDir, []string{
		`{"event":"added","eid":"e-other","node":"` + otherID + `","name":"Other","node_type":"component","module":"alpha","before":null,"after":"h-other","git_head":"cafe0000","proposal":"earlier-proposal"}`,
		`{"event":"task_created","for":"e-other","task_id":"spex-other"}`,
		`{"event":"registered","eid":"cafe0000:2026-04-18-decouple-spex-from-br","proposal":"2026-04-18-decouple-spex-from-br","git_head":"cafe0000"}`,
	})

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:           "create",
				Module:         "alpha",
				Node:           "Comp1",
				NodeType:       "component",
				SpecNodeID:     compID,
				SpecHash:       "h1",
				DepSpecNodeIDs: []string{otherID},
				Reason:         "New spec node: alpha/Comp1",
			},
		},
		Obsoletes: []impact.Action{},
		Summary:   impact.Summary{CreateCount: 1, ObsoleteCount: 0},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runEmit(t, string(data),
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefcafe",
		"--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}

	var cs emit.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}

	var compOp *emit.Op
	for i := range cs.Ops {
		if cs.Ops[i].SpecNodeID == compID {
			compOp = &cs.Ops[i]
		}
	}
	if compOp == nil {
		t.Fatalf("want an op for Comp1, got ops: %+v", cs.Ops)
	}
	if len(compOp.Deps) != 1 || compOp.Deps[0].Kind != emit.RefBead || compOp.Deps[0].BeadID != "spex-other" {
		t.Fatalf("want dep ref:bead spex-other (out-of-batch, journal-paired), got %+v", compOp.Deps)
	}
}

// TestEmitCommand_CleanupPriorBatchRemoval_LabelsFromRealJournalFold pins
// the production journalFold adapter (newJournalFold's Entry method): a
// cleanup create answering a removal that already landed in an earlier
// batch must label off the fold's real removed-event eid, not a same-batch
// close op. emit's own package tests exercise this branch through a fake
// fold that seeds RemovedEID directly — only this adapter proves
// mapping.FoldEntry.Source.EID actually reaches emit.FoldEntry.RemovedEID
// in the shipped binary.
func TestEmitCommand_CleanupPriorBatchRemoval_LabelsFromRealJournalFold(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	removedID := schema.IdentityHash("alpha", "component", "Removed")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-emit",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": []
	}`)

	writeTestJournal(t, specDir, []string{
		`{"event":"removed","eid":"e-removed","node":"` + removedID + `","name":"Removed","node_type":"component","module":"alpha","before":"h-old","after":null,"git_head":"cafe0000","proposal":"earlier-proposal"}`,
		`{"event":"registered","eid":"cafe0000:2026-04-18-decouple-spex-from-br","proposal":"2026-04-18-decouple-spex-from-br","git_head":"cafe0000"}`,
	})

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{
				Type:       "create",
				Module:     "alpha",
				Node:       "Removed",
				NodeType:   "component",
				SpecNodeID: removedID,
				OldBeadID:  "spexmachina-old",
				Reason:     "Code cleanup: alpha/Removed",
			},
		},
		Obsoletes: []impact.Action{},
		Summary:   impact.Summary{CreateCount: 1, ObsoleteCount: 0},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runEmit(t, string(data),
		"--proposal", "2026-04-18-decouple-spex-from-br",
		"--git-head", "deadbeefface",
		"--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("emit failed: %v\nstderr: %s", err, stderr)
	}

	var cs emit.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}

	var cleanupOp *emit.Op
	for i := range cs.Ops {
		if cs.Ops[i].SpecNodeID == removedID {
			cleanupOp = &cs.Ops[i]
		}
	}
	if cleanupOp == nil {
		t.Fatalf("want a cleanup op for the removed node, got ops: %+v", cs.Ops)
	}
	if cleanupOp.Idempotency == nil || cleanupOp.Idempotency.Label != "spex:e-removed" {
		t.Errorf("idempotency.label: want spex:e-removed (real journal fold's removed event, not a same-batch close), got %+v", cleanupOp.Idempotency)
	}
}

// TestEmitCommand_MalformedJournal_Errors rounds out the exit-code
// contract for the emit.go:62 journal read: a journal line that fails
// schema validation must surface as a validation error (exit 1), same
// as the other read failures in this file.
func TestEmitCommand_MalformedJournal_Errors(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	compID := schema.IdentityHash("alpha", "component", "Comp1")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-emit",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [{"id": "`+compID+`", "name": "Comp1", "content": "arch_comp1.md"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_comp1.md", "# Comp1\n")
	writeTestJournal(t, specDir, []string{"not-json"})

	report := impact.ImpactReport{
		Creates: []impact.Action{
			{Type: "create", Module: "alpha", Node: "Comp1", NodeType: "component", SpecNodeID: compID, SpecHash: "h1", Reason: "New spec node: alpha/Comp1"},
		},
		Summary: impact.Summary{CreateCount: 1},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = runEmit(t, string(data),
		"--proposal", "p",
		"--git-head", "deadbeefcafe",
		"--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed journal, got nil")
	}
	if !strings.Contains(err.Error(), "read journal") {
		t.Fatalf("want error mentioning 'read journal', got: %v", err)
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit code 1 for malformed journal, got %d (err: %v)", code, err)
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
		"--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want builder error for in-batch cycle, got nil")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit code 2 for builder error, got %d (err: %v)", code, err)
	}
}
