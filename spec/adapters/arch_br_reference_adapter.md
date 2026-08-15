# BrReferenceAdapter

Bash + jq script at `scripts/apply-br.sh`. Reads `changeset.json` on stdin (or `$1`), executes operations against the `br` CLI, writes `receipts.json` to stdout (or `$2`).

## Responsibilities

- Parse changeset v3. The top-level `absorbed` array is ignored entirely — it is ingest's input, not the adapter's, and no tracker call ever comes of it.
- For each op in order:
  - Resolve every ref field (`parent`, `deps`, `target`) to a concrete bead id, per [[a2645b77b8bc|the ref shapes]]: a `bead` ref passes through literally and an `op` ref is looked up in the substitution table. The changeset carries no third shape — plan resolves node references before the adapter ever runs, so the adapter reads no spex-owned file.
  - Check idempotency before create/close — for a create that check runs before any of the op's refs are resolved, for a close after the target ref is resolved. A retarget op needs no check at all: every call it makes — the update and each dep add — is naturally idempotent.
  - Invoke the appropriate `br` subcommand with correct flags.
  - Write a receipt entry.
- Emit a top-level status field: `complete` when every op ended `ok` or was intentionally skipped, `partial` as soon as one op ends `error`. A skip is not a failure — it means the adapter deliberately did nothing. If the adapter dies before the file is written at all, no `receipts.json` exists: `spex ingest` fails with `ingest: read receipts: …`, exits 1, and the run must be re-run rather than ingested — it is never read as `partial`.

## Interface

```
scripts/apply-br.sh [<changeset.json>] [<receipts.json>]

# Defaults: changeset on stdin, receipts on stdout.
```

## Dependencies

- `br` — any version that supports `create`, `list --json`, `show <id> --format json`, `update`, `dep add`, `close`. Dependency mutation is a subcommand of its own: `update` carries label and parent flags but nothing that touches deps, so a retarget's dep half cannot ride on the same call as its label half. No minimum version is enforced: the pre-flight only checks that `$BR_BIN --version` exits 0, discarding its output.
- `jq` — 1.6+.
- Bash 4.0+ (for associative arrays used by the substitution table).

## Changeset Preconditions

[[4277dbd90063|Reading a changeset v3]] begins with the envelope, and only the envelope: the document must parse, `version` must be exactly `3`, `git_head` and `proposal` must both be present, and `ops` must be an array. Refusing every other version is the field's whole point — a v2-era consumer meeting a `retarget` would silently drop the op and report a complete run over an incomplete one, and the version check is what makes that impossible. Each condition below exits 1 with its reason on stderr and writes **no** `receipts.json` at all: the failure is at the changeset level, not at an op level, and receipts exist only for ops that were attempted. Nothing inside an op is inspected at this point — a missing `op_id`, an unknown `type` or an unresolvable ref surfaces only when the loop reaches that op, and becomes that op's `error` receipt rather than a refusal of the document. An op of the wrong *shape* is the exception, and it fails later and harder: see "The shape hole" below.

| Condition | Reason on stderr |
|---|---|
| the input is not valid JSON | `changeset is not valid JSON` |
| `version` absent | `changeset missing required field: version` |
| `version` is anything but `3` | `unsupported changeset version: <v> (expected 3)` |
| `git_head` absent | `changeset missing required field: git_head` |
| `proposal` absent | `changeset missing required field: proposal` |
| `ops` absent, or present and not an array | `changeset missing or malformed required field: ops` |

`git_head` and `proposal` are required even though op processing never reads either one. The adapter checks that both are present and non-empty and then makes no further use of them: the `commit:<HEAD>` label on a close op reaches the adapter already assembled inside that op's own `labels` array, so nothing here builds a label out of `git_head`. Requiring the two fields is a check on the document's provenance, not an input to any `br` invocation — a changeset that omits either is refused before the first op runs, so no tracker state is ever produced by a run whose commit and proposal are unrecorded.

## Op Processing Loop

Ops are dispatched on their `type` field, one at a time, in the order the changeset lists them. Order is contract, not convenience — a later op may reference the bead an earlier op produced.

| `type` | Effect |
|---|---|
| `create` | Create a bead, subject to the create-idempotency check below. |
| `close` | Close a bead, subject to the close-idempotency check below. |
| `retarget` | Update the target bead: add the op's event label, add its missing deps — the label through `br update`, each dep through `br dep add`. See "retarget op" below. |
| `label` | Add the op's labels to the target bead. |
| `tag` | Add the op's labels to the target bead; `br` draws no distinction between the two. |
| anything else | No tracker call at all. The op gets an `error` receipt reading `unknown op type: <type>`. |

