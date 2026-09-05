package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/internal/perf"
	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/plan"
	"github.com/dmitriyb/spexmachina/schema"
)

// runPlan executes spex plan via cobra and returns stdout, stderr, and the
// command-level error. stdinData feeds the command's stdin.
func runPlan(t *testing.T, stdinData string, args ...string) (string, string, error) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newPlanCmd())

	in := strings.NewReader(stdinData)
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	rootCmd.SetIn(in)
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"plan"}, args...))

	err := rootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// seedPlanSnapshot writes an empty-tree snapshot at dir's resolved .spex/
// location, marking dir's project as initialised for the lifecycle
// pre-flight plan.go's command runs — same as cmd/spex/diff.go's and
// cmd/spex/map.go's own tests seed before exercising a command that
// expects to run past the uninitialised-project refusal.
func seedPlanSnapshot(t *testing.T, dir string) {
	t.Helper()
	stateDir := projectStateDir(dir)
	if err := merkle.Save(merkle.EmptyTree(), filepath.Join(stateDir, lifecycle.SnapshotFileName), time.Now()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
}

func parsePlanChangeset(t *testing.T, stdout string) plan.Changeset {
	t.Helper()
	var cs plan.Changeset
	if err := json.Unmarshal([]byte(stdout), &cs); err != nil {
		t.Fatalf("invalid changeset JSON: %v\nstdout: %s", err, stdout)
	}
	return cs
}

func writePlanDiff(t *testing.T, dir, name string, changes []diffChange, errs []merkle.DiffError) string {
	t.Helper()
	if changes == nil {
		changes = []diffChange{}
	}
	if errs == nil {
		errs = []merkle.DiffError{}
	}
	doc := struct {
		Changes []diffChange       `json:"changes"`
		Errors  []merkle.DiffError `json:"errors"`
	}{Changes: changes, Errors: errs}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writePlanTasks writes a version-1 task-state artifact (schema/task-state.
// schema.json) listing one entry per statuses map key, the shape --tasks
// expects (spec/plan/test_plan_command.md, Setup item 4).
func writePlanTasks(t *testing.T, dir, name string, statuses map[string]string) string {
	t.Helper()
	type taskEntry struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
	}
	tasks := make([]taskEntry, 0, len(statuses))
	for id, status := range statuses {
		tasks = append(tasks, taskEntry{TaskID: id, Status: status})
	}
	data, err := json.Marshal(struct {
		Version int         `json:"version"`
		Tasks   []taskEntry `json:"tasks"`
	}{Version: 1, Tasks: tasks})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePlanAbsorb(t *testing.T, dir, name string, entries []map[string]string) string {
	t.Helper()
	type absorbEntry struct {
		Node   string `json:"node"`
		Reason string `json:"reason"`
	}
	list := make([]absorbEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, absorbEntry{Node: e["node"], Reason: e["reason"]})
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// setupMinimalPlanSpec writes a spec tree with one empty module — enough
// structure for scenarios that don't exercise ActionClassifier's node
// lookups (an empty diff, a diff carrying only errors).
func setupMinimalPlanSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"m","modules":[{"id":"`+modID+`","name":"alpha","path":"alpha"}]}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{"name":"alpha"}`)
	seedPlanSnapshot(t, specDir)
	return specDir
}

// planFixture is the shared on-disk state for the Plan command tests
// (spec/plan/test_plan_command.md, "Setup"): a spec tree with three current
// components (Existing, Existing2, New) plus a fourth (Removed) that only
// survives in the journal, a journal seeding one added event + task_created
// per tracked node and a registered event for the fixture's proposal, a
// diff reporting Existing/Existing2 modified, New added and Removed
// removed, and task-state artifact variants (--tasks) driving the cleanup
// gate and the claimed-task refusal: all-open, one-unlisted (task-removed
// absent), one-in_progress and empty.
type planFixture struct {
	specDir     string
	proposal    string
	gitHead     string
	existingID  string
	existing2ID string
	newID       string
	removedID   string
	diffPath    string

	tasksAllOpen           string
	tasksOneUnlisted       string
	tasksOneInProgress     string
	tasksRemovedInProgress string
	tasksEmpty             string
}

func setupPlanFixture(t *testing.T) planFixture {
	t.Helper()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	modID := schema.IdentityHash("module", "alpha")
	existingID := schema.IdentityHash("alpha", "component", "Existing")
	existing2ID := schema.IdentityHash("alpha", "component", "Existing2")
	newID := schema.IdentityHash("alpha", "component", "New")
	removedID := schema.IdentityHash("alpha", "component", "Removed")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{
		"name": "test-plan",
		"modules": [{"id": "`+modID+`", "name": "alpha", "path": "alpha"}]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+existingID+`", "name": "Existing", "content": "arch_existing.md"},
			{"id": "`+existing2ID+`", "name": "Existing2", "content": "arch_existing2.md"},
			{"id": "`+newID+`", "name": "New", "content": "arch_new.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_existing.md", "# Existing\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_existing2.md", "# Existing2\n")
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_new.md", "# New\n")
	seedPlanSnapshot(t, specDir)

	proposal := "2026-08-14-plan-fixture"
	gitHead := "deadbeefcafe"

	writeTestJournal(t, specDir, []string{
		`{"event":"added","eid":"e-existing","node":"` + existingID + `","name":"Existing","node_type":"component","module":"alpha","before":null,"after":"h0","git_head":"cafe0000","proposal":"` + proposal + `"}`,
		`{"event":"task_created","for":"e-existing","task_id":"task-existing"}`,
		`{"event":"added","eid":"e-existing2","node":"` + existing2ID + `","name":"Existing2","node_type":"component","module":"alpha","before":null,"after":"h0b","git_head":"cafe0000","proposal":"` + proposal + `"}`,
		`{"event":"task_created","for":"e-existing2","task_id":"task-existing2"}`,
		`{"event":"added","eid":"e-removed","node":"` + removedID + `","name":"Removed","node_type":"component","module":"alpha","before":null,"after":"h-removed","git_head":"cafe0000","proposal":"` + proposal + `"}`,
		`{"event":"task_created","for":"e-removed","task_id":"task-removed"}`,
		`{"event":"registered","eid":"cafe0000:` + proposal + `","proposal":"` + proposal + `","git_head":"cafe0000"}`,
	})

	changes := []diffChange{
		{Path: existingID, Type: "modified", Impact: "arch_impl", Module: "alpha", NodeType: "component", OldHash: "h0", NewHash: "h1"},
		{Path: existing2ID, Type: "modified", Impact: "arch_impl", Module: "alpha", NodeType: "component", OldHash: "h0b", NewHash: "h1b"},
		{Path: newID, Type: "added", Impact: "arch_impl", Module: "alpha", NodeType: "component", NewHash: "h-new"},
		{Path: removedID, Type: "removed", Impact: "arch_impl", Module: "alpha", NodeType: "component", OldHash: "h-removed"},
	}
	diffPath := writePlanDiff(t, dir, "diff.json", changes, nil)

	tasksAllOpen := writePlanTasks(t, dir, "tasks-all-open.json", map[string]string{
		"task-existing": "open", "task-existing2": "open", "task-removed": "open",
	})
	// one-unlisted: task-removed is absent from the artifact — the fold's
	// pairing for the removed node then joins no status, which the classifier
	// reads as "finished", driving the cleanup gate rather than a close.
	tasksOneUnlisted := writePlanTasks(t, dir, "tasks-one-unlisted.json", map[string]string{
		"task-existing": "open", "task-existing2": "open",
	})
	tasksOneInProgress := writePlanTasks(t, dir, "tasks-one-in-progress.json", map[string]string{
		"task-existing": "in_progress", "task-existing2": "in_progress", "task-removed": "open",
	})
	// removed-in_progress: the removed node's own task is claimed while
	// existing/existing2 stay open — isolates classifyOrphaned's
	// in_progress branch (plan/action_classifier.go:168-169) from the
	// modified-node refusal S7's first arm already covers.
	tasksRemovedInProgress := writePlanTasks(t, dir, "tasks-removed-in-progress.json", map[string]string{
		"task-existing": "open", "task-existing2": "open", "task-removed": "in_progress",
	})
	tasksEmpty := writePlanTasks(t, dir, "tasks-empty.json", map[string]string{})

	return planFixture{
		specDir: specDir, proposal: proposal, gitHead: gitHead,
		existingID: existingID, existing2ID: existing2ID, newID: newID, removedID: removedID,
		diffPath:               diffPath,
		tasksAllOpen:           tasksAllOpen,
		tasksOneUnlisted:       tasksOneUnlisted,
		tasksOneInProgress:     tasksOneInProgress,
		tasksRemovedInProgress: tasksRemovedInProgress,
		tasksEmpty:             tasksEmpty,
	}
}

// --- S1: Full pipeline — diff file to changeset on stdout ---

func TestPlanCommand_S1_FullPipeline_DiffFileToChangesetStdout(t *testing.T) {
	f := setupPlanFixture(t)
	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("plan failed: %v\nstderr: %s", err, stderr)
	}

	cs := parsePlanChangeset(t, stdout)
	if cs.Version != plan.ChangesetVersion {
		t.Errorf("version: want %d, got %d", plan.ChangesetVersion, cs.Version)
	}
	if cs.GitHead != f.gitHead {
		t.Errorf("git_head: want %s, got %s", f.gitHead, cs.GitHead)
	}
	if cs.Proposal != f.proposal {
		t.Errorf("proposal: want %s, got %s", f.proposal, cs.Proposal)
	}
	if len(cs.Ops) == 0 {
		t.Fatal("want a non-empty ops array")
	}
	if cs.Ops[0].SpecNodeKind != plan.KindProposalEpic {
		t.Errorf("want the epic first, got %+v", cs.Ops[0])
	}

	// The one-unlisted task-state variant: task-existing/task-existing2 are
	// open (retargeted in place — spexmachina-swvx.16 retired the
	// close-and-recreate step) and task-removed is absent from the artifact,
	// which mints a cleanup create rather than a close
	// (spec/plan/test_classification.md S4) — so this fixture produces no
	// close op at all.
	lastCreate, firstRetarget := -1, -1
	for i, op := range cs.Ops {
		if op.Type == plan.OpCreate {
			lastCreate = i
		}
		if op.Type == plan.OpRetarget && firstRetarget == -1 {
			firstRetarget = i
		}
	}
	if firstRetarget == -1 {
		t.Fatal("want at least one retarget op (the one-unlisted task-state variant retargets the open existing/existing2 pairings)")
	}
	if lastCreate > firstRetarget {
		t.Errorf("want every create before every retarget, got ops: %+v", cs.Ops)
	}
}

// --- S2: Diff input from stdin (pipe), and --diff - ---

func TestPlanCommand_S2_DiffFromStdinAndDashFlag(t *testing.T) {
	f := setupPlanFixture(t)
	diffData, err := os.ReadFile(f.diffPath)
	if err != nil {
		t.Fatal(err)
	}

	want, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("file form: %v\n%s", err, stderr)
	}

	stdinOut, stderr, err := runPlan(t, string(diffData),
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("stdin form: %v\n%s", err, stderr)
	}
	if stdinOut != want {
		t.Errorf("stdin form differs from file form\nfile:  %s\nstdin: %s", want, stdinOut)
	}

	dashOut, stderr, err := runPlan(t, string(diffData),
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", "-", "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("--diff - form: %v\n%s", err, stderr)
	}
	if dashOut != want {
		t.Errorf("--diff - form differs from file form\nfile: %s\n-:    %s", want, dashOut)
	}
}

