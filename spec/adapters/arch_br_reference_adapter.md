# BrReferenceAdapter

Bash + jq scripts targeting the `br` CLI, in two halves. The export half, `scripts/export-br.sh`, derives the task-state artifact `spex plan` reads from the tracker. The apply half, `scripts/apply-br.sh`, reads `changeset.json` on stdin (or `$1`), executes operations against `br`, and writes `receipts.json` to stdout (or `$2`). Together they are the one place the tracker's own format is read or written: everything on the spex side of them is a document spex declares.

The tracker's own name for what it manages never enters this contract. The scripts speak `br`'s surface through its command strings — `br create`, `br list`, `br close` and the rest — and call what those commands manage tasks.

## Responsibilities

### Export half

- Read the tracker's listing — `br list --json --all --limit 0`, status-unfiltered and unbounded because `br list`'s defaults hide finished tasks and cap the row count — and project it onto the version-1 task-state document: keep only tasks whose status is `open` or `in_progress`, carry each one's id as `task_id` and its status verbatim, drop every other field.
- Write `{"version": 1, "tasks": [...]}` to stdout or `$1`. A tracker with nothing in flight yields an empty `tasks` array, not an absent file and not an error. A failure of the listing itself exits 1 and writes no document — not an empty one — so that `spex plan` refuses for want of its required input rather than reading every task as finished.
- Own the tracker-format boundary in the inbound direction: a change to `br list`'s output shape is a change to this script and never to the binary, because the document it writes is validated by plan against `schema/task-state.schema.json`, and a field that leaked through would fail there.

### Apply half

- Parse changeset v4. The top-level `absorbed` array is ignored entirely — it is ingest's input, not the adapter's, and no tracker call ever comes of it.
- For each op in order:
  - Resolve every ref field (`parent`, `deps`, `target`) to a concrete task id, per [[a2645b77b8bc|the ref shapes]]: a `task` ref passes through literally and an `op` ref is looked up in the substitution table. The changeset carries no third shape — plan resolves node references before the adapter ever runs, so the adapter reads no spex-owned file — and no ref carries an edge-type field.
  - Check idempotency before create/close — for a create that check runs before any of the op's refs are resolved, for a close after the target ref is resolved. A retarget op needs no check at all: every call it makes — the update and each dep add — is naturally idempotent.
  - Invoke the appropriate `br` subcommand with correct flags.
  - Write a receipt entry.
- Emit a top-level status field: `complete` when every op ended `ok` or was intentionally skipped, `partial` as soon as one op ends `error`. A skip is not a failure — it means the adapter deliberately did nothing. If the adapter dies before the file is written at all, no `receipts.json` exists: `spex ingest` fails with `ingest: read receipts: …`, exits 1, and the run must be re-run rather than ingested — it is never read as `partial`.

## Interface

```
scripts/export-br.sh [<tasks.json>]

# Default: the task-state document on stdout.

scripts/apply-br.sh [<changeset.json>] [<receipts.json>]

# Defaults: changeset on stdin, receipts on stdout.
```

One cycle runs `export-br.sh`, then `spex plan --tasks`, then `apply-br.sh`, then `spex ingest`; the export must run before every plan, because the artifact is required and describes the tracker at the moment it was written.

## Dependencies

- `br` — any version that supports `create`, `list --json`, `show <id> --format json`, `update`, `dep add`, `close`. Dependency mutation is a subcommand of its own: `update` carries label and parent flags but nothing that touches deps, so a retarget's dep half cannot ride on the same call as its label half. No minimum version is enforced: the pre-flight only checks that `$BR_BIN --version` exits 0, discarding its output.
- `jq` — 1.6+.
- Bash 4.0+ (for associative arrays used by the substitution table).

## Changeset Preconditions

[[4277dbd90063|Reading a changeset v4]] begins with the envelope, and only the envelope: the document must parse, `version` must be exactly `4`, `git_head` and `proposal` must both be present, and `ops` must be an array. Refusing every other version is the field's whole point — a v2-era consumer meeting a `retarget` would silently drop the op, a v3-era consumer meeting a `ref:task` would resolve nothing, and either would report a complete run over an incomplete one; the version check is what makes that impossible. Each condition below exits 1 with its reason on stderr and writes **no** `receipts.json` at all: the failure is at the changeset level, not at an op level, and receipts exist only for ops that were attempted. Nothing inside an op is inspected at this point — a missing `op_id`, an unknown `type` or an unresolvable ref surfaces only when the loop reaches that op, and becomes that op's `error` receipt rather than a refusal of the document. An op of the wrong *shape* is the exception, and it fails later and harder: see "The shape hole" below.

