# Substitution table tests

Tests for the op_id → task_id substitution table that resolves `{"ref":"op","op_id":"<id>"}` refs at exec time.

## Setup

- Fixtures constructed as small changesets with known in-batch dep chains.
- The adapter runs each and the test inspects the invocations it made against br.

## Scenarios

### Linear chain: A → B → C

**Given** the `linear_chain` changeset — three create ops chained A → B → C by ref:op deps — run against the mock br.

- Changeset:
  - op-component-a1b2c3d4e5f6: create (A, no deps)
  - op-component-b2c3d4e5f6a1: create (B, deps=[{ref:op,op_id:op-component-a1b2c3d4e5f6}])
  - op-component-c3d4e5f6a1b2: create (C, deps=[{ref:op,op_id:op-component-b2c3d4e5f6a1}])
- Simulated br creates produce task ids: A=br-100, B=br-101, C=br-102.
- Expected: br invocations:
  - `br create ... (for A, no deps flag)`
  - `br create ... --deps blocked-by:br-100 (for B, resolved from op-component-a1b2c3d4e5f6)`
  - `br create ... --deps blocked-by:br-101 (for C, resolved from op-component-b2c3d4e5f6a1)`

### Parent ref:op

**Given** the `parent_ref` changeset — a create for a proposal epic and a create for a component whose parent is a ref:op naming that epic op — run against the mock br.

- Changeset:
  - op-proposal_epic-2026-04-18-decouple-spex-from-br: create proposal epic.
  - op-component-a1b2c3d4e5f6: create component with parent=`{ref:op,op_id:op-proposal_epic-2026-04-18-decouple-spex-from-br}`.
- Simulated: the epic op produces task br-50.
- Expected: the component's br create uses `--parent br-50`.

### Mixed refs in one op's deps

**Given** an op whose deps are `[{ref:op,op_id:op-component-c3d4e5f6a1b2}, {ref:task,task_id:br-77}]` — the two shapes the changeset admits, neither carrying an edge-type key.

- Simulated: op-component-c3d4e5f6a1b2 → br-200.
- Expected: `br create ... --deps blocked-by:br-200 --deps blocked-by:br-77` — one edge spelling for every dep, because v4 refs carry no type of their own.

### Ref to errored op degrades gracefully

**Given** op-component-e5f6a1b2c3d4 depending on op-component-d4e5f6a1b2c3 by ref:op, with op-component-d4e5f6a1b2c3 errored.

- Expected: op-component-e5f6a1b2c3d4 receipt `status=error`, error message `"dependency op-component-d4e5f6a1b2c3 errored; cannot resolve op ref"`. op-component-e5f6a1b2c3d4 is NOT executed against br.

### Ref to an idempotent re-match (was_existing=true) still resolves

**Given** op-component-c3d4e5f6a1b2 receipted `ok` with was_existing=true and task_id=br-10 — a create whose label matched an existing task — and op-component-e5f6a1b2c3d4 depending on it.

- Expected: op-component-e5f6a1b2c3d4's deps resolve op-component-c3d4e5f6a1b2 → br-10; execution proceeds normally.

### Forward ref before table has entry (programmer error)

**Given** a malformed changeset in which op-component-b2c3d4e5f6a1 references op-component-a1b2c3d4e5f6 which hasn't been executed yet (or doesn't exist).

- If the changeset is malformed and op-component-b2c3d4e5f6a1 references op-component-a1b2c3d4e5f6 which hasn't been executed yet (or doesn't exist), adapter errors out at op-component-b2c3d4e5f6a1 with `"dep ref: dependency op-component-a1b2c3d4e5f6 not yet resolved"`. No fixture under `scripts/testdata/substitution/` exercises this path.

### A v3 ref shape is not resolved

**Given** op-component-b2c3d4e5f6a1 carrying a dep written in the v3 task-ref spelling — the retired discriminator and its id key, as `scripts/testdata/substitution/legacy_ref/changeset.json` fixes them.

- Expected: op-component-b2c3d4e5f6a1 receipt `status=error` with reason `unknown ref kind: <the v3 discriminator>`; nothing executed against br for it. The adapter recognises exactly two discriminators and adapts no legacy one, which is what makes the version bump honest rather than cosmetic.

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
