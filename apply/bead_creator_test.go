package apply

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

const testProposal = "2026-04-12-data-flow-contract-layer"

// mockCLI implements BeadCLI for testing without external binaries.
type mockCLI struct {
	created    []CreateOpts      // recorded Create calls
	findCalls  [][]string        // recorded FindExisting label args
	findResult map[string]string // label key → bead ID for FindExisting
	findErr    error             // error to return from FindExisting
	createFn   func(CreateOpts) (string, error)
	closeFn    func(id string, labels []string) error
	updateFn   func(id string, metadata map[string]string) error
	statusFn   func(id string) (string, error)
	closed     []closedBead  // recorded Close calls
	updated    []updatedBead // recorded Update calls
	nextID     int
}

type closedBead struct {
	ID     string
	Labels []string
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

func (m *mockCLI) Close(_ context.Context, id string, labels []string) error {
	if m.closeFn != nil {
		return m.closeFn(id, labels)
	}
	m.closed = append(m.closed, closedBead{ID: id, Labels: labels})
	return nil
}

func (m *mockCLI) Update(_ context.Context, id string, metadata map[string]string) error {
	if m.updateFn != nil {
		return m.updateFn(id, metadata)
	}
	m.updated = append(m.updated, updatedBead{ID: id, Metadata: metadata})
	return nil
}

func (m *mockCLI) Status(_ context.Context, id string) (string, error) {
	if m.statusFn != nil {
		return m.statusFn(id)
	}
	return "open", nil
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

func (s *mockStore) NextRecordID() (int, error) {
	return s.nextID, nil
}

func (s *mockStore) Replace(records []mapping.Record, nextID int) error {
	out := make([]mapping.Record, len(records))
	copy(out, records)
	s.records = out
	s.nextID = nextID
	return nil
}

func (s *mockStore) GetByProposalEpic(proposal string) (mapping.Record, error) {
	var match mapping.Record
	var found bool
	for _, r := range s.records {
		if r.NodeType != "proposal" || r.SpecNodeID != proposal {
			continue
		}
		if r.BeadStatus == "closed" {
			continue
		}
		if !found || r.ID > match.ID {
			match = r
			found = true
		}
	}
	if !found {
		return mapping.Record{}, fmt.Errorf("not found: proposal epic %s", proposal)
	}
	return match, nil
}

func (s *mockStore) addRecord(r mapping.Record) {
	if r.ID == 0 {
		r.ID = s.nextID
		s.nextID++
	} else if r.ID >= s.nextID {
		s.nextID = r.ID + 1
	}
	s.records = append(s.records, r)
}

// --- Proposal Epic Scenarios (S0a, S0b, S0c) ---

func TestREQ3_S0a_ProposalEpicCreatedFirst(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, "test-proposal-2026-04-20", actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.created) < 1 {
		t.Fatalf("want at least 1 Create call, got %d", len(cli.created))
	}

	epic := cli.created[0]
	if epic.Title != "test-proposal-2026-04-20" {
		t.Errorf("epic title: want %q, got %q", "test-proposal-2026-04-20", epic.Title)
	}
	if epic.Type != "epic" {
		t.Errorf("epic type: want %q, got %q", "epic", epic.Type)
	}
	if epic.Parent != "" {
		t.Errorf("epic parent: want empty, got %q", epic.Parent)
	}
	if len(epic.Deps) != 0 {
		t.Errorf("epic deps: want empty, got %v", epic.Deps)
	}

	// Subsequent creates use the epic bead ID as --parent.
	epicBeadID := ids[0]
	if len(cli.created) < 2 {
		t.Fatalf("want at least 2 Create calls, got %d", len(cli.created))
	}
	if cli.created[1].Parent != epicBeadID {
		t.Errorf("second create parent: want %q, got %q", epicBeadID, cli.created[1].Parent)
	}

	// Bead-map record is written for the epic.
	recs, _ := store.List()
	var epicRec *mapping.Record
	for i := range recs {
		if recs[i].BeadID == epicBeadID {
			epicRec = &recs[i]
			break
		}
	}
	if epicRec == nil {
		t.Fatalf("no bead-map record for epic %q", epicBeadID)
	}
	if epicRec.NodeType != "proposal" {
		t.Errorf("epic record node_type: want %q, got %q", "proposal", epicRec.NodeType)
	}
	if epicRec.BeadType != "epic" {
		t.Errorf("epic record bead_type: want %q, got %q", "epic", epicRec.BeadType)
	}
	if epicRec.SpecNodeID != "test-proposal-2026-04-20" {
		t.Errorf("epic record spec_node_id: want %q, got %q", "test-proposal-2026-04-20", epicRec.SpecNodeID)
	}
	if epicRec.SpecHash != "" {
		t.Errorf("epic record spec_hash: want empty, got %q", epicRec.SpecHash)
	}
	if epicRec.Module != "" {
		t.Errorf("epic record module: want empty, got %q", epicRec.Module)
	}
}

func TestREQ3_S0b_NoEpicWhenNoCreates(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.created) != 0 {
		t.Errorf("want 0 Create calls for empty creates, got %d", len(cli.created))
	}
	if len(ids) != 0 {
		t.Errorf("want 0 IDs, got %d: %v", len(ids), ids)
	}
}

