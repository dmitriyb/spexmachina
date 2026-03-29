package apply

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// mockCLI implements BeadCLI for testing without external binaries.
type mockCLI struct {
	created    []CreateOpts      // recorded Create calls
	findCalls  [][]string        // recorded FindExisting label args
	findResult map[string]string // label key → bead ID for FindExisting
	findErr    error             // error to return from FindExisting
	createFn   func(CreateOpts) (string, error)
	closeFn    func(id, reason string) error
	updateFn   func(id string, metadata map[string]string) error
	closed     []closedBead    // recorded Close calls
	updated    []updatedBead   // recorded Update calls
	nextID     int
}

type closedBead struct {
	ID     string
	Reason string
}

type updatedBead struct {
	ID       string
	Metadata map[string]string
}

func newMockCLI() *mockCLI {
	return &mockCLI{
		findResult: make(map[string]string),
	}
}

func (m *mockCLI) Create(_ context.Context, opts CreateOpts) (string, error) {
	if m.createFn != nil {
		return m.createFn(opts)
	}
	m.created = append(m.created, opts)
	m.nextID++
	return fmt.Sprintf("mock-%d", m.nextID), nil
}

func (m *mockCLI) FindExisting(_ context.Context, labels []string) (string, error) {
	m.findCalls = append(m.findCalls, labels)
	if m.findErr != nil {
		return "", m.findErr
	}
	key := strings.Join(labels, ",")
	if id, ok := m.findResult[key]; ok {
		return id, nil
	}
	return "", nil
}

func (m *mockCLI) Close(_ context.Context, id string, reason string) error {
	if m.closeFn != nil {
		return m.closeFn(id, reason)
	}
	m.closed = append(m.closed, closedBead{ID: id, Reason: reason})
	return nil
}

func (m *mockCLI) Update(_ context.Context, id string, metadata map[string]string) error {
	if m.updateFn != nil {
		return m.updateFn(id, metadata)
	}
	m.updated = append(m.updated, updatedBead{ID: id, Metadata: metadata})
	return nil
}

// mockStore is an in-memory mapping.Store for tests.
type mockStore struct {
	records []mapping.Record
	nextID  int
}

func newMockStore() *mockStore {
	return &mockStore{nextID: 1}
}

func (s *mockStore) Create(r mapping.Record) (int, error) {
	r.ID = s.nextID
	s.nextID++
	s.records = append(s.records, r)
	return r.ID, nil
}

func (s *mockStore) Get(id int) (mapping.Record, error) {
	for _, r := range s.records {
		if r.ID == id {
			return r, nil
		}
	}
	return mapping.Record{}, fmt.Errorf("not found: %d", id)
}

func (s *mockStore) GetByBead(beadID string) (mapping.Record, error) {
	for _, r := range s.records {
		if r.BeadID == beadID {
			return r, nil
		}
	}
	return mapping.Record{}, fmt.Errorf("not found: bead %s", beadID)
}

func (s *mockStore) GetBySpecNode(specNodeID string) ([]mapping.Record, error) {
	var matches []mapping.Record
	for _, r := range s.records {
		if r.SpecNodeID == specNodeID {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("not found: %s", specNodeID)
	}
	return matches, nil
}

func (s *mockStore) Update(id int, updates map[string]string) error {
	for i, r := range s.records {
		if r.ID == id {
			if v, ok := updates["bead_id"]; ok {
				s.records[i].BeadID = v
			}
			if v, ok := updates["spec_hash"]; ok {
				s.records[i].SpecHash = v
			}
			return nil
		}
	}
	return fmt.Errorf("not found: %d", id)
}

func (s *mockStore) Delete(id int) error {
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found: %d", id)
}

func (s *mockStore) List() ([]mapping.Record, error) {
	result := make([]mapping.Record, len(s.records))
	copy(result, s.records)
	return result, nil
}

// addRecord inserts a pre-populated record into the mock store.
func (s *mockStore) addRecord(r mapping.Record) {
	if r.ID == 0 {
		r.ID = s.nextID
		s.nextID++
	} else if r.ID >= s.nextID {
		s.nextID = r.ID + 1
	}
	s.records = append(s.records, r)
}

// --- BeadCreator Scenarios (S1–S11, S15) ---

func TestREQ1_S1_CreateBead_ComponentType(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids))
	}
	if len(cli.created) != 1 {
		t.Fatalf("want 1 Create call, got %d", len(cli.created))
	}

	got := cli.created[0]
	if got.Title != "validator: ContentResolver" {
		t.Errorf("title: want %q, got %q", "validator: ContentResolver", got.Title)
	}
	if got.Type != "feature" {
		t.Errorf("type: want %q, got %q", "feature", got.Type)
	}
}

