package emit

import (
	"strings"
	"testing"
)

// orderOf returns a map from spec_node_id to its position (0-based) in the
// ordered op slice. Helper for asserting relative ordering without pinning
// exact indices.
func orderOf(ordered []OrderedOp) map[string]int {
	m := map[string]int{}
	for i, o := range ordered {
		m[o.Action.SpecNodeID] = i
	}
	return m
}

func TestSorter_LinearChain(t *testing.T) {
	// C depends on B; B depends on A. Expected order: A, B, C.
	creates := []CreateAction{
		{SpecNodeID: "A", NodeType: "component"},
		{SpecNodeID: "B", NodeType: "component", DepSpecNodeIDs: []string{"A"}},
		{SpecNodeID: "C", NodeType: "component", DepSpecNodeIDs: []string{"B"}},
	}
	s := &Sorter{}
	ordered, batch, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	if len(ordered) != 3 {
		t.Fatalf("len(ordered)=%d want 3", len(ordered))
	}
	pos := orderOf(ordered)
	if !(pos["A"] < pos["B"] && pos["B"] < pos["C"]) {
		t.Fatalf("linear chain order broken: %+v", pos)
	}
	for id, opID := range batch {
		if opID == "" {
			t.Fatalf("batch map missing op_id for %s", id)
		}
	}
	if batch["A"] != ordered[0].OpID || batch["B"] != ordered[1].OpID || batch["C"] != ordered[2].OpID {
		t.Fatalf("batch map does not match emitted order: batch=%v ordered=%v", batch, ordered)
	}
}

