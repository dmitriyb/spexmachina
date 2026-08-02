# Resolver

Resolves each create action's `DepSpecNodeIDs` (from impact) and its parent into one of three ref shapes, and computes the action's priority via the project-requirement chain.

## The Three Ref Shapes

A dep a create action names is written as one of the shapes below, or dropped — [[d096fd5d6cd8|the encoding that lets a changeset name work that does not exist yet]]. For each dep spec_node_id, first match wins:

1. **`ref:op`** — another create op in the same batch targets this spec_node_id.
2. **`ref:bead`** — the mapping store holds at least one record for this spec_node_id whose bead is not closed. Where several such records exist the highest record id wins, so the latest re-implementation supersedes the earlier ones; a record carrying no status at all counts as open, the conservative reading that attaches an edge rather than dropping one.
3. **`ref:spec_node`** — the mapping store knows nothing about this spec_node_id. The adapter resolves it at exec time by querying the mapping store at that moment, which covers the race where a record appeared between emit and the adapter run.

That ordering is a precedence, not a search order: a dep that is both in the batch and open in the mapping store is written `ref:op`, because the in-batch op is the authoritative latest work and the record can be stale before the batch lands.

If every record for the spec_node_id is closed, the dep is **dropped** — the work is satisfied, no edge needed.

## Why Three Shapes

`ref:op` is the structural fix for the broken-dep-graph bug (commit `21defea`). Pre-decouple, impact resolved deps against the mapping store at impact time. When a dep was itself being obsoleted+recreated in the same batch, impact picked up the OLD (soon-closed) bead ID, passed it to `br create --deps blocks:<old>`, and the close phase then killed the referenced bead — leaving the new bead pointing at a dead predecessor.

`ref:op` sidesteps the problem by deferring resolution to adapter-exec time: the adapter builds A, knows A's fresh bead_id, then builds B with `--deps depends:<A-new>`. Structural guarantee, no post-hoc patching.

Resolver is where the project requirement's "parent hierarchy, lineage tracking, and priority propagation" is actually decided, and it decides all three **without knowing what tracker will execute the result**. A ref names a spec node, an op, or a bead — never a `br` flag. The translation to `--parent`, `--deps <edge>:<id>` and `--priority` happens in the adapter, which is the only component permitted to know a tracker's command surface. That separation is what lets a second adapter target a different tracker against an unchanged changeset.

## Parent Resolution

The proposal epic is the parent of every non-epic create in the run, and [[79c821e01654|the mapping store is consulted first for which epic that is]]:

- If the proposal is **new** (this is the first emit for this proposal), the epic is also a new create op (the first op). Each subsequent create's `parent` is `{ref:op,op_id:"<epic op>"}`.
- If the proposal **already has an epic bead** in the mapping store (re-run of a partial run, or idempotent re-emit), the epic op is skipped; each create's `parent` is `{ref:bead,bead_id:"<existing epic bead>"}`. An epic that already exists wins over an in-batch one, so a re-run that misread the epic as new still parents its creates under the bead that is already there.

The epic's own `spec_node_id` is synthetic: no node in the spec tree corresponds to it, so the value carried in `changeset.json` is the proposal ref itself rather than a 12-hex identity hash, and a reader can tell the epic apart from every other create by that shape alone.

## Priority

Per create action, [[af8c8ecf3519|priority is inherited from the project requirements the component ultimately implements]]. Walk the chain:

1. Component → `implements: [req_id, …]`.
2. For each `req_id`, module requirement → `preq_id`.
3. Project requirement → `priority` field.

Take the **minimum** priority across all reachable project requirements. Lowest number wins.

If no priority chain is reachable (missing `preq_id` or missing project req), apply a deterministic fallback: priority `3` (mid-range). The fallback is silent — the op carries `3` and nothing in the changeset or on stderr records that the chain was unreachable — and it is not an error, because the validator's `requirement_coverage_checker` is the authoritative gate for upstream chain completeness.

## Interface

Resolver is set up with three things — the spec graph, the mapping store, and the batch map of spec_node_id to op_id — and answers three questions per create action: what refs its deps become, what ref its parent becomes, and what priority number it carries.

Its read surface on the spec graph is deliberately narrow. It reads the implements → preq_id → priority chain and nothing else, so nothing about a component's name, description or `uses` edges can reach a ref or a priority. The command adapts the parsed spec directory onto that surface; tests substitute a stand-in.

The batch map must be complete before any dep is resolved. TopologicalSorter runs first for exactly that reason: it fixes the order the op_ids are handed out in, and ChangesetBuilder hands Resolver the finished map before the first dep is classified.

## Every ref names a node that can own a bead

`DepSpecNodeIDs` arrives already filtered: impact produces actions only for the node kinds that can own a bead, and the sorter refuses to tier a create whose kind it does not recognise. The three ref shapes are therefore exhaustive over what actually reaches Resolver. There is no fourth case for a spec node that has no bead and can never acquire one — for such a node `ref:bead` would find no record and `ref:spec_node` would hand the adapter a hash the mapping store can never satisfy, so the dep would be unresolvable at exec time rather than merely deferred.

The priority walk enters the same set from the other end. It starts at a component's `implements` array, so a project requirement's priority reaches a bead through the component that implements the requirement and through no other kind of node. A section of that component's contract carries no `implements` edge and never begins a chain of its own; it inherits whatever priority the component it belongs to resolves.

## Determinism

- Iteration order over `DepSpecNodeIDs` in the impact report is preserved as given (impact emits them in a deterministic order).
- Priority computation uses `min` on a finite set; result is independent of enumeration order.
- Parent resolution has one deterministic output per `(proposal, mapping store state)` pair.

## Test surface

Resolver has no public API surface independent of `ChangesetBuilder` —
nothing else in the emit module's `uses` graph consumes it. Cross-component
integration coverage (Resolver paired with Sorter, Labeler, and Builder)
lives in `test_changeset_builder`'s `describes` array, exercised through
`Builder.Build()`'s public API. Per-method unit tests for the individual
classification, priority, and parent-resolution paths live in
`emit/resolver_test.go` and ship with this component's implementation bead.
