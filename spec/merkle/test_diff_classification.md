# Diff and Classification Tests

Integration and acceptance tests for the DiffEngine (component 4), ImpactClassifier (component 5), and CompletenessChecker (component 8). Validates that tree comparison correctly identifies added, removed, and modified nodes (including requirement leaf nodes), that impact classification assigns the right level based on node type and module aggregation, and that the completeness checker catches incomplete spec edits.

## Setup

This leaf covers three test files that fixture themselves differently, and the distinction matters when reading the scenarios below.

- The **Diff** scenarios (S1–S5, E-cases) are backed by `merkle/diff_engine_test.go`, which writes a real spec directory with `setupSpecDir(t)` and produces both trees from it with `BuildTree`, so the keys under test are real identity hashes. `findChild(t, parent *Node, key string) *Node` locates a child by key; changes are located by inline loops over the returned slice.
- The **Classify** scenarios (S7–S13, R1–R2) are backed by `merkle/impact_classifier_test.go`, which passes synthetic `[]Change` literals straight into `Classify` and builds no tree for them. That is why they can exercise a `data_flow` node type, which `setupSpecDir` does not declare. `Classify` takes the resolved profile as its required third argument; in the call shapes below `defaultProfile` stands for `schema.DefaultProfile()`, and every scenario passes it unless the scenario says otherwise (S15 is the one that does). The one test in that file that does build trees is S14, the end-to-end `Diff`-then-`Classify` pairing (`merkle/impact_classifier_test.go:175`). R3–R5 are Diff scenarios, not Classify ones.
- The **Completeness** scenarios (C1–C11) are backed by `merkle/completeness_checker_test.go`, whose `TestREQ8_C1`…`TestREQ8_C11` functions carry them one-for-one.

The listing below is the abstract leaf shape the scenarios reason about, with placeholder keys standing in for identity hashes — not a transcript of `setupSpecDir`, which declares two project requirements, two alpha requirements, two alpha components, one alpha test_section and one beta component, and no data_flow.

**Snapshot tree** (the "before" state):
```
meta/project             hash=pj1  type=leaf  node_type=meta
meta/<ALPHA_HASH>        hash=am1  type=leaf  node_type=meta
COMP1_HASH               hash=aw1  type=leaf  node_type=component
TEST1_HASH               hash=al1  type=leaf  node_type=test_section
FLOW1_HASH               hash=af1  type=leaf  node_type=data_flow
meta/<BETA_HASH>         hash=bm1  type=leaf  node_type=meta
COMP2_HASH               hash=bs1  type=leaf  node_type=component
TEST2_HASH               hash=bh1  type=leaf  node_type=test_section
```

**Current tree** is cloned from the snapshot and then selectively mutated per scenario.

## Scenarios

### S1: No changes — empty diff

**Given** the current tree is identical to the snapshot tree (all hashes match)
**When** `Diff(current, snapshot)` is called
**Then** the result is an empty slice (no changes)

**Rationale**: Baseline correctness — when nothing changed, no changes should be reported. This validates the map comparison logic handles the "all keys present, all hashes equal" case.

### S2: Single leaf modified

**Given** the current tree has `TEST1_HASH` with `hash=al2` (changed from `al1`)
**When** `Diff(current, snapshot)` is called
**Then** the result contains exactly one change
**And** that change has Key=`TEST1_HASH`, Type=`modified`, OldHash=`al1`, NewHash=`al2`

**Rationale**: The simplest mutation case. Validates that DiffEngine detects a hash difference for an existing leaf.

### S3: New leaf added

**Given** the current tree has all snapshot nodes plus a new component leaf `COMP3_HASH` with `hash=ag1`
**When** `Diff(current, snapshot)` is called
**Then** the result contains one change with Key=`COMP3_HASH`, Type=`added`, OldHash=`""`, NewHash=`ag1`

**Rationale**: Validates the set-difference logic: keys in current but not in snapshot are reported as added.

### S4: Leaf removed

**Given** the current tree is the snapshot tree minus `TEST2_HASH`
**When** `Diff(current, snapshot)` is called
**Then** the result contains one change with Key=`TEST2_HASH`, Type=`removed`, OldHash=`bh1`, NewHash=`""`

**Rationale**: Validates the reverse set-difference: keys in snapshot but not in current are reported as removed.

### S5: Multiple changes across modules

**Given** the current tree has:
- `TEST1_HASH` modified (hash changed)
- `COMP3_HASH` added
- `COMP2_HASH` removed
**When** `Diff(current, snapshot)` is called
**Then** the result contains exactly 3 changes
**And** changes are sorted ascending by key, whatever hex order the three identity hashes fall into

**Rationale**: Validates that DiffEngine handles mixed change types across multiple modules and returns results sorted by key for deterministic output (per `arch_diff_engine.md`).