func TestSorter_Diamond(t *testing.T) {
	// A depends on B and C; B and C depend on D. Expected order: D, B, C, A
	// (B before C because lex tiebreak sorts ready set by spec_node_id).
	creates := []CreateAction{
		{SpecNodeID: "A", NodeType: "component", DepSpecNodeIDs: []string{"B", "C"}},
		{SpecNodeID: "B", NodeType: "component", DepSpecNodeIDs: []string{"D"}},
		{SpecNodeID: "C", NodeType: "component", DepSpecNodeIDs: []string{"D"}},
		{SpecNodeID: "D", NodeType: "component"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	pos := orderOf(ordered)
	if !(pos["D"] < pos["B"] && pos["D"] < pos["C"] && pos["B"] < pos["A"] && pos["C"] < pos["A"]) {
		t.Fatalf("diamond constraints violated: %+v", pos)
	}
	if !(pos["B"] < pos["C"]) {
		t.Fatalf("lex tiebreak: B should come before C in ready set, got %+v", pos)
	}
}

func TestSorter_IndependentLexTiebreak(t *testing.T) {
	creates := []CreateAction{
		{SpecNodeID: "003abc", NodeType: "component"},
		{SpecNodeID: "001def", NodeType: "component"},
		{SpecNodeID: "002ghi", NodeType: "component"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	want := []string{"001def", "002ghi", "003abc"}
	for i, w := range want {
		if ordered[i].Action.SpecNodeID != w {
			t.Fatalf("lex tiebreak: at index %d got %s want %s", i, ordered[i].Action.SpecNodeID, w)
		}
	}
}

func TestSorter_CycleDetected(t *testing.T) {
	creates := []CreateAction{
		{SpecNodeID: "A", NodeType: "component", DepSpecNodeIDs: []string{"B"}},
		{SpecNodeID: "B", NodeType: "component", DepSpecNodeIDs: []string{"A"}},
	}
	s := &Sorter{}
	_, _, err := s.Sort(creates)
	if err == nil {
		t.Fatalf("expected cycle error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "A") || !strings.Contains(msg, "B") {
		t.Fatalf("cycle error must name both spec_node_ids in cycle, got: %s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "cycle") {
		t.Fatalf("cycle error message must mention 'cycle', got: %s", msg)
	}
}

func TestSorter_TypeTierRespected(t *testing.T) {
	// One proposal epic, two component features, one multi-component test.
	// Features depend on epic; the test depends on both features.
	// Even though the test's deps run cross-tier (which is allowed but
	// resolved out-of-tier), tier ordering alone must place: epic, features
	// (lex tiebreak: F1 before F2), test.
	creates := []CreateAction{
		{SpecNodeID: "T1", NodeType: "test_section", DepSpecNodeIDs: []string{"F1", "F2"}},
		{SpecNodeID: "F2", NodeType: "component", DepSpecNodeIDs: []string{"E"}},
		{SpecNodeID: "F1", NodeType: "component", DepSpecNodeIDs: []string{"E"}},
		{SpecNodeID: "E", NodeType: "proposal"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	if len(ordered) != 4 {
		t.Fatalf("len(ordered)=%d want 4", len(ordered))
	}
	want := []string{"E", "F1", "F2", "T1"}
	for i, w := range want {
		if ordered[i].Action.SpecNodeID != w {
			t.Fatalf("tier ordering: at index %d got %s want %s (full=%v)",
				i, ordered[i].Action.SpecNodeID, w, ordered)
		}
	}
}

func TestSorter_DataFlowSharesTierWithComponents(t *testing.T) {
	// data_flow nodes belong to tier 1 (alongside components). With no
	// deps, the only order constraint is lex tiebreak across the tier.
	creates := []CreateAction{
		{SpecNodeID: "comp", NodeType: "component"},
		{SpecNodeID: "flow", NodeType: "data_flow"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	// Both are tier 1 — lex puts "comp" before "flow".
	if ordered[0].Action.SpecNodeID != "comp" || ordered[1].Action.SpecNodeID != "flow" {
		t.Fatalf("data_flow + component lex order broken: %v", ordered)
	}
}

func TestSorter_OutOfBatchDepsIgnored(t *testing.T) {
	// A depends on Z which is not in the batch. Sorter must still produce a
	// valid order — Z is the Resolver's problem (ref:bead or ref:spec_node).
	creates := []CreateAction{
		{SpecNodeID: "A", NodeType: "component", DepSpecNodeIDs: []string{"Z"}},
		{SpecNodeID: "B", NodeType: "component"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	if len(ordered) != 2 {
		t.Fatalf("len(ordered)=%d want 2", len(ordered))
	}
	pos := orderOf(ordered)
	// A and B are both effectively dep-free within batch — lex picks A first.
	if pos["A"] != 0 || pos["B"] != 1 {
		t.Fatalf("lex tiebreak with out-of-batch dep stripped: %+v", pos)
	}
}

func TestSorter_CrossTierDepsAreNotErrors(t *testing.T) {
	// A test (tier 2) depends on a component (tier 1) — this is the
	// canonical cross-tier shape. Sorter restricts the DAG per tier so the
	// dep is invisible during in-tier Kahn; the natural tier ordering
	// (component first) ensures correctness anyway.
	creates := []CreateAction{
		{SpecNodeID: "test", NodeType: "test_section", DepSpecNodeIDs: []string{"comp"}},
		{SpecNodeID: "comp", NodeType: "component"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	pos := orderOf(ordered)
	if pos["comp"] >= pos["test"] {
		t.Fatalf("comp must come before test (tier order): %+v", pos)
	}
}

func TestSorter_OpIDPaddingMatchesBatchSize(t *testing.T) {
	// 3 ops → pad=1 → op-1, op-2, op-3.
	creates := []CreateAction{
		{SpecNodeID: "a", NodeType: "component"},
		{SpecNodeID: "b", NodeType: "component"},
		{SpecNodeID: "c", NodeType: "component"},
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	want := []string{"op-1", "op-2", "op-3"}
	for i, w := range want {
		if ordered[i].OpID != w {
			t.Fatalf("op_id pad: at %d got %s want %s", i, ordered[i].OpID, w)
		}
	}
}

func TestSorter_OpIDPaddingTwoDigits(t *testing.T) {
	creates := make([]CreateAction, 12)
	for i := range creates {
		creates[i] = CreateAction{
			SpecNodeID: string(rune('a' + i)),
			NodeType:   "component",
		}
	}
	s := &Sorter{}
	ordered, _, err := s.Sort(creates)
	if err != nil {
		t.Fatalf("Sort error: %v", err)
	}
	if ordered[0].OpID != "op-01" {
		t.Fatalf("12-op batch first id should be op-01, got %s", ordered[0].OpID)
	}
	if ordered[11].OpID != "op-12" {
		t.Fatalf("12-op batch last id should be op-12, got %s", ordered[11].OpID)
	}
}

func TestSorter_EmptyInput(t *testing.T) {
	s := &Sorter{}
	ordered, batch, err := s.Sort(nil)
	if err != nil {
		t.Fatalf("empty input: %v", err)
	}
	if len(ordered) != 0 {
		t.Fatalf("empty input must yield no ops, got %d", len(ordered))
	}
	if len(batch) != 0 {
		t.Fatalf("empty input must yield empty batch map, got %v", batch)
	}
}

func TestSorter_UnknownNodeTypeIsError(t *testing.T) {
	creates := []CreateAction{
		{SpecNodeID: "x", NodeType: "unknown_kind"},
	}
	s := &Sorter{}
	_, _, err := s.Sort(creates)
	if err == nil {
		t.Fatalf("unknown NodeType must produce an error")
	}
	if !strings.Contains(err.Error(), "unknown_kind") {
		t.Fatalf("error must name unknown NodeType, got: %s", err)
	}
}

func TestSorter_DeterministicAcrossRuns(t *testing.T) {
	// Same input → byte-identical OpIDs and order on every run.
	build := func() []CreateAction {
		return []CreateAction{
			{SpecNodeID: "alpha", NodeType: "component"},
			{SpecNodeID: "beta", NodeType: "component", DepSpecNodeIDs: []string{"alpha"}},
			{SpecNodeID: "gamma", NodeType: "component", DepSpecNodeIDs: []string{"alpha"}},
			{SpecNodeID: "delta", NodeType: "component", DepSpecNodeIDs: []string{"beta", "gamma"}},
		}
	}
	s := &Sorter{}
	first, _, err := s.Sort(build())
	if err != nil {
		t.Fatalf("first sort: %v", err)
	}
	for i := 0; i < 50; i++ {
		got, _, err := s.Sort(build())
		if err != nil {
			t.Fatalf("repeat sort: %v", err)
		}
		if len(got) != len(first) {
			t.Fatalf("length drift across runs")
		}
		for j := range got {
			if got[j].OpID != first[j].OpID || got[j].Action.SpecNodeID != first[j].Action.SpecNodeID {
				t.Fatalf("non-deterministic at index %d on run %d: %+v vs %+v",
					j, i, got[j], first[j])
			}
		}
	}
}

func TestSorter_CycleAcrossThreeNodes(t *testing.T) {
	// A→B→C→A — full SCC must be reported.
	creates := []CreateAction{
		{SpecNodeID: "A", NodeType: "component", DepSpecNodeIDs: []string{"C"}},
		{SpecNodeID: "B", NodeType: "component", DepSpecNodeIDs: []string{"A"}},
		{SpecNodeID: "C", NodeType: "component", DepSpecNodeIDs: []string{"B"}},
	}
	s := &Sorter{}
	_, _, err := s.Sort(creates)
	if err == nil {
		t.Fatalf("expected cycle error")
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("cycle error must name %s, got: %s", want, err)
		}
	}
}
