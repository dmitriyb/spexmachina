# Validation Pipeline Tests

Integration and acceptance test scenarios for ErrorReporter and ValidateCommand.

## Setup

These tests exercise the aggregation and orchestration layers. They use a temporary spec directory and invoke the full validation pipeline (or individual components in isolation for ErrorReporter unit scenarios). The fixture builder can inject specific errors at each checker level.

### Fixture Structure

```
tmp/spec/
  project.json                 # one module: alpha
  alpha/
    module.json                # 1 component Comp1, 1 test_section describing it
    arch_comp1.md
    test_comp1.md
```

### CLI Invocation Pattern

```
spex validate tmp/spec/
```

Output goes to stdout as JSON. Exit code is the primary assertion target for CLI-level tests.

---

## Scenarios

### ErrorReporter Scenarios

#### R1: Empty error list produces valid report

**Given** all checkers return empty error slices.
**When** `Report(errors, w, false)` is called with the aggregated (empty) slice.
**Then** it writes JSON to the writer:
```json
{
  "valid": true,
  "error_count": 0,
  "warning_count": 0,
  "errors": []
}
```

#### R2: Single error produces correct report structure

**Given** one `ValidationError` with `check: "schema"`, `severity: "error"`, `path: "project.json:name"`, `message: "required field missing"`.
**When** `Report(errors, w, false)` is called.
**Then** it writes:
- `valid: false`
- `error_count: 1`
- `warning_count: 0`
- `errors` array with exactly one entry matching the input

#### R5: Errors sorted by path

**Given** errors in arbitrary order: an entry at path `z`, an entry at path `b`, an entry at path `a`.
**When** `Report(errors, w, false)` is called.
**Then** the `errors` array is sorted `a`, `b`, `z`. Path is the only sort key.

#### R6: Errors from multiple checkers are aggregated

**Given** errors from `"schema"`, `"content"`, `"dag"`, `"id"`, `"name_consistency"`, and `"test_coverage"` checkers.
**When** `Report(errors, w, false)` is called.
**Then** all six appear in the `errors` array, each with its correct `check` field. No errors are dropped during aggregation.

#### R7: Report output is valid JSON

**Given** any combination of errors (including errors with special characters in messages like quotes, newlines, unicode).
**When** `Report(errors, w, false)` is called.
**Then** the output is parseable by `json.Unmarshal` into a `ValidationReport` struct without error.

#### R8: Compact JSON when writing to non-TTY

**Given** the writer is a `bytes.Buffer` and the caller passes `isTTY: false`.
**When** `Report(errors, w, false)` is called.
**Then** the output is compact JSON (no indentation, no trailing newlines between fields).

#### R9: Pretty-printed JSON when writing to TTY

**Given** the caller passes `isTTY: true`. The reporter does not detect the terminal itself — the flag is the only input that changes formatting, so the writer can still be a `bytes.Buffer` for assertion purposes.
**When** `Report(errors, w, true)` is called.
**Then** the output uses 2-space indentation for human readability.

---

### ValidateCommand Scenarios

#### V1: Valid spec exits 0

**Given** a fully valid spec directory with no schema errors, no missing content, no cycles, no ID issues, no name mismatches, and full test coverage.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 0. Stdout contains JSON with `valid: true` and `error_count: 0`.

#### V2: Schema error exits 1

**Given** `project.json` is missing the `name` field.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1. Stdout JSON has `valid: false`, `error_count >= 1`, and at least one error with `check: "schema"`.

#### V3: Content error exits 1

**Given** a component references a content file that does not exist.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `"content"` check error in the output.

#### V4: DAG cycle exits 1

**Given** modules alpha and beta form a circular dependency.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `"dag"` check error.

#### V6: ID duplication exits 1

**Given** alpha has two requirements with the same ID.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with an `"id"` check error.

#### V7: Name mismatch exits 1

**Given** `project.json` name for alpha is `"alpha"` but `alpha/module.json` has `name: "Alpha"`.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `"name_consistency"` check error.