// --- S3: Pipeline composition — spex diff piped into spex plan ---

func TestPlanCommand_S3_PipelineComposition_DiffIntoPlan(t *testing.T) {
	dir := setupTestSpec(t)
	seedProjectState(t, dir, merkle.EmptyTree(), time.Now())

	tree, err := merkle.BuildTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "custom-snapshot.json")
	if err := merkle.Save(tree, snapshotPath, time.Now()); err != nil {
		t.Fatal(err)
	}

	comp1ID := schema.IdentityHash("alpha", "component", "Comp1")
	newID := schema.IdentityHash("alpha", "component", "New")
	test1ID := schema.IdentityHash("alpha", "test_section", "Comp1 tests")
	writeTestFile(t, filepath.Join(dir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+comp1ID+`", "name": "Comp1", "content": "arch_comp1.md"},
			{"id": "`+newID+`", "name": "New", "content": "arch_new.md"}
		],
		"test_sections": [
			{"id": "`+test1ID+`", "name": "Comp1 tests", "content": "test_comp1.md", "describes": ["`+comp1ID+`"]}
		]
	}`)
	writeTestFile(t, filepath.Join(dir, "alpha"), "arch_new.md", "# New\n")
	// Touching Comp1's own content leaf too satisfies the completeness
	// checker's rule that a module meta change (triggered by adding New to
	// module.json) requires every existing component's leaf to change as
	// well — unrelated to what this scenario is actually testing.
	writeTestFile(t, filepath.Join(dir, "alpha"), "arch_comp1.md", "# Comp1 architecture, updated\n")

	diffOut, err := runSpex(t, "diff", "--json", "--snapshot", snapshotPath, "--spec-dir", dir)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}

	writeTestJournal(t, dir, []string{
		`{"event":"registered","eid":"cafe0000:pipeline-prop","proposal":"pipeline-prop","git_head":"cafe0000"}`,
	})

	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})
	stdout, stderr, err := runPlan(t, diffOut,
		"--proposal", "pipeline-prop", "--git-head", "deadbeefcafe", "--tasks", tasksPath, "--spec-dir", dir,
	)
	if err != nil {
		t.Fatalf("plan: %v\nstderr: %s\ndiff output: %s", err, stderr, diffOut)
	}
	cs := parsePlanChangeset(t, stdout)
	if cs.Version != plan.ChangesetVersion {
		t.Errorf("version: want %d, got %d", plan.ChangesetVersion, cs.Version)
	}
	if len(cs.Ops) == 0 {
		t.Fatal("want at least the epic + New create op")
	}
}

