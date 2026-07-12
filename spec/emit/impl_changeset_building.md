# Changeset building

Implementation notes for `ChangesetBuilder.Build`.

## Steps

```go
func (b *Builder) Build(report impact.ImpactReport) (Changeset, error) {
    // 1. Detect a pre-existing proposal epic; a fresh proposal prepends
    //    a synthetic epic create to the batch.
    hasExistingEpic, err := b.lookupExistingEpic()
    if err != nil { return Changeset{}, err }
    creates := b.collectCreates(report, hasExistingEpic)

    // 2. Sort creates (tiered Kahn, lex tiebreak), then assign
    //    zero-padded op ids in sorted order and build batchMap
    //    (spec_node_id → op_id, plus the synthetic
    //    "proposal/<ref>/epic" key when the epic is in-batch).
    ordered, _, err := (&Sorter{}).Sort(creates)
    if err != nil { return Changeset{}, err }
    batchMap := assignOpIDs(ordered)

    // 3. Wire the per-action Labeler and the Resolver (which reads
    //    batchMap for in-batch dep classification).
    labeler := &Labeler{MappingStore: b.MappingStore}
    resolver := &Resolver{SpecGraph: b.SpecGraph, MappingStore: b.MappingStore, Batch: batchMap}

    // 4. One label and one op per ordered create action. LabelFor
    //    branches per action class: fresh creates consume the cursor,
    //    cleanup creates derive spex:cleanup-<spec_node_id>, and
    //    modify-pairs reuse the existing record id via GetByBead —
    //    there is no bulk Reserve(len(ordered)) call.
    ops := make([]Op, 0, len(ordered)+len(report.Obsoletes))
    for _, oc := range ordered {
        label, err := labeler.LabelFor(oc.Action)
        if err != nil { return Changeset{}, err }
        // makeCreateOp applies the epic/cleanup/conventional shapes:
        // parent + deps via Resolver, priority via the implements
        // chain, Title via titleFor(oc.Action), Body via
        // bodyFor(oc.Action, b.SpecGraph).
        op, err := b.makeCreateOp(oc, label, resolver)
        if err != nil { return Changeset{}, err }
        ops = append(ops, op)
    }

    // 5. Append close ops.
    for _, ob := range report.Obsoletes {
        ops = append(ops, Op{
            OpID: nextOpID(),
            Type: "close",
            Target: &Ref{Kind: RefBead, BeadID: ob.BeadID},
            Labels: []string{"spex:obsolete", "commit:" + b.GitHead},
            Reason: ob.Reason,
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
