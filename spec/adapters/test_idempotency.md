# Idempotency tests

Tests that exercise the adapter's idempotency guarantees on both create and close ops, plus the export half's determinism.

## Setup

- Synthetic changeset fixtures under `scripts/testdata/idempotency/`.
- A mocked br CLI that records invocations — `scripts/testdata/mock_br.sh`, reached through the adapter's `BR_BIN` override — so the suite runs wherever `jq` runs, with no `br` on PATH. The real-sandbox counterpart is `test_br_integration.md`, the one suite gated on `br`; both live in the same harness file.
- The adapter under test is `scripts/apply-br.sh`; the export scenarios drive `scripts/export-br.sh`.

## Scenarios

### Create: first run

**Given** a changeset with one create op whose idempotency.label is `spex:cafe1234:op-component-a1b2c3d4e5f6`, and an empty br sandbox.

- Expected: `br create` invoked; new task created; receipt `was_existing=false`, `task_id=<new>`, `status=ok`.

### Create: re-run matches existing

**Given** the same changeset with the same label `spex:cafe1234:op-component-a1b2c3d4e5f6`, and a br sandbox holding a task carrying that label from a prior run.

- Expected: `br create` NOT invoked; the status-unfiltered probe (`br list --json --all --limit 0 --label …`) locates the existing task; receipt `was_existing=true`, `task_id=<existing>`, `status=ok`.

### Create: re-run matches a closed task too

**Given** the same changeset and the same label, and a sandbox whose task carrying `spex:cafe1234:op-component-a1b2c3d4e5f6` is closed in the tracker.

- Expected: still a match — exact label, any status; `br create` NOT invoked; receipt `was_existing=true`, `task_id=<existing>`, `status=ok`. The open-only filter died with the node-keyed labels whose collisions it dodged: a task carrying this exact eid label, whatever its status, can only be this op's own earlier product.

### Create: label mismatch surfaces as new create

