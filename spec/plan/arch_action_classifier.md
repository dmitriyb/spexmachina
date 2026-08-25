# ActionClassifier

[[de42d9efa750|Deciding what happened to each affected spec node]] is this component's whole job, and the answer is one of exactly three words: `create`, `obsolete`, or `retarget`. Each create or retarget also collects `DepSpecNodeIDs` — identity hashes of the spec nodes the task will depend on. Bead-ID resolution is NOT done here; it belongs to the Resolver, downstream in the same pass.

## Responsibilities

- Assign actions based on match results, change types, and — for matched changes — the live pairing's tracker status.
- Handle modified nodes with a closed pairing by generating both obsolete (old bead) and create (new bead) actions; handle modified nodes with an open pairing by generating [[7d45c20bd0f7|one retarget action that moves the task's target instead of recreating it]]; refuse the whole run when a modified node's pairing is claimed (`in_progress`).
- Gate action production by node type and, for test_sections, by `len(describes)`.
- Choose between obsolete-only and obsolete-plus-cleanup-create for a removed node from the live bead status [[9f1578d7af6d|BeadReader]] carried off the caller's tracker file. [[8987ef169e48|A closed bead means the code already shipped]], so the repository now holds code answering to no spec node and a cleanup bead is created to have it deleted; an open or in-progress bead means nothing shipped and there is nothing to clean up.
- For each create or retarget action, collect `DepSpecNodeIDs` by walking component `uses` (direct), module `requires_module` (transitive) and — for a test_section — its `describes` array.

## Node-Type Gate

A change with no existing bead is gated by its node type before the state transition table runs. [[3ec0a433e476|The contract layer earns its beads at this gate]]: a data_flow is admitted with no further condition, and a test_section when it spans two or more components — or when the gate cannot establish how many it spans, because no spec graph or module name reached it, the module name does not resolve to a module, or the section is not declared in the module it does resolve to. Coupling that cannot be established is admitted rather than dropped. Admission is not yet a bead — what an admitted change then produces is the state transition table's business.

| NodeType | Admitted by the gate? | Notes |
|----------|-----------------------|-------|
| `component` | yes (feature) | primary work unit |
| `data_flow` | yes | cross-component contract; the gate asks nothing further of it |
| `test_section`, `len(describes) >= 2` | yes (task) | cross-component integration test, needs its own bead |
| `test_section`, `len(describes) == 1` | no | bundled into the single described component's feature bead |
| `api` | no | declared external surface; the components in its `provided_by` array carry the work |
| `meta`, `requirement` | no | filtered upstream by NodeMatcher (`structural` skip) |
| `module` | yes, but the row is dead | admitted, yet no change ever carries this node type — see below |

The `module` row admits a node type that never arrives. A diff reports leaves and nothing else, and the node types a leaf is ever given are `meta`, `requirement`, `api`, `component`, `data_flow` and `test_section`; a module is an interior node of the merkle tree, so it is never diffed as a leaf and no change reaches this gate carrying `module`. The row is therefore unreachable rather than wrong, and no `module` action is ever produced. It is listed all the same, because the gate does admit it: a table that quietly left it out would disagree with the gate it documents, and with the flow leaf, whose shorter gating table carries the same `module` row for the same reason.

The node-type half of that table is consulted on one path only — the unmatched changes [[972faea162a6|NodeMatcher]] hands over, those with no journal pairing. The matched and orphaned paths walk pairings rather than changes, and a pairing exists only where a create once ran, so a type the gate has always rejected can never have acquired one and there is nothing there for a gate to reject. The two halves hold each other up: with no pairing only the unmatched path is reachable, the unmatched path drops the change, and so no pairing is ever appended.

The `describes`-length check is the exception, because a test_section *can* hold a pairing and later drop to one component. It is applied on the matched path too, and there it means "obsolete the old bead, create no replacement" — the section's coverage folds back into the described component's feature bead. The fold-back is consulted before the status split: a section dropped to one component is obsoleted whatever its task's status — open, claimed or closed — because the node no longer owes a task of its own, so there is nothing to retarget and no claimed target to protect. That is the pre-retarget behavior, unchanged. A one-component section earns no bead of its own because its content is read as part of that component's feature bead's work, so a separate bead would be a redundant hand-off; a section describing two or more components can be bundled into no single component bead.

`api` is the one row in this table that has to be here rather than upstream. Merkle classifies an api change as `contract` — the same level as `data_flow`, because an api is a contract — so the `structural` skip that removes `meta` and `requirement` never sees it, and the change arrives at this gate. It is filtered by omission from the bead-producing set: an api names a surface, and every unit of work behind that surface is a component the api's `provided_by` array already points at. A bead per api would duplicate those components' beads and then churn them whenever the surface's description changed.

An added, modified or removed api therefore yields zero actions, and the reason is the reachability argument above rather than the gate alone: an added or modified api arrives unmatched and the gate drops it, a removed one has no pairing to orphan, and neither ever leaves a pairing behind for the ungated paths to pick up on a later run. That invariant lives only in the absence of an `api` entry in the bead-producing set, so it is pinned by a dedicated test rather than by the shape of the code. The test covers the added and modified cases on the unmatched path — the two where the gate is what makes the difference; a removed api reaches the same gate but would yield nothing even past it.

## State Transition Table

| Spec Change | Existing Bead? | Action |
|-------------|---------------|--------|
| added | no | **create** new bead |
| added | yes, already tracked (see below) | no action |
| added | yes, hashes differ, task open | **retarget** the task |
| added | yes, hashes differ, task in_progress | **refuse the run** |
| added | yes, hashes differ, task closed | **obsolete** old bead + **create** new bead |
| modified | no | **create** new bead |
| modified | yes, already tracked (see below) | no action |
| modified | yes, hashes differ, task open | **retarget** the task |
| modified | yes, hashes differ, task in_progress | **refuse the run** |
| modified | yes, hashes differ, task closed | **obsolete** old bead + **create** new bead |
| removed | no | no action |
| removed | yes, open/in_progress | **obsolete** old bead |
| removed | yes, closed | **obsolete** old bead + **create** cleanup bead |

**Already tracked**: a matched `added` or `modified` change whose open pairing's sourcing event
records an `after` hash equal to the change's current hash yields no action. The journal already
pairs a live task with exactly this state; the change resurfaced only because a partial run left
the snapshot unsaved, not because new work exists. This one cell is what makes the re-run story
true — the changeset after a partial ingest carries only the ops whose pairings never landed,
so fresh labels can never obsolete-and-duplicate work a prior run already created. It is checked
before the status split: an already-tracked node is never retargeted, whatever its status.

**The status split**: a task is work on a change, not on a node's final state, so a genuinely
changed node whose pairing is still `open` (unclaimed) does not spend a bead lifecycle — the
existing task's target moves forward. The split is reached only by a node that still owes a task:
the test_section fold-back above is consulted first, and a marked cosmetic change never arrives
here at all — PlanCommand withheld it from matching before any pairing was consulted. A pairing whose status never joined (no `--beads` file, or
the bead absent from the listing) is not known-open and takes the closed path's obsolete+create,
the direction that never moves a task silently. `in_progress` refuses the entire run — the error
names every claimed task whose node changed, no partial action list escapes, and the operator
waits for the task to settle or splits the change. `closed` keeps today's lineage: obsolete plus
successor create.

The "review" action stays eliminated. Any spec change to a node with an existing bead that is not
already tracked either retargets that bead or obsoletes it; nothing is ever left for a human to
re-classify.

## DepSpecNodeIDs Collection

After classification, [[e4234a9928e6|each create or retarget action collects the identity hashes its task is to depend on]], from four places:

1. **Component `uses` edges** (direct only): read the component's `uses` array; each entry is a sibling component's identity hash. Add each to `DepSpecNodeIDs`.
2. **Module `requires_module` edges** (transitive, with cycle detection): walk the module dependency graph by identity hash. For each reachable required module, add every component's identity hash from that module to `DepSpecNodeIDs`.
3. **Data_flow add-ons** (contract layer): for each create with node_type=component whose spec_node appears in a changed data_flow's `uses` array, add the data_flow's identity hash to the component's `DepSpecNodeIDs`. This ensures the topological sort places the data_flow create op first and the component ops gain `ref:op` deps on it.
4. **Test_section `describes` edges** (direct, unconditional): for a create or retarget whose node is a test_section, read the section's `describes` array; each entry is a described component's identity hash. Add each to `DepSpecNodeIDs`. Collection is deliberately not batch-aware: whether a described component is an in-batch create (`ref:op`), in-flight work (`ref:bead`), already shipped (dep dropped) or never tracked (a plan error naming the hash) is the Resolver's existing precedence, unchanged. This is what keeps a multi-component test task off the ready list until the components it describes exist — the sorter's features-before-test-tasks tier order was advisory without these deps and is load-bearing with them.

The output is a set (deduplicated) of identity hashes. No bead lookup, no status filtering — those belong to the Resolver. A retarget's set is recomputed from the node's current edges, exactly as a fresh create's would be; how the adapter applies it (add-only) is the op's contract, not a collection rule.

## Why DepSpecNodeIDs, Not DepBeadIDs

Prior to the decouple proposal, deps were resolved to bead IDs at classification time by querying the mapping store. When a dep was being obsoleted+recreated in the same batch, the resolver picked up the OLD (soon-closed) bead ID, leading to the broken-dep-graph bug (commit `21defea`). The fix is to defer resolution to changeset-build time where the ref scheme (ref:op / ref:bead) can distinguish same-run work from upstream deps.

ActionClassifier emits spec_node_ids — identity hashes that stay stable across batches. The Resolver classifies each at build time with full knowledge of the current batch's op_ids.

## Where classification stops

An `Action` is a decision about *what happened to a spec node*, never an instruction to a tracker. Everything in the project requirement's "deterministic type assignment, parent hierarchy, lineage tracking, and priority propagation" list belongs downstream, not here:

| Concern | Owner |
|---------|-------|
| bead type (`epic` / `feature` / `task`) | derived once, by the adapter from the op's `spec_node_kind` for the tracker bead. Ingest derives none: the journal stores the node's `node_type` from the spec graph and never a bead type, so the retired two-table disagreement (adapter and Reconciler classifying `data_flow` and unnamed kinds differently) is gone with the record that stored the result |
| `spec_node_kind` on the op | ChangesetBuilder |
| parent (the proposal epic) | Resolver |
| dep refs resolved from an action's `DepSpecNodeIDs`, and whether each takes the `ref:op` or `ref:bead` shape | Resolver |
| the extra `ref:bead` lineage dep on a modify pair's create op, derived from that create's old bead id | ChangesetBuilder, appending it after Resolver has returned |
| the idempotency label — `spex:<eid>` of the op's referent journal event, distinct across a modify pair by construction — and the retarget op's event label | IdempotencyLabeler and ChangesetBuilder respectively |
| priority, via the `implements → preq_id → priority` chain | Resolver |
| op ordering and `op_id` assignment | TopologicalSorter |

The node-type gate above is the one apparent exception, and it is not one: it decides whether a node produces an action *at all*, which is a property of the spec graph, not of the tracker. The action's node type is carried forward as data so the builder can make the type decision — ActionClassifier does not make it.

Holding this line is what lets classification be re-run against a changed tracker state without re-deciding anything structural, and what keeps the whole pass a pure function of `(diff, tracker listing, journal, spec dir, git HEAD)`.

## Interface

One call, over the three lists [[972faea162a6|NodeMatcher]] returned together with the current spec graph, returning a flat list of actions — or, when a claimed task's node changed, an error naming every such task and no list at all. Each action carries:

| Field | Meaning |
|-------|---------|
| type | `create`, `obsolete`, or `retarget` |
| bead id | the existing bead, on an obsolete or retarget; empty on a create |
| module | the affected module's name, carried alongside the identity hash so output a person reads is legible |
| node | the affected spec node's name |
| node type | the affected spec node's type — the value the gate above is applied to on the unmatched path |
| spec node id | the identity hash of the affected node — the join key into the journal fold |
| spec hash | the node's current merkle content hash, on a create or retarget |
| old bead id | on a create that replaces an obsoleted bead, the id of the bead it replaces |
| dep spec node ids | identity hashes this action's task is to depend on, left for the Resolver to resolve into refs |
| change type | `modified` or `removed`, on an obsolete; absent otherwise |
| reason | one human-readable sentence, per the table below |

The node type travels as its own field because an identity hash does not embed one; neither the change nor the action can recover it from the hash alone. It travels on every action, matched and unmatched paths alike — the gate above is only the first reader, and the builder reads the same field later to fill `spec_node_kind`, so an action that reached classification with a type must leave it carrying that type unchanged.

The name is resolved, not carried: the diff names a node by identity hash only, so the classifier looks the hash up in the current spec graph — among the module's components, its data flows or its test sections, according to the node type — and takes the declared name it finds. Two cases resolve to nothing and both fall back to printing the identity hash itself: a module the graph does not hold, and a hash the module declares under no section of that type. That fallback is deliberate and is why reasons are never blank; it is reached in normal operation by a removed node, whose declaration is gone from the graph by the time the run reads it, which is why an orphaned pairing supplies the name from the journal instead of asking the graph for one.

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified node): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
- retarget: `"Spec node modified (retarget): {module}/{node_name}"`
