# Hashing Tests

Integration and acceptance tests for the Hasher (component 1) and TreeBuilder (component 2). Validates that SHA-256 leaf hashing, sorted interior hashing, and full tree construction are correct and deterministic.

## Setup

All scenarios operate against a temporary spec directory created in `t.TempDir()`. The fixture layout mirrors a minimal valid spec:

```
tmpdir/
  project.json          (lists one module: "alpha")
  alpha/
    module.json          (lists components, impl_sections with content paths)
    arch_widget.md       ("# Widget\nHandles widgets.")
    arch_gadget.md       ("# Gadget\nHandles gadgets.")
    impl_widget_logic.md ("# Widget Logic\nImplementation details.")
    flow_data_path.md    ("# Data Path\nData flows from A to B.")
```

Helper function `writeFixture(t, root, relPath, content)` writes a file and returns its absolute path. A second helper `sha256Hex(content string) string` computes the expected SHA-256 hex digest of a given string for assertion comparisons.

## Scenarios

### S1: Leaf hash matches independent SHA-256

**Given** a file `arch_widget.md` with content `"# Widget\nHandles widgets."`
**When** `HashFile(path)` is called
**Then** the returned hex string equals `sha256Hex("# Widget\nHandles widgets.")`
**And** the hex string is exactly 64 characters long

**Rationale**: Confirms that `HashFile` reads the file and computes the correct SHA-256 digest — the foundational operation for the entire merkle tree.

### S2: HashFile streams content rather than buffering

**Given** a file written with 10 MB of repeated content
**When** `HashFile(path)` is called
**Then** it returns a valid 64-character hex hash without error
**And** the hash matches `sha256Hex` of the same 10 MB content

**Rationale**: Verifies the streaming `io.Copy` implementation described in `impl_hash_computation.md` handles large files without memory issues.

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
**Then** the root node has type `"project"` and two children: `project.json` (leaf) and `alpha` (module)
**And** the `alpha` module node has children: `alpha/module.json` (leaf), `alpha/arch` (interior), `alpha/impl` (interior), `alpha/flow` (interior)
**And** the `alpha/arch` interior node has children `alpha/arch_widget.md` and `alpha/arch_gadget.md`
**And** the `alpha/impl` interior node has child `alpha/impl_widget_logic.md`
**And** the `alpha/flow` interior node has child `alpha/flow_data_path.md`

**Rationale**: Validates the tree structure algorithm from `arch_tree_builder.md` and `impl_tree_construction.md`: `project.json` discovery, module enumeration, and content file grouping by prefix type.

### S6: BuildTree hashes propagate bottom-up

**Given** the full fixture directory
**When** `BuildTree(tmpdir)` is called
**Then** each leaf node's hash matches `HashFile` of its corresponding file
**And** the `alpha/arch` interior hash equals `HashChildren` of its two leaf hashes
**And** the `alpha` module hash equals `HashChildren` of `[module.json hash, arch hash, impl hash, flow hash]`
**And** the root hash equals `HashChildren` of `[project.json hash, alpha module hash]`

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
**Then** the root has children: `project.json`, `alpha`, `beta`
**And** `alpha` and `beta` each have their own correct subtrees
**And** the root hash equals `HashChildren` of `[project.json hash, alpha hash, beta hash]`

**Rationale**: Ensures the tree builder correctly handles the real-world case of multiple modules in a project, and that module hashes are combined in sorted order at the root level.

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

**Rationale**: Per `impl_tree_construction.md`, missing content files are a build failure. The validator should be run first, but BuildTree must fail cleanly if a file is absent.

### E4: BuildTree ignores extraneous files in the directory

**Given** the fixture directory plus an extra file `alpha/notes.txt` not referenced in `module.json`
**When** `BuildTree(tmpdir)` is called
**Then** the tree contains only files referenced by `module.json` content fields
**And** `alpha/notes.txt` does not appear in the tree
**And** the tree hashes are identical to a build without the extra file

**Rationale**: Per `impl_tree_construction.md`, content files are discovered from `module.json`, not from directory listing. Extraneous files must be invisible to the merkle tree.

### E5: Content file with empty body

**Given** a content file `arch_empty.md` that exists but has zero bytes
**When** `BuildTree(tmpdir)` is called
**Then** the leaf hash equals `sha256Hex("")`
**And** the tree builds successfully (empty files are valid leaves)
