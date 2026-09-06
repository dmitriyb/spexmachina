# Hashing Tests

Integration and acceptance tests for the Hasher (component 1) and TreeBuilder (component 2). Validates that SHA-256 leaf hashing, sorted interior hashing, and full tree construction are correct and deterministic.

## Setup

All scenarios operate against a temporary spec directory created in `t.TempDir()`. The fixture layout mirrors a minimal valid spec:

```
tmpdir/
  project.json          (two project requirements and two modules, "Alpha"
                         at path alpha and "Beta" at path beta, each with a
                         12-char hex identity id)
  alpha/
    module.json          (2 requirements, 2 components, 1 test_section —
                          each carrying a 12-char hex identity `id`, and the
                          components and test_section additionally a content
                          path; requirement leaves hash their JSON fields.
                          Leaf keys are read from those ids)
    arch_comp1.md        ("# Comp1 architecture")
    arch_comp2.md        ("# Comp2 architecture")
    test_comp1.md        ("# Comp1 tests")
  beta/
    module.json          (1 component)
    arch_beta.md         ("# Beta architecture")
```


## Scenarios

### S6: BuildTree hashes propagate bottom-up

**Given** the full fixture directory
**When** `BuildTree(tmpdir)` is called
**Then** each leaf node's hash matches `HashFile` of its corresponding file
**And** the `alpha` module interior hash equals `HashChildren` over the meta envelope leaf hash plus each spec-node leaf hash directly — no intermediate group hashes
**And** the root hash equals `HashChildren` over its five children's hashes — the project envelope leaf, the two project requirement leaves, and the `alpha` and `beta` module hashes

**Rationale**: End-to-end verification that the bottom-up hash propagation produces a consistent merkle tree. This is the core correctness property of the entire module.

### S8: Multi-module tree construction

**Given** a fixture with two modules `alpha` and `beta`, each with distinct content files
**When** `BuildTree(tmpdir)` is called
**Then** the root has children: `meta/project`, one leaf per project requirement, `alpha` and `beta`
**And** `alpha` and `beta` each have their own correct subtrees, and editing a leaf under `alpha` leaves `beta`'s module hash unchanged
**And** the root hash equals `HashChildren` over all of those child hashes

**Rationale**: Ensures the tree builder correctly handles the real-world case of multiple modules in a project, and that module hashes are combined in sorted order at the root level.

### S10: Hashed serialization derives from the resolved profile's field declarations

**Given** the fixture directory, built once under the default profile and once under a `spec/profile.json` byte-identical to the default declaration
**When** `BuildTree` is called on each
**Then** every leaf hash and the root are identical across the two trees — the serialization TreeBuilder puts each JSON-backed leaf through is derived from its type's field declarations (every declared field unless `hashed: false`, plus the envelope fields), and the default profile's declarations reproduce the retired per-type allowlists exactly
**And** building this repository's own spec under the default profile reproduces every identity hash the snapshot records, and — once the title-to-name adoption rename is refreshed into the snapshot — every leaf hash as well

**Rationale**: The acceptance criterion that no existing hash moves when the node shape becomes data. The hash inputs — identity strings and the declaration-derived field sets — are unchanged under the default profile, so an implementation that derived either from the profile incorrectly surfaces here as a moved hash. Hasher itself is deliberately untouched: file, byte and child hashing are type-agnostic and read nothing from the profile.

### S11: A profile-declared type gets a leaf like any built-in type

**Given** a profile declaring an `endpoint` type (content-bearing, module-scoped) and a fixture module carrying one endpoint with a content file
**When** `BuildTree` is called
**Then** the module node holds one leaf for the endpoint, keyed by its identity hash, hashed from its content file — a content-bearing type hashes from file bytes alone, like every built-in content-bearing type — and the tree shape (project root, module interiors, identity-hash-keyed leaves, the synthetic meta leaf) is fixed: the profile only decides what leaves exist.

### S12: A field declared hashed: false does not move the leaf

**Given** a profile declaring an `endpoint` type (module-scoped, no content leaf) with a required `protocol` text field and an optional `note` text field declared `hashed: false`, and two fixture builds identical except for the `note` value on one endpoint
**When** `BuildTree` is called on each
**Then** the endpoint's leaf hash is identical across the two builds, while editing `protocol` instead moves it
**And** the `meta/<module-hash>` envelope leaf differs in both cases — the envelope is hashed from `module.json`'s literal bytes

**Rationale**: Hash participation is the field declaration's own flag, not a compiled-in exclusion, so a declared type gets the same still-leaf guarantee `derivation` gets today.

## Edge Cases

No module-level scenarios remain in this section; the case-level checks that were here live in Go `_test.go` files beside the component.
