# RequirementCoverageChecker

Validates that every requirement in the spec is covered by implementation: project requirements must be derived into module requirements, and module requirements must be implemented by components.

## Responsibilities

- Check that every project requirement has at least one module requirement with a matching `preq_id`
- Check that every module requirement has at least one component with a matching `implements` entry
- Report uncovered requirements as validation errors with the requirement ID, title, and what's missing

## Checks

### Project requirement → module requirement coverage

For each project requirement ID, scan all module requirements across all modules for a matching `preq_id`. If no module requirement derives from a project requirement, report:

```
project requirement 15 "Delivery specification" is not derived into any module requirement (no preq_id references it)
```

### Module requirement → component coverage

For each module requirement ID within a module, scan the module's components for a matching `implements` entry. If no component implements a module requirement, report:

```
validator requirement 14 "Requirement coverage validation" is not implemented by any component (no implements references it)
```

## Design Rationale

This checker complements the existing OrphanDetector (component 4), which checks that every requirement is referenced by at least one component. RequirementCoverageChecker adds the project→module derivation check (preq_id coverage) which OrphanDetector does not cover.

Together they ensure the full chain: project requirement → module requirement (preq_id) → component (implements) is wired up.
