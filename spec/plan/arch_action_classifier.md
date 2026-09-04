# ActionClassifier

[[de42d9efa750|Deciding what happened to each affected spec node]] is this component's whole job, and the answer is one of exactly three words: `create`, `close`, or `retarget`. Each create or retarget also collects `DepSpecNodeIDs` — identity hashes of the spec nodes the task will depend on. Task-id resolution is NOT done here; it belongs to the Resolver, downstream in the same pass.

Every decision is made from one bounded input beside the diff and the journal: the task-state artifact, which lists in-flight tasks and nothing else. A pairing's task is therefore in one of three states — `open`, `in_progress`, or absent from the artifact — and there is no fourth. Absence means the task has no live work; completion is never read as a status.

## Responsibilities

- Assign actions based on match results, change types, and — for matched changes — the live pairing's status or its absence.
- Handle a modified node whose pairing's task is absent by generating one plain create — no close of the predecessor, no old task id carried, no lineage dependency; handle a modified node with an open pairing by generating [[7d45c20bd0f7|one retarget action that moves the task's target instead of recreating it]]; refuse the whole run when a modified node's pairing is claimed (`in_progress`).
- Gate action production by node type and, for test_sections, by `len(describes)`.
- Decide a removed node's fate from the same three states, read off [[80afb22dab75|TaskReader]]'s entries joined onto the journal's pairings: an open task is closed, because cancelling live work is a real action; a claimed task refuses the run; [[8987ef169e48|an absent task means the code already shipped]], so the repository now holds code answering to no spec node — or, when the removal is the old half of a rename, code a live node still calls until the new half and its callers land — and a cleanup task is created to have it deleted. Which of the two it is, the classifier cannot tell and does not decide: the diff ties a rename's halves to nothing, and what makes the deletion safe is the cleanup op's place in the last layer and the deps ChangesetBuilder composes from it. There is no close beside a cleanup — nothing is live to close.
- For each create or retarget action, collect `DepSpecNodeIDs` by walking component `uses` (direct), module `requires_module` (transitive) and — for a test_section — its `describes` array.

## Node-Type Gate

A change with no existing task is gated by its node type before the state transition table runs. The task-producing set the gate admits is the resolved profile's plan-relevant declaration rather than a compiled-in constant — the profile declares which types produce tasks and in what order; the tracker type filed for each kind is the adapter's own mapping, not the profile's — and the table below states the default profile's declaration, which reproduces the previous fixed set exactly, so a profile-declared type is admitted or skipped by the same gate reading the same declaration. [[c215b0738f13|The contract layer earns its tasks at this gate]]: a data_flow is admitted with no further condition, and a test_section when it spans two or more components — or when the gate cannot establish how many it spans, because no spec graph or module name reached it, the module name does not resolve to a module, or the section is not declared in the module it does resolve to. Coupling that cannot be established is admitted rather than dropped. Admission is not yet a task — what an admitted change then produces is the state transition table's business.

| NodeType | Admitted by the gate? | Notes |
|----------|-----------------------|-------|
| `component` | yes (feature) | primary work unit |
| `data_flow` | yes | cross-component contract; the gate asks nothing further of it |
| `test_section`, `len(describes) >= 2` | yes (task) | cross-component integration test, needs its own task |
| `test_section`, `len(describes) == 1` | no | bundled into the single described component's feature task |
| `api` | no | declared external surface; the components in its `provided_by` array carry the work |
| `meta`, `requirement` | no | filtered upstream by NodeMatcher (`structural` skip) |
| `module` | yes, but the row is dead | admitted, yet no change ever carries this node type — see below |

The `module` row admits a node type that never arrives. A diff reports leaves and nothing else, and the node types a leaf is ever given are `meta`, `requirement`, `api`, `component`, `data_flow` and `test_section`; a module is an interior node of the merkle tree, so it is never diffed as a leaf and no change reaches this gate carrying `module`. The row is therefore unreachable rather than wrong, and no `module` action is ever produced. It is listed all the same, because the gate does admit it: a table that quietly left it out would disagree with the gate it documents, and with the flow leaf, whose shorter gating table carries the same `module` row for the same reason.

The node-type half of that table is consulted on one path only — the unmatched changes [[972faea162a6|NodeMatcher]] hands over, those with no journal pairing. The matched and orphaned paths walk pairings rather than changes, and a pairing exists only where a create once ran, so a type the gate has always rejected can never have acquired one and there is nothing there for a gate to reject. The two halves hold each other up: with no pairing only the unmatched path is reachable, the unmatched path drops the change, and so no pairing is ever appended.

The `describes`-length check is the exception, because a test_section *can* hold a pairing and later drop to one component. It is applied on the matched path too, and there it means the section's coverage folds back into the described component's feature task, so the section's own task is treated as the task of a removed node: open → close it, `in_progress` → refuse the run, absent → nothing owed. The fold-back is consulted before the status split proper, and it reads the split the removal way because for the section's own task, dropping to one component *is* a removal. A one-component section earns no task of its own because its content is read as part of that component's feature task's work, so a separate task would be a redundant hand-off; a section describing two or more components can be bundled into no single component task.

