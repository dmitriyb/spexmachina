# ChangesetBuilder

Composes `changeset.json` v3 from the classified actions, the spec graph, the task journal's fold, the run's registration, the proposal ref, the absorb list, and a caller-supplied git HEAD SHA. Those inputs are the whole of what it reads — [[cf4f1ab8264a|it opens no other file, starts no subprocess and asks no tracker anything]], so the same inputs always compose the same bytes. The no-subprocess requirement's sole sanctioned exception (the cli upgrade surface) sits outside every plan path, so nothing loosens here. The actions come straight from [[8aa1ab5ac102|ActionClassifier]] — there is no intermediate document between deciding what happened and composing what to do about it. Dep resolution is delegated to [[e9a3b1b85953|Resolver]], ordering to [[659abe167891|TopologicalSorter]], and idempotency label assignment to [[6efd7f8ebdb2|IdempotencyLabeler]].

## Responsibilities

- Take the classified actions (create, obsolete/close, retarget) and classify the creates by spec node type tier (proposal epic / feature+data_flow task / multi-component test task).
- **Detect cleanup actions** by the `"Code cleanup:"` prefix on the action's reason. Cleanup actions get a distinct op shape — see "Cleanup op shape" below.
- Resolve each create action's parent and deps — and each retarget action's deps — via Resolver into the two ref shapes.
- Order the create ops via TopologicalSorter so in-batch deps come before dependents.
- **Ask IdempotencyLabeler for one label at a time, one create action at a time — never for a block of labels reserved up front.** Every label is `spex:<eid>` of the op's referent journal event; which event that is depends on what the action is (a modify-pair, a cleanup, or a fresh create), not on where the action sits in the ordered batch; see `arch_idempotency_labeler.md` for the referent rules. Eids embed [[26b2fdc6e7ea|the SHA the caller passed in]] — the builder never asks git for it.
- Emit retarget ops for retarget actions — target ref, new content hash, the run's `modified`-event label, recomputed deps — per "Retarget op shape" below.
- Emit close ops as target and reason alone — no labels. The retired `spex:obsolete` / `commit:<HEAD>` markers are not emitted: close idempotency keys on the tracker's own status, and a run's provenance lives in the changeset's top-level `git_head`, not on individual ops.
- Write the absorbed entries PlanCommand composed into the top-level `absorbed` array — see "Absorbed array" below.
- Write the final v3 changeset with canonical field order and stable key ordering inside every nested object.

## Interface

The builder is set up once per run from six values that do not change while it runs — the spec graph, the journal fold, the run's registration, the git HEAD SHA, the proposal ref and the composed absorbed entries — and is then handed exactly one batch of classified actions. It answers with one v3 changeset or with an error, never with both. Every one of those six arrives finished: the fold, the registration and the absorbed entries were read by PlanCommand from the journal and absorb file at their resolved locations, so the builder is indifferent to where the project keeps its state — the lifecycle module's location resolution is invisible from here, and no relocation of the state directory can reach into the composition.

The document it answers with carries five top-level fields in this order: the schema version (always `3`), the git HEAD, the proposal ref, the ordered op list, and the absorbed array.

## Op Kinds

Five op kinds flow through the builder:

| Op type | Emitted for | Adapter action |
|---------|-------------|----------------|
| `create` | new or replacement spec node | create a bead in the tracker |
| `close`  | obsoleted bead (old modified-with-closed-pairing, or removed) | close the bead |
| `retarget` | a modified node whose open task moves to the new state | update the bead: add the event label, add missing deps |
| `label`  | standalone label additions | add labels without create/close |
| `tag`    | proposal tagging on existing beads | add `spec_proposal:<ref>` label |

Label and tag are reserved for future flows (e.g., cross-proposal tagging).

