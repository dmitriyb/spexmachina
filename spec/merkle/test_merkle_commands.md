# Merkle Command Tests

Integration and acceptance tests for the HashCommand (component 6) and DiffCommand (component 7). Validates the CLI entry points `spex hash` and `spex diff` which wire together the internal components (Hasher, TreeBuilder, SnapshotStore, DiffEngine, ImpactClassifier) and produce user-facing output.

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

Tests invoke the commands programmatically (calling the command's `Run` function with args, capturing stdout and stderr) rather than spawning a subprocess. This enables reliable exit code checking and output assertion without PATH dependencies.

Helper `runHash(args ...string) (stdout, stderr string, exitCode int)` and `runDiff(args ...string) (stdout, stderr string, exitCode int)` wrap the command execution.

## Scenarios

### S1: `spex hash` computes tree and writes snapshot

**Given** the full fixture directory with no existing snapshot
**When** `runHash(tmpdir)` is called
**Then** exit code is 0
**And** `spec/.snapshot.json` exists in the fixture directory
**And** the snapshot file is valid JSON containing a `root_hash` field
**And** stdout contains the root hash (64 hex characters)

**Rationale**: The primary acceptance test for `spex hash` — the happy path described in `impl_hash_command.md`. Proves that Hasher, TreeBuilder, and SnapshotStore are wired correctly.

### S2: `spex hash --json` outputs structured JSON

**Given** the full fixture directory
**When** `runHash(tmpdir, "--json")` is called
**Then** exit code is 0
**And** stdout is valid JSON
**And** the JSON contains `root_hash` as a string field
**And** the JSON contains a `nodes` object with entries for each file and interior node

**Rationale**: Validates the `--json` flag for machine-readable output, enabling piping to other tools (e.g., `spex hash --json | jq .root_hash`).

### S3: `spex hash` is idempotent

**Given** the fixture directory
**When** `runHash(tmpdir)` is called twice without modifying any files
**Then** both runs produce the same root hash on stdout
**And** the snapshot file content is identical after both runs (same hashes, same structure)

**Rationale**: Determinism guarantee — hashing the same spec twice must produce the same result. The `created_at` timestamp will differ, but all hashes and node structures must be identical.

### S4: `spex hash` updates snapshot after file modification

**Given** the fixture directory with an existing snapshot from a previous `spex hash` run
**When** `alpha/impl_widget_logic.md` is modified (content appended)
**And** `runHash(tmpdir)` is called again
**Then** the new snapshot has a different root hash than the previous one
**And** the `alpha/impl_widget_logic.md` node hash in the snapshot has changed
**And** the `alpha/impl` interior hash has changed
**And** the `alpha` module hash has changed
**And** unchanged nodes (`beta/*`) retain their original hashes

**Rationale**: Validates that re-running `spex hash` correctly detects changes and propagates them through the tree. This is the setup step for a subsequent `spex diff`.

### S5: `spex diff` with no previous snapshot reports all as added

**Given** the fixture directory with no `spec/.snapshot.json`
**When** `runDiff(tmpdir)` is called
**Then** exit code is 0
**And** stdout lists every leaf file as `added`
**And** each added entry includes the file path

**Rationale**: Per `impl_diff_algorithm.md`, the first diff (no snapshot) treats everything as new. This is the baseline case described in the DiffEngine spec.

### S6: `spex diff` after impl-only change

**Given** the fixture directory with a snapshot from `spex hash`
**When** `alpha/impl_widget_logic.md` is modified
**And** `runDiff(tmpdir)` is called
**Then** exit code is 0
**And** stdout reports `alpha/impl_widget_logic.md` as `modified` with impact `impl_only`
**And** no other files are listed as changed

**Rationale**: End-to-end test of the diff pipeline for the most common change type. Validates that TreeBuilder computes a new tree, DiffEngine finds the single changed leaf, and ImpactClassifier labels it correctly.

### S7: `spex diff` after arch change

**Given** the fixture directory with a snapshot
**When** `alpha/arch_widget.md` is modified
**And** `runDiff(tmpdir)` is called
**Then** stdout reports `alpha/arch_widget.md` as `modified` with impact `arch_impl`

**Rationale**: Architecture changes have a higher impact level. Validates the filename pattern matching in ImpactClassifier wired through DiffCommand.

### S8: `spex diff` after structural change

**Given** the fixture directory with a snapshot
**When** `alpha/module.json` is modified (e.g., a new component added to the JSON)
**And** `runDiff(tmpdir)` is called
**Then** stdout reports `alpha/module.json` as `modified` with impact `structural`

**Rationale**: Structural changes are the highest impact level and trigger the most downstream work (new beads, updated mappings).

### S9: `spex diff --json` outputs structured JSON

**Given** the fixture directory with a snapshot and one modified file
**When** `runDiff(tmpdir, "--json")` is called
**Then** exit code is 0
**And** stdout is valid JSON
**And** the JSON is an array of change objects, each with fields: `path`, `type`, `impact`, `module`

**Rationale**: Machine-readable diff output for piping into the Impact module or other automation. This is the composability contract from the project's technical constraints.

### S10: `spex diff --snapshot <path>` uses explicit snapshot

**Given** the fixture directory and a snapshot file saved at a custom path `/tmp/custom-snapshot.json`
**When** `runDiff(tmpdir, "--snapshot", "/tmp/custom-snapshot.json")` is called
**Then** the diff is computed against the custom snapshot, not the default `spec/.snapshot.json`

**Rationale**: The `--snapshot` flag (from `arch_diff_command.md`) allows comparing against any previous state, not just the most recent snapshot. Useful for comparing across branches or historical states.

### S11: `spex diff` with multiple changes shows module-level aggregation

**Given** the fixture directory with a snapshot
**When** both `alpha/impl_widget_logic.md` and `alpha/arch_widget.md` are modified
**And** `runDiff(tmpdir, "--json")` is called
**Then** the JSON output lists both changes individually
**And** both changes have Module=`alpha`
**And** the output includes a module-level summary showing alpha's aggregate impact as `arch_impl`

**Rationale**: Validates that DiffCommand wires through the module-level aggregation logic from ImpactClassifier, giving users both per-file and per-module impact views.

### S12: Full cycle — hash, modify, diff, hash again

**Given** a clean fixture directory
**When** `runHash(tmpdir)` is called (creates initial snapshot)
**And** `alpha/impl_widget_logic.md` and `beta/arch_service.md` are modified
**And** `runDiff(tmpdir)` is called (reports changes against initial snapshot)
**And** `runHash(tmpdir)` is called again (updates snapshot to current state)
**And** `runDiff(tmpdir)` is called again (compares against updated snapshot)
**Then** the first diff reports 2 changes (one impl_only in alpha, one arch_impl in beta)
**And** the second diff reports 0 changes (snapshot now matches current state)

**Rationale**: The complete usage cycle for the merkle module. This proves that hash-then-diff-then-hash advances the baseline correctly and that the system converges (no phantom changes after re-snapshotting).

## Edge Cases

### E1: `spex hash` on invalid spec directory

**Given** a directory that does not contain `project.json`
**When** `runHash(badDir)` is called
**Then** exit code is non-zero (1)
**And** stderr contains an error message about the missing `project.json`

### E2: `spex diff` on invalid spec directory

**Given** a directory with no valid spec structure
**When** `runDiff(badDir)` is called
**Then** exit code is non-zero (1)
**And** stderr contains a descriptive error message

### E3: `spex hash` with no arguments defaults to current directory

**Given** the working directory is the fixture spec root
**When** `runHash()` is called with no directory argument
**Then** it uses the current working directory as the spec root
**And** behaves identically to `runHash(".")`

### E4: `spex diff` on corrupted snapshot

**Given** a `spec/.snapshot.json` file containing invalid JSON
**When** `runDiff(tmpdir)` is called
**Then** exit code is non-zero (1)
**And** stderr reports a snapshot parse error with the file path

### E5: `spex hash` output is pipeable

**Given** the fixture directory
**When** `runHash(tmpdir)` is called
**Then** stdout contains only the root hash (and optionally a tree summary), no ANSI color codes or interactive formatting
**And** stderr is used for any progress or diagnostic messages

**Rationale**: Per the project's technical constraints, every subcommand must be composable and pipeable. stdout is for data, stderr is for diagnostics.

### E6: `spex diff` exit code semantics

**Given** a valid spec directory with a snapshot
**When** `runDiff(tmpdir)` is called and there are changes
**Then** exit code is 0 (changes found is not an error)
**When** `runDiff(tmpdir)` is called and there are no changes
**Then** exit code is 0

**Rationale**: Both "changes found" and "no changes" are successful outcomes. Non-zero exit codes are reserved for actual errors (missing files, parse failures). This follows Unix convention for diff-like tools where the exit code signals error status, not change presence.