#### V8: Multiple checker errors all reported

**Given** a spec with a schema error in project.json, a missing content file in alpha, AND a cycle in the module dependency graph.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1. The output JSON contains errors from at least three different checkers (`schema`, `content`, `dag`). All checkers run regardless of earlier failures.

#### V9: Checkers run in defined order

**Given** a spec with errors in every checker.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** every checker has contributed to the output. The command runs them in the fixed order `flow_validation_pipeline.md` numbers — schema, content, link, id, id_derivation, dag, name_consistency, test_coverage, requirement_coverage, coupled_section — and the aggregated report is then sorted by path, so entries from different checkers interleave by location. Where one path collects several, assert the set and not the sequence: a `module.json` that will not parse reports once per checker, all ten at that one path, carrying the ten `check` values listed above. Do not assert the order of those ten — the sort compares `path` alone, and tied entries come back in whatever order it produced.

#### V10: Default spec directory

**Given** the current working directory contains a `spec/` subdirectory with a valid spec.
**When** `spex validate` is executed with no arguments.
**Then** it defaults to `spec/` and exit code is 0.

#### V11: Explicit directory via --spec-dir flag

**Given** a valid spec at `/tmp/myspec/`.
**When** `spex validate --spec-dir /tmp/myspec/` is executed.
**Then** it validates that directory and exits 0.

#### V12: Non-existent directory

**Given** the path `/tmp/nonexistent/` does not exist.
**When** `spex validate /tmp/nonexistent/` is executed.
**Then** exit code is 1 with an error indicating the directory or `project.json` was not found. The output is still valid JSON.

#### V13: Self-validation passes

**Given** the spex-machina repo's own `spec/` directory.
**When** `spex validate spec/` is executed from the repo root.
**Then** exit code is 0. This validates requirement 8 (self-validate): the tool can validate its own spec.

#### V14: Piped output is compact JSON

**Given** a valid spec.
**When** `spex validate tmp/spec/ | cat` is executed (stdout piped to cat, not a TTY).
**Then** the output is compact JSON (single line, no indentation).

#### V15: Test coverage error exits 1

**Given** alpha has a component with no test_section coverage and no other errors.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `"test_coverage"` check error identifying the uncovered component.

---

## Edge Cases

### E1: Empty spec directory (no project.json)

**Given** an empty directory with no files.
**When** `spex validate tmp/empty/` is executed.
**Then** exit code is 1 with a structured error about missing `project.json`. Not a panic or unhandled exception.

### E2: project.json with zero modules

**Given** `project.json` has `modules: []`.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 0. A project with no modules is structurally valid (no nodes to check). Schema validation determines whether empty modules is allowed.

### E3: Concurrent-safe error aggregation

**Given** a future implementation that runs checkers in parallel.
**When** multiple checkers append errors concurrently to a shared slice.
**Then** all errors appear in the final report without data races. The current sequential implementation avoids this, but the ErrorReporter interface must not preclude parallelism.

### E4: Very large error count

**Given** a spec with 500 validation errors across all checkers.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** all 500 errors appear in the JSON output. No truncation or "and N more..." unless explicitly designed.

### E5: Validate command resolves relative paths

**Given** `spex validate ./spec/` is called from the repo root.
**When** the command runs.
**Then** it resolves `./spec/` to an absolute path before passing to checkers. All error paths in the output are relative to the spec directory (e.g., `alpha/module.json`), not absolute filesystem paths.

### E6: Checker produces error with special characters

**Given** a module name containing a quote character (e.g., `name: "it's"`).
**When** NameConsistencyChecker detects a mismatch and ErrorReporter serializes it.
**Then** the JSON output correctly escapes the quote in the message field. The output remains valid JSON.

### E7: Performance budget for full pipeline

**Given** a spec with 100 modules, 10 requirements per module, 5 components per module and 5 test_sections per module — 2000 module-scoped nodes plus the 100 module declarations themselves.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** the full pipeline completes in under 1 second (requirement 7: fast validation). Each checker operates in linear or near-linear time relative to node count.
