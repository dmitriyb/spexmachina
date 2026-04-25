package emit

import (
	"fmt"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// stubStore is a minimal mapping.Store double for Labeler tests. Only
// NextRecordID is exercised; the other methods return errors so any
// accidental dependency on full Store behavior fails loudly.
type stubStore struct {
	next int
	err  error
}

func (s *stubStore) NextRecordID() (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.next, nil
}

func (s *stubStore) Create(mapping.Record) (int, error) {
	return 0, fmt.Errorf("stubStore.Create: not implemented")
}
func (s *stubStore) Get(int) (mapping.Record, error) {
	return mapping.Record{}, fmt.Errorf("stubStore.Get: not implemented")
}
func (s *stubStore) GetByBead(string) (mapping.Record, error) {
	return mapping.Record{}, fmt.Errorf("stubStore.GetByBead: not implemented")
}
func (s *stubStore) GetBySpecNode(string) ([]mapping.Record, error) {
	return nil, fmt.Errorf("stubStore.GetBySpecNode: not implemented")
}
func (s *stubStore) GetByProposalEpic(string) (mapping.Record, error) {
	return mapping.Record{}, fmt.Errorf("stubStore.GetByProposalEpic: not implemented")
}
func (s *stubStore) Update(int, map[string]string) error {
	return fmt.Errorf("stubStore.Update: not implemented")
}
func (s *stubStore) Delete(int) error    { return fmt.Errorf("stubStore.Delete: not implemented") }
func (s *stubStore) List() ([]mapping.Record, error) {
	return nil, fmt.Errorf("stubStore.List: not implemented")
}
func (s *stubStore) Replace([]mapping.Record, int) error {
	return fmt.Errorf("stubStore.Replace: not implemented")
}

// TestReserveMonotonicLabels covers the spec's "Monotonic label assignment"
// scenario: starting counter 42, three creates → spex:42..spex:44 with the
// in-memory cursor advancing to 45.
func TestReserveMonotonicLabels(t *testing.T) {
	store := &stubStore{next: 42}
	l := &Labeler{MappingStore: store}

	got, err := l.Reserve(3)
	if err != nil {
		t.Fatalf("Reserve(3): unexpected error: %v", err)
	}
	want := []string{"spex:42", "spex:43", "spex:44"}
	if len(got) != len(want) {
		t.Fatalf("len(labels): want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d]: want %q, got %q", i, want[i], got[i])
		}
	}

	next, err := l.NextLabel()
	if err != nil {
		t.Fatalf("NextLabel: unexpected error: %v", err)
	}
	if next != 45 {
		t.Errorf("cursor after Reserve(3) starting at 42: want 45, got %d", next)
	}
}

// TestReserveZero ensures Reserve(0) returns an empty slice without
// touching the underlying store. Important because ChangesetBuilder may
// call Reserve with the count of creates, which can legitimately be zero.
func TestReserveZero(t *testing.T) {
	store := &stubStore{next: 7}
	l := &Labeler{MappingStore: store}

	got, err := l.Reserve(0)
	if err != nil {
		t.Fatalf("Reserve(0): unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Reserve(0): want empty slice, got %v", got)
	}

	// Cursor should still be initialized to the store's value.
	next, err := l.NextLabel()
	if err != nil {
		t.Fatalf("NextLabel: unexpected error: %v", err)
	}
	if next != 7 {
		t.Errorf("cursor after Reserve(0) with start 7: want 7, got %d", next)
	}
}

// TestReserveContinuesCursor covers "Labels reserved at emit time survive
// to ingest" by exercising the in-memory cursor across multiple Reserve
// calls within one emit run. The persisted counter is read once on the
// first Reserve; subsequent calls continue from the in-memory cursor.
func TestReserveContinuesCursor(t *testing.T) {
	store := &stubStore{next: 100}
	l := &Labeler{MappingStore: store}

	first, err := l.Reserve(2)
	if err != nil {
		t.Fatalf("Reserve(2) #1: %v", err)
	}
	if first[0] != "spex:100" || first[1] != "spex:101" {
		t.Errorf("first batch: want [spex:100 spex:101], got %v", first)
	}

	// Mutate the store under the labeler to prove the in-memory cursor
	// is sticky: the second Reserve must NOT reread the store.
	store.next = 999

	second, err := l.Reserve(1)
	if err != nil {
		t.Fatalf("Reserve(1) #2: %v", err)
	}
	if len(second) != 1 || second[0] != "spex:102" {
		t.Errorf("second batch: want [spex:102], got %v", second)
	}
}

// TestNoLabelReuseAcrossRuns covers the spec's "No label reuse across runs"
// case: a fresh Labeler reads the store's now-advanced counter, so the
// second run's first label is strictly greater than the first run's last.
func TestNoLabelReuseAcrossRuns(t *testing.T) {
	store := &stubStore{next: 42}
	run1 := &Labeler{MappingStore: store}
	got1, err := run1.Reserve(3)
	if err != nil {
		t.Fatalf("run1 Reserve: %v", err)
	}
	last1 := got1[len(got1)-1] // spex:44

	// Simulate ingest committing the run: store counter advances by 3.
	store.next = 45

	run2 := &Labeler{MappingStore: store}
	got2, err := run2.Reserve(1)
	if err != nil {
		t.Fatalf("run2 Reserve: %v", err)
	}
	first2 := got2[0]

	if last1 != "spex:44" {
		t.Fatalf("run1 last label: want spex:44, got %q", last1)
	}
	if first2 != "spex:45" {
		t.Errorf("run2 first label: want spex:45, got %q", first2)
	}
	// Strict-greater check, per the spec language.
	if !(first2 > last1) {
		t.Errorf("run2 first label %q must be > run1 last label %q", first2, last1)
	}
}

// TestReserveSurfacesStoreError ensures a store load failure on the first
// Reserve propagates as an error rather than silently producing labels
// from a zero counter.
func TestReserveSurfacesStoreError(t *testing.T) {
	store := &stubStore{err: fmt.Errorf("boom")}
	l := &Labeler{MappingStore: store}

	_, err := l.Reserve(1)
	if err == nil {
		t.Fatal("Reserve: want error from failing store, got nil")
	}
}

// TestReserveNegative rejects nonsensical input rather than silently
// producing an empty or misordered batch.
func TestReserveNegative(t *testing.T) {
	store := &stubStore{next: 1}
	l := &Labeler{MappingStore: store}

	_, err := l.Reserve(-1)
	if err == nil {
		t.Fatal("Reserve(-1): want error, got nil")
	}
}
