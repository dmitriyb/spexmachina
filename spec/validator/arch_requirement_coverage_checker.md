# RequirementCoverageChecker

Validates that every requirement in the spec is covered by implementation: project requirements must be derived into module requirements, and module requirements must be implemented by components. That two-link chain is what [[168ae8fde8e2|requirement coverage validation]] asks the spec to hold.

## Responsibilities

- Check that every project requirement has at least one module requirement with a matching `preq_id`
- Check that every module requirement has at least one component with a matching `implements` entry
- Report uncovered requirements as validation errors with the requirement ID, title, and what's missing

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when both links of the chain hold. If the spec cannot be loaded, the load failures are returned under this checker's own name and no coverage is computed. The project-level pass walks `project.json`'s requirements in declaration order; the module-level pass visits modules in sorted name order and, within a module, requirements in declaration order. The same spec therefore produces the same entries in the same sequence.

## Checks

### Project requirement → module requirement coverage

For each project requirement ID, scan all module requirements across all modules for a matching `preq_id`. A module requirement whose `preq_id` is empty derives from nothing and covers nothing here. If no module requirement derives from a project requirement, report an entry at path `project.json` whose message interpolates the requirement's identity hash and its title, quoted — no ordinal, no trailing parenthetical. Were `a65bbd37c7ec` uncovered, the message would read:

```
project requirement a65bbd37c7ec "Coupled sections" is not derived into any module requirement
```

### Module requirement → component coverage

For each module requirement ID within a module, scan the module's components for a matching `implements` entry. If no component implements a module requirement, report an entry at path `<module>/module.json` whose message leads with the module name, then the requirement's identity hash and quoted title. Were `168ae8fde8e2` uncovered, the message would read:

```
validator requirement 168ae8fde8e2 "Requirement coverage validation" is not implemented by any component
```

## Design Rationale

This checker owns both links of the coverage chain: project requirement → module requirement (`preq_id`) and module requirement → component (`implements`). Keeping them in one checker means a requirement that is declared but never derived, and one that is derived but never implemented, are reported by the same check with the same shape of message.

Together the two checks ensure the full chain: project requirement → module requirement (preq_id) → component (implements) is wired up.
