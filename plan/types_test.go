package plan

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fieldOrder asserts that the named JSON keys appear in body in the given
// order. It tolerates other keys interleaved but rejects any out-of-order
// pair.
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

func TestChangesetVersion_IsThree(t *testing.T) {
	if ChangesetVersion != 3 {
		t.Fatalf("ChangesetVersion: got %d want 3", ChangesetVersion)
	}
}

func TestOpTypeConstants(t *testing.T) {
	want := map[string]string{
		"OpCreate":   "create",
		"OpClose":    "close",
		"OpRetarget": "retarget",
		"OpLabel":    "label",
		"OpTag":      "tag",
	}
	got := map[string]string{
		"OpCreate":   OpCreate,
		"OpClose":    OpClose,
		"OpRetarget": OpRetarget,
		"OpLabel":    OpLabel,
		"OpTag":      OpTag,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func TestSpecNodeKindConstants(t *testing.T) {
	want := map[string]string{
		"KindProposalEpic": "proposal_epic",
		"KindComponent":    "component",
		"KindDataFlow":     "data_flow",
		"KindTestSection":  "test_section",
		"KindCleanup":      "cleanup",
	}
	got := map[string]string{
		"KindProposalEpic": KindProposalEpic,
		"KindComponent":    KindComponent,
		"KindDataFlow":     KindDataFlow,
		"KindTestSection":  KindTestSection,
		"KindCleanup":      KindCleanup,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func TestRefKindConstants(t *testing.T) {
	if RefBead != "bead" {
		t.Fatalf("RefBead: got %q want %q", RefBead, "bead")
	}
	if RefOp != "op" {
		t.Fatalf("RefOp: got %q want %q", RefOp, "op")
	}
}

func TestLabelConstants(t *testing.T) {
	want := map[string]string{
		"IdempotencyLabelPrefix": "spex:",
		"ObsoleteLabel":          "spex:obsolete",
		"CommitLabelPrefix":      "commit:",
		"CleanupLabel":           "spex:cleanup",
	}
	got := map[string]string{
		"IdempotencyLabelPrefix": IdempotencyLabelPrefix,
		"ObsoleteLabel":          ObsoleteLabel,
		"CommitLabelPrefix":      CommitLabelPrefix,
		"CleanupLabel":           CleanupLabel,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
	}
	if !strings.HasPrefix(ObsoleteLabel, IdempotencyLabelPrefix) {
		t.Fatalf("ObsoleteLabel %q must carry the spex: prefix", ObsoleteLabel)
	}
	if !strings.HasPrefix(CleanupLabel, IdempotencyLabelPrefix) {
		t.Fatalf("CleanupLabel %q must carry the spex: prefix", CleanupLabel)
	}
}

// TestOp_CreateCanonicalFieldOrder pins the field order
// test_changeset_builder.md's "Canonical schema and field order" scenario
// names for a create op: op_id, type, spec_node_kind, spec_node_id,
// idempotency, parent, deps, priority, title, body.
func TestOp_CreateCanonicalFieldOrder(t *testing.T) {
	op := Op{
		OpID:         "op-2",
		Type:         OpCreate,
		SpecNodeKind: KindComponent,
		SpecNodeID:   "4c1146bb7287",
		Idempotency:  &Idem{Label: "spex:deadbeef:op-2"},
		Parent:       &Ref{Kind: RefOp, OpID: "op-1"},
		Deps:         []Ref{{Kind: RefBead, BeadID: "spexmachina-ab1"}},
		Priority:     1,
		Title:        "plan: ChangesetBuilder",
		Body:         "spec/plan/arch_changeset_builder.md",
	}
	got := encode(t, op)
	fieldOrder(t, got, "create op",
		"op_id", "type", "spec_node_kind", "spec_node_id",
		"idempotency", "parent", "deps", "priority", "title", "body",
	)
	for _, banned := range []string{`"spec_hash"`, `"target"`, `"labels"`, `"reason"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("conventional create leaked field %s: %s", banned, got)
		}
	}
}

// TestOp_RetargetCanonicalFieldOrder matches arch_changeset_builder.md's
// "Retarget op shape" table: type, target, spec_node_id, spec_hash,
// labels, deps, reason — and no idempotency, parent, priority, title or
// body.
func TestOp_RetargetCanonicalFieldOrder(t *testing.T) {
	op := Op{
		OpID:       "op-3",
		Type:       OpRetarget,
		SpecNodeID: "9f1578d7af6d",
		SpecHash:   "bbb",
		Target:     &Ref{Kind: RefBead, BeadID: "spexmachina-hun"},
		Labels:     []string{"spex:deadbeef:op-3"},
		Deps:       []Ref{{Kind: RefOp, OpID: "op-2"}},
		Reason:     "Spec node modified (retarget): plan/BeadReader",
	}
	got := encode(t, op)
	// Canonical order per arch_changeset_builder.md's "Canonical Output"
	// section is fixed across every op kind (op_id, type, spec_node_kind,
	// spec_node_id, spec_hash, idempotency, parent, deps, priority, title,
	// body, target, labels, reason) — deps precedes target for a retarget
	// op even though the informal "Retarget op shape" table lists target
	// first.
	fieldOrder(t, got, "retarget op",
		"op_id", "type", "spec_node_id", "spec_hash", "deps", "target", "labels", "reason",
	)
	for _, banned := range []string{`"spec_node_kind"`, `"idempotency"`, `"parent"`, `"priority"`, `"title"`, `"body"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("retarget op leaked field %s: %s", banned, got)
		}
	}
}

// TestOp_CloseMatchesArchSpecExample pins byte-identical output against
// the literal close-op example in arch_changeset_builder.md's "Op Shape"
// section.
func TestOp_CloseMatchesArchSpecExample(t *testing.T) {
	op := Op{
		OpID:   "op-0042",
		Type:   OpClose,
		Target: &Ref{Kind: RefBead, BeadID: "spexmachina-tjs"},
		Labels: []string{ObsoleteLabel, CommitLabelPrefix + "deadbeefcafe1234"},
		Reason: "Spec node modified: apply/ApplyCommand",
	}
	got := encode(t, op)
	want := `{"op_id":"op-0042","type":"close","target":{"ref":"bead","bead_id":"spexmachina-tjs"},"labels":["spex:obsolete","commit:deadbeefcafe1234"],"reason":"Spec node modified: apply/ApplyCommand"}`
	if got != want {
		t.Fatalf("close op mismatch:\n got %s\nwant %s", got, want)
	}
}

// TestOp_CleanupCreateShape matches arch_changeset_builder.md's "Cleanup
// op shape" table: spec_node_kind "cleanup", the reason verbatim as
// title, a single blocks-edge dep to the old bead, and the
// "spex:cleanup" discriminator label riding on the create itself (not
// just on closes).
func TestOp_CleanupCreateShape(t *testing.T) {
	op := Op{
		OpID:         "op-9",
		Type:         OpCreate,
		SpecNodeKind: KindCleanup,
		SpecNodeID:   "abc123def456",
		Idempotency:  &Idem{Label: "spex:deadbeef:op-8"},
		Parent:       &Ref{Kind: RefOp, OpID: "op-1"},
		Deps:         []Ref{{Kind: RefBead, BeadID: "spexmachina-old", EdgeType: "blocks"}},
		Priority:     3,
		Title:        "Code cleanup: m/X",
		Labels:       []string{CleanupLabel},
	}
	got := encode(t, op)
	if op.SpecNodeKind != KindCleanup {
		t.Fatalf("cleanup create must carry spec_node_kind=cleanup: %s", got)
	}
	if !strings.Contains(got, `"labels":["spex:cleanup"]`) {
		t.Fatalf("cleanup create must carry the spex:cleanup label: %s", got)
	}
	if !strings.Contains(got, `"type":"blocks"`) {
		t.Fatalf("cleanup create's dep must carry edge type blocks: %s", got)
	}
	fieldOrder(t, got, "cleanup create",
		"op_id", "type", "spec_node_kind", "spec_node_id",
		"idempotency", "parent", "deps", "priority", "title", "labels",
	)
}

func TestAbsorbedEntry_CanonicalFieldOrder(t *testing.T) {
	e := AbsorbedEntry{
		Node:   "972faea162a6",
		Before: "aaa",
		After:  "bbb",
		Reason: "typo sweep, no contract section touched",
	}
	got := encode(t, e)
	fieldOrder(t, got, "absorbed entry", "node", "before", "after", "reason")
}

func TestChangeset_TopLevelCanonicalFieldOrder(t *testing.T) {
	cs := Changeset{
		Version:  ChangesetVersion,
		GitHead:  "deadbeef",
		Proposal: "2026-08-13-plan-module",
		Ops: []Op{
			{OpID: "op-1", Type: OpCreate, SpecNodeKind: KindProposalEpic, SpecNodeID: "2026-08-13-plan-module",
				Idempotency: &Idem{Label: "spex:beef0001:2026-08-13-plan-module"}, Priority: 3, Title: "Proposal: 2026-08-13-plan-module"},
		},
		Absorbed: []AbsorbedEntry{
			{Node: "972faea162a6", Before: "aaa", After: "bbb", Reason: "typo sweep"},
		},
	}
	got := encode(t, cs)
	fieldOrder(t, got, "changeset", "version", "git_head", "proposal", "ops", "absorbed")
	if !strings.Contains(got, `"version":3`) {
		t.Fatalf("changeset must declare version 3: %s", got)
	}
}

func TestChangeset_DeterministicEncoding(t *testing.T) {
	cs := Changeset{
		Version:  ChangesetVersion,
		GitHead:  "deadbeef",
		Proposal: "2026-08-13-plan-module",
		Ops: []Op{
			{OpID: "op-1", Type: OpCreate, SpecNodeKind: KindComponent, SpecNodeID: "4c1146bb7287"},
			{OpID: "op-2", Type: OpRetarget, SpecNodeID: "9f1578d7af6d", SpecHash: "bbb",
				Target: &Ref{Kind: RefBead, BeadID: "spexmachina-hun"}, Labels: []string{"spex:deadbeef:op-2"}},
		},
	}
	first := encode(t, cs)
	second := encode(t, cs)
	if first != second {
		t.Fatalf("encoding is non-deterministic:\n%s\nvs\n%s", first, second)
	}
}

func TestOp_RoundTrip(t *testing.T) {
	original := Op{
		OpID:       "op-3",
		Type:       OpRetarget,
		SpecNodeID: "9f1578d7af6d",
		SpecHash:   "bbb",
		Target:     &Ref{Kind: RefBead, BeadID: "spexmachina-hun"},
		Labels:     []string{"spex:deadbeef:op-3"},
		Deps:       []Ref{{Kind: RefOp, OpID: "op-2"}, {Kind: RefBead, BeadID: "spexmachina-abc", EdgeType: "blocks"}},
		Reason:     "Spec node modified (retarget): plan/BeadReader",
	}
	wire := encode(t, original)
	var got Op
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.OpID != original.OpID || got.Type != original.Type || got.SpecNodeID != original.SpecNodeID ||
		got.SpecHash != original.SpecHash || got.Reason != original.Reason {
		t.Fatalf("scalar field mismatch: got %+v want %+v", got, original)
	}
	if got.Target == nil || *got.Target != *original.Target {
		t.Fatalf("target mismatch: got %+v want %+v", got.Target, original.Target)
	}
	if len(got.Deps) != len(original.Deps) {
		t.Fatalf("deps length: got %d want %d", len(got.Deps), len(original.Deps))
	}
	for i, d := range got.Deps {
		if d != original.Deps[i] {
			t.Fatalf("dep %d mismatch: got %+v want %+v", i, d, original.Deps[i])
		}
	}
}

func TestRef_EdgeTypeOmittedWhenEmpty(t *testing.T) {
	r := Ref{Kind: RefBead, BeadID: "spexmachina-abc"}
	got := encode(t, r)
	if strings.Contains(got, `"type"`) {
		t.Fatalf("ref without an edge type must omit the type key: %s", got)
	}
}

func TestRef_EdgeTypePresentWhenSet(t *testing.T) {
	r := Ref{Kind: RefBead, BeadID: "spexmachina-abc", EdgeType: "blocks"}
	got := encode(t, r)
	if !strings.Contains(got, `"type":"blocks"`) {
		t.Fatalf("ref with an edge type must carry it: %s", got)
	}
}

func TestIdem_OmittedOnOpWithoutOne(t *testing.T) {
	op := Op{OpID: "op-3", Type: OpRetarget, SpecNodeID: "9f1578d7af6d"}
	got := encode(t, op)
	if strings.Contains(got, `"idempotency"`) {
		t.Fatalf("op without an Idem must omit idempotency: %s", got)
	}
}
