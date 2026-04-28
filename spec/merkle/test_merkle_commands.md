# Merkle Command Tests

Integration and acceptance tests for the `DiffCommand` (id `c8b958ec310d`).
Validates the CLI entry point `spex diff`, which wires together the internal
components (Hasher, TreeBuilder, SnapshotStore, DiffEngine, ImpactClassifier,
CompletenessChecker) and produces the user-facing diff and classification
output.

This test_section covers the single CLI surface the merkle module exposes.
There is no `spex hash` command — building and persisting the tree happens
inside `spex diff` (read) and `spex ingest`'s SnapshotSaver path (write); see
`flow_hash_computation.md`.

## Setup

All scenarios operate against a temporary spec directory with a complete, valid fixture:

```
tmpdir/
  project.json
  alpha/
    module.json
    arch_widget.md
    arch_gadget.md
    impl_widget_logic.md
    flow_data_path.md
  beta/
    module.json
    arch_service.md
    impl_handler.md
```

Tests invoke the command programmatically (calling the command's `Run` function
with args, capturing stdout and stderr) rather than spawning a subprocess. This
enables reliable exit code checking and output assertion without PATH
dependencies.

Helper `runDiff(args ...string) (stdout, stderr string, exitCode int)` wraps
the command execution.

## Scenarios

### S1: `spex diff` on fresh project (no snapshot) reports every leaf as added

**Given** the full fixture directory with no existing `spec/.snapshot.json`
**When** `runDiff(tmpdir)` is called
**Then** exit code is 0
**And** stdout lists every spec leaf (component, impl_section, data_flow,
test_section, requirement, meta) as `added`
**And** every change carries an impact classification consistent with its node type

**Rationale**: This is the bootstrap entry point — the first diff after `spex
validate` on a fresh project. `SnapshotStore.Load` returns the empty tree when
the snapshot file is absent (per the contract in `flow_hash_computation.md`),
so the diff reports the entire spec as new. This drives the first
impact → emit → adapter → ingest cycle that creates the initial bead-map and
writes the first snapshot. Replaces what an earlier draft of the pipeline
attempted with a separate `spex hash` step. Exercises Hasher, TreeBuilder,
SnapshotStore, DiffEngine, and ImpactClassifier end-to-end.

### S2: `spex diff --json` outputs structured JSON with full snapshot view

**Given** the full fixture directory with no existing snapshot
**When** `runDiff(tmpdir, "--json")` is called
**Then** exit code is 0
**And** stdout is valid JSON
**And** the JSON is an object with at least a `changes` array (one entry per
leaf), each entry carrying `id`, `type`, `impact`, and `module` fields
**And** the structure is suitable for piping to `spex impact` or `jq`

**Rationale**: Validates the `--json` flag for machine-readable output, which
is the contract `spex impact` consumes. Also covers the bootstrap-with-JSON
scenario — the same flag works whether the diff is empty, fully added, or
mixed.

### S3: `spex diff` after impl-only change reports a single modified leaf

**Given** the fixture directory with a snapshot from a previous complete
ingest run
**When** `alpha/impl_widget_logic.md` is modified (content appended)
**And** `runDiff(tmpdir)` is called
**Then** exit code is 0
**And** stdout reports `alpha/impl_widget_logic.md` (or its identity hash) as
`modified` with impact `impl_only`
**And** no other leaves are listed as changed
**And** the surrounding interior hashes (`alpha/impl`, `alpha`, root) update
internally but are not surfaced as user-facing changes

**Rationale**: End-to-end test of the diff pipeline for the most common change
type. Validates that TreeBuilder rebuilds the current tree, SnapshotStore
loads the previous snapshot, DiffEngine finds the single changed leaf, and
ImpactClassifier labels it correctly.

### S4: `spex diff` after arch change

**Given** the fixture directory with a snapshot
**When** `alpha/arch_widget.md` is modified
**And** `runDiff(tmpdir)` is called
**Then** stdout reports `alpha/arch_widget.md` as `modified` with impact `arch_impl`

**Rationale**: Architecture changes have a higher impact level. Validates the
filename-pattern + node-metadata classification in ImpactClassifier wired
through DiffCommand.

### S5: `spex diff` after structural change

**Given** the fixture directory with a snapshot
**When** `alpha/module.json` is modified (e.g., a new component added to the JSON)
**And** `runDiff(tmpdir)` is called
**Then** stdout reports the change as `modified` with impact `structural`

**Rationale**: Structural changes are the highest impact level and trigger the
most downstream work (new beads, updated mappings).

### S6: `spex diff --snapshot <path>` uses explicit snapshot

**Given** the fixture directory and a snapshot file saved at a custom path `/tmp/custom-snapshot.json`
**When** `runDiff(tmpdir, "--snapshot", "/tmp/custom-snapshot.json")` is called
**Then** the diff is computed against the custom snapshot, not the default `spec/.snapshot.json`

