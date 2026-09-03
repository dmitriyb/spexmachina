# Substitution table tests

Tests for the op_id → task_id substitution table that resolves `{"ref":"op","op_id":"<id>"}` refs at exec time.

## Setup

- Fixtures constructed as small changesets with known in-batch dep chains.
- The adapter runs each and the test inspects the invocations it made against br.

## Scenarios

### Linear chain: A → B → C

- Changeset:
  - op-0001: create (A, no deps)
  - op-0002: create (B, deps=[{ref:op,op_id:op-0001}])
  - op-0003: create (C, deps=[{ref:op,op_id:op-0002}])
- Simulated br creates produce task ids: A=br-100, B=br-101, C=br-102.
- Expected: br invocations:
  - `br create ... (for A, no deps flag)`
  - `br create ... --deps blocked-by:br-100 (for B, resolved from op-0001)`
  - `br create ... --deps blocked-by:br-101 (for C, resolved from op-0002)`

### Parent ref:op

- Changeset:
  - op-0001: create proposal epic.
  - op-0002: create component with parent=`{ref:op,op_id:op-0001}`.
- Simulated: op-0001 produces task br-50.
- Expected: op-0002's br create uses `--parent br-50`.

### Mixed refs in one op's deps

- op has deps: `[{ref:op,op_id:op-0003}, {ref:task,task_id:br-77}]` — the two shapes the changeset admits, neither carrying an edge-type key.
- Simulated: op-0003 → br-200.
- Expected: `br create ... --deps blocked-by:br-200 --deps blocked-by:br-77` — one edge spelling for every dep, because v4 refs carry no type of their own.

### Ref to errored op degrades gracefully

- op-0005 deps on op-0004 (ref:op). op-0004 errored.
- Expected: op-0005 receipt `status=error`, error message `"dependency op-0004 errored; cannot resolve op ref"`. op-0005 is NOT executed against br.

### Ref to an idempotent re-match (was_existing=true) still resolves

- op-0003 was a create whose label matched an existing task: receipted `ok` with was_existing=true and task_id=br-10 (the existing task). op-0005 deps on op-0003.
- Expected: op-0005's deps resolve op-0003 → br-10; execution proceeds normally.

### Forward ref before table has entry (programmer error)

- If the changeset is malformed and op-0002 references op-0001 which hasn't been executed yet (or doesn't exist), adapter errors out at op-0002 with `"dep ref: dependency op-0001 not yet resolved"`. No fixture under `scripts/testdata/substitution/` exercises this path.

### A v3 ref shape is not resolved

- op-0002 carries a dep written in the v3 task-ref spelling — the retired discriminator and its id key, as `scripts/testdata/substitution/legacy_ref/changeset.json` fixes them.
- Expected: op-0002 receipt `status=error` with reason `unknown ref kind: <the v3 discriminator>`; nothing executed against br for it. The adapter recognises exactly two discriminators and adapts no legacy one, which is what makes the version bump honest rather than cosmetic.

## Helpers

- Mock br: `scripts/testdata/mock_br.sh` — echoes a canned response, records invocations to a log file for inspection.
- Fixture cases: `scripts/testdata/substitution/<case>/` — each holding `changeset.json`, `state_before.json` and `expected_receipts.json`, plus `expected_log.txt` where the case pins the br invocation sequence (all but `errored_dep`). `scripts/apply-br_test.sh` compares the log only when the file is present.

## Fixtures

- `scripts/testdata/substitution/linear_chain/`
- `scripts/testdata/substitution/parent_ref/`
- `scripts/testdata/substitution/mixed_refs/`
- `scripts/testdata/substitution/errored_dep/`
- `scripts/testdata/substitution/skipped_dep_resolves/`
- `scripts/testdata/substitution/legacy_ref/`
