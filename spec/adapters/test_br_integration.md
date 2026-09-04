# Br integration test

End-to-end adapter run against a real br sandbox — both halves, export and apply. Gated on `br` being present on PATH; skipped otherwise. Lives at `scripts/apply-br_test.sh` — outside the Go test suite.

## Gate

```bash
if ! command -v br >/dev/null; then
    echo "br not on PATH — skipping adapter integration tests"
    exit 0
fi
```

## Scenarios

### Export against a live sandbox

- Seed br sandbox: two open tasks, one claimed (`in_progress`) task, two closed tasks.
- Run `scripts/export-br.sh tasks.json`.
- Assertions:
  - `tasks.json` validates against `schema/task-state.schema.json`.
  - It lists exactly the three unfinished tasks with their statuses as `br list` reports them, and neither closed one.
  - Feeding it to `spex plan --tasks tasks.json` over a fixture diff and journal exits 0 — the artifact the export half writes is the artifact the binary reads, with no format adapter between them.

### Full happy path

- Seed br sandbox: create a fake proposal epic and a handful of feature tasks (analogues of existing journal pairings).
- Run `scripts/apply-br.sh` with a changeset containing 2 new creates, 1 close (removed), and 1 create for a modified node whose earlier task is finished.
- Assertions:
  - All 4 ops land as receipts.
  - `br list --json` output matches expected tasks (count, statuses, labels).
  - `spex:<eid>` labels present on new tasks — each op's referent event id, `<git_head>:<op_id>`.
  - the modified node's new task carries no dependency on its finished predecessor — the changeset named none, and the adapter invented none.
  - the closed task carries no new labels — close ops apply none; its `status` is the whole record of the close.
  - Receipts top-level status: complete; `"version": 2`; every entry keyed `task_id`.

### Partial run (injected failure)

- Seed sandbox.
- Run adapter against a changeset where one create op has an invalid priority (e.g., priority=-1) that br rejects.
- Assertions:
  - Adapter processes ops until the bad one; records error receipt; continues with remaining ops OR halts depending on the adapter's documented policy (this test pins the policy).
  - Top-level status: partial.
  - Successful ops left their traces in br; failed one did not.

### Re-run idempotency

- Run the happy-path changeset once. Capture the br state.
- Run it again with the same changeset.
- Assertions:
  - Second run's receipts all show `status=ok` — `was_existing=true` on creates, the status-keyed skip branch on closes.
  - br state after second run is byte-identical to state after first run.

### Both ref shapes

- Changeset mixing ref:op (new-to-new) and ref:task (existing, listed as open) — the two shapes the changeset admits; a dep plan could not resolve never reaches the adapter.
- Assertions: each new task's `--deps blocked-by:<>` is correct per the ref resolution.

## Harness

`scripts/apply-br_test.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Gate
if ! command -v br >/dev/null; then
    echo "br not on PATH — skipping"
    exit 0
fi

SCRIPTS="$(cd "$(dirname "$0")" && pwd)"
TESTS_DIR="$SCRIPTS/testdata/integration"
for case in "$TESTS_DIR"/*/; do
    name=$(basename "$case")
    echo "--- $name ---"
    SANDBOX=$(mktemp -d)
    trap "rm -rf $SANDBOX" EXIT
    cd "$SANDBOX"
    cp -r "$case"/* .
    br init >/dev/null 2>&1

    if [[ -f seed.sh ]]; then
        ./seed.sh    # may write the ids it created into changeset.json
    fi

    if [[ -f changeset.json ]]; then
        "$SCRIPTS/apply-br.sh" changeset.json > receipts.json
        # br assigns task ids the fixture cannot predict: compare with
        # every non-empty task_id masked to the __ANY__ the expectation carries.
        diff <(jq -S '.ops |= map(.task_id |= (if . == "" then . else "__ANY__" end))' receipts.json) \
             <(jq -S . expected_receipts.json)
    else
        # No changeset: an export-half case. The argument form writes
        # tasks.json for verify.sh; the ids are the seed's, never a fixture's.
        "$SCRIPTS/export-br.sh" tasks.json
    fi

    if [[ -f verify.sh ]]; then
        ./verify.sh
    fi
    cd -
done

echo "ok"
```

## Fixtures

- `scripts/testdata/integration/export/` — seed.sh + verify.sh. There is no `expected_tasks.json`: real `br` assigns task ids the fixture cannot predict, so an exact diff of the export is unreachable, and verify.sh checks the document against the ids the seed recorded instead. This case is the one place the script's `<tasks.json>` argument form runs under test — the mock-mode export suite captures stdout.
- `scripts/testdata/integration/happy_path/` — seed.sh + changeset.json + expected_receipts.json + verify.sh. The seed establishes what the scenario presupposes: the task the close op cancels — its id written into `changeset.json`'s close target in place of a placeholder, since the fixture cannot know it — and the finished predecessor the modified node's new create must not depend on. `expected_receipts.json` carries `__ANY__` wherever a br-assigned id lands. `scripts/apply-br_test.sh` runs `seed.sh` only when the case supplies one, and runs the shipped scripts from `scripts/` rather than copies.
- `scripts/testdata/integration/close_removed/` — seed.sh + changeset.json + expected_receipts.json + verify.sh.

The partial-run, re-run-idempotency and both-ref-shape scenarios above, and the retarget path, are owed integration fixtures of the same shape and have none yet. The mock-mode suite under `scripts/testdata/{idempotency,substitution}/` covers them meanwhile against the stand-in `br`, which does not discharge what this test claims: the requirement asks for those cases against a real sandbox.

## Non-Responsibilities

- This test does not validate the adapter's bash style or its jq expressions — that's a separate linting step.
- It does not stress-test concurrent adapter runs — adapter is assumed single-instance per pipeline run.
