# Apply Command Tests

Integration and acceptance tests for ApplyCommand (component 6). These tests verify the CLI entry point `spex apply` which orchestrates all bead actions, proposal tagging, and snapshot saving. The key property under test is idempotency: applying the same impact report twice produces no additional changes.

## Setup

### Test Fixture: End-to-End Harness

The apply command tests use a full integration harness that wires together all components with fakes:

```go
type applyHarness struct {
    cli         *fakeBeadCLI
    specDir     string          // temp dir with valid spec
    reportFile  string          // path to impact report JSON file
    proposalRef string
    stdout      *bytes.Buffer
    stderr      *bytes.Buffer
}
```

The harness constructs the `ApplyCommand` with the fake BeadCLI injected (via the `BeadCLI` interface), a temporary spec directory with a valid spec tree, and a pre-written impact report file.

### Test Fixture: Impact Report Files

Pre-serialized JSON impact report files used across scenarios:

**report_mixed.json** — two creates, one close, three reviews:
```json
{
  "creates": [
    {"type":"create","module":"validator","node":"ContentResolver","node_type":"component","impact":"arch_impl","spec_hash":"aaa111","reason":"New spec node"},
    {"type":"create","module":"merkle","node":"SnapshotFormat","node_type":"impl_section","impact":"impl_only","spec_hash":"bbb222","reason":"New spec node"}
  ],
  "closes": [
    {"type":"close","bead_id":"spexmachina-42","module":"validator","node":"LegacyChecker","reason":"Spec node removed: validator/LegacyChecker"}
  ],
  "reviews": [
    {"type":"review","bead_id":"spexmachina-77","module":"merkle","node":"Hasher","impact":"impl_only","spec_hash":"ccc333","reason":"Spec node modified"},
    {"type":"review","bead_id":"spexmachina-78","module":"merkle","node":"TreeBuilder","impact":"arch_impl","spec_hash":"ddd444","reason":"Spec node modified"},
    {"type":"review","bead_id":"spexmachina-79","module":"impact","node":"ActionClassifier","impact":"impl_only","spec_hash":"eee555","reason":"Spec node modified"}
  ],
  "summary": {"create_count":2,"close_count":1,"review_count":3}
}
```

**report_empty.json** — no changes detected:
```json
{
  "creates": [],
  "closes": [],
  "reviews": [],
  "summary": {"create_count":0,"close_count":0,"review_count":0}
}
```

**report_creates_only.json** — only new nodes, no modifications or removals.

### Test Fixture: Temporary Spec Directory

Same as `test_tagging_snapshot.md` setup, extended with a pre-existing `.snapshot.json` representing the baseline state before the impact report was generated.

## Scenarios

### S1: Apply command executes all action types in correct order

Given `report_mixed.json` as input and proposal `2026-02-23-spex-machina`.

When `spex apply --report report_mixed.json --proposal 2026-02-23-spex-machina` runs:

Then the fake BeadCLI records calls in this exact order:
1. Two `Create` calls (validator/ContentResolver, then merkle/SnapshotFormat)
2. Three `Update` calls for spec_hash changes (merkle/Hasher, merkle/TreeBuilder, impact/ActionClassifier)
3. One `Close` call (spexmachina-42)
4. Six `Update` calls for proposal tagging (one per affected bead: 2 created + 3 reviewed + 1 closed)
5. `.snapshot.json` is written after all bead actions

This order matches the flow spec: creates first, updates second, closes third, then tag all, then snapshot.

### S2: Apply command reads impact report from stdin

Given `report_mixed.json` piped via stdin (no `--report` flag).

When `cat report_mixed.json | spex apply --proposal 2026-02-23-spex-machina` runs:

Then the same actions execute as S1. The command reads the full JSON from stdin before beginning bead actions. This enables the pipeline `spex impact | spex apply --proposal <ref>`.

### S3: Apply command reads impact report from file

Given `report_mixed.json` at path `/tmp/report.json`.

When `spex apply --report /tmp/report.json --proposal 2026-02-23-spex-machina` runs:

Then the report is parsed from the file and all actions execute correctly. Both stdin and file input produce identical behavior.

### S4: Apply command handles empty report as no-op

Given `report_empty.json` as input.

When `spex apply --report report_empty.json --proposal 2026-02-23-spex-machina` runs:

Then no `Create`, `Close`, or `Update` calls are made on the fake. `TagWithProposal` is called with an empty bead ID list (no-op). `SaveSnapshot` is still called — even with no bead changes, the snapshot is updated to record that the apply completed. Exit code is 0.

### S5: Idempotency — applying same report twice produces no additional changes

Given `report_creates_only.json` with two create actions.

First run: `spex apply --report report_creates_only.json --proposal 2026-02-23-spex-machina` completes successfully. The fake's `FindExisting` returns no matches, so two beads are created.

Second run: the fake's `FindExisting` now returns the bead IDs from the first run for the matching labels.

When the second run completes:

Then zero `Create` calls are made (both beads already exist). `TagWithProposal` is called with the existing bead IDs (re-tagging is idempotent). `SaveSnapshot` writes an identical snapshot (spec content unchanged). The second run produces the same end state as the first — this is the core idempotency guarantee.

### S6: Idempotency — closed beads are not re-closed

Given `report_mixed.json` applied twice. After the first run, the close action's bead is already closed.

When the second run executes:

