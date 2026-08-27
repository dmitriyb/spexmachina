# RequirementCoverageChecker

Validates that every requirement in the spec is covered by implementation: project requirements must be derived into module requirements, and module requirements must be implemented by components. That two-link chain is what [[168ae8fde8e2|requirement coverage validation]] asks the spec to hold.

The two links are declared coverage chains read from the resolved profile — each the triple of covered type, edge kind and covering type — not constants of the checker. The default profile declares exactly these two, so everything below describes the default declaration; a profile that drops one drops its check, and a profile that renames a type sees its declared name interpolated into the same message shapes.

## Responsibilities

- Check that every project requirement has at least one module requirement with a matching `preq_id`
- Skip a project requirement that declares `derivation: pending` and emit a disclosure note in its place — a declared gap, never an error
- Check that every module requirement has at least one component with a matching `implements` entry
- Report uncovered requirements as validation errors with the requirement ID, title, and what's missing

## Interface

Given the path to a spec directory, the checker returns a flat list of validation entries — empty when both links of the chain hold — together with a list of disclosure notes, one per underived project requirement declaring `derivation: pending`, empty when no requirement is in that state. If the spec cannot be loaded, the load failures are returned under this checker's own name and no coverage is computed. The project-level pass walks `project.json`'s requirements in declaration order; the module-level pass visits modules in sorted name order and, within a module, requirements in declaration order. The same spec therefore produces the same entries and the same notes in the same sequence.

## Checks

### Project requirement → module requirement coverage

For each project requirement ID, scan all module requirements across all modules for a matching `preq_id`. A module requirement whose `preq_id` is empty derives from nothing and covers nothing here. If no module requirement derives from a project requirement, report an entry at path `project.json` whose message interpolates the requirement's identity hash and its title, quoted — no ordinal, no trailing parenthetical. Were `a65bbd37c7ec` uncovered, the message would read:

```
project requirement a65bbd37c7ec "Coupled sections" is not derived into any module requirement
```

A project requirement declaring `derivation: pending` is exempt from this link: no entry is reported for it, and a disclosure note of type `pending_derivation` takes its place, in the same shape the error uses. Were `a65bbd37c7ec` declared pending, the note's message would read:

```
project requirement a65bbd37c7ec "Coupled sections" declares derivation pending and is not derived into any module requirement
```

The note stands strictly in the error's place: it is emitted exactly where the error would have been, so a pending requirement that has in fact gained a deriver produces neither — it is covered like any other, and its `derivation` field is simply stale, inert until removed. The whole of what the field buys is the downgrade from error to note on the underived case; a requirement without it goes down the error path above unchanged.

### Module requirement → component coverage

For each module requirement ID within a module, scan the module's components for a matching `implements` entry. If no component implements a module requirement, report an entry at path `<module>/module.json` whose message leads with the module name, then the requirement's identity hash and quoted title. Were `168ae8fde8e2` uncovered, the message would read:

```
validator requirement 168ae8fde8e2 "Requirement coverage validation" is not implemented by any component
```

## Design Rationale

This checker owns both links of the coverage chain: project requirement → module requirement (`preq_id`) and module requirement → component (`implements`). Keeping them in one checker means a requirement that is declared but never derived, and one that is derived but never implemented, are reported by the same check with the same shape of message.

Together the two checks ensure the full chain: project requirement → module requirement (preq_id) → component (implements) is wired up.
