package plan

import (
	"strings"
	"testing"
)

// defaultPlanRelevant mirrors schema.DefaultProfile().PlanRelevant
// ([]string{"data_flow", "component", "test_section"}) without importing
// schema into this test file: TopologicalSorter only ever consumes the
// ordered list of names, never the Profile type itself.
var defaultPlanRelevant = []string{"data_flow", "component", "test_section"}

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

func mustSort(t *testing.T, actions []Action, planRelevant []string) ([]OrderedOp, map[string]string) {
	t.Helper()
	ops, m, err := Sort(actions, planRelevant)
	if err != nil {
		t.Fatalf("Sort: unexpected error: %v", err)
	}
	return ops, m
}

// TestSort_EmptyBatch matches "empty action batch" edge case: no error,
// no ops, no map entries.
func TestSort_EmptyBatch(t *testing.T) {
	ops, m := mustSort(t, nil, defaultPlanRelevant)
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
	ops, m := mustSort(t, []Action{b, a}, defaultPlanRelevant)
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
	ops, _ := mustSort(t, []Action{c, b, a}, defaultPlanRelevant)
	got := specIDs(ops)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("order: got %v want [a b c]", got)
	}
}

// TestSort_LexTiebreakAmongIndependentNodes: with no dependency relation,
// same-layer nodes emit in spec_node_id lex order.
func TestSort_LexTiebreakAmongIndependentNodes(t *testing.T) {
	actions := []Action{
		{Type: ActionCreate, SpecNodeID: "zzz", NodeType: KindComponent},
		{Type: ActionCreate, SpecNodeID: "aaa", NodeType: KindComponent},
		{Type: ActionCreate, SpecNodeID: "mmm", NodeType: KindComponent},
	}
	ops, _ := mustSort(t, actions, defaultPlanRelevant)
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
	ops, _ := mustSort(t, []Action{c, b, a}, defaultPlanRelevant)
	got := specIDs(ops)
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("order: got %v want [a b c]", got)
	}
}

// TestSort_LayerOrderOverridesDependency: under the default profile, the
// proposal epic always emits first and the test_section layer always
// emits last, regardless of any lex ordering that would otherwise
// interleave them — layer order comes from planRelevant, not spec_node_id.
func TestSort_LayerOrderOverridesDependency(t *testing.T) {
	epic := Action{Type: ActionCreate, SpecNodeID: "z-epic", NodeType: KindProposalEpic}
	test := Action{Type: ActionCreate, SpecNodeID: "a-test", NodeType: KindTestSection}
	comp := Action{Type: ActionCreate, SpecNodeID: "m-comp", NodeType: KindComponent}
	flow := Action{Type: ActionCreate, SpecNodeID: "b-flow", NodeType: KindDataFlow}
	ops, _ := mustSort(t, []Action{test, comp, flow, epic}, defaultPlanRelevant)
	got := specIDs(ops)
	want := []string{"z-epic", "b-flow", "m-comp", "a-test"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v", got, want)
		}
	}
}

// TestSort_DataFlowAndComponentSitInSeparateLayers: under the default
// profile, data_flow and component no longer share a layer (the retired
// fixed-tier scheme's behavior) — data_flow occupies its own, earlier
// layer. A component's in-batch dep on a data_flow is satisfied by layer
// order alone: the edge is invisible to Kahn (cross-layer), but the layers
// already place the data_flow first.
func TestSort_DataFlowAndComponentSitInSeparateLayers(t *testing.T) {
	flow := Action{Type: ActionCreate, SpecNodeID: "flow", NodeType: KindDataFlow}
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"flow"}}
	ops, _ := mustSort(t, []Action{comp, flow}, defaultPlanRelevant)
	got := specIDs(ops)
	if got[0] != "flow" || got[1] != "comp" {
		t.Fatalf("order: got %v want [flow comp]", got)
	}
}

