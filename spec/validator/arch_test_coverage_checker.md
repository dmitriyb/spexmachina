# TestCoverageChecker

Validates that every component in each module is described by at least one `test_section`, which is what [[a88e6fb4463d|test coverage checking]] requires of a spec.

The rule is a declared coverage chain read from the resolved profile — covered type, edge kind, covering type — with the checker owning the enforcement and the profile owning the triple. The default profile declares the component-describes-test_section link, so everything below describes the default declaration; the declared type names are interpolated into the error shape, and a profile that drops the link drops the check.

## Responsibilities

- Walk all modules and their components
- For each component, check if any `test_section` in the same module has the component's ID in its `describes` array
- Report uncovered components as validation errors

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when every component is covered. If the spec cannot be loaded, the load failures are returned under this checker's own name and no coverage is computed. Modules are visited in sorted name order and, within a module, components in declaration order, so the same spec produces the same entries in the same sequence.

## Behavior

1. For each module in `project.json`, read its `module.json`
2. Collect all component IDs in the module
3. Collect all component IDs referenced by `test_sections[].describes`
4. Any component ID not in the describes set is uncovered and produces one entry, located at that component's own declaration

Coverage is judged inside a module: a `test_section` in one module does not cover a component in another, because `describes` is a module-local edge.

## Error Format

Each entry raised against an uncovered component carries:
- A path locating the declaration to fix: `<module>/module.json:/components/<component id>`
- A message naming the component and repeating its identity hash: `component <name> (id:<id>) has no test_section coverage`

The load failures `## Interface` describes are the exception: they are located at the file that failed to load and carry that file's read or parse error as their message.

## Edge Cases

- Module with no components: no errors (nothing to cover)
- Module with no `test_sections` array: every component is uncovered
- `test_section` with empty `describes`: valid but covers no components
- Component covered by multiple test_sections: valid (no uniqueness constraint)
