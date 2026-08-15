package plan

import (
	"strings"
	"testing"
)

func opIDs(ops []OrderedOp) []string {
	ids := make([]string, len(ops))
	for i, o := range ops {
		ids[i] = o.OpID
	}
	return ids
}

func specIDs(ops []OrderedOp) []string {
	ids := make([]string, len(ops))
	for i, o := range ops {
		ids[i] = o.Action.SpecNodeID
	}
	return ids
}

func mustSort(t *testing.T, actions []Action) ([]OrderedOp, map[string]string) {
	t.Helper()
	ops, m, err := Sort(actions)
	if err != nil {
		t.Fatalf("Sort: unexpected error: %v", err)
	}
	return ops, m
}

// TestSort_EmptyBatch matches "empty action batch" edge case: no error,
// no ops, no map entries.
func TestSort_EmptyBatch(t *testing.T) {
	ops, m := mustSort(t, nil)
	if len(ops) != 0 {
		t.Fatalf("ops: got %d want 0", len(ops))
	}
	if len(m) != 0 {
		t.Fatalf("map: got %d entries want 0", len(m))
	}
}

// TestSort_InBatchDepOrdersBefore pins the algorithm's core rule: if
// create op B declares a dep on the spec_node_id of op A, A comes before
// B (arch_topological_sorter.md, "Ordering Rules", rule 2).
func TestSort_InBatchDepOrdersBefore(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent}
	b := Action{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	ops, m := mustSort(t, []Action{b, a})
	got := specIDs(ops)
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("order: got %v want [a b]", got)
	}
	if m["a"] != ops[0].OpID || m["b"] != ops[1].OpID {
		t.Fatalf("spec_node_id-to-op_id map wrong: %+v", m)
	}
}

// TestSort_ChainOrdersTransitively covers a three-deep chain: C depends on
// B, B depends on A.
func TestSort_ChainOrdersTransitively(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent}
	b := Action{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	c := Action{Type: ActionCreate, SpecNodeID: "c", NodeType: KindComponent, DepSpecNodeIDs: []string{"b"}}
	ops, _ := mustSort(t, []Action{c, b, a})
	got := specIDs(ops)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("order: got %v want [a b c]", got)
	}
}

// TestSort_LexTiebreakAmongIndependentNodes: with no dependency relation,
// same-tier nodes emit in spec_node_id lex order.
func TestSort_LexTiebreakAmongIndependentNodes(t *testing.T) {
	actions := []Action{
		{Type: ActionCreate, SpecNodeID: "zzz", NodeType: KindComponent},
		{Type: ActionCreate, SpecNodeID: "aaa", NodeType: KindComponent},
		{Type: ActionCreate, SpecNodeID: "mmm", NodeType: KindComponent},
	}
	ops, _ := mustSort(t, actions)
	got := specIDs(ops)
	if got[0] != "aaa" || got[1] != "mmm" || got[2] != "zzz" {
		t.Fatalf("order: got %v want [aaa mmm zzz]", got)
	}
}

// TestSort_TiebreakRespectsNewlyReadyNodes: B and C both depend on A; once
// A is emitted, both B and C become ready in the same step. Lex order
// still decides, not discovery order.
func TestSort_TiebreakRespectsNewlyReadyNodes(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent}
	c := Action{Type: ActionCreate, SpecNodeID: "c", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	b := Action{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	ops, _ := mustSort(t, []Action{c, b, a})
	got := specIDs(ops)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("order: got %v want [a b c]", got)
	}
}

// TestSort_TierOrderingOverridesDependency: the proposal epic tier always
// emits first, and the multi-component test tier always emits last,
// regardless of any lex ordering that would otherwise interleave them.
func TestSort_TierOrderingOverridesDependency(t *testing.T) {
	epic := Action{Type: ActionCreate, SpecNodeID: "z-epic", NodeType: KindProposalEpic}
	test := Action{Type: ActionCreate, SpecNodeID: "a-test", NodeType: KindTestSection}
	comp := Action{Type: ActionCreate, SpecNodeID: "m-comp", NodeType: KindComponent}
	flow := Action{Type: ActionCreate, SpecNodeID: "b-flow", NodeType: KindDataFlow}
	ops, _ := mustSort(t, []Action{test, comp, flow, epic})
	got := specIDs(ops)
	want := []string{"z-epic", "b-flow", "m-comp", "a-test"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v", got, want)
		}
	}
}

