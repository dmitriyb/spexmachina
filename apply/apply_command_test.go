package apply

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/mapping"
)

// --- Recording mock for operation order verification ---

type opRecord struct {
	Op       string
	ID       string
	Opts     CreateOpts
	Metadata map[string]string
}

type recordingCLI struct {
	ops  []opRecord
	mock *mockCLI
}

func newRecordingCLI() *recordingCLI {
	return &recordingCLI{mock: newMockCLI()}
}

func (r *recordingCLI) Create(ctx context.Context, opts CreateOpts) (string, error) {
	id, err := r.mock.Create(ctx, opts)
	r.ops = append(r.ops, opRecord{Op: "create", ID: id, Opts: opts})
	return id, err
}

func (r *recordingCLI) FindExisting(ctx context.Context, labels []string) (string, error) {
	return r.mock.FindExisting(ctx, labels)
}

func (r *recordingCLI) Close(ctx context.Context, id string, labels []string) error {
	err := r.mock.Close(ctx, id, labels)
	r.ops = append(r.ops, opRecord{Op: "close", ID: id})
	return err
}

func (r *recordingCLI) Update(ctx context.Context, id string, metadata map[string]string) error {
	err := r.mock.Update(ctx, id, metadata)
	m := make(map[string]string)
	for k, v := range metadata {
		m[k] = v
	}
	r.ops = append(r.ops, opRecord{Op: "update", ID: id, Metadata: m})
	return err
}

func (r *recordingCLI) Status(ctx context.Context, id string) (string, error) {
	return r.mock.Status(ctx, id)
}

// classifyPhase returns the pipeline phase for an operation.
func classifyPhase(op opRecord) string {
	switch op.Op {
	case "create":
		return "create"
	case "close":
		return "close"
	case "update":
		if v, ok := op.Metadata["spex"]; ok {
			if v == "obsolete" || v == "cleanup" {
				return "label"
			}
			return "create_label"
		}
		if _, ok := op.Metadata["commit"]; ok {
			return "label"
		}
		if _, ok := op.Metadata["spec_proposal"]; ok {
			return "tag"
		}
	}
	return "unknown"
}

// --- Test helpers ---

func defaultApplyOpts(creates []Action, obsoletes []Action, specDir string) ApplyOpts {
	return ApplyOpts{
		Creates:     creates,
		Obsoletes:   obsoletes,
		ProposalRef: "2026-02-23-spex-machina",
		SpecDir:     specDir,
		Logger:      testLogger(),
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		Now:         func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
}

// --- S1: Full action order (label -> create -> close -> tag -> snapshot) ---

func TestREQ8_S1_FullActionOrder(t *testing.T) {
	cli := newRecordingCLI()
	store := newMockStore()
	specDir := setupSpecDir(t)

	creates := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "aaa111", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "merkle", Node: "SnapshotFormat", NodeType: "component", SpecHash: "bbb222", SpecNodeID: "merkle/component/1", Priority: -1},
	}
	obsoletes := []Action{
		{BeadID: "spexmachina-77", Module: "merkle", Node: "Hasher", ChangeType: "modified"},
		{BeadID: "spexmachina-78", Module: "merkle", Node: "TreeBuilder", ChangeType: "modified"},
		{BeadID: "spexmachina-42", Module: "validator", Node: "LegacyChecker", ChangeType: "removed"},
	}

	// Add mapping record for removed node (LabelObsoletes deletes it).
	store.addRecord(mapping.Record{ID: 100, SpecNodeID: "validator/component/99", BeadID: "spexmachina-42", Module: "validator"})

	opts := defaultApplyOpts(creates, obsoletes, specDir)
	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("RunApply: %v", err)
	}

	// Classify all operations by phase.
	lastLabel := -1
	firstCreate := -1
	lastCreatePhase := -1
	firstClose := -1
	lastClose := -1
	firstTag := -1

	var labelCount, createCount, closeCount, tagCount int

	for i, op := range cli.ops {
		phase := classifyPhase(op)
		switch phase {
		case "label":
			lastLabel = i
			labelCount++
		case "create", "create_label":
			if firstCreate == -1 {
				firstCreate = i
			}
			lastCreatePhase = i
			if phase == "create" {
				createCount++
			}
		case "close":
			if firstClose == -1 {
				firstClose = i
			}
			lastClose = i
			closeCount++
		case "tag":
			if firstTag == -1 {
				firstTag = i
			}
			tagCount++
		}
	}

	// Verify phase ordering.
	if firstCreate != -1 && lastLabel != -1 && firstCreate <= lastLabel {
		t.Error("create phase started before label phase ended")
	}
	if firstClose != -1 && lastCreatePhase != -1 && firstClose <= lastCreatePhase {
		t.Error("close phase started before create phase ended")
	}
	if firstTag != -1 && lastClose != -1 && firstTag <= lastClose {
		t.Error("tag phase started before close phase ended")
	}

	// Verify counts. The proposal epic is created first on every run with
	// at least one create, producing one extra create and one extra tag.
	if labelCount != 3 {
		t.Errorf("want 3 label updates, got %d", labelCount)
	}
	if createCount != 3 {
		t.Errorf("want 3 creates (epic + 2 components), got %d", createCount)
	}
	if closeCount != 3 {
		t.Errorf("want 3 closes, got %d", closeCount)
	}
	if tagCount != 6 {
		t.Errorf("want 6 tag updates (epic + 2 new + 3 obsolete), got %d", tagCount)
	}

	// Verify snapshot was written.
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Errorf("snapshot not written: %v", err)
	}
}