// --- S4: Empty diff produces an epic-only or empty changeset ---

func TestPlanCommand_S4_EmptyDiff_SynthesizesEpicWhenUnregisteredInFold(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	writeTestJournal(t, specDir, []string{
		`{"event":"registered","eid":"cafe:prop-a","proposal":"prop-a","git_head":"cafe0000"}`,
	})
	diffPath := writePlanDiff(t, t.TempDir(), "empty.json", nil, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", "prop-a", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)
	if len(cs.Ops) != 1 || cs.Ops[0].SpecNodeKind != plan.KindProposalEpic {
		t.Fatalf("want a single synthesized epic op, got %+v", cs.Ops)
	}
}

func TestPlanCommand_S4_EmptyDiff_NoOpsWhenEpicAlreadyPaired(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	writeTestJournal(t, specDir, []string{
		`{"event":"task_created","proposal":"prop-b","task_id":"existing-epic"}`,
	})
	diffPath := writePlanDiff(t, t.TempDir(), "empty.json", nil, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", "prop-b", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)
	if len(cs.Ops) != 0 {
		t.Fatalf("want an empty op list when an epic task is already paired, got %+v", cs.Ops)
	}
	if cs.Ops == nil {
		t.Error("ops must marshal as an empty array, not null")
	}
}

// --- S5: Diff input containing errors refuses to proceed ---

func TestPlanCommand_S5_DiffErrors_RefusesToProceed(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	diffPath := writePlanDiff(t, t.TempDir(), "diff.json", nil, []merkle.DiffError{
		{Type: "incomplete_change", Message: "structural change lacks corresponding leaf changes", Path: "abcdef012345", Related: []string{"abcdef012345"}},
		{Type: "surviving_name", Message: "removed node's name is claimed by a surviving node", Path: "fedcba987654", Related: []string{"fedcba987654"}},
	})

	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})
	stdout, stderr, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error when the diff carries errors")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "incomplete_change") || !strings.Contains(stderr, "structural change lacks corresponding leaf changes") {
		t.Errorf("want stderr to carry the first diff error, got %q", stderr)
	}
	if !strings.Contains(stderr, "surviving_name") || !strings.Contains(stderr, "removed node's name is claimed by a surviving node") {
		t.Errorf("want stderr to carry the second diff error, not merely the first, got %q", stderr)
	}
}

// --- S6: --tasks drives both decisions ---

// TestPlanCommand_S6_TasksDriveBothDecisions runs the fixture's one diff
// (existing/existing2 modified, removed removed) through three task-state
// files that differ in nothing else: one-unlisted (existing's task absent —
// modified with no live work; removed's task absent too), all-open (every
// pairing open) and empty ("tasks": []). The empty run must match the
// one-unlisted run's treatment extended to every pairing
// (spec/plan/test_plan_command.md S6).
func TestPlanCommand_S6_TasksDriveBothDecisions(t *testing.T) {
	f := setupPlanFixture(t)
	dir := t.TempDir()

	// One modified node's task (existing) and the removed node's task
	// (removed) are unlisted; the other modified node's task (existing2)
	// stays open, so the one-unlisted and all-open runs actually differ.
	tasksOneUnlisted := writePlanTasks(t, dir, "one-unlisted.json", map[string]string{
		"task-existing2": "open",
	})
	tasksAllOpen := writePlanTasks(t, dir, "all-open.json", map[string]string{
		"task-existing": "open", "task-existing2": "open", "task-removed": "open",
	})
	tasksEmpty := writePlanTasks(t, dir, "empty.json", map[string]string{})

	run := func(tasksPath string) plan.Changeset {
		t.Helper()
		stdout, stderr, err := runPlan(t, "",
			"--proposal", f.proposal, "--git-head", f.gitHead,
			"--diff", f.diffPath, "--tasks", tasksPath, "--spec-dir", f.specDir,
		)
		if err != nil {
			t.Fatalf("plan --tasks %s: %v\n%s", tasksPath, err, stderr)
		}
		return parsePlanChangeset(t, stdout)
	}

	// opsFor collects every op naming id, either as SpecNodeID (create,
	// retarget) or as a close op's Target.TaskID — a close carries no
	// SpecNodeID, only the task ref it closes (arch_changeset_builder.md,
	// "Close ops use Target and Reason alone").
	opsFor := func(cs plan.Changeset, id, taskID string) []plan.Op {
		var out []plan.Op
		for _, op := range cs.Ops {
			if op.SpecNodeID == id {
				out = append(out, op)
			}
			if op.Type == plan.OpClose && op.Target != nil && op.Target.TaskID == taskID {
				out = append(out, op)
			}
		}
		return out
	}
	hasType := func(ops []plan.Op, kind string) bool {
		for _, op := range ops {
			if op.Type == kind {
				return true
			}
		}
		return false
	}

	// one-unlisted: existing (modified, unlisted task) -> one plain create,
	// no close; removed (removed, unlisted task) -> cleanup create, no
	// close; existing2 (modified, open task) -> retarget.
	unlisted := run(tasksOneUnlisted)
	existingOps := opsFor(unlisted, f.existingID, "task-existing")
	if !hasType(existingOps, plan.OpCreate) || hasType(existingOps, plan.OpClose) || hasType(existingOps, plan.OpRetarget) {
		t.Errorf("one-unlisted: want a plain create and nothing else for existing, got %+v", existingOps)
	}
	removedOps := opsFor(unlisted, f.removedID, "task-removed")
	var cleanupOp *plan.Op
	for i := range removedOps {
		if removedOps[i].SpecNodeKind == plan.KindCleanup {
			cleanupOp = &removedOps[i]
		}
	}
	if cleanupOp == nil || hasType(removedOps, plan.OpClose) {
		t.Errorf("one-unlisted: want a cleanup create and no close for removed, got %+v", removedOps)
	} else if cleanupOp.Title != "Code cleanup: alpha/Removed" {
		t.Errorf("cleanup title: got %q", cleanupOp.Title)
	}
	if !hasType(opsFor(unlisted, f.existing2ID, "task-existing2"), plan.OpRetarget) {
		t.Errorf("one-unlisted: want a retarget for the still-open existing2 pairing, got %+v", opsFor(unlisted, f.existing2ID, "task-existing2"))
	}

	// all-open: both modified nodes retarget, the removed node closes, and
	// no cleanup is minted.
	allOpen := run(tasksAllOpen)
	for _, tc := range []struct{ id, taskID string }{{f.existingID, "task-existing"}, {f.existing2ID, "task-existing2"}} {
		if !hasType(opsFor(allOpen, tc.id, tc.taskID), plan.OpRetarget) {
			t.Errorf("all-open: want a retarget for %s, got %+v", tc.id, opsFor(allOpen, tc.id, tc.taskID))
		}
	}
	removedAllOpen := opsFor(allOpen, f.removedID, "task-removed")
	if !hasType(removedAllOpen, plan.OpClose) {
		t.Errorf("all-open: want a close for removed, got %+v", removedAllOpen)
	}
	for _, op := range removedAllOpen {
		if op.SpecNodeKind == plan.KindCleanup {
			t.Errorf("all-open: want no cleanup create for removed, got %+v", op)
		}
	}

	// empty: every tracked node reads as finished — the one-unlisted run's
	// treatment of existing/removed, now extended to existing2 too.
	empty := run(tasksEmpty)
	for _, tc := range []struct{ id, taskID string }{{f.existingID, "task-existing"}, {f.existing2ID, "task-existing2"}} {
		ops := opsFor(empty, tc.id, tc.taskID)
		if !hasType(ops, plan.OpCreate) || hasType(ops, plan.OpClose) || hasType(ops, plan.OpRetarget) {
			t.Errorf("empty: want a plain create and nothing else for %s, got %+v", tc.id, ops)
		}
	}
	removedEmpty := opsFor(empty, f.removedID, "task-removed")
	hasCleanup := false
	for _, op := range removedEmpty {
		if op.SpecNodeKind == plan.KindCleanup {
			hasCleanup = true
		}
	}
	if !hasCleanup || hasType(removedEmpty, plan.OpClose) {
		t.Errorf("empty: want a cleanup create and no close for removed, got %+v", removedEmpty)
	}
}