// TestSort_ComponentAndDataFlowShareATier: components and data_flows sit
// in the same tier, so an in-batch dep between them still orders within
// that shared tier rather than being invisible across a tier boundary.
func TestSort_ComponentAndDataFlowShareATier(t *testing.T) {
	flow := Action{Type: ActionCreate, SpecNodeID: "flow", NodeType: KindDataFlow}
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"flow"}}
	ops, _ := mustSort(t, []Action{comp, flow})
	got := specIDs(ops)
	if got[0] != "flow" || got[1] != "comp" {
		t.Fatalf("order: got %v want [flow comp]", got)
	}
}

// TestSort_DepOutsideTierIsInvisible: a dep naming a spec_node_id in a
// different tier is invisible to the sort (left for Resolver to
// classify) — it neither errors nor affects ordering within the tier.
func TestSort_DepOutsideTierIsInvisible(t *testing.T) {
	epic := Action{Type: ActionCreate, SpecNodeID: "epic", NodeType: KindProposalEpic}
	// comp declares a dep on the epic's spec_node_id, which sits in a
	// different (already-earlier) tier, so the edge is invisible here.
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"epic"}}
	ops, _ := mustSort(t, []Action{comp, epic})
	got := specIDs(ops)
	if got[0] != "epic" || got[1] != "comp" {
		t.Fatalf("order: got %v want [epic comp]", got)
	}
}

// TestSort_DepOutsideBatchIsInvisible: a dep naming a spec_node_id that
// never appears in the batch at all is likewise invisible to the sort.
func TestSort_DepOutsideBatchIsInvisible(t *testing.T) {
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"not-in-batch"}}
	ops, m := mustSort(t, []Action{comp})
	if len(ops) != 1 || ops[0].Action.SpecNodeID != "comp" {
		t.Fatalf("ops: got %+v", ops)
	}
	if _, ok := m["not-in-batch"]; ok {
		t.Fatalf("map must not carry an entry for a node outside the batch: %+v", m)
	}
}

// TestSort_OpIDsNumberedFromOneInEmittedOrder pins the provisional op_id
// scheme: "op-<n>", numbered from 1 in emitted order.
func TestSort_OpIDsNumberedFromOneInEmittedOrder(t *testing.T) {
	actions := []Action{
		{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}},
		{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent},
	}
	ops, m := mustSort(t, actions)
	got := opIDs(ops)
	if got[0] != "op-1" || got[1] != "op-2" {
		t.Fatalf("op ids: got %v want [op-1 op-2]", got)
	}
	if m["a"] != "op-1" || m["b"] != "op-2" {
		t.Fatalf("map: got %+v", m)
	}
}

// TestSort_CycleDetected: a two-node in-batch cycle is refused, naming
// both spec_node_ids.
func TestSort_CycleDetected(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent, DepSpecNodeIDs: []string{"b"}}
	b := Action{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	ops, m, err := Sort([]Action{a, b})
	if err == nil {
		t.Fatalf("Sort: want error for cycle, got ops=%+v map=%+v", ops, m)
	}
	if ops != nil || m != nil {
		t.Fatalf("Sort: want no ordering on cycle error, got ops=%+v map=%+v", ops, m)
	}
	if !strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("cycle error must name both nodes: %v", err)
	}
}

