# Emit command tests

End-to-end tests for the `spex emit` CLI subcommand — full process invocation against fixture inputs.

## Setup

- Use `testdata/pipeline/` with a synthetic spec tree, `.bead-map.json`, and impact_report.json.
- Invoke `spex emit` via the test binary harness (same pattern as `cmd/spex/impact_test.go`).

## Scenarios

### Happy path: stdin impact, stdout changeset

- `echo "$IMPACT" | spex emit --proposal 2026-04-18-decouple-spex-from-br --git-head deadbeef`
- Expected: exit 0, stdout is canonical `changeset.json` v1.

### --impact flag reads file

- `spex emit --proposal <ref> --git-head <sha> --impact testdata/impact.json`
- Expected: same output as stdin form; exit 0.

### --out flag writes file

- `spex emit --proposal <ref> --git-head <sha> --impact testdata/impact.json --out /tmp/changeset.json`
- Expected: exit 0, stdout empty, file at `/tmp/changeset.json` matches canonical output.

### Missing --git-head is an error

- `spex emit --proposal <ref> --impact testdata/impact.json`
- Expected: exit non-zero; stderr names the missing flag. No partial output written.

### Missing --proposal is an error

- Same but without --proposal. Exit non-zero, stderr names the missing flag.

### Malformed impact JSON surfaces a clear error

- Impact input with syntax error. Expected: exit non-zero, stderr carries the `decode impact:` context and the decoder's message. No changeset written.

### Impact report with diff errors refuses to proceed

- Impact report contains an `errors` array from the upstream completeness check. Expected: exit non-zero; stderr echoes the errors. (Same gate as the impact command has upstream; emit double-checks.)

### Help and --help

- `spex emit --help` exits 0 with usage text including all flags.

### Determinism across repeated invocations

- Run emit twice with identical inputs and `--git-head`. Expected: the two stdout outputs are byte-identical.

### Cycle in impact deps surfaces a structured error

- Impact fixture with an in-batch cycle. Expected: exit non-zero; error message names the cycle's spec_node_ids; no changeset file is written even if `--out` was specified.

## Fixtures

In-code fixtures, no on-disk testdata: `setupEmitFixture` in `cmd/spex/emit_test.go` writes the spec tree and `.bead-map.json` into a `t.TempDir()` and returns the impact report as a string, which most scenarios pipe on stdin; the two `--impact` scenarios write it to a file first.