// --- S7: A claimed task refuses the run — exit 2 ---

func TestPlanCommand_S7_ClaimedTaskRefusesRun(t *testing.T) {
	f := setupPlanFixture(t)
	outPath := filepath.Join(t.TempDir(), "changeset.json")

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneInProgress, "--spec-dir", f.specDir,
		"--out", outPath,
	)
	if err == nil {
		t.Fatal("want error when a claimed task's node changed")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit 2, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want empty stdout, got %q", stdout)
	}
	msg := err.Error() + stderr
	if !strings.Contains(msg, "task-existing") || !strings.Contains(msg, "task-existing2") {
		t.Errorf("want the error to name both claimed tasks, got %q", msg)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Errorf("want no --out file written on refusal")
	}
}

// S7's second arm (test_plan_command.md:60): the claimed task's node is
// removed rather than modified — cancelling claimed work is refused
// exactly as moving it is.
func TestPlanCommand_S7_ClaimedTaskRefusesRun_RemovedNode(t *testing.T) {
	f := setupPlanFixture(t)
	outPath := filepath.Join(t.TempDir(), "changeset.json")

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksRemovedInProgress, "--spec-dir", f.specDir,
		"--out", outPath,
	)
	if err == nil {
		t.Fatal("want error when a claimed task's node was removed")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit 2, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want empty stdout, got %q", stdout)
	}
	msg := err.Error() + stderr
	if !strings.Contains(msg, "task-removed") {
		t.Errorf("want the error to name the removed node's claimed task, got %q", msg)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Errorf("want no --out file written on refusal")
	}
}

// --- S8: --absorb marks a node out of the op stream ---

func TestPlanCommand_S8_AbsorbMarksNodeOutOfStream(t *testing.T) {
	f := setupPlanFixture(t)
	absorbPath := writePlanAbsorb(t, t.TempDir(), "absorb.json", []map[string]string{
		{"node": f.existingID, "reason": "cosmetic rewording"},
	})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--absorb", absorbPath, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)

	for _, op := range cs.Ops {
		if op.SpecNodeID == f.existingID {
			t.Errorf("want no op for the absorbed node, got %+v", op)
		}
	}
	if len(cs.Absorbed) != 1 || cs.Absorbed[0].Node != f.existingID || cs.Absorbed[0].Reason != "cosmetic rewording" {
		t.Fatalf("want one absorbed entry for %s, got %+v", f.existingID, cs.Absorbed)
	}
	if cs.Absorbed[0].Before != "h0" || cs.Absorbed[0].After != "h1" {
		t.Errorf("want before/after hashes copied from the diff, got %+v", cs.Absorbed[0])
	}
}

func TestPlanCommand_S8_AbsorbInvalidMarks_Exit2(t *testing.T) {
	f := setupPlanFixture(t)
	cases := []struct {
		name string
		node string
	}{
		{"added node", f.newID},
		{"removed node", f.removedID},
		{"absent node", "aaaaaaaaaaaa"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			absorbPath := writePlanAbsorb(t, t.TempDir(), "absorb.json", []map[string]string{
				{"node": tc.node, "reason": "bogus"},
			})
			stdout, _, err := runPlan(t, "",
				"--proposal", f.proposal, "--git-head", f.gitHead,
				"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--absorb", absorbPath, "--spec-dir", f.specDir,
			)
			if err == nil {
				t.Fatalf("want error marking %s", tc.name)
			}
			if code := exitCodeOf(err); code != 2 {
				t.Errorf("want exit 2, got %d (%v)", code, err)
			}
			if !strings.Contains(err.Error(), tc.node) {
				t.Errorf("want error naming %s, got %v", tc.node, err)
			}
			if stdout != "" {
				t.Errorf("want empty stdout, got %q", stdout)
			}
		})
	}
}

func TestPlanCommand_S8_AbsorbBypassesClaimedTaskRefusal(t *testing.T) {
	f := setupPlanFixture(t)
	absorbPath := writePlanAbsorb(t, t.TempDir(), "absorb.json", []map[string]string{
		{"node": f.existingID, "reason": "cosmetic"},
		{"node": f.existing2ID, "reason": "cosmetic2"},
	})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneInProgress, "--absorb", absorbPath, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("with absorb marks on both in_progress nodes: want exit 0, got %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)
	if len(cs.Absorbed) != 2 {
		t.Fatalf("want 2 absorbed entries, got %+v", cs.Absorbed)
	}

	_, _, err2 := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneInProgress, "--spec-dir", f.specDir,
	)
	if err2 == nil {
		t.Fatal("want the same fixture, unmarked, to refuse — proving the mark is what makes the difference")
	}
	if code := exitCodeOf(err2); code != 2 {
		t.Errorf("want exit 2 without the mark, got %d", code)
	}
}

// --- S9: Missing required flags are errors ---