// TestSort_CycleErrorIncludesStrandedNode: a node depending on a cycle
// member, but not itself part of the cycle, never reaches zero incoming
// edges either and must be named alongside the cycle.
func TestSort_CycleErrorIncludesStrandedNode(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent, DepSpecNodeIDs: []string{"b"}}
	b := Action{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	c := Action{Type: ActionCreate, SpecNodeID: "c", NodeType: KindComponent, DepSpecNodeIDs: []string{"b"}}
	_, _, err := Sort([]Action{a, b, c})
	if err == nil {
		t.Fatalf("Sort: want error for cycle")
	}
	for _, want := range []string{"a", "b", "c"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cycle error must name stranded node %q: %v", want, err)
		}
	}
}

// TestSort_CycleErrorMessageIsLexOrdered pins the deterministic message
// requirement: nodes are listed in lex order so the same input always
// produces the same message.
func TestSort_CycleErrorMessageIsLexOrdered(t *testing.T) {
	z := Action{Type: ActionCreate, SpecNodeID: "z", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent, DepSpecNodeIDs: []string{"z"}}
	_, _, err := Sort([]Action{z, a})
	if err == nil {
		t.Fatalf("Sort: want error for cycle")
	}
	if strings.Index(err.Error(), "a") > strings.Index(err.Error(), "z") {
		t.Fatalf("cycle error must list nodes in lex order: %v", err)
	}
}

// TestSort_SelfDependencyIsACycle: an action depending on its own
// spec_node_id can never reach zero incoming edges and is reported as a
// cycle of one.
func TestSort_SelfDependencyIsACycle(t *testing.T) {
	a := Action{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}}
	_, _, err := Sort([]Action{a})
	if err == nil {
		t.Fatalf("Sort: want error for self-dependency")
	}
	if !strings.Contains(err.Error(), "a") {
		t.Fatalf("self-dependency error must name the node: %v", err)
	}
}

// TestSort_UnknownNodeKindErrors: a create whose spec node kind belongs
// to no tier is refused with no ordering returned at all.
func TestSort_UnknownNodeKindErrors(t *testing.T) {
	bad := Action{Type: ActionCreate, SpecNodeID: "x", NodeType: KindCleanup}
	good := Action{Type: ActionCreate, SpecNodeID: "y", NodeType: KindComponent}
	ops, m, err := Sort([]Action{good, bad})
	if err == nil {
		t.Fatalf("Sort: want error for untiered node kind, got ops=%+v map=%+v", ops, m)
	}
	if ops != nil || m != nil {
		t.Fatalf("Sort: want no ordering on tier error, got ops=%+v map=%+v", ops, m)
	}
	if !strings.Contains(err.Error(), "x") {
		t.Fatalf("tier error must name the offending node: %v", err)
	}
}

// TestSort_Deterministic: the same batch sorted twice produces the same
// order and the same op_id map every time.
func TestSort_Deterministic(t *testing.T) {
	actions := []Action{
		{Type: ActionCreate, SpecNodeID: "epic", NodeType: KindProposalEpic},
		{Type: ActionCreate, SpecNodeID: "flow", NodeType: KindDataFlow},
		{Type: ActionCreate, SpecNodeID: "comp-b", NodeType: KindComponent, DepSpecNodeIDs: []string{"comp-a"}},
		{Type: ActionCreate, SpecNodeID: "comp-a", NodeType: KindComponent},
		{Type: ActionCreate, SpecNodeID: "test", NodeType: KindTestSection},
	}
	ops1, m1 := mustSort(t, actions)
	ops2, m2 := mustSort(t, actions)
	if strings := opIDs(ops1); len(strings) != len(opIDs(ops2)) {
		t.Fatalf("op id count differs across runs")
	}
	for i := range ops1 {
		if ops1[i].OpID != ops2[i].OpID || ops1[i].Action.SpecNodeID != ops2[i].Action.SpecNodeID {
			t.Fatalf("run mismatch at %d: %+v vs %+v", i, ops1[i], ops2[i])
		}
	}
	for k, v := range m1 {
		if m2[k] != v {
			t.Fatalf("map mismatch for %q: %q vs %q", k, v, m2[k])
		}
	}
}
