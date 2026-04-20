# Change Proposal: Data Flow Contract Layer and Hierarchy Bootstrap

## Context

The bead type hierarchy was designed in proposal 2026-03-15-deterministic-spec-bead-state-machine: modules produce `epic` beads, components produce `feature` beads, test_sections produce `task` beads. The code implements this in `beadType()` (apply/bead_creator.go:220) and the action gate (impact/action_classifier.go:71). `spex apply` has executed three times across three proposals, creating 37 beads.

Three problems have emerged:

**1. The hierarchy was never bootstrapped.** The bead-map contains 46 records — all for components. Zero epic beads for modules, zero task beads for test_sections, zero data_flow records. The 3 apply runs only processed component content hash changes because the initial spec was populated manually before `spex apply` existed. The hierarchy code works but has never created epics or test_section tasks because those spec nodes were never in the diff (they existed before the first snapshot).

**2. Data flows are excluded from bead creation but represent cross-component contracts.** The action classifier (impact/action_classifier.go:71) gates bead creation on `module`, `component`, and `test_section` node types. Data flows and impl_sections are explicitly skipped. The ImpactClassifier (merkle/impact_classifier.go) classifies data_flow changes as `impl_only`. But data flows describe the data shapes moving between components — when those shapes change, every participating component breaks. This was the root cause of the cascade during the identity-hash-ids proposal (2026-04-06): changing `Node.Module` from integer to string was a contract change to flow_hash_computation (data_flow 1, uses: Hasher, TreeBuilder, SnapshotStore) and flow_diff_classification (data_flow 2, uses: SnapshotStore, DiffEngine, ImpactClassifier). Three consecutive bead implementations (spexmachina-03l, spexmachina-e8t, spexmachina-kdb) each cascaded into other components' code, producing 184 TODO markers across the codebase.

**3. Single-component test_sections create unnecessary beads.** The spec says all test_sections produce task beads. In practice, the implement skill writes tests as part of the component's feature bead (TDD workflow). Of the 29 test_sections across all modules, 15 have a single entry in their `describes` array — these are unit/component tests naturally coupled with one component's work. The remaining 14 have multiple entries — these are cross-component integration tests that cannot be bundled into any single component bead and legitimately need their own beads.

These three problems are related: the bead-map is missing two layers of the hierarchy (epics and test tasks), and missing the contract layer (data flows) that would prevent cross-bead cascading. Fixing them together in one migration establishes the complete hierarchy before the remaining 16 open beads ship.

## Proposed change

### 1. Data flow content enrichment

Each data_flow content file (flow_*.md) gains a **Data Shapes** section describing the concrete data structures flowing between components in the `uses` array. Shapes are described in a language-agnostic format — field names, value types, and semantics — not in language-specific syntax.

Example for merkle flow_hash_computation.md (data_flow 1, uses: Hasher, TreeBuilder, SnapshotStore):

```
## Data Shapes

### Hasher → TreeBuilder
- hash: string, 64-character hex SHA-256 digest

### TreeBuilder → SnapshotStore
- Node: key (string, identity hash or "meta/" prefix for envelope leaves),
  hash (string, SHA-256 hex), type (string: "leaf"|"module"|"project"),
  node_type (string), module (string, identity hash of parent module,
  empty for project-level), children (list of Node)
```

When a proposal changes a data shape, the data_flow content hash changes, making the contract change visible as a distinct diff entry that triggers bead creation.

All 9 data_flows across 8 modules are affected: merkle (2), impact (1), apply (1), map (1), render (1), validator (1), cli (1), proposal (1).

### 2. Data flows become bead-producing nodes

The action classifier's gate (impact/action_classifier.go:71) changes from:

```
module, component, test_section → produce beads
impl_section, data_flow → skip
```

to:

```
module, component, data_flow, test_section → produce beads
impl_section → skip
```

The `beadType()` function (apply/bead_creator.go:220) adds:

```
data_flow → task
```

Data_flow beads are `task` type, parented under the module's epic bead. Component beads that participate in a data_flow (listed in the flow's `uses` array) gain a `depends` dependency on the data_flow's bead through the existing spec-graph dependency resolution (proposal 2026-03-20-spec-graph-bead-deps). This means the data_flow bead must be completed before any participating component bead can start — the existing bead dependency machinery handles ordering automatically.

Affected spec nodes:
- impact ActionClassifier (component 3): add data_flow to the gate
- apply BeadCreator (component 1): add data_flow → task mapping
- apply test_bead_actions.md: update test that asserts data_flows are skipped
- apply arch_bead_creator.md: update type mapping table

### 3. New impact classification for data_flow changes

The ImpactClassifier (merkle component 5, implements requirement 5) currently classifies `data_flow` as `impl_only`. Add a new impact level `contract` between `arch_impl` and `structural`:

| NodeType | Impact Level |
|----------|-------------|
| impl_section, test_section | impl_only |
| data_flow | contract |
| component | arch_impl |
| meta, requirement | structural |

The `contract` level signals a cross-component boundary change within the module. The NodeMatcher (impact component 2) already skips `structural` changes; it must also NOT skip `contract` changes — they need to reach the ActionClassifier to produce beads.

