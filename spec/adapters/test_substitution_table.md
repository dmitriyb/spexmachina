# Substitution table tests

Tests for the op_id → bead_id substitution table that resolves `{"ref":"op","op_id":"<id>"}` refs at exec time.

## Setup

- Fixtures constructed as small changesets with known in-batch dep chains.
- The adapter runs each and the test inspects the invocations it made against br.

## Scenarios

### Linear chain: A → B → C

- Changeset:
  - op-0001: create (A, no deps)
  - op-0002: create (B, deps=[{ref:op,op_id:op-0001}])
  - op-0003: create (C, deps=[{ref:op,op_id:op-0002}])
- Simulated br creates produce bead_ids: A=br-100, B=br-101, C=br-102.
- Expected: br invocations:
  - `br create ... (for A, no deps flag)`
  - `br create ... --deps depends:br-100 (for B, resolved from op-0001)`
  - `br create ... --deps depends:br-101 (for C, resolved from op-0002)`

### Parent ref:op

- Changeset:
  - op-0001: create proposal epic.
  - op-0002: create component with parent=`{ref:op,op_id:op-0001}`.
- Simulated: op-0001 produces bead br-50.
- Expected: op-0002's br create uses `--parent br-50`.

### Mixed refs in one op's deps

- op has deps: `[{ref:op,op_id:op-0003}, {ref:bead,bead_id:br-77}, {ref:spec_node,spec_node_id:abcdef123456}]`.
- Simulated: op-0003 → br-200; spec_node abcdef123456 resolves via mapping store to br-88.
- Expected: `br create ... --deps depends:br-200 --deps depends:br-77 --deps depends:br-88`.

### Ref to errored op degrades gracefully

- op-0005 deps on op-0004 (ref:op). op-0004 errored.
- Expected: op-0005 receipt `status=error`, error message `"dependency op-0004 errored; cannot resolve op ref"`. op-0005 is NOT executed against br.

### Ref to skipped op (was_existing=true) still resolves

- op-0003 was a create, receipted skipped with was_existing=true and bead_id=br-10 (the existing bead). op-0005 deps on op-0003.
- Expected: op-0005's deps resolve op-0003 → br-10; execution proceeds normally.

### Forward ref before table has entry (programmer error)

- If the changeset is malformed and op-0002 references op-0001 which hasn't been executed yet (or doesn't exist), adapter errors out at op-0002 with `"unknown op_id in ref: op-0001"`.

## Helpers

- Mock br: `scripts/testdata/mock_br.sh` — echoes a canned response, records invocations to a log file for inspection.
- Sandbox: `scripts/testdata/sandbox/` — fixture helpers.

## Fixtures

- `scripts/testdata/substitution/linear_chain.json`
- `scripts/testdata/substitution/parent_ref.json`
- `scripts/testdata/substitution/mixed_refs.json`
- `scripts/testdata/substitution/errored_dep.json`
