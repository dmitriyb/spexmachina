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
	next   int
	err    error
	byBead map[string]mapping.Record
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
func (s *stubStore) GetByBead(beadID string) (mapping.Record, error) {
	if rec, ok := s.byBead[beadID]; ok {
		return rec, nil
	}
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

// TestLabelForFreshAdvancesCursor covers the fresh-create branch of the
// per-action rules in arch_idempotency_labeler.md: a create with no
// OldBeadID and no cleanup Reason gets spex:<cursor> and advances the
// cursor by one.
func TestLabelForFreshAdvancesCursor(t *testing.T) {
	store := &stubStore{next: 100}
	l := &Labeler{MappingStore: store}

	first, err := l.LabelFor(CreateAction{SpecNodeID: "node_a"})
	if err != nil {
		t.Fatalf("LabelFor fresh #1: unexpected error: %v", err)
	}
	if first != "spex:100" {
		t.Errorf("fresh label #1: want spex:100, got %q", first)
	}

	second, err := l.LabelFor(CreateAction{SpecNodeID: "node_b"})
	if err != nil {
		t.Fatalf("LabelFor fresh #2: unexpected error: %v", err)
	}
	if second != "spex:101" {
		t.Errorf("fresh label #2: want spex:101, got %q", second)
	}

	next, err := l.NextLabel()
	if err != nil {
		t.Fatalf("NextLabel: unexpected error: %v", err)
	}
	if next != 102 {
		t.Errorf("cursor after two fresh creates from 100: want 102, got %d", next)
	}
}

// TestLabelForCleanupDoesNotAdvanceCursor covers the cleanup branch: a
// create whose Reason starts "Code cleanup:" gets the per-spec-node label
// spex:cleanup-<SpecNodeID> and leaves the cursor untouched.
func TestLabelForCleanupDoesNotAdvanceCursor(t *testing.T) {
	store := &stubStore{next: 50}
	l := &Labeler{MappingStore: store}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	})
	if err != nil {
		t.Fatalf("LabelFor cleanup: unexpected error: %v", err)
	}
	if label != "spex:cleanup-abc123def456" {
		t.Errorf("cleanup label: want spex:cleanup-abc123def456, got %q", label)
	}

	next, err := l.NextLabel()
	if err != nil {
		t.Fatalf("NextLabel: unexpected error: %v", err)
	}
	if next != 50 {
		t.Errorf("cursor after cleanup create: want 50 (unchanged), got %d", next)
	}
}

// TestLabelForModifyPairReusesRecordID covers the modify-pair branch: a
// create with OldBeadID set (and not a cleanup) reuses the existing
// record's id via MappingStore.GetByBead and does NOT advance the cursor.
// This is the record-id-reuse rule that lets the Reconciler hit the
// modify-pair-update branch rather than inserting a parallel record.
func TestLabelForModifyPairReusesRecordID(t *testing.T) {
	store := &stubStore{
		next:   100,
		byBead: map[string]mapping.Record{"spexmachina-abc": {ID: 42, BeadID: "spexmachina-abc"}},
	}
	l := &Labeler{MappingStore: store}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "node_q",
		OldBeadID:  "spexmachina-abc",
	})
	if err != nil {
		t.Fatalf("LabelFor modify-pair: unexpected error: %v", err)
	}
	if label != "spex:42" {
		t.Errorf("modify-pair label: want spex:42 (existing record id), got %q", label)
	}

	next, err := l.NextLabel()
	if err != nil {
		t.Fatalf("NextLabel: unexpected error: %v", err)
	}
	if next != 100 {
		t.Errorf("cursor after modify-pair create: want 100 (unchanged), got %d", next)
	}
}

// TestLabelForCleanupTakesPrecedenceOverModifyPair guards the branch order:
// a cleanup action also carries OldBeadID (lineage), so cleanup must be
// checked before modify-pair. Otherwise a cleanup create would get a
// spex:<existing-id> label instead of spex:cleanup-<spec_node_id>.
func TestLabelForCleanupTakesPrecedenceOverModifyPair(t *testing.T) {
	store := &stubStore{
		next:   100,
		byBead: map[string]mapping.Record{"spexmachina-old": {ID: 7, BeadID: "spexmachina-old"}},
	}
	l := &Labeler{MappingStore: store}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	})
	if err != nil {
		t.Fatalf("LabelFor cleanup-with-oldbead: unexpected error: %v", err)
	}
	if label != "spex:cleanup-abc123def456" {
		t.Errorf("cleanup-with-oldbead label: want spex:cleanup-abc123def456, got %q", label)
	}
}

// TestLabelForModifyPairSurfacesLookupError ensures a failed GetByBead
// lookup propagates as an error rather than silently producing a label
// from a zero or fallback record id.
func TestLabelForModifyPairSurfacesLookupError(t *testing.T) {
	store := &stubStore{next: 100} // byBead nil → GetByBead returns its error
	l := &Labeler{MappingStore: store}

	_, err := l.LabelFor(CreateAction{
		SpecNodeID: "node_q",
		OldBeadID:  "spexmachina-missing",
	})
	if err == nil {
		t.Fatal("LabelFor modify-pair with failing lookup: want error, got nil")
	}
}

// TestLabelForSurfacesInitError ensures a store counter read failure on the
// first LabelFor (which triggers lazy cursor init) propagates as an error.
func TestLabelForSurfacesInitError(t *testing.T) {
	store := &stubStore{err: fmt.Errorf("boom")}
	l := &Labeler{MappingStore: store}

	_, err := l.LabelFor(CreateAction{SpecNodeID: "node_a"})
	if err == nil {
		t.Fatal("LabelFor with failing store init: want error, got nil")
	}
}
