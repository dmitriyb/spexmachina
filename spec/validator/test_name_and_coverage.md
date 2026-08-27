# Name and Coverage Tests

Integration and acceptance test scenarios for NameConsistencyChecker (component 7), TestCoverageChecker (component 8) and RequirementCoverageChecker (component 9).

## Setup

Each scenario reads one checked-in fixture directory under `validator/testdata/` (`name_*` for NameConsistencyChecker, `coverage_*` for TestCoverageChecker, `reqcov_*` for RequirementCoverageChecker). The RequirementCoverageChecker's eight pre-existing tests live in `validator/requirement_coverage_checker_test.go`; the scenarios this leaf carries for it are the derivation-pending ones below, which cover the checker's skip-and-note path and its unchanged error path. Node ids are 12-character identity hashes, written in these fixtures as `000000000001`, `000000000002`, … — the scenario text below numbers components 1, 2, 3 as shorthand for those hashes, never as literal JSON values.

### Fixture Structure

```
tmp/spec/
  project.json                 # declares modules: alpha (id 1), beta (id 2)
  alpha/
    module.json                # name: "alpha", 2 components (1,2), 2 test_sections covering both
    arch_parser.md
    arch_renderer.md
    test_parser.md
    test_renderer.md
  beta/
    module.json                # name: "beta", 1 component (1), 1 test_section covering it
    arch_encoder.md
    test_encoder.md
```

### Naming Conventions

- Module names must be lowercase.
- `project.json` `modules[].name` must exactly equal the corresponding `module.json` `name`.
- Directory names follow the module path, which is separate from the module name but conventionally matches.

---

## Scenarios

### NameConsistencyChecker Scenarios

#### N1: Matching names pass

**Given** `project.json` declares module with `name: "alpha"` and `alpha/module.json` has `name: "alpha"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** zero errors for module alpha.

#### N2: All modules consistent

**Given** every module in `project.json` has a name matching its `module.json` name field.
**When** `CheckNameConsistency(specDir)` is called.
**Then** an empty error slice.

#### N3: Case mismatch detected with fix suggestion

**Given** `project.json` declares `name: "alpha"` but `alpha/module.json` has `name: "Alpha"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** one error with:
- `check`: `"name_consistency"`
- `message` containing both `"alpha"` and `"Alpha"`
- `message` containing a fix suggestion (e.g., "change module.json name to 'alpha'")
**Rationale** Case-insensitive comparison detects likely matches (requirement 9).

#### N4: Entirely different names

**Given** `project.json` declares `name: "alpha"` but `alpha/module.json` has `name: "widget"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** one error referencing both names `"alpha"` and `"widget"`, reported as a name conflict rather than a case mismatch. No fix suggestion since the names are unrelated.

#### N5: Uppercase name violates convention

**Given** `project.json` declares `name: "Alpha"` (uppercase A) and `alpha/module.json` also has `name: "Alpha"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** one error or warning flagging the non-lowercase name convention violation. The names match each other, but both violate the lowercase rule.

#### N6: Multiple mismatches across modules

**Given** alpha has a case mismatch and beta has a name conflict.
**When** `CheckNameConsistency(specDir)` is called.
**Then** two errors, one per module. Each error identifies the specific module path and both name values.

#### N7: module.json unreadable

**Given** `project.json` declares module `alpha` but `alpha/module.json` is not valid JSON.
**When** `CheckNameConsistency(specDir)` is called.
**Then** one error indicating the module.json could not be parsed. The checker does not panic on invalid input.

#### N8: Hyphenated names match exactly