func TestREQ3_S0c_SingleEpicPerRun(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "merkle", Node: "B", NodeType: "component", SpecNodeID: "merkle/component/1", Priority: -1},
		{Module: "impact", Node: "C", NodeType: "component", SpecNodeID: "impact/component/1", Priority: -1},
		{Module: "apply", Node: "D", NodeType: "component", SpecNodeID: "apply/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.created) != 5 {
		t.Fatalf("want 5 Create calls (1 epic + 4 components), got %d", len(cli.created))
	}

	// Exactly one epic call.
	epicCount := 0
	for _, c := range cli.created {
		if c.Type == "epic" {
			epicCount++
		}
	}
	if epicCount != 1 {
		t.Errorf("want exactly 1 epic Create, got %d", epicCount)
	}

	epicID := ids[0]
	for i, c := range cli.created[1:] {
		if c.Parent != epicID {
			t.Errorf("child %d parent: want %q, got %q", i, epicID, c.Parent)
		}
	}
}

// --- Create Scenarios (S1–S11, S15) ---

func TestREQ1_S1_CreateBead_ComponentType(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("want 2 IDs (epic + component), got %d", len(ids))
	}
	if len(cli.created) != 2 {
		t.Fatalf("want 2 Create calls, got %d", len(cli.created))
	}

	got := cli.created[1]
	if got.Title != "validator: ContentResolver" {
		t.Errorf("title: want %q, got %q", "validator: ContentResolver", got.Title)
	}
	if got.Type != "feature" {
		t.Errorf("type: want %q, got %q", "feature", got.Type)
	}
	if got.Parent != ids[0] {
		t.Errorf("parent: want epic %q, got %q", ids[0], got.Parent)
	}
}

func TestREQ1_S2_CreateBead_DataFlowType(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "merkle", Node: "Hash computation flow", NodeType: "data_flow", SpecHash: "def456", SpecNodeID: "merkle/data_flow/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[1]
	if got.Title != "merkle: Hash computation flow" {
		t.Errorf("title: want %q, got %q", "merkle: Hash computation flow", got.Title)
	}
	if got.Type != "task" {
		t.Errorf("type: want %q, got %q", "task", got.Type)
	}
	if got.Parent != ids[0] {
		t.Errorf("parent: want epic %q, got %q", ids[0], got.Parent)
	}
}

func TestREQ1_S3_CreateBead_MultiComponentTestSection(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{
			Module:         "apply",
			Node:           "Bead action tests",
			NodeType:       "test_section",
			SpecHash:       "fff000",
			SpecNodeID:     "apply/test_section/1",
			DescribesCount: 2,
			Priority:       -1,
		},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := cli.created[1]
	if got.Type != "task" {
		t.Errorf("type: want %q, got %q", "task", got.Type)
	}
	if got.Parent != ids[0] {
		t.Errorf("parent: want epic %q, got %q", ids[0], got.Parent)
	}
}

