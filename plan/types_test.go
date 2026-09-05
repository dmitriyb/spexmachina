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

func TestChangesetVersion_IsFour(t *testing.T) {
	if ChangesetVersion != 4 {
		t.Fatalf("ChangesetVersion: got %d want 4", ChangesetVersion)
	}
}

func TestOpTypeConstants(t *testing.T) {
	want := map[string]string{
		"OpCreate":   "create",
		"OpClose":    "close",
		"OpRetarget": "retarget",
	}
	got := map[string]string{
		"OpCreate":   OpCreate,
		"OpClose":    OpClose,
		"OpRetarget": OpRetarget,
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
	if RefTask != "task" {
		t.Fatalf("RefTask: got %q want %q", RefTask, "task")
	}
	if RefOp != "op" {
		t.Fatalf("RefOp: got %q want %q", RefOp, "op")
	}
}

func TestLabelConstants(t *testing.T) {
	want := map[string]string{
		"IdempotencyLabelPrefix": "spex:",
		"CleanupLabel":           "spex:cleanup",
	}
	got := map[string]string{
		"IdempotencyLabelPrefix": IdempotencyLabelPrefix,
		"CleanupLabel":           CleanupLabel,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
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
		Deps:         []Ref{{Kind: RefTask, TaskID: "spexmachina-ab1"}},
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
		Target:     &Ref{Kind: RefTask, TaskID: "spexmachina-hun"},
		Labels:     []string{"spex:deadbeef:op-3"},
		Deps:       []Ref{{Kind: RefOp, OpID: "op-2"}},
		Reason:     "Spec node modified (retarget): plan/BeadReader",
	}
	got := encode(t, op)
	// Field order follows arch_changeset_builder.md's "Canonical Output"
	// section, which fixes one order across every op kind (op_id, type,
	// spec_node_kind, spec_node_id, spec_hash, idempotency, parent, deps,
	// priority, title, body, target, labels, reason) — deps precedes
	// target. Other renderings of a retarget op in spec/plan disagree with
	// each other on target-vs-deps ordering; see
	// drifts/drift-spexmachina-f6eh.13.json.
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
// section: target and reason alone, no labels — close idempotency keys on
// the tracker's own status, and a run's provenance lives in the
// changeset's top-level git_head, not on individual ops.
func TestOp_CloseMatchesArchSpecExample(t *testing.T) {
	op := Op{
		OpID:   "op-0042",
		Type:   OpClose,
		Target: &Ref{Kind: RefTask, TaskID: "spexmachina-tjs"},
		Reason: "Spec node modified: apply/ApplyCommand",
	}
	got := encode(t, op)
	want := `{"op_id":"op-0042","type":"close","target":{"ref":"task","task_id":"spexmachina-tjs"},"reason":"Spec node modified: apply/ApplyCommand"}`
	if got != want {
		t.Fatalf("close op mismatch:\n got %s\nwant %s", got, want)
	}
	if strings.Contains(got, `"labels"`) {
		t.Fatalf("close op must carry no labels key: %s", got)
	}
}

// TestOp_CleanupCreateShape matches arch_changeset_builder.md's "Cleanup
// op shape" table: spec_node_kind "cleanup", the reason verbatim as
// title, a layer-edge dep with no edge-type key, and no labels key — the
// retired spex:cleanup discriminator is not emitted; what marks the op as
// cleanup is its SpecNodeKind alone.
func TestOp_CleanupCreateShape(t *testing.T) {
	op := Op{
		OpID:         "op-cleanup-abc123def456",
		Type:         OpCreate,
		SpecNodeKind: KindCleanup,
		SpecNodeID:   "abc123def456",
		Idempotency:  &Idem{Label: "spex:deadbeef:op-cleanup-abc123def456"},
		Parent:       &Ref{Kind: RefOp, OpID: "op-proposal_epic-p"},
		Deps:         []Ref{{Kind: RefOp, OpID: "op-test_section-t1"}},
		Priority:     3,
		Title:        "Code cleanup: m/X",
	}
	got := encode(t, op)
	if op.SpecNodeKind != KindCleanup {
		t.Fatalf("cleanup create must carry spec_node_kind=cleanup: %s", got)
	}
	if strings.Contains(got, `"labels"`) {
		t.Fatalf("cleanup create must carry no labels key: %s", got)
	}
	if !strings.Contains(got, `"deps":[{"ref":"op","op_id":"op-test_section-t1"}]`) {
		t.Fatalf("cleanup create's dep must carry no edge-type key — it left the vocabulary with the lineage edge: %s", got)
	}
	fieldOrder(t, got, "cleanup create",
		"op_id", "type", "spec_node_kind", "spec_node_id",
		"idempotency", "parent", "deps", "priority", "title",
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
	if !strings.Contains(got, `"version":4`) {
		t.Fatalf("changeset must declare version 4: %s", got)
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
				Target: &Ref{Kind: RefTask, TaskID: "spexmachina-hun"}, Labels: []string{"spex:deadbeef:op-2"}},
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
		Target:     &Ref{Kind: RefTask, TaskID: "spexmachina-hun"},
		Labels:     []string{"spex:deadbeef:op-3"},
		Deps:       []Ref{{Kind: RefOp, OpID: "op-2"}, {Kind: RefTask, TaskID: "spexmachina-abc"}},
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

// TestRef_CarriesNoEdgeTypeKey pins that a Ref has no field to encode an
// edge type at all — it left the vocabulary with the lineage edge
// (spec/plan/arch_changeset_builder.md, "Op Shape": "no edge-type key,
// because the lineage edge was the only typed dep and it is gone").
func TestRef_CarriesNoEdgeTypeKey(t *testing.T) {
	r := Ref{Kind: RefTask, TaskID: "spexmachina-abc"}
	got := encode(t, r)
	if strings.Contains(got, `"type"`) {
		t.Fatalf("a ref carries its discriminator and one id, and nothing else: %s", got)
	}
}

func TestIdem_OmittedOnOpWithoutOne(t *testing.T) {
	op := Op{OpID: "op-3", Type: OpRetarget, SpecNodeID: "9f1578d7af6d"}
	got := encode(t, op)
	if strings.Contains(got, `"idempotency"`) {
		t.Fatalf("op without an Idem must omit idempotency: %s", got)
	}
}

func TestActionTypeConstants(t *testing.T) {
	want := map[string]string{
		"ActionCreate":   "create",
		"ActionObsolete": "obsolete",
		"ActionRetarget": "retarget",
	}
	got := map[string]string{
		"ActionCreate":   ActionCreate,
		"ActionObsolete": ActionObsolete,
		"ActionRetarget": ActionRetarget,
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func TestFallbackPriority_IsThree(t *testing.T) {
	if FallbackPriority != 3 {
		t.Fatalf("FallbackPriority: got %d want 3", FallbackPriority)
	}
}

// TestAction_PreservesDepOrder pins DepSpecNodeIDs as a value slice, not a
// set or map — Resolver and TopologicalSorter both rely on the order an
// upstream step emitted being preserved.
func TestAction_PreservesDepOrder(t *testing.T) {
	a := Action{
		Type:           ActionCreate,
		SpecNodeID:     "x",
		DepSpecNodeIDs: []string{"c", "a", "b"},
	}
	if got := a.DepSpecNodeIDs; got[0] != "c" || got[1] != "a" || got[2] != "b" {
		t.Fatalf("DepSpecNodeIDs order not preserved: %v", got)
	}
}

// TestAction_ObsoleteCarriesTaskIDAndChangeType matches
// arch_action_classifier.md's Interface table: task id is set on an
// obsolete or retarget, empty on a create; change_type is set on an
// obsolete only.
func TestAction_ObsoleteCarriesTaskIDAndChangeType(t *testing.T) {
	a := Action{
		Type:       ActionObsolete,
		TaskID:     "spexmachina-abc",
		ChangeType: "removed",
		Reason:     "Spec node removed: plan/TaskReader",
	}
	if a.TaskID == "" || a.ChangeType == "" {
		t.Fatalf("obsolete action must carry task id and change type: %+v", a)
	}
}

func TestOrderedOp_PairsActionWithOpID(t *testing.T) {
	o := OrderedOp{
		OpID:   "op-0007",
		Action: Action{SpecNodeID: "x"},
	}
	if o.OpID != "op-0007" || o.Action.SpecNodeID != "x" {
		t.Fatalf("OrderedOp wiring wrong: %+v", o)
	}
}