**Given** `project.json` declares `name: "my-module"` and `my-module/module.json` has `name: "my-module"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** zero errors. Hyphens are valid in module names and must match exactly.

#### N9: Name with trailing whitespace

**Given** `alpha/module.json` has `name: "alpha "` (trailing space).
**When** `CheckNameConsistency(specDir)` is called.
**Then** one error. The comparison is exact, so `"alpha"` does not equal `"alpha "`. This may also be caught by schema validation if the schema enforces a name pattern.

---

### TestCoverageChecker Scenarios

#### T1: All components covered

**Given** alpha has components 1 and 2. Test sections have `describes: [1]` and `describes: [2]` respectively.
**When** `CheckTestCoverage(specDir)` is called.
**Then** zero errors.

#### T2: One uncovered component

**Given** alpha has components 1 and 2. Only one test_section exists with `describes: [1]`. Component 2 has no test_section.
**When** `CheckTestCoverage(specDir)` is called.
**Then** one error with:
- `message` containing the component name and id
- `message` containing `"no test_section coverage"` (or similar)
- The error identifies module alpha

#### T3: Multiple uncovered components

**Given** alpha has components 1, 2, and 3. Only component 1 is covered by a test_section.
**When** `CheckTestCoverage(specDir)` is called.
**Then** two errors, one for component 2 and one for component 3. Each error includes the component name and ID.

#### T4: Module with no test_sections array

**Given** alpha's `module.json` has no `test_sections` key (the array is absent or null).
**When** `CheckTestCoverage(specDir)` is called.
**Then** one error per component in alpha, since none can be covered.

#### T5: Module with no components

**Given** beta's `module.json` has `components: []` (empty array) and no test_sections.
**When** `CheckTestCoverage(specDir)` is called.
**Then** zero errors for beta. There is nothing to cover.

#### T6: test_section with empty describes array

**Given** alpha has one test_section with `describes: []` and two components.
**When** `CheckTestCoverage(specDir)` is called.
**Then** two errors (one per uncovered component). An empty `describes` array is valid but covers nothing.

#### T7: Component covered by multiple test_sections

**Given** alpha has component 1. Two test_sections both include `1` in their `describes` arrays.
**When** `CheckTestCoverage(specDir)` is called.
**Then** zero errors for component 1. Multiple coverage is allowed.

#### T8: Single test_section covers all components

**Given** alpha has components 1, 2, and 3. One test_section has `describes: [1, 2, 3]`.
**When** `CheckTestCoverage(specDir)` is called.
**Then** zero errors. A single test_section can cover multiple components.

#### T9: Uncovered components across multiple modules

**Given** alpha has one uncovered component and beta has one uncovered component.
**When** `CheckTestCoverage(specDir)` is called.
**Then** two errors, each identifying the correct module.

#### T10: test_section describes non-existent component

**Given** alpha has a test_section with `describes: [99]` but no component 99 exists. Component 1 exists and is not covered.
**When** `CheckTestCoverage(specDir)` is called.
**Then** one error for uncovered component 1. The dangling reference `99` is an IDValidator concern, not a TestCoverageChecker concern. TestCoverageChecker only checks that each component ID appears in at least one `describes` set.

---

### RequirementCoverageChecker Scenarios

#### RC1: Underived requirement without the field is still an error

**Given** `project.json` declares a requirement with no `derivation` field, and no module requirement in any module carries its id as `preq_id`.
**When** the requirement-coverage check runs.
**Then** one error at path `project.json`, message `project requirement <hash> "<title>" is not derived into any module requirement` — byte-for-byte the pre-existing message — and zero notes.

#### RC2: Pending requirement produces a note, not an error

**Given** the same fixture, with the underived requirement declaring `derivation: "pending"`.
**When** the requirement-coverage check runs.
**Then** zero errors and one note with:
- `type`: `"pending_derivation"`
- `message`: `project requirement <hash> "<title>" declares derivation pending and is not derived into any module requirement`
- `related` containing exactly that requirement's identity hash

#### RC3: Derived pending requirement produces neither

**Given** a requirement declaring `derivation: "pending"` that a module requirement nevertheless derives via `preq_id`.
**When** the requirement-coverage check runs.
**Then** zero errors and zero notes for it. The note stands strictly in the error's place, so a covered requirement emits nothing; the stale field is inert.

#### RC4: The module-level link admits no pending state

**Given** a module requirement not implemented by any component, in a spec whose project requirements all declare `derivation: "pending"`.
**When** the requirement-coverage check runs.
**Then** the `<module> requirement <hash> "<title>" is not implemented by any component` error is reported unchanged. The pending state exempts only the project-to-module link.

#### RC5: Notes are deterministic and ordered

**Given** three underived pending requirements declared in a fixed order in `project.json`.
**When** the check runs twice.
**Then** both runs return the same three notes in declaration order.

### Declared Coverage Chain Scenarios

The coverage rules both checkers enforce are declared chains read from the resolved profile — each the triple "every node of type A must be the target of at least one edge of kind E from some node of type B". These scenarios exercise the declaration; every scenario above runs under the default profile, whose three declared links reproduce the fixed rules byte-for-byte.

#### CC1: Dropping a declared link removes the check

**Given** a `spec/profile.json` identical to the default except the component-to-test_section coverage link is absent, over a fixture with an uncovered component.
**When** the validation pipeline runs.
**Then** zero test-coverage errors — the checker enforces only the chains the profile declares. The same fixture under the default profile yields the T2 error unchanged.

#### CC2: Declared type names are interpolated into the error shapes

**Given** a profile that renames `component` to `endpoint` (same shape, different name), over a fixture with an uncovered endpoint.
**When** the coverage checks run.
**Then** the error carries the same shape as T2 with the declared type names interpolated — the uncovered node named as an endpoint, the covering type as declared — rather than the literal words "component" and "test_section".

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

### E3: Module path differs from module name

**Given** `project.json` declares a module with `name: "core-lib"` and `path: "core"`. The directory is `spec/core/module.json` with `name: "core-lib"`.
**When** `CheckNameConsistency(specDir)` is called.
**Then** zero errors. The name comparison is between `project.json` `name` and `module.json` `name`, not the directory path.

### E4: Empty module name

**Given** `alpha/module.json` has `name: ""` and `project.json` also has `name: ""` for that module.
**When** `CheckNameConsistency(specDir)` is called.
**Then** the names match, but the empty-string name may be flagged by the schema checker or by a lowercase convention check. NameConsistencyChecker reports zero errors for the name comparison itself.

### E5: Large module count performance

**Given** a project with 100 modules, each with 10 components and test_sections covering all of them.
**When** `CheckTestCoverage(specDir)` is called.
**Then** it completes within the 1-second performance budget and returns zero errors. The algorithm is O(modules * components) per module with a set lookup for coverage.