func TestREQ1_S2_CreateBead_TestSectionType(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "SchemaTests", NodeType: "test_section", SpecHash: "fff000", SpecNodeID: "validator/test_section/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids))
	}
	if cli.created[0].Type != "task" {
		t.Errorf("type: want %q, got %q", "task", cli.created[0].Type)
	}
}

func TestREQ1_S3_CreateBead_ModuleType(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "validator", NodeType: "module", SpecNodeID: "validator/module", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids))
	}
	if cli.created[0].Type != "epic" {
		t.Errorf("type: want %q, got %q", "epic", cli.created[0].Type)
	}
}

func TestREQ8_S4_ParentForComponent(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		SpecNodeID: "validator/module",
		BeadID:     "epic-001",
		Module:     "validator",
	})
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cli.created[0].Parent != "epic-001" {
		t.Errorf("parent: want %q, got %q", "epic-001", cli.created[0].Parent)
	}
}

func TestREQ8_S5_ParentForTestSection(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		SpecNodeID: "validator/component/1",
		BeadID:     "feature-002",
		Module:     "validator",
	})
	actions := []Action{
		{Module: "validator", Node: "SchemaTests", NodeType: "test_section", SpecNodeID: "validator/test_section/1", ParentSpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cli.created[0].Parent != "feature-002" {
		t.Errorf("parent: want %q, got %q", "feature-002", cli.created[0].Parent)
	}
}

func TestREQ8_S6_DepsBlocksForLineage(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "new123", SpecNodeID: "validator/component/1", OldBeadID: "spexmachina-77", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[0]
	wantDep := "blocks:spexmachina-77"
	if len(got.Deps) == 0 || got.Deps[0] != wantDep {
		t.Errorf("deps: want [%q], got %v", wantDep, got.Deps)
	}
}

func TestREQ8_S7_NoDepsForNewNodes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.created[0].Deps) != 0 {
		t.Errorf("deps: want empty for new node, got %v", cli.created[0].Deps)
	}
}

func TestREQ9_S8_PriorityPropagation(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: 0},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cli.created[0].Priority != 0 {
		t.Errorf("priority: want 0, got %d", cli.created[0].Priority)
	}
}

func TestREQ6_S9_Idempotency(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		ID:         42,
		SpecNodeID: "validator/component/1",
		BeadID:     "spexmachina-99",
		Module:     "validator",
		Component:  "ContentResolver",
	})
	cli.findResult["spex:42"] = "spexmachina-99"

	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.created) != 0 {
		t.Errorf("want 0 Create calls (idempotent), got %d", len(cli.created))
	}
	if len(ids) != 1 || ids[0] != "spexmachina-99" {
		t.Errorf("want [spexmachina-99], got %v", ids)
	}
}

func TestREQ1_S10_SequentialBatch(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "validator", Node: "DagChecker", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
		{Module: "merkle", Node: "Hasher", NodeType: "component", SpecNodeID: "merkle/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("want 3 IDs, got %d", len(ids))
	}
	if len(cli.created) != 3 {
		t.Fatalf("want 3 Create calls, got %d", len(cli.created))
	}
}