// --- S2: Hierarchy ordering (epics -> features -> tasks) ---

func TestREQ8_S2_HierarchyOrdering(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	specDir := setupSpecDir(t)

	creates := []Action{
		{Module: "apply", Node: "BeadActionTests", NodeType: "test_section", SpecNodeID: "apply/test_section/1", DescribesCount: 2, Priority: -1},
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "merkle", Node: "HashFlow", NodeType: "data_flow", SpecNodeID: "merkle/data_flow/1", Priority: -1},
		{Module: "validator", Node: "DagChecker", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
	}

	opts := defaultApplyOpts(creates, nil, specDir)
	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("RunApply: %v", err)
	}

	// Epic (1) + 2 components + 1 data_flow + 1 test_section = 5 creates.
	if len(cli.created) != 5 {
		t.Fatalf("want 5 creates, got %d", len(cli.created))
	}

	// Order: proposal epic, then features/data_flow tasks, then test_section task.
	if cli.created[0].Type != "epic" {
		t.Errorf("first create type: want epic, got %s", cli.created[0].Type)
	}
	// Levels 0 (component, data_flow) come before level 1 (test_section).
	// Last create must be the test_section task.
	if cli.created[4].Type != "task" {
		t.Errorf("last create type: want task, got %s", cli.created[4].Type)
	}
	// Everything between epic and last is feature/task at level 0.
	for i := 1; i <= 3; i++ {
		if cli.created[i].Type != "feature" && cli.created[i].Type != "task" {
			t.Errorf("create[%d] type: want feature or task, got %s", i, cli.created[i].Type)
		}
	}
}

// --- S5: Empty report is a no-op but snapshot still saved ---

func TestREQ8_S5_EmptyReportNoOp(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	specDir := setupSpecDir(t)

	opts := defaultApplyOpts(nil, nil, specDir)
	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("RunApply: %v", err)
	}

	if len(cli.created) != 0 {
		t.Errorf("want 0 creates for empty report, got %d", len(cli.created))
	}
	if len(cli.closed) != 0 {
		t.Errorf("want 0 closes for empty report, got %d", len(cli.closed))
	}

	// Snapshot is still written.
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Errorf("snapshot not written for empty report: %v", err)
	}
}

// --- S6: Idempotency — applying same report twice produces no extra creates ---

func TestREQ6_S6_IdempotencyNoExtraCreates(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	specDir := setupSpecDir(t)

	creates := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "aaa111", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "merkle", Node: "SnapshotFormat", NodeType: "component", SpecHash: "bbb222", SpecNodeID: "merkle/component/1", Priority: -1},
	}

	opts := defaultApplyOpts(creates, nil, specDir)

	// First run: creates epic + 2 components.
	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(cli.created) != 3 {
		t.Fatalf("first run: want 3 creates (epic + 2 components), got %d", len(cli.created))
	}

	// Set up FindExisting to return existing component beads for second run.
	// Records 1 = epic, 2 and 3 = components.
	cli.findResult["spex:2"] = "mock-2"
	cli.findResult["spex:3"] = "mock-3"
	cli.created = nil

	// Second run: should still create a fresh epic but skip the component creates.
	err = RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(cli.created) != 1 {
		t.Errorf("second run: want 1 create (epic only), got %d", len(cli.created))
	}
}

// --- S7: Idempotency — already-closed beads are skipped (no close attempt) ---

