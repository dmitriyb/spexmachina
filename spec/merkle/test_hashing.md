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

Helper function `writeFile(t, dir, name, content)` writes one file into a directory. Expected digests are computed inline with `sha256.Sum256` and `hex.EncodeToString`; where this document writes `sha256Hex(x)` it means that pair.

## Scenarios

### S1: Leaf hash matches independent SHA-256

**Given** a file `arch_widget.md` with content `"# Widget\nHandles widgets."`
**When** `HashFile(path)` is called
**Then** the returned hex string equals `sha256Hex("# Widget\nHandles widgets.")`
**And** the hex string is exactly 64 characters long

**Rationale**: Confirms that `HashFile` reads the file and computes the correct SHA-256 digest — the foundational operation for the entire merkle tree.

### S2: HashFile streams content rather than buffering

**Given** a file of ordinary size
**When** `HashFile(path)` is called
**Then** it returns a valid 64-character hex hash without error
**And** the hash matches `sha256Hex` of the same content. `HashFile` streams via `io.Copy` (`merkle/hasher.go:21`); no test exercises a large file.

**Rationale**: Verifies the streaming implementation handles large files without memory issues.

### S3: Interior hash sorts children before concatenation

**Given** child hashes `["cccc", "aaaa", "bbbb"]`
**When** `HashChildren(childHashes)` is called
**Then** the result equals `sha256Hex("aaaabbbbcccc")` (sorted order)
**And** calling `HashChildren(["aaaa", "bbbb", "cccc"])` produces the same result

**Rationale**: Validates the determinism guarantee from `arch_hasher.md` — child discovery order must not affect the interior hash.

### S4: Interior hash with single child

**Given** child hashes `["abcdef1234"]`
**When** `HashChildren(childHashes)` is called
**Then** the result equals `sha256Hex("abcdef1234")`

**Rationale**: Boundary condition for modules with a single content file. Sorting a one-element slice is a no-op and the concatenation is trivial.

### S5: BuildTree produces correct tree structure

**Given** the full fixture directory described in Setup
**When** `BuildTree(tmpdir)` is called
**Then** the root node has type `"project"` and five children: the `meta/project` envelope leaf, one leaf per project requirement (two), and the `alpha` and `beta` module nodes (each keyed by its identity hash)
**And** the `alpha` module node's children are flat leaves — the `meta/<alpha-module-hash>` envelope leaf plus one leaf per spec node (the two requirements, the two components, and the test_section), each keyed directly by its 12-char hex identity `id`
**And** there are no per-type `arch`/`flow`/`test` interior group nodes and no path-style keys anywhere in the tree

**Rationale**: Validates the flat identity-hash tree structure from `arch_tree_builder.md`: `project.json` discovery, module enumeration, and one leaf per module.json-declared spec node. The earlier path-style grouped scheme was deleted.

### S6: BuildTree hashes propagate bottom-up

**Given** the full fixture directory
**When** `BuildTree(tmpdir)` is called
**Then** each leaf node's hash matches `HashFile` of its corresponding file
**And** the `alpha` module interior hash equals `HashChildren` over the meta envelope leaf hash plus each spec-node leaf hash directly — no intermediate group hashes
**And** the root hash equals `HashChildren` over its five children's hashes — the project envelope leaf, the two project requirement leaves, and the `alpha` and `beta` module hashes

**Rationale**: End-to-end verification that the bottom-up hash propagation produces a consistent merkle tree. This is the core correctness property of the entire module.

### S7: Deterministic — identical content always produces identical tree

**Given** the fixture directory is created twice in separate temp directories with identical file contents
**When** `BuildTree` is called on each
**Then** both root hashes are equal
**And** every corresponding node hash is equal

**Rationale**: Validates requirement 6 (deterministic hashing). Same spec state must always yield the same merkle tree, regardless of OS, filesystem ordering, or timing.

### S8: Multi-module tree construction

**Given** a fixture with two modules `alpha` and `beta`, each with distinct content files
**When** `BuildTree(tmpdir)` is called
**Then** the root has children: `meta/project`, one leaf per project requirement, `alpha` and `beta`
**And** `alpha` and `beta` each have their own correct subtrees, and editing a leaf under `alpha` leaves `beta`'s module hash unchanged
**And** the root hash equals `HashChildren` over all of those child hashes

**Rationale**: Ensures the tree builder correctly handles the real-world case of multiple modules in a project, and that module hashes are combined in sorted order at the root level.

### S9: Project requirement leaf hash is invariant under derivation graduation