func TestREQ1_S11_ErrorStopsBatch(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	callCount := 0
	cli.createFn = func(opts CreateOpts) (string, error) {
		callCount++
		if callCount == 2 {
			return "", fmt.Errorf("connection refused")
		}
		return fmt.Sprintf("mock-%d", callCount), nil
	}

	actions := []Action{
		{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
		{Module: "validator", Node: "C", NodeType: "component", SpecNodeID: "validator/component/3", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("want error containing %q, got %v", "connection refused", err)
	}
	if len(ids) != 1 {
		t.Errorf("want 1 ID (first succeeded), got %d: %v", len(ids), ids)
	}
	if callCount != 2 {
		t.Errorf("want 2 Create calls (stopped after error), got %d", callCount)
	}
}

func TestREQ1_S15_CleanupBead(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "apply", Node: "BeadUpdater", NodeType: "component", OldBeadID: "spexmachina-lvf", Reason: "Code cleanup: BeadUpdater", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids))
	}

	if len(cli.created) != 1 {
		t.Fatalf("want 1 Create call, got %d", len(cli.created))
	}
	got := cli.created[0]
	if got.Title != "Code cleanup: BeadUpdater" {
		t.Errorf("title: want %q, got %q", "Code cleanup: BeadUpdater", got.Title)
	}
	if got.Type != "task" {
		t.Errorf("type: want %q, got %q", "task", got.Type)
	}
	wantDep := "blocks:spexmachina-lvf"
	if len(got.Deps) != 1 || got.Deps[0] != wantDep {
		t.Errorf("deps: want [%q], got %v", wantDep, got.Deps)
	}

	// Verify spex:cleanup label was set via Update.
	if len(cli.updated) != 1 {
		t.Fatalf("want 1 Update call (cleanup label), got %d", len(cli.updated))
	}
	if cli.updated[0].Metadata["spex"] != "cleanup" {
		t.Errorf("label: want spex:cleanup, got spex:%s", cli.updated[0].Metadata["spex"])
	}

	// No mapping record should be created.
	recs, _ := store.List()
	if len(recs) != 0 {
		t.Errorf("want 0 mapping records for cleanup bead, got %d", len(recs))
	}
}

// --- Edge Cases (E1, E3, E4, E5, E6) ---

func TestREQ1_E1_EmptySpecHash(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids))
	}
	// Bead created successfully despite empty hash.
	if len(cli.created) != 1 {
		t.Errorf("want 1 Create call, got %d", len(cli.created))
	}
}

func TestREQ6_E3_FindExistingError(t *testing.T) {
	cli := newMockCLI()
	cli.findErr = fmt.Errorf("bead CLI timeout")
	store := newMockStore()
	store.addRecord(mapping.Record{
		ID:         10,
		SpecNodeID: "validator/component/1",
		BeadID:     "old-bead",
		Module:     "validator",
	})

	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err == nil {
		t.Fatal("want error from FindExisting, got nil")
	}
	if !strings.Contains(err.Error(), "bead CLI timeout") {
		t.Errorf("want error containing %q, got %v", "bead CLI timeout", err)
	}
	if len(cli.created) != 0 {
		t.Errorf("want 0 Create calls after FindExisting error, got %d", len(cli.created))
	}
}

func TestREQ1_E4_EmptyCreates(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	ids, err := CreateBeads(context.Background(), cli, store, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want 0 IDs for empty input, got %d", len(ids))
	}
}

func TestREQ1_E5_LargeBatchOrdering(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	actions := make([]Action, 50)
	for i := range actions {
		actions[i] = Action{
			Module:     fmt.Sprintf("mod%d", i),
			Node:       fmt.Sprintf("Comp%d", i),
			NodeType:   "component",
			SpecNodeID: fmt.Sprintf("mod%d/component/%d", i, i),
			Priority:   -1,
		}
	}

	ids, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 50 {
		t.Fatalf("want 50 IDs, got %d", len(ids))
	}
	if len(cli.created) != 50 {
		t.Fatalf("want 50 Create calls, got %d", len(cli.created))
	}

	// Verify order matches input.
	for i, opts := range cli.created {
		wantTitle := fmt.Sprintf("mod%d: Comp%d", i, i)
		if opts.Title != wantTitle {
			t.Errorf("call %d: want title %q, got %q", i, wantTitle, opts.Title)
		}
	}
}

