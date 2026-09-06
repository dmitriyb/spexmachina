# Name and Coverage Tests

Integration and acceptance test scenarios for NameConsistencyChecker (component 7) and TestCoverageChecker (component 8). RequirementCoverageChecker is exercised through `spex validate` in test_validation_pipeline.md (V16, V18).

## Setup

Each scenario reads one checked-in fixture directory under `validator/testdata/` (`name_*` for NameConsistencyChecker, `coverage_*` for TestCoverageChecker, `reqcov_*` for RequirementCoverageChecker). The RequirementCoverageChecker's eight pre-existing tests live in `validator/requirement_coverage_checker_test.go`. Node ids are 12-character identity hashes, written in these fixtures as `000000000001`, `000000000002`, … — the scenario text below numbers components 1, 2, 3 as shorthand for those hashes, never as literal JSON values.

### Naming Conventions


- Module names must be lowercase.
- `project.json` `modules[].name` must exactly equal the corresponding `module.json` `name`.
- Directory names follow the module path, which is separate from the module name but conventionally matches.

---

## Scenarios

### Declared Coverage Chain Scenarios

The coverage rules both checkers enforce are declared chains read from the resolved profile — each the triple "every node of type A must be the target of at least one edge of kind E from some node of type B". These scenarios exercise the declaration; the default profile's three declared links reproduce the fixed rules byte-for-byte.

#### CC1: Dropping a declared link removes the check

**Given** a `spec/profile.json` identical to the default except the component-to-test_section coverage link is absent, over a fixture with an uncovered component.
**When** the validation pipeline runs.
**Then** zero test-coverage errors — the checker enforces only the chains the profile declares. The same fixture under the default profile yields the uncovered-component error unchanged.

#### CC3: A spec validates under its own profile and fails under the default

**Given** a fixture spec authored against a deliberately different profile — one type renamed, one coverage link dropped — with the profile file in place.
**When** the full validation pipeline runs, then runs again with the profile file removed.
**Then** the first run reports zero errors; the second reports schema-conformance errors for the renamed type's array and would report the dropped link's coverage errors. This is the acceptance test that the profile is load-bearing, not decorative.

---

## Edge Cases

### E1: Interaction between NameConsistencyChecker and SchemaChecker

**Given** `alpha/module.json` fails schema validation (e.g., missing required fields) but has a `name` field that mismatches project.json.
**When** both SchemaChecker and NameConsistencyChecker run in the pipeline.
**Then** both produce errors independently. NameConsistencyChecker does not depend on schema validity to extract the `name` field from a parseable JSON file.

