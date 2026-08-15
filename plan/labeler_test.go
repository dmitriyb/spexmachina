package plan

import (
	"strings"
	"testing"
)

// fakeRemovalFold is a map-backed RemovalLookup stand-in for tests.
type fakeRemovalFold map[string]RemovalEntry

func (f fakeRemovalFold) Removal(specNodeID string) (RemovalEntry, bool) {
	e, ok := f[specNodeID]
	return e, ok
}

// --- LabelFor: node-bearing creates (fresh and modify-pair alike) ---

func TestLabelFor_NodeBearingCreate_DerivesFromGitHeadAndOpID(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "4c1146bb7287",
		SpecHash:   "abc",
	}

	got, err := l.LabelFor(action, "op-2", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-2"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLabelFor_ModifyPairCreate_SameDerivationAsFreshCreate(t *testing.T) {
	// A modify-pair create (OldBeadID set, Reason "modified (new)") carries
	// no shape distinct from a fresh create in the label rule — only
	// cleanup and epic branch differently.
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "4c1146bb7287",
		SpecHash:   "abc",
		OldBeadID:  "spexmachina-abc",
		Reason:     "Spec node modified (new): plan/ChangesetBuilder",
	}

	got, err := l.LabelFor(action, "op-5", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-5"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLabelFor_TwoNodeBearingCreates_DistinctLabels(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	a1 := Action{NodeType: KindComponent, SpecNodeID: "aaa"}
	a2 := Action{NodeType: KindComponent, SpecNodeID: "bbb"}

	l1, err := l.LabelFor(a1, "op-1", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l2, err := l.LabelFor(a2, "op-2", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l1 == l2 {
		t.Fatalf("two creates in the same batch must not share a label, both got %q", l1)
	}
}

func TestLabelFor_NoSpecNodeID_IsError(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{NodeType: KindComponent}

	_, err := l.LabelFor(action, "op-1", Registration{})
	if err == nil {
		t.Fatal("want an error for an action with no spec_node_id to derive a referent from")
	}
}

// --- LabelFor: retarget carries the same node-bearing derivation ---

func TestLabelFor_Retarget_SameDerivationAsCreate(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{
		Type:       ActionRetarget,
		BeadID:     "spexmachina-hun",
		NodeType:   KindComponent,
		SpecNodeID: "9f1578d7af6d",
		SpecHash:   "bbb",
		Reason:     "Spec node modified (retarget): plan/BeadReader",
	}

	got, err := l.LabelFor(action, "op-3", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-3"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLabelFor_TwoRetargets_DistinctLabels(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	a1 := Action{Type: ActionRetarget, NodeType: KindComponent, SpecNodeID: "x", Reason: "Spec node modified (retarget): m/X"}
	a2 := Action{Type: ActionRetarget, NodeType: KindComponent, SpecNodeID: "y", Reason: "Spec node modified (retarget): m/Y"}

	l1, err := l.LabelFor(a1, "op-4", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l2, err := l.LabelFor(a2, "op-6", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l1 == l2 {
		t.Fatalf("two retargets must carry distinct labels (each embeds its own op_id), both got %q", l1)
	}
}

// --- LabelFor: epic ---

func TestLabelFor_Epic_UsesRegistrationEID_NotGitHeadOrOpID(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindProposalEpic,
		SpecNodeID: "2026-08-13-plan-module",
	}
	reg := Registration{EID: "reg-head:2026-08-13-plan-module"}

	got, err := l.LabelFor(action, "op-1", reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:reg-head:2026-08-13-plan-module"
	if got != want {
		t.Fatalf("got %q, want %q (registered event's eid, not the run's own git_head)", got, want)
	}
}

func TestLabelFor_Epic_NoRegistration_IsError(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{Type: ActionCreate, NodeType: KindProposalEpic, SpecNodeID: "proposal-a"}

	_, err := l.LabelFor(action, "op-1", Registration{})
	if err == nil {
		t.Fatal("want an error when the epic's proposal has no registered event")
	}
	if !strings.Contains(err.Error(), "proposal-a") {
		t.Fatalf("error should name the proposal: %v", err)
	}
}

// --- LabelFor: cleanup, same-batch close ---

func TestLabelFor_Cleanup_SameBatchClose_DerivesFromCloseOpID(t *testing.T) {
	l := &Labeler{
		GitHead:    "deadbeef",
		Fold:       fakeRemovalFold{},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-8"},
	}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	got, err := l.LabelFor(action, "op-9", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-8"
	if got != want {
		t.Fatalf("got %q, want %q (the close op's own op_id, not the cleanup create's op-9)", got, want)
	}
}

func TestLabelFor_Cleanup_NoFoldEntry_NoSameBatchClose_IsError(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef", Fold: fakeRemovalFold{}, CloseOpIDs: map[string]string{}}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	_, err := l.LabelFor(action, "op-9", Registration{})
	if err == nil {
		t.Fatal("want an error when the cleanup has neither a fold removal entry nor a same-batch close op")
	}
	if !strings.Contains(err.Error(), "spexmachina-old") {
		t.Fatalf("error should name the old bead id: %v", err)
	}
}

// --- LabelFor: cleanup, prior-batch removal already in the fold ---

func TestLabelFor_Cleanup_PriorBatchRemoval_ReadsFoldEID(t *testing.T) {
	l := &Labeler{
		GitHead: "deadbeef",
		Fold: fakeRemovalFold{
			"abc123def456": {Removed: true, EID: "E1"},
		},
		CloseOpIDs: map[string]string{}, // no same-batch close: it landed last run
	}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	got, err := l.LabelFor(action, "op-9", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:E1"
	if got != want {
		t.Fatalf("got %q, want %q (verbatim from the fold, not re-derived from this run's git_head)", got, want)
	}
}

func TestLabelFor_Cleanup_FoldCheckedBeforeSameBatchClose(t *testing.T) {
	// Even when a same-batch close op exists, a fold removal entry answers
	// first — the same resolution order the reconciler pairs the receipt
	// by, so label and referent stay one fact whichever run the removal
	// actually landed in.
	l := &Labeler{
		GitHead: "deadbeef",
		Fold: fakeRemovalFold{
			"abc123def456": {Removed: true, EID: "E1"},
		},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-8"},
	}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	got, err := l.LabelFor(action, "op-9", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:E1"
	if got != want {
		t.Fatalf("got %q, want %q (fold wins over the same-batch close derivation)", got, want)
	}
}

func TestLabelFor_Cleanup_FoldEntryNotRemoved_FallsBackToSameBatchClose(t *testing.T) {
	// A fold entry that exists but is not (yet) a removal must not be
	// mistaken for one.
	l := &Labeler{
		GitHead: "deadbeef",
		Fold: fakeRemovalFold{
			"abc123def456": {Removed: false},
		},
		CloseOpIDs: map[string]string{"spexmachina-old": "op-8"},
	}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	got, err := l.LabelFor(action, "op-9", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-8"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLabelFor_Cleanup_NilFold_FallsBackToSameBatchClose(t *testing.T) {
	l := &Labeler{
		GitHead:    "deadbeef",
		Fold:       nil,
		CloseOpIDs: map[string]string{"spexmachina-old": "op-8"},
	}
	action := Action{
		Type:       ActionCreate,
		NodeType:   KindComponent,
		SpecNodeID: "abc123def456",
		OldBeadID:  "spexmachina-old",
		Reason:     "Code cleanup: m/X",
	}

	got, err := l.LabelFor(action, "op-9", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "spex:deadbeef:op-8"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// --- isCleanup discriminator ---

func TestIsCleanup_ReasonPrefixDiscriminates(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{"cleanup prefix", "Code cleanup: m/X", true},
		{"fresh create", "New spec node: m/X", false},
		{"modify-pair create", "Spec node modified (new): m/X", false},
		{"retarget", "Spec node modified (retarget): m/X", false},
		{"empty", "", false},
		{"short string shorter than prefix", "Code", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCleanup(Action{Reason: tt.reason})
			if got != tt.want {
				t.Fatalf("isCleanup(%q) = %v, want %v", tt.reason, got, tt.want)
			}
		})
	}
}

// --- Determinism: same inputs, same label, independent of call order ---

func TestLabelFor_DeterministicAcrossCalls(t *testing.T) {
	l := &Labeler{GitHead: "deadbeef"}
	action := Action{NodeType: KindComponent, SpecNodeID: "4c1146bb7287"}

	first, err := l.LabelFor(action, "op-2", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := l.LabelFor(action, "op-2", Registration{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Fatalf("LabelFor must be a pure function: got %q then %q", first, second)
	}
}

// --- RemovalLookup: map-backed fake honors the interface ---

func TestFakeRemovalFold_ImplementsRemovalLookup(t *testing.T) {
	var _ RemovalLookup = fakeRemovalFold{}
}
