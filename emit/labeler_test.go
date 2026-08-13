package emit

import "testing"

// labelerFold is a minimal JournalFold double for Labeler's unit tests —
// same shape as resolver_test.go's fakeFold, kept local so labeler_test.go
// stays self-contained.
type labelerFold map[string]FoldEntry

func (f labelerFold) Entry(key string) (FoldEntry, bool) {
	e, ok := f[key]
	return e, ok
}

// TestLabelForNodeBearingUsesGitHeadAndOwnOpID covers the node-bearing
// branch of arch_idempotency_labeler.md's per-action rules: a fresh create
// (no OldBeadID) gets spex:<git_head>:<its own op_id> — the eid of the
// change event ingest will mint, derived from (git_head, op_id) with no
// fold or registration lookup involved.
func TestLabelForNodeBearingUsesGitHeadAndOwnOpID(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}

	label, err := l.LabelFor(CreateAction{SpecNodeID: "node_a"}, "op-3", Registration{})
	if err != nil {
		t.Fatalf("LabelFor fresh: unexpected error: %v", err)
	}
	if label != "spex:deadbeef:op-3" {
		t.Errorf("fresh label: want spex:deadbeef:op-3, got %q", label)
	}
}

// TestLabelForModifyPairUsesOwnOpIDNotOldBeadID covers the other half of
// the node-bearing branch: a modify-pair create (OldBeadID set, not a
// cleanup) also gets spex:<git_head>:<its own op_id> — the replacement's
// own change event, distinct from whatever label the closed predecessor
// carried, with no lookup against OldBeadID or the fold at all.
func TestLabelForModifyPairUsesOwnOpIDNotOldBeadID(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef", Fold: labelerFold{}}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "node_q",
		OldBeadID:  "spexmachina-abc",
	}, "op-2", Registration{})
	if err != nil {
		t.Fatalf("LabelFor modify-pair: unexpected error: %v", err)
	}
	if label != "spex:deadbeef:op-2" {
		t.Errorf("modify-pair label: want spex:deadbeef:op-2 (own op_id, no lookup), got %q", label)
	}
}

// TestLabelForCleanupUsesSameBatchCloseOpID covers the cleanup branch's
// primary case: the removal lands in this same batch, so the cleanup's
// referent is the event the same-batch close op implies — eid derived from
// that close op's (git_head, op_id), read via CloseOpIDs keyed on the
// cleanup's OldBeadID. The fold carries no removed event yet, matching a
// same-run removal.
func TestLabelForCleanupUsesSameBatchCloseOpID(t *testing.T) {
	l := &Labeler{
		GitHead:    "deadbeef",
		Fold:       labelerFold{},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-5"},
	}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, "op-9", Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup: unexpected error: %v", err)
	}
	if label != "spex:deadbeef:op-5" {
		t.Errorf("cleanup label: want spex:deadbeef:op-5 (same-batch close op's id, not the cleanup's own op-9), got %q", label)
	}
}

// TestLabelForCleanupUsesFoldRemovedEventForPriorBatchRemoval covers the
// cleanup branch's other case: the removal already landed in an earlier
// run, so the journal fold carries the removed event, and its eid is the
// referent — not derived from any op in this batch (there is no same-batch
// close op for it).
func TestLabelForCleanupUsesFoldRemovedEventForPriorBatchRemoval(t *testing.T) {
	l := &Labeler{
		GitHead: "deadbeef",
		Fold: labelerFold{
			"abc123def456": {Removed: true, RemovedEID: "E1"},
		},
		CloseOpIDs: map[string]string{},
	}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, "op-2", Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup (prior-batch removal): unexpected error: %v", err)
	}
	if label != "spex:E1" {
		t.Errorf("cleanup label: want spex:E1 (fold's removed event, not derived from any op in this batch), got %q", label)
	}
}

// TestLabelForCleanupFoldTakesPrecedenceOverCloseOp covers the documented
// resolution order: the fold is checked first, so a removal already
// recorded there wins even if a same-batch close op for the same OldBeadID
// is also present (e.g. a re-run whose earlier attempt landed the close
// but errored before ingest recorded it — a receipt guards that recovery,
// but the fold winning here keeps label and reconciler pairing on the same
// resolution order regardless).
func TestLabelForCleanupFoldTakesPrecedenceOverCloseOp(t *testing.T) {
	l := &Labeler{
		GitHead: "deadbeef",
		Fold: labelerFold{
			"abc123def456": {Removed: true, RemovedEID: "E1"},
		},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-5"},
	}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, "op-9", Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup: unexpected error: %v", err)
	}
	if label != "spex:E1" {
		t.Errorf("cleanup label: want spex:E1 (fold wins over same-batch close op), got %q", label)
	}
}

