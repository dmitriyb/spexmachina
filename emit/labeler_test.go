package emit

import "testing"

// TestLabelForNodeBearingUsesSpecNodeID covers the node-bearing branch of
// arch_idempotency_labeler.md's per-action rules: a fresh create (no
// OldBeadID) gets spex:<spec_node_id> — a pure read off the action, no
// store or cursor involved.
func TestLabelForNodeBearingUsesSpecNodeID(t *testing.T) {
	l := &Labeler{}

	label, err := l.LabelFor(CreateAction{SpecNodeID: "node_a"}, Registration{})
	if err != nil {
		t.Fatalf("LabelFor fresh: unexpected error: %v", err)
	}
	if label != "spex:node_a" {
		t.Errorf("fresh label: want spex:node_a, got %q", label)
	}
}

// TestLabelForModifyPairUsesOwnSpecNodeID covers the other half of the
// node-bearing branch: a modify-pair create (OldBeadID set, not a cleanup)
// gets spex:<spec_node_id> too — the node's identity hash is unchanged
// across the pair, so the replacement create carries the same label as the
// original by construction, with no lookup against OldBeadID at all.
func TestLabelForModifyPairUsesOwnSpecNodeID(t *testing.T) {
	l := &Labeler{}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "node_q",
		OldBeadID:  "spexmachina-abc",
	}, Registration{})
	if err != nil {
		t.Fatalf("LabelFor modify-pair: unexpected error: %v", err)
	}
	if label != "spex:node_q" {
		t.Errorf("modify-pair label: want spex:node_q (own spec_node_id, no lookup), got %q", label)
	}
}

// TestLabelForCleanupUsesCleanupPrefix covers the cleanup branch: a create
// whose Reason starts "Code cleanup:" gets spex:cleanup-<spec_node_id>,
// keyed by the removed node's identity hash so it never collides with the
// node's own ordinary-task label.
func TestLabelForCleanupUsesCleanupPrefix(t *testing.T) {
	l := &Labeler{}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup: unexpected error: %v", err)
	}
	if label != "spex:cleanup-abc123def456" {
		t.Errorf("cleanup label: want spex:cleanup-abc123def456, got %q", label)
	}
}

// TestLabelForCleanupTakesPrecedenceOverModifyPair guards the branch order:
// a cleanup action also carries OldBeadID (lineage), so cleanup must be
// checked before the plain node-bearing case. Otherwise a cleanup create
// would get spex:<spec_node_id> instead of spex:cleanup-<spec_node_id>.
func TestLabelForCleanupTakesPrecedenceOverModifyPair(t *testing.T) {
	l := &Labeler{}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}, Registration{})
	if err != nil {
		t.Fatalf("LabelFor cleanup-with-oldbead: unexpected error: %v", err)
	}
	if label != "spex:cleanup-abc123def456" {
		t.Errorf("cleanup-with-oldbead label: want spex:cleanup-abc123def456, got %q", label)
	}
}

// TestLabelForEpicUsesRegistrationEID covers the epic branch: an epic
// action's label is spex:<eid> of the run's registration — not the action's
// own SpecNodeID (the bare proposal slug) — because the fold only carries
// the epic once its task exists, so the registration is the only referent
// available at label time.
func TestLabelForEpicUsesRegistrationEID(t *testing.T) {
	l := &Labeler{}

	label, err := l.LabelFor(CreateAction{
		SpecNodeID: "2026-04-18-decouple-spex-from-br",
		NodeType:   "proposal",
	}, Registration{EID: "deadbeef:2026-04-18-decouple-spex-from-br", OK: true})
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
	}, Registration{})
	if err == nil {
		t.Fatal("LabelFor epic with no registration: want error, got nil")
	}
}

// TestLabelForMissingSpecNodeIDIsError covers the interface contract: an
// action too malformed to read a spec_node_id from is an error, not a guess.
func TestLabelForMissingSpecNodeIDIsError(t *testing.T) {
	l := &Labeler{}

	_, err := l.LabelFor(CreateAction{}, Registration{})
	if err == nil {
		t.Fatal("LabelFor with empty SpecNodeID: want error, got nil")
	}
}

// TestLabelForIsPureFunctionOfAction guards the per-action, per-batch
// independence rule: two independent Labeler instances given the same
// action produce the same label, and repeated calls on the same instance
// for different actions never influence one another (no cursor, no shared
// state).
func TestLabelForIsPureFunctionOfAction(t *testing.T) {
	action := CreateAction{SpecNodeID: "node_a"}

	l1 := &Labeler{}
	l2 := &Labeler{}

	first, err := l1.LabelFor(action, Registration{})
	if err != nil {
		t.Fatalf("LabelFor l1: unexpected error: %v", err)
	}
	// Interleave an unrelated call on l1 before asking l2 for the same
	// action — a stateful implementation would let this shift l2's answer.
	if _, err := l1.LabelFor(CreateAction{SpecNodeID: "node_b"}, Registration{}); err != nil {
		t.Fatalf("LabelFor l1 (unrelated): unexpected error: %v", err)
	}
	second, err := l2.LabelFor(action, Registration{})
	if err != nil {
		t.Fatalf("LabelFor l2: unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("LabelFor not pure: l1 gave %q, l2 gave %q for the same action", first, second)
	}
}