**Given** three copies of the fixture identical except for one project requirement: in the first it carries no `derivation` field, in the second it carries `"derivation": "pending"`, in the third the field has been removed again (the graduation edit)
**When** `BuildTree` is called on each
**Then** that requirement's leaf hash is identical across all three trees
**And** the first and third trees are identical at every key — every leaf hash and the root — because their `project.json` files are byte-identical: the graduation round trip returns the tree it started from
**And** the second tree differs from the first at exactly one key, `meta/project`, and therefore also at the root
**And** every other leaf of the second tree, the requirement's own included, equals the first tree's

**Rationale**: two rules meet in this fixture and the scenario pins both. `derivation` is deliberately absent from the project-requirement field allowlist in `arch_tree_builder.md`, so declaring a gap and later graduating out of it moves no requirement leaf — that is what fails if the exclusion is implemented wrongly, since a serializer that hashes whatever fields it finds would move the leaf on both edits. The `meta/project` envelope leaf is hashed from `project.json`'s literal bytes, so it necessarily *does* move on the `pending` variant; asserting that it is the only leaf that moves is what keeps the exclusion honest, because a `derivation` leaked into the requirement serialization would surface here as a second differing key. The root-hash equality is therefore asserted between the first and third trees rather than across all three: an assertion that all three roots agree would contradict the envelope's raw-bytes rule in the same leaf and cannot hold. What the exclusion buys is stated in `arch_tree_builder.md` — the envelope entry is `structural`, so it is filtered before matching and obliges nothing downstream.

### S10: Per-type hashed field allowlists are read from the resolved profile

**Given** the fixture directory, built once under the default profile and once under a `spec/profile.json` byte-identical to the default declaration
**When** `BuildTree` is called on each
**Then** every leaf hash and the root are identical across the two trees — the allowlists TreeBuilder serializes each JSON-backed leaf through come from the resolved profile, and the default profile declares exactly the allowlists previously compiled in
**And** building this repository's own spec under the default profile reproduces every identity hash and every leaf hash the current snapshot records

**Rationale**: The acceptance criterion that no existing hash moves when the taxonomy becomes data. The hash inputs — identity strings and per-type field allowlists — are unchanged under the default profile, so an implementation that derived either from the profile incorrectly surfaces here as a moved hash. Hasher itself is deliberately untouched: file, byte and child hashing are type-agnostic and read nothing from the profile.

### S11: A profile-declared type gets a leaf like any built-in type

**Given** a profile declaring an `endpoint` type (content-bearing, module-scoped) and a fixture module carrying one endpoint with a content file
**When** `BuildTree` is called
**Then** the module node holds one leaf for the endpoint, keyed by its identity hash, hashed from its content file and its declared hashed fields — the tree shape (project root, module interiors, identity-hash-keyed leaves, the synthetic meta leaf) is fixed, and the profile only decides what leaves exist.

## Edge Cases

### E1: HashFile on non-existent file

**Given** a path to a file that does not exist
**When** `HashFile(path)` is called
**Then** it returns an error wrapping the OS error
**And** the error message contains the file path for debuggability

### E2: HashChildren with empty slice

**Given** an empty child hash slice `[]string{}`
**When** `HashChildren(childHashes)` is called
**Then** it returns the SHA-256 hash of the empty string (i.e., `sha256Hex("")`)

**Rationale**: Degenerate case — a module with no content files. The hash should still be deterministic and valid.

### E3: BuildTree fails on missing content file

**Given** a `module.json` that references `arch_missing.md` which does not exist on disk
**When** `BuildTree(tmpdir)` is called
**Then** it returns an error indicating the missing content file path
**And** no partial tree is returned

**Rationale**: Per `arch_tree_builder.md`, missing content files are a build failure. The validator should be run first, but BuildTree must fail cleanly if a file is absent.

### E4: BuildTree ignores extraneous files in the directory

**Given** the fixture directory plus an extra file `alpha/notes.txt` not referenced in `module.json`
**When** `BuildTree(tmpdir)` is called
**Then** the tree contains only files referenced by `module.json` content fields
**And** `alpha/notes.txt` does not appear in the tree
**And** the module node's child count is unchanged from a build without the extra file

**Rationale**: Per `arch_tree_builder.md`, content files are discovered from `module.json`, not from directory listing. Extraneous files must be invisible to the merkle tree.

### E5: Content file with empty body

**Given** a content file `arch_empty.md` that exists but has zero bytes
**When** `BuildTree(tmpdir)` is called
**Then** the leaf hash equals `sha256Hex("")`
**And** the tree builds successfully (empty files are valid leaves)
