# End-to-end pipeline consistency

Cross-module test scenario covering emit + adapter + ingest as a single pipeline run. Verifies that `.bead-map.json` stays consistent with the tracker state through create, close, modified (close+create), partial failure, and re-run.

## Modules Under Test

- **emit** — produces changeset.json with correctly ordered ops and three-shape refs.
- **adapters** — BrReferenceAdapter consumes changeset.json against a real br sandbox, writes receipts.json.
- **ingest** — reconciles receipts into .bead-map.json, writes snapshot iff complete.
- **map** — owns .bead-map.json invariants.
- **merkle** — provides the snapshot that ingest saves.

## Pipeline Shape

```
spec change  ──▶  spex impact (with --beads)  ──▶  spex emit
                                                     │
                                                     ▼
                                             changeset.json
                                                     │
                                                     ▼
                                          scripts/apply-br.sh
                                                     │
                                                     ▼
                                              receipts.json
                                                     │
                                                     ▼
                                            spex ingest  ──▶  .bead-map.json
                                                                  + snapshot (iff complete)
```

## Scenarios

### Scenario 1: happy path complete

Setup: a synthetic spec change adding one module and modifying one component. br sandbox seeded with the current mapping's beads.

Steps:
1. `spex merkle diff > diff.json`
2. `br list --json | spex impact --beads=/dev/stdin --diff diff.json > impact.json`
3. `spex emit --proposal <ref> --git-head <sha> --impact impact.json > changeset.json`
4. `scripts/apply-br.sh < changeset.json > receipts.json`
5. `spex ingest --changeset changeset.json --receipts receipts.json`

Assertions:
- receipts.json status is `complete`.
- `.bead-map.json` after ingest has records for every spec node that got a bead; bead_ids match receipts.
- `spec/.snapshot.json` is rewritten to reflect the new spec state.
- Running the same pipeline again with no spec changes produces an empty impact report, no ops, no-op ingest.

### Scenario 2: partial failure then retry

Setup: same spec change as Scenario 1, but mock the adapter to fail on the last create op (simulating a tracker error).

Steps 1-4 as before, adapter writes `receipts.json` with `status: partial` and one error op.

Step 5: `spex ingest` reconciles the successful ops, leaves the errored op untouched, does NOT save the snapshot.

Assertions after Run 1:
- `.bead-map.json` has records for all ok-create ops, no record for the errored op.
- `spec/.snapshot.json` unchanged on disk.
- Exit code 0 (partial is not a hard failure).

Run 2 (retry): fix the mock, re-run steps 1-5. Emit sees only the missing op in the impact report (the ok ops are now mapped). Emit reserves the same label for the retry. Adapter succeeds. Ingest commits the last record and saves the snapshot.

Assertions after Run 2:
- `.bead-map.json` is fully consistent.
- Snapshot is saved and matches the spec.
- Running a third time produces no new ops.

### Scenario 3: modified node obsolete+create lineage

Setup: modify one component's description (triggering a modified change).

Assertions:
- Close op for the old bead carries `spex:obsolete` + `commit:<HEAD>` labels.
- Create op for the replacement has `deps: [{ref:bead,bead_id:<old>,type:"blocks"}]`.
- After ingest: the mapping record for that spec_node_id has `bead_id` pointing to the NEW bead, not the old one.
- The old bead in br has `status: closed` with the obsolete labels.

### Scenario 4: removed node with closed bead → cleanup bead

Setup: delete a component whose bead is `closed` in the mapping.

Assertions:
- Impact report includes a cleanup create for the removed spec_node.
- Changeset has a close op for the old bead (no-op on tracker since already closed — adapter records skipped) AND a create op for the cleanup task bead.
- After ingest: the original spec_node's mapping record is deleted; the cleanup bead has a new mapping record.

### Scenario 5: same-run dep chain (the bug fix)

Setup: three new components A, B, C where B uses A and C uses B.

Assertions:
- Emit's changeset has A's create before B's; B's deps = `[{ref:op,op_id:<A op>}]`. Same for C → B.
- Adapter runs create A → bead_A, then create B with `--deps depends:bead_A`, then C with `--deps depends:bead_B`.
- After ingest: all three beads exist in br; their `--deps depends:` edges connect new-to-new.
- Verify this is NOT the old broken pattern (where B would have pointed to a stale pre-existing bead).

## Fixtures and Harness

- Harness lives as a shell script outside the Go test surface: `scripts/pipeline_test.sh`.
- Gated on `br` being present; skipped otherwise.
- Synthetic spec tree under `scripts/testdata/pipeline_e2e/`.
- Deterministic `--git-head` value for reproducible output.

## Invariants Enforced

After every run, the test asserts:
1. Every successful create receipt has a mapping record.
2. Every successful close-on-removed has no mapping record.
3. Modified pairs point to new bead_id.
4. No orphan records.
5. No duplicate records.
6. Snapshot saved iff complete.
7. `.bead-map.json` passes schema.

These are the seven consistency invariants defined in `spec/ingest/test_consistency_invariants.md`; this scenario exercises them against a real br sandbox rather than synthetic fixtures.
