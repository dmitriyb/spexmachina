package plan

import (
	"fmt"
	"sort"
	"strings"
)

// tierOf maps a create action's NodeType to the type tier
// (spec/plan/arch_topological_sorter.md, "Ordering Rules"): proposal epic
// first, then components and data_flows together, then multi-component
// test sections. A NodeType absent from this table belongs to no tier.
var tierOf = map[string]int{
	KindProposalEpic: TierProposalEpic,
	KindComponent:    TierFeatureOrFlow,
	KindDataFlow:     TierFeatureOrFlow,
	KindTestSection:  TierMultiCompTest,
}

// Sort orders a batch's create actions so every in-batch dependency comes
// before its dependent (spec/plan/arch_topological_sorter.md). It
// partitions the actions by type tier, then runs Kahn's algorithm within
// each tier with a smallest-lex-spec_node_id tiebreak among nodes ready at
// each step. The DAG is rebuilt per tier, so a dep pointing outside the
// tier or outside the batch is invisible to the sort and left for
// Resolver to classify.
//
// It answers with the actions in emitted order, each paired with a
// provisional "op-<n>" id numbered from 1, plus the spec_node_id-to-op_id
// map built from that order. ChangesetBuilder keeps the order and
// discards both, renumbering every op itself once the retarget and close
// ops are counted.
//
// Two kinds of batch are refused, with no ordering returned at all: one
// holding an in-batch dependency cycle, and one holding a create whose
// spec node kind belongs to no tier.
func Sort(actions []Action) ([]OrderedOp, map[string]string, error) {
	for _, a := range actions {
		if _, ok := tierOf[a.NodeType]; !ok {
			return nil, nil, fmt.Errorf("plan: sort: spec node %s has kind %q, which belongs to no tier", a.SpecNodeID, a.NodeType)
		}
	}

	byTier := make(map[int][]Action)
	for _, a := range actions {
		t := tierOf[a.NodeType]
		byTier[t] = append(byTier[t], a)
	}

	ordered := make([]Action, 0, len(actions))
	for tier := TierProposalEpic; tier <= TierMultiCompTest; tier++ {
		nodes := byTier[tier]
		if len(nodes) == 0 {
			continue
		}
		tierOrder, err := kahnSort(nodes)
		if err != nil {
			return nil, nil, err
		}
		ordered = append(ordered, tierOrder...)
	}

	ops := make([]OrderedOp, len(ordered))
	specToOp := make(map[string]string, len(ordered))
	for i, a := range ordered {
		opID := fmt.Sprintf("op-%d", i+1)
		ops[i] = OrderedOp{OpID: opID, Action: a}
		specToOp[a.SpecNodeID] = opID
	}
	return ops, specToOp, nil
}

// kahnSort orders one tier's actions with Kahn's algorithm: an edge runs
// from A to B when B declares a dep (via DepSpecNodeIDs) on A's
// spec_node_id and A is in this same tier. At each step, among nodes with
// zero remaining incoming edges, the smallest spec_node_id (lex order) is
// emitted next, which is what makes the output deterministic across runs.
//
// If nodes remain with unsatisfied incoming edges once no more nodes are
// ready, that is a cycle: the error names every such node — the cycle
// itself and anything left stranded behind it — in lex order.
func kahnSort(nodes []Action) ([]Action, error) {
	inTier := make(map[string]bool, len(nodes))
	bySpecID := make(map[string]Action, len(nodes))
	for _, n := range nodes {
		inTier[n.SpecNodeID] = true
		bySpecID[n.SpecNodeID] = n
	}

	children := make(map[string][]string)
	indegree := make(map[string]int, len(nodes))
	for _, n := range nodes {
		indegree[n.SpecNodeID] = 0
	}
	for _, n := range nodes {
		for _, dep := range n.DepSpecNodeIDs {
			if !inTier[dep] {
				continue
			}
			children[dep] = append(children[dep], n.SpecNodeID)
			indegree[n.SpecNodeID]++
		}
	}

	ready := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if indegree[n.SpecNodeID] == 0 {
			ready = append(ready, n.SpecNodeID)
		}
	}
	sort.Strings(ready)

	order := make([]Action, 0, len(nodes))
	for len(ready) > 0 {
		next := ready[0]
		ready = ready[1:]
		order = append(order, bySpecID[next])
		for _, child := range children[next] {
			indegree[child]--
			if indegree[child] == 0 {
				i := sort.SearchStrings(ready, child)
				ready = append(ready, "")
				copy(ready[i+1:], ready[i:])
				ready[i] = child
			}
		}
	}

	if len(order) != len(nodes) {
		stuck := make([]string, 0, len(nodes)-len(order))
		for id, deg := range indegree {
			if deg > 0 {
				stuck = append(stuck, id)
			}
		}
		sort.Strings(stuck)
		return nil, fmt.Errorf("plan: sort: cycle among spec nodes: %s", strings.Join(stuck, ", "))
	}
	return order, nil
}
