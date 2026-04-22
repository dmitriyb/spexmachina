# Resolver

Resolves each create action's `DepSpecNodeIDs` (from impact) and its parent into one of three ref shapes, and computes the action's priority via the project-requirement chain.

## The Three Ref Shapes

For every spec_node_id in `DepSpecNodeIDs`:

1. **`ref:op`** — if another create op in the same batch targets this spec_node_id. The op_id is known at resolve time because TopologicalSorter has already assigned op_ids in dependency order.
2. **`ref:bead`** — if the mapping store has a record for this spec_node_id with `status != "closed"`. The bead_id is taken from the record.
3. **`ref:spec_node`** — fallback; neither of the above applied. The adapter resolves at exec time by querying the mapping store at that moment (handles races where a record appeared between emit and adapter run).

If the mapping store has a record with `status == "closed"`, the dep is **dropped** — the work is satisfied, no edge needed.

## Why Three Shapes

`ref:op` is the structural fix for the broken-dep-graph bug (commit `21defea`). Pre-this-proposal, impact resolved deps against the mapping store at impact time. When a dep was itself being obsoleted+recreated in the same batch, impact picked up the OLD (soon-closed) bead ID. The current `apply` then passed that old ID to `br create --deps blocks:<old>`, and the close phase killed the referenced bead, leaving the new bead pointing at a dead predecessor.

`ref:op` sidesteps the problem by deferring resolution to adapter-exec time: the adapter builds A, knows A's fresh bead_id, then builds B with `--deps depends:<A-new>`. Structural guarantee, no apply-time patching.

## Parent Resolution

The proposal epic is the parent of every non-epic create in the run:

- If the proposal is **new** (this is the first emit for this proposal), the epic is also a new create op (the first op). Each subsequent create's `parent` is `{ref:op,op_id:"<epic op>"}`.
- If the proposal **already has an epic bead** in the mapping store (re-run of a partial run, or idempotent re-emit), the epic op is skipped; each create's `parent` is `{ref:bead,bead_id:"<existing epic bead>"}`.

## Priority

Per create action, walk the chain:

1. Component → `implements: [req_id, …]`.
2. For each `req_id`, module requirement → `preq_id`.
3. Project requirement → `priority` field.

Take the **minimum** priority across all reachable project requirements. Lowest number wins.

If no priority chain is reachable (missing `preq_id` or missing project req), apply a deterministic fallback: priority `3` (mid-range). Surface a warning through the emit report but do not fail — the validator's `requirement_coverage_checker` is the authoritative gate for upstream chain completeness.

## Interface

```go
type Resolver struct {
    SpecGraph    *spec.Graph
    MappingStore mapping.Store
    Batch        map[string]string // spec_node_id → op_id, populated by TopologicalSorter
}

func (r *Resolver) ResolveDeps(specNodeIDs []string) ([]Ref, error)
func (r *Resolver) ResolveParent(proposal string) (Ref, error)
func (r *Resolver) Priority(componentID string) int
```

`Batch` is populated before `ResolveDeps` is called — TopologicalSorter runs first to assign op_ids; Resolver then classifies each dep.

## Determinism

- Iteration order over `DepSpecNodeIDs` in the impact report is preserved as given (impact emits them in a deterministic order).
- Priority computation uses `min` on a finite set; result is independent of enumeration order.
- Parent resolution has one deterministic output per `(proposal, mapping store state)` pair.