func TestREQ8_S5_ParentIsAlwaysProposalEpic(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "Component", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "merkle", Node: "Flow", NodeType: "data_flow", SpecNodeID: "merkle/data_flow/1", Priority: -1},
		{Module: "apply", Node: "MultiTest", NodeType: "test_section", SpecNodeID: "apply/test_section/1", DescribesCount: 2, Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.created) != 4 {
		t.Fatalf("want 4 Create calls (epic + 3), got %d", len(cli.created))
	}

	epicID := ids[0]
	// Epic itself has no parent.
	if cli.created[0].Parent != "" {
		t.Errorf("epic parent: want empty, got %q", cli.created[0].Parent)
	}
	// All other creates parent under the epic.
	for i := 1; i < 4; i++ {
		if cli.created[i].Parent != epicID {
			t.Errorf("create[%d] parent: want %q, got %q", i, epicID, cli.created[i].Parent)
		}
	}
}

func TestREQ8_S6_DepsBlocksForLineage(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "new123", SpecNodeID: "validator/component/1", OldBeadID: "spexmachina-77", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[1]
	wantDep := "blocks:spexmachina-77"
	if len(got.Deps) != 1 || got.Deps[0] != wantDep {
		t.Errorf("deps: want [%q], got %v", wantDep, got.Deps)
	}
}

func TestREQ8_S7_NoDepsForNewNodes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.created[1].Deps) != 0 {
		t.Errorf("deps: want empty for new node, got %v", cli.created[1].Deps)
	}
}

func TestREQ9_S8_PriorityPropagation(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "abc123", SpecNodeID: "validator/component/1", Priority: 0},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cli.created[1].Priority != 0 {
		t.Errorf("priority: want 0, got %d", cli.created[1].Priority)
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

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Epic is still created; the component create is skipped via idempotency.
	if len(cli.created) != 1 {
		t.Errorf("want 1 Create call (epic only), got %d", len(cli.created))
	}
	if len(ids) != 2 || ids[1] != "spexmachina-99" {
		t.Errorf("want [<epic>, spexmachina-99], got %v", ids)
	}
}

func TestREQ1_S10_SequentialBatch(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
		{Module: "merkle", Node: "C", NodeType: "component", SpecNodeID: "merkle/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 4 {
		t.Fatalf("want 4 IDs (epic + 3), got %d", len(ids))
	}
	if len(cli.created) != 4 {
		t.Fatalf("want 4 Create calls, got %d", len(cli.created))
	}
	// Component creates follow input order.
	for i, want := range []string{"validator: A", "validator: B", "merkle: C"} {
		if cli.created[i+1].Title != want {
			t.Errorf("call %d: want title %q, got %q", i+1, want, cli.created[i+1].Title)
		}
	}
}

