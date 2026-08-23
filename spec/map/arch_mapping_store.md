# MappingStore

Owner of the task journal — the append-only event log that is the
single source of truth for spec-to-task correlation. Parsing, scanning, folding and appending it
is the whole of [[934d627f0e90|storing mapping records]]: the journal remembers every structural
event a baselining absorbed, every proposal lifecycle a registration opened, and every task the
tracker minted, and the store turns that log into answers.

## Responsibilities

- Parse the journal into typed events, in file order — change events (`added`, `removed`,
  `modified`), the `registered` event, and receipt events (`task_created`, `task_closed`,
  `task_retargeted`, `refresh`)
- Compute the fold: the latest task-bearing event per node, which is the current node-to-task
  linkage every consumer derives on demand — there is no materialized map file to maintain or
  drift. [[76fe608c3a40|`task_retargeted` is task-bearing exactly as `task_created` is]]:
  latest-wins moves the pairing's sourcing event forward to the retargeted event's referent
  while the task id stays put, so the fold always answers with the newest state a task owes
- Provide lookup by identity hash or by task id — the two keys are interchangeable ways to reach
  one node, distinguished by shape (12-hex is a node, anything else is a task)
- Answer for removed nodes: a node's biography survives its removal, so name, node type, module,
  the removing proposal and the bracketing `git_head` refs stay resolvable forever. Both keys
  reach it: the identity hash, and the id of the cleanup task born to answer the removal
- Carry, on a removed entry, the cleanup task's id and not the one the node held while it was
  live — the removal supersedes the pre-removal pairing, which stays in the journal as lineage
  and is reachable through the event history rather than through the fold. A removal no cleanup
  task answers carries no task id at all
- [[4aee62bd3c15|Check each line against the journal-line schema]] on the way in, naming the line
  number on violation

The store is the journal's one writer-owner. Every append goes through its append primitive —
`ingest` hands it the reconciliation and refresh batches at baselining, the proposal module's
Registrar hands it the `registered` event that opens a proposal's lifecycle — and the primitive
lands each batch with the same atomic write-and-rename the snapshot uses, validating every line
against the journal-line schema before the write commits. Nothing writes around it; the one-shot
backfill that seeded the journal from the retired bead-map's git history ran once and is history
itself. Append-only describes the format's semantics, not an I/O contract.

The schema check has two audiences with different stakes. The map query surface fails loudly: a
malformed line is an error naming the file and line, and the query never runs. Gating callers — the
diff removal sweep — treat the same condition as journal-absent and degrade detection instead of
failing, because the journal is never load-bearing for the pipeline: it can weaken a sweep to
`unverifiable` notes, it cannot block a gate or fake a pass.

## Event Format

One JSON object per line. Change events record what a baselining absorbed:

```json
{"event": "removed", "eid": "cafe1234:op-7", "node": "a1b2c3d4e5f6", "name": "ActionClassifier",
 "node_type": "component", "module": "impact", "before": "e3b0c44298fc", "after": null,
 "path": "impact/arch_action_classifier.md", "git_head": "cafe1234", "proposal": "2026-08-13-plan-module"}
```

The registered event records the opening of a proposal's lifecycle — appended at registration,
before any spec change exists, so the epic's receipt has an event to reference:

```json
{"event": "registered", "eid": "cafe1234:2026-08-11-event-keyed-linkage",
 "proposal": "2026-08-11-event-keyed-linkage", "git_head": "cafe1234"}
```

Receipt events record what the tracker did:

```json
{"event": "task_created", "for": "cafe1234:op-7", "task_id": "spexmachina-abc"}
{"event": "task_retargeted", "for": "cafe1234:op-9", "task_id": "spexmachina-abc"}
{"event": "refresh", "git_head": "cafe1234", "absorbed": ["cafe1234:op-7"]}
```

| Field | On | Description |
|-------|----|-------------|
| `event` | all | `added`, `removed`, `modified`, `registered`, `task_created`, `task_closed`, `task_retargeted`, `refresh` |
| `eid` | change, registered | Event id — deterministic so ingest re-runs append nothing new and plan can derive it at changeset-build time. Op-born events derive it from `(git_head, op_id)`; refresh-born and absorb-born events, which have no create op, derive it from `(node, before, after)`; the registered event's is `<git_head>:<slug>` |
| `node` | change | Identity hash of the spec node — byte-identical to the merkle tree key, so every pipeline stage joins on it with no translation |
| `name`, `node_type`, `module` | change | The node's declared identity at event time — the journal is the only record of these once the node is removed |
| `before`, `after` | change | Leaf hashes bracketing the change; `null` marks absence (an add has no before, a removal no after) |
| `git_head` | change, registered, refresh | The commit the changeset carried; registration records the head at register time; refresh records its own when given `--git-head` and its absence otherwise |
| `path` | change, optional | The node's content-leaf path relative to the spec directory, present when the node has one — what makes `git show <head>:<path>` runnable for a removed node. Requirement and api events carry none |
| `proposal` | change, registered | The proposal that drove the change, or — on the registered event — the slug whose lifecycle opened. A `task_created` carrying a slug instead of `for` is a legacy pre-migration shape, read but never appended anew |
| `for` | receipts | The `eid` of the event this receipt answers. A modify pair's `task_closed` and `task_created` both reference the pair's `modified` event; a removal's `task_closed` references the `removed` event; an epic's `task_created` references the `registered` event; a `task_retargeted` references the retarget's own `modified` event |
| `task_id` | receipts | The tracker's id, applied by the adapter, reported in receipts |

