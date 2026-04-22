# Changeset building

Implementation notes for `ChangesetBuilder.Build`.

## Steps

```go
func (b *Builder) Build(report impact.Report) (Changeset, error) {
    // 1. Partition actions by type tier.
    tiers := partitionByTier(report.Actions, b.SpecGraph)

    // 2. Sort creates within each tier.
    sorter := &Sorter{}
    ordered, batchMap, err := sorter.Sort(tiers.AllCreates())
    if err != nil {
        return Changeset{}, err
    }

    // 3. Reserve idempotency labels.
    labels := (&Labeler{MappingStore: b.MappingStore}).Reserve(len(ordered))

    // 4. Resolve parents and deps via Resolver (which now has batchMap).
    resolver := &Resolver{SpecGraph: b.SpecGraph, MappingStore: b.MappingStore, Batch: batchMap}

    ops := make([]Op, 0, len(ordered)+len(tiers.Closes))
    for i, oc := range ordered {
        parent, err := resolver.ResolveParent(b.Proposal)
        if err != nil { return Changeset{}, err }
        deps, err := resolver.ResolveDeps(oc.Action.DepSpecNodeIDs)
        if err != nil { return Changeset{}, err }
        ops = append(ops, Op{
            OpID: oc.OpID,
            Type: "create",
            SpecNodeKind: oc.Action.NodeType,
            SpecNodeID: oc.Action.SpecNodeID,
            Idempotency: Idem{Label: labels[i]},
            Parent: parent,
            Deps: deps,
            Priority: resolver.Priority(oc.Action.SpecNodeID),
            Title: titleFor(oc.Action, b.SpecGraph),
            Body: bodyFor(oc.Action, b.SpecGraph),
        })
    }

    // 5. Append close ops.
    for _, cl := range tiers.Closes {
        ops = append(ops, Op{
            OpID: nextOpID(),
            Type: "close",
            Target: Ref{Kind: "bead", BeadID: cl.BeadID},
            Labels: []string{"spex:obsolete", "commit:" + b.GitHead},
            Reason: cl.Reason,
        })
    }

    return Changeset{Version: 1, GitHead: b.GitHead, Proposal: b.Proposal, Ops: ops}, nil
}
```

## Canonical JSON Encoding

Use `encoding/json` with a custom `MarshalJSON` on `Op` that enforces field order via a struct tag-defined shape. Alternative: encode via `json.Encoder` with `SetIndent("", "  ")` and rely on the struct field order (Go's default). The struct field order IS the canonical order.

## No Go-Side Embedding

Op's fields are plain strings / slices / ints — no generic interfaces, no `map[string]any`. A ref is a typed struct:

```go
type Ref struct {
    Kind       string `json:"ref"`                   // "bead" | "op" | "spec_node"
    BeadID     string `json:"bead_id,omitempty"`
    OpID       string `json:"op_id,omitempty"`
    SpecNodeID string `json:"spec_node_id,omitempty"`
    // Optional edge type for obsolete+create lineage: "blocks"
    EdgeType   string `json:"type,omitempty"`
}
```

Only one of `BeadID` / `OpID` / `SpecNodeID` is set per ref; `omitempty` keeps JSON clean.

## Title and Body

- Title for a component create: `"<module>: <component_name>"` (e.g., `"emit: ChangesetBuilder"`).
- Title for a data_flow task: `"<module>: data_flow <flow_name>"`.
- Title for a multi-component test task: `"<module>: test <test_name>"`.
- Title for a proposal epic: `"Proposal: <proposal_ref>"`.
- Body: a markdown blob with links to the spec files (`arch_*.md`, `impl_*.md`, etc.) resolved via `spex map context`. The adapter passes body through to the tracker's description field.

## Error Surface

All errors from sub-components are wrapped with `"emit: build: ..."` context. A spec_node_id with no component record in the spec graph is a validator-layer issue; Builder returns a structured error naming the spec_node_id so the caller can re-run validate.