Then `CloseBeads` calls `Close` on bead `spexmachina-42`. The fake returns "already closed" error. This is treated as a warning, not a fatal error. The apply run completes successfully with exit code 0.

### S7: Idempotency — updated beads tolerate re-update

Given `report_mixed.json` applied twice. After the first run, the review beads already have the updated `spec_hash`.

When the second run executes:

Then `UpdateBeads` calls `Update` on all three review beads. The `--add-label` command overwrites the existing label with the same value (idempotent). No errors.

### S8: Dry-run mode prints actions without executing

Given `report_mixed.json` as input.

When `spex apply --report report_mixed.json --proposal 2026-02-23-spex-machina --dry-run` runs:

Then stdout contains a human-readable listing of planned actions:
- `create validator/ContentResolver`
- `create merkle/SnapshotFormat`
- `update spexmachina-77 spec_hash:ccc333`
- `update spexmachina-78 spec_hash:ddd444`
- `update spexmachina-79 spec_hash:eee555`
- `close spexmachina-42`
- `tag 6 beads with proposal 2026-02-23-spex-machina`
- `save snapshot`

No `Create`, `Close`, or `Update` calls are made on the fake. No `.snapshot.json` is written. Exit code is 0.

### S9: Apply command fails when bead CLI binary is not found

Given a `--bead-cli` flag pointing to a nonexistent binary `/usr/bin/no-such-bead-tool`.

When `spex apply --bead-cli /usr/bin/no-such-bead-tool --report report_mixed.json --proposal ref` runs:

Then the command exits with code 1 and stderr contains `"apply: bead CLI not found: /usr/bin/no-such-bead-tool"`. No bead actions are attempted.

### S10: Apply command aborts and preserves snapshot on create failure

Given `report_mixed.json` where the second create action fails (fake returns error).

When `spex apply` runs:

Then the first create succeeds (bead created), the second fails, and the command stops. No `Update`, `Close`, or `Tag` calls are made. `.snapshot.json` is not updated — it retains the old baseline. Exit code is 1. On the next run, the impact report will include both creates again, and the first create will be skipped by idempotency (already exists), while the second is retried.

### S11: Apply command continues through close warnings but reports them

Given `report_mixed.json` where the close action returns a warning (bead already closed).

When `spex apply` runs:

Then all creates succeed, all updates succeed, the close logs a warning to stderr, tagging succeeds, and the snapshot is saved. Exit code is 0. The warning appears in stderr output for operator visibility.

### S12: Apply command requires --proposal flag

Given a valid impact report but no `--proposal` flag.

When `spex apply --report report_mixed.json` runs:

Then the command exits with code 1 and stderr contains a usage error indicating `--proposal` is required. No bead actions are attempted. Every apply run must be traceable to a proposal.

## Edge Cases

### E1: Impact report with invalid JSON

Given a report file containing `{invalid json`.

When `spex apply --report bad.json --proposal ref` runs:

Then the command exits with code 1 and stderr contains a JSON parse error. No bead actions are attempted. No snapshot changes.

### E2: Impact report file does not exist

Given `--report /tmp/nonexistent.json`.

When `spex apply` runs:

Then the command exits with code 1 and stderr contains a file-not-found error. No bead actions are attempted.

### E3: Proposal reference points to nonexistent proposal file

Given `--proposal 2099-01-01-does-not-exist`.

When `spex apply --report report_mixed.json --proposal 2099-01-01-does-not-exist` runs:

Then the command logs a warning that `spec/proposals/2099-01-01-does-not-exist.md` does not exist, but proceeds with the apply. The proposal reference is still tagged on beads. The warning gives the user a chance to notice the typo without blocking the apply.

### E4: Impact report with creates but no bead_id on close actions (malformed)

Given a close action missing the `bead_id` field.

When `spex apply` attempts to parse the report:

Then the command exits with code 1 and stderr contains a validation error: close actions require a `bead_id`. The report is rejected before any bead actions begin.

### E5: Very large impact report (100+ actions)

Given an impact report with 50 creates, 30 reviews, and 20 closes.

When `spex apply` runs:

Then all 100 actions are processed in the correct order (50 creates, then 30 reviews, then 20 closes). All 100 beads are tagged with the proposal. The snapshot is saved. Total fake call count: 50 Create + 30 Update (spec_hash) + 20 Close + 100 Update (proposal tag) = 200 calls. Exit code 0.

### E6: Concurrent apply runs are not supported

The apply command does not implement file locking on `.snapshot.json`. If two `spex apply` processes run simultaneously against the same spec directory, the behavior is undefined — the last writer wins on the snapshot, and bead actions may be duplicated.

This is acceptable because:
1. `spex apply` is designed to be run by a single operator (human or CI) after reviewing the impact report.
2. The idempotency guarantee means a second run will converge to the correct state even if the first run's snapshot was clobbered.
3. Bead creation idempotency (FindExisting check) prevents duplicate beads.

This edge case documents the design decision rather than specifying a test — it is verified by inspection, not automation.

### E7: Apply with --bead-cli flag selects alternate binary

Given `--bead-cli bd` instead of the default `br`.

When `spex apply --bead-cli bd --report report_creates_only.json --proposal ref` runs:

Then the fake is constructed with binary name `bd`. All shelled-out commands use `bd` as the binary. The behavior is otherwise identical to using `br`. This validates that the `--bead-cli` flag is threaded through to all components.