| Condition | Reason on stderr |
|---|---|
| the input is not valid JSON | `changeset is not valid JSON` |
| `version` absent | `changeset missing required field: version` |
| `version` is anything but `4` | `unsupported changeset version: <v> (expected 4)` |
| `git_head` absent | `changeset missing required field: git_head` |
| `proposal` absent | `changeset missing required field: proposal` |
| `ops` absent, or present and not an array | `changeset missing or malformed required field: ops` |

`git_head` and `proposal` are required even though op processing never reads either one. The adapter checks that both are present and non-empty and then makes no further use of them: no label is built from `git_head` here — the close markers of the label era are gone, and the eids inside op labels reach the adapter already assembled. Requiring the two fields is a check on the document's provenance, not an input to any `br` invocation — a changeset that omits either is refused before the first op runs, so no tracker state is ever produced by a run whose commit and proposal are unrecorded.

## Op Processing Loop

Ops are dispatched on their `type` field, one at a time, in the order the changeset lists them. Order is contract, not convenience — a later op may reference the task an earlier op produced.

| `type` | Effect |
|---|---|
| `create` | Create a task, subject to the create-idempotency check below. |
| `close` | Close a task, subject to the close-idempotency check below. |
| `retarget` | Update the target task: add the op's event label, add its missing deps — the label through `br update`, each dep through `br dep add`. See "retarget op" below. |
| anything else | No tracker call at all. The op gets an `error` receipt reading `unknown op type: <type>`. The v3 `label` and `tag` kinds land here: nothing ever emitted them, and v4 retired them from the vocabulary. |

**A bad op is recorded, not raised — for everything the adapter checks for.** In each case below it finishes that op with an `error` receipt and moves to the next, so `receipts.json` accounts for every op the changeset listed and the caller can see exactly how far the run got. The reason recorded is what went wrong:

| What went wrong | Reason recorded |
|---|---|
| the op carries no `op_id` or no `type` | `malformed op: missing op_id or type` |
| an `op` ref names an op with no substitution-table entry and no error on record | `dependency <op_id> not yet resolved` — never reachable from a well-formed changeset, since plan orders ops so that every referenced op precedes its referent |
| an `op` ref names an op that errored before reaching a task | `dependency <op_id> errored; cannot resolve op ref` |
| a ref carries an unrecognised discriminator — the v3 task-ref spelling included | `unknown ref kind: <kind>` |
| a `br create`, `br update`, `br show` or `br close` invocation exits non-zero | the command that failed, followed by everything that invocation wrote on stdout and stderr |
| the idempotency query before a create fails | the fixed string `br list failed during idempotency check`; that query's stderr is discarded rather than recorded |

A ref that fails to resolve stops its op **before** the `br` call that would change tracker state, so a failed ref never leaves a half-built task behind.

### The shape hole

Some ops whose JSON is the wrong *shape* rather than merely wrong escape all of that. The script runs under `set -euo pipefail`, and two reads reach an op with bare `jq` expressions before any of the checks above apply: the main loop's own `.ops[i]`, `.op_id` and `.type` reads that open every iteration, and the create path's `idempotency.label` read. Hand either a scalar where an object belongs and `jq` fails, `set -e` takes the script down, and the run ends with jq's exit code and **no `receipts.json` at all** — no entry for the offending op and none for the ops after it. Reproduced: a changeset whose first op carries `"idempotency": 5` exits 5 with `Cannot index number with string "label"` on stderr and writes nothing, while the same changeset with a well-formed `idempotency` object completes.

That failure reaches the caller as the same signal a pre-flight refusal does — an absent receipts file — and is read the same way: `spex ingest` fails with `ingest: read receipts: …`, exits 1, and the run must be re-run rather than ingested. It is a gap in this reference implementation rather than in the contract; a production adapter guards those reads and turns each into an `error` receipt.