A receipt pairs with the event its `for` names by eid alone, never by file position. Ingest lands
a batch whole and is free to write a receipt on either side of the event it references: the
journal really does carry a cleanup's `task_created` ahead of the `removed` event that receipt
answers. Folding therefore indexes every event by eid before it walks them — an index built
during the walk would meet that receipt early, find nothing, and report a pairing that exists as
dangling. Dangling means the journal carries no such event anywhere in the file, which is a fact
about the file and not about the order two lines happen to sit in.

A pointer discipline holds throughout: the journal stores identity, names, hashes and commit refs —
never leaf content. Showing a change is `git diff <before-head> <after-head> -- <leaf>`, derived
from the refs, so the journal cannot drift from the bytes git already owns.

## Interface

Every request the store answers is a request about events and folds — never about the tracker,
which it does not contact. A caller can

- parse the journal and receive every event in file order;
- ask for the fold and receive one entry per task-bearing node — hash, current task id, and the
  sourcing event;
- resolve one key, hash or task id, to its fold entry — including removed nodes, which answer to
  both keys alike: the hash reaches the biography, and so does the cleanup task id, since that
  task is what the entry now carries in place of a live one;
- ask for a node's full event history, oldest first, which is how lineage questions ("which tasks
  has this node had?") are answered without any stored back-pointers;
- append events — the store validates each line against the journal-line schema and lands the
  batch with one atomic write-and-rename. Its two callers are `ingest`, appending reconciliation
  and refresh batches at baselining, and the Registrar, appending the `registered` event.

Two of those reads are exposed on the command line unchanged in name: [[38ddf587012f|`spex map get`]]
hands back one fold entry and [[394ec2c8d669|`spex map list`]] hands back the whole fold, each as
JSON. Nothing writes through that surface.

## File Location

The journal lives beside the snapshot at the location the lifecycle pre-flight
([[a9aa93774cc2|ProjectResolver]]) answers: inside the `.spex/` state directory — the tool's half
of the tool-writes/person-writes split. The store takes the resolved path as input and computes
no location of its own. The journal is committed to git and is NOT hashed into the merkle tree —
the tree hashes only the content files module.json declares — so appends move no hash and trigger
no diff. Sitting under `.spex/`, outside the spec tree, the journal is also invisible to the
removal sweep's corpus scan, so its own `removed` events — which carry exactly the names the
sweep hunts — can never count as survivors. The retired `--map`/`--map-file`
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

The label is `spex:<eid>` — event ids are deterministic, derived from the op (`git_head`,
`op_id`), from the drift itself (`node`, `before`, `after`) for refresh-born events, or as
`<git_head>:<slug>` for the registered event. Determinism serves two consumers at once: ingest
re-runs derive the same eids and append nothing, and plan derives every eid at changeset-build
time to mint the op's label — the eid's optional tracker-side carrier — so idempotency needs no
counter, no reservation read, and no coordination. Legacy `spex:<int>`, `spex:<spec_node_id>` and `spex:cleanup-<hash>` labels on
existing tasks are inert history: the backfill seeded the journal with every pairing that ever
existed, so nothing old became unresolvable.

### Why a removed entry carries the cleanup task, not the one it had alive

Both readings of "the entry carries a biography instead of a live task" are defensible on their
own — the journal holds the pre-removal pairing either way, as lineage — so the choice is settled
here rather than left to whoever implements next. The narrow reading wins because ingest depends
on it: the reconciler reads a fold entry that is `removed` and already carries a task id as
"this removal's cleanup has landed", and mints nothing further. Under the broad reading every
removed node that ever had a task would present that signal, and no genuine cleanup receipt would
ever be recognised as new. Cleanup idempotency therefore rests on this sentence, not on a
convention, and a future implementer who broadens it breaks that silently.

### Why one writer-owner?

One writer means append atomicity and schema validation are solved exactly once, and the module
that owns the file is the right place to solve them. Two lifecycles need to append — baselining
(ingest's reconciliation and refresh batches) and registration (the Registrar's `registered`
event) — and both go through the store's single primitive, so every other consumer reads a file
that is either the pre-append or the post-append state, never a torn intermediate, whichever
caller wrote last.
