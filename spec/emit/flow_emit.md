# Emit flow

## Position in the pipeline

Emit sits fourth in a six-stage run, and it is the stage that has to be right
about both of its neighbours: it consumes the impact report the upstream stages
produce, and the changeset it writes is a contract the adapter executes and
`spex ingest` reconciles. Every hand-off from `spex diff` onwards goes through a
file, so each of those stages can be re-run from the artifact its predecessor
wrote. `spex validate` is the exception — it is a gate, not a producer, and
`spex diff` reads only `--snapshot`, `--map` and `--spec-dir` — so restarting at
`diff` means re-reading the spec directory, not a validate artifact.

```
   spec change (project.json, module.json, *.md)
         │
         ▼
   spex validate  ──▶  spex diff  ──▶  spex impact --beads
                                             │
                                             ▼
                                      impact report
                                             │
                                             ▼
                              ┌──────────────────────────┐
                              │ spex emit                │
                              └─────────────┬────────────┘
                                            │
                                            ▼
                                     changeset.json
                                            │
                                            ▼
                              scripts/apply-br.sh (reference adapter)
                                            │
                                            ▼
                                      receipts.json
                                            │
                                            ▼
                              ┌──────────────────────────┐
                              │ spex ingest              │
                              └─────────────┬────────────┘
                                            │
                                            ▼
                          .bead-map.json  +  spec/.snapshot.json
                                             (snapshot iff status == complete)
```

A partial run — any op with `status: error` in the receipts — leaves the
snapshot untouched, so the next `spex diff` recomputes from the same baseline
and emit sees only the ops that never landed.

## Emit internals

```
                    ┌──────────────────────┐
                    │  spex emit            │
                    │  --proposal <ref>     │
                    │  --git-head <sha>     │
                    │  [--impact file]      │
                    └──────────┬───────────┘
                               │
                               ▼
           ┌───────────────────────────────────────┐
           │ 1. Load inputs                        │
           │   - impact report (stdin/file)        │
           │   - .bead-map.json (mapping.Store)    │
           │   - spec/project.json + module trees  │
           │   - --git-head (caller-supplied)      │
           └─────────────┬─────────────────────────┘
                         │
                         ▼
           ┌───────────────────────────────────────┐
           │ 2. Partition actions by tier          │
           │   - tier 0: proposal epic             │
           │   - tier 1: features, data_flow tasks │
           │   - tier 2: multi-component test tasks│
           └─────────────┬─────────────────────────┘
                         │
                         ▼
           ┌───────────────────────────────────────┐
           │ 3. TopologicalSorter                  │
           │   Kahn within each tier, lex tiebreak │
           │   Assigns op-0001..op-NNNN            │
           │   Produces batchMap: spec_node_id→op_id│
           └─────────────┬─────────────────────────┘
                         │
                         ▼
           ┌───────────────────────────────────────┐
           │ 4. IdempotencyLabeler                 │
           │   LabelFor per create action:         │
           │   fresh → spex:<cursor>, advance      │
           │   (cursor seeded from mapping store); │
           │   cleanup → spex:cleanup-<node-id>;   │
           │   modify-pair → reuse record's id     │
           └─────────────┬─────────────────────────┘
                         │
                         ▼
           ┌───────────────────────────────────────┐
           │ 5. Resolver                           │
           │   - Deps → ref:op | ref:bead | ref:spec_node │
           │   - Parent → proposal epic ref        │
           │   - Priority → implements→preq→priority chain│
           └─────────────┬─────────────────────────┘
                         │
                         ▼
           ┌───────────────────────────────────────┐
           │ 6. ChangesetBuilder                   │
           │   Composes Op records + close ops     │
           │   Applies commit:<HEAD> labels        │
           │   Canonical field order               │
           └─────────────┬─────────────────────────┘
                         │
                         ▼
                  changeset.json (v1)
                         │
                  stdout or --out file
```

## Data Shapes

### Impact report (input)

Reuses `impact.Report` shape defined in the impact module. Key field for emit: `Actions[].DepSpecNodeIDs` (the proposal's dep-flow contract change — see §2 of the proposal).

### Mapping store reads (input)

- `store.GetBySpecNode(id)` — lookup for ref:bead classification.
- `store.GetByProposalEpic(proposal)` — lookup for parent resolution on re-runs.
- `store.NextRecordID()` — counter read for label reservation (no write).

### changeset.json (output)

```json
{
  "version": 1,
  "git_head": "deadbeef...",
  "proposal": "2026-04-18-decouple-spex-from-br",
  "ops": [
    {
      "op_id": "op-0001",
      "type": "create",
      "spec_node_kind": "proposal_epic",
      "spec_node_id": "<synthetic>",
      "idempotency": { "label": "spex:142" },
      "priority": 3,
      "title": "Proposal: 2026-04-18-decouple-spex-from-br",
      "body": "…"
    },
    {
      "op_id": "op-0002",
      "type": "create",
      "spec_node_kind": "component",
      "spec_node_id": "7f06f7d80e94",
      "idempotency": { "label": "spex:143" },
      "parent": { "ref": "op", "op_id": "op-0001" },
      "deps": [
        { "ref": "op", "op_id": "op-0003" }
      ],
      "priority": 1,
      "title": "emit: ChangesetBuilder",
      "body": "…"
    }
  ]
}
```

## Error Paths

- Impact report has `errors` → abort before loading mapping store. Exit 1.
- Cycle in in-batch deps → abort after sort. Exit 2.
- Missing project requirement in priority chain → default priority, no error (non-blocking warning).
- `--git-head` missing or malformed → pre-flight rejection before any processing. Exit 1.
