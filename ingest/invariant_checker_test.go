package ingest

import (
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

// Tests for spec/ingest/arch_invariant_checker.md, calling
// InvariantChecker.Check directly rather than through Reconciler.Apply —
// invariants 1 and 2 are this component's own contract, and these tests
// pin it independently of how Reconciler eventually dispatches to it
// (spexmachina-ugrs.5). Scenarios mirror test_consistency_invariants.md's
// "Invariant 1" and "Invariant 2" entries; the integration-level Happy
// Path and invariant-4 (snapshot gate) scenarios run through
// Reconciler.Apply + Saver.Save in consistency_invariants_test.go.

// TestInvariantChecker_Check_HappyPath covers the positive case: a
// well-formed batch of paired events and receipts, referencing only eids
// present in existing-plus-batch, passes both checks cleanly.
func TestInvariantChecker_Check_HappyPath(t *testing.T) {
	existing := []mapping.Event{{Event: "added", EID: "E0"}}
	batch := []mapping.Event{
		{Event: "added", EID: "e1"},
		{Event: "task_created", TaskID: "t1", For: "e1"},
		{Event: "modified", EID: "e2"},
		{Event: "task_retargeted", TaskID: "t2", For: "e2"},
		{Event: "task_closed", TaskID: "t3", For: "E0"},
		{Event: "modified", EID: "e3"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e3"}},
	}
	if err := NewInvariantChecker().Check(existing, batch); err != nil {
		t.Fatalf("Check: unexpected error %v", err)
	}
}

// TestInvariantChecker_Invariant1_DoublePairing_TaskCreated covers
// "Invariant 1: double pairing refused": two task_created receipts
// pairing with the same change event.
func TestInvariantChecker_Invariant1_DoublePairing_TaskCreated(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t1", For: "eid-1"},
		{Event: "task_created", TaskID: "t2", For: "eid-1"},
	}
	err := NewInvariantChecker().Check(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Check: got %v, want invariant 1 error", err)
	}
}

// TestInvariantChecker_Invariant1_DoublePairing_ProposalEpic covers the
// epic analogue: two task_created receipts both claiming the same
// proposal slug.
func TestInvariantChecker_Invariant1_DoublePairing_ProposalEpic(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t1", Proposal: "stem"},
		{Event: "task_created", TaskID: "t2", Proposal: "stem"},
	}
	err := NewInvariantChecker().Check(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Check: got %v, want invariant 1 error", err)
	}
}

// TestInvariantChecker_Invariant1_DoublePairing_Retargeted covers the
// retarget analogue from "Invariant 1: retarget pairing": two
// task_retargeted receipts pairing with the same modified event.
func TestInvariantChecker_Invariant1_DoublePairing_Retargeted(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_retargeted", TaskID: "t1", For: "eid-1"},
		{Event: "task_retargeted", TaskID: "t2", For: "eid-1"},
	}
	err := NewInvariantChecker().Check(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Check: got %v, want invariant 1 error", err)
	}
}

// TestInvariantChecker_Invariant1_SeesAcrossExistingAndBatch proves the
// double-pairing check spans the existing journal and the batch, not
// just the batch in isolation — a second task_created in the batch
// pairing with an eid the existing journal's task_created already
// claimed is refused exactly the same as an in-batch collision.
func TestInvariantChecker_Invariant1_SeesAcrossExistingAndBatch(t *testing.T) {
	existing := []mapping.Event{
		{Event: "added", EID: "E1"},
		{Event: "task_created", TaskID: "t1", For: "E1"},
	}
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t2", For: "E1"},
	}
	err := NewInvariantChecker().Check(existing, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Check: got %v, want invariant 1 error", err)
	}
}

// TestInvariantChecker_Invariant2_DanglingReference_TaskCreated covers
// "Invariant 2: dangling receipt reference".
func TestInvariantChecker_Invariant2_DanglingReference_TaskCreated(t *testing.T) {
	batch := []mapping.Event{{Event: "task_created", TaskID: "t1", For: "missing-eid"}}
	err := NewInvariantChecker().Check(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("Check: got %v, want %q", err, want)
	}
}

// TestInvariantChecker_Invariant2_DanglingReference_Retargeted covers
// the retarget analogue: a task_retargeted receipt's for names an eid
// neither the journal nor the batch contains.
func TestInvariantChecker_Invariant2_DanglingReference_Retargeted(t *testing.T) {
	batch := []mapping.Event{{Event: "task_retargeted", TaskID: "t1", For: "missing-eid"}}
	err := NewInvariantChecker().Check(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("Check: got %v, want %q", err, want)
	}
}

// TestInvariantChecker_Invariant2_DanglingReference_AbsorbedRefresh
// covers "Invariant 1: absorbed batch closes under one refresh
// receipt"'s dangling variant: invariant 2's no-unknown-referent rule
// covers a refresh receipt's absorbed list exactly as it covers for.
func TestInvariantChecker_Invariant2_DanglingReference_AbsorbedRefresh(t *testing.T) {
	batch := []mapping.Event{
		{Event: "modified", EID: "e1"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e1", "missing-eid"}},
	}
	err := NewInvariantChecker().Check(nil, batch)
	want := "ingest: receipt references unknown event missing-eid"
	if err == nil || err.Error() != want {
		t.Fatalf("Check: got %v, want %q", err, want)
	}
}

// TestInvariantChecker_Invariant2_KnownAgainstExisting confirms a
// receipt whose for resolves against the EXISTING journal (not just the
// batch) passes.
func TestInvariantChecker_Invariant2_KnownAgainstExisting(t *testing.T) {
	existing := []mapping.Event{{Event: "added", EID: "E1"}}
	batch := []mapping.Event{{Event: "task_created", TaskID: "t1", For: "E1"}}
	if err := NewInvariantChecker().Check(existing, batch); err != nil {
		t.Fatalf("Check: unexpected error %v", err)
	}
}

// TestInvariantChecker_Check_NumericOrder proves "the checks run in
// numeric order, so the first message a caller sees names the most
// upstream cause": a batch that violates both invariant 1 (double
// pairing) and invariant 2 (a dangling reference elsewhere in the same
// batch) must fail with invariant 1's error, not invariant 2's.
func TestInvariantChecker_Check_NumericOrder(t *testing.T) {
	batch := []mapping.Event{
		{Event: "task_created", TaskID: "t1", For: "eid-1"},
		{Event: "task_created", TaskID: "t2", For: "eid-1"},
		{Event: "task_closed", TaskID: "t3", For: "missing-eid"},
	}
	err := NewInvariantChecker().Check(nil, batch)
	if err == nil || !strings.Contains(err.Error(), "invariant 1") {
		t.Fatalf("Check: got %v, want invariant 1 error to surface before invariant 2's", err)
	}
}