func TestREQ6_S7_IdempotencyAlreadyClosed(t *testing.T) {
	cli := newMockCLI()
	cli.statusFn = func(id string) (string, error) {
		return "closed", nil
	}
	cli.closeFn = func(id string, labels []string) error {
		t.Fatalf("Close should not be called for already-closed bead %s", id)
		return nil
	}
	store := newMockStore()
	specDir := setupSpecDir(t)

	obsoletes := []Action{
		{BeadID: "spexmachina-77", Module: "merkle", Node: "Hasher", ChangeType: "modified"},
	}

	opts := defaultApplyOpts(nil, obsoletes, specDir)

	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("want no error (already-closed beads skipped), got: %v", err)
	}
	if len(cli.closed) != 0 {
		t.Errorf("want 0 Close calls (bead already closed), got %d", len(cli.closed))
	}
}

// --- S8: Dry-run prints actions without executing ---

func TestREQ8_S8_DryRunPrintsActions(t *testing.T) {
	stdout := &bytes.Buffer{}
	creates := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", Priority: -1},
		{Module: "merkle", Node: "SnapshotFormat", NodeType: "component", Priority: -1},
	}
	obsoletes := []Action{
		{BeadID: "spexmachina-77", Module: "merkle", Node: "Hasher"},
		{BeadID: "spexmachina-78", Module: "merkle", Node: "TreeBuilder"},
		{BeadID: "spexmachina-42", Module: "validator", Node: "LegacyChecker"},
	}

	opts := ApplyOpts{
		Creates:     creates,
		Obsoletes:   obsoletes,
		ProposalRef: "2026-02-23-spex-machina",
		DryRun:      true,
		Stdout:      stdout,
		Stderr:      &bytes.Buffer{},
	}

	err := RunApply(context.Background(), nil, nil, opts)
	if err != nil {
		t.Fatalf("RunApply dry-run: %v", err)
	}

	output := stdout.String()

	// Verify expected output lines.
	expected := []string{
		"label spexmachina-77 (spex:obsolete, commit:<HEAD>)",
		"label spexmachina-78 (spex:obsolete, commit:<HEAD>)",
		"label spexmachina-42 (spex:obsolete, commit:<HEAD>)",
		"create validator/ContentResolver --type feature",
		"create merkle/SnapshotFormat --type feature",
		"close spexmachina-77",
		"close spexmachina-78",
		"close spexmachina-42",
		"tag 6 beads with proposal 2026-02-23-spex-machina",
		"save snapshot",
	}
	for _, line := range expected {
		if !strings.Contains(output, line) {
			t.Errorf("dry-run output missing line %q\ngot:\n%s", line, output)
		}
	}

	// Verify order: labels before creates, creates before closes.
	labelEnd := strings.LastIndex(output, "label ")
	createStart := strings.Index(output, "create ")
	closeStart := strings.Index(output, "close ")
	tagStart := strings.Index(output, "tag ")

	if createStart < labelEnd {
		t.Error("creates appear before labels in dry-run output")
	}
	if closeStart < createStart {
		t.Error("closes appear before creates in dry-run output")
	}
	if tagStart < closeStart {
		t.Error("tag appears before closes in dry-run output")
	}
}

// --- S10: Create failure aborts and preserves snapshot ---

func TestREQ8_S10_CreateFailureAbortsNoSnapshot(t *testing.T) {
	cli := newMockCLI()
	callCount := 0
	cli.createFn = func(opts CreateOpts) (string, error) {
		callCount++
		if callCount == 2 {
			return "", fmt.Errorf("bead CLI unavailable")
		}
		return fmt.Sprintf("mock-%d", callCount), nil
	}
	store := newMockStore()
	specDir := setupSpecDir(t)

	creates := []Action{
		{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
	}

	opts := defaultApplyOpts(creates, nil, specDir)
	err := RunApply(context.Background(), cli, store, opts)
	if err == nil {
		t.Fatal("want error from create failure, got nil")
	}
	if !strings.Contains(err.Error(), "bead CLI unavailable") {
		t.Errorf("want error containing %q, got: %v", "bead CLI unavailable", err)
	}

	// Snapshot should NOT be written.
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err == nil {
		t.Error("snapshot was written despite create failure")
	}
}

// --- S11: Close error continues, snapshot still saved ---

func TestREQ8_S11_CloseWarningContinues(t *testing.T) {
	cli := newMockCLI()
	closeCallCount := 0
	cli.closeFn = func(id string, labels []string) error {
		closeCallCount++
		if id == "spexmachina-77" {
			return fmt.Errorf("network timeout")
		}
		return nil
	}
	store := newMockStore()
	specDir := setupSpecDir(t)

	obsoletes := []Action{
		{BeadID: "spexmachina-77", Module: "merkle", Node: "Hasher", ChangeType: "modified"},
		{BeadID: "spexmachina-78", Module: "merkle", Node: "TreeBuilder", ChangeType: "modified"},
	}

	opts := defaultApplyOpts(nil, obsoletes, specDir)

	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("want no error (close errors do not abort), got: %v", err)
	}

	// Both beads are open (default mock), so both get Close calls.
	if closeCallCount != 2 {
		t.Errorf("want 2 Close calls (both attempted), got %d", closeCallCount)
	}

	// Snapshot still saved despite close errors.
	snapshotPath := filepath.Join(specDir, ".snapshot.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Errorf("snapshot not written despite close errors: %v", err)
	}
}

