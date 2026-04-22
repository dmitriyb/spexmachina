# Apply Command Tests

Integration and acceptance tests for ApplyCommand (component 6). These tests verify the CLI entry point `spex apply` which orchestrates all bead actions, proposal tagging, and snapshot saving. Key properties under test: idempotency (applying the same impact report twice produces no additional changes) and creation ordering (epics before features before tasks).

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

**report_mixed.json** — two creates, three obsoletes:
```json
{
  "creates": [
    {"type":"create","module":"validator","node":"ContentResolver","node_type":"component","spec_hash":"aaa111","reason":"Spec node modified (new)"},
    {"type":"create","module":"merkle","node":"SnapshotFormat","node_type":"component","spec_hash":"bbb222","reason":"New spec node"}
  ],
  "obsoletes": [
    {"type":"obsolete","bead_id":"spexmachina-77","module":"merkle","node":"Hasher","reason":"Spec node modified: merkle/Hasher"},
    {"type":"obsolete","bead_id":"spexmachina-78","module":"merkle","node":"TreeBuilder","reason":"Spec node modified: merkle/TreeBuilder"},
    {"type":"obsolete","bead_id":"spexmachina-42","module":"validator","node":"LegacyChecker","reason":"Spec node removed: validator/LegacyChecker"}
  ],
  "summary": {"create_count":2,"obsolete_count":3}
}
```