// TestLabelForCleanupNoReferentIsError covers the case where a cleanup
// action can be labeled deterministically by neither route: no removed
// event in the fold and no same-batch close op for its OldBeadID.
func TestLabelForCleanupNoReferentIsError(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef", Fold: labelerFold{}, CloseOpIDs: map[string]string{}}

	_, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, "op-9", Registration{})
	if err == nil {
		t.Fatal("LabelFor cleanup with no referent: want error, got nil")
	}
}

// TestLabelForCleanupTakesPrecedenceOverModifyPair guards the branch
// order: a cleanup action also carries OldBeadID (lineage), so cleanup
// must be checked before the plain node-bearing case — otherwise a cleanup
// create would get the node-bearing spex:<git_head>:<own op_id> label
// instead of keying off its removal referent.
func TestLabelForCleanupTakesPrecedenceOverModifyPair(t *testing.T) {
	l := &Labeler{
		GitHead:    "deadbeef",
		Fold:       labelerFold{},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-5"},
	}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, "op-9", Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup-with-oldbead: unexpected error: %v", err)
	}
	if label != "spex:deadbeef:op-5" {
		t.Errorf("cleanup-with-oldbead label: want spex:deadbeef:op-5 (cleanup branch, not own op-9), got %q", label)
	}
}

// TestLabelForEpicUsesRegistrationEID covers the epic branch: an epic
// action's label is spex:<eid> of the run's registration — not derived
// from GitHead or the action's own op_id — because the fold only carries
// the epic once its task exists, so the registration is the only referent
// available at label time.
func TestLabelForEpicUsesRegistrationEID(t *testing.T) {
	l := &Labeler{GitHead: "unrelated-head"}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "2026-04-18-decouple-spex-from-br",
		NodeType:   "proposal",
	}, "op-1", Registration{EID: "deadbeef:2026-04-18-decouple-spex-from-br", OK: true})
	if err != nil {
		t.Fatalf("LabelFor epic: unexpected error: %v", err)
	}
	const want = "spex:deadbeef:2026-04-18-decouple-spex-from-br"
	if label != want {
		t.Errorf("epic label: want %s, got %q", want, label)
	}
}

// TestLabelForEpicNoRegistrationIsError covers the interface contract: an
// epic action for a proposal with no registration in the journal is an
// error — the fix is `spex register`, not a guessed label. This is the
// same verdict Resolver's missing-parent error reads.
func TestLabelForEpicNoRegistrationIsError(t *testing.T) {
	l := &Labeler{}

	_, err := l.LabelFor(CreateAction{
		SpecNodeID: "2026-04-18-decouple-spex-from-br",
		NodeType:   "proposal",
	}, "op-1", Registration{})
	if err == nil {
		t.Fatal("LabelFor epic with no registration: want error, got nil")
	}
}

// TestLabelForMissingSpecNodeIDIsError covers the interface contract: an
// action too malformed to read a spec_node_id from is an error, not a guess.
func TestLabelForMissingSpecNodeIDIsError(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}

	_, err := l.LabelFor(CreateAction{}, "op-1", Registration{})
	if err == nil {
		t.Fatal("LabelFor with empty SpecNodeID: want error, got nil")
	}
}

// TestLabelForIsPureFunctionOfAction guards the per-action independence
// rule: two independent Labeler instances given the same action and op_id
// produce the same label, and an unrelated interleaved call never shifts a
// later answer — no cursor, no shared state.
func TestLabelForIsPureFunctionOfAction(t *testing.T) {
	action := CreateAction{SpecNodeID: "node_a"}

	l1 := &Labeler{GitHead: "deadbeef"}
	l2 := &Labeler{GitHead: "deadbeef"}

	first, err := l1.LabelFor(action, "op-1", Registration{})
	if err != nil {
		t.Fatalf("LabelFor l1: unexpected error: %v", err)
	}
	// Interleave an unrelated call on l1 before asking l2 for the same
	// action — a stateful implementation would let this shift l2's answer.
	if _, err := l1.LabelFor(CreateAction{SpecNodeID: "node_b"}, "op-2", Registration{}); err != nil {
		t.Fatalf("LabelFor l1 (unrelated): unexpected error: %v", err)
	}
	second, err := l2.LabelFor(action, "op-1", Registration{})
	if err != nil {
		t.Fatalf("LabelFor l2: unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("LabelFor not pure: l1 gave %q, l2 gave %q for the same action and op_id", first, second)
	}
}
