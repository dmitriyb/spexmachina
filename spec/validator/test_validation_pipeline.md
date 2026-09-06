# Validation Pipeline Tests

Integration and acceptance test scenarios for ErrorReporter and ValidateCommand, and for DAGChecker, IDValidator, CoupledSectionChecker and RequirementCoverageChecker as `spex validate` drives them (V4, V6, V9, V16, V17, V18).

## Setup

These tests exercise the aggregation and orchestration layers. They use a temporary spec directory and invoke the full validation pipeline. The fixture builder can inject specific errors at each checker level.

### Fixture Structure


```
tmp/spec/
  project.json                 # one module: alpha
  alpha/
    module.json                # 1 component Comp1, 1 test_section describing it
    arch_comp1.md
    test_comp1.md
```

### Dependency Baseline

V4's Given is written against a two-module baseline — modules `alpha` and `beta`, each declaring `requires_module` on the other, which is the cycle. It is scenario shorthand, not a description of any checked-in directory; the command test reads the `dag_module_cycle` fixture under `validator/testdata/`.

### CLI Invocation Pattern


```
spex validate --spec-dir tmp/spec/
```

Output goes to stdout as JSON. Exit code is the primary assertion target for CLI-level tests.

---

## Scenarios

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
**When** `spex validate --spec-dir /tmp/nonexistent/` is executed.
**Then** exit code is 1 with an error indicating the directory or `project.json` was not found. The output is still valid JSON.

#### V13: Self-validation passes

**Given** the spex-machina repo's own `spec/` directory.
**When** `spex validate --spec-dir spec/` is executed from the repo root.
**Then** exit code is 0. This validates requirement 8 (self-validate): the tool can validate its own spec.

#### V14: Piped output is compact JSON

**Given** a valid spec.
**When** `spex validate --spec-dir tmp/spec/ | cat` is executed (stdout piped to cat, not a TTY).
**Then** the output is compact JSON (single line, no indentation).

#### V15: Test coverage error exits 1

**Given** alpha has a component with no test_section coverage and no other errors.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `"test_coverage"` check error identifying the uncovered component.

#### V16: A declared derivation gap exits 0

**Given** an otherwise valid spec whose `project.json` carries one requirement declaring `derivation: "pending"` that no module requirement derives.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 0. Stdout JSON has `valid: true`, `error_count: 0`, `warning_count: 0`, and a `notes` array with one `pending_derivation` entry naming the requirement. Removing the `derivation` field from the same fixture flips the run to exit 1 with the `requirement_coverage` error — the pair of runs is what proves the field, and only the field, downgrades the finding.


#### V17: Coupled section errors exit 1
**Given** `project.json` declares a `sections` array with one entry of `type: "coupled"` whose `name` matches no module in `modules`, and a second entry whose `name` duplicates the first's.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1. Stdout JSON has `valid: false` and at least two errors with `check: "coupled_section"`, each located at `project.json:/sections/<index>`: one naming the absent module the first section expects, one naming the duplicated section name. A run over the same spec with the module present, its `section.schema.json` in place, the section's body (envelope fields stripped) valid against that schema, and the duplicate removed exits 0 with no `coupled_section` error.

#### V18: An unimplemented module requirement exits 1
**Given** an otherwise valid spec whose module `alpha` declares one requirement that no component's `implements` names.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1. Stdout JSON has `valid: false` and one error with `check: "requirement_coverage"` naming the requirement's id and name and stating that no component implements it. Adding `implements: [<that id>]` to one of alpha's components flips the same run to exit 0 with no `requirement_coverage` error.

---



## Edge Cases

### E1: Empty spec directory (no project.json)

**Given** an empty directory with no files.
**When** `spex validate --spec-dir tmp/empty/` is executed.
**Then** exit code is 1 with a structured error about missing `project.json`. Not a panic or unhandled exception.

### E2: project.json with zero modules

**Given** `project.json` has `modules: []`.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** exit code is 1 with a `schema` check error: the project schema requires at least one module (`minItems: 1`), so SchemaChecker rejects the file before any structural check runs. Structural validation alone would pass — no nodes to check — but the schema is the component that decides an empty-modules project is not yet valid.

### E4: Very large error count

**Given** a spec with 500 validation errors across all checkers.
**When** `spex validate --spec-dir tmp/spec/` is executed.
**Then** all 500 errors appear in the JSON output. No truncation or "and N more..." unless explicitly designed.

### E5: Validate command resolves relative paths

**Given** `spex validate --spec-dir ./spec/` is called from the repo root.
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