### S6: First diff with no snapshot (all nodes added)

**Given** the snapshot is an empty tree (no nodes)
**And** the current tree has the full base fixture
**When** `Diff(current, snapshot)` is called
**Then** every leaf in the current tree appears as Type=`added`
**And** the number of changes equals the number of leaves in the current tree

**Rationale**: Per `arch_diff_engine.md`, the first run diffs against the empty tree and reports everything as added. The empty tree reaches the engine as an ordinary input — on a real project it is the snapshot `spex init` seeded, loaded like any other; the engine itself has no notion of a "missing" baseline, and `SnapshotStore.Load` no longer invents one.

### S7: Classify impl_only change

**Given** changes: `[{Key: TEST1_HASH, Type: "modified", NodeType: "test_section", Module: ALPHA_HASH}]` and `moduleNames = {ALPHA_HASH: "Alpha"}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`impl_only` and Module=`"Alpha"`

**Rationale**: Classification reads the node metadata (NodeType, Module) carried on each Change — never the filename or path. `test_section` is the one node type that reaches `impl_only` per `arch_impact_classifier.md`.

### S8: Classify data_flow as contract

**Given** changes: `[{Key: FLOW1_HASH, Type: "modified", NodeType: "data_flow", Module: ALPHA_HASH}]` and `moduleNames = {ALPHA_HASH: "Alpha"}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`contract` and Module=`"Alpha"`

**Rationale**: Per the data-flow-contract-layer proposal (and requirement `425146f32e96`), data_flow nodes are inter-component contracts: a changed flow can invalidate its consumers, so it ranks above `impl_only` while below `arch_impl`.

### S9: Classify arch_impl change

**Given** changes: `[{Key: COMP1_HASH, Type: "modified", NodeType: "component", Module: BETA_HASH}]` and `moduleNames = {BETA_HASH: "Beta"}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`arch_impl` and Module=`"Beta"`

**Rationale**: `component` nodes are architecture contracts whose changes may affect dependent modules.

### S10: Classify structural change — module envelope

**Given** changes: `[{Key: "meta/" + ALPHA_HASH, Type: "modified", NodeType: "meta", Module: ALPHA_HASH}]` and `moduleNames = {ALPHA_HASH: "Alpha"}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`"Alpha"`

**Rationale**: A modified `meta/<module-hash>` envelope means module.json itself changed — the spec graph was altered (added/removed nodes, changed edges).

### S11: Classify structural change — project envelope

**Given** changes: `[{Key: "meta/project", Type: "modified", NodeType: "meta", Module: ""}]`
**When** `Classify(changes, nil, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`""` (project-level, no specific module)

**Rationale**: `project.json` changes affect the entire project structure (modules added or removed).

### S12: Per-change classification — mixed impacts in one module

**Given** changes within the same module alpha:
- `{Key: TEST1_HASH, Type: "modified", NodeType: "test_section", Module: ALPHA_HASH}`
- `{Key: COMP2_HASH, Type: "modified", NodeType: "component", Module: ALPHA_HASH}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called with `moduleNames = {ALPHA_HASH: "Alpha"}`
**Then** both changes carry Module=`"Alpha"`
**And** the test_section change is classified `impl_only` and the component change `arch_impl` — one classification per change, no aggregation

**Rationale**: The merkle module only classifies individual changes; per-module propagation belongs to the downstream plan module. If a caller needs the module's max impact it computes it locally over the per-change results using the full order `structural > arch_impl > contract > impl_only`.

### S13: Per-change classification — structural present alongside lower impacts

