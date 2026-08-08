package emit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fieldOrder asserts that the named JSON keys appear in body in the given
// order. It tolerates other keys interleaved (e.g. unrelated fields) but
// rejects any out-of-order pair.
func fieldOrder(t *testing.T, body, label string, keys ...string) {
	t.Helper()
	last := -1
	for _, k := range keys {
		needle := `"` + k + `":`
		idx := strings.Index(body, needle)
		if idx < 0 {
			t.Fatalf("%s: missing key %q in %s", label, k, body)
		}
		if idx <= last {
			t.Fatalf("%s: key %q at %d is not after previous key (idx %d) in %s",
				label, k, idx, last, body)
		}
		last = idx
	}
}

func encode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func TestRef_BeadShape(t *testing.T) {
	r := Ref{Kind: RefBead, BeadID: "spexmachina-abc"}
	got := encode(t, r)
	want := `{"ref":"bead","bead_id":"spexmachina-abc"}`
	if got != want {
		t.Fatalf("bead ref: got %s want %s", got, want)
	}
}

func TestRef_OpShape(t *testing.T) {
	r := Ref{Kind: RefOp, OpID: "op-0003"}
	got := encode(t, r)
	want := `{"ref":"op","op_id":"op-0003"}`
	if got != want {
		t.Fatalf("op ref: got %s want %s", got, want)
	}
}

func TestRef_OmitsUnusedDiscriminators(t *testing.T) {
	r := Ref{Kind: RefBead, BeadID: "br-1"}
	got := encode(t, r)
	for _, banned := range []string{`"op_id"`, `"type"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("bead ref must not contain %s: %s", banned, got)
		}
	}
}

func TestRef_BlocksEdgeType(t *testing.T) {
	// Obsolete+create lineage attaches an edge type to the bead ref.
	r := Ref{Kind: RefBead, BeadID: "spexmachina-old", EdgeType: "blocks"}
	got := encode(t, r)
	want := `{"ref":"bead","bead_id":"spexmachina-old","type":"blocks"}`
	if got != want {
		t.Fatalf("blocks ref: got %s want %s", got, want)
	}
}

func TestOp_CreateCanonicalFieldOrder(t *testing.T) {
	op := Op{
		OpID:         "op-0002",
		Type:         OpCreate,
		SpecNodeKind: "component",
		SpecNodeID:   "7f06f7d80e94",
		Idempotency:  &Idem{Label: "spex:143"},
		Parent:       &Ref{Kind: RefOp, OpID: "op-0001"},
		Deps:         []Ref{{Kind: RefOp, OpID: "op-0003"}},
		Priority:     1,
		Title:        "emit: ChangesetBuilder",
		Body:         "Composes changeset.json …",
	}
	got := encode(t, op)
	fieldOrder(t, got, "create op",
		"op_id", "type", "spec_node_kind", "spec_node_id",
		"idempotency", "parent", "deps", "priority", "title", "body",
	)
	for _, banned := range []string{`"target"`, `"labels"`, `"reason"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("create op leaked close-only field %s: %s", banned, got)
		}
	}
}

func TestOp_CloseCanonicalFieldOrder(t *testing.T) {
	op := Op{
		OpID:   "op-0042",
		Type:   OpClose,
		Target: &Ref{Kind: RefBead, BeadID: "spexmachina-tjs"},
		Labels: []string{"spex:obsolete", "commit:deadbeefcafe1234"},
		Reason: "Spec node modified: apply/ApplyCommand",
	}
	got := encode(t, op)
	fieldOrder(t, got, "close op",
		"op_id", "type", "target", "labels", "reason",
	)
	for _, banned := range []string{
		`"spec_node_kind"`, `"spec_node_id"`, `"idempotency"`,
		`"parent"`, `"deps"`, `"priority"`, `"title"`, `"body"`,
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("close op leaked create-only field %s: %s", banned, got)
		}
	}
}

func TestOp_PriorityOmittedWhenZero(t *testing.T) {
	// Close ops have no priority. Priority 3 (fallback) and above are valid;
	// 0 should never reach the wire and is treated as "absent" by omitempty.
	op := Op{OpID: "op-0042", Type: OpClose, Target: &Ref{Kind: RefBead, BeadID: "x"}}
	got := encode(t, op)
	if strings.Contains(got, `"priority"`) {
		t.Fatalf("close op must not emit priority field: %s", got)
	}
}