// TestSort_CustomProfileOrderReordersLayers: a profile listing "component"
// before "data_flow" places the component layer first — the layer order is
// entirely planRelevant's to decide, not a compiled-in constant.
func TestSort_CustomProfileOrderReordersLayers(t *testing.T) {
	flow := Action{Type: ActionCreate, SpecNodeID: "flow", NodeType: KindDataFlow}
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent}
	ops, _ := mustSort(t, []Action{flow, comp}, []string{"component", "data_flow", "test_section"})
	got := specIDs(ops)
	if got[0] != "comp" || got[1] != "flow" {
		t.Fatalf("order: got %v want [comp flow]", got)
	}
}

// TestSort_DepOutsideLayerIsInvisible: a dep naming a spec_node_id in an
// earlier layer is invisible to the sort (left for Resolver to classify) —
// it neither errors nor affects ordering within the layer.
func TestSort_DepOutsideLayerIsInvisible(t *testing.T) {
	epic := Action{Type: ActionCreate, SpecNodeID: "epic", NodeType: KindProposalEpic}
	// comp declares a dep on the epic's spec_node_id, which sits in a
	// different (already-earlier) layer, so the edge is invisible here.
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"epic"}}
	ops, _ := mustSort(t, []Action{comp, epic}, defaultPlanRelevant)
	got := specIDs(ops)
	if got[0] != "epic" || got[1] != "comp" {
		t.Fatalf("order: got %v want [epic comp]", got)
	}
}

// TestSort_DepOutsideBatchIsInvisible: a dep naming a spec_node_id that
// never appears in the batch at all is likewise invisible to the sort.
func TestSort_DepOutsideBatchIsInvisible(t *testing.T) {
	comp := Action{Type: ActionCreate, SpecNodeID: "comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"not-in-batch"}}
	ops, m := mustSort(t, []Action{comp}, defaultPlanRelevant)
	if len(ops) != 1 || ops[0].Action.SpecNodeID != "comp" {
		t.Fatalf("ops: got %+v", ops)
	}
	if _, ok := m["not-in-batch"]; ok {
		t.Fatalf("map must not carry an entry for a node outside the batch: %+v", m)
	}
}

// TestSort_DepPointingAtLaterLayerErrors: under a profile ordering
// component before data_flow, a component depending on a data_flow points
// at a later layer's op — refused naming both spec_node_ids, never a
// silently reordered file (arch_topological_sorter.md, "Algorithm", step 2).
func TestSort_DepPointingAtLaterLayerErrors(t *testing.T) {
	comp := Action{Type: ActionCreate, SpecNodeID: "a-comp", NodeType: KindComponent, DepSpecNodeIDs: []string{"f-flow"}}
	flow := Action{Type: ActionCreate, SpecNodeID: "f-flow", NodeType: KindDataFlow}
	ops, m, err := Sort([]Action{comp, flow}, []string{"component", "data_flow", "test_section"})
	if err == nil {
		t.Fatalf("Sort: want error for forward-layer dep, got ops=%+v map=%+v", ops, m)
	}
	if ops != nil || m != nil {
		t.Fatalf("Sort: want no ordering on forward-layer error, got ops=%+v map=%+v", ops, m)
	}
	if !strings.Contains(err.Error(), "a-comp") || !strings.Contains(err.Error(), "f-flow") {
		t.Fatalf("forward-layer error must name both nodes: %v", err)
	}
}

// TestSort_CleanupLayerIsLast: a cleanup create (discriminated by the
// "Code cleanup:" reason prefix) is placed after every plan-relevant
// layer, including test_section — the last layer, by rule.
func TestSort_CleanupLayerIsLast(t *testing.T) {
	test := Action{Type: ActionCreate, SpecNodeID: "t1", NodeType: KindTestSection}
	comp := Action{Type: ActionCreate, SpecNodeID: "c1", NodeType: KindComponent}
	cleanup := Action{Type: ActionCreate, SpecNodeID: "x1", NodeType: KindComponent, Reason: "Code cleanup: m/X"}
	ops, _ := mustSort(t, []Action{cleanup, test, comp}, defaultPlanRelevant)
	got := specIDs(ops)
	want := []string{"c1", "t1", "x1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order: got %v want %v (cleanup must be last)", got, want)
		}
	}
}

