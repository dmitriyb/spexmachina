# Tagging and Snapshot Tests

Integration and acceptance tests for ProposalTagger (component 4) and SnapshotSaver (component 5). These tests verify that proposal references are correctly applied to all affected beads and that the merkle snapshot is saved only after all actions complete successfully.

## Setup

### Test Fixture: Fake BeadCLI for Tagging

Extends the fake from `test_bead_actions.md` to track `--add-label spec_proposal:<ref>` calls separately from metadata updates:

```go
type fakeBeadCLI struct {
    // ... fields from test_bead_actions.md ...
    tagCalls    []tagCall    // records (beadID, proposalRef) pairs
    tagErrors   map[string]error // bead ID -> error to return for tagging
}

type tagCall struct {
    BeadID      string
    ProposalRef string
}
```

### Test Fixture: Temporary Spec Directory

SnapshotSaver tests use a temporary directory populated with a minimal valid spec structure:

```
tmpdir/
  project.json       (minimal: name, modules referencing "testmod")
  testmod/
    module.json       (minimal: name, one component, one impl_section)
    arch_widget.md    (content leaf)
    impl_widget.md    (content leaf)
```

This directory is passed to `SaveSnapshot` as the `specDir` parameter. The `createdAt` timestamp is fixed to `2026-01-15T10:00:00Z` for deterministic output.

### Test Fixture: Bead ID Sets

Standard bead ID sets from the preceding apply steps:

- **Creates only**: `["spexmachina-101", "spexmachina-102"]`
- **Closes only**: `["spexmachina-42"]`
- **Reviews only**: `["spexmachina-77", "spexmachina-78"]`
- **Mixed**: all five IDs from the above sets combined
- **Empty**: `[]` (impact report with no actions)

## Scenarios

### S1: ProposalTagger tags all created beads with proposal reference

Given bead IDs `["spexmachina-101", "spexmachina-102"]` from two create actions, and proposal reference `2026-02-23-spex-machina`.

When `TagWithProposal` is called:

Then the fake receives exactly two update calls:
- `spexmachina-101` with label `spec_proposal:2026-02-23-spex-machina`
- `spexmachina-102` with label `spec_proposal:2026-02-23-spex-machina`

### S2: ProposalTagger tags closed beads alongside created beads

Given the mixed set of five bead IDs (two creates, one close, two reviews) and proposal `2026-03-01-validator-refactor`.

When `TagWithProposal` is called:

Then the fake receives exactly five tag calls, one per bead ID, all with `spec_proposal:2026-03-01-validator-refactor`. Closed beads are tagged before they are considered done — the tag provides an audit trail for why the bead was closed.

### S3: ProposalTagger strips .md extension from proposal filename

Given a proposal reference passed as `2026-02-23-spex-machina.md` (with extension).

When `TagWithProposal` is called:

Then the label value is `spec_proposal:2026-02-23-spex-machina` (extension stripped). The `br` label format does not allow dots, so the `.md` suffix is always removed before tagging.

### S4: ProposalTagger handles empty bead ID list as no-op

Given an empty bead ID list (impact report detected no changes).

When `TagWithProposal` is called:

Then no update calls are made and the returned error is nil. This covers the case where `spex apply` runs against an empty impact report.

### S5: ProposalTagger continues on individual tag failures and returns summary error

Given three bead IDs where tagging the second one fails (bead was deleted between create and tag steps).

When `TagWithProposal` is called:

Then all three tag calls are attempted. The returned error aggregates the warning from the second bead. The first and third beads are successfully tagged.

### S6: SnapshotSaver writes snapshot to correct path

Given the temporary spec directory at `tmpdir/`.

When `SaveSnapshot(ctx, "tmpdir/", fixedTime)` is called:

Then a file exists at `tmpdir/.snapshot.json`. The file contains valid JSON. Parsing the JSON yields a merkle snapshot with a `created_at` field matching `2026-01-15T10:00:00Z`.

