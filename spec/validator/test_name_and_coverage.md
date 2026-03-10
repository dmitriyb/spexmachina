# Name and Coverage Tests

Integration and acceptance test scenarios for NameConsistencyChecker (component 8) and TestCoverageChecker (component 9).

## Setup

All scenarios use a temporary spec directory with a valid baseline project. The fixture includes a `project.json` that declares modules with names matching their `module.json` `name` fields, and each module has `test_sections` covering all components.

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
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** zero errors for module alpha.

#### N2: All modules consistent

**Given** every module in `project.json` has a name matching its `module.json` name field.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** an empty error slice.

#### N3: Case mismatch detected with fix suggestion

**Given** `project.json` declares `name: "alpha"` but `alpha/module.json` has `name: "Alpha"`.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** one error with:
- `check`: `"name_consistency"`
- `message` containing both `"alpha"` and `"Alpha"`
- `message` containing a fix suggestion (e.g., "change module.json name to 'alpha'")
**Rationale** Case-insensitive comparison detects likely matches (requirement 10).

#### N4: Entirely different names

**Given** `project.json` declares `name: "alpha"` but `alpha/module.json` has `name: "widget"`.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** one error referencing both names `"alpha"` and `"widget"`, reported as a name conflict rather than a case mismatch. No fix suggestion since the names are unrelated.

#### N5: Uppercase name violates convention

**Given** `project.json` declares `name: "Alpha"` (uppercase A) and `alpha/module.json` also has `name: "Alpha"`.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** one error or warning flagging the non-lowercase name convention violation. The names match each other, but both violate the lowercase rule.

#### N6: Multiple mismatches across modules

**Given** alpha has a case mismatch and beta has a name conflict.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** two errors, one per module. Each error identifies the specific module path and both name values.

#### N7: module.json unreadable

**Given** `project.json` declares module `alpha` but `alpha/module.json` is not valid JSON.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** one error indicating the module.json could not be parsed. The checker does not panic on invalid input.

#### N8: Hyphenated names match exactly

**Given** `project.json` declares `name: "my-module"` and `my-module/module.json` has `name: "my-module"`.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** zero errors. Hyphens are valid in module names and must match exactly.

#### N9: Name with trailing whitespace

**Given** `alpha/module.json` has `name: "alpha "` (trailing space).
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** one error. The comparison is exact, so `"alpha"` does not equal `"alpha "`. This may also be caught by schema validation if the schema enforces a name pattern.

---

### TestCoverageChecker Scenarios

#### T1: All components covered

**Given** alpha has components 1 and 2. Test sections have `describes: [1]` and `describes: [2]` respectively.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** zero errors.

#### T2: One uncovered component

**Given** alpha has components 1 and 2. Only one test_section exists with `describes: [1]`. Component 2 has no test_section.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** one error with:
- `message` containing the component name and id
- `message` containing `"no test_section coverage"` (or similar)
- The error identifies module alpha

#### T3: Multiple uncovered components

**Given** alpha has components 1, 2, and 3. Only component 1 is covered by a test_section.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** two errors, one for component 2 and one for component 3. Each error includes the component name and ID.

#### T4: Module with no test_sections array

**Given** alpha's `module.json` has no `test_sections` key (the array is absent or null).
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** one error per component in alpha, since none can be covered.

#### T5: Module with no components

**Given** beta's `module.json` has `components: []` (empty array) and no test_sections.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** zero errors for beta. There is nothing to cover.

#### T6: test_section with empty describes array

**Given** alpha has one test_section with `describes: []` and two components.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** two errors (one per uncovered component). An empty `describes` array is valid but covers nothing.

#### T7: Component covered by multiple test_sections

**Given** alpha has component 1. Two test_sections both include `1` in their `describes` arrays.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** zero errors for component 1. Multiple coverage is allowed.

#### T8: Single test_section covers all components

**Given** alpha has components 1, 2, and 3. One test_section has `describes: [1, 2, 3]`.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** zero errors. A single test_section can cover multiple components.

#### T9: Uncovered components across multiple modules

**Given** alpha has one uncovered component and beta has one uncovered component.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** two errors, each identifying the correct module.

#### T10: test_section describes non-existent component

**Given** alpha has a test_section with `describes: [99]` but no component 99 exists. Component 1 exists and is not covered.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** one error for uncovered component 1. The dangling reference `99` is an IDValidator concern, not a TestCoverageChecker concern. TestCoverageChecker only checks that each component ID appears in at least one `describes` set.

---

## Edge Cases

### E1: Interaction between NameConsistencyChecker and SchemaChecker

**Given** `alpha/module.json` fails schema validation (e.g., missing required fields) but has a `name` field that mismatches project.json.
**When** both SchemaChecker and NameConsistencyChecker run in the pipeline.
**Then** both produce errors independently. NameConsistencyChecker does not depend on schema validity to extract the `name` field from a parseable JSON file.

### E2: TestCoverageChecker does not duplicate OrphanDetector

**Given** alpha has component 1 which is not described by any impl_section (orphan) and not described by any test_section (uncovered).
**When** both OrphanDetector and TestCoverageChecker run.
**Then** OrphanDetector reports component 1 as an orphan (no impl_section). TestCoverageChecker reports component 1 as uncovered (no test_section). These are independent, non-duplicative findings.

### E3: Module path differs from module name

**Given** `project.json` declares a module with `name: "core-lib"` and `path: "core"`. The directory is `spec/core/module.json` with `name: "core-lib"`.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** zero errors. The name comparison is between `project.json` `name` and `module.json` `name`, not the directory path.

### E4: Empty module name

**Given** `alpha/module.json` has `name: ""` and `project.json` also has `name: ""` for that module.
**When** `CheckNameConsistency(specDir, project)` is called.
**Then** the names match, but the empty-string name may be flagged by the schema checker or by a lowercase convention check. NameConsistencyChecker reports zero errors for the name comparison itself.

### E5: Large module count performance

**Given** a project with 100 modules, each with 10 components and test_sections covering all of them.
**When** `CheckTestCoverage(specDir, project)` is called.
**Then** it completes within the 1-second performance budget and returns zero errors. The algorithm is O(modules * components) per module with a set lookup for coverage.