**Given** a changeset label `spex:<eid-A>` and a sandbox holding a task with label `spex:<eid-B>` (a different event's eid — including a finished predecessor of the same node, whose label is a different eid by construction).

- Expected: `br create` invoked; new task; receipt `was_existing=false`.

### Create: cleanup task production

**Given** a changeset create op with `spec_node_kind: "cleanup"`, `idempotency.label: "spex:E1"` (the eid of the removal event the cleanup answers), `title: "Code cleanup: m/X"`, no `labels` key, no `deps` and `priority: 3`, and an empty br sandbox.

- Expected: idempotency check via `br list --json --all --limit 0 --label spex:E1` finds nothing → adapter invokes `br create --title "Code cleanup: m/X" --labels spex:E1 --type task --json --priority 3` — no `--deps` flag at all, because the cleanup carries no lineage edge to the finished task it follows, and no `br update` — a cleanup create carries no `op.Labels` to apply. Receipt `status=ok`, `was_existing=false`, `task_id=<new>`. After the run, `br show <new> --json` returns `issue_type=task`, `labels` contains exactly `spex:E1`, and no dependency.
- Rationale: what marks the task as cleanup bookkeeping is the journal, not a tracker label or edge — its receipt pairs with the removal event it answers rather than a fresh change event, and "is this task cleanup?" is answered by the `removed` event its `task_created` references; the idempotency label is that removal event's own eid, so the linkage key and the receipt's referent are one fact. Type `task` (not `feature` derived from the underlying `component` kind) marks cleanup as bookkeeping work.

### Create: cleanup task re-run is idempotent

**Given** the same cleanup changeset, and a sandbox already holding a task with label `spex:E1` from a prior run.

- Expected: `br create` NOT invoked; `br list --json --all --limit 0 --label spex:E1` finds the existing task; receipt `was_existing=true`, `task_id=<existing>`, `status=ok`.
- This is what pre-decouple lacked — the in-binary cleanup creator had no idempotency check and would create duplicates on every re-run.

### Close: first run

**Given** a changeset close op targeting task `br-abc` with no `labels` key, and `br-abc` existing and open.

- Expected: `br show br-abc --format json` reads the status; no `br update` of any kind — close ops carry no labels, the close markers of the label era with them; `br close br-abc --force` invoked, plus `--reason <op.reason>` when the op carries one — the `close_first_run` fixture does; receipt `status=ok`.

### Close: re-run against an already-closed task

**Given** a changeset close op targeting task `br-abc`, and `br-abc` closed in the tracker by an earlier run of this changeset — nothing else issues a close for a task plan read as live.

- Expected: `br close` NOT invoked (the close transition is a no-op against an already-closed target — `br close` exits 3 in that case); no `br update`; receipt `status=ok`.
- Rationale: this is the skip branch of the close-op idempotency table in `arch_br_reference_adapter.md`, keyed on the tracker's own status and nothing else. Convergence on `status=ok` (not `skipped`) is deliberate: the journal's eid-deduped receipts absorb a re-run without a second `task_closed` line.

### Close: task doesn't exist

**Given** a changeset close op targeting `br-xyz`, and a tracker with no such task.

- Expected: receipt `status=error`, error message includes the task_id and tracker response.

### Retarget: first run

**Given** a changeset retarget op targeting task `br-open` with `labels: ["spex:cafe1234:op-retarget-a1b2c3d4e5f6"]` and two deps resolving to `br-dep1` (which `br-open` already carries) and `br-dep2` (which it does not).

- Expected: no probe — no `br list` precedes the update; `br show br-open --format json` reads
  current deps; `br update br-open --add-label spex:cafe1234:op-retarget-a1b2c3d4e5f6` for the label and
  `br dep add br-open br-dep2 --type blocks` for the missing dep only — a second `br update` is
  not what carries a dep, and `blocks` is the tracker's name for the edge the changeset's create
  path spells `blocked-by`; nothing removed. Receipt `status=ok`, `task_id=br-open`, no
  `was_existing` field.

### Retarget: re-run converges

**Given** the same changeset again, with `br-open` now carrying the event label and both deps.

- Expected: the update adds nothing new and errors nothing, and the current-deps read leaves no
  dep missing, so no `br dep add` is invoked at all — `status=ok`, tracker state identical before
  and after. Idempotency here is the calls' own convergence, not a label probe: were a dep-add
  issued anyway, an edge the task already carries is a no-op rather than a failure.

### Export: the artifact is a projection of the live listing

**Given** a sandbox holding three tasks: one open, one in_progress, one closed.

- Run `scripts/export-br.sh` with stdout captured.
- Expected: a document validating against the task-state schema — `"version": 1` and a `tasks`
  array holding exactly two entries, `{task_id, status}` for the open and the in_progress task, in
  the order `br list` returned them, and nothing for the closed one. No entry carries a third
  key: the listing's `labels`, `title`, `issue_type` and the rest are dropped, not copied.

### Export: empty and deterministic

**Given** a sandbox holding only closed tasks, and separately an empty sandbox.

- Expected: `{"version": 1, "tasks": []}` from both — an empty list, not an absent file and not
  an error. Run the export twice over an unchanged sandbox and assert byte-identical output.

### Full idempotent round-trip

**Given** a changeset with 3 creates (2 new, 1 matching existing) and 2 closes (1 open, 1 already closed).

- Run adapter once. Assert all 5 receipts match expectations.
- Run adapter AGAIN with the same inputs.
- Expected: second run's receipts show `status=ok` on every op — `was_existing=true` on every create, the status-keyed skip branch on every close; tracker state is identical before and after the second run.

## Fixtures

- `scripts/testdata/idempotency/<case>/changeset.json`
- `scripts/testdata/idempotency/<case>/state_before.json` (seed state for `scripts/testdata/mock_br.sh`)
- `scripts/testdata/export/<case>/state_before.json` plus `expected_tasks.json` for the export scenarios

## Test Harness

`scripts/apply-br_test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
MOCK_BR=scripts/testdata/mock_br.sh
for case in scripts/testdata/idempotency/*/; do
    seed_mock_state "$case/state_before.json"      # the mock's state, never a real tracker
    actual=$(BR_BIN="$MOCK_BR" scripts/apply-br.sh "$case/changeset.json")
    expected=$(cat "$case/expected_receipts.json")
    diff <(jq -S . <<< "$actual") <(jq -S . <<< "$expected")
done
for case in scripts/testdata/export/*/; do
    seed_mock_state "$case/state_before.json"
    diff <(BR_BIN="$MOCK_BR" scripts/export-br.sh | jq -S .) <(jq -S . "$case/expected_tasks.json")
done
```

No PATH gate: the mock is a script in the tree, so these cases run unconditionally, and only the integration suite behind them checks for `br`.
