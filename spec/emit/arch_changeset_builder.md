# ChangesetBuilder

Composes `changeset.json` v2 from the impact report, the spec graph, the task journal's fold, the proposal ref, and a caller-supplied git HEAD SHA. Those inputs are the whole of what it reads — [[aa2375420738|it opens no other file, starts no subprocess and asks no tracker anything]], so the same inputs always compose the same bytes. The no-subprocess requirement's sole sanctioned exception (the cli upgrade surface) sits outside every emit path, so nothing loosens here. Dep resolution is delegated to [[f7775ac5f1f3|Resolver]], ordering to [[7249fd093b8a|TopologicalSorter]], and idempotency label assignment to [[6f4b6dd8928f|IdempotencyLabeler]].

## Responsibilities

- Load impact report actions (create, obsolete/close) and classify them by spec node type tier (proposal epic / feature+data_flow task / multi-component test task).
- **Detect cleanup actions** by the `"Code cleanup:"` prefix on the action's reason. Cleanup actions get a distinct op shape — see "Cleanup op shape" below.
- Resolve each create action's parent and deps via Resolver into the two ref shapes.
- Order the create ops via TopologicalSorter so in-batch deps come before dependents.
- **Ask IdempotencyLabeler for one label at a time, one create action at a time — never for a block of labels reserved up front.** Every label is `spex:<eid>` of the op's referent journal event; which event that is depends on what the action is (a modify-pair, a cleanup, or a fresh create), not on where the action sits in the ordered batch; see `arch_idempotency_labeler.md` for the referent rules.
- Emit close ops carrying the obsolete labels: `spex:obsolete`, and `commit:<git_head>` built from [[22e63e959749|the SHA the caller passed in]] — the builder never asks git for it.
- Write the final v2 changeset with canonical field order and stable key ordering inside every nested object.

## Interface

The builder is set up once per run from four values that do not change while it runs — the spec graph, the journal fold, the git HEAD SHA and the proposal ref — and is then handed exactly one impact report. It answers with one v2 changeset or with an error, never with both.

The document it answers with carries four top-level fields in this order: the schema version (always `2`), the git HEAD, the proposal ref, and the ordered op list.

## Op Kinds

Four op kinds flow through the builder:

| Op type | Emitted for | Adapter action |
|---------|-------------|----------------|
| `create` | new or replacement spec node | create a bead in the tracker |
| `close`  | obsoleted bead (old modified or removed) | close the bead with labels |
| `label`  | standalone label additions | add labels without create/close |
| `tag`    | proposal tagging on existing beads | add `spec_proposal:<ref>` label |

The current proposal produces mostly create and close ops plus the initial proposal-epic create. Label and tag are reserved for future flows (e.g., cross-proposal tagging).

Those four names are [[de8ba472d9f6|the whole vocabulary a changeset may use]], and the bar that requirement sets is that no tracker-specific flag or subcommand name appears in `changeset.json`; the adapter column above is a mapping the adapter owns, not something the changeset states. That is what lets one changeset drive an adapter for a different tracker unchanged.

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

The vocabulary is closed, and `api` is deliberately not in it. An api is a declared external surface, not a unit of work, and impact produces no action for one, so no action carrying an api ever reaches `Build()`.

The closure is upstream, not here. On a conventional create the builder copies `Action.NodeType` into `spec_node_kind` verbatim — it overrides the value only for the cleanup shape below — so it validates nothing against the table above. An api action that did reach `Build()` would emit `"spec_node_kind": "api"`, a value outside the table, and the adapter's kind-to-bead-type mapping would fall through its default arm and file the surface as a `feature` bead. The guarantee is therefore ActionClassifier's, not the builder's: it holds because impact never emits an api action, not because emit would refuse one. A future kind added to the table has to be added there too.

Declaring, describing or retiring a surface therefore produces an empty changeset unless a component leaf moved alongside it — which is the intended shape, because the work behind a surface belongs to the components its `provided_by` array names.

## Cleanup op shape

Cleanup actions — those whose reason starts with `"Code cleanup:"` — are emitted with a distinct op shape rather than the conventional component/data_flow form. That reason prefix is the same discriminator the retired apply path used; emit lifts it out of the implementation layer and into the changeset contract:

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
| `labels`          | `["spex:cleanup"]` — a cleanup create is the only create-class op that carries labels, so the adapter can stamp the discriminator label on the bead as it creates it. Close ops carry labels too; conventional creates do not. |
| `body`            | empty                                                                                       |

Modify-pair creates — a create paired with the close of the bead it replaces, with no cleanup reason — keep the conventional shape, and their `idempotency.label` is `spex:<eid>` of this run's `modified` event, distinct from the closed predecessor's label by construction — each change in a node's lineage references its own event, so no lookup and no reuse rule is needed and the old label can never collide. Every create that replaces an obsoleted bead, cleanup and modify-pair alike, also carries one extra dep naming that old bead with edge type `blocks`, so the replacement's lineage survives in the tracker after the close op runs.

## Op Shape

```json
{
  "op_id": "op-0004",
  "type": "create",
  "spec_node_kind": "component",
  "spec_node_id": "7f06f7d80e94",
  "idempotency": { "label": "spex:deadbeefcafe1234:op-0004" },
  "parent": { "ref": "op", "op_id": "op-0001" },
  "deps": [
    { "ref": "bead", "bead_id": "spexmachina-ab1" },
    { "ref": "op", "op_id": "op-0003" }
  ],
  "priority": 1,
  "title": "emit: ChangesetBuilder",
  "body": "…"
}
```

Close ops omit `deps`/`parent` and add `target` + `labels`:

```json
{
  "op_id": "op-0042",
  "type": "close",
  "target": { "ref": "bead", "bead_id": "spexmachina-tjs" },
  "labels": ["spex:obsolete", "commit:deadbeefcafe1234"],
  "reason": "Spec node modified: apply/ApplyCommand"
}
```

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

- Field order inside every op is fixed (op_id, type, spec_node_kind, spec_node_id, idempotency, parent, deps, priority, title, body / target, labels, reason).
- `ops` array preserves the order produced by TopologicalSorter — never re-sorted at write time.
- Op ids are `op-<n>`, numbered from 1 in that order — the creates first, then one per close — and zero-padded to the digit width of the changeset's total op count. Nine ops number `op-1` through `op-9`; forty number `op-01` through `op-40`; a changeset reaching four digits numbers `op-0001` upward. The width is computed here, over creates and closes together — TopologicalSorter is handed the creates alone and so cannot compute it.
- JSON is indented 2 spaces, LF-only newlines, no trailing whitespace.
- Characters carrying an HTML meaning are written through as themselves: a title or body holding `<`, `>` or `&` appears as that character, not as a numeric escape.

Together those five rules are [[240f59bf64f6|what makes two runs over identical inputs byte-identical]], which is what lets `changeset.json` be reviewed in a git diff rather than merely re-parsed.

## Error Handling

- Any error from Resolver, TopologicalSorter or IdempotencyLabeler aborts the build; no partial changeset is ever produced.
- Missing `git_head` is checked by EmitCommand before Builder runs.
- An impact report with top-level errors is rejected upstream (EmitCommand); Builder assumes a clean report.

Every error a sub-component raises reaches the caller carrying an
`emit: build:` prefix, so the line `spex emit` prints on stderr names the
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
implementation in `emit/{builder,resolver,sorter,labeler}_test.go` and are
bundled with that component's own implementation bead.
