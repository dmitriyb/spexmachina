package emit

import (
	"container/heap"
	"fmt"
	"sort"
	"strconv"
)

// Sorter orders create actions for the changeset. It partitions actions by
// type tier (proposal_epic → features+data_flow → multi-component tests),
// runs Kahn's algorithm with a lex-spec_node_id tiebreak inside each tier,
// and assigns sequential op_ids zero-padded to the batch's total width.
type Sorter struct{}

// Sort orders creates and returns: the ordered ops with assigned op_ids,
// a batch map of spec_node_id → op_id (so Resolver can encode in-batch
// deps as ref:op), and an error if the in-batch dep graph contains a
// cycle or carries an unknown NodeType.
func (s *Sorter) Sort(creates []CreateAction) ([]OrderedOp, map[string]string, error) {
	tiers := make([][]CreateAction, 3)
	for _, a := range creates {
		t, err := tierOf(a.NodeType)
		if err != nil {
			return nil, nil, err
		}
		tiers[t] = append(tiers[t], a)
	}

	ordered := make([]OrderedOp, 0, len(creates))
	batch := make(map[string]string, len(creates))
	pad := digits(len(creates))
	counter := 1

	for _, tier := range tiers {
		sorted, err := kahn(tier)
		if err != nil {
			return nil, nil, err
		}
		for _, a := range sorted {
			opID := fmt.Sprintf("op-%0*d", pad, counter)
			counter++
			ordered = append(ordered, OrderedOp{OpID: opID, Action: a})
			batch[a.SpecNodeID] = opID
		}
	}

	return ordered, batch, nil
}

// tierOf maps a spec node type to its emit tier. Unknown types are an
// error: every CreateAction reaching the sorter must come from impact's
// classifier and so must carry a tier-able type.
func tierOf(nodeType string) (int, error) {
	switch nodeType {
	case "proposal":
		return TierProposalEpic, nil
	case "component", "data_flow":
		return TierFeatureOrFlow, nil
	case "test_section":
		return TierMultiCompTest, nil
	default:
		return 0, fmt.Errorf("emit: topological sort: unknown NodeType %q", nodeType)
	}
}

// kahn runs Kahn's topological sort within a single tier. Ready nodes are
// drawn from a lex-min heap keyed by spec_node_id, so unconstrained pairs
// emit in deterministic lex order regardless of input order or Go map
// iteration.
func kahn(actions []CreateAction) ([]CreateAction, error) {
	if len(actions) == 0 {
		return nil, nil
	}

	inDeg := make(map[string]int, len(actions))
	adj := make(map[string][]string, len(actions))
	idToAction := make(map[string]CreateAction, len(actions))
	for _, a := range actions {
		idToAction[a.SpecNodeID] = a
		inDeg[a.SpecNodeID] = 0
	}
	for _, a := range actions {
		for _, dep := range a.DepSpecNodeIDs {
			if _, ok := idToAction[dep]; !ok {
				continue
			}
			adj[dep] = append(adj[dep], a.SpecNodeID)
			inDeg[a.SpecNodeID]++
		}
	}
	ready := &lexHeap{}
	heap.Init(ready)
	for _, a := range actions {
		if inDeg[a.SpecNodeID] == 0 {
			heap.Push(ready, a.SpecNodeID)
		}
	}

	out := make([]CreateAction, 0, len(actions))
	for ready.Len() > 0 {
		id := heap.Pop(ready).(string)
		out = append(out, idToAction[id])
		for _, nbr := range adj[id] {
			inDeg[nbr]--
			if inDeg[nbr] == 0 {
				heap.Push(ready, nbr)
			}
		}
	}

	if len(out) != len(actions) {
		var remaining []string
		for id, d := range inDeg {
			if d > 0 {
				remaining = append(remaining, id)
			}
		}
		sort.Strings(remaining)
		return nil, fmt.Errorf("emit: topological sort: cycle among spec_node_ids %v", remaining)
	}
	return out, nil
}

// digits returns the number of decimal digits in n, with a floor of 1 so
// op-ids for empty or single-element batches stay well-formed.
func digits(n int) int {
	if n <= 0 {
		return 1
	}
	return len(strconv.Itoa(n))
}

// lexHeap is a min-heap of spec_node_id strings. Using a heap (rather than
// re-sorting a slice on every step) keeps Kahn at O((V+E) log V) without
// disturbing determinism.
type lexHeap []string

func (h lexHeap) Len() int           { return len(h) }
func (h lexHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h lexHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *lexHeap) Push(x any)        { *h = append(*h, x.(string)) }
func (h *lexHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}
