package mapping

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// stubStore implements Store using in-memory records for testing.
type stubStore struct {
	records []Record
	nextID  int
}

func newStubStore() *stubStore {
	return &stubStore{nextID: 1}
}

func (s *stubStore) Create(r Record) (int, error) {
	r.ID = s.nextID
	s.nextID++
	s.records = append(s.records, r)
	return r.ID, nil
}

func (s *stubStore) Get(id int) (Record, error) {
	for _, r := range s.records {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: %d", ErrNotFound, id)
}

func (s *stubStore) GetByBead(beadID string) (Record, error) {
	for _, r := range s.records {
		if r.BeadID == beadID {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: bead_id %q", ErrNotFound, beadID)
}

func (s *stubStore) GetBySpecNode(specNodeID string) (Record, error) {
	for _, r := range s.records {
		if r.SpecNodeID == specNodeID {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("map: %w: spec_node_id %q", ErrNotFound, specNodeID)
}

func (s *stubStore) UpdateSpecHash(id int, hash string) error {
	for i, r := range s.records {
		if r.ID == id {
			s.records[i].SpecHash = hash
			return nil
		}
	}
	return fmt.Errorf("map: %w: %d", ErrNotFound, id)
}

func (s *stubStore) Delete(id int) error {
	for i, r := range s.records {
		if r.ID == id {
			s.records = append(s.records[:i], s.records[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("map: %w: %d", ErrNotFound, id)
}

func (s *stubStore) List() ([]Record, error) {
	result := make([]Record, len(s.records))
	copy(result, s.records)
	return result, nil
}

// stubSpecGraph implements SpecGraph for testing.
type stubSpecGraph struct {
	modules map[string]ModuleInfo // keyed by name
	hashes  map[string]string     // spec_node_id → hash
}

func newStubSpecGraph() *stubSpecGraph {
	return &stubSpecGraph{
		modules: map[string]ModuleInfo{},
		hashes:  map[string]string{},
	}
}

func (sg *stubSpecGraph) ModuleByName(name string) (ModuleInfo, error) {
	m, ok := sg.modules[name]
	if !ok {
		return ModuleInfo{}, fmt.Errorf("module %q not found", name)
	}
	return m, nil
}

func (sg *stubSpecGraph) ModuleByID(id int) (ModuleInfo, error) {
	for _, m := range sg.modules {
		if m.ID == id {
			return m, nil
		}
	}
	return ModuleInfo{}, fmt.Errorf("module id %d not found", id)
}

func (sg *stubSpecGraph) NodeHash(specNodeID string) (string, error) {
	h, ok := sg.hashes[specNodeID]
	if !ok {
		return "", fmt.Errorf("hash not found for %q", specNodeID)
	}
	return h, nil
}

func TestFR2_Check_Ready_AllDependenciesClosed(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// Module B (dependency) with one component, fully implemented.
	spec.modules["modB"] = ModuleInfo{
		ID:   2,
		Name: "modB",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hashB1",
		BeadStatus: "closed",
	})

	// Module A depends on B.
	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "hashA1",
	})

	spec.hashes["modA/component/1"] = "hashA1"

	result, err := Check(context.Background(), store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "ready" {
		t.Fatalf("status: want ready, got %s (blockers: %v)", result.Status, result.Blockers)
	}
	if result.Record.BeadID != "bead-a1" {
		t.Fatalf("record bead_id: want bead-a1, got %s", result.Record.BeadID)
	}
}

func TestFR2_Check_Blocked_DependencyModuleNotImplemented(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// Module B has a component with open bead.
	spec.modules["modB"] = ModuleInfo{
		ID:   2,
		Name: "modB",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hashB1",
		BeadStatus: "open",
	})

	// Module A depends on B.
	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "hashA1",
	})

	spec.hashes["modA/component/1"] = "hashA1"

	result, err := Check(context.Background(), store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status: want blocked, got %s", result.Status)
	}
	if len(result.Blockers) != 1 {
		t.Fatalf("blockers count: want 1, got %d", len(result.Blockers))
	}
	if result.Blockers[0].SpecNodeID != "modB/component/1" {
		t.Fatalf("blocker spec_node_id: want modB/component/1, got %s", result.Blockers[0].SpecNodeID)
	}
	if result.Blockers[0].BeadID != "bead-b1" {
		t.Fatalf("blocker bead_id: want bead-b1, got %s", result.Blockers[0].BeadID)
	}
}

func TestFR2_Check_Blocked_ComponentUsesNotClosed(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// Module with two components: X uses Y.
	spec.modules["mod"] = ModuleInfo{
		ID:   1,
		Name: "mod",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompY"},
			{ID: 2, Name: "CompX", Uses: []int{1}},
		},
	}

	store.Create(Record{
		SpecNodeID: "mod/component/1",
		BeadID:     "bead-y",
		Module:     "mod",
		Component:  "CompY",
		SpecHash:   "hashY",
		BeadStatus: "open",
	})
	store.Create(Record{
		SpecNodeID: "mod/component/2",
		BeadID:     "bead-x",
		Module:     "mod",
		Component:  "CompX",
		SpecHash:   "hashX",
	})

	spec.hashes["mod/component/2"] = "hashX"

	result, err := Check(context.Background(), store, spec, "bead-x")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status: want blocked, got %s", result.Status)
	}
	if len(result.Blockers) != 1 {
		t.Fatalf("blockers count: want 1, got %d", len(result.Blockers))
	}
	if result.Blockers[0].SpecNodeID != "mod/component/1" {
		t.Fatalf("blocker: want mod/component/1, got %s", result.Blockers[0].SpecNodeID)
	}
}

func TestFR2_Check_UnknownBead(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	_, err := Check(context.Background(), store, spec, "nonexistent")
	if err == nil {
		t.Fatal("want error for unknown bead, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR2_Check_Stale(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	spec.modules["mod"] = ModuleInfo{
		ID:   1,
		Name: "mod",
		Components: []ComponentInfo{
			{ID: 1, Name: "Comp1"},
		},
	}

	store.Create(Record{
		SpecNodeID: "mod/component/1",
		BeadID:     "bead-1",
		Module:     "mod",
		Component:  "Comp1",
		SpecHash:   "old-hash",
	})

	// Current hash differs from stored hash.
	spec.hashes["mod/component/1"] = "new-hash"

	result, err := Check(context.Background(), store, spec, "bead-1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "stale" {
		t.Fatalf("status: want stale, got %s", result.Status)
	}
	if result.StaleHash != "new-hash" {
		t.Fatalf("stale_hash: want new-hash, got %s", result.StaleHash)
	}
}

func TestNFR4_Check_Deterministic(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	spec.modules["modB"] = ModuleInfo{
		ID:   2,
		Name: "modB",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
			{ID: 2, Name: "CompB2"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hb1",
		BeadStatus: "open",
	})
	store.Create(Record{
		SpecNodeID: "modB/component/2",
		BeadID:     "bead-b2",
		Module:     "modB",
		Component:  "CompB2",
		SpecHash:   "hb2",
		BeadStatus: "open",
	})

	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "ha1",
	})

	spec.hashes["modA/component/1"] = "ha1"

	ctx := context.Background()
	r1, err := Check(ctx, store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check 1: %v", err)
	}
	r2, err := Check(ctx, store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check 2: %v", err)
	}

	if r1.Status != r2.Status {
		t.Fatalf("status differs: %s vs %s", r1.Status, r2.Status)
	}
	if len(r1.Blockers) != len(r2.Blockers) {
		t.Fatalf("blocker count differs: %d vs %d", len(r1.Blockers), len(r2.Blockers))
	}
	for i := range r1.Blockers {
		if r1.Blockers[i].SpecNodeID != r2.Blockers[i].SpecNodeID {
			t.Fatalf("blocker[%d] spec_node_id differs: %s vs %s", i, r1.Blockers[i].SpecNodeID, r2.Blockers[i].SpecNodeID)
		}
		if r1.Blockers[i].BeadID != r2.Blockers[i].BeadID {
			t.Fatalf("blocker[%d] bead_id differs: %s vs %s", i, r1.Blockers[i].BeadID, r2.Blockers[i].BeadID)
		}
	}
}

func TestFR2_Check_CyclicModuleDependency(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// A requires B, B requires A.
	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	spec.modules["modB"] = ModuleInfo{
		ID:             2,
		Name:           "modB",
		RequiresModule: []int{1},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hb1",
		BeadStatus: "closed",
	})
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "ha1",
	})

	spec.hashes["modA/component/1"] = "ha1"

	_, err := Check(context.Background(), store, spec, "bead-a1")
	if err == nil {
		t.Fatal("want error for cyclic dependency, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("error should mention cycle, got: %v", err)
	}
}