// TestSort_CleanupPlacementIgnoresItsOwnNodeType: a cleanup action carries
// the removed node's own NodeType (here "component"), but that never
// places it in the component layer — the "Code cleanup:" reason prefix
// decides, not NodeType.
func TestSort_CleanupPlacementIgnoresItsOwnNodeType(t *testing.T) {
	comp := Action{Type: ActionCreate, SpecNodeID: "live-comp", NodeType: KindComponent}
	cleanup := Action{Type: ActionCreate, SpecNodeID: "dead-comp", NodeType: KindComponent, Reason: "Code cleanup: m/Dead"}
	ops, _ := mustSort(t, []Action{cleanup, comp}, defaultPlanRelevant)
	got := specIDs(ops)
	if got[0] != "live-comp" || got[1] != "dead-comp" {
		t.Fatalf("order: got %v want [live-comp dead-comp] (cleanup after the component layer)", got)
	}
}

// TestSort_OpIDsNumberedFromOneInEmittedOrder pins the provisional op_id
// scheme: "op-<n>", numbered from 1 in emitted order.
func TestSort_OpIDsNumberedFromOneInEmittedOrder(t *testing.T) {
	actions := []Action{
		{Type: ActionCreate, SpecNodeID: "b", NodeType: KindComponent, DepSpecNodeIDs: []string{"a"}},
		{Type: ActionCreate, SpecNodeID: "a", NodeType: KindComponent},
	}
	ops, m := mustSort(t, actions, defaultPlanRelevant)
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
	ops, m, err := Sort([]Action{a, b}, defaultPlanRelevant)
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
	_, _, err := Sort([]Action{a, b, c}, defaultPlanRelevant)
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
	_, _, err := Sort([]Action{z, a}, defaultPlanRelevant)
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
	_, _, err := Sort([]Action{a}, defaultPlanRelevant)
	if err == nil {
		t.Fatalf("Sort: want error for self-dependency")
	}
	if !strings.Contains(err.Error(), "a") {
		t.Fatalf("self-dependency error must name the node: %v", err)
	}
}

// TestSort_UnplacedKindErrors: a create whose spec node kind is placed by
// none of the three rules (epic, cleanup, planRelevant) is refused with no
// ordering returned at all.
func TestSort_UnplacedKindErrors(t *testing.T) {
	bad := Action{Type: ActionCreate, SpecNodeID: "x", NodeType: "endpoint"}
	good := Action{Type: ActionCreate, SpecNodeID: "y", NodeType: KindComponent}
	ops, m, err := Sort([]Action{good, bad}, defaultPlanRelevant)
	if err == nil {
		t.Fatalf("Sort: want error for unplaced node kind, got ops=%+v map=%+v", ops, m)
	}
	if ops != nil || m != nil {
		t.Fatalf("Sort: want no ordering on unplaced-kind error, got ops=%+v map=%+v", ops, m)
	}
	if !strings.Contains(err.Error(), "x") {
		t.Fatalf("unplaced-kind error must name the offending node: %v", err)
	}
}

// TestSort_ProfileDeclaredKindOutsideDefaultVocabularyIsPlaced: a profile
// that appends a node type ("endpoint") the default profile never declared
// is honored — the sorter consults planRelevant, never a compiled-in
// vocabulary, so the new kind is placed in its own layer, last among the
// non-cleanup layers.
func TestSort_ProfileDeclaredKindOutsideDefaultVocabularyIsPlaced(t *testing.T) {
	test := Action{Type: ActionCreate, SpecNodeID: "t1", NodeType: KindTestSection}
	endpoint := Action{Type: ActionCreate, SpecNodeID: "e1", NodeType: "endpoint"}
	ops, _ := mustSort(t, []Action{endpoint, test}, []string{"data_flow", "component", "test_section", "endpoint"})
	got := specIDs(ops)
	if got[0] != "t1" || got[1] != "e1" {
		t.Fatalf("order: got %v want [t1 e1]", got)
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
	ops1, m1 := mustSort(t, actions, defaultPlanRelevant)
	ops2, m2 := mustSort(t, actions, defaultPlanRelevant)
	if len(opIDs(ops1)) != len(opIDs(ops2)) {
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