**Rationale**: The `--snapshot` flag (from `arch_diff_command.md`) allows
comparing against any previous state, not just the most recent snapshot.
Useful for comparing across branches or historical states. Also exercises
SnapshotStore.Load against a non-default path.

### S7: `spex diff` with multiple changes shows module-level aggregation

**Given** the fixture directory with a snapshot
**When** both `alpha/impl_widget_logic.md` and `alpha/arch_widget.md` are modified
**And** `runDiff(tmpdir, "--json")` is called
**Then** the JSON output lists both changes individually
**And** both changes have `module` set to alpha's identity hash
**And** the output includes a module-level summary showing alpha's aggregate
impact as `arch_impl` (the higher of the two)

**Rationale**: Validates that DiffCommand wires through the module-level
aggregation logic from ImpactClassifier, giving users both per-file and
per-module impact views.

### S8: Full bootstrap-then-steady-state cycle through diff alone

**Given** a clean fixture directory with no snapshot
**When** `runDiff(tmpdir)` is called (reports everything as added)
**And** the impact + emit + adapter + ingest cycle runs against that diff
(simulated by writing a complete-status receipts file and invoking
`spex ingest`, which causes SnapshotSaver to write the first snapshot)
**And** `alpha/impl_widget_logic.md` and `beta/arch_service.md` are modified
**And** `runDiff(tmpdir)` is called again (compares against the
ingest-written snapshot)
**And** another simulated complete ingest runs
**And** `runDiff(tmpdir)` is called once more
**Then** the first diff reports the full spec as added
**And** the second diff reports exactly two modified leaves
**And** the third diff reports zero changes (snapshot now matches current state)

**Rationale**: The complete bootstrap-and-steady-state usage cycle for the
merkle module's CLI surface. Proves that diff-then-ingest-then-diff converges
(no phantom changes after a complete ingest) and that bootstrap and
steady-state share the same component composition. There is no separate
"hash" step; ingest is the only path that writes the snapshot.

## Edge Cases

### E1: `spex diff` on invalid spec directory

**Given** a directory that does not contain `project.json`
**When** `runDiff(badDir)` is called
**Then** exit code is non-zero (1)
**And** stderr contains an error message about the missing `project.json`

### E2: `spex diff` on corrupted snapshot

**Given** a `spec/.snapshot.json` file containing invalid JSON
**When** `runDiff(tmpdir)` is called
**Then** exit code is non-zero (1)
**And** stderr reports a snapshot parse error with the file path

### E3: `spex diff` with no arguments defaults to current directory

**Given** the working directory is the fixture spec root
**When** `runDiff()` is called with no directory argument
**Then** it uses the current working directory as the spec root
**And** behaves identically to `runDiff(".")`

### E4: `spex diff` output is pipeable

**Given** the fixture directory with a snapshot
**When** `runDiff(tmpdir, "--json")` is called
**Then** stdout contains only the JSON payload, no ANSI color codes or
interactive formatting
**And** stderr is used for any progress or diagnostic messages

**Rationale**: Per the project's technical constraints, every subcommand must
be composable and pipeable. stdout is for data, stderr is for diagnostics.

### E5: `spex diff` exit code semantics

**Given** a valid spec directory with a snapshot
**When** `runDiff(tmpdir)` is called and there are changes but no
CompletenessChecker errors
**Then** exit code is 0 (changes found is not an error)
**When** `runDiff(tmpdir)` is called and there are no changes
**Then** exit code is 0
**When** `runDiff(tmpdir)` is called and the diff includes any
CompletenessChecker error (i.e. the `errors` array is non-empty)
**Then** exit code is 2
**And** the full diff (changes + errors) is still emitted on stdout
**And** the text output renders each finding under an `error(s):` heading
with each line prefixed `error:` — never `warning:`

**Rationale**: Both "changes found" and "no changes" are successful outcomes.
Non-zero exit codes are reserved for actual errors. CompletenessChecker
findings are errors (per `arch_completeness_checker.md`), and the pipeline
contract is that downstream steps (`spex impact`) refuse a diff with errors —
so the exit code MUST signal that refusal. Aligning text labels with JSON
labels (both call them errors) prevents the "looks like a warning, must be
advisory" trap that masks pipeline halts.

### E6: `spex diff --json` errors-array shape and text labels match

**Given** a valid spec directory and a snapshot, with at least one edit that
triggers a CompletenessChecker finding (e.g. a requirement leaf was modified
without its implementing component's content also changing)
**When** `runDiff(tmpdir, "--json")` is called
**Then** the JSON output has a top-level `errors` array (never `warnings`)
**And** each entry has `type`, `message`, `path`, `related` fields
**And** the corresponding text output (a separate run without `--json`)
labels the same entries with `error:`, not `warning:`

**Rationale**: Locks the terminology contract. JSON and text output are
synonyms — both call CompletenessChecker findings errors. A future
implementation that drifts back to `warning:` in text or `warnings:` in JSON
fails this test.
