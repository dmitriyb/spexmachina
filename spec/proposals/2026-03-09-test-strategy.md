# Change Proposal: Test Strategy in Spec

## Context

The spec defines WHAT to build (requirements, components) and HOW to build it (impl_sections, data_flows), but not HOW TO VERIFY it. No testing concept exists in the spec format, the `/spec` skill, or the Schema module.

Unit tests are handled by the `/implement` skill (Go `_test.go` files), but there is no way to specify module-level integration scenarios or cross-module pipeline tests in the spec. This means test coverage is ad hoc — it depends on whatever the implementing agent decides to test, with no traceability back to spec requirements.

## Proposed change

### Two-Level Test Strategy

Mirrors the existing project→module decomposition:

**Level 1 — Module integration/acceptance tests** (`test_sections` in `module.json`):
- Per-component or per-module verification scenarios
- Content in `test_*.md` leaves
- `describes` field links to components (like `impl_sections`)
- Covers: do the components within this module work together correctly?
- NOT unit tests — those remain in Go `_test.go` files

**Level 2 — Pipeline/E2E tests** (`test_plan` in `project.json`):
- Cross-module scenarios (e.g. validate → diff → impact → apply produces correct beads)
- Content in project-level `test_*.md` leaves
- Covers: does the full pipeline work end-to-end?

### Schema changes

**`module.schema.json`**: Add `test_sections` array (parallel to `impl_sections`):
```json
{
  "id": "<integer>",
  "name": "<string>",
  "content": "<path to test_*.md>",
  "describes": ["<component IDs>"]
}
```

**`project.schema.json`**: Add `test_plan` section:
```json
{
  "scenarios": [
    {
      "id": "<integer>",
      "name": "<string>",
      "content": "<path to test_*.md>",
      "modules": ["<module IDs>"]
    }
  ]
}
```

### Validator changes

- **TestCoverageChecker**: New checker — every component must be described by at least one `test_section`.
- **ContentResolver**: Updated to validate `test_*.md` paths.

### `/spec` skill changes

- New step after writing impl content: generate test scenarios for each component.
- Schema reference section updated with test format.
- Validate step checks test coverage (every component has ≥1 test scenario).

## Impact expectation

- **Modified beads**: Schema/ProjectSchema (add `test_plan`), Schema/ModuleSchema (add `test_sections`), Validator/ContentResolver (validate test paths), Validator (new TestCoverageChecker component)
- **Modified skill**: `/spec` (generate test sections, validate test coverage)
- **New content**: `test_*.md` files added to every existing module retrospectively