Affected spec nodes:
- merkle ImpactClassifier (component 5): add `contract` level
- merkle requirement 5 (Classify impact): update description
- merkle arch_impact_classifier.md, impl_impact_classification.md: update rules

### 4. Single-component test_sections coupled with component beads

The `describes` array length determines whether a test_section produces a bead:

- `len(describes) == 1`: no separate bead. Tests are bundled with the component's feature bead. The implement skill reads the test_section's content file as part of its TDD workflow.
- `len(describes) >= 2`: separate `task` bead. Cross-component integration tests cannot be bundled into a single component bead.

Project-level `test_plan.scenarios` (cross-module tests) always produce separate beads.

This changes the action classifier: for unmatched test_section nodes, check the `describes` array length from the module spec before creating an action. If single-component, skip (the component bead handles it).

Affected spec nodes:
- impact ActionClassifier (component 3): describes-length check for test_sections
- apply arch_bead_creator.md: document the coupling rule
- apply test_bead_actions.md: update test_section bead creation tests

### 5. Implement skill gains data_flow bead mode

The implement skill needs two behavioral modes based on whether the bead is linked to a data_flow or a component spec node.

**Data_flow bead mode** (bead linked to a data_flow spec node):
- Read the data_flow spec to understand what data shapes changed
- Update shared types and interfaces across all participating components
- Make everything compile
- Do not change component logic beyond what compilation requires
- Mark tests that need logic updates with deferred-work markers for the appropriate component beads
- Scope covers the entire set of participating components, but only for shared type signatures and compilation

**Component bead mode** (bead linked to a component spec node, depends on a completed data_flow bead):
- Shared types are already correct (data_flow bead handled that)
- Only touch files this component owns
- Remove deferred-work markers that point to this bead
- Update logic to work with the new data shapes
- Never touch other components' files

When no data_flow bead exists in the dependency chain (backward compatibility), the component bead mode falls back to the current behavior with scope boundary markers.

### 6. Epic lifecycle: per-proposal-wave progress tracking

Epics follow the same obsolete+create state machine as components. Each `spex apply` run that changes a module.json (the `meta/<module-hash>` leaf) obsoletes the old epic and creates a new one. Newly created component features from that same run are parented under the new epic. The old epic stays closed with its old (now obsoleted) children.

Epics are not for module grouping — module grouping is already available through labels and `spex` queries against the spec graph. Epics are for **proposal wave progress tracking**: they group exactly the work that one proposal created, and `br epic close-eligible` closes the epic when that wave's work is done. This gives a clear answer to "what's left from this proposal?" without overloading the epic concept as a permanent container.

### 7. Hierarchy bootstrap migration

A one-time migration performed during implementation to align the bead-map with the full hierarchy:

**Epic beads for modules:** For each of the 9 modules, create a closed epic bead and a bead-map record with `spec_node_id` = the module's identity hash, `bead_type` = `epic`. Reparent existing open feature beads under their module's epic via `br update <id> --parent <epic-id>`. These initial epics represent the baseline. The next `spex apply` run that touches a module will obsolete the baseline epic and create a fresh one with the new proposal's features parented under it.

**Data_flow bead-map records:** For all 9 data_flows, create bead-map records with `bead_type` = `task` and closed bead IDs (the current data shapes are already implemented).

**Multi-component test_section bead-map records:** For the 14 test_sections with `len(describes) >= 2`, create bead-map records with `bead_type` = `task` and closed bead IDs (the current tests are already implemented).

**Fix legacy bead_type values:** The 35 bead-map records with `bead_type` = `task` that are actually component records should be updated to `bead_type` = `feature` to match the hierarchy.

**New snapshot:** After all bead-map changes, run `spex hash` to capture the new baseline including data_flow content enrichment.

## Impact expectation

This proposal is deferred until after the current 16 open beads ship. When activated:

**New beads:**
- ImpactClassifier update (merkle component 5): add `contract` classification level
- ActionClassifier update (impact component 3): add data_flow gate, describes-length check for test_sections
- BeadCreator update (apply component 1): add data_flow → task mapping
- Data_flow content updates: all 9 data_flow files gain Data Shapes sections (batched by module, 2-3 beads)

**Modified spec nodes:**
- All 9 data_flow content files
- merkle requirement 5 description
- merkle arch_impact_classifier.md, impl_impact_classification.md
- impact ActionClassifier content
- apply BeadCreator content, test_bead_actions.md, arch_bead_creator.md

**Skill changes (not tracked as beads):**
- Implement skill: data_flow bead mode vs component bead mode

**Migration (not a separate bead, performed during implementation):**
- 9 epic bead-map records (modules)
- 9 data_flow bead-map records
- 14 multi-component test_section bead-map records
- 35 legacy bead_type corrections (task → feature)
- Reparent open feature beads under module epics

**No existing beads closed or modified.**

**Estimated scope:** 4-5 sessions:
- Session 1: Data_flow content enrichment (add Data Shapes to all 9 flow files)
- Session 2: Hierarchy bootstrap migration (epics, data_flows, test_sections, legacy fixes)
- Session 3: ImpactClassifier contract level
- Session 4: ActionClassifier + BeadCreator updates (data_flow gate, describes-length check)
- Session 5: Implement skill update + validation with simulated contract change
