# Graph Structure Tests

Integration and acceptance test scenarios for DAGChecker (component 3), OrphanDetector (component 4), and IDValidator (component 5).

## Setup

All scenarios use a temporary spec directory with a valid baseline project. The fixture builder creates well-formed JSON that passes schema validation, then introduces targeted mutations for each scenario.

### Fixture Structure

```
tmp/spec/
  project.json                 # 3 modules: alpha, beta, gamma
  alpha/
    module.json                # 3 requirements (1,2,3), 2 components (1,2), 2 impl_sections (1,2)
  beta/
    module.json                # 2 requirements (1,2), 1 component (1), 1 impl_section (1)
  gamma/
    module.json                # 1 requirement (1), 1 component (1), 1 impl_section (1)
```

### Dependency Baseline

- Module `beta` has `requires_module: [1]` (depends on alpha).
- Module `gamma` has `requires_module: [2]` (depends on beta).
- In alpha: requirement 2 has `depends_on: [1]`; component 2 has `uses: [1]`.

---

## Scenarios

### DAGChecker Scenarios

#### D1: Clean dependency graphs pass

**Given** the baseline fixture with no cycles in module, requirement, or component graphs.
**When** `CheckDAG(project, modules)` is called.
**Then** it returns an empty error slice.

#### D2: Direct module dependency cycle (A requires B, B requires A)

**Given** module `alpha` has `requires_module: [2]` and module `beta` has `requires_module: [1]`.
**When** `CheckDAG(project, modules)` is called.
**Then** one error with:
- `check`: `"dag"`
- `message` containing the cycle path `alpha -> beta -> alpha` (or equivalent)

#### D3: Indirect module dependency cycle (A -> B -> C -> A)

**Given** module `alpha` has `requires_module: [2]`, `beta` has `requires_module: [3]`, `gamma` has `requires_module: [1]`.
**When** `CheckDAG(project, modules)` is called.
**Then** one error whose `message` includes the full three-node cycle path.

#### D4: Self-referential module dependency

**Given** module `alpha` has `requires_module: [1]` (its own ID).
**When** `CheckDAG(project, modules)` is called.
**Then** one error detecting the self-cycle `alpha -> alpha`.

#### D5: Requirement dependency cycle within a module

**Given** in alpha: requirement 1 has `depends_on: [2]` and requirement 2 has `depends_on: [1]`.
**When** `CheckDAG(project, modules)` is called.
**Then** one error referencing the requirement cycle within module `alpha`, with the cycle path in the message.

#### D6: Component `uses` cycle within a module

**Given** in alpha: component 1 has `uses: [2]` and component 2 has `uses: [1]`.
**When** `CheckDAG(project, modules)` is called.
**Then** one error referencing the component cycle within module `alpha`.

#### D7: Cycles in multiple graphs reported independently

**Given** a module cycle (alpha <-> beta) AND a requirement cycle within alpha.
**When** `CheckDAG(project, modules)` is called.
**Then** at least two errors, one for the module graph cycle and one for the requirement graph cycle. Each error identifies which graph type it belongs to.

#### D8: DAG check on module with no dependencies

**Given** module `gamma` has an empty `requires_module` array and its requirements have no `depends_on`.
**When** `CheckDAG(project, modules)` is called.
**Then** zero errors for gamma. Isolated nodes are valid DAG members.

---

### OrphanDetector Scenarios

#### O1: No orphans in baseline

**Given** every requirement in alpha is referenced by at least one component's `implements`, and every component is referenced by at least one impl_section's `describes`.
**When** `CheckOrphans(modules)` is called.
**Then** it returns an empty slice (or zero warnings, depending on severity model).

#### O2: Orphan requirement (not implemented by any component)

**Given** alpha has requirement 3 and no component includes `3` in its `implements` array.
**When** `CheckOrphans(modules)` is called.
**Then** one warning with:
- `severity`: `"warning"` (orphans are warnings, not hard errors)
- `message` identifying requirement 3 in module alpha as unimplemented

#### O3: Orphan component (not described by any impl_section)

**Given** alpha has component 2 and no impl_section includes `2` in its `describes` array.
**When** `CheckOrphans(modules)` is called.
**Then** one warning identifying component 2 in alpha as undescribed.

#### O4: Multiple orphans across multiple modules

**Given** alpha has one orphan requirement AND beta has one orphan component.
**When** `CheckOrphans(modules)` is called.
**Then** two warnings, each referencing the correct module.

#### O5: Requirement referenced by component in another module

**Given** component in beta includes requirement ID 1 in its `implements`, but requirement 1 belongs to alpha, not beta. Beta has no requirement 1.
**When** `CheckOrphans(modules)` is called.
**Then** alpha's requirement 1 is still orphaned within alpha (cross-module implements is not valid coverage). Beta's cross-reference is an IDValidator concern, not an OrphanDetector concern.

#### O6: All requirements orphaned (empty components list)

**Given** alpha has three requirements and an empty `components` array.
**When** `CheckOrphans(modules)` is called.
**Then** three warnings, one per orphan requirement.