**Given** changes within module alpha:
- `{Key: TEST1_HASH, Type: "modified", NodeType: "test_section", Module: ALPHA_HASH}`
- `{Key: COMP2_HASH, Type: "modified", NodeType: "component", Module: ALPHA_HASH}`
- `{Key: "meta/" + ALPHA_HASH, Type: "modified", NodeType: "meta", Module: ALPHA_HASH}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called with `moduleNames = {ALPHA_HASH: "Alpha"}`
**Then** each change keeps its own level (`impl_only`, `arch_impl`, `structural` respectively)
**And** a locally computed per-module max over those results is `structural`

**Rationale**: Structural is the highest impact level in the order `structural > arch_impl > contract > impl_only`; the classifier itself still reports per-change.

### S14: Diff then Classify end-to-end integration

**Given** a snapshot tree and a current tree where:
- alpha's test_section leaf (`TEST1_HASH`) hash changed
- beta's component leaf (`COMP1_HASH`) hash changed
- a new module gamma appears, adding its `meta/<GAMMA_HASH>` envelope leaf
**When** `Diff(current, snapshot)` is called, then `Classify(changes, ModuleNames(current), defaultProfile)` is called on the result
**Then** the test_section change is classified as `impl_only`
**And** the component change is classified as `arch_impl`
**And** gamma's envelope change is classified as `structural`

**Rationale**: Full pipeline integration: DiffEngine produces raw changes carrying node metadata (Key, NodeType, Module), ImpactClassifier annotates them from that metadata. This is the exact data path described in `flow_diff_classification.md`.

### S15: Impact levels and completeness triggers follow the profile's declarations

**Given** the classify fixtures above, run once with `defaultProfile` and once with a resolved profile declaring an `endpoint` type mapped to the `contract` level (the shape a project's `spec/profile.json` would resolve to)
**When** `Classify` is called with that profile as its third argument on a change carrying `NodeType: "endpoint"`
**Then** the change is classified `contract` — the mapping from declared node type to impact level is read from the profile handed in; the four levels themselves are fixed, `meta`'s `structural` classification is the frame's fixed rule, and under the default profile S7–S13 and R1–R2 hold byte-for-byte because the default assigns today's types to today's levels
**And** the completeness scenarios (C1–C11) likewise hold unchanged under the default profile, because the requirement-leaf trigger is a per-type role flag the default profile marks on requirement, read by CompletenessChecker rather than compiled in — while the meta-envelope sweep runs on the fixed meta leaves under any profile

**Rationale**: The classifier and the completeness checker branch on the profile's declarations — plus the frame's fixed meta rules — never on hard-coded declared-type names, and the default profile is the golden record of the previous constants.

### S16: Undeclared node type classifies as unknown

**Given** changes: `[{Key: FLOW1_HASH, Type: "removed", NodeType: "endpoint", Module: ALPHA_HASH}]` and `moduleNames = {ALPHA_HASH: "Alpha"}` — a removed leaf takes its node type from the snapshot side verbatim, so a type outside the resolved profile's declarations is reachable
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`unknown` and Module=`"Alpha"` — no level from the profile's rules, and no failure

**Rationale**: Per `arch_impact_classifier.md`, a node_type the resolved profile does not declare (and that is not `meta`) gets no level at all and is reported as `unknown`; the classifier still has no failure mode.

## Edge Cases

### E1: Diff with identical trees returns empty, Classify with empty returns empty

**Given** an empty changes slice
**When** `Classify([]Change{}, nil, defaultProfile)` is called — and, as `TestREQ5_Classify_EmptyChanges` also covers, `Classify(nil, nil, defaultProfile)`
**Then** the result is an empty slice

**Rationale**: No changes means no impact. Classify must not panic or inject synthetic entries.

### E2: Added leaf in new module

**Given** the snapshot has modules alpha and beta, current tree adds a new module gamma with its `meta/<GAMMA_HASH>` envelope leaf and a component leaf
**When** `Diff` then `Classify` are called
**Then** gamma's envelope change is classified as `structural`
**And** gamma's component change is classified as `arch_impl` with Module=`"gamma"`

**Rationale**: A brand-new module means both structural changes (new module.json) and arch changes (new component files). The module-level aggregate should be `structural`.

### E3: Removed entire module

**Given** the snapshot has alpha and beta, current tree has only alpha (beta entirely removed)
**When** `Diff` then `Classify` are called
**Then** all of beta's nodes appear as `removed` changes
**And** the removal of beta's `meta/<BETA_HASH>` envelope leaf is classified as `structural`

### E4: Node renamed (appears as add + remove pair)

**Given** the snapshot has component `Comp1` keyed `COMP1_HASH`, and the current tree has the same content under the name `Component1`, keyed `COMP1B_HASH`
**When** `Diff(current, snapshot)` is called
**Then** it reports two changes: `COMP1_HASH` removed and `COMP1B_HASH` added, with equal `OldHash`/`NewHash` content digests
**And** DiffEngine does not attempt rename detection (it compares keys, not content)

**Rationale**: Rename detection is out of scope for the merkle module. The diff is purely key-and-hash based.

### E5: Deterministic ordering across runs

**Given** a diff with changes across the two-module fixture
**When** `Diff` is called twice with the same inputs
**Then** both change lists are element-for-element identical and sorted ascending by key

**Rationale**: Per `arch_diff_engine.md`, deterministic output ordering is a hard requirement. No randomness or map iteration order leakage.

## Requirement Leaf Scenarios

In the scenarios below, identifiers like `REQ1_HASH`, `COMP1_HASH`, `ALPHA_HASH`, etc. are placeholder constants that the test fixture computes via `schema.IdentityHash` at setup time. They stand in for actual 12-character hex strings so the test descriptions stay readable.

### R1: Classify requirement change as structural

**Given** changes: `[{Key: REQ2_HASH, Type: "modified", NodeType: "requirement", Module: ALPHA_HASH}]` and `moduleNames = {ALPHA_HASH: "Alpha"}`
**When** `Classify(changes, moduleNames, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`"Alpha"`

**Rationale**: Requirement changes are structural signals — they indicate the spec contract changed. The NodeMatcher (plan module) skips structural changes, so requirement leaf changes do not produce bead actions.

### R2: Classify project requirement change as structural

**Given** changes: `[{Key: PROJ_REQ1_HASH, Type: "modified", NodeType: "requirement", Module: ""}]`
**When** `Classify(changes, nil, defaultProfile)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`""`

### R3: Requirement leaf added in diff

**Given** a snapshot tree with module alpha containing one requirement, and a current tree with one additional requirement (a new identity hash, `REQ2_HASH`)
**When** `Diff(current, snapshot)` is called
**Then** `REQ2_HASH` appears as Type=`added`

### R4: Requirement leaf removed in diff

**Given** a snapshot tree with module alpha containing two requirements, and a current tree with only the first
**When** `Diff(current, snapshot)` is called
**Then** the removed requirement's identity hash (`REQ2_HASH`) appears as Type=`removed`

### R5: Requirement description modified in diff

**Given** a snapshot tree where requirement `REQ1_HASH` has description "original", and a current tree where the same requirement (same identity hash, because the title is unchanged) has description "updated"
**When** `Diff(current, snapshot)` is called
**Then** `REQ1_HASH` appears as Type=`modified` with different OldHash and NewHash

## Completeness Checker Scenarios

### C1: Modified requirement with component leaf changed — no error

**Given** a diff where `REQ1_HASH` is modified and `COMP1_HASH` (whose `implements` array contains `REQ1_HASH`) is also modified
**When** `CheckCompleteness(changes, specDir)` is called
**Then** no errors are returned

### C2: Modified requirement without component leaf changed — error

**Given** a diff where `REQ1_HASH` is modified but `COMP1_HASH` (which implements it) is NOT in the diff
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned with type `"incomplete_change"`, path `REQ1_HASH`, and related `[COMP1_HASH]`

### C3: Added requirement with no implementing component — error

**Given** a diff where `REQ3_HASH` is added, but no component in alpha has `REQ3_HASH` in its `implements` array
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned indicating the new requirement is not implemented

### C4: Added requirement with implementing component leaf unchanged — error

**Given** a diff where `REQ3_HASH` is added, `COMP2_HASH` implements it, but `COMP2_HASH` is NOT in the diff
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned for the implementing component's unchanged content leaf

### C5: Removed requirement still referenced by component — error

**Given** a diff where `REQ2_HASH` is removed, but `COMP1_HASH` in the current module.json still has `REQ2_HASH` in its `implements` array
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned indicating `COMP1_HASH` still implements the removed requirement

### C6: Project requirement changed with no module requirement deriving from it — error

**Given** a diff where `PROJ_REQ5_HASH` is modified, but no module requirement has `preq_id == PROJ_REQ5_HASH`
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned indicating no module requirement derives from the changed project requirement

### C7: Project requirement changed, module requirement exists, component leaf unchanged — error

**Given** a diff where `PROJ_REQ1_HASH` is modified, module requirement `MOD_REQ2_HASH` has `preq_id == PROJ_REQ1_HASH`, `COMP3_HASH` implements `MOD_REQ2_HASH`, but `COMP3_HASH` is NOT in the diff
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned for the implementing component's unchanged content leaf

### C8: Project requirement changed, full chain complete — no error

**Given** a diff where `PROJ_REQ1_HASH` is modified, `MOD_REQ2_HASH` derives from it, `COMP3_HASH` implements `MOD_REQ2_HASH`, and `COMP3_HASH` IS in the diff
**When** `CheckCompleteness(changes, specDir)` is called
**Then** no errors are returned

### C9: Meta changed without requirement changes — component edge check

**Given** a diff where `meta/<ALPHA_HASH>` is modified but no requirement nodes in module alpha changed, and no component content leaves changed
**When** `CheckCompleteness(changes, specDir)` is called
**Then** `DiffError`s are returned for each component whose content leaf did not change

### C10: Multiple requirements changed, partial coverage — errors for uncovered

**Given** a diff where two requirements in alpha (`REQ1_HASH`, `REQ2_HASH`) are both modified. `COMP_A_HASH` implements `REQ1_HASH` and its leaf changed. `COMP_B_HASH` implements `REQ2_HASH` and its leaf did NOT change.
**When** `CheckCompleteness(changes, specDir)` is called
**Then** one `DiffError` is returned for `COMP_B_HASH` only — `COMP_A_HASH` is covered

### C11: No structural or requirement changes — no errors

**Given** a diff with only `impl_only` and `arch_impl` changes (no requirement or meta changes)
**When** `CheckCompleteness(changes, specDir)` is called
**Then** no errors are returned
