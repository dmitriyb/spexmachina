# Changeset builder tests

Integration tests for `ChangesetBuilder` against synthetic impact reports and mapping stores.

## Setup

- Use `testdata/` fixtures: impact_report.json, bead_map.json, project.json, and a minimal spec tree.
- Set `--git-head` to a fixed SHA string for byte-identical output assertions.

## Scenarios

### Canonical schema and field order

- Build a changeset from a single-create impact report. Assert the output has `"version": 1` at the top, `git_head` set to the fixed SHA, and canonical field order on every op (`op_id`, `type`, `target`, `spec_node_id`, `idempotency`, `parent`, `deps`, `priority`, `title`, `body`).
- Same inputs produce byte-identical output across two runs.

### Proposal epic parents every non-epic create

- Impact report with one proposal, one component create, one data_flow create. Assert: first op is `type: create` with `spec_node_kind: proposal_epic`; following two creates' `parent` fields are `{"ref":"op","op_id":"<epic op_id>"}`.

### In-batch dep chain resolves to ref:op

- Three new components A, B, C where B uses A and C uses B. Impact emits DepSpecNodeIDs on each. Assert:
  - A's create has empty `deps`.
  - B's create has `deps: [{"ref":"op","op_id":"<A op>"}]`.
  - C's create has `deps: [{"ref":"op","op_id":"<B op>"}]`.

### Existing-bead dep resolves to ref:bead

- New component X uses existing-and-open component Y (Y has a mapping record with `status: "open"`). Assert X's create has `deps: [{"ref":"bead","bead_id":"<Y's bead>"}]`.

### Closed-bead dep is dropped

- New component X uses Y; Y's mapping record has `status: "closed"`. Assert X's `deps` is empty (closed dependency is satisfied — no edge).

### Spec-node fallback when mapping has no record

- New component X uses spec_node Z; Z has no mapping record and no in-batch op. Assert X's deps include `{"ref":"spec_node","spec_node_id":"<Z>"}` so the adapter can resolve at exec time.

### Priority propagation

- Component implements two module requirements with preq priorities `2` and `1`. Assert the create op's `priority` is `1` (lowest wins).
- Component implements a module requirement with no priority chain (no preq_id found upstream). Assert priority defaults per the documented fallback (e.g., 3 or omitted — verify against impl).

### Obsolete + create lineage

- Modified component Q: old bead `spexmachina-abc` closed, new create op. Assert:
  - Close op carries `target: {"ref":"bead","bead_id":"spexmachina-abc"}` and `labels: ["spex:obsolete","commit:<git-head>"]`.
  - Create op for the replacement includes `deps: [{"ref":"bead","bead_id":"spexmachina-abc","type":"blocks"}]` for lineage.

### Error: cycle in batch deps

- Constructed impact report with in-batch cycle (A uses B, B uses A — invalid spec, but fabricate). Assert ChangesetBuilder returns a structured error naming the cycle; no partial changeset written.

## Fixtures

Under `emit/testdata/`:
- `changeset_canonical.json` — expected output for the canonical test.
- `impact_chain.json` — dep-chain fixture.
- `bead_map_mixed.json` — mix of open, closed, and missing mapping records.

## Edge cases

- Empty impact report → changeset with only the proposal epic op (if any) or an empty op list.
- Impact report with only closes (no creates) → no parent/dep resolution needed.
- DepSpecNodeIDs with a spec_node_id that is itself a closed bead AND a fresh create in the same batch → the open in-batch op wins (ref:op) over the closed bead (skipped).