---

### IDValidator Scenarios

#### I1: All IDs unique and references valid

**Given** the baseline fixture with unique IDs and all cross-references pointing to existing nodes.
**When** `CheckIDs(project, modules)` is called.
**Then** it returns an empty error slice.

#### I2: Duplicate requirement IDs within a module

**Given** alpha has two requirements both with `id: 1`.
**When** `CheckIDs(project, modules)` is called.
**Then** one error identifying the duplicate ID `1` in `alpha/module.json:requirements`.

#### I3: Duplicate component IDs within a module

**Given** alpha has two components both with `id: 1`.
**When** `CheckIDs(project, modules)` is called.
**Then** one error identifying duplicate component ID `1` in alpha.

#### I4: Duplicate impl_section IDs within a module

**Given** alpha has two impl_sections both with `id: 1`.
**When** `CheckIDs(project, modules)` is called.
**Then** one error identifying the duplicate.

#### I5: Duplicate module IDs in project.json

**Given** `project.json` has two modules both with `id: 1`.
**When** `CheckIDs(project, modules)` is called.
**Then** one error referencing `project.json:modules` and the duplicate ID.

#### I6: Component `implements` references non-existent requirement

**Given** alpha's component 1 has `implements: [99]` and there is no requirement with id 99 in alpha.
**When** `CheckIDs(project, modules)` is called.
**Then** one error with:
- Source: component 1 in alpha
- Dangling reference: requirement 99
- `message` indicating the target does not exist

#### I7: Component `uses` references non-existent component

**Given** alpha's component 1 has `uses: [50]` and no component 50 exists in alpha.
**When** `CheckIDs(project, modules)` is called.
**Then** one error identifying the dangling `uses` reference.

#### I8: impl_section `describes` references non-existent component

**Given** alpha's impl_section 1 has `describes: [77]` and no component 77 exists.
**When** `CheckIDs(project, modules)` is called.
**Then** one error identifying the dangling `describes` reference.

#### I9: Requirement `depends_on` references non-existent requirement

**Given** alpha's requirement 1 has `depends_on: [42]` and no requirement 42 exists in alpha.
**When** `CheckIDs(project, modules)` is called.
**Then** one error for the dangling `depends_on` reference.

#### I10: Module `requires_module` references non-existent module

**Given** alpha has `requires_module: [99]` and no module with id 99 exists in `project.json`.
**When** `CheckIDs(project, modules)` is called.
**Then** one error referencing the dangling module dependency.

#### I11: Milestone `groups` references non-existent module

**Given** `project.json` has a milestone with `groups: [88]` and no module with id 88 exists.
**When** `CheckIDs(project, modules)` is called.
**Then** one error for the dangling milestone group reference.

#### I12: Requirement `preq_id` references non-existent project requirement

**Given** alpha's requirement 1 has `preq_id: 999` and no project-level requirement with id 999 exists.
**When** `CheckIDs(project, modules)` is called.
**Then** one error for the dangling `preq_id` reference.

#### I13: Multiple dangling references in one module

**Given** alpha has a dangling `implements`, a dangling `uses`, and a dangling `describes` reference.
**When** `CheckIDs(project, modules)` is called.
**Then** three errors, one per dangling reference. All are reported, not just the first.

#### I14: data_flow `uses` references non-existent component

**Given** alpha has a data_flow with `uses: [42]` and no component 42 exists.
**When** `CheckIDs(project, modules)` is called.
**Then** one error for the dangling data_flow `uses` reference.

---

## Edge Cases

### E1: Module with empty arrays

**Given** alpha has `requirements: []`, `components: []`, `impl_sections: []`.
**When** all three checkers run.
**Then** DAGChecker: zero errors (no edges to form cycles). OrphanDetector: zero warnings (nothing to be orphaned). IDValidator: zero errors (no IDs to duplicate or reference).

### E2: Same numeric ID reused across different array types

**Given** alpha has requirement id 1, component id 1, and impl_section id 1. All cross-references are valid.
**When** `CheckIDs(project, modules)` is called.
**Then** zero errors. IDs only need to be unique within their own array type, not globally across types.

### E3: Large graph performance

**Given** a project with 50 modules, each with 20 requirements and 10 components, forming a deep but acyclic dependency chain.
**When** `CheckDAG(project, modules)` is called.
**Then** it completes in under 100ms and returns zero errors. Validates the O(V+E) complexity claim.

### E4: Orphan detection severity does not affect exit code

**Given** alpha has one orphan requirement (warning) and zero hard errors across all other checkers.
**When** the full validation pipeline runs.
**Then** exit code is 0 because warnings do not cause failure. The report includes the warning in the `errors` array with `severity: "warning"`.

### E5: test_section `describes` references non-existent component

**Given** alpha has a test_section with `describes: [99]` and no component 99 exists.
**When** `CheckIDs(project, modules)` is called.
**Then** one error for the dangling test_section `describes` reference. IDValidator must walk test_sections in addition to impl_sections.