Two nearby reads look like the same hole and are not. The create path's later `title`, `body`, `spec_node_kind` and `priority` reads run on an op the loop has already read `.op_id` and `.type` off, so any shape that would break them has already taken the script down an iteration's-worth of reads earlier. And ref resolution's `.ref` read, though equally unguarded, is made inside the resolver the call sites invoke, which runs on past the failed read to write its unknown-kind sentinel — and that write, not the failed read, is what the call site sees: the op ends with an `error` receipt like any other unresolvable ref, and the loop carries on. Reproduced: a changeset whose first op carries `"parent": 5` records `parent ref: unknown ref kind: missing` against that op, runs the next op, reaches phase 3, and exits 0 with a full `receipts.json`.

## Substitution Table

The [[2f0a1f1152a0|substitution table]] maps a create op's `op_id` to the task id that op reached. The entry is written the moment a create op reaches a task — a fresh create, as soon as `br create` returns an id, or an idempotent re-match, which records the pre-existing task id with `was_existing=true` — and nothing withdraws it afterwards. A create that reached its task and then failed while applying the rest of its labels therefore ends with an `error` receipt *and* a table entry, and a later op whose `parent` or `deps` reference it resolves to that task rather than failing. A create op that never reached a task is absent from the table, and a reference to one of those fails by name at resolve time rather than pointing at nothing.

The table is process-local and lives only for the length of the run; a crash loses it and nothing reloads it from disk. Nothing needs to: a re-run replays the same create ops, the idempotent re-matches repopulate the same op ids with the same task ids, and same-run forward refs resolve exactly as they did the first time. Every same-run forward ref therefore either resolves or fails by name — there is no third outcome.

## The only tracker caller

The adapter is the sole participant in the pipeline that executes a tracker CLI. `spex diff`, `plan` and `ingest` read and write files only; every `br` invocation in the whole flow happens inside these two scripts — the export's one listing, and the apply loop's mutations. That is what the boundary buys: the changeset names spec nodes, ops and refs, the task-state artifact names task ids and two statuses, and the adapter is where those become `br list`, `br create`, `br update`, `br close` and their flags.

Two consequences follow, and both are contract rather than convention:

- **Every tracker-specific decision belongs here.** Tracker type vocabulary, flag spelling, dep-edge syntax, the `--force` on close, the exit-code quirks, the listing's own field names and status values — none of it may leak upward into plan. An adapter for a different tracker replaces these two files and nothing else.
- **Every non-tracker decision belongs upstream.** The adapter does not reorder ops, choose priorities, allocate record ids, decide what work exists, or decide what a task's absence from the artifact means. It receives those already settled and executes them in file order. An adapter that started making structural decisions would make the run non-deterministic, since nothing upstream could reproduce it.

## Op Translation

The adapter maps changeset op fields to `br` subcommand flags. Four of those mappings are contract rather than incidental detail, because the obvious reading of the changeset gets each of them wrong:

### `spec_node_kind` → `br --type`

| `spec_node_kind` | `br --type` |
|------------------|-------------|
| `proposal_epic`  | `epic`      |
| `component`      | `feature`   |
| `data_flow`      | `task`      |
| `test_section`   | `task`      |
| `cleanup`        | `task`      |
| (other / unset)  | `feature` (default) |

The `cleanup → task` mapping carries forward the pre-decouple cleanup creator's explicit choice of the `task` type; cleanup tasks are bookkeeping/maintenance work, not features.

### `op.Labels` → `br update --add-label`

Each entry of `op.Labels` must reach the target task, in the order the changeset lists them. `br create` has no `--add-label` flag — its `--labels` carries the idempotency label alone — so on a create the adapter would attach them immediately after the create returns, one `br update --add-label <label>` per entry, and an update that exits non-zero ends the op with an `error` receipt naming the label it was applying. In current plan output only retarget ops populate the field — creates and closes carry no `labels` at all, the retired `spex:cleanup` discriminator and the close markers of the label era with them — but the application path stays generic.

The `idempotency.label` is applied separately and earlier: the adapter queries for a task carrying it (any status), and on no match hands it to `br create` as `--labels <label>`, so it is on the task from the moment the task exists. This is independent of `op.Labels`, and it is this adapter's implementation of an *optional* capability: the label surface is insurance the contract offers, not a bar it sets — an adapter for a tracker without label support skips the probe and every label application and stays conformant, accepting that a run crashed between a create and its receipt can duplicate that create on a blind re-run.