// --- E4: Large report (100+ actions) ---

func TestREQ8_E4_LargeReport(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	specDir := setupSpecDir(t)

	creates := make([]Action, 50)
	for i := range creates {
		creates[i] = Action{
			Module:     fmt.Sprintf("mod%d", i),
			Node:       fmt.Sprintf("Comp%d", i),
			NodeType:   "component",
			SpecNodeID: fmt.Sprintf("mod%d/component/%d", i, i),
			Priority:   -1,
		}
	}

	obsoletes := make([]Action, 50)
	for i := range obsoletes {
		obsoletes[i] = Action{
			BeadID:     fmt.Sprintf("bead-%d", i),
			Module:     fmt.Sprintf("mod%d", i),
			Node:       fmt.Sprintf("Old%d", i),
			ChangeType: "modified",
		}
	}

	opts := defaultApplyOpts(creates, obsoletes, specDir)
	err := RunApply(context.Background(), cli, store, opts)
	if err != nil {
		t.Fatalf("RunApply with 100 actions: %v", err)
	}

	// Epic + 50 components.
	if len(cli.created) != 51 {
		t.Errorf("want 51 creates (epic + 50 components), got %d", len(cli.created))
	}
}

// --- Topological Ordering Scenarios ---

// T1: Topological ordering within feature level based on DepBeadIDs

