# ActionClassifier

[[06a081811e38|Deciding what happened to each affected spec node]] is this component's whole job, and the answer is one of exactly two words: `create` or `obsolete`. Each create also collects `DepSpecNodeIDs` — identity hashes of the spec nodes the created bead will depend on. Bead-ID resolution is NOT done here; it moves to the emit module.

## Responsibilities

- Assign actions based on match results and change types.
- Handle modified nodes by generating both obsolete (old bead) and create (new bead) actions.
- Gate action production by node type and, for test_sections, by `len(describes)`.
- Choose between obsolete-only and obsolete-plus-cleanup-create for a removed node from the live bead status [[bec96486c6b2|BeadReader]] carried off the caller's tracker file. [[3dcf3c279ac5|A closed bead means the code already shipped]], so the repository now holds code answering to no spec node and a cleanup bead is created to have it deleted; an open or in-progress bead means nothing shipped and there is nothing to clean up.
- For each create action, collect `DepSpecNodeIDs` by walking component `uses` (direct) and module `requires_module` (transitive).

## Node-Type Gate

A change with no existing bead is gated by its node type before the state transition table runs. [[81aac298ce04|The contract layer earns its beads at this gate]]: a data_flow is admitted with no further condition, and a test_section when it spans two or more components — or when the gate cannot establish how many it spans, because no spec graph or module name reached it, the module name does not resolve to a module, or the section is not declared in the module it does resolve to. Coupling that cannot be established is admitted rather than dropped. Admission is not yet a bead — what an admitted change then produces is the state transition table's business.

| NodeType | Admitted by the gate? | Notes |
|----------|-----------------------|-------|
| `component` | yes (feature) | primary work unit |
| `data_flow` | yes (task) | cross-component contract; the gate asks nothing further of it |
| `test_section`, `len(describes) >= 2` | yes (task) | cross-component integration test, needs its own bead |
| `test_section`, `len(describes) == 1` | no | bundled into the single described component's feature bead |
| `impl_section` | no | implementation detail; owned by the component bead |
| `api` | no | declared external surface; the components in its `provided_by` array carry the work |
| `meta`, `requirement` | no | filtered upstream by NodeMatcher (`structural` skip) |
| `module` | yes, but the row is dead | admitted, yet no change ever carries this node type — see below |

The `module` row admits a node type that never arrives. A diff reports leaves and nothing else, and the node types a leaf is ever given are `meta`, `requirement`, `api`, `component`, `data_flow`, `test_section` and `impl_section`; a module is an interior node of the merkle tree, so it is never diffed as a leaf and no change reaches this gate carrying `module`. The row is therefore unreachable rather than wrong, and no `module` action is ever produced. It is listed all the same, because the gate does admit it: a table that quietly left it out would disagree with the gate it documents, and with the flow leaf, whose shorter gating table carries the same `module` row for the same reason.

The node-type half of that table is consulted on one path only — the unmatched changes NodeMatcher hands over, those with no `.bead-map.json` record. The matched and orphaned paths walk records rather than changes, and a record exists only where a create once ran, so a type the gate has always rejected can never have acquired one and there is nothing there for a gate to reject. The two halves hold each other up: with no record only the unmatched path is reachable, the unmatched path drops the change, and so no record is ever written.

The `describes`-length check is the exception, because a test_section *can* hold a record and later drop to one component. It is applied on the matched path too, and there it means "obsolete the old bead, create no replacement" — the section's coverage folds back into the described component's feature bead. A one-component section earns no bead of its own because its content is read as part of that component's feature bead's work, so a separate bead would be a redundant hand-off; a section describing two or more components can be bundled into no single component bead.

`api` is the one row in this table that has to be here rather than upstream. Merkle classifies an api change as `contract` — the same level as `data_flow`, because an api is a contract — so the `structural` skip that removes `meta` and `requirement` never sees it, and the change arrives at this gate. It is filtered by omission from the bead-producing set: an api names a surface, and every unit of work behind that surface is a component the api's `provided_by` array already points at. A bead per api would duplicate those components' beads and then obsolete-and-recreate them whenever the surface's description changed.

An added, modified or removed api therefore yields zero actions, and the reason is the reachability argument above rather than the gate alone: an added or modified api arrives unmatched and the gate drops it, a removed one has no record to orphan, and neither ever leaves a record behind for the ungated paths to pick up on a later run. That invariant lives only in the absence of an `api` entry in the bead-producing set, so it is pinned by a dedicated test rather than by the shape of the code. The test covers the added and modified cases on the unmatched path — the two where the gate is what makes the difference; a removed api reaches the same gate but would yield nothing even past it.

## State Transition Table