`api` is the one row in this table that has to be here rather than upstream. Merkle classifies an api change as `contract` — the same level as `data_flow`, because an api is a contract — so the `structural` skip that removes `meta` and `requirement` never sees it, and the change arrives at this gate. It is filtered by omission from the task-producing set: an api names a surface, and every unit of work behind that surface is a component the api's `provided_by` array already points at. A task per api would duplicate those components' tasks and then churn them whenever the surface's description changed.

An added, modified or removed api therefore yields zero actions, and the reason is the reachability argument above rather than the gate alone: an added or modified api arrives unmatched and the gate drops it, a removed one has no pairing to orphan, and neither ever leaves a pairing behind for the ungated paths to pick up on a later run. That invariant lives only in the absence of an `api` entry in the task-producing set, so it is pinned by a dedicated test rather than by the shape of the code. The test covers the added and modified cases on the unmatched path — the two where the gate is what makes the difference; a removed api reaches the same gate but would yield nothing even past it.

## State Transition Table

Two decisions, one input. The first column is what the diff says, the second what the task-state artifact says about the pairing's task, the third the action.

| Spec Change | Existing Task? | Action |
|-------------|---------------|--------|
| added | no | **create** new task |
| added | yes, already tracked (see below) | no action |
| added | yes, hashes differ, task open | **retarget** the task |
| added | yes, hashes differ, task in_progress | **refuse the run** |
| added | yes, hashes differ, task absent from the artifact | **create** new task |
| modified | no | **create** new task |
| modified | yes, already tracked (see below) | no action |
| modified | yes, hashes differ, task open | **retarget** the task |
| modified | yes, hashes differ, task in_progress | **refuse the run** |
| modified | yes, hashes differ, task absent from the artifact | **create** new task |
| removed | no | no action |
| removed | yes, task open | **close** the task |
| removed | yes, task in_progress | **refuse the run** |
| removed | yes, task absent from the artifact | **create** cleanup task |

**Already tracked**: a matched `added` or `modified` change whose pairing's sourcing event
records an `after` hash equal to the change's current hash yields no action. The journal already
pairs a task with exactly this state; the change resurfaced only because a partial run left
the snapshot unsaved, not because new work exists. This one cell is what makes the re-run story
true — the changeset after a partial ingest carries only the ops whose pairings never landed,
so fresh labels can never duplicate work a prior run already created. It is checked
before the status split: an already-tracked node is never retargeted and never re-created,
whatever its status and even when its task has since finished.

**The status split**: a task is work on a change, not on a node's final state, so a genuinely
changed node whose pairing is still `open` (unclaimed) does not spend a task lifecycle — the
existing task's target moves forward. The split is reached only by a node that still owes a task:
the test_section fold-back above is consulted first, and a marked cosmetic change never arrives
here at all — PlanCommand withheld it from matching before any pairing was consulted.
`in_progress` refuses the entire run — the error names every claimed task whose node changed, no
partial action list escapes, and the operator waits for the task to settle or splits the change.
A task absent from the artifact is finished: the change is new work on a node whose earlier work
shipped, and it gets the same plain create a never-tracked node gets. Nothing closes the
predecessor — it is already done, and a close op against it would be a no-op the adapter had to
converge — and nothing ties the new task to it in the tracker: the journal's event chain is the
lineage, and `spex map context`'s bracket is how an implementer reads it.

There is no "unknown status" cell. The artifact is a required input, so every pairing's task is
either listed or not, and "not listed" is a decision, not a gap to default around.

The "review" action stays eliminated. Any spec change to a node with an existing task that is not
already tracked either retargets that task, refuses the run, or opens a successor; nothing is
ever left for a human to re-classify.

## DepSpecNodeIDs Collection

After classification, [[e4234a9928e6|each create or retarget action collects the identity hashes its task is to depend on]], from four places:

1. **Component `uses` edges** (direct only): read the component's `uses` array; each entry is a sibling component's identity hash. Add each to `DepSpecNodeIDs`.
2. **Module `requires_module` edges** (transitive, with cycle detection): walk the module dependency graph by identity hash. For each reachable required module, add every component's identity hash from that module to `DepSpecNodeIDs`.
3. **Data_flow add-ons** (contract layer): for each create with node_type=component whose spec_node appears in a changed data_flow's `uses` array, add the data_flow's identity hash to the component's `DepSpecNodeIDs`. The layer order places the data_flow create op before the component ops; this add-on gives each named component its `ref:op` dep on the flow, and it is what the sorter's forward-dep refusal fires on when a profile inverts that order. The layer edges ChangesetBuilder adds from every component create to every data_flow create make the same order hold for components the flow does not name; this add-on stays as the precise relation beside them, the one edge a reader can trace to a `uses` entry.
4. **Test_section `describes` edges** (direct, unconditional): for a create or retarget whose node is a test_section, read the section's `describes` array; each entry is a described component's identity hash. Add each to `DepSpecNodeIDs`. Collection is deliberately not batch-aware: whether a described component is an in-batch create (`ref:op`), in-flight work (`ref:task`), already shipped (dep dropped) or never tracked (a plan error naming the hash) is the Resolver's existing precedence, unchanged. This is what keeps a multi-component test task off the ready list until the components it describes exist — the sorter's components-before-test-sections layer order was advisory without these deps, and the layer edges now make every layer boundary load-bearing the same way.

