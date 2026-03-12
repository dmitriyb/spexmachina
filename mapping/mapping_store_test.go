package mapping

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".bead-map.json")
	return NewFileStore(path)
}

func testRecord() Record {
	return Record{
		SpecNodeID:  "schema/component/1",
		BeadID:      "abc-123",
		Module:      "schema",
		Component:   "ProjectSchema",
		ContentFile: "spec/schema/arch_project_schema.md",
		SpecHash:    "e3b0c44",
	}
}

func TestFR1_Create_AssignsSequentialID(t *testing.T) {
	s := testStore(t)

	r1 := testRecord()
	id1, err := s.Create(r1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id1 != 1 {
		t.Fatalf("first ID: want 1, got %d", id1)
	}

	r2 := Record{
		SpecNodeID:  "schema/component/2",
		BeadID:      "abc-456",
		Module:      "schema",
		Component:   "ModuleSchema",
		ContentFile: "spec/schema/arch_module_schema.md",
		SpecHash:    "d4e5f6",
	}
	id2, err := s.Create(r2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id2 != 2 {
		t.Fatalf("second ID: want 2, got %d", id2)
	}
}

func TestFR1_Create_WritesValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bead-map.json")
	s := NewFileStore(path)

	r := testRecord()
	_, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var mf mapFile
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("parse file: %v", err)
	}

	if mf.NextID != 2 {
		t.Fatalf("next_id: want 2, got %d", mf.NextID)
	}
	if len(mf.Records) != 1 {
		t.Fatalf("records count: want 1, got %d", len(mf.Records))
	}
	if mf.Records[0].BeadID != "abc-123" {
		t.Fatalf("bead_id: want abc-123, got %s", mf.Records[0].BeadID)
	}
}

func TestFR1_Get_ByID(t *testing.T) {
	s := testStore(t)

	r := testRecord()
	id, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != id {
		t.Fatalf("ID: want %d, got %d", id, got.ID)
	}
	if got.BeadID != r.BeadID {
		t.Fatalf("BeadID: want %s, got %s", r.BeadID, got.BeadID)
	}
	if got.SpecNodeID != r.SpecNodeID {
		t.Fatalf("SpecNodeID: want %s, got %s", r.SpecNodeID, got.SpecNodeID)
	}
	if got.Module != r.Module {
		t.Fatalf("Module: want %s, got %s", r.Module, got.Module)
	}
	if got.Component != r.Component {
		t.Fatalf("Component: want %s, got %s", r.Component, got.Component)
	}
	if got.ContentFile != r.ContentFile {
		t.Fatalf("ContentFile: want %s, got %s", r.ContentFile, got.ContentFile)
	}
	if got.SpecHash != r.SpecHash {
		t.Fatalf("SpecHash: want %s, got %s", r.SpecHash, got.SpecHash)
	}
}

