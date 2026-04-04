# Change Proposal: Impact Structural Change Cascade Fix

## Context

The impact module's NodeMatcher treats `project/meta` and `module/X/meta` structural changes as triggers to obsolete and recreate beads for ALL matched records. When project.json changes (e.g., adding a requirement), the matcher matches every bead-map record in the system with `module: ""`, producing obsolete+create pairs for every component across all modules. When a module.json changes (e.g., adding a requirement), it matches all records in that module.

This is wrong. Beads should only be obsoleted and recreated when the actual component or content leaf changes. When requirements change, the expectation is that affected components will be updated too — and those component-level changes are already detected by merkle as separate `arch_impl` or `impl_only` changes on their own. The `meta` node is a structural signal that the JSON envelope changed (a requirement was added, a dependency was modified, etc.), but bead impact comes from the leaf-level changes that follow — not from `meta` itself.

The current behavior:
- `project/meta` structural change → matches ALL records → obsolete+create for every bead
- `module/X/meta` structural change → matches all records in module → obsolete+create for every component

The correct behavior:
- `project/meta` structural change → no bead actions (informational only)
- `module/X/meta` structural change → no bead actions (informational only)
- `module/X/component/Y` arch_impl change → obsolete+create for that component's bead (already works)
- `module/X/impl_section/Z` impl_only change → obsolete+create for that section's bead (already works)

The test `TestFR2_ProjectMetaMatchesAllRecords` explicitly asserts the wrong behavior. The spec content leaf for NodeMatcher (`spec/impact/arch_node_matcher.md`) needs to be updated to describe the correct behavior.

## Proposed change

### 1. Impact module spec changes

In `spec/impact/module.json`:

- Update the NodeMatcher component's content leaf (`arch_node_matcher.md`) to specify: structural changes at `project/meta` and `module/X/meta` level produce no bead actions. Only leaf-level changes (component, impl_section, test_section, data_flow) with `arch_impl` or `impl_only` impact trigger matching against bead-map records.

### 2. Code changes

In `impact/node_matcher.go`:

- Remove the structural change branch that matches all records when `module == ""` (project-level) or all module records when `module != ""` (module-level)
- Structural (`meta`) changes should be skipped — they produce no matches, no unmatched, no orphans
- Only `arch_impl` and `impl_only` changes (which have specific paths like `module/X/component/Y`) go through the matching logic

### 3. Test changes

In `impact/node_matcher_test.go`:

- Fix `TestFR2_ProjectMetaMatchesAllRecords` — rename and update to assert that project/meta changes produce zero matches
- Fix `TestFR2_StructuralChangeMatchesAllModuleRecords` — rename and update to assert that module/meta changes produce zero matches
- Add test: structural change coexists with leaf-level changes — the leaf-level changes still produce correct matches

## Impact expectation

This touches one component (NodeMatcher) in the impact module. After manual bead simulation:

**Modified beads:**
- NodeMatcher component bead — spec content updated, code changed

**New beads:** None.

**Closed beads:** None.

Estimated scope: 1 session. Update spec content leaf, fix code, fix tests.