The output is a set (deduplicated) of identity hashes. No task lookup, no status filtering — those belong to the Resolver. A retarget's set is recomputed from the node's current edges, exactly as a fresh create's would be; how the adapter applies it (add-only) is the op's contract, not a collection rule.

## Why DepSpecNodeIDs, Not DepTaskIDs

Prior to the decouple proposal, deps were resolved to task ids at classification time by querying the mapping store. When a dep was being replaced in the same batch, the resolver picked up the OLD (soon-closed) task id, leading to the broken-dep-graph bug (commit `21defea`). The fix is to defer resolution to changeset-build time where the ref scheme (ref:op / ref:task) can distinguish same-run work from upstream deps.

ActionClassifier emits spec_node_ids — identity hashes that stay stable across batches. The Resolver classifies each at build time with full knowledge of the current batch's op_ids.

## Where classification stops

An `Action` is a decision about *what happened to a spec node*, never an instruction to a tracker. Everything in the project requirement's "deterministic type assignment, parent hierarchy, and priority propagation" list belongs downstream, not here:

| Concern | Owner |
|---------|-------|
| tracker type (`epic` / `feature` / `task`) | derived once, by the adapter from the op's `spec_node_kind` for the tracker. Ingest derives none: the journal stores the node's `node_type` from the spec graph and never a tracker type |
| `spec_node_kind` on the op | ChangesetBuilder |
| parent (the proposal epic) | Resolver |
| dep refs resolved from an action's `DepSpecNodeIDs`, and whether each takes the `ref:op` or `ref:task` shape | Resolver |
| the idempotency label — `spex:<eid>` of the op's referent journal event — and the retarget op's event label | IdempotencyLabeler and ChangesetBuilder respectively |
| priority, via the `implements → preq_id → priority` chain | Resolver |
| op ordering | TopologicalSorter |
| `op_id` derivation, and the layer edges between adjacent layers | ChangesetBuilder |

No row for lineage: there is none to own. The retired modify pair carried an old task id and a `blocks` dep from successor to predecessor, and both are gone with the pair — a create is a create, whatever the node's history, and the history is the journal's.

The node-type gate above is the one apparent exception, and it is not one: it decides whether a node produces an action *at all*, which is a property of the spec graph, not of the tracker. The action's node type is carried forward as data so the builder can make the type decision — ActionClassifier does not make it.

Holding this line is what lets classification be re-run against a changed tracker state without re-deciding anything structural, and what keeps the whole pass a pure function of `(diff, task-state artifact, journal, spec dir, git HEAD)`.

## Interface

One call, over the three lists [[972faea162a6|NodeMatcher]] returned together with the current spec graph, returning a flat list of actions — or, when a claimed task's node changed or was removed, an error naming every such task and no list at all. Each action carries:

| Field | Meaning |
|-------|---------|
| type | `create`, `close`, or `retarget` |
| task id | the existing task, on a close or retarget; empty on a create — a create never names a prior task |
| module | the affected module's name, carried alongside the identity hash so output a person reads is legible |
| node | the affected spec node's name |
| node type | the affected spec node's type — the value the gate above is applied to on the unmatched path |
| spec node id | the identity hash of the affected node — the join key into the journal fold |
| spec hash | the node's current merkle content hash, on a create or retarget |
| dep spec node ids | identity hashes this action's task is to depend on, left for the Resolver to resolve into refs |
| change type | `modified` or `removed`, on a close; absent otherwise |
| reason | one human-readable sentence, per the table below |

The node type travels as its own field because an identity hash does not embed one; neither the change nor the action can recover it from the hash alone. It travels on every action, matched and unmatched paths alike — the gate above is only the first reader, and the builder reads the same field later to fill `spec_node_kind`, so an action that reached classification with a type must leave it carrying that type unchanged.

The name is resolved, not carried: the diff names a node by identity hash only, so the classifier looks the hash up in the current spec graph — among the module's components, its data flows or its test sections, according to the node type — and takes the declared name it finds. Two cases resolve to nothing and both fall back to printing the identity hash itself: a module the graph does not hold, and a hash the module declares under no section of that type. That fallback is deliberate and is why reasons are never blank; it is reached in normal operation by a removed node, whose declaration is gone from the graph by the time the run reads it, which is why an orphaned pairing supplies the name from the journal instead of asking the graph for one.

## Reason Generation

Each action includes a human-readable reason:
- create (new node): `"New spec node: {module}/{node_name}"`
- create (modified node whose earlier task is finished): `"Spec node modified (new): {module}/{node_name}"`
- close (fold-back of a live test_section): `"Spec node modified: {module}/{node_name}"`
- close (removed node): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`
- retarget: `"Spec node modified (retarget): {module}/{node_name}"`

The two close reasons are contract, not decoration: ingest discriminates a removal close from a fold-back close on their prefixes, because the two construct different journal events.