func TestREQ11_T1_TopoSortWithinFeatureLevel(t *testing.T) {
	actions := []Action{
		{Module: "m", Node: "A", NodeType: "component", OldBeadID: "old-A"},
		{Module: "m", Node: "B", NodeType: "component", OldBeadID: "old-B", DepBeadIDs: []string{"old-A"}},
		{Module: "m", Node: "C", NodeType: "component", OldBeadID: "old-C", DepBeadIDs: []string{"old-B"}},
	}

	sorted, err := SortCreateActions(actions)
	if err != nil {
		t.Fatalf("SortCreateActions: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("want 3 actions, got %d", len(sorted))
	}
	if sorted[0].Node != "A" {
		t.Errorf("first: want A, got %s", sorted[0].Node)
	}
	if sorted[1].Node != "B" {
		t.Errorf("second: want B, got %s", sorted[1].Node)
	}
	if sorted[2].Node != "C" {
		t.Errorf("third: want C, got %s", sorted[2].Node)
	}
}

// T2: Topological ordering does not affect cross-type ordering

func TestREQ11_T2_TopoSortDoesNotAffectCrossType(t *testing.T) {
	// Features (components) and data_flow tasks share level 0 and are
	// topologically sorted; test_section tasks are level 1 and come last.
	actions := []Action{
		{Module: "m", Node: "X", NodeType: "component", OldBeadID: "old-X", DepBeadIDs: []string{"old-Y"}},
		{Module: "m", Node: "Y", NodeType: "component", OldBeadID: "old-Y"},
		{Module: "m", Node: "Task", NodeType: "test_section"},
	}

	sorted, err := SortCreateActions(actions)
	if err != nil {
		t.Fatalf("SortCreateActions: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("want 3 actions, got %d", len(sorted))
	}

	// Level 0 first (Y before X via topo sort), then level 1 (Task).
	if sorted[0].Node != "Y" {
		t.Errorf("first: want Y (dependency of X), got %s", sorted[0].Node)
	}
	if sorted[1].Node != "X" {
		t.Errorf("second: want X, got %s", sorted[1].Node)
	}
	if sorted[2].Node != "Task" {
		t.Errorf("third: want Task, got %s", sorted[2].Node)
	}
}

// T3: Independent beads within a type level maintain stable order

func TestREQ11_T3_StableOrderNoDeps(t *testing.T) {
	actions := []Action{
		{Module: "m", Node: "A", NodeType: "component"},
		{Module: "m", Node: "B", NodeType: "component"},
		{Module: "m", Node: "C", NodeType: "component"},
	}

	sorted, err := SortCreateActions(actions)
	if err != nil {
		t.Fatalf("SortCreateActions: %v", err)
	}

	// Original order preserved.
	for i, want := range []string{"A", "B", "C"} {
		if sorted[i].Node != want {
			t.Errorf("position %d: want %s, got %s", i, want, sorted[i].Node)
		}
	}
}

// T4: Circular dependency within a type level is detected

func TestREQ11_T4_CircularDependencyDetected(t *testing.T) {
	actions := []Action{
		{Module: "m", Node: "A", NodeType: "component", OldBeadID: "old-A", DepBeadIDs: []string{"old-B"}},
		{Module: "m", Node: "B", NodeType: "component", OldBeadID: "old-B", DepBeadIDs: []string{"old-A"}},
	}

	_, err := SortCreateActions(actions)
	if err == nil {
		t.Fatal("want error for circular dependency, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("want error containing %q, got: %v", "circular dependency", err)
	}
}

// T5: DepBeadIDs referencing already-existing beads do not affect ordering

func TestREQ11_T5_ExternalDepsIgnored(t *testing.T) {
	actions := []Action{
		{Module: "m", Node: "A", NodeType: "component", DepBeadIDs: []string{"spex-existing"}},
		{Module: "m", Node: "B", NodeType: "component"},
	}

	sorted, err := SortCreateActions(actions)
	if err != nil {
		t.Fatalf("SortCreateActions: %v", err)
	}

	// spex-existing is not in the batch (no matching OldBeadID), so no reordering.
	if sorted[0].Node != "A" {
		t.Errorf("first: want A, got %s", sorted[0].Node)
	}
	if sorted[1].Node != "B" {
		t.Errorf("second: want B, got %s", sorted[1].Node)
	}
}

// --- Unit tests for topoSortActions ---

func TestREQ11_TopoSort_Empty(t *testing.T) {
	sorted, err := topoSortActions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 0 {
		t.Errorf("want 0 actions, got %d", len(sorted))
	}
}

func TestREQ11_TopoSort_Single(t *testing.T) {
	actions := []Action{{Module: "m", Node: "A"}}
	sorted, err := topoSortActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 1 || sorted[0].Node != "A" {
		t.Errorf("want [A], got %v", sorted)
	}
}

func TestREQ11_TopoSort_Chain(t *testing.T) {
	actions := []Action{
		{Node: "C", OldBeadID: "old-C", DepBeadIDs: []string{"old-B"}},
		{Node: "A", OldBeadID: "old-A"},
		{Node: "B", OldBeadID: "old-B", DepBeadIDs: []string{"old-A"}},
	}

	sorted, err := topoSortActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sorted[0].Node != "A" {
		t.Errorf("first: want A, got %s", sorted[0].Node)
	}
	if sorted[1].Node != "B" {
		t.Errorf("second: want B, got %s", sorted[1].Node)
	}
	if sorted[2].Node != "C" {
		t.Errorf("third: want C, got %s", sorted[2].Node)
	}
}

// --- Unit tests for collectAffectedBeadIDs ---

func TestREQ4_CollectAffectedBeadIDs(t *testing.T) {
	created := []string{"new-1", "new-2"}
	obsoletes := []Action{
		{BeadID: "old-1"},
		{BeadID: "new-1"}, // duplicate with created
		{BeadID: "old-2"},
	}

	ids := collectAffectedBeadIDs(created, obsoletes)

	// Should deduplicate.
	if len(ids) != 4 {
		t.Fatalf("want 4 unique IDs, got %d: %v", len(ids), ids)
	}

	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("duplicate ID: %s", id)
		}
		seen[id] = true
	}
}

// --- Unit tests for typePriority ---

func TestREQ8_TypePriority(t *testing.T) {
	tests := []struct {
		nodeType string
		want     int
	}{
		{"component", 0},
		{"data_flow", 0},
		{"test_section", 1},
		{"module", 2},
		{"unknown", 2},
		{"", 2},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			if got := typePriority(tt.nodeType); got != tt.want {
				t.Errorf("typePriority(%q): want %d, got %d", tt.nodeType, tt.want, got)
			}
		})
	}
}