func TestPlanCommand_S9_MissingRequiredFlags(t *testing.T) {
	f := setupPlanFixture(t)
	cases := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{"missing proposal", []string{"--git-head", f.gitHead, "--diff", f.diffPath, "--tasks", f.tasksAllOpen, "--spec-dir", f.specDir}, "proposal"},
		{"missing git-head", []string{"--proposal", f.proposal, "--diff", f.diffPath, "--tasks", f.tasksAllOpen, "--spec-dir", f.specDir}, "git-head"},
		{"malformed git-head", []string{"--proposal", f.proposal, "--git-head", "not-a-sha", "--diff", f.diffPath, "--tasks", f.tasksAllOpen, "--spec-dir", f.specDir}, "git-head"},
		// The --tasks arm is what pins the flag as required: a command that
		// defaulted a missing artifact to "nothing in flight" would pass
		// every other scenario and re-create in-flight work in production
		// (spec/plan/test_plan_command.md, S9).
		{"missing tasks", []string{"--proposal", f.proposal, "--git-head", f.gitHead, "--diff", f.diffPath, "--spec-dir", f.specDir}, "tasks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runPlan(t, "", tc.args...)
			if err == nil {
				t.Fatalf("want error for %s", tc.name)
			}
			if stdout != "" {
				t.Errorf("want no partial output, got %q", stdout)
			}
			msg := err.Error() + stderr
			if !strings.Contains(msg, tc.wantFlag) {
				t.Errorf("want error naming %s, got %q", tc.wantFlag, msg)
			}
		})
	}
}

// --- S10: --out writes atomically ---

func TestPlanCommand_S10_OutWritesAtomically(t *testing.T) {
	f := setupPlanFixture(t)
	outPath := filepath.Join(t.TempDir(), "changeset.json")

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--out", outPath, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	if stdout != "" {
		t.Errorf("want empty stdout when --out is set, got %q", stdout)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read --out file: %v", err)
	}

	stdoutForm, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("stdout form: %v", err)
	}
	if string(data) != stdoutForm {
		t.Fatalf("--out file does not match stdout form\nfile: %s\nstdout: %s", data, stdoutForm)
	}

	entries, err := os.ReadDir(filepath.Dir(outPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("want no leftover temp file, found %s", e.Name())
		}
	}

	prior := string(data)
	_, _, err2 := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneInProgress, "--out", outPath, "--spec-dir", f.specDir,
	)
	if err2 == nil {
		t.Fatal("want the claimed-task fixture to refuse")
	}
	after, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read --out file after refused run: %v", err)
	}
	if string(after) != prior {
		t.Errorf("want the --out target untouched after a refused run")
	}
}

// --- S11: Deterministic output across runs ---

func TestPlanCommand_S11_DeterministicAcrossRuns(t *testing.T) {
	f := setupPlanFixture(t)
	var first string
	for i := 0; i < 5; i++ {
		stdout, stderr, err := runPlan(t, "",
			"--proposal", f.proposal, "--git-head", f.gitHead,
			"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir,
		)
		if err != nil {
			t.Fatalf("run %d: %v\n%s", i, err, stderr)
		}
		if i == 0 {
			first = stdout
			continue
		}
		if stdout != first {
			t.Fatalf("run %d differs from run 0\nrun0: %s\nrun%d: %s", i, first, i, stdout)
		}
	}
}

// --- S12: Exit codes ---