func TestFR2_Check_Ready_NoDependencies(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	spec.modules["mod"] = ModuleInfo{
		ID:   1,
		Name: "mod",
		Components: []ComponentInfo{
			{ID: 1, Name: "Comp1"},
		},
	}

	store.Create(Record{
		SpecNodeID: "mod/component/1",
		BeadID:     "bead-1",
		Module:     "mod",
		Component:  "Comp1",
		SpecHash:   "h1",
	})

	spec.hashes["mod/component/1"] = "h1"

	result, err := Check(context.Background(), store, spec, "bead-1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "ready" {
		t.Fatalf("status: want ready, got %s", result.Status)
	}
}

func TestFR2_Check_Blocked_DependencyNoMappingRecord(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// Module B has a component with no mapping record at all.
	spec.modules["modB"] = ModuleInfo{
		ID:   2,
		Name: "modB",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}

	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "ha1",
	})

	spec.hashes["modA/component/1"] = "ha1"

	result, err := Check(context.Background(), store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status: want blocked, got %s", result.Status)
	}
	if len(result.Blockers) != 1 {
		t.Fatalf("blockers count: want 1, got %d", len(result.Blockers))
	}
	if !strings.Contains(result.Blockers[0].Reason, "no mapping record") {
		t.Fatalf("reason: want 'no mapping record', got %q", result.Blockers[0].Reason)
	}
}