### `op.deps` → `br create --deps`

Each entry of an op's `deps` becomes its own `--deps blocked-by:<task-id>` flag, in the order the changeset lists them — never one flag carrying a joined list. The edge is always `blocked-by`, since the reading of a dep is that this task is blocked by that one, and v4 refs carry no edge-type field that could say otherwise: the lineage edge was the only typed dep, and it is gone. `parent` is a separate `--parent <task-id>` flag and is never expressed as a dep edge.

### retarget op → `br update` + `br dep add`

[[fcb32354630e|A retarget op is applied through the tracker's mutation surface]] and touches nothing else: the target ref resolves like any other (a `task` ref literally, an `op` ref through the substitution table), then one `br update --add-label <label>` per entry of `op.Labels` — the run's `spex:<eid>` event label — followed by one `br dep add <task-id> <dep-task-id> --type blocks` per resolved dep the task does not already carry, read off a single `br show <task_id> --format json` of its current deps. The two halves cannot share a call: `br update` carries no dep flag of any name, so the dep half is the one part of a retarget that leaves the update surface. `blocks` is the tracker's own dep-type vocabulary rather than the changeset's spelling — the same edge the create path names `blocked-by`, because `br create --deps` takes that alias and records `blocks` while `br dep add --type` rejects it outright; both paths therefore land the identical edge under different names. Deps are add-only by contract: nothing is removed, because a stale dep is closed by its own lifecycle and a removal here would make re-runs diverge. No probe precedes any of it — an update applied twice converges on the same state, and re-adding an edge the task already carries is itself a no-op, so re-running a retarget adds nothing and errors nothing. The receipt records `op_id`, `status` and the target `task_id`; `was_existing` does not apply. A `br show`, `br update` or `br dep add` that exits non-zero ends the op with an `error` receipt like any other failed invocation.

## Idempotency

Before every `br create`, [[b8d894dff9b5|the create-idempotency check]] asks the tracker whether the op's task already exists, by querying `br list --json --all --limit 0 --label <op.idempotency.label>` — status-unfiltered and unbounded, because `br list`'s defaults hide finished tasks and cap the row count, and either default would silently reintroduce the retired open-only semantics. **A match is a task carrying the exact label, in any status.** On a match, skip create, record `was_existing=true` with the existing task_id.

The retired open-only filter existed solely to dodge node-key collisions: under `spex:<spec_node_id>` labels, a successor create carried the same label as the finished task it followed, so the adapter had to filter finished tasks out or it would skip the create. Labels are now `spex:<eid>` — unique per change (see `spec/plan/arch_idempotency_labeler.md`) — so a finished predecessor carries a *different* label and no filtering is needed: any task carrying this op's exact label, whatever its status, can only be the product of this same op's earlier run, and exact match is stricter than filtered match ever was.

Plan produces one label shape for every action class; the adapter just looks up whatever label the op carries, treating it as opaque. Tasks wearing legacy shapes (`spex:<spec_node_id>`, `spex:cleanup-<hash>`, `spex:<proposal-slug>`, `spex:<int>`) are inert to this check, because plan never mints those shapes again and exact matching means they can never capture a new create.

Before every `br close`, [[7bad082a34b6|the close-idempotency check]] reads the target's current `status` with a single `br show <task_id> --format json`. The two branches below are decided from that one response, and nothing is re-queried between the decision and the action — no label is read or written anywhere on this path, because close idempotency keys on the tracker's own status and close ops carry no labels:

| Pre-state             | Action                                                                          | Receipt |
|-----------------------|---------------------------------------------------------------------------------|---------|
| `status == "closed"`  | Skip — do NOT call `br close` (it exits 3 on already-closed targets).            | `status=ok` |
| `status == "open"`    | `br close --force --reason …`, the reason present when the op carries one.       | `status=ok` |

Both branches converge on `status=ok` deliberately: whichever run closed the target first, a close op against a closed task is complete, and the journal's eid-deduped receipts absorb a re-run without a second `task_closed` line. The skip branch matters because `br close` exits 3 on already-closed targets; treating that exit as `status=error` would push top-level receipts to `partial` and block ingest from saving the snapshot. It is now the re-run branch alone: plan issues a close only for a task the task-state artifact listed as live, so on a first run the target is open by construction, and the routine no-op closes the retired modify pair used to issue against finished tasks no longer exist to converge.

## Receipt Atomicity

Each op's receipt is appended to an in-memory array. After the last op, the full receipts.json is written atomically via temp file + mv (analogous to plan's --out). A reader therefore sees either the complete v2 document or the pre-run content of that path — never a half-written file.

If the adapter crashes mid-run, partial receipts are lost — the reference adapter implements no checkpointing; it's a reference, not a crash-safe production adapter. Users building production adapters add checkpointing as an enhancement.

## Receipt Shape

[[3486b44f4f64|Receipts v2]] is the apply half's only output artifact, and ingest's only input from it:

```json
{
  "version": 2,
  "status": "complete | partial",
  "ops": [
    {
      "op_id": "op-component-a1b2c3d4e5f6",
      "status": "ok | skipped | error",
      "task_id": "<the task reached, or empty if none was>",
      "was_existing": true,
      "reason": "<present on skipped>",
      "error": "<present on error>"
    }
  ]
}
```

Version 2 renamed the per-op tracker id field to `task_id`; ingest refuses a receipts file of any other version rather than reading the retired key. In a file that gets written, every op the changeset listed has exactly one entry, and the entries are in changeset order. A run that dies mid-loop writes no file at all rather than a short one — see "The shape hole" above. `reason` appears only on a `skipped` entry and `error` only on an `error` entry; an `ok` entry carries neither. An op too malformed to have an `op_id` still gets an entry, filed under a synthetic id derived from its position, so the two lists never drift out of alignment.

## Export Shape

The export half's only output is the version-1 task-state document:

```json
{
  "version": 1,
  "tasks": [
    {"task_id": "spexmachina-abc", "status": "open"},
    {"task_id": "spexmachina-def", "status": "in_progress"}
  ]
}
```

The projection is exact: the two statuses are carried as `br` spells them, and every other status the listing reports — `closed` above all — drops the task from the document rather than mapping to a value, because the document has no value for it. Entries keep the listing's order. The script writes nothing else — no header, no count — so the file is the document.

## Reference-Adapter Limitations

[[7c2fea6b1963|The reference implementation's scope]] is deliberately bounded: this module's functional requirements are the contract any adapter must satisfy, and these scripts are one implementation of them rather than the definition. Anyone needing production hardening forks them. Explicitly documented:

- Single-threaded sequential processing. Concurrent invocations will trample receipts.
- No retry on transient `br` failures — user's responsibility to fix and re-run.
- No dry-run mode.
- No pre-flight check that the tracker state matches what plan assumed — that the task-state artifact plan read is still the tracker's state at apply time. Production adapters should add this.

## Header Notice

Each script begins with:

```
#!/usr/bin/env bash
#
# apply-br.sh — Reference adapter consuming spex changeset.json v4 and invoking br.
#
# REFERENCE IMPLEMENTATION. Vet before production use. See spec/adapters/ for the
# adapter contract that any implementation (this one or your own) must satisfy.
#
# Usage: apply-br.sh [<changeset.json>] [<receipts.json>]
#
set -euo pipefail
```

with the export script carrying the same notice for its own usage line, `export-br.sh [<tasks.json>]`.

## Test Hooks

- `BR_BIN` env var — overrides `br` binary path (for tests with a mock), honoured by both halves.
- `SPEX_ADAPTER_DEBUG=1` — after each op, dump the substitution table to stderr, one `op_id → task_id` line per entry. Stdout stays clean, so a debug run still pipes its receipts.

Each defaults to the real-world value: the `br` on PATH and no debug output. The retired `SPEX_MAPPING_FILE` override is gone with the mapping lookup it served — the adapter reads no spex-owned file. The fixture runs against a stand-in `br` via `BR_BIN` at a mock binary. [[970260050e3e|The integration test against a real `br`]] sets nothing: gated on `br` being on PATH, it runs both scripts from a throwaway sandbox where the default already resolves to the real `br`, and its fixtures reach every op type this leaf describes — create, close and retarget — plus the export half. Both paths exercise the shipped scripts unmodified rather than copies that have diverged.
