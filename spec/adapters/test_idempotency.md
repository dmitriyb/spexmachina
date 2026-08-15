# Idempotency tests

Tests that exercise the adapter's idempotency guarantees on both create and close ops.

## Setup

- Synthetic changeset fixtures under `scripts/testdata/idempotency/`.
- A br sandbox (fresh `.beads/` directory per test via `br init`) OR a mocked br CLI that records invocations.
- The adapter under test is `scripts/apply-br.sh`.

## Scenarios

### Create: first run

- Changeset with one create op, idempotency.label = `spex:cafe1234:op-1`.
- br sandbox empty.
- Expected: `br create` invoked; new bead created; receipt `was_existing=false`, `bead_id=<new>`, `status=ok`.

### Create: re-run matches existing

- Same changeset, same label `spex:cafe1234:op-1`.
- br sandbox has a bead with label `spex:cafe1234:op-1` (from a prior run).
- Expected: `br create` NOT invoked; the status-unfiltered probe (`br list --json --all --limit 0 --label …`) locates the existing bead; receipt `was_existing=true`, `bead_id=<existing>`, `status=ok`.

### Create: re-run matches a closed bead too

- Same changeset, same label. The sandbox bead carrying `spex:cafe1234:op-1` is `status=closed`.
- Expected: still a match — exact label, any status; `br create` NOT invoked; receipt `was_existing=true`, `bead_id=<existing>`, `status=ok`. The open-only filter died with the node-keyed labels whose collisions it dodged: a bead carrying this exact eid label, whatever its status, can only be this op's own earlier product.

### Create: label mismatch surfaces as new create

- Changeset label `spex:<eid-A>`. Sandbox has a bead with label `spex:<eid-B>` (a different event's eid — including a closed modify-pair predecessor, whose label is a different eid by construction).
- Expected: `br create` invoked; new bead; receipt `was_existing=false`.

### Create: cleanup-bead production

- Changeset create op with `spec_node_kind: "cleanup"`, `idempotency.label: "spex:cafe1234:op-9"` (the eid of the removal event the same-batch close op `op-9` implies), `title: "Code cleanup: m/X"`, `labels: ["spex:cleanup"]`, `deps: [{ref:bead, bead_id:"spexmachina-old", type:"blocks"}]`, `priority: 3`.
- br sandbox empty.
- Expected: idempotency check via `br list --json --all --limit 0 --label spex:cafe1234:op-9` finds nothing → adapter invokes `br create --title "Code cleanup: m/X" --labels spex:cafe1234:op-9 --type task --json --priority 3 --deps blocks:spexmachina-old`, then `br update <new> --add-label spex:cleanup`. Receipt `status=ok`, `was_existing=false`, `bead_id=<new>`. After the run, `br show <new> --json` returns `issue_type=task` and `labels` contains both `spex:cleanup` and `spex:cafe1234:op-9`.
- Rationale: cleanup beads carry distinct shape per the pre-decouple `apply/bead_creator.go::createCleanupBead` contract. The discriminator label `spex:cleanup` is what marks a bead as cleanup bookkeeping — its journal receipt pairs with the removal event it answers rather than a fresh change event, and any tracker-side query for cleanup work keys off it; the idempotency label is that removal event's own eid, so the linkage key and the receipt's referent are one fact. Type `task` (not `feature` derived from the underlying `component` kind) marks cleanup as bookkeeping work.

### Create: cleanup-bead re-run is idempotent

- Same changeset as above. Sandbox already has a bead with label `spex:cafe1234:op-9` (from a prior run).
- Expected: `br create` NOT invoked; `br list --json --all --limit 0 --label spex:cafe1234:op-9` finds the existing bead; receipt `was_existing=true`, `bead_id=<existing>`, `status=ok`.
- This is what pre-decouple lacked — `createCleanupBead` had no idempotency check and would create duplicates on every re-run.

### Close: first run

- Changeset close op targeting bead `br-abc`. `br-abc` exists and is open.
- Expected: `br update br-abc --add-label spex:obsolete` then `br update br-abc --add-label commit:<HEAD>` (one call per label), then `br close br-abc --force`, plus `--reason <op.reason>` when the op carries one — the `close_first_run` fixture does — invoked; receipt `status=ok`.

### Close: re-run of already-obsoleted bead

- Changeset close op targeting bead `br-abc`. `br-abc` already has label `spex:obsolete`.
- Expected: `br close` NOT invoked; receipt `status=skipped`, reason `"already obsoleted"`.

### Close: bead is already closed without `spex:obsolete`

- Changeset close op targeting bead `br-abc`. `br-abc` is `status=closed` (closed by an earlier bead-lifecycle run, after a previous PR shipped) but its `labels` array does NOT contain `spex:obsolete`.
- Expected: `br update --add-label spex:obsolete --add-label commit:HEAD` invoked (the labels apply); `br close` NOT invoked (the close transition is a no-op against an already-closed target — `br close` exits 3 in that case); receipt `status=ok`.
- Rationale: this is the third branch of the close-op idempotency table in `arch_br_reference_adapter.md`. Without it, every modify-pair close on a previously-shipped component records `status=error`, top-level receipts go `partial`, and ingest's atomicity gate refuses to save the snapshot — blocking the whole run.

### Close: bead doesn't exist

- Changeset close op targets `br-xyz`. Tracker has no such bead.
- Expected: receipt `status=error`, error message includes the bead_id and tracker response.

### Retarget: first run

- Changeset retarget op targeting bead `br-open` with `labels: ["spex:cafe1234:op-4"]` and two
  deps resolving to `br-dep1` (which `br-open` already carries) and `br-dep2` (which it does not).
- Expected: no probe — no `br list` precedes the update; `br show br-open --format json` reads
  current deps; `br update br-open --add-label spex:cafe1234:op-4` for the label and
  `br dep add br-open br-dep2 --type blocks` for the missing dep only — a second `br update` is
  not what carries a dep, and `blocks` is the tracker's name for the edge the changeset's create
  path spells `blocked-by`; nothing removed. Receipt `status=ok`, `bead_id=br-open`, no
  `was_existing` field.

### Retarget: re-run converges

- Same changeset again. `br-open` now carries the event label and both deps.
- Expected: the update adds nothing new and errors nothing, and the current-deps read leaves no
  dep missing, so no `br dep add` is invoked at all — `status=ok`, tracker state identical before
  and after. Idempotency here is the calls' own convergence, not a label probe: were a dep-add
  issued anyway, an edge the bead already carries is a no-op rather than a failure.

### Full idempotent round-trip

- Changeset with 3 creates (2 new, 1 matching existing) and 2 closes (1 open, 1 already-obsoleted).
- Run adapter once. Assert all 5 receipts match expectations.
- Run adapter AGAIN with the same inputs.
- Expected: second run's receipts show `status=ok` with `was_existing=true` on every create and `status=skipped` (`"already obsoleted"`) on every close; tracker state is identical before and after the second run.

## Fixtures

- `scripts/testdata/idempotency/<case>/changeset.json`
- `scripts/testdata/idempotency/<case>/state_before.json` (seed state for `scripts/testdata/mock_br.sh`)

## Test Harness

`scripts/apply-br_test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail
require_br_on_path
for case in scripts/testdata/idempotency/*/; do
    setup_sandbox "$case"
    actual=$(scripts/apply-br.sh "$case/changeset.json")
    expected=$(cat "$case/expected_receipts.json")
    diff <(jq -S . <<< "$actual") <(jq -S . <<< "$expected")
done
```