func TestFR2_Check_DiamondDependency(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// Diamond: A→B→D, A→C→D. D is a shared dependency (not a cycle).
	spec.modules["modD"] = ModuleInfo{
		ID:   4,
		Name: "modD",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompD1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modD/component/1",
		BeadID:     "bead-d1",
		Module:     "modD",
		Component:  "CompD1",
		SpecHash:   "hd1",
		BeadStatus: "closed",
	})

	spec.modules["modB"] = ModuleInfo{
		ID:             2,
		Name:           "modB",
		RequiresModule: []int{4},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hb1",
		BeadStatus: "closed",
	})

	spec.modules["modC"] = ModuleInfo{
		ID:             3,
		Name:           "modC",
		RequiresModule: []int{4},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompC1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modC/component/1",
		BeadID:     "bead-c1",
		Module:     "modC",
		Component:  "CompC1",
		SpecHash:   "hc1",
		BeadStatus: "closed",
	})

	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2, 3},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "ha1",
	})

	spec.hashes["modA/component/1"] = "ha1"

	result, err := Check(context.Background(), store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check: %v (diamond dependency should not be a cycle)", err)
	}
	if result.Status != "ready" {
		t.Fatalf("status: want ready, got %s (blockers: %v)", result.Status, result.Blockers)
	}
}

func TestFR2_Check_TransitiveDependency(t *testing.T) {
	store := newStubStore()
	spec := newStubSpecGraph()

	// A requires B, B requires C. C has an open bead.
	spec.modules["modC"] = ModuleInfo{
		ID:   3,
		Name: "modC",
		Components: []ComponentInfo{
			{ID: 1, Name: "CompC1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modC/component/1",
		BeadID:     "bead-c1",
		Module:     "modC",
		Component:  "CompC1",
		SpecHash:   "hc1",
		BeadStatus: "open",
	})

	spec.modules["modB"] = ModuleInfo{
		ID:             2,
		Name:           "modB",
		RequiresModule: []int{3},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompB1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modB/component/1",
		BeadID:     "bead-b1",
		Module:     "modB",
		Component:  "CompB1",
		SpecHash:   "hb1",
		BeadStatus: "closed",
	})

	spec.modules["modA"] = ModuleInfo{
		ID:             1,
		Name:           "modA",
		RequiresModule: []int{2},
		Components: []ComponentInfo{
			{ID: 1, Name: "CompA1"},
		},
	}
	store.Create(Record{
		SpecNodeID: "modA/component/1",
		BeadID:     "bead-a1",
		Module:     "modA",
		Component:  "CompA1",
		SpecHash:   "ha1",
	})

	spec.hashes["modA/component/1"] = "ha1"

	result, err := Check(context.Background(), store, spec, "bead-a1")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status: want blocked, got %s", result.Status)
	}
	if len(result.Blockers) != 1 {
		t.Fatalf("blockers count: want 1, got %d", len(result.Blockers))
	}
	if result.Blockers[0].SpecNodeID != "modC/component/1" {
		t.Fatalf("blocker: want modC/component/1, got %s", result.Blockers[0].SpecNodeID)
	}
}