**A bad op is recorded, not raised — for everything the adapter checks for.** In each case below it finishes that op with an `error` receipt and moves to the next, so `receipts.json` accounts for every op the changeset listed and the caller can see exactly how far the run got. The reason recorded is what went wrong:

| What went wrong | Reason recorded |
|---|---|
| the op carries no `op_id` or no `type` | `malformed op: missing op_id or type` |
| an `op` ref names an op with no substitution-table entry and no error on record | `dependency <op_id> not yet resolved` — never reachable from a well-formed changeset, since plan orders ops so that every referenced op precedes its referent |
| an `op` ref names an op that errored before reaching a bead | `dependency <op_id> errored; cannot resolve op ref` |
| a ref carries an unrecognised discriminator | `unknown ref kind: <kind>` |
| a `br create`, `br update`, `br show` or `br close` invocation exits non-zero | the command that failed, followed by everything that invocation wrote on stdout and stderr |
| the idempotency query before a create fails | the fixed string `br list failed during idempotency check`; that query's stderr is discarded rather than recorded |

A ref that fails to resolve stops its op **before** the `br` call that would change tracker state, so a failed ref never leaves a half-built bead behind.

### The shape hole

Some ops whose JSON is the wrong *shape* rather than merely wrong escape all of that. The script runs under `set -euo pipefail`, and two reads reach an op with bare `jq` expressions before any of the checks above apply: the main loop's own `.ops[i]`, `.op_id` and `.type` reads that open every iteration, and the create path's `idempotency.label` read. Hand either a scalar where an object belongs and `jq` fails, `set -e` takes the script down, and the run ends with jq's exit code and **no `receipts.json` at all** — no entry for the offending op and none for the ops after it. Reproduced: a changeset whose first op carries `"idempotency": 5` exits 5 with `Cannot index number with string "label"` on stderr and writes nothing, while the same changeset with a well-formed `idempotency` object completes.

That failure reaches the caller as the same signal a pre-flight refusal does — an absent receipts file — and is read the same way: `spex ingest` fails with `ingest: read receipts: …`, exits 1, and the run must be re-run rather than ingested. It is a gap in this reference implementation rather than in the contract; a production adapter guards those reads and turns each into an `error` receipt.

Two nearby reads look like the same hole and are not. The create path's later `title`, `body`, `spec_node_kind` and `priority` reads run on an op the loop has already read `.op_id` and `.type` off, so any shape that would break them has already taken the script down an iteration's-worth of reads earlier. And ref resolution's `.ref` read, though equally unguarded, is made inside the resolver the call sites invoke, which runs on past the failed read to write its unknown-kind sentinel — and that write, not the failed read, is what the call site sees: the op ends with an `error` receipt like any other unresolvable ref, and the loop carries on. Reproduced: a changeset whose first op carries `"parent": 5` records `parent ref: unknown ref kind: missing` against that op, runs the next op, reaches phase 3, and exits 0 with a full `receipts.json`.

## Substitution Table

The [[2f0a1f1152a0|substitution table]] maps a create op's `op_id` to the bead id that op reached. The entry is written the moment a create op reaches a bead — a fresh create, as soon as `br create` returns an id, or an idempotent re-match, which records the pre-existing bead id with `was_existing=true` — and nothing withdraws it afterwards. A create that reached its bead and then failed while applying the rest of its labels therefore ends with an `error` receipt *and* a table entry, and a later op whose `parent` or `deps` reference it resolves to that bead rather than failing. A create op that never reached a bead is absent from the table, and a reference to one of those fails by name at resolve time rather than pointing at nothing.

The table is process-local and lives only for the length of the run; a crash loses it and nothing reloads it from disk. Nothing needs to: a re-run replays the same create ops, the idempotent re-matches repopulate the same op ids with the same bead ids, and same-run forward refs resolve exactly as they did the first time. Every same-run forward ref therefore either resolves or fails by name — there is no third outcome.

## The only tracker caller

The adapter is the sole participant in the pipeline that executes a bead CLI. `spex diff`, `plan` and `ingest` read and write files only; every `br` invocation in the whole flow happens inside this script. That is what the boundary buys: the changeset names spec nodes, ops and refs, and the adapter is where those become `br create`, `br update`, `br close` and their flags.

Two consequences follow, and both are contract rather than convention:

- **Every tracker-specific decision belongs here.** Bead-type vocabulary, flag spelling, dep-edge syntax, the `--force` on close, the exit-code quirks — none of it may leak upward into plan. An adapter for a different tracker replaces this file and nothing else.
- **Every non-tracker decision belongs upstream.** The adapter does not reorder ops, choose priorities, allocate record ids, or decide what work exists. It receives those already settled and executes them in file order. An adapter that started making structural decisions would make the run non-deterministic, since nothing upstream could reproduce it.

## Op Translation