Those five names are [[77f5182ac5f7|the whole vocabulary a changeset may use]], and the bar that requirement sets is that no tracker-specific flag or subcommand name appears in `changeset.json`; the adapter column above is a mapping the adapter owns, not something the changeset states. That is what lets one changeset drive an adapter for a different tracker unchanged. Adding `retarget` to the vocabulary is what forced [[7f275787df34|the version bump to 3]]: a v2 consumer meeting an op type it does not know must refuse the document, never silently drop the op.

### `spec_node_kind` vocabulary

Create ops carry `spec_node_kind` matching the underlying spec node category:

| Kind | When emitted | Bead-type via adapter mapping |
|------|--------------|-------------------------------|
| `proposal_epic` | First op of a fresh proposal — represents the proposal as an epic bead | `epic` |
| `component`     | New or replacement component arch leaf | `feature` |
| `data_flow`     | New or replacement data_flow leaf | `task` |
| `test_section`  | New or replacement test_section leaf with `describes >= 2` | `task` |
| `cleanup`       | Code-cleanup bead for a removed spec node — see "Cleanup op shape" below | `task` |

The `cleanup` value is what tells the adapter and Reconciler that this op's receipt pairs with the prior removed event in the journal rather than with a fresh change event.

Which node types produce beads is the resolved profile's plan-relevant declaration; the bead-type column is the adapter's own kind-to-bead-type mapping, never the profile's or the changeset's. The table's kinds are nonetheless the only ones a changeset can carry today: TopologicalSorter tiers only this vocabulary and refuses a batch holding a create whose spec node kind belongs to no tier, so a profile-declared plan-relevant type outside it makes `Build()` return an error naming the kind, and no changeset is written — the op never reaches an adapter. Extending the tier assignment to profile-declared types is a change to the sorter's own contract, made in the authoring loop. Under the default profile the kinds emitted are exactly the rows above.

The vocabulary is closed per profile, and `api` is deliberately not in the default's. An api is a declared external surface, not a unit of work, and the classifier produces no action for one, so no action carrying an api ever reaches `Build()`.

The closure is upstream, not here. On a conventional create the builder copies `Action.NodeType` into `spec_node_kind` verbatim — it overrides the value only for the cleanup shape below — so it validates nothing against the table above. An api action that did reach `Build()` would carry `"spec_node_kind": "api"`, a value outside the table and outside the sorter's tier vocabulary, so the sorter would refuse the batch and `Build()` would error rather than emit the op. The guarantee is therefore ActionClassifier's first and TopologicalSorter's as backstop, never the builder's: the builder copies the value verbatim and refuses nothing itself. A future kind added to the table has to be added to the classifier's admitted set and the sorter's tiers too.

Declaring, describing or retiring a surface therefore produces an empty changeset unless a component leaf moved alongside it — which is the intended shape, because the work behind a surface belongs to the components its `provided_by` array names.

## Retarget op shape

A retarget action — a modified node whose live pairing's task is open — is emitted as its own op type:

Rows are in the canonical field order "Canonical Output" fixes below, as they are in every shape table on this page — one order governs every op kind, and a table that enumerated its fields in some other order would read as a second, competing rule.

| Field | Retarget op value |
|-------|-------------------|
| `type` | `"retarget"` |
| `spec_node_id` | the identity hash of the modified node |
| `spec_hash` | the node's new content hash |
| `deps` | the recomputed refs from Resolver — applied add-only by the adapter; nothing here expresses removal |
| `target` | `{ref:bead, bead_id:"<the open task>"}` — the task whose target moves |
| `labels` | `["spex:<eid>"]` of this run's `modified` event, derived from `(git_head, op_id)` like every node-bearing create's label. It rides in `labels`, not under `idempotency`: updates are naturally idempotent, so there is nothing to probe for |
| `reason` | the action's reason verbatim |

No `parent`, no `priority`, no `title`, no `body`: the task already exists with all four, and a retarget moves only its target state and deps. No close accompanies it and no `blocks` lineage dep is minted — the generations that lineage edge used to connect no longer exist as separate beads, and the accumulation is readable from the journal's `task_retargeted` receipts instead.

