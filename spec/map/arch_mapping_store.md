# MappingStore

Read access to the task journal — `spec/.history.jsonl`, the append-only event log that is the
single source of truth for spec-to-task correlation. Parsing, scanning and folding it is the whole
of [[934d627f0e90|storing mapping records]]: the journal remembers every structural event a
baselining absorbed and every task the tracker minted for one, and the store turns that log into
answers.

## Responsibilities

- Parse the journal into typed events, in file order — change events (`added`, `removed`,
  `modified`) and receipt events (`task_created`, `task_closed`, `refresh`)
- Compute the fold: the latest task-bearing event per node, which is the current node-to-task
  linkage every consumer derives on demand — there is no materialized map file to maintain or
  drift
- Provide lookup by identity hash or by task id — the two keys are interchangeable ways to reach
  one node, distinguished by shape (12-hex is a node, anything else is a task)
- Answer for removed nodes: a node's biography survives its removal, so name, node type, module,
  the removing proposal and the bracketing `git_head` refs stay resolvable forever
- [[4aee62bd3c15|Check each line against the journal-line schema]] on the way in, naming the line
  number on violation

The store never writes. `ingest` is the journal's only writer — it appends at baselining with the
same atomic write-and-rename the snapshot uses — and the one-shot backfill that seeded the journal
from the retired bead-map's git history ran once and is history itself. Append-only describes the
format's semantics, not an I/O contract.

The schema check has two audiences with different stakes. The map query surface fails loudly: a
malformed line is an error naming the file and line, and the query never runs. Gating callers — the
diff removal sweep — treat the same condition as journal-absent and degrade detection instead of
failing, because the journal is never load-bearing for the pipeline: it can weaken a sweep to
`unverifiable` notes, it cannot block a gate or fake a pass.

## Event Format

One JSON object per line. Change events record what a baselining absorbed:

```json
{"event": "removed", "eid": "9f2c41a0b7d3", "node": "a1b2c3d4e5f6", "name": "ActionClassifier",
 "node_type": "component", "module": "impact", "before": "e3b0c44298fc", "after": null,
 "path": "impact/arch_action_classifier.md", "git_head": "cafe1234", "proposal": "2026-08-02-merge-impact-emit"}
```

Receipt events record what the tracker did:

```json
{"event": "task_created", "for": "9f2c41a0b7d3", "task_id": "spexmachina-abc"}
{"event": "task_created", "proposal": "2026-04-18-decouple-spex-from-br", "task_id": "spexmachina-0lk"}
{"event": "refresh", "git_head": "cafe1234", "absorbed": ["9f2c41a0b7d3"]}
```

| Field | On | Description |
|-------|----|-------------|
| `event` | all | `added`, `removed`, `modified`, `task_created`, `task_closed`, `refresh` |
| `eid` | change | Event id — deterministic so ingest re-runs append nothing new. Op-born events derive it from `(git_head, op_id)`; refresh-born events, which have no op, derive it from `(node, before, after)` |
| `node` | change | Identity hash of the spec node — byte-identical to the merkle tree key, so every pipeline stage joins on it with no translation |
| `name`, `node_type`, `module` | change | The node's declared identity at event time — the journal is the only record of these once the node is removed |
| `before`, `after` | change | Leaf hashes bracketing the change; `null` marks absence (an add has no before, a removal no after) |
| `git_head` | change, refresh | The commit the changeset carried; refresh records its own when given `--git-head` and its absence otherwise |
| `path` | change, optional | The node's content-leaf path relative to the spec directory, present when the node has one — what makes `git show <head>:<path>` runnable for a removed node. Requirement and api events carry none |
| `proposal` | change, epic receipts | The proposal that drove the change; on a `task_created` with no `for`, the slug identifies a proposal-epic task |
| `for` | receipts | The `eid` of the change event this receipt answers. A modify pair's `task_closed` and `task_created` both reference the pair's `modified` event; a removal's `task_closed` references the `removed` event |
| `task_id` | receipts | The tracker's id, applied by the adapter, reported in receipts |

A pointer discipline holds throughout: the journal stores identity, names, hashes and commit refs —
never leaf content. Showing a change is `git diff <before-head> <after-head> -- <leaf>`, derived
from the refs, so the journal cannot drift from the bytes git already owns.

## Interface

Every request the store answers is a request about events and folds — never about the tracker,
which it does not contact. A caller can

- parse the journal and receive every event in file order;
- ask for the fold and receive one entry per task-bearing node — hash, current task id, and the
  sourcing event;
- resolve one key, hash or task id, to its fold entry — including removed nodes, whose entries
  carry the biography instead of a live task;
- ask for a node's full event history, oldest first, which is how lineage questions ("which tasks
  has this node had?") are answered without any stored back-pointers.

Two of those reads are exposed on the command line unchanged in name: [[38ddf587012f|`spex map get`]]
hands back one fold entry and [[394ec2c8d669|`spex map list`]] hands back the whole fold, each as
JSON. Nothing writes through that surface.

## File Location

The journal is `spec/.history.jsonl`, beside `spec/.snapshot.json` — spec history belongs to the
spec tree, so pointing spex at a different `--spec-dir` points it at that spec's own history. Like
the snapshot it is committed to git and is NOT hashed into the merkle tree — the tree hashes only
the content files module.json declares — so appends move no hash and trigger no diff. The removal
sweep's corpus scan skips dot-prefixed files, which is load-bearing: the journal's own `removed`
events carry exactly the names the sweep hunts, and must never count as survivors. The retired `--map`/`--map-file`
flags are gone with the file they pointed at; the journal's location is a function of `--spec-dir`
alone.

## Design Rationale

### Why an event log instead of a record file?

The retired `.bead-map.json` stored one record per node — current state only, nine denormalized
fields, an integer counter minting `spex:<int>` labels. Three failures were structural: a task for
a *removed* node had no record by design, so cleanup labels resolved to nothing; several tasks
sharing one node across its modify lineage collided on one record; and the validator's removal
sweep depended on a tracker-side artifact for retired names. An event log answers all three because
a task pairs with the *change* that spawned it, not with the node's current state — and beads
already work this way (`issues.jsonl` is append-only; current state is a projection). The fold is
that projection.

### Why no integer ids?

The label is `spex:<spec_node_id>` — the changeset op already carries the node's identity hash, so
idempotency needs no counter, no reservation read, and no coordination. Event ids exist only for
ingest re-run idempotency and derive from the op (`git_head`, `op_id`) or, for refresh-born
events, from the drift itself (`node`, `before`, `after`); nothing outside the journal ever
references them. Legacy `spex:<int>` labels on closed tasks are inert history: the backfill seeded
the journal with every int-to-node pairing that ever existed, so nothing old became unresolvable.

### Why is the store read-only?

One writer means append atomicity is the writer's problem exactly once. `ingest` already owns
baselining — snapshot write, receipt processing — so it owns the journal append in the same
transaction-shaped step, and every other consumer reads a file that is either the pre-baselining
or the post-baselining state, never a torn intermediate.