The adapter maps changeset op fields to `br` subcommand flags. Three of those mappings are contract rather than incidental detail, because the obvious reading of the changeset gets each of them wrong:

### `spec_node_kind` → `br --type`

| `spec_node_kind` | `br --type` |
|------------------|-------------|
| `proposal_epic`  | `epic`      |
| `component`      | `feature`   |
| `data_flow`      | `task`      |
| `test_section`   | `task`      |
| `cleanup`        | `task`      |
| (other / unset)  | `feature` (default) |

The `cleanup → task` mapping carries forward pre-decouple `apply/bead_creator.go::createCleanupBead`'s explicit `Type: "task"` choice; cleanup beads are bookkeeping/maintenance work, not features.

### `op.Labels` → `br update --add-label`

Each entry of `op.Labels` must reach the bead a create op produces, in the order the changeset lists them. `br create` has no `--add-label` flag — its `--labels` carries the idempotency label alone — so the adapter attaches them immediately after the create returns, one `br update --add-label <label>` per entry, and an update that exits non-zero ends the op with an `error` receipt naming the label it was applying. Cleanup creates are the first create-op consumer of this pre-existing field: without it the discriminator label `spex:cleanup` never reaches the tracker, and cleanup beads become indistinguishable from ordinary work in any tracker-side query. Close ops and `label`/`tag` ops apply their labels through the same `br update` call.