## Cleanup op shape

Cleanup actions — those whose reason starts with `"Code cleanup:"` — are emitted with a distinct op shape rather than the conventional component/data_flow form. That reason prefix is the same discriminator the retired apply path used; the builder lifts it out of the implementation layer and into the changeset contract:

| Field             | Cleanup op value                                                                            |
|-------------------|---------------------------------------------------------------------------------------------|
| `type`            | `"create"`                                                                                  |
| `spec_node_kind`  | `"cleanup"` (NOT the underlying spec node's kind — adapter and reconciler key off this)     |
| `spec_node_id`    | the identity hash of the now-removed spec node, for traceability                            |
| `idempotency.label` | `"spex:<eid>"` of the removal event the cleanup answers — a prior removal read from the fold, or the one the same-batch close implies — per the labeler's cleanup referent rule |
| `parent`          | proposal-epic ref (same as other creates)                                                   |
| `deps`            | one `bead` ref to the removed node's old bead, edge type `blocks` — lineage                 |
| `priority`        | `3`, the fallback                                                                           |
| `title`           | the action's reason verbatim (e.g. `"Code cleanup: m/X"`) — NOT `"<module>: <node>"`         |
| `body`            | empty                                                                                       |
| `labels`          | not populated — the retired `spex:cleanup` discriminator is gone. What marks the op as cleanup is its `spec_node_kind`, and what answers "is this bead cleanup?" afterwards is the journal: its `task_created` references a `removed` event. `Op.Labels` is populated only on retarget ops. |

Modify-pair creates — a create paired with the close of the bead it replaces, minted only when the old pairing is closed, with no cleanup reason — keep the conventional shape, and their `idempotency.label` is `spex:<eid>` of this run's `modified` event, distinct from the closed predecessor's label by construction — each change in a node's lineage references its own event, so no lookup and no reuse rule is needed and the old label can never collide. Every create that replaces an obsoleted bead, cleanup and modify-pair alike, also carries one extra dep naming that old bead with edge type `blocks`, so the replacement's lineage survives in the tracker after the close op runs.

## Absorbed array

A marked node produces no op at all — [[049e8ae9cc51|cosmetic means the task owes nothing]] — and by the time the builder runs there is no action for one to suppress: PlanCommand withheld every marked change from matching and classification and composed the absorbed entries itself, so neither a retarget nor a modify pair nor a refusal can arise from a marked node, whatever its pairing's status. What the document carries instead is one entry per mark in the top-level `absorbed` array: the node's identity hash, its before and after content hashes from the diff, and the authored reason copied from the absorb file. The builder's whole contribution is writing those finished entries in canonical field order. The adapter ignores the array entirely; ingest consumes it, minting the `modified` events — eids derived from `(node, before, after)` exactly as refresh-born events are — and closing them with the existing `refresh` receipt.

The marking rules are enforced upstream too: PlanCommand refuses, exit 2, a mark on an `added` or `removed` node or on one absent from the diff. The judgment of *whether* an edit is cosmetic never enters the run at all — the absorb file is an authored, git-committed, reviewed input, and the pipeline's contribution is copying its reasons into an auditable place.

## Op Shape

```json
{
  "op_id": "op-0004",
  "type": "create",
  "spec_node_kind": "component",
  "spec_node_id": "4c1146bb7287",
  "idempotency": { "label": "spex:deadbeefcafe1234:op-0004" },
  "parent": { "ref": "op", "op_id": "op-0001" },
  "deps": [
    { "ref": "bead", "bead_id": "spexmachina-ab1" },
    { "ref": "op", "op_id": "op-0003" }
  ],
  "priority": 1,
  "title": "plan: ChangesetBuilder",
  "body": "…"
}
```

Close ops omit `deps`/`parent` and add `target`; they carry no `labels`:

```json
{
  "op_id": "op-0042",
  "type": "close",
  "target": { "ref": "bead", "bead_id": "spexmachina-tjs" },
  "reason": "Spec node modified: apply/ApplyCommand"
}
```

Retarget ops carry `spec_node_id`, `spec_hash`, `deps`, `target`, `labels` and `reason` per the shape table above — `deps` ahead of `target`, as the canonical order has it for every op kind alike.

## Title and body

Every create op carries a title the adapter files as the bead's title without editing it:

| Create for | Title |
|------------|-------|
| a component | `<module>: <component name>` |
| a data_flow | `<module>: data_flow <flow name>` |
| a multi-component test section | `<module>: test <test name>` |
| the proposal epic | `Proposal: <proposal ref>` |

A cleanup create is the one exception and takes its reason verbatim instead, per "Cleanup op shape" above.

The body is the spec context a reader of the bead needs and nothing beyond it: the repo-relative path of the node's own content leaf, and the path of the `module.json` that declares it. A node with no content leaf on disk — the proposal epic — gets an empty body, and so does every cleanup create. The body is markdown; the adapter passes it through to the tracker's description field unchanged.

## Canonical Output

- Field order inside every op is fixed — op_id, type, spec_node_kind, spec_node_id, spec_hash, idempotency, parent, deps, priority, title, body, target, labels, reason. It is one sequence, not a per-kind choice: each op writes the fields its kind carries and omits the rest, in that order, so a retarget's `deps` precede its `target` exactly as a create's `deps` precede a close's `target`. No op kind reorders, and no shape table on this page states an order of its own.
- `ops` array preserves the order produced by TopologicalSorter — never re-sorted at write time; the retarget ops follow the creates, and the close ops follow the retargets, each block in the classifier's deterministic action order.
- Op ids are `op-<n>`, numbered from 1 in that order — the creates first, then the retargets, then the closes — and zero-padded to the digit width of the changeset's total op count. Nine ops number `op-1` through `op-9`; forty number `op-01` through `op-40`; a changeset reaching four digits numbers `op-0001` upward. The width is computed here, over every op kind together — TopologicalSorter is handed the creates alone and so cannot compute it.
- JSON is indented 2 spaces, LF-only newlines, no trailing whitespace.
- Characters carrying an HTML meaning are written through as themselves: a title or body holding `<`, `>` or `&` appears as that character, not as a numeric escape.

Together those five rules are [[7f275787df34|what makes two runs over identical inputs byte-identical]], which is what lets `changeset.json` be reviewed in a git diff rather than merely re-parsed. On a batch with no retarget-eligible modifications and no absorb file, the output is the v2 pipeline's output modulo the version field: same ops, same order, same labels.

## Error Handling

- Any error from Resolver, TopologicalSorter or IdempotencyLabeler aborts the build; no partial changeset is ever produced.
- Absorb-entry validation happens upstream in PlanCommand; no absorb rule is checked here, and the entries arrive finished.
- Missing `git_head` is checked by PlanCommand before Builder runs.
- A diff carrying errors is rejected upstream (PlanCommand); Builder assumes clean actions.

Every error a sub-component raises reaches the caller carrying a
`plan: build:` prefix, so the line `spex plan` prints on stderr names the
build stage before it names the failure.

## Test surface

ChangesetBuilder, Resolver, TopologicalSorter, and IdempotencyLabeler form
a closed four-component composition: Builder is the only consumer of the
other three (per the module's `uses` graph), and the three subordinates
have no public API surface independent of `Builder.Build()`. Cross-component
integration coverage therefore lives in a single `test_changeset_builder`
test_section whose `describes` array names all four components — exercised
through `Builder.Build()`'s public API rather than against each subordinate
in isolation. Per-method unit tests for each component live alongside its
implementation in `plan/{builder,resolver,sorter,labeler}_test.go` and are
bundled with that component's own implementation bead.
