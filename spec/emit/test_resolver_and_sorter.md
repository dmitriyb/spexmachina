# Resolver and sorter tests

Unit-adjacent integration tests for `Resolver`, `TopologicalSorter`, and `IdempotencyLabeler`. These components compose into `ChangesetBuilder`; tests here exercise them against direct inputs without full-pipeline assembly.

## Resolver

### Classifies each dep into the correct ref shape

- Input: a create action with `DepSpecNodeIDs: [A, B, C]`, a set of other in-batch ops creating spec_node B, a mapping store with spec_node A → open bead `br-1`, and no record for C.
- Expected: the create's `deps` array contains `{ref:bead,bead_id:"br-1"}` for A, `{ref:op,op_id:"<B op>"}` for B, `{ref:spec_node,spec_node_id:"C"}` for C.

### Drops closed-bead deps

- Input: DepSpecNodeIDs contains a spec_node with a `status: "closed"` mapping record.
- Expected: that dep is not present in the resolved deps array.

### Priority: walks implements → preq_id → project priority

- Component implements module requirements R1 and R2; R1.preq_id → project req P1 (priority 2); R2.preq_id → project req P2 (priority 1).
- Expected: resolved priority is 1.

### Priority: fallback when chain cannot be walked

- Component implements a module requirement whose preq_id does not resolve (project req missing).
- Expected: fallback priority applied per the implementation's rule (document what it is — e.g., 3), and a validator-level warning is surfaced upstream, not an emit error.

### Parent resolution

- Run contains a proposal epic op with `op_id: "op-001"`. A component create comes later. Expected: parent is `{ref:op,op_id:"op-001"}`.
- Run does NOT create a proposal epic; the proposal epic already exists in the mapping store (re-run case). Expected: parent is `{ref:bead,bead_id:"<existing epic bead>"}`.

## TopologicalSorter

### Linear chain

- Input ops: [A, B, C] where C depends on B depends on A (via `DepSpecNodeIDs` resolving to same-batch op_ids).
- Expected order: A, B, C.

### Diamond

- A has two deps B and C; both B and C depend on D.
- Expected order: D, (B and C in spec_node_id lex order), A.

### Independent ops: lex tiebreak

- Three ops with no deps, spec_node_ids "003abc", "001def", "002ghi".
- Expected order: 001def, 002ghi, 003abc.

### Cycle detected

- A deps B; B deps A.
- Expected: structured error naming both spec_node_ids in the cycle.

### Type-tier respected

- Input: one proposal-epic op, two feature (component) ops, one task (multi-component test) op. Feature ops depend on the epic; task op depends on features.
- Expected order: epic, features (lex tiebreak within tier), task.

## IdempotencyLabeler

### Monotonic label assignment

- Mapping store next-record-id counter is 42. Input: 3 new create ops.
- Expected: idempotency.label values are `spex:42`, `spex:43`, `spex:44`; counter advanced to 45.

### Labels reserved at emit time survive to ingest

- Simulate the emit→ingest round-trip: the labels reserved by emit appear unchanged in ingest's reconciled records. (This is a contract check — actual roundtrip lives in `test_end_to_end_pipeline.md`.)

### No label reuse across runs

- Run emit twice back-to-back with the mapping store persisted. Expected: the second run's first label is strictly greater than the first run's last label.

## Fixtures

- `testdata/impact_chain.json` and `testdata/impact_diamond.json` for sorter scenarios.
- `testdata/bead_map_for_priority.json` with the priority-chain fixture.