// A malformed journal line is now caught by the lifecycle pre-flight
// (spec/lifecycle/arch_project_resolver.md, spec/plan/arch_plan_command.md
// pre-flight step 3: "the lifecycle pre-flight refuses an uninitialised or
// broken project before the fold is reached") before PlanCommand's own
// journal read is ever attempted — a broken project, naming spex doctor,
// not the fold-level "read journal" error. See
// drifts/drift-spexmachina-uiei.8-plan-exit-codes.json for the still-open
// doc gap this closes in code but not yet in arch_plan_command.md's Exit
// Codes section or test_plan_command.md's S12 prose (both still list a
// malformed journal under exit 1; drift-spexmachina-uiei.7.json covers only
// the missing not-a-spex-project bullet, not this one).
func TestPlanCommand_S12_MalformedJournalLine_BrokenProject(t *testing.T) {
	f := setupPlanFixture(t)
	if err := os.WriteFile(filepath.Join(projectStateDir(f.specDir), lifecycle.JournalFileName), []byte("not-json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead, "--diff", f.diffPath, "--tasks", f.tasksAllOpen, "--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for a malformed journal line")
	}
	if code := exitCodeOf(err); code != exitNotAProject {
		t.Errorf("want the not-a-project exit code %d, got %d (%v)", exitNotAProject, code, err)
	}
	if !strings.Contains(err.Error(), "spex doctor") {
		t.Errorf("want error naming 'spex doctor', got %v", err)
	}
}

func TestPlanCommand_S12_UnresolvableDep_Exit2(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	aID := schema.IdentityHash("alpha", "component", "A")
	ghostID := schema.IdentityHash("alpha", "component", "Ghost")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"m","modules":[{"id":"`+modID+`","name":"alpha","path":"alpha"}]}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+aID+`", "name": "A", "content": "arch_a.md", "uses": ["`+ghostID+`"]}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_a.md", "# A\n")
	seedPlanSnapshot(t, specDir)
	writeTestJournal(t, specDir, []string{
		`{"event":"registered","eid":"cafe:p","proposal":"p","git_head":"cafe0000"}`,
	})
	diffPath := writePlanDiff(t, t.TempDir(), "diff.json", []diffChange{
		{Path: aID, Type: "added", Impact: "arch_impl", Module: "alpha", NodeType: "component", NewHash: "h-a"},
	}, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	_, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for an unresolvable dep")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit 2, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), ghostID) {
		t.Errorf("want error naming the unresolvable spec_node_id %s, got %v", ghostID, err)
	}
}

// --- E1: --tasks names a file that does not exist ---

func TestPlanCommand_E1_TasksFileMissing(t *testing.T) {
	f := setupPlanFixture(t)
	stdout, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", filepath.Join(t.TempDir(), "does-not-exist.json"), "--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for a missing --tasks file")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), "plan: read tasks:") {
		t.Errorf("want error prefixed 'plan: read tasks:', got %v", err)
	}
	if stdout != "" {
		t.Errorf("want empty stdout, got %q", stdout)
	}
}

// --- E2: The task-state file is valid but names no task the fold knows ---

func TestPlanCommand_E2_TasksFileNamesUnknownTask_MatchesEmpty(t *testing.T) {
	f := setupPlanFixture(t)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{"task-unknown": "open"})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", tasksPath, "--spec-dir", f.specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}

	stdout2, _, err2 := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksEmpty, "--spec-dir", f.specDir,
	)
	if err2 != nil {
		t.Fatalf("plan (empty tasks): %v", err2)
	}
	if stdout != stdout2 {
		t.Errorf("want output identical to the empty task-state variant\nwith unknown task: %s\nempty:             %s", stdout, stdout2)
	}
}

// A file failing the task-state schema — here, a closed status, which the
// artifact's two-value enum (open, in_progress) forbids — exits 1 naming the
// violated constraint; plan never adapts a foreign shape
// (spec/plan/test_plan_command.md, E2).
func TestPlanCommand_E2_TasksFileFailsSchema_Exit1(t *testing.T) {
	f := setupPlanFixture(t)
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"tasks":[{"task_id":"task-existing","status":"closed"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", path, "--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for a task-state file with a closed status")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), "plan: read tasks:") {
		t.Errorf("want error prefixed 'plan: read tasks:', got %v", err)
	}
}

// --- E3: Malformed absorb file ---

func TestPlanCommand_E3_MalformedAbsorbJSON_Exit1(t *testing.T) {
	f := setupPlanFixture(t)
	path := filepath.Join(t.TempDir(), "absorb.json")
	if err := os.WriteFile(path, []byte(`{"broken":`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksAllOpen, "--absorb", path, "--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed absorb JSON")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("want error naming the file, got %v", err)
	}
}

func TestPlanCommand_E3_AbsorbEntryNotHex_Exit2(t *testing.T) {
	f := setupPlanFixture(t)
	path := writePlanAbsorb(t, t.TempDir(), "absorb.json", []map[string]string{
		{"node": "not-a-hex-id", "reason": "x"},
	})
	_, _, err := runPlan(t, "",
		"--proposal", f.proposal, "--git-head", f.gitHead,
		"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--absorb", path, "--spec-dir", f.specDir,
	)
	if err == nil {
		t.Fatal("want error for a non-hex absorb entry")
	}
	if code := exitCodeOf(err); code != 2 {
		t.Errorf("want exit 2, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), "not-a-hex-id") {
		t.Errorf("want error naming the entry, got %v", err)
	}
}

// --- E4: Concurrent invocations are safe ---

func TestPlanCommand_E4_ConcurrentInvocationsSafe(t *testing.T) {
	f := setupPlanFixture(t)
	binPath := buildSpexBinary(t)

	const n = 3
	var wg sync.WaitGroup
	outs := make([]string, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := exec.Command(binPath, "plan",
				"--proposal", f.proposal, "--git-head", f.gitHead,
				"--diff", f.diffPath, "--tasks", f.tasksOneUnlisted, "--spec-dir", f.specDir)
			out, err := cmd.Output()
			outs[i] = string(out)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("run %d failed: %v", i, err)
		}
	}
	for i := 1; i < n; i++ {
		if outs[i] != outs[0] {
			t.Errorf("run %d output differs from run 0", i)
		}
	}
}

// --- E5: Large diff ---

func TestPlanCommand_E5_LargeDiffPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-diff performance test in -short mode")
	}
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")

	const numModules = 20
	const compsPerModule = 25 // 500 total

	var modulesJSON strings.Builder
	modulesJSON.WriteString("[")
	var changes []diffChange
	for m := 0; m < numModules; m++ {
		modName := fmt.Sprintf("mod%d", m)
		modID := schema.IdentityHash("module", modName)
		if m > 0 {
			modulesJSON.WriteString(",")
		}
		requiresModuleJSON := "[]"
		if m > 0 {
			prevModID := schema.IdentityHash("module", fmt.Sprintf("mod%d", m-1))
			requiresModuleJSON = fmt.Sprintf(`["%s"]`, prevModID)
		}
		modulesJSON.WriteString(fmt.Sprintf(`{"id":"%s","name":"%s","path":"%s","requires_module":%s}`, modID, modName, modName, requiresModuleJSON))

		if err := os.MkdirAll(filepath.Join(specDir, modName), 0o755); err != nil {
			t.Fatal(err)
		}
		var compsJSON strings.Builder
		compsJSON.WriteString("[")
		for c := 0; c < compsPerModule; c++ {
			compName := fmt.Sprintf("Comp%d", c)
			compID := schema.IdentityHash(modName, "component", compName)
			if c > 0 {
				compsJSON.WriteString(",")
			}
			usesJSON := "[]"
			if c > 0 {
				prevCompID := schema.IdentityHash(modName, "component", fmt.Sprintf("Comp%d", c-1))
				usesJSON = fmt.Sprintf(`["%s"]`, prevCompID)
			}
			content := fmt.Sprintf("arch_%d.md", c)
			compsJSON.WriteString(fmt.Sprintf(`{"id":"%s","name":"%s","content":"%s","uses":%s}`, compID, compName, content, usesJSON))
			writeTestFile(t, filepath.Join(specDir, modName), content, "# "+compName+"\n")
			changes = append(changes, diffChange{Path: compID, Type: "added", Impact: "arch_impl", Module: modName, NodeType: "component", NewHash: "h-" + compID})
		}
		compsJSON.WriteString("]")
		writeTestFile(t, filepath.Join(specDir, modName), "module.json", `{"name":"`+modName+`","components":`+compsJSON.String()+`}`)
	}
	modulesJSON.WriteString("]")
	writeTestFile(t, specDir, "project.json", `{"name":"large","modules":`+modulesJSON.String()+`}`)
	seedPlanSnapshot(t, specDir)

	writeTestJournal(t, specDir, []string{
		`{"event":"registered","eid":"cafe:large-prop","proposal":"large-prop","git_head":"cafe0000"}`,
	})

	taskStatus := make(map[string]string, 300)
	for i := 0; i < 300; i++ {
		taskStatus[fmt.Sprintf("task-unrelated-%d", i)] = "open"
	}
	tasksPath := writePlanTasks(t, dir, "tasks.json", taskStatus)
	diffPath := writePlanDiff(t, dir, "diff.json", changes, nil)

	var (
		stdout, stderr string
		err            error
	)
	perf.Within(t, 5*time.Second, func() {
		stdout, stderr, err = runPlan(t, "",
			"--proposal", "large-prop", "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
		)
	})
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)
	wantOps := numModules*compsPerModule + 1 // + epic
	if len(cs.Ops) != wantOps {
		t.Errorf("want %d ops, got %d", wantOps, len(cs.Ops))
	}
}

// --- S13: The diff document itself is malformed or empty ---

func TestPlanCommand_DiffFileMalformedJSON_Exit1(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	diffPath := filepath.Join(t.TempDir(), "bad_diff.json")
	if err := os.WriteFile(diffPath, []byte(`[{"path": "foo"`), 0644); err != nil {
		t.Fatal(err)
	}
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for malformed diff JSON")
	}
	if !strings.Contains(err.Error(), "parse diff JSON") {
		t.Errorf("want error referencing diff parsing, got %v", err)
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want no changeset on a refused run, got %q", stdout)
	}
}

func TestPlanCommand_DiffFileEmpty_Exit1(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	diffPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(diffPath, nil, 0644); err != nil {
		t.Fatal(err)
	}
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for a zero-length diff file")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want no changeset on a refused run, got %q", stdout)
	}
}

func TestPlanCommand_S13_BareArrayDiff_Exit1(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	diffPath := filepath.Join(t.TempDir(), "bare_array.json")
	if err := os.WriteFile(diffPath, []byte(`[{"path":"abcdef012345","type":"added"}]`), 0644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "changeset.json")
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
		"--out", outPath,
	)
	if err == nil {
		t.Fatal("want error for a bare array rather than a diff document object")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want no changeset on a refused run, got %q", stdout)
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		t.Errorf("want no --out file written on a refused run")
	}
}

func TestPlanCommand_S13_ZeroBytesOnStdin_Exit1(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for zero bytes piped on stdin")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if stdout != "" {
		t.Errorf("want no changeset on a refused run, got %q", stdout)
	}
}

// TestPlanCommand_S12_MissingDiffFile_Exit1 covers the one S12 "missing diff
// file" case that plan_test.go otherwise left unexercised: --diff pointing
// at a path that does not exist (the `plan: read diff:` branch, distinct
// from the malformed-JSON and empty-file cases above).
func TestPlanCommand_S12_MissingDiffFile_Exit1(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	diffPath := filepath.Join(t.TempDir(), "does-not-exist.json")
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeef", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for a nonexistent --diff path")
	}
	if code := exitCodeOf(err); code != 1 {
		t.Errorf("want exit 1, got %d (%v)", code, err)
	}
	if !strings.Contains(err.Error(), diffPath) {
		t.Errorf("want error naming the missing diff path %q, got %v", diffPath, err)
	}
	if stdout != "" {
		t.Errorf("want no changeset on a refused run, got %q", stdout)
	}
}

// TestPlanCommand_NotAProject_ExitNotAProject covers the pre-flight
// arch_plan_command.md's pre-flight step 3 describes ("the lifecycle
// pre-flight refuses an uninitialised or broken project before the fold is
// reached") — a scenario test_plan_command.md's S12 Exit Codes list does not
// yet enumerate (see drifts/drift-spexmachina-uiei.7.json). Mirrors
// cmd/spex/diff.go's and cmd/spex/map.go's own tests for the same interim
// signal: the default snapshot's absence marks a never-initialised project.
func TestPlanCommand_NotAProject_ExitNotAProject(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	diffPath := writePlanDiff(t, dir, "diff.json", nil, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	_, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error when no project state exists")
	}
	if code := exitCodeOf(err); code != exitNotAProject {
		t.Errorf("want exit code %d, got %d (%v)", exitNotAProject, code, err)
	}
	if !strings.Contains(err.Error(), "spex init") {
		t.Errorf("want error naming 'spex init', got %v", err)
	}
}

// TestPlanCommand_BrokenProject_ExitNotAProject covers the pre-flight's
// other branch: a corrupted snapshot is a broken project (the
// not-a-spex-project exit code, naming 'spex doctor'), never the "run spex
// init" refusal — same distinction cmd/spex/diff.go's
// TestFR4_E2_DiffCommand_CorruptedSnapshot asserts.
func TestPlanCommand_BrokenProject_ExitNotAProject(t *testing.T) {
	specDir := setupMinimalPlanSpec(t)
	writeTestJournal(t, specDir, nil)
	snapshotPath := filepath.Join(projectStateDir(specDir), lifecycle.SnapshotFileName)
	if err := os.WriteFile(snapshotPath, []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}
	diffPath := writePlanDiff(t, t.TempDir(), "diff.json", nil, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	_, _, err := runPlan(t, "",
		"--proposal", "p", "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err == nil {
		t.Fatal("want error for a corrupted snapshot")
	}
	if code := exitCodeOf(err); code != exitNotAProject {
		t.Errorf("want the not-a-project exit code %d, got %d (%v)", exitNotAProject, code, err)
	}
	if !strings.Contains(err.Error(), "spex doctor") {
		t.Errorf("want error naming 'spex doctor', got %v", err)
	}
}

// --- S14: A removed node's tombstone participates in nothing ---

// TestPlanCommand_S14_ReAddedNode_YieldsPlainCreate seeds the journal with a
// node's added + task_created, then its removed + task_closed, then presents
// a diff reporting that same identity hash as added again — a re-add carries
// the same hash, since the hash is a function of module, kind and name. The
// tombstone must be withheld from the pairings NodeMatcher sees, so the
// re-add matches nothing and yields exactly one plain create: no close
// against the already-closed task, and no lineage dep to it
// (spec/plan/arch_plan_command.md, pre-flight step 3).
func TestPlanCommand_S14_ReAddedNode_YieldsPlainCreate(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	ghostID := schema.IdentityHash("alpha", "component", "Ghost")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"m","modules":[{"id":"`+modID+`","name":"alpha","path":"alpha"}]}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+ghostID+`", "name": "Ghost", "content": "arch_ghost.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_ghost.md", "# Ghost\n")
	seedPlanSnapshot(t, specDir)

	proposal := "2026-08-15-readd"
	writeTestJournal(t, specDir, []string{
		`{"event":"added","eid":"e1","node":"` + ghostID + `","name":"Ghost","node_type":"component","module":"alpha","before":null,"after":"h1","git_head":"cafe0000","proposal":"` + proposal + `"}`,
		`{"event":"task_created","for":"e1","task_id":"task-ghost"}`,
		`{"event":"removed","eid":"e2","node":"` + ghostID + `","name":"Ghost","node_type":"component","module":"alpha","before":"h1","after":null,"git_head":"cafe1111","proposal":"` + proposal + `"}`,
		`{"event":"task_closed","for":"e2","task_id":"task-ghost"}`,
		`{"event":"registered","eid":"cafe0000:` + proposal + `","proposal":"` + proposal + `","git_head":"cafe0000"}`,
	})

	diffPath := writePlanDiff(t, dir, "diff.json", []diffChange{
		{Path: ghostID, Type: "added", Impact: "arch_impl", Module: "alpha", NodeType: "component", NewHash: "h2"},
	}, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", proposal, "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)

	var creates, closes int
	var ghostOp *plan.Op
	for i := range cs.Ops {
		op := &cs.Ops[i]
		if op.Type == plan.OpClose {
			closes++
			t.Errorf("want no close op at all for a plain re-add (nothing tracks the pre-tombstone task), got %+v", op)
		}
		if op.SpecNodeID == ghostID && op.Type == plan.OpCreate {
			creates++
			ghostOp = op
		}
	}
	if creates != 1 {
		t.Fatalf("want exactly one plain create for the re-added node, got %d: %+v", creates, cs.Ops)
	}
	if ghostOp.SpecNodeKind != plan.KindComponent {
		t.Errorf("want a plain component create, got kind %q", ghostOp.SpecNodeKind)
	}
	if len(ghostOp.Deps) != 0 {
		t.Errorf("want no lineage dep (or any other dep) on the re-add, got %+v", ghostOp.Deps)
	}
	if len(ghostOp.Labels) > 0 {
		for _, l := range ghostOp.Labels {
			if l == plan.CleanupLabel {
				t.Errorf("want no cleanup label on a plain re-add create, got %v", ghostOp.Labels)
			}
		}
	}
}

// TestPlanCommand_S14_DeadEpicTombstoneNeverParents recreates the PR #217
// regression: a removed event whose node key collides with the epic's own
// key in the journal fold — the "registered"+task_created epic pairing gets
// overwritten by a later "removed" tombstone sharing that same key. Per
// arch_plan_command.md's tombstone rule, the removed entry must be withheld
// from the lookup Resolver reads for epic and parent resolution, so the run
// still resolves its epic normally (from the registration this command
// resolves independently of the fold) and no op is parented at a dead task.
func TestPlanCommand_S14_DeadEpicTombstoneNeverParents(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	newID := schema.IdentityHash("alpha", "component", "New")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"m","modules":[{"id":"`+modID+`","name":"alpha","path":"alpha"}]}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+newID+`", "name": "New", "content": "arch_new.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_new.md", "# New\n")
	seedPlanSnapshot(t, specDir)

	// The proposal slug is itself a 12-hex identity hash — a legal shape for
	// both a registeredEvent's proposal field (any non-empty string) and a
	// changeEvent's node field (schema-constrained to 12-hex). That lets the
	// removed event below name it as the node it removes, so the two events
	// share one key in the journal fold's latest-wins map — the exact
	// collision the PR #217 regression exploited: the epic pairing gets
	// overwritten by this tombstone in the raw journal fold.
	proposal := schema.IdentityHash("alpha", "component", "EpicKeyCollision")
	writeTestJournal(t, specDir, []string{
		`{"event":"registered","eid":"e-reg","proposal":"` + proposal + `","git_head":"cafe0000"}`,
		`{"event":"task_created","for":"e-reg","task_id":"epic-task-1"}`,
		`{"event":"removed","eid":"e-rm","node":"` + proposal + `","name":"Ghost","node_type":"component","module":"alpha","before":"h1","after":null,"git_head":"cafe1111","proposal":"other-proposal"}`,
	})

	diffPath := writePlanDiff(t, dir, "diff.json", []diffChange{
		{Path: newID, Type: "added", Impact: "arch_impl", Module: "alpha", NodeType: "component", NewHash: "h-new"},
	}, nil)
	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})

	stdout, stderr, err := runPlan(t, "",
		"--proposal", proposal, "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, stderr)
	}
	cs := parsePlanChangeset(t, stdout)

	var epicOpID string
	for _, op := range cs.Ops {
		if op.SpecNodeKind == plan.KindProposalEpic {
			epicOpID = op.OpID
		}
	}

	var newOp *plan.Op
	for i := range cs.Ops {
		if cs.Ops[i].SpecNodeID == newID && cs.Ops[i].Type == plan.OpCreate {
			newOp = &cs.Ops[i]
		}
	}
	if newOp == nil {
		t.Fatalf("want a create op for the new component, got ops: %+v", cs.Ops)
	}
	if newOp.Parent == nil {
		t.Fatal("want the create op to carry a parent ref")
	}
	switch newOp.Parent.Kind {
	case plan.RefOp:
		if epicOpID == "" || newOp.Parent.OpID != epicOpID {
			t.Errorf("want the parent ref to point at this run's synthesized epic op, got %+v (epic op id %q)", newOp.Parent, epicOpID)
		}
	case plan.RefTask:
		if newOp.Parent.TaskID != "epic-task-1" {
			t.Errorf("want the parent ref to point at the live epic task, got %+v", newOp.Parent)
		}
	default:
		t.Errorf("want a bead or op parent ref, got %+v", newOp.Parent)
	}
}

// --- S12: An empty plan-relevant list warns rather than refuses ---

func TestPlanCommand_S12_EmptyPlanRelevantList_WarnsAndProceeds(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, "spec")
	modID := schema.IdentityHash("module", "alpha")
	aID := schema.IdentityHash("alpha", "component", "A")

	if err := os.MkdirAll(filepath.Join(specDir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, specDir, "project.json", `{"name":"m","modules":[{"id":"`+modID+`","name":"alpha","path":"alpha"}]}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "module.json", `{
		"name": "alpha",
		"components": [
			{"id": "`+aID+`", "name": "A", "content": "arch_a.md"}
		]
	}`)
	writeTestFile(t, filepath.Join(specDir, "alpha"), "arch_a.md", "# A\n")
	// A resolved profile whose plan_relevant list is empty: legal per
	// schema.Profile.Validate — no minimum-length rule applies — and the
	// one shape the node-type gate reads as "nothing is task-producing"
	// (spec/plan/arch_action_classifier.md, "Node-Type Gate").
	writeTestFile(t, specDir, "profile.json", `{
		"node_types": [],
		"coverage_chains": [],
		"plan_relevant": [],
		"impact_levels": {},
		"absorbable": {}
	}`)
	seedPlanSnapshot(t, specDir)
	writeTestJournal(t, specDir, []string{
		`{"event":"registered","eid":"cafe:p-empty-plan-relevant","proposal":"p-empty-plan-relevant","git_head":"cafe0000"}`,
	})

	tasksPath := writePlanTasks(t, t.TempDir(), "tasks.json", map[string]string{})
	diffPath := writePlanDiff(t, dir, "diff.json", []diffChange{
		{Path: aID, Type: "added", Impact: "arch_impl", Module: "alpha", NodeType: "component", NewHash: "h-a"},
	}, nil)

	stdout, stderr, err := runPlan(t, "",
		"--proposal", "p-empty-plan-relevant", "--git-head", "deadbeefcafe", "--diff", diffPath, "--tasks", tasksPath, "--spec-dir", specDir,
	)
	if err != nil {
		t.Fatalf("want exit 0 for an empty plan-relevant list, got %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "no node type") {
		t.Errorf("want stderr to warn that no node type produces tasks, got %q", stderr)
	}
	cs := parsePlanChangeset(t, stdout)
	// The empty list gates the unmatched component out, and nothing else:
	// the proposal epic sits outside the plan-relevant order and survives
	// (spec/plan/arch_plan_command.md, "Exit Codes"), so the changeset
	// holds exactly the epic create and no op for the component.
	if len(cs.Ops) != 1 {
		t.Fatalf("want exactly the proposal epic op, got %d ops: %+v", len(cs.Ops), cs.Ops)
	}
	if cs.Ops[0].SpecNodeKind != plan.KindProposalEpic {
		t.Errorf("want the surviving op to be the proposal epic, got %+v", cs.Ops[0])
	}
	if cs.Ops[0].SpecNodeID == aID {
		t.Errorf("want no op for the gated-out component, got %+v", cs.Ops[0])
	}
}
