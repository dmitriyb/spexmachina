# Reference resolution

Implementation notes for `Resolver`.

## Dep Classification

```go
func (r *Resolver) ResolveDeps(depSpecNodeIDs []string) ([]Ref, error) {
    out := make([]Ref, 0, len(depSpecNodeIDs))
    for _, id := range depSpecNodeIDs {
        // 1. In-batch?
        if opID, ok := r.Batch[id]; ok {
            out = append(out, Ref{Kind: "op", OpID: opID})
            continue
        }
        // 2. Mapping store has an open bead? GetBySpecNode returns
        //    ALL records for the node; pickOpenRecord chooses the
        //    highest-ID record whose BeadStatus is not "closed"
        //    (empty status counts as open).
        recs, err := r.MappingStore.GetBySpecNode(id)
        if err == nil {
            if rec, anyOpen := pickOpenRecord(recs); anyOpen {
                out = append(out, Ref{Kind: "bead", BeadID: rec.BeadID})
                continue
            }
            // 2b. All records closed → drop (satisfied).
            continue
        }
        // 3. Fallback: adapter-time lookup.
        out = append(out, Ref{Kind: "spec_node", SpecNodeID: id})
    }
    return out, nil
}
```

The order of `depSpecNodeIDs` is preserved in the output (deterministic).

## Parent Resolution

```go
func (r *Resolver) ResolveParent(proposal string) (Ref, error) {
    // Epic is always in-batch first op OR an existing epic bead in the mapping store.
    if rec, err := r.MappingStore.GetByProposalEpic(proposal); err == nil {
        // Existing epic — re-run case.
        return Ref{Kind: "bead", BeadID: rec.BeadID}, nil
    }
    // New epic — its op_id is the first entry in r.Batch keyed by a synthetic
    // "proposal/<ref>/epic" spec_node_id. Builder inserts this key into Batch
    // before calling ResolveParent.
    epicKey := "proposal/" + proposal + "/epic"
    if opID, ok := r.Batch[epicKey]; ok {
        return Ref{Kind: "op", OpID: opID}, nil
    }
    return Ref{}, fmt.Errorf("emit: resolver: proposal epic not found for %q", proposal)
}
```

The proposal epic's spec_node_id is synthetic — there's no corresponding spec-tree node. It's a runtime marker that lives only in the emit batch.

## Priority

```go
func (r *Resolver) Priority(specNodeID string) int {
    comp, ok := r.SpecGraph.Component(specNodeID)
    if !ok {
        return fallbackPriority // 3
    }
    best := math.MaxInt
    for _, reqID := range comp.Implements {
        modReq, ok := r.SpecGraph.ModuleRequirement(reqID)
        if !ok || modReq.PreqID == "" {
            continue
        }
        projReq, ok := r.SpecGraph.ProjectRequirement(modReq.PreqID)
        if !ok || projReq.Priority == nil {
            continue
        }
        if *projReq.Priority < best {
            best = *projReq.Priority
        }
    }
    if best == math.MaxInt {
        return fallbackPriority
    }
    return best
}
```

`fallbackPriority` is a package constant — kept explicit and documented so the test for it pins behavior.

## Edge Cases

- A dep spec_node_id that appears both in `r.Batch` AND in the mapping store as open: prefer `ref:op` (the in-batch op is the authoritative latest work; the mapping record may be stale mid-batch).
- A dep spec_node_id with a mapping record but no `status` field: treat as open (conservative — err toward attaching an edge rather than silently dropping).
- Priority tie at `best` across multiple project reqs: one value is returned; min is associative.

## Testing

All three dep classification branches are exercised in `test_resolver_and_sorter.md`. Priority tests assert both the chain walk and the fallback.