func TestREQ1_S11_ErrorStopsBatch(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	callCount := 0
	cli.createFn = func(opts CreateOpts) (string, error) {
		callCount++
		// Epic (1), first action (2) succeed; second action (3) fails.
		if callCount == 3 {
			return "", fmt.Errorf("connection refused")
		}
		return fmt.Sprintf("mock-%d", callCount), nil
	}

	actions := []Action{
		{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", Priority: -1},
		{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", Priority: -1},
		{Module: "validator", Node: "C", NodeType: "component", SpecNodeID: "validator/component/3", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("want error containing %q, got %v", "connection refused", err)
	}
	// Epic + first action succeeded, second failed, third not attempted.
	if callCount != 3 {
		t.Errorf("want 3 Create calls (stopped after error), got %d", callCount)
	}
	if len(ids) != 2 {
		t.Errorf("want 2 IDs (epic + first action), got %d: %v", len(ids), ids)
	}
}

func TestREQ1_S15_CleanupBead(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "apply", Node: "BeadUpdater", NodeType: "component", OldBeadID: "spexmachina-lvf", Reason: "Code cleanup: BeadUpdater", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("want 2 IDs (epic + cleanup), got %d", len(ids))
	}

	if len(cli.created) != 2 {
		t.Fatalf("want 2 Create calls, got %d", len(cli.created))
	}
	got := cli.created[1]
	if got.Title != "Code cleanup: BeadUpdater" {
		t.Errorf("title: want %q, got %q", "Code cleanup: BeadUpdater", got.Title)
	}
	if got.Type != "task" {
		t.Errorf("type: want %q, got %q", "task", got.Type)
	}
	if got.Parent != ids[0] {
		t.Errorf("parent: want epic %q, got %q", ids[0], got.Parent)
	}
	wantDep := "blocks:spexmachina-lvf"
	if len(got.Deps) != 1 || got.Deps[0] != wantDep {
		t.Errorf("deps: want [%q], got %v", wantDep, got.Deps)
	}

	// Verify spex:cleanup label was set via Update.
	// Find update call for the cleanup bead (ids[1]).
	var found bool
	for _, u := range cli.updated {
		if u.ID == ids[1] && u.Metadata["spex"] == "cleanup" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cleanup label spex:cleanup not set on %s; updates: %v", ids[1], cli.updated)
	}

	// No mapping record for the cleanup bead itself (only the epic record).
	recs, _ := store.List()
	if len(recs) != 1 {
		t.Errorf("want 1 mapping record (epic only), got %d", len(recs))
	}
	if recs[0].NodeType != "proposal" {
		t.Errorf("only record should be the proposal epic, got NodeType %q", recs[0].NodeType)
	}
}

// --- Describes-length Gate Scenarios (G1, G2) ---

func TestREQ1_G1_SingleComponentTestSectionRejected(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{
			Module:         "apply",
			Node:           "SingleTest",
			NodeType:       "test_section",
			SpecNodeID:     "apply/test_section/1",
			DescribesCount: 1,
			Priority:       -1,
		},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err == nil {
		t.Fatal("want error for single-component test_section, got nil")
	}
	if !strings.Contains(err.Error(), "single-component test_section reached BeadCreator") {
		t.Errorf("error message: want 'single-component test_section reached BeadCreator', got %v", err)
	}
	// Only the epic was created; no test_section CLI call.
	if len(cli.created) != 1 {
		t.Errorf("want only epic created, got %d Create calls", len(cli.created))
	}
}

func TestREQ1_G2_MultiComponentTestSectionAccepted(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{
			Module:         "apply",
			Node:           "MultiTest",
			NodeType:       "test_section",
			SpecNodeID:     "apply/test_section/1",
			DescribesCount: 2,
			Priority:       -1,
		},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.created) != 2 {
		t.Fatalf("want 2 Create calls, got %d", len(cli.created))
	}
	if cli.created[1].Type != "task" {
		t.Errorf("type: want %q, got %q", "task", cli.created[1].Type)
	}
	if cli.created[1].Parent != ids[0] {
		t.Errorf("parent: want epic %q, got %q", ids[0], cli.created[1].Parent)
	}
}

// --- Edge Cases (E1, E3, E4, E5, E6, E7) ---

func TestREQ1_E1_EmptySpecHash(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecHash: "", SpecNodeID: "validator/component/1", Priority: -1},
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("want 2 IDs (epic + component), got %d", len(ids))
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

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err == nil {
		t.Fatal("want error from FindExisting, got nil")
	}
	if !strings.Contains(err.Error(), "bead CLI timeout") {
		t.Errorf("want error containing %q, got %v", "bead CLI timeout", err)
	}
	// Only the epic create was issued; no component create after FindExisting failure.
	if len(cli.created) != 1 {
		t.Errorf("want 1 Create call (epic only) after FindExisting error, got %d", len(cli.created))
	}
}