### S7: SnapshotSaver snapshot reflects current spec content

Given the temporary spec directory with known content in `arch_widget.md`.

When `SaveSnapshot` is called:

Then the snapshot's merkle tree contains a hash entry for `testmod/arch_widget.md`. Modifying the content of `arch_widget.md` and calling `SaveSnapshot` again produces a different root hash. This confirms the snapshot captures actual file content, not just file existence.

### S8: SnapshotSaver overwrites previous snapshot

Given the temporary spec directory with an existing `.snapshot.json` from a prior run.

When `SaveSnapshot` is called:

Then the `.snapshot.json` file is overwritten (not appended to, not placed alongside with a different name). Only one snapshot file exists after the call. The new snapshot's root hash differs from the old one if spec content changed.

### S9: SnapshotSaver uses deterministic timestamp

Given two calls to `SaveSnapshot` with the same `createdAt` value and unchanged spec content.

When both calls complete:

Then both produce byte-identical `.snapshot.json` files. This confirms determinism: same input + same timestamp = same output, which is critical for the merkle diff to function correctly.

### S10: Full tagging-then-snapshot sequence

Given a complete apply cycle: BeadCreator returns `["spexmachina-201"]`, BeadUpdater processes one review for `spexmachina-77`, BeadCloser processes one close for `spexmachina-42`, all succeed.

When ProposalTagger runs with all three bead IDs and proposal `2026-02-23-spex-machina`, then SnapshotSaver runs:

Then:
1. Three tag calls are recorded in the fake, all with the same proposal reference.
2. `.snapshot.json` exists and is valid.
3. The execution order is: all tags complete before snapshot save begins (per the flow spec).

## Edge Cases

### E1: SnapshotSaver fails on invalid spec directory

Given a `specDir` pointing to a nonexistent path `/tmp/no-such-dir`.

When `SaveSnapshot` is called:

Then the error message starts with `"apply: build tree for snapshot:"` and wraps the underlying file system error. No `.snapshot.json` is written.

### E2: SnapshotSaver fails on malformed module.json

Given a spec directory where `testmod/module.json` contains invalid JSON.

When `SaveSnapshot` is called:

Then the error propagates from `merkle.BuildTree`. No snapshot is written. The next `spex apply` will retry, giving the user a chance to fix the malformed JSON before the snapshot baseline advances.

### E3: ProposalTagger with proposal reference containing special characters

Given a proposal reference `2026-03-01-add-test_plan-v2` (underscores and hyphens, no dots).

When `TagWithProposal` is called:

Then the label is `spec_proposal:2026-03-01-add-test_plan-v2` (passed through unchanged since it contains no dots). Only `.md` is stripped; other characters valid in `br` labels are preserved.

### E4: Snapshot not saved when preceding tagging step fails fatally

Given a scenario where `TagWithProposal` returns a fatal error (e.g., bead CLI binary disappeared mid-run, not a per-bead warning).

When the apply flow checks the error:

Then `SaveSnapshot` is never called. The old snapshot remains, so the next `spex apply` will recompute the same impact and retry all actions including tagging. This is the retry guarantee from the flow spec.

### E5: ProposalTagger called with duplicate bead IDs

Given bead IDs `["spexmachina-101", "spexmachina-101"]` (same bead appeared in both create and review lists due to a spec node being added and immediately modified in the same proposal).

When `TagWithProposal` is called:

Then two tag calls are made for the same bead ID. The second call is idempotent (same label value overwrites the same label). No error is raised. Deduplication is not required by the spec — the CLI handles it gracefully.

### E6: SnapshotSaver with read-only spec directory

Given a spec directory where `.snapshot.json` cannot be written (permissions issue).

When `SaveSnapshot` is called:

Then the error message starts with `"apply: save snapshot:"` and wraps the underlying permission error. The apply run fails, and the user sees a clear message about the write failure. Since no snapshot was saved, the next run will retry.