func TestChangeset_V2Schema(t *testing.T) {
	cs := Changeset{
		Version:  ChangesetVersion,
		GitHead:  "deadbeefcafe1234",
		Proposal: "2026-04-18-decouple-spex-from-br",
		Ops: []Op{
			{
				OpID:         "op-0001",
				Type:         OpCreate,
				SpecNodeKind: "proposal_epic",
				Idempotency:  &Idem{Label: "spex:142"},
				Priority:     3,
				Title:        "Proposal: 2026-04-18-decouple-spex-from-br",
			},
		},
	}
	got := encode(t, cs)
	if !strings.Contains(got, `"version":2`) {
		t.Fatalf("changeset must declare version 2: %s", got)
	}
	fieldOrder(t, got, "changeset", "version", "git_head", "proposal", "ops")
}

func TestChangeset_VersionConstantIsTwo(t *testing.T) {
	if ChangesetVersion != 2 {
		t.Fatalf("ChangesetVersion: got %d want 2", ChangesetVersion)
	}
}

func TestChangeset_DeterministicEncoding(t *testing.T) {
	cs := Changeset{
		Version:  ChangesetVersion,
		GitHead:  "deadbeefcafe1234",
		Proposal: "2026-04-18-decouple-spex-from-br",
		Ops: []Op{
			{OpID: "op-0001", Type: OpCreate, SpecNodeID: "abc", Title: "x"},
			{OpID: "op-0002", Type: OpCreate, SpecNodeID: "def", Title: "y"},
		},
	}
	first := encode(t, cs)
	second := encode(t, cs)
	if first != second {
		t.Fatalf("encoding is non-deterministic:\n%s\nvs\n%s", first, second)
	}
}

func TestOpKindConstants(t *testing.T) {
	cases := map[string]string{
		"OpCreate": OpCreate,
		"OpClose":  OpClose,
		"OpLabel":  OpLabel,
		"OpTag":    OpTag,
	}
	want := map[string]string{
		"OpCreate": "create",
		"OpClose":  "close",
		"OpLabel":  "label",
		"OpTag":    "tag",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s: got %q want %q", k, v, want[k])
		}
	}
}

func TestRefKindConstants(t *testing.T) {
	cases := map[string]string{
		"RefBead": RefBead,
		"RefOp":   RefOp,
	}
	want := map[string]string{
		"RefBead": "bead",
		"RefOp":   "op",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s: got %q want %q", k, v, want[k])
		}
	}
}

func TestTierConstantsOrdered(t *testing.T) {
	if !(TierProposalEpic < TierFeatureOrFlow && TierFeatureOrFlow < TierMultiCompTest) {
		t.Fatalf("tiers must be strictly ascending: epic=%d feature=%d test=%d",
			TierProposalEpic, TierFeatureOrFlow, TierMultiCompTest)
	}
	if TierProposalEpic != 0 {
		t.Fatalf("TierProposalEpic must be 0 (first tier emitted), got %d", TierProposalEpic)
	}
}

func TestFallbackPriorityIsMidRange(t *testing.T) {
	if FallbackPriority != 3 {
		t.Fatalf("FallbackPriority: got %d want 3", FallbackPriority)
	}
}

func TestCreateAction_PreservesDepOrder(t *testing.T) {
	// Resolver and Sorter both rely on DepSpecNodeIDs being preserved as
	// emitted by impact — this test pins the slice as a value type rather
	// than a set or map.
	a := CreateAction{
		SpecNodeID:     "x",
		DepSpecNodeIDs: []string{"c", "a", "b"},
	}
	if got := a.DepSpecNodeIDs; got[0] != "c" || got[1] != "a" || got[2] != "b" {
		t.Fatalf("DepSpecNodeIDs order not preserved: %v", got)
	}
}

func TestOrderedOp_PairsActionWithOpID(t *testing.T) {
	o := OrderedOp{
		OpID:   "op-0007",
		Action: CreateAction{SpecNodeID: "x"},
	}
	if o.OpID != "op-0007" || o.Action.SpecNodeID != "x" {
		t.Fatalf("OrderedOp wiring wrong: %+v", o)
	}
}
