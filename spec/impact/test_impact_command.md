# Impact Command Tests

Integration and acceptance tests for ImpactCommand (component 5). These tests verify the CLI entry point `spex impact`, which reads a merkle diff, wires all internal components (BeadReader, NodeMatcher, ActionClassifier, ReportGenerator), and outputs the impact report as JSON to stdout.

## Setup

Tests use a temporary directory containing:

1. **A spec tree** with `project.json` and two module directories (`validator/`, `merkle/`), each with `module.json` and content leaf files.

2. **A snapshot file** representing the previous state, so that `spex diff` can produce a meaningful diff. The snapshot is pre-computed with known hashes.

3. **A diff file** (or piped stdin) containing the merkle diff output. The diff fixture represents:
   - `validator/arch_schema_checker.md` modified (impact: arch_impl)
   - `validator/arch_orphan_detector.md` added (impact: arch_impl)
   - `merkle/impl_hash_computation.md` modified (impact: impl_only)
   - `merkle/arch_diff_engine.md` removed (impact: arch_impl)

4. **A mock bead CLI** — a shell script or test binary at a known path that outputs the fixture bead JSON when called with `list --json`. This avoids depending on a real `br` or `bd` installation.

```bash
#!/bin/sh
# mock_br: returns fixture bead data
if [ "$1" = "list" ] && [ "$2" = "--json" ]; then
  cat <<'BEADS'
[
  {"id":"spex-001","title":"Implement SchemaChecker","labels":["spec_module:validator","spec_component:SchemaChecker","spec_hash:abc123"]},
  {"id":"spex-003","title":"Implement hash computation","labels":["spec_module:merkle","spec_impl_section:Hash computation","spec_hash:ghi789"]},
  {"id":"spex-010","title":"Implement LegacyHasher","labels":["spec_module:merkle","spec_component:LegacyHasher","spec_hash:zzz000"]}
]
BEADS
  exit 0
fi
exit 1
```

The mock binary is placed on PATH or referenced via the `--bead-cli` flag.

## Scenarios

### S1: Full pipeline — diff file to JSON report on stdout

Run:
```
spex impact --diff diff.json --bead-cli ./mock_br
```

Capture stdout. Parse the output as JSON. Assert the report contains:
- **creates**: 1 entry for `validator/OrphanDetector` (added node, no matching bead)
- **closes**: 1 entry for bead `spex-010` / `merkle/LegacyHasher` (bead references a component in merkle, and `arch_diff_engine.md` was removed — but LegacyHasher is the orphaned bead whose spec node was removed)
- **reviews**: 2 entries — `spex-001` for `validator/SchemaChecker` (modified) and `spex-003` for `merkle/Hash computation` (modified)
- **summary**: `create_count: 1, close_count: 1, review_count: 2`

Assert exit code is 0.

### S2: Diff input from stdin (pipe)

Run:
```
cat diff.json | spex impact --bead-cli ./mock_br
```

Assert the output is identical to S1. The command must accept diff input on stdin when `--diff` is not specified.

### S3: Diff input from stdin with --diff flag set to "-"

Run:
```
cat diff.json | spex impact --diff - --bead-cli ./mock_br
```

Assert the output is identical to S1. The `-` convention for stdin must be supported.

### S4: No changes — empty diff produces empty report

Provide an empty diff (empty JSON array `[]`). Run:
```
spex impact --diff empty_diff.json --bead-cli ./mock_br
```

Assert stdout contains a valid JSON report with empty arrays and zero counts. Assert exit code is 0. An empty diff is not an error condition.

### S5: --json flag (explicit JSON output)

Run:
```
spex impact --diff diff.json --bead-cli ./mock_br --json
```

Assert the output is valid JSON. The `--json` flag should be the default behavior but must be accepted without error for explicitness and forward compatibility with potential non-JSON output formats.

### S6: Pipeline composition — spex diff piped into spex impact

This acceptance test validates the full pipeline integration:

```
spex diff --snapshot snapshot.json --spec-dir ./spec | spex impact --bead-cli ./mock_br
```

Assert the composed pipeline produces a valid impact report. This tests that `spex diff` stdout format is exactly the format `spex impact` expects on stdin — no format adapter needed.

### S7: Exit code 0 on success, exit code 1 on error

Run `spex impact` with valid inputs. Assert exit code 0.

Run `spex impact --diff nonexistent_file.json --bead-cli ./mock_br`. Assert exit code 1 and stderr contains an error message about the missing file.

### S8: Bead CLI binary selection via --bead-cli flag

Run with `--bead-cli ./mock_br` (custom path). Assert the command uses the specified binary. Then run with `--bead-cli bd` to confirm it accepts alternative bead CLI names.

### S9: Default bead CLI is "br"

Run `spex impact --diff diff.json` without the `--bead-cli` flag, with a mock `br` binary on PATH. Assert the command invokes `br list --json`. This confirms the default value.

### S10: Deterministic output across runs

Run `spex impact --diff diff.json --bead-cli ./mock_br` five times. Capture stdout each time. Assert all five outputs are byte-for-byte identical. This validates the determinism requirement: same merkle diff + same bead state always produces the same impact report.

### S11: Report output is suitable for piping to spex apply

Run:
```
spex impact --diff diff.json --bead-cli ./mock_br > report.json
```

Then verify `report.json` can be parsed as an `ImpactReport` struct and passed to `spex apply`. Specifically:
- The file contains only the JSON report (no log lines, no progress output mixed in)
- Diagnostic or error messages go to stderr, not stdout
- The JSON is terminated with a newline

### S12: Large diff with many changes

Generate a diff with 500 changed nodes across 20 modules. Provide a bead fixture with 300 beads. Run `spex impact` and assert:
- The command completes in under 5 seconds
- The report is valid JSON
- Summary counts match the expected values based on the generated data
- Exit code is 0

## Edge Cases

### E1: Bead CLI not found

Run `spex impact --diff diff.json --bead-cli nonexistent_binary`. Assert:
- Exit code is 1
- stderr contains `"impact: read beads:"` error context
- stdout is empty (no partial report)

### E2: Bead CLI returns non-zero exit code

Mock bead CLI exits with code 1 and writes `"database locked"` to stderr. Assert:
- `spex impact` exits with code 1
- stderr includes the bead CLI's error message in the wrapped error

### E3: Bead CLI returns malformed JSON

Mock bead CLI outputs `{"broken":`. Assert:
- Exit code is 1
- Error message references JSON parsing

### E4: Diff file contains malformed JSON

Provide a diff file containing `[{"path": "foo"` (truncated). Assert:
- Exit code is 1
- Error message references diff parsing

### E5: Diff file with zero-length content

Provide a diff file that is completely empty (0 bytes, not `[]`). Assert:
- Exit code is 1 (empty file is not valid JSON)
- Error message indicates the diff input could not be parsed

### E6: Concurrent access safety

If `spex impact` is run twice simultaneously against the same spec directory and bead CLI, both invocations must complete without corrupting each other's output. This is naturally satisfied because `spex impact` is read-only — it does not write to the spec directory or bead database. Assert both processes exit 0 and produce identical reports.

### E7: Spec directory with no modules

Provide a diff referencing modules that do not exist in the spec tree. Assert that unmatched changes are reported (creates for added nodes) and no panics occur due to missing module.json files.

### E8: Bead CLI returns beads with duplicate IDs

Mock bead CLI returns two beads with the same ID but different spec labels. Assert the command handles this gracefully — both entries are processed, and if they match different spec nodes, both matches appear in the report.
