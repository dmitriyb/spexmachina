package plan

import (
	"fmt"
	"sort"
	"strings"
)

// Sort orders a batch's create actions so every in-batch dependency comes
// before its dependent (spec/plan/arch_topological_sorter.md, "Ordering
// Rules" and "Algorithm"). It partitions the actions into layers — the
// proposal epic first, then one layer per plan-relevant node type in the
// order planRelevant declares them, then cleanup creates last — and runs
// Kahn's algorithm within each layer with a smallest-lex-spec_node_id
// tiebreak among nodes ready at each step. The DAG is rebuilt per layer, so
// a dep pointing at an earlier layer — or outside the batch — is invisible
// to the sort and left for Resolver to classify. A dep pointing at a
// *later* layer is refused outright: it would resolve to a forward ref:op
// the adapter cannot follow, so planRelevant's declared order has to agree
// with the edges the classifier collected.
//
// It answers with the actions in emitted order, each paired with a
// provisional "op-<n>" id numbered from 1, plus the spec_node_id-to-op_id
// map built from that order. ChangesetBuilder keeps the order and discards
// both, renumbering every op itself once the retarget and close ops are
// counted.
//
// Three kinds of batch are refused, with no ordering returned at all: one
// holding an in-batch dependency cycle, one holding a create whose spec
// node kind belongs to no layer — the epic and cleanup kinds are placed by
// rule, everything else by planRelevant — and one holding a dep that points
// at a later layer's op.
func Sort(actions []Action, planRelevant []string) ([]OrderedOp, map[string]string, error) {
	layerIndex := make(map[string]int, len(planRelevant))
	for i, t := range planRelevant {
		layerIndex[t] = i + 1 // layer 0 is reserved for the proposal epic
	}
	cleanupLayer := len(planRelevant) + 1

	layerOf := make(map[string]int, len(actions))
	for _, a := range actions {
		l, err := layerFor(a, layerIndex, cleanupLayer)
		if err != nil {
			return nil, nil, err
		}
		layerOf[a.SpecNodeID] = l
	}

	if err := checkNoForwardLayerDeps(actions, layerOf); err != nil {
		return nil, nil, err
	}

	byLayer := make(map[int][]Action)
	for _, a := range actions {
		l := layerOf[a.SpecNodeID]
		byLayer[l] = append(byLayer[l], a)
	}

	ordered := make([]Action, 0, len(actions))
	for layer := 0; layer <= cleanupLayer; layer++ {
		nodes := byLayer[layer]
		if len(nodes) == 0 {
			continue
		}
		layerOrder, err := kahnSort(nodes)
		if err != nil {
			return nil, nil, err
		}
		ordered = append(ordered, layerOrder...)
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

// layerFor answers the layer index for one create action. The proposal
// epic is placed first by rule; a cleanup create is placed last by rule,
// discriminated by the "Code cleanup:" reason prefix (isCleanup,
// plan/labeler.go) rather than by NodeType, since a cleanup action arrives
// carrying the removed node's own type — a cleanup for a removed component
// must land in the cleanup layer, never the component layer. Every other
// create is placed by looking up its NodeType in planRelevant's declared
// order. A NodeType placed by none of the three is an error naming the
// offending spec node and its kind.
func layerFor(a Action, layerIndex map[string]int, cleanupLayer int) (int, error) {
	if a.NodeType == KindProposalEpic {
		return 0, nil
	}
	if isCleanup(a) {
		return cleanupLayer, nil
	}
	if l, ok := layerIndex[a.NodeType]; ok {
		return l, nil
	}
	return 0, fmt.Errorf("plan: sort: spec node %s has kind %q, which belongs to no layer", a.SpecNodeID, a.NodeType)
}

// checkNoForwardLayerDeps refuses a batch where a create's dep names an
// in-batch op placed in a *later* layer than the create itself: since layers
// emit low-to-high, that dep could never be satisfied by file order. The
// error names both the dependency and the dependent so a reader can find
// the offending profile/spec-graph mismatch (arch_topological_sorter.md,
// "Algorithm", step 2; "Interface").
func checkNoForwardLayerDeps(actions []Action, layerOf map[string]int) error {
	var bad []string
	for _, b := range actions {
		for _, dep := range b.DepSpecNodeIDs {
			depLayer, ok := layerOf[dep]
			if !ok {
				continue // outside the batch: invisible, left for Resolver
			}
			if depLayer > layerOf[b.SpecNodeID] {
				bad = append(bad, fmt.Sprintf("%s depends on later-layer %s", b.SpecNodeID, dep))
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("plan: sort: dep points at a later layer: %s", strings.Join(bad, "; "))
}

// kahnSort orders one layer's actions with Kahn's algorithm: an edge runs
// from A to B when B declares a dep (via DepSpecNodeIDs) on A's
// spec_node_id and A is in this same layer. At each step, among nodes with
// zero remaining incoming edges, the smallest spec_node_id (lex order) is
// emitted next, which is what makes the output deterministic across runs.
//
// If nodes remain with unsatisfied incoming edges once no more nodes are
// ready, that is a cycle: the error names every such node — the cycle
// itself and anything left stranded behind it — in lex order.
func kahnSort(nodes []Action) ([]Action, error) {
	inLayer := make(map[string]bool, len(nodes))
	bySpecID := make(map[string]Action, len(nodes))
	for _, n := range nodes {
		inLayer[n.SpecNodeID] = true
		bySpecID[n.SpecNodeID] = n
	}

	children := make(map[string][]string)
	indegree := make(map[string]int, len(nodes))
	for _, n := range nodes {
		indegree[n.SpecNodeID] = 0
	}
	for _, n := range nodes {
		for _, dep := range n.DepSpecNodeIDs {
			if !inLayer[dep] {
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
