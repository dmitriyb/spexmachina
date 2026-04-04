# Change Proposal: Structural Change Validation

## Context

The impact module's NodeMatcher treats `project/meta` and `module/X/meta` structural changes as triggers to obsolete and recreate beads for ALL matched records. When project.json changes (e.g., adding a requirement), the matcher matches every bead-map record in the system with `module: ""`, producing obsolete+create pairs for every component across all modules. When a module.json changes (e.g., adding a requirement), it matches all records in that module.

This is wrong. Beads should only be obsoleted and recreated when the actual component or content leaf changes. When requirements change, the expectation is that affected components will be updated too — and those component-level changes are already detected by merkle as separate `arch_impl` or `impl_only` changes on their own. The `meta` node is a structural signal that the JSON envelope changed (a requirement was added, a dependency was modified, etc.), but bead impact comes from the leaf-level changes that follow — not from `meta` itself.

Rather than silently skipping structural changes, the fix is to **validate them at every pipeline stage** so that by the time impact runs, structural consistency is already guaranteed and only leaf-level changes need to produce bead actions.

### Three-stage validation

Each pipeline stage gets a clear responsibility for structural changes:

**`spex validate` — state checks.** The spec must be structurally complete right now:
- Every requirement is implemented by at least one component (`implements` edge)
- Every project requirement is derived into at least one module requirement (`preq_id` edge)
- Every component has a content leaf (file exists)
- No orphaned references (component implements a requirement ID that doesn't exist, module requires a module ID that doesn't exist, etc.)

These checks catch additions and deletions that aren't wired up. If you added a requirement but no component implements it, validate fails. If you deleted a requirement but a component still references it, validate fails.

**`spex diff` — change completeness checks.** The diff must be internally consistent:
- If a requirement's text changed, at least one of its implementing components' content leaves must also have changed in the same diff
- If a component's `implements` or `uses` edges changed (meta), the component's content leaf should also have changed
- Errors are included in the diff output alongside the normal change entries

These checks catch incomplete edits. If you changed a requirement but forgot to update the implementing component, diff reports an error.

**`spex impact` — refuses to proceed if diff contains errors.** If the diff output includes errors from the change completeness checks, impact prints "fix the errors first" and exits non-zero. If the diff is clean, impact only processes leaf-level changes (`arch_impl`, `impl_only`). Structural (`meta`) changes are not matched against bead-map records — they've already been validated by the first two stages.

## Proposed change

### 1. Validator module changes

In `spec/validator/module.json`:

- Add requirement (next ID): "Validate requirement coverage" (`preq_id: 1`). Every project requirement must have at least one module requirement with matching `preq_id`. Every module requirement must have at least one component with matching `implements`. Errors if uncovered.
- Add or extend a component to implement the requirement coverage check (e.g., RequirementCoverageChecker, or extend an existing checker).
- Update content leaves to describe the new validation rules.

Note: some state checks already exist (content path resolution, test coverage, ID uniqueness, orphan detection). The new checks extend the existing validation with requirement-to-component coverage.

### 2. Merkle/diff module changes

In `spec/merkle/module.json`:

- Add requirement (next ID): "Change completeness validation in diff" (`preq_id: 2`). When a requirement's description changes (structural meta), verify that at least one implementing component's content leaf also changed in the same diff. When a component's edges change (implements, uses), verify the component's content leaf also changed. Include errors in the diff JSON output.
- Update DiffCommand component to include an `errors` array in the JSON output alongside `changes`.
- Update content leaves to describe the change completeness logic.

The diff output format becomes:
```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "Requirement 4 (impact) description changed but implementing component NodeMatcher content leaf unchanged",
      "path": "module/4/meta",
      "related": ["module/4/component/2"]
    }
  ]
}
```

### 3. Impact module changes

In `spec/impact/module.json`:

- Update NodeMatcher component (component 2): structural changes (`impact: "structural"`) are not matched against bead-map records. They produce no matches, no unmatched entries, no orphans. Only `arch_impl` and `impl_only` changes go through matching.
- Update ImpactCommand component (component 5): if the diff input contains a non-empty `errors` array, refuse to proceed — print the errors and exit non-zero.
- Update content leaves (`arch_node_matcher.md`, `impl_node_matching.md`) to describe the corrected behavior.
- Update test content leaf (`test_bead_matching.md`): fix S6 (structural changes produce zero matches), add scenarios for error rejection.

### 4. Code changes

**`impact/node_matcher.go`:**
- Remove the structural change branch that matches all records (lines 50-56)
- Skip changes with `impact == Structural` — they produce no matches

**`impact/node_matcher_test.go`:**
- Fix `TestFR2_ProjectMetaMatchesAllRecords` → assert zero matches
- Fix `TestFR2_StructuralChangeMatchesAllModuleRecords` → assert zero matches
- Add test: structural change coexists with leaf-level changes — leaf changes still match correctly

**`cmd/spex/impact.go`:**
- Parse `errors` array from diff JSON input
- If non-empty, print errors to stderr and exit 1

**`cmd/spex/diff.go`:**
- After computing changes, run change completeness checks
- Include errors in JSON output

**Validator (new checker):**
- Implement requirement coverage validation

## Impact expectation

This proposal touches three spec modules (validator, merkle, impact).

**Validator module:**
- New requirement + new or extended component for requirement coverage checking
- New impl and test content leaves

**Merkle module:**
- New requirement for change completeness validation
- DiffCommand component content updated
- DiffEngine or new component for completeness checking
- Updated impl and test content leaves

**Impact module:**
- NodeMatcher component content updated (no new requirement — existing req 2 "match changed nodes to beads" covers the corrected behavior)
- ImpactCommand component content updated (error rejection)
- Updated impl and test content leaves

**Estimated scope:** 3-4 sessions:
- Session 1: Impact fix (NodeMatcher skip structural, error rejection) — unblocks the pipeline
- Session 2: Validator requirement coverage checker
- Session 3: Diff change completeness checks
- Session 4: Integration testing across the pipeline
