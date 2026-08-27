# Graph Structure Tests

Integration and acceptance test scenarios for DAGChecker and IDValidator.

## Setup

IDValidator scenarios each read one checked-in fixture directory under `validator/testdata/id_*`, already carrying the mutations the scenarios that read it name. One fixture may serve several scenarios: `id_dangling` carries six kinds of dangling reference and is read by ten tests. DAGChecker fixtures cover four shapes only — `dag_valid` (two acyclic modules, `core` and `api`), `dag_module_cycle` (a two-module cycle), `dag_req_cycle` and `dag_comp_cycle`; the remaining DAG scenarios have no dedicated fixture, and each carries a note below saying what covers it, if anything. Node ids in fixtures are 12-character identity-hash strings (`000000000001`, `000000000099`, …); the numbering below — module 1, requirement 2, component 99 — is shorthand for them, never a literal JSON value.

### Scenario Model

The Givens below are written against a conceptual three-module baseline. It is scenario shorthand, not a description of any checked-in directory:

```
tmp/spec/
  project.json                 # 3 modules: alpha, beta, gamma
  alpha/
    module.json                # 3 requirements (1,2,3), 2 components (1,2), 2 test_sections (1,2)
  beta/
    module.json                # 2 requirements (1,2), 1 component (1), 1 test_section (1)
  gamma/
    module.json                # 1 requirement (1), 1 component (1), 1 test_section (1)
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
**When** `CheckDAG(specDir)` is called.
**Then** it returns an empty error slice.

#### D2: Direct module dependency cycle (A requires B, B requires A)

**Given** module `alpha` has `requires_module: [2]` and module `beta` has `requires_module: [1]`.
**When** `CheckDAG(specDir)` is called.
**Then** one error with:
- `check`: `"dag"`
- `message` containing the cycle path `alpha -> beta -> alpha` (or equivalent)

#### D3: Indirect module dependency cycle (A -> B -> C -> A)

**Given** module `alpha` has `requires_module: [2]`, `beta` has `requires_module: [3]`, `gamma` has `requires_module: [1]`.
**When** `CheckDAG(specDir)` is called.
**Then** one error whose `message` includes the full three-node cycle path.

> No implementing test or fixture exists for the three-node shape. Fixture-tested cycles stop at two nodes (D2), and the in-memory unit cases cover self-loops and two-node cycles only.

#### D4: Self-referential module dependency

**Given** module `alpha` has `requires_module: [1]` (its own ID).
**When** `CheckDAG(specDir)` is called.
**Then** one error detecting the self-cycle `alpha -> alpha`.

> Covered at the unit level only: the cycle detector is exercised on an in-memory self-loop (`a -> a`). No fixture realizes this Given end-to-end through `CheckDAG`.

#### D5: Requirement dependency cycle within a module

**Given** in alpha: requirement 1 has `depends_on: [2]` and requirement 2 has `depends_on: [1]`.
**When** `CheckDAG(specDir)` is called.
**Then** one error referencing the requirement cycle within module `alpha`, with the cycle path in the message.

#### D6: Component `uses` cycle within a module

**Given** in alpha: component 1 has `uses: [2]` and component 2 has `uses: [1]`.
**When** `CheckDAG(specDir)` is called.
**Then** one error referencing the component cycle within module `alpha`.

#### D7: Cycles in multiple graphs reported independently

**Given** a module cycle (alpha <-> beta) AND a requirement cycle within alpha.
**When** `CheckDAG(specDir)` is called.
**Then** at least two errors, one for the module graph cycle and one for the requirement graph cycle. Each error identifies which graph type it belongs to.

> No implementing test: no fixture or unit case exercises two independent cycles in one run. The single-graph halves are covered separately by D2 and D5.

#### D8: DAG check on module with no dependencies

**Given** module `gamma` has an empty `requires_module` array and its requirements have no `depends_on`.
**When** `CheckDAG(specDir)` is called.
**Then** zero errors for gamma. Isolated nodes are valid DAG members.

> No dedicated fixture; the property is exercised implicitly by D1, whose `dag_valid` fixture contains `core`, a module declaring no `requires_module`.

---

### IDValidator Scenarios

#### I1: All IDs unique and references valid

**Given** the baseline fixture with unique IDs and all cross-references pointing to existing nodes.
**When** `CheckIDs(specDir)` is called.
**Then** it returns an empty error slice.

#### I2: Duplicate requirement IDs within a module

**Given** alpha has two requirements both with `id: 1`.
**When** `CheckIDs(specDir)` is called.
**Then** one error at path `alpha/module.json:/requirements` whose message is `duplicate ID <hash>`.

#### I3: Duplicate component IDs within a module

**Given** alpha has two components both with `id: 1`.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying duplicate component ID `1` in alpha.

#### I4: Duplicate test_section IDs within a module

**Given** alpha has two test_sections both with `id: 1`.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the duplicate.

#### I5: Duplicate module IDs in project.json

**Given** `project.json` has two modules both with `id: 1`.
**When** `CheckIDs(specDir)` is called.
**Then** one error at path `project.json:/modules` whose message is `duplicate ID <hash>`.

#### I6: Component `implements` references non-existent requirement

**Given** alpha's component 1 has `implements: [99]` and there is no requirement with id 99 in alpha.
**When** `CheckIDs(specDir)` is called.
**Then** one error with:
- Source: component 1 in alpha
- Dangling reference: requirement 99
- `message` indicating the target does not exist

#### I7: Component `uses` references non-existent component

**Given** alpha's component 1 has `uses: [50]` and no component 50 exists in alpha.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the dangling `uses` reference.

#### I8: test_section `describes` references non-existent component

**Given** alpha's test_section 1 has `describes: [77]` and no component 77 exists.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the dangling `describes` reference.

#### I9: Requirement `depends_on` references non-existent requirement

**Given** alpha's requirement 1 has `depends_on: [42]` and no requirement 42 exists in alpha.
**When** `CheckIDs(specDir)` is called.
**Then** one error for the dangling `depends_on` reference.

#### I10: Module `requires_module` references non-existent module

**Given** alpha has `requires_module: [99]` and no module with id 99 exists in `project.json`.
**When** `CheckIDs(specDir)` is called.
**Then** one error referencing the dangling module dependency.

#### I11: Project requirement `depends_on` references non-existent project requirement

**Given** `project.json` has a requirement whose `depends_on` names an id that no project-level requirement carries.
**When** `CheckIDs(specDir)` is called.
**Then** one error located at that requirement in `project.json`, reporting the dangling `depends_on` target. This is the project-level counterpart of I9, which covers the same field inside a module.

> This scenario replaced a milestone `groups` check. Milestones were retired from the spec and from `schema/project.schema.json`, whose root rejects unknown properties, so a `project.json` carrying a `milestones` array now fails the schema check outright and there is no reference for IDValidator to dangle.

#### I12: Requirement `preq_id` references non-existent project requirement

**Given** alpha's requirement 1 has `preq_id: 999` and no project-level requirement with id 999 exists.
**When** `CheckIDs(specDir)` is called.
**Then** one error for the dangling `preq_id` reference.

#### I13: Multiple dangling references in one module

**Given** alpha has a dangling `implements`, a dangling `uses`, and a dangling `describes` reference.
**When** `CheckIDs(specDir)` is called.
**Then** three errors, one per dangling reference. All are reported, not just the first.

#### I14: data_flow `uses` references non-existent component

**Given** alpha has a data_flow with `uses: [42]` and no component 42 exists.
**When** `CheckIDs(specDir)` is called.
**Then** one error for the dangling data_flow `uses` reference.

#### I18: Cross-reference integrity over profile-declared edges

**Given** a `spec/profile.json` declaring an `endpoint` type carrying a `serves` reference field pointing at components, and alpha's endpoint has `serves: [99]` with no component 99.
**When** `CheckIDs(specDir)` is called.
**Then** one error for the dangling `serves` reference. The reference fields a type may carry, and what each may point at, are the resolved profile's legal edges — the checks above are the default profile's edge set, and a declared edge is checked with the same set-membership machinery.

#### D9: Acyclicity over profile-declared edges

**Given** the same profile, where two endpoints reference each other through `serves`-style edges the profile declares without a `cyclic` flag.
**When** `CheckDAG(specDir)` is called.
**Then** one cycle error naming the two nodes. The DAG check enforces acyclicity over every edge kind the resolved profile declares that is not marked `cyclic: true`; under the default profile — which omits the flag on all seven edge kinds — the graphs that can actually cycle are exactly the three the scenarios above exercise, the other four passing vacuously.

In a further variant the profile marks the endpoints' edge `cyclic: true`: the same loop then validates with zero DAG errors — a `cyclic` edge is descriptive, exempt from the cycle check.

---

## Edge Cases

### E1: Module with empty arrays

**Given** alpha has `requirements: []`, `components: []`, `test_sections: []`.
**When** both checkers run.
**Then** DAGChecker: zero errors (no edges to form cycles). IDValidator: zero errors (no IDs to duplicate or reference).

### E2: Identical identity hash collision across different array types is impossible by construction

**Given** alpha has a requirement, component, and test_section all named `"Foo"`. Each gets a different identity hash because the type segment differs (`alpha/requirement/Foo` vs `alpha/component/Foo` vs `alpha/test_section/Foo`).
**When** `CheckIDs(specDir)` is called.
**Then** zero errors. The identity hash function takes the node type as a part, so two nodes with the same name in different array types always produce different hashes. Uniqueness is still checked per array (defense in depth against hand-edited collisions), but cross-array collisions cannot occur naturally.

### E3: Large graph performance

**Given** a project with 50 modules, each with 20 requirements and 10 components, forming a deep but acyclic dependency chain.
**When** `CheckDAG(specDir)` is called.
**Then** it completes in under 100ms and returns zero errors.

#### I15: Module requirement missing preq_id fails validation

**Given** alpha has a requirement with no `preq_id` field (preq_id is now required on every module requirement).
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the requirement in alpha as missing `preq_id`. The error message indicates that every module requirement must trace to a project requirement.

#### I16: Project requirement missing priority field

**Given** a project.json with a requirement that has no `priority` field.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the project requirement as missing `priority`. The error message indicates that every project requirement must have a priority (integer 0-4).

#### I17: Project requirement with out-of-range priority

**Given** a project.json with a requirement that has `priority: 5`.
**When** `CheckIDs(specDir)` is called.
**Then** one error identifying the invalid priority value. Priority must be 0-4.

---

