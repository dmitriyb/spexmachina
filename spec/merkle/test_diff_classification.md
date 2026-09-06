# Diff and Classification Tests

Integration and acceptance tests for the DiffEngine (component 4), ImpactClassifier (component 5), and CompletenessChecker (component 8). Validates that tree comparison correctly identifies added, removed, and modified nodes (including requirement leaf nodes), that impact classification assigns the right level based on node type and module aggregation, and that the completeness checker catches incomplete spec edits.

## Setup

This leaf's scenarios — S14 and the Diff E-cases — are fixtured as follows.

The listing below is the abstract leaf shape the scenarios reason about, with placeholder keys standing in for identity hashes — not a transcript of `setupSpecDir`, which declares two project requirements, two alpha requirements, two alpha components, one alpha test_section and one beta component, and no data_flow.

**Snapshot tree** (the "before" state):
```
meta/project             hash=pj1  type=leaf  node_type=meta
meta/<ALPHA_HASH>        hash=am1  type=leaf  node_type=meta
COMP1_HASH               hash=aw1  type=leaf  node_type=component
TEST1_HASH               hash=al1  type=leaf  node_type=test_section
meta/<BETA_HASH>         hash=bm1  type=leaf  node_type=meta
COMP2_HASH               hash=bs1  type=leaf  node_type=component
TEST2_HASH               hash=bh1  type=leaf  node_type=test_section
```

**Current tree** is cloned from the snapshot and then selectively mutated per scenario.

## Scenarios

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

## Edge Cases

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

## Requirement Leaf Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.

## Completeness Checker Scenarios

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