**report_empty.json** — no changes detected:
```json
{
  "creates": [],
  "obsoletes": [],
  "summary": {"create_count":0,"obsolete_count":0}
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
1. Three `Update` calls adding `spex:obsolete` + `commit:<HEAD>` labels (label phase — beads stay open)
2. Two `Create` calls with correct types, `--parent`, and `--deps blocks` where applicable
3. Three `Close` calls on the obsoleted beads (close phase — replacements exist)
4. `.snapshot.json` is written after all bead actions

This order matches the flow spec: label obsoletes, then creates in hierarchy order, then close obsoletes, then snapshot.

### S2: Apply command enforces creation ordering (epics before features before tasks)

Given a report with creates for a module (epic), two components (features), and a test_section (task).

When `spex apply` runs:

Then `Create` calls occur in this order:
1. Module (epic) first
2. Component features second (parent set to the newly created epic)
3. Test_section task last (parent set to the relevant component feature)

### S3: Apply command reads impact report from stdin

Given `report_mixed.json` piped via stdin (no `--report` flag).

When `cat report_mixed.json | spex apply --proposal 2026-02-23-spex-machina` runs:

Then the same actions execute as S1. This enables the pipeline `spex impact | spex apply --proposal <ref>`.

### S4: Apply command reads impact report from file

Given `report_mixed.json` at path `/tmp/report.json`.

When `spex apply --report /tmp/report.json --proposal 2026-02-23-spex-machina` runs:

Then the report is parsed from the file and all actions execute correctly.

### S5: Apply command handles empty report as no-op

Given `report_empty.json` as input.

When `spex apply --report report_empty.json --proposal 2026-02-23-spex-machina` runs:

Then no `Create` or `Close` calls are made on the fake. `SaveSnapshot` is still called — even with no bead changes, the snapshot is updated. Exit code is 0.

### S6: Idempotency — applying same report twice produces no additional changes

Given `report_creates_only.json` with two create actions.

First run: two beads are created. Second run: the fake's `FindExisting` now returns the bead IDs from the first run.

When the second run completes:

Then zero `Create` calls are made. The second run produces the same end state as the first.

### S7: Idempotency — obsoleted beads are not re-closed

Given `report_mixed.json` applied twice. After the first run, the obsolete actions' beads are already closed.

When the second run executes:

Then `CloseBeads` calls `Close` on the beads. The fake returns "already closed" error. This is treated as a warning, not a fatal error. Exit code is 0.

### S8: Dry-run mode prints actions without executing

Given `report_mixed.json` as input.

When `spex apply --report report_mixed.json --proposal 2026-02-23-spex-machina --dry-run` runs:

Then stdout contains a human-readable listing of planned actions:
- `label spexmachina-77 (spex:obsolete, commit:<HEAD>)`
- `label spexmachina-78 (spex:obsolete, commit:<HEAD>)`
- `label spexmachina-42 (spex:obsolete, commit:<HEAD>)`
- `create validator/ContentResolver --type feature`
- `create merkle/SnapshotFormat --type feature`
- `close spexmachina-77`
- `close spexmachina-78`
- `close spexmachina-42`
- `save snapshot`

No `Create` or `Close` calls are made on the fake. Exit code is 0.

### S9: Apply command fails when bead CLI binary is not found

Given a `--bead-cli` flag pointing to a nonexistent binary `/usr/bin/no-such-bead-tool`.

When `spex apply --bead-cli /usr/bin/no-such-bead-tool --report report_mixed.json --proposal ref` runs:

Then the command exits with code 1 and stderr contains `"apply: bead CLI not found: /usr/bin/no-such-bead-tool"`.

### S10: Apply command aborts and preserves snapshot on create failure

Given `report_mixed.json` where the second create action fails (fake returns error).

When `spex apply` runs:

Then the obsoletes succeed, the first create succeeds, the second fails, and the command stops. `.snapshot.json` is not updated. Exit code is 1.

### S11: Apply command continues through obsolete warnings but reports them

Given `report_mixed.json` where one obsolete action returns a warning (bead already closed).

When `spex apply` runs:

Then all obsoletes are attempted, creates succeed, tagging succeeds, snapshot is saved. Exit code is 0. The warning appears in stderr.

### S12: Apply command requires --proposal flag

Given a valid impact report but no `--proposal` flag.

When `spex apply --report report_mixed.json` runs:

Then the command exits with code 1 and stderr contains a usage error indicating `--proposal` is required.

## Edge Cases

### E1: Impact report with invalid JSON

Given a report file containing `{invalid json`.

When `spex apply --report bad.json --proposal ref` runs:

Then the command exits with code 1 and stderr contains a JSON parse error.

### E2: Impact report file does not exist

Given `--report /tmp/nonexistent.json`.

When `spex apply` runs:

Then the command exits with code 1 and stderr contains a file-not-found error.

### E3: Proposal reference points to nonexistent proposal file

Given `--proposal 2099-01-01-does-not-exist`.

When `spex apply --report report_mixed.json --proposal 2099-01-01-does-not-exist` runs:

Then the command logs a warning but proceeds with the apply. The proposal reference is still tagged on beads.

### E4: Very large impact report (100+ actions)

Given an impact report with 50 creates and 50 obsoletes.

When `spex apply` runs:

Then all 100 actions are processed in the correct order (label obsoletes, then creates in hierarchy order, then close obsoletes). All 100 beads are tagged with the proposal. Exit code 0.

### E5: Concurrent apply runs are not supported

The apply command does not implement file locking on `.snapshot.json`. If two `spex apply` processes run simultaneously, the behavior is undefined. The idempotency guarantee means a second run will converge to the correct state.

### E6: Apply with --bead-cli flag selects alternate binary

Given `--bead-cli bd` instead of the default `br`.

When `spex apply --bead-cli bd --report report_creates_only.json --proposal ref` runs:

Then the fake is constructed with binary name `bd`. All commands use `bd`. This validates the `--bead-cli` flag threading.

## Topological Ordering Scenarios

These scenarios test that ApplyCommand performs topological ordering within each type level when spec-graph dependencies exist between beads being created in the same run (requirement 11).

### T1: Topological ordering within feature level based on DepBeadIDs

Given a report with three component create actions:
- Component A in module M (no DepBeadIDs)
- Component B in module M (DepBeadIDs references the bead that will be created for A)
- Component C in module M (DepBeadIDs references the bead that will be created for B)

When `spex apply` runs:

Then `Create` calls for features occur in order: A, B, C. A is created first because B depends on it, and B before C.

### T2: Topological ordering does not affect cross-type ordering

Given a report with:
- One module create (epic)
- Two component creates where component X depends on component Y (both features)
- One test_section create (task)

When `spex apply` runs:

Then the overall order is: epic first, then features (Y before X due to topological sort), then task last. Type-level ordering (epic→feature→task) is preserved; topological sort only reorders within the feature level.

### T3: Independent beads within a type level maintain stable order

Given three component create actions with no DepBeadIDs (no dependencies between them).

When `spex apply` runs:

Then the three features are created in their original order from the impact report. Topological sort is stable — when no dependency constraints exist, input order is preserved.

### T4: Circular dependency within a type level is detected

Given two component create actions where A's DepBeadIDs references B and B's DepBeadIDs references A (circular — should not happen with valid spec, but must not hang).

When `spex apply` runs:

Then the command detects the cycle and exits with code 1 and an error message indicating the circular dependency. No beads are created for the cycle.

### T5: DepBeadIDs referencing already-existing beads do not affect ordering

Given two component create actions: A with `DepBeadIDs: ["spex-existing"]` (an already-created bead, not in the current create batch) and B with no dependencies.

When `spex apply` runs:

Then A and B are created in their original order. The `--deps depends:spex-existing` flag is passed for A, but since `spex-existing` is not being created in this batch, it doesn't affect topological ordering.

## End-to-End Pipeline Integration

This scenario implements `spec/project.json` test_plan scenario 2 ("Cross-module mapping integration") as a single end-to-end test that runs the full pipeline (`diff` → `impact` → `apply`) against a real spec fixture. It exists because the unit-level apply tests above use a mock mapping store that skips schema validation, so they cannot catch format mismatches between pipeline stages. PR #99 shipped with five such mismatches surviving CI; this test closes the gap.

### Sim project fixture

A miniature spec lives under `cmd/spex/testdata/sim/`. It is small enough to be exhaustive and large enough to exercise cross-module behavior:

```
cmd/spex/testdata/sim/
  spec/
    project.json              # 2 modules: alpha, beta (beta requires alpha)
    alpha/
      module.json             # 1 component, 1 impl_section
      arch_alpha_thing.md
      impl_alpha_detail.md
    beta/
      module.json             # 1 component that uses alpha's component
      arch_beta_thing.md
  .bead-map.json              # pre-existing records keyed by identity hash
  .snapshot.json              # baseline merkle snapshot of the initial state
```

Every `id` and cross-reference in the sim spec is an identity hash — there are no integer IDs anywhere in the fixture. The bead-map records and the snapshot keys are also identity hashes, so the fixture exercises the same format end to end.

### S13: Full pipeline runs cleanly with real mapping store

Given the sim fixture copied to a temp directory.

When the test:
1. Modifies a markdown content leaf (e.g., adds a paragraph to `arch_alpha_thing.md`)
2. Builds the merkle tree with the real `merkle` package
3. Diffs against `.snapshot.json`
4. Runs impact analysis with `mapping.NewFileStore()` (real store, not a mock — schema validation runs on every load)
5. Runs apply with a mock bead CLI but the same real `mapping.FileStore`
6. Saves the new snapshot

Then:
- The diff reports a single `modified` change whose key is the alpha component's identity hash
- The impact report's create and obsolete actions reference identity hashes in their `SpecNodeID` field — never merkle paths, never integer IDs
- The new mapping record written by apply passes `bead-map.schema.json` validation (the real `FileStore.Save` would fail if `spec_node_id` did not match `^[a-f0-9]{12}$`)
- The cross-module dependency from beta is preserved: beta's component still references alpha's component via the same identity hash after apply
- The updated snapshot reflects the new content hash for `arch_alpha_thing.md` and otherwise matches the old snapshot

### S14: Pipeline determinism

Given the same sim fixture and the same edit.

When the test runs the pipeline twice (in two separate temp directories):

Then both runs produce byte-identical impact reports and byte-identical bead-map writes. This catches any non-determinism in the order records are emitted, the order beads are created, or the way identity hashes are computed.

### S15: Real mapping store rejects format mismatches

Given a deliberately corrupted `.bead-map.json` where one record's `spec_node_id` is `"alpha/component/1"` (the old integer-based format).

When the pipeline loads the bead-map:

Then `mapping.NewFileStore()` fails with a schema validation error referencing the `spec_node_id` pattern. This is the regression guard for the specific bug class that PR #99 hit — a record in the wrong format must never load successfully, even from a mock or test fixture.

### S16: Cross-module create action carries dependency identity hash

Given the sim fixture.

When the test edits beta's component (changing `arch_beta_thing.md`) and runs the full pipeline:

Then the impact report's create action for beta's component has `DepBeadIDs` populated with the bead ID of alpha's component, resolved through the mapping store by looking up alpha's component identity hash from beta's `uses` field. The lookup is a single map access — no key translation. Apply passes `--deps depends:<alpha-bead-id>` to the bead CLI for beta's create.

### Why this lives in the apply tests

The end-to-end test is anchored in apply because apply is the last stage and the only one that touches the real mapping store on disk. Every earlier stage's output flows through it, so a format mismatch at any stage manifests as either an apply error or a downstream schema validation failure. Putting the test here keeps the assertion site close to the failure mode.