func TestREQ8_E6_TypeTableExhaustive(t *testing.T) {
	tests := []struct {
		nodeType string
		wantType string
		wantOK   bool
	}{
		{"module", "epic", true},
		{"component", "feature", true},
		{"test_section", "task", true},
		{"impl_section", "", false},
		{"data_flow", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			got := beadType(tt.nodeType)
			if got != tt.wantType {
				t.Errorf("beadType(%q): want %q, got %q", tt.nodeType, tt.wantType, got)
			}
		})
	}
}

func TestREQ8_E6_RejectNonBeadNodeTypes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	for _, nodeType := range []string{"impl_section", "data_flow"} {
		t.Run(nodeType, func(t *testing.T) {
			actions := []Action{
				{Module: "test", Node: "Something", NodeType: nodeType, SpecNodeID: "test/" + nodeType + "/1", Priority: -1},
			}
			_, err := CreateBeads(context.Background(), cli, store, actions)
			if err == nil {
				t.Fatalf("want error for node type %q, got nil", nodeType)
			}
			if !strings.Contains(err.Error(), "does not get a bead") {
				t.Errorf("want 'does not get a bead' error, got %v", err)
			}
		})
	}
}

// --- Spec-Graph Dependency Scenarios (D1–D4) ---

func TestREQ10_D1_DepsDepends(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", DepBeadIDs: []string{"spex-200", "spex-201"}, Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[0]
	if len(got.Deps) != 2 {
		t.Fatalf("want 2 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	if got.Deps[0] != "blocked-by:spex-200" {
		t.Errorf("deps[0]: want %q, got %q", "blocked-by:spex-200", got.Deps[0])
	}
	if got.Deps[1] != "blocked-by:spex-201" {
		t.Errorf("deps[1]: want %q, got %q", "blocked-by:spex-201", got.Deps[1])
	}
}

func TestREQ10_D2_BothBlocksAndDepends(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", OldBeadID: "spex-100", DepBeadIDs: []string{"spex-200"}, Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[0]
	if len(got.Deps) != 2 {
		t.Fatalf("want 2 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	if got.Deps[0] != "blocks:spex-100" {
		t.Errorf("deps[0]: want %q, got %q", "blocks:spex-100", got.Deps[0])
	}
	if got.Deps[1] != "blocked-by:spex-200" {
		t.Errorf("deps[1]: want %q, got %q", "blocked-by:spex-200", got.Deps[1])
	}
}

func TestREQ10_D3_SkipDependsWhenEmpty(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	t.Run("nil DepBeadIDs", func(t *testing.T) {
		actions := []Action{
			{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", OldBeadID: "old-1", Priority: -1},
		}
		_, err := CreateBeads(context.Background(), cli, store, actions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := cli.created[len(cli.created)-1]
		if len(got.Deps) != 1 || got.Deps[0] != "blocks:old-1" {
			t.Errorf("want only blocks dep, got %v", got.Deps)
		}
	})

	t.Run("empty DepBeadIDs", func(t *testing.T) {
		actions := []Action{
			{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", DepBeadIDs: []string{}, Priority: -1},
		}
		_, err := CreateBeads(context.Background(), cli, store, actions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := cli.created[len(cli.created)-1]
		if len(got.Deps) != 0 {
			t.Errorf("want no deps for empty DepBeadIDs, got %v", got.Deps)
		}
	})
}

func TestREQ10_D4_MultipleDependsDeps(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", DepBeadIDs: []string{"a", "b", "c", "d", "e"}, Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[0]
	if len(got.Deps) != 5 {
		t.Fatalf("want 5 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	for i, want := range []string{"blocked-by:a", "blocked-by:b", "blocked-by:c", "blocked-by:d", "blocked-by:e"} {
		if got.Deps[i] != want {
			t.Errorf("deps[%d]: want %q, got %q", i, want, got.Deps[i])
		}
	}
}

// --- Mapping Record Tests ---

func TestREQ7_CreatesMappingRecord(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", ContentFile: "spec/validator/arch_content_resolver.md", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs, err := store.List()
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}

	r := recs[0]
	if r.SpecNodeID != "validator/component/1" {
		t.Errorf("SpecNodeID: want %q, got %q", "validator/component/1", r.SpecNodeID)
	}
	if r.BeadID != "mock-1" {
		t.Errorf("BeadID: want %q, got %q", "mock-1", r.BeadID)
	}
	if r.BeadType != "feature" {
		t.Errorf("BeadType: want %q, got %q", "feature", r.BeadType)
	}
	if r.Module != "validator" {
		t.Errorf("Module: want %q, got %q", "validator", r.Module)
	}
	if r.Component != "ContentResolver" {
		t.Errorf("Component: want %q, got %q", "ContentResolver", r.Component)
	}
	if r.ContentFile != "spec/validator/arch_content_resolver.md" {
		t.Errorf("ContentFile: want %q, got %q", "spec/validator/arch_content_resolver.md", r.ContentFile)
	}
	if r.SpecHash != "abc123" {
		t.Errorf("SpecHash: want %q, got %q", "abc123", r.SpecHash)
	}
}

func TestREQ7_UpdatesMappingRecordForModifiedNodes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		ID:         10,
		SpecNodeID: "validator/component/1",
		BeadID:     "old-bead",
		BeadType:   "feature",
		Module:     "validator",
		Component:  "ContentResolver",
		SpecHash:   "oldhash",
	})

	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "newhash", SpecNodeID: "validator/component/1", OldBeadID: "old-bead", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := store.Get(10)
	if err != nil {
		t.Fatalf("store.Get(10): %v", err)
	}
	if rec.BeadID != "mock-1" {
		t.Errorf("BeadID: want %q (updated), got %q", "mock-1", rec.BeadID)
	}
	if rec.SpecHash != "newhash" {
		t.Errorf("SpecHash: want %q (updated), got %q", "newhash", rec.SpecHash)
	}
}

func TestREQ7_SetsBeadLabel(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated) != 1 {
		t.Fatalf("want 1 Update call (set label), got %d", len(cli.updated))
	}
	if cli.updated[0].ID != "mock-1" {
		t.Errorf("Update bead ID: want %q, got %q", "mock-1", cli.updated[0].ID)
	}
	if cli.updated[0].Metadata["spex"] != "1" {
		t.Errorf("label: want spex:1, got spex:%s", cli.updated[0].Metadata["spex"])
	}
}

// --- Helper function tests ---

func TestREQ8_BeadType(t *testing.T) {
	tests := []struct {
		nodeType string
		want     string
	}{
		{"module", "epic"},
		{"component", "feature"},
		{"test_section", "task"},
		{"impl_section", ""},
		{"data_flow", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			if got := beadType(tt.nodeType); got != tt.want {
				t.Errorf("beadType(%q): want %q, got %q", tt.nodeType, tt.want, got)
			}
		})
	}
}

func TestREQ8_ResolveParent_NoParent(t *testing.T) {
	store := newMockStore()

	// Module nodes have no parent.
	got := resolveParent(store, Action{NodeType: "module", Module: "validator"})
	if got != "" {
		t.Errorf("module parent: want empty, got %q", got)
	}

	// Component with no module epic in store.
	got = resolveParent(store, Action{NodeType: "component", Module: "validator"})
	if got != "" {
		t.Errorf("component parent (no epic): want empty, got %q", got)
	}
}

func TestREQ1_IsCleanup(t *testing.T) {
	if !isCleanup(Action{Reason: "Code cleanup: BeadUpdater"}) {
		t.Error("want true for cleanup reason")
	}
	if isCleanup(Action{Reason: "Spec node modified"}) {
		t.Error("want false for non-cleanup reason")
	}
	if isCleanup(Action{Reason: ""}) {
		t.Error("want false for empty reason")
	}
}

func containsLabel(labels []string, prefix string) bool {
	for _, l := range labels {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}