The `idempotency.label` is applied separately and earlier: the adapter queries for a bead carrying it (any status), and on no match hands it to `br create` as `--labels <label>`, so it is on the bead from the moment the bead exists. This is independent of `op.Labels`; cleanup creates carry both `idempotency.label = "spex:<eid>"` (the implied removal event's) AND `Labels = ["spex:cleanup"]` — the linkage key and the state discriminator — and both end up on the bead.

### `op.deps` → `br create --deps`

Each entry of an op's `deps` becomes its own `--deps <edge>:<bead-id>` flag, in the order the changeset lists them — never one flag carrying a joined list. `<edge>` is the dep ref's own `type` when it carries one and `blocked-by` otherwise, since the default reading of a dep is that this bead is blocked by that one. `parent` is a separate `--parent <bead-id>` flag and is never expressed as a dep edge.

### retarget op → `br update` + `br dep add`

[[fcb32354630e|A retarget op is applied through the tracker's mutation surface]] and touches nothing else: the target ref resolves like any other (a `bead` ref literally, an `op` ref through the substitution table), then one `br update --add-label <label>` per entry of `op.Labels` — the run's `spex:<eid>` event label — followed by one `br dep add <bead-id> <dep-bead-id> --type <edge>` per resolved dep the bead does not already carry, read off a single `br show <bead_id> --format json` of its current deps. The two halves cannot share a call: `br update` carries no dep flag of any name, so the dep half is the one part of a retarget that leaves the update surface. `<edge>` is the tracker's own dep-type vocabulary rather than the changeset's spelling — the default dep is `blocks` here, the same edge the create path names `blocked-by`, because `br create --deps` takes that alias and records `blocks` while `br dep add --type` rejects it outright; both paths therefore land the identical edge under different names, and an edge the tracker does not know exits non-zero. Deps are add-only by contract: nothing is removed, because a stale dep is closed by its own lifecycle and a removal here would make re-runs diverge. No probe precedes any of it — an update applied twice converges on the same state, and re-adding an edge the bead already carries is itself a no-op, so re-running a retarget adds nothing and errors nothing. The receipt records `op_id`, `status` and the target `bead_id`; `was_existing` does not apply. A `br show`, `br update` or `br dep add` that exits non-zero ends the op with an `error` receipt like any other failed invocation.

## Idempotency

Before every `br create`, [[b8d894dff9b5|the create-idempotency check]] asks the tracker whether the op's bead already exists, by querying `br list --json --all --limit 0 --label <op.idempotency.label>` — status-unfiltered and unbounded, because `br list`'s defaults hide closed beads and cap the row count, and either default would silently reintroduce the retired open-only semantics. **A match is a bead carrying the exact label, in any status.** On a match, skip create, record `was_existing=true` with the existing bead_id.

The retired open-only filter existed solely to dodge node-key collisions: under `spex:<spec_node_id>` labels, a modify-pair's new create carried the same label as the closed bead it replaced, so the adapter had to filter closed beads out or it would skip the create. Labels are now `spex:<eid>` — unique per change (see `spec/plan/arch_idempotency_labeler.md`) — so a closed predecessor carries a *different* label and no filtering is needed: any bead carrying this op's exact label, whatever its status, can only be the product of this same op's earlier run, and exact match is stricter than filtered match ever was.

Plan produces one label shape for every action class; the adapter just looks up whatever label the op carries, treating it as opaque. Beads wearing legacy shapes (`spex:<spec_node_id>`, `spex:cleanup-<hash>`, `spex:<proposal-slug>`, `spex:<int>`) are inert to this check, because plan never mints those shapes again and exact matching means they can never capture a new create.

Before every `br close`, [[7bad082a34b6|the close-idempotency check]] reads the target's current state with a single `br show <bead_id> --format json`, taking both `labels` and `status` from that one response. The three branches below are decided from that combined state, and nothing is re-queried between the decision and the action:

| Pre-state                                  | Action                                                       | Receipt |
|--------------------------------------------|--------------------------------------------------------------|---------|
| `labels` contain `spex:obsolete`           | Skip — already obsoleted in a prior run.                     | `status=skipped`, reason `"already obsoleted"` |
| `status == "closed"` (no `spex:obsolete`)  | Apply labels via `br update --add-label …` only; do NOT call `br close` (it exits 3 on already-closed targets). | `status=ok` |
| `status == "open"` (no `spex:obsolete`)    | Apply labels via `br update --add-label …`, then `br close --force --reason …`. | `status=ok` |

Pre-decouple's binary split this into two phases (`LabelObsoletes` then `CloseBeads` with `cli.Status` check between them). Post-decouple, the adapter does the same status check internally; the contract from plan is unchanged (one close op per obsoleted bead with labels and reason). The status branch is purely adapter-internal.

The label-only branch matters because `br close` exits 3 on already-closed targets even though the labels DO get applied. Treating that exit as `status=error` would push top-level receipts to `partial` and block ingest from saving the snapshot — every modify-pair close on a previously-shipped bead would fail this way.

## Receipt Atomicity

Each op's receipt is appended to an in-memory array. After the last op, the full receipts.json is written atomically via temp file + mv (analogous to plan's --out). A reader therefore sees either the complete v1 document or the pre-run content of that path — never a half-written file.

If the adapter crashes mid-run, partial receipts are lost — the reference adapter implements no checkpointing; it's a reference, not a crash-safe production adapter. Users building production adapters add checkpointing as an enhancement.

## Receipt Shape

[[3486b44f4f64|Receipts v1]] is the adapter's only output artifact, and ingest's only input from it:

```json
{
  "version": 1,
  "status": "complete | partial",
  "ops": [
    {
      "op_id": "op-0001",
      "status": "ok | skipped | error",
      "bead_id": "<the bead reached, or empty if none was>",
      "was_existing": true,
      "reason": "<present on skipped>",
      "error": "<present on error>"
    }
  ]
}
```

In a file that gets written, every op the changeset listed has exactly one entry, and the entries are in changeset order. A run that dies mid-loop writes no file at all rather than a short one — see "The shape hole" above. `reason` appears only on a `skipped` entry and `error` only on an `error` entry; an `ok` entry carries neither. An op too malformed to have an `op_id` still gets an entry, filed under a synthetic id derived from its position, so the two lists never drift out of alignment.

## Reference-Adapter Limitations

[[7c2fea6b1963|The reference implementation's scope]] is deliberately bounded: this module's functional requirements are the contract any adapter must satisfy, and this script is one implementation of them rather than the definition. Anyone needing production hardening forks it. Explicitly documented:

- Single-threaded sequential processing. Concurrent invocations will trample receipts.
- No retry on transient `br` failures — user's responsibility to fix and re-run.
- No dry-run mode.
- No pre-flight check that the tracker state matches what plan assumed (e.g., existing bead statuses). Production adapters should add this.

## Header Notice

The script begins with:

```
#!/usr/bin/env bash
#
# apply-br.sh — Reference adapter consuming spex changeset.json v3 and invoking br.
#
# REFERENCE IMPLEMENTATION. Vet before production use. See spec/adapters/ for the
# adapter contract that any implementation (this one or your own) must satisfy.
#
# Usage: apply-br.sh [<changeset.json>] [<receipts.json>]
#
set -euo pipefail
```

## Test Hooks

- `BR_BIN` env var — overrides `br` binary path (for tests with a mock).
- `SPEX_ADAPTER_DEBUG=1` — after each op, dump the substitution table to stderr, one `op_id → bead_id` line per entry. Stdout stays clean, so a debug run still pipes its receipts.

Each defaults to the real-world value: the `br` on PATH and no debug output. The retired `SPEX_MAPPING_FILE` override is gone with the mapping lookup it served — the adapter reads no spex-owned file. The fixture runs against a stand-in `br` via `BR_BIN` at a mock binary. [[970260050e3e|The integration test against a real `br`]] sets nothing: gated on `br` being on PATH, it runs the script from a throwaway sandbox where the default already resolves to the real `br`. Both paths exercise the shipped script unmodified rather than a copy that has diverged.
