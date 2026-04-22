# Topological sorting

Implementation notes for `TopologicalSorter`.

## Kahn's Algorithm with Lex Tiebreak

```go
func (s *Sorter) Sort(creates []CreateAction) ([]OrderedOp, map[string]string, error) {
    // Partition by type tier.
    tiers := [][]CreateAction{
        nil, // 0: proposal_epic
        nil, // 1: features + data_flow tasks
        nil, // 2: multi-component test tasks
    }
    for _, a := range creates {
        tiers[tierOf(a.NodeType)] = append(tiers[tierOf(a.NodeType)], a)
    }

    var ordered []OrderedOp
    batch := map[string]string{}
    opCounter := 1
    pad := digits(len(creates))

    for _, tier := range tiers {
        sorted, err := kahn(tier)
        if err != nil {
            return nil, nil, err
        }
        for _, a := range sorted {
            opID := fmt.Sprintf("op-%0*d", pad, opCounter)
            opCounter++
            ordered = append(ordered, OrderedOp{OpID: opID, Action: a})
            batch[a.SpecNodeID] = opID
        }
    }

    return ordered, batch, nil
}
```

## Per-Tier Kahn

```go
func kahn(actions []CreateAction) ([]CreateAction, error) {
    // Build in-degree and adjacency lists over the in-tier DAG.
    inDeg := map[string]int{}
    adj := map[string][]string{}
    idToAction := map[string]CreateAction{}
    for _, a := range actions {
        idToAction[a.SpecNodeID] = a
        inDeg[a.SpecNodeID] = 0
    }
    for _, a := range actions {
        for _, dep := range a.DepSpecNodeIDs {
            if _, ok := idToAction[dep]; !ok {
                continue // dep is out-of-tier or out-of-batch — handled by Resolver
            }
            adj[dep] = append(adj[dep], a.SpecNodeID)
            inDeg[a.SpecNodeID]++
        }
    }

    // Priority queue of nodes with in-degree 0, ordered by spec_node_id lex.
    ready := newLexMinHeap()
    for id, d := range inDeg {
        if d == 0 {
            ready.Push(id)
        }
    }

    var out []CreateAction
    for !ready.Empty() {
        id := ready.Pop()
        out = append(out, idToAction[id])
        for _, nbr := range adj[id] {
            inDeg[nbr]--
            if inDeg[nbr] == 0 {
                ready.Push(nbr)
            }
        }
    }

    if len(out) != len(actions) {
        // Cycle — collect remaining nodes with inDeg > 0.
        var remaining []string
        for id, d := range inDeg {
            if d > 0 {
                remaining = append(remaining, id)
            }
        }
        return nil, fmt.Errorf("emit: topological sort: cycle among spec_node_ids %v", remaining)
    }
    return out, nil
}
```

## Op ID Format

- Width-padded to the batch's size: 1-9 ops → `op-1` … `op-9`; 10-99 → `op-01` … `op-99`; 100+ → `op-001` … etc.
- `digits(n)` returns `len(strconv.Itoa(n))`, min 1.

## Cross-Tier Deps

A dep from tier 2 (test task) to tier 1 (component) is allowed — component comes first by tier, so tier 2 ops can always resolve tier 1 deps as `ref:op`. A dep from tier 1 to tier 2 is disallowed (tests cannot be parents of features). If encountered, return a structured error — this is a spec-shape violation.

## Determinism Notes

- `map` iteration in Go is random; the algorithm uses explicit iteration over `actions` (stable) for building in-degree, and the priority queue for selecting the next node. `map` iteration for reading `adj` is fine because the reduction (decrement `inDeg[nbr]`) is commutative.
- Tie-breaking with lex `spec_node_id` guarantees a single output order for any input.
