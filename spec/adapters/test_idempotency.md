# Idempotency tests

Tests that exercise the adapter's idempotency guarantees on both create and close ops.

## Setup

- Synthetic changeset fixtures under `scripts/testdata/idempotency/`.
- A br sandbox (fresh `.beads/` directory per test via `br init`) OR a mocked br CLI that records invocations.
- The adapter under test is `scripts/apply-br.sh`.

## Scenarios

### Create: first run

- Changeset with one create op, idempotency.label = `spex:100`.
- br sandbox empty.
- Expected: `br create` invoked; new bead created; receipt `was_existing=false`, `bead_id=<new>`, `status=ok`.

### Create: re-run matches existing

- Same changeset, same label `spex:100`.
- br sandbox has a bead with label `spex:100` (from a prior run).
- Expected: `br create` NOT invoked; `br list --json | jq` locates the existing bead; receipt `was_existing=true`, `bead_id=<existing>`, `status=ok`.

### Create: label mismatch surfaces as new create

- Changeset label `spex:200`. Sandbox has a bead with label `spex:100` (different record-id).
- Expected: `br create` invoked (no match on 200); new bead; receipt `was_existing=false`.

### Close: first run

- Changeset close op targeting bead `br-abc`. `br-abc` exists and is open.
- Expected: `br update br-abc --add-label spex:obsolete --add-label commit:HEAD` and `br close br-abc` invoked; receipt `status=ok`.

### Close: re-run of already-obsoleted bead

- Changeset close op targeting bead `br-abc`. `br-abc` already has label `spex:obsolete`.
- Expected: `br close` NOT invoked; receipt `status=skipped`, reason `"already obsoleted"`.

### Close: bead doesn't exist

- Changeset close op targets `br-xyz`. Tracker has no such bead.
- Expected: receipt `status=error`, error message includes the bead_id and tracker response.

### Full idempotent round-trip

- Changeset with 3 creates (2 new, 1 matching existing) and 2 closes (1 open, 1 already-obsoleted).
- Run adapter once. Assert all 5 receipts match expectations.
- Run adapter AGAIN with the same inputs.
- Expected: second run's receipts all show `was_existing=true` / `status=skipped` as appropriate; tracker state is identical before and after the second run.

## Fixtures

- `scripts/testdata/idempotency/changeset_*.json`
- `scripts/testdata/idempotency/br_sandbox_*.jsonl` (seed data for the sandbox)

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