| Spec Change | Existing Bead? | Action |
|-------------|---------------|--------|
| added | no | **create** new bead |
| added | yes (unexpected) | **obsolete** old bead + **create** new bead |
| modified | no | **create** new bead |
| modified | yes | **obsolete** old bead + **create** new bead |
| removed | no | no action |
| removed | yes, open/in_progress | **obsolete** old bead |
| removed | yes, closed | **obsolete** old bead + **create** cleanup bead |

The "review" action is eliminated. Any spec change to a node with an existing bead always obsoletes the old bead. If the node still exists (added or modified), a fresh bead is created.

## DepSpecNodeIDs Collection

After classification, [[a3ecff50de68|each create action collects the identity hashes its bead is to depend on]], from three places:

1. **Component `uses` edges** (direct only): read the component's `uses` array; each entry is a sibling component's identity hash. Add each to `DepSpecNodeIDs`.
2. **Module `requires_module` edges** (transitive, with cycle detection): walk the module dependency graph by identity hash. For each reachable required module, add every component's identity hash from that module to `DepSpecNodeIDs`.
3. **Data_flow add-ons** (contract layer): for each create with node_type=component whose spec_node appears in a changed data_flow's `uses` array, add the data_flow's identity hash to the component's `DepSpecNodeIDs`. This ensures emit's topological sort places the data_flow create op first and the component ops gain `ref:op` deps on it.

The output is a set (deduplicated) of identity hashes. No bead lookup, no status filtering — those belong to emit's Resolver.

## Why DepSpecNodeIDs, Not DepBeadIDs

Prior to this proposal, ActionClassifier resolved deps to bead IDs at impact time by querying the mapping store. When a dep was being obsoleted+recreated in the same batch, the resolver picked up the OLD (soon-closed) bead ID, leading to the broken-dep-graph bug (commit `21defea`). The fix is to defer resolution to emit time where the three-shape ref scheme (ref:op / ref:bead / ref:spec_node) can distinguish same-run work from upstream deps.

ActionClassifier now emits spec_node_ids — identity hashes that stay stable across batches. Emit's Resolver classifies each at emit time with full knowledge of the current batch's op_ids.

## Where classification stops

An `Action` is a decision about *what happened to a spec node*, never an instruction to a tracker. Everything in the project requirement's "deterministic type assignment, parent hierarchy, lineage tracking, and priority propagation" list belongs to emit, not here:

| Concern | Owner |
|---------|-------|
| bead type (`epic` / `feature` / `task`) | the adapter, from the op's `spec_node_kind` |
| `spec_node_kind` on the op | ChangesetBuilder |
| parent (the proposal epic) | Resolver |
| dep refs resolved from a create's `DepSpecNodeIDs`, and whether each takes the `ref:op` / `ref:bead` / `ref:spec_node` shape | Resolver |
| the extra `ref:bead` lineage dep on a modify pair's create op, derived from that create's old bead id | ChangesetBuilder, appending it after Resolver has returned |
| the idempotency label, and the record id looked back up from the create's old bead id so a modify pair reuses one record | IdempotencyLabeler |
| priority, via the `implements → preq_id → priority` chain | Resolver |
| op ordering and `op_id` assignment | TopologicalSorter |

The node-type gate above is the one apparent exception, and it is not one: it decides whether a node produces an action *at all*, which is a property of the spec graph, not of the tracker. The action's node type is carried forward as data so emit can make the type decision — ActionClassifier does not make it.

Holding this line is what lets impact be re-run against a changed tracker state without re-deciding anything structural, and what lets emit be a pure function of `(impact report, bead-map, spec dir, git HEAD)`.

## Interface

One call, over the three lists [[06035e7f0c39|NodeMatcher]] returned together with the current spec graph, returning a flat list of actions. Each action carries:

| Field | Meaning |
|-------|---------|
| type | `create` or `obsolete` |
| bead id | the existing bead, on an obsolete; empty on a create |
| module | the affected module's name, carried alongside the identity hash so output a person reads is legible |
| node | the affected spec node's name |
| node type | the affected spec node's type — the value the gate above is applied to on the unmatched path |
| spec node id | the identity hash of the affected node — the lookup key into the mapping store |
| spec hash | the node's current merkle content hash, on a create |
| old bead id | on a create that replaces an obsoleted bead, the id of the bead it replaces; a modified or unexpectedly-matched-added node produces exactly such a pair |
| dep spec node ids | identity hashes this action's bead is to depend on, left for emit to resolve into refs |
| change type | `modified` or `removed`, on an obsolete; absent on a create |
| reason | one human-readable sentence, per the table below |

The node type travels as its own field because an identity hash does not embed one; neither the change nor the action can recover it from the hash alone.

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified node): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