func TestFR1_Get_NotFound(t *testing.T) {
	s := testStore(t)

	_, err := s.Get(999)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR1_GetByBead(t *testing.T) {
	s := testStore(t)

	r := testRecord()
	id, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetByBead("abc-123")
	if err != nil {
		t.Fatalf("GetByBead: %v", err)
	}
	if got.ID != id {
		t.Fatalf("ID: want %d, got %d", id, got.ID)
	}
}

func TestFR1_GetByBead_NotFound(t *testing.T) {
	s := testStore(t)

	_, err := s.GetByBead("nonexistent")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR1_GetBySpecNode(t *testing.T) {
	s := testStore(t)

	r := testRecord()
	id, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.GetBySpecNode("schema/component/1")
	if err != nil {
		t.Fatalf("GetBySpecNode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("count: want 1, got %d", len(got))
	}
	if got[0].ID != id {
		t.Fatalf("ID: want %d, got %d", id, got[0].ID)
	}
}

func TestFR1_GetBySpecNode_NotFound(t *testing.T) {
	s := testStore(t)

	_, err := s.GetBySpecNode("nonexistent/node/1")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR1_GetBySpecNode_MultipleBeads(t *testing.T) {
	s := testStore(t)

	r1 := Record{
		SpecNodeID:  "schema/component/1",
		BeadID:      "bead-old",
		Module:      "schema",
		Component:   "ProjectSchema",
		ContentFile: "spec/schema/arch_project_schema.md",
		SpecHash:    "h1",
		BeadStatus:  "closed",
	}
	r2 := Record{
		SpecNodeID:  "schema/component/1",
		BeadID:      "bead-new",
		Module:      "schema",
		Component:   "ProjectSchema",
		ContentFile: "spec/schema/arch_project_schema.md",
		SpecHash:    "h2",
		BeadStatus:  "open",
	}
	if _, err := s.Create(r1); err != nil {
		t.Fatalf("Create r1: %v", err)
	}
	if _, err := s.Create(r2); err != nil {
		t.Fatalf("Create r2: %v", err)
	}

	got, err := s.GetBySpecNode("schema/component/1")
	if err != nil {
		t.Fatalf("GetBySpecNode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("count: want 2, got %d", len(got))
	}
}

func TestFR1_Update_SpecHash(t *testing.T) {
	s := testStore(t)

	r := testRecord()
	id, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = s.UpdateSpecHash(id, "new-hash")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.SpecHash != "new-hash" {
		t.Fatalf("SpecHash: want new-hash, got %s", got.SpecHash)
	}
	if got.BeadID != r.BeadID {
		t.Fatalf("BeadID changed: want %s, got %s", r.BeadID, got.BeadID)
	}
}

func TestFR1_Update_NotFound(t *testing.T) {
	s := testStore(t)

	err := s.UpdateSpecHash(999, "x")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR1_Delete(t *testing.T) {
	s := testStore(t)

	r := testRecord()
	id, err := s.Create(r)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = s.Delete(id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get(id)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got: %v", err)
	}
}

func TestFR1_Delete_NotFound(t *testing.T) {
	s := testStore(t)

	err := s.Delete(999)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestFR1_Delete_IDsNeverReused(t *testing.T) {
	s := testStore(t)

	r1 := testRecord()
	id1, err := s.Create(r1)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = s.Delete(id1)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	r2 := Record{
		SpecNodeID:  "schema/component/2",
		BeadID:      "abc-456",
		Module:      "schema",
		Component:   "ModuleSchema",
		ContentFile: "spec/schema/arch_module_schema.md",
		SpecHash:    "d4e5f6",
	}
	id2, err := s.Create(r2)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id2 <= id1 {
		t.Fatalf("new ID %d should be greater than deleted ID %d", id2, id1)
	}
}

func TestFR1_List_Sorted(t *testing.T) {
	s := testStore(t)

	records := []Record{
		{SpecNodeID: "a/component/1", BeadID: "b1", Module: "a", Component: "C1", ContentFile: "f1", SpecHash: "h1"},
		{SpecNodeID: "a/component/2", BeadID: "b2", Module: "a", Component: "C2", ContentFile: "f2", SpecHash: "h2"},
		{SpecNodeID: "a/component/3", BeadID: "b3", Module: "a", Component: "C3", ContentFile: "f3", SpecHash: "h3"},
	}
	for _, r := range records {
		if _, err := s.Create(r); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("count: want 3, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i].ID <= list[i-1].ID {
			t.Fatalf("records not sorted: ID %d after %d", list[i].ID, list[i-1].ID)
		}
	}
}

func TestFR1_List_Empty(t *testing.T) {
	s := testStore(t)

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty list, got %d records", len(list))
	}
}

func TestFR1_DuplicateBeadID(t *testing.T) {
	s := testStore(t)

	r1 := testRecord()
	if _, err := s.Create(r1); err != nil {
		t.Fatalf("Create: %v", err)
	}

	r2 := Record{
		SpecNodeID:  "different/component/1",
		BeadID:      "abc-123", // same bead ID
		Module:      "different",
		Component:   "Other",
		ContentFile: "f",
		SpecHash:    "h",
	}
	_, err := s.Create(r2)
	if err == nil {
		t.Fatal("want error for duplicate bead_id, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate bead_id") {
		t.Fatalf("error should mention duplicate bead_id, got: %v", err)
	}
}

func TestFR1_MultipleBeadsPerSpecNode(t *testing.T) {
	s := testStore(t)

	r1 := testRecord()
	if _, err := s.Create(r1); err != nil {
		t.Fatalf("Create: %v", err)
	}

	r2 := Record{
		SpecNodeID:  "schema/component/1", // same spec node ID, different bead
		BeadID:      "different-bead",
		Module:      "schema",
		Component:   "ProjectSchema",
		ContentFile: "f",
		SpecHash:    "h",
	}
	_, err := s.Create(r2)
	if err != nil {
		t.Fatalf("Create with same spec_node_id should succeed: %v", err)
	}
}

func TestFR1_MissingFile_CreatedOnFirstWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bead-map.json")
	s := NewFileStore(path)

	// List before any write returns empty
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want empty list, got %d", len(list))
	}

	// First write creates the file
	r := testRecord()
	if _, err := s.Create(r); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after create: %v", err)
	}
}

func TestFR1_ConcurrentCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bead-map.json")
	s := NewFileStore(path)

	var wg sync.WaitGroup
	errs := make([]error, 10)
	ids := make([]int, 10)

	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := Record{
				SpecNodeID:  strings.Replace("mod/component/X", "X", string(rune('0'+idx)), 1),
				BeadID:      strings.Replace("bead-X", "X", string(rune('0'+idx)), 1),
				Module:      "mod",
				Component:   strings.Replace("CompX", "X", string(rune('0'+idx)), 1),
				ContentFile: "f",
				SpecHash:    "h",
			}
			ids[idx], errs[idx] = s.Create(r)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Create failed: %v", i, err)
		}
	}

	// Verify file is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var mf mapFile
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("invalid JSON after concurrent writes: %v", err)
	}
	if len(mf.Records) != 10 {
		t.Fatalf("records: want 10, got %d", len(mf.Records))
	}

	// All IDs should be unique
	seen := make(map[int]bool)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID: %d", id)
		}
		seen[id] = true
	}
}