func TestREQ1_E4_EmptyCreates(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want 0 IDs for empty input, got %d", len(ids))
	}
	if len(cli.created) != 0 {
		t.Errorf("want 0 Create calls for empty input, got %d", len(cli.created))
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

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 51 {
		t.Fatalf("want 51 IDs (epic + 50), got %d", len(ids))
	}
	if len(cli.created) != 51 {
		t.Fatalf("want 51 Create calls, got %d", len(cli.created))
	}

	// Verify order matches input (epic at 0, then actions).
	for i, a := range actions {
		wantTitle := fmt.Sprintf("%s: %s", a.Module, a.Node)
		if cli.created[i+1].Title != wantTitle {
			t.Errorf("call %d: want title %q, got %q", i+1, wantTitle, cli.created[i+1].Title)
		}
	}
}

func TestREQ8_E6_TypeTableExhaustive(t *testing.T) {
	tests := []struct {
		nodeType string
		wantType string
	}{
		{"proposal", "epic"},
		{"component", "feature"},
		{"data_flow", "task"},
		{"test_section", "task"},
		{"impl_section", ""},
		{"module", ""},
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

	for _, nodeType := range []string{"impl_section", "module", "unknown"} {
		t.Run(nodeType, func(t *testing.T) {
			actions := []Action{
				{Module: "test", Node: "Something", NodeType: nodeType, SpecNodeID: "test/" + nodeType + "/1", Priority: -1},
			}
			_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
			if err == nil {
				t.Fatalf("want error for node type %q, got nil", nodeType)
			}
			if !strings.Contains(err.Error(), "does not get a bead") {
				t.Errorf("want 'does not get a bead' error, got %v", err)
			}
		})
	}
}

func TestREQ1_E7_EpicIDReuseAcrossRun(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := make([]Action, 20)
	for i := range actions {
		actions[i] = Action{
			Module:     "mod",
			Node:       fmt.Sprintf("C%d", i),
			NodeType:   "component",
			SpecNodeID: fmt.Sprintf("mod/component/%d", i),
			Priority:   -1,
		}
	}

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	epicID := ids[0]

	// All 20 subsequent creates use the epic bead ID as --parent, and no
	// other bead ID is used as parent during this run.
	for i := 1; i < len(cli.created); i++ {
		if cli.created[i].Parent != epicID {
			t.Errorf("create %d: want parent %q, got %q", i, epicID, cli.created[i].Parent)
		}
	}
}

// --- Spec-Graph Dependency Scenarios (D1–D4) ---

func TestREQ10_D1_DepsDepends(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", DepBeadIDs: []string{"spex-200", "spex-201"}, Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[1]
	if len(got.Deps) != 2 {
		t.Fatalf("want 2 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	if got.Deps[0] != "depends:spex-200" {
		t.Errorf("deps[0]: want %q, got %q", "depends:spex-200", got.Deps[0])
	}
	if got.Deps[1] != "depends:spex-201" {
		t.Errorf("deps[1]: want %q, got %q", "depends:spex-201", got.Deps[1])
	}
}

func TestREQ10_D2_BothBlocksAndDepends(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "ContentResolver", NodeType: "component", SpecNodeID: "validator/component/1", OldBeadID: "spex-100", DepBeadIDs: []string{"spex-200"}, Priority: -1},
	}

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[1]
	if len(got.Deps) != 2 {
		t.Fatalf("want 2 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	if got.Deps[0] != "blocks:spex-100" {
		t.Errorf("deps[0]: want %q, got %q", "blocks:spex-100", got.Deps[0])
	}
	if got.Deps[1] != "depends:spex-200" {
		t.Errorf("deps[1]: want %q, got %q", "depends:spex-200", got.Deps[1])
	}
}

func TestREQ10_D3_SkipDependsWhenEmpty(t *testing.T) {
	t.Run("nil DepBeadIDs", func(t *testing.T) {
		cli := newMockCLI()
		store := newMockStore()
		actions := []Action{
			{Module: "validator", Node: "A", NodeType: "component", SpecNodeID: "validator/component/1", OldBeadID: "old-1", Priority: -1},
		}
		_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := cli.created[1]
		if len(got.Deps) != 1 || got.Deps[0] != "blocks:old-1" {
			t.Errorf("want only blocks dep, got %v", got.Deps)
		}
	})

	t.Run("empty DepBeadIDs", func(t *testing.T) {
		cli := newMockCLI()
		store := newMockStore()
		actions := []Action{
			{Module: "validator", Node: "B", NodeType: "component", SpecNodeID: "validator/component/2", DepBeadIDs: []string{}, Priority: -1},
		}
		_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := cli.created[1]
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

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := cli.created[1]
	if len(got.Deps) != 5 {
		t.Fatalf("want 5 deps, got %d: %v", len(got.Deps), got.Deps)
	}
	for i, want := range []string{"depends:a", "depends:b", "depends:c", "depends:d", "depends:e"} {
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

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	recs, err := store.List()
	if err != nil {
		t.Fatalf("store.List: %v", err)
	}
	// Expect epic record + component record.
	if len(recs) != 2 {
		t.Fatalf("want 2 records (epic + component), got %d", len(recs))
	}

	// Find the component record (non-proposal).
	var compRec *mapping.Record
	for i := range recs {
		if recs[i].NodeType != "proposal" {
			compRec = &recs[i]
			break
		}
	}
	if compRec == nil {
		t.Fatal("no component mapping record created")
	}
	if compRec.SpecNodeID != "validator/component/1" {
		t.Errorf("SpecNodeID: want %q, got %q", "validator/component/1", compRec.SpecNodeID)
	}
	if compRec.BeadType != "feature" {
		t.Errorf("BeadType: want %q, got %q", "feature", compRec.BeadType)
	}
	if compRec.NodeType != "component" {
		t.Errorf("NodeType: want %q, got %q", "component", compRec.NodeType)
	}
	if compRec.Module != "validator" {
		t.Errorf("Module: want %q, got %q", "validator", compRec.Module)
	}
	if compRec.Component != "ContentResolver" {
		t.Errorf("Component: want %q, got %q", "ContentResolver", compRec.Component)
	}
	if compRec.ContentFile != "spec/validator/arch_content_resolver.md" {
		t.Errorf("ContentFile: want %q, got %q", "spec/validator/arch_content_resolver.md", compRec.ContentFile)
	}
	if compRec.SpecHash != "abc123" {
		t.Errorf("SpecHash: want %q, got %q", "abc123", compRec.SpecHash)
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

	_, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rec, err := store.Get(10)
	if err != nil {
		t.Fatalf("store.Get(10): %v", err)
	}
	// The component's new bead ID is the second create (after epic).
	if rec.BeadID != "mock-2" {
		t.Errorf("BeadID: want %q (updated), got %q", "mock-2", rec.BeadID)
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

	ids, err := CreateBeads(context.Background(), cli, store, testProposal, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the update for the component bead (ids[1]).
	componentBead := ids[1]
	var found bool
	for _, u := range cli.updated {
		if u.ID == componentBead {
			if v := u.Metadata["spex"]; v == "" {
				t.Errorf("expected spex label on %s, got metadata %v", componentBead, u.Metadata)
			} else if v != "2" {
				// Record IDs: 1=epic, 2=component.
				t.Errorf("label: want spex:2, got spex:%s", v)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no Update call for component bead %s; all updates: %v", componentBead, cli.updated)
	}
}

// --- Helper function tests ---

func TestREQ8_BeadType(t *testing.T) {
	tests := []struct {
		nodeType string
		want     string
	}{
		{"proposal", "epic"},
		{"component", "feature"},
		{"data_flow", "task"},
		{"test_section", "task"},
		{"impl_section", ""},
		{"module", ""},
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

func TestREQ9_InheritedPriority(t *testing.T) {
	tests := []struct {
		name    string
		actions []Action
		want    int
	}{
		{"none set", []Action{{Priority: -1}, {Priority: -1}}, -1},
		{"single set", []Action{{Priority: 2}, {Priority: -1}}, 2},
		{"lowest wins", []Action{{Priority: 2}, {Priority: 0}, {Priority: 3}}, 0},
		{"empty", nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inheritedPriority(tt.actions); got != tt.want {
				t.Errorf("want %d, got %d", tt.want, got)
			}
		})
	}
}
