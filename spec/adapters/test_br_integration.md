# Br integration test

End-to-end adapter run against a real br sandbox. Gated on `br` being present on PATH; skipped otherwise. Lives at `scripts/apply-br_test.sh` — outside the Go test suite.

## Gate

```bash
if ! command -v br >/dev/null; then
    echo "br not on PATH — skipping adapter integration tests"
    exit 0
fi
```

## Scenarios

### Full happy path

- Seed br sandbox: create a fake proposal epic and a handful of feature beads (analogues of existing journal pairings).
- Run `scripts/apply-br.sh` with a changeset containing 2 new creates, 1 close (removed), 1 modified close+create pair.
- Assertions:
  - All 5 ops land as receipts.
  - `br list --json` output matches expected beads (count, statuses, labels).
  - `spex:<eid>` labels present on new beads — each op's referent event id, `<git_head>:<op_id>`.
  - `spex:obsolete` + `commit:<HEAD>` labels present on the closed beads.
  - Receipts top-level status: complete.

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
  - Second run's receipts all show `was_existing=true` on creates, `status=skipped` with reason "already obsoleted" on closes.
  - br state after second run is byte-identical to state after first run.

### Both ref shapes

- Changeset mixing ref:op (new-to-new) and ref:bead (existing open) — the two shapes the changeset admits; a dep plan could not resolve never reaches the adapter.
- Assertions: each new bead's `--deps blocked-by:<>` is correct per the ref resolution.

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

TESTS_DIR="$(dirname "$0")/testdata/integration"
for case in "$TESTS_DIR"/*/; do
    name=$(basename "$case")
    echo "--- $name ---"
    SANDBOX=$(mktemp -d)
    trap "rm -rf $SANDBOX" EXIT
    cd "$SANDBOX"
    cp -r "$case"/* .
    br init >/dev/null 2>&1

    if [[ -f seed.sh ]]; then
        ./seed.sh
    fi

    ./apply-br.sh < changeset.json > receipts.json
    diff <(jq -S . receipts.json) <(jq -S . expected_receipts.json)

    if [[ -f verify.sh ]]; then
        ./verify.sh
    fi
    cd -
done

echo "ok"
```

## Fixtures

- `scripts/testdata/integration/happy_path/` — changeset.json + expected_receipts.json + verify.sh. It seeds nothing: `scripts/apply-br_test.sh:172` runs `seed.sh` only when the case supplies one.
- `scripts/testdata/integration/close_obsolete/` — seed.sh + changeset.json + expected_receipts.json + verify.sh.

The partial-run, re-run-idempotency and both-ref-shape scenarios above have no integration fixture, and neither does the retarget path; their coverage is the mock-mode suite under `scripts/testdata/{idempotency,substitution}/`.

## Non-Responsibilities

- This test does not validate the adapter's bash style or its jq expressions — that's a separate linting step.
- It does not stress-test concurrent adapter runs — adapter is assumed single-instance per pipeline run.
