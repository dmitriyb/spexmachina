# Change Proposal: Requirement Leaf Nodes in Merkle Tree

## Context

The merkle tree currently hashes each `module.json` (and `project.json`) as a single "meta" leaf node keyed by `module/X/meta` (or `project/meta`). Requirements, component edge declarations, and all other structural data are lumped into this one hash. When any field in `module.json` changes — a requirement description, a component's `implements` array, or even the module name — the diff reports a single change: "`module/X/meta` modified."

This blocks the CompletenessChecker (merkle component 8, requirement 8). The checker must verify that when a requirement's description changes, the implementing components' content leaves also changed in the same diff. But the diff only says "meta changed" — it cannot tell *which* requirement changed without comparing old and new JSON file content. That would require either storing old spec files alongside the snapshot or invoking git, both of which violate the project's design constraint of no external state beyond committed files and deterministic operation from snapshot + current spec.

The fix is to make requirements individual leaf nodes in the merkle tree. Each requirement gets its own hash, keyed by spec ID (e.g., `module/1/requirement/3` or `project/requirement/2`). When a requirement's description changes, the diff reports that specific requirement leaf as modified — the CompletenessChecker can then resolve `implements` edges from the current `module.json` and check for corresponding component leaf changes, using only the diff and the current spec directory.

This proposal supersedes the CompletenessChecker portion of proposal 2026-04-04-structural-change-validation.md. The three-stage validation model (validate → diff → impact) remains correct. What changes is the mechanism: instead of comparing old/new JSON files to detect which requirements changed, the diff engine detects requirement changes directly via individual leaf hashes.

## Proposed change

### 1. Merkle module: TreeBuilder (component 2)

Add requirement leaf nodes to the tree. For each requirement in `module.json`, create a leaf node:

- **Key**: `module/<module_id>/requirement/<req_id>`
- **NodeType**: `"requirement"`
- **Module**: the module ID
- **Hash**: SHA-256 of the requirement's deterministic JSON serialization (fields sorted by key: `depends_on`, `description`, `id`, `preq_id`, `title`, `type` — omitting zero-value fields to match `omitempty` semantics)

For each requirement in `project.json`, create a leaf node:

- **Key**: `project/requirement/<req_id>`
- **NodeType**: `"requirement"`
- **Module**: 0
- **Hash**: SHA-256 of the deterministic JSON serialization (fields: `depends_on`, `description`, `id`, `priority`, `title`, `type`)

Requirement leaves are children of their module interior node (or the project root), sorted alongside other leaves by key.

The `module/X/meta` leaf remains — it still hashes the full `module.json` file. When a requirement changes, *both* the meta hash and the individual requirement hash change. This is intentional: meta captures any structural change (including non-requirement changes like component edge modifications), while requirement leaves provide granular requirement-level tracking.

Update `arch_tree_builder.md` and `impl_tree_construction.md` to describe the new requirement leaf nodes.

### 2. Merkle module: ImpactClassifier (component 5)

Add classification rule for the new `"requirement"` node type. Requirement changes are structural signals — they indicate the spec contract changed. Classify as:

- `node_type == "requirement"` → `Structural`

This keeps requirement changes in the same category as meta changes. The NodeMatcher (impact component 2) already skips structural changes, so requirement leaf changes will not produce bead actions — they are validated by the CompletenessChecker instead.

Update `arch_impact_classifier.md` and `impl_impact_classification.md`.

### 3. Merkle module: CompletenessChecker (component 8)

Simplify the algorithm. The function signature becomes:

```go
func CheckCompleteness(changes []ClassifiedChange, specDir string) []DiffError
```

No `snapshotPath` parameter needed — the diff already contains individual requirement changes.

Algorithm:
1. Collect all requirement leaf changes from the diff (`NodeType == "requirement"`)
2. For each **modified** module-level requirement (`module/X/requirement/Y`, `Type == Modified`):
   - Read the current `module.json` for module X
   - Find all components whose `implements` array contains requirement Y
   - For each such component, check whether its content leaf (`module/X/component/Z`) also changed in the diff
   - For each component whose content leaf did NOT change, report a `DiffError`: "requirement Y description changed but component Z content leaf unchanged"
3. For each **added** module-level requirement (`module/X/requirement/Y`, `Type == Added`):
   - Read the current `module.json` for module X
   - Find all components whose `implements` array contains requirement Y
   - If no component implements it, report a `DiffError`: "requirement Y added but no component implements it"
   - For each component that implements it, check whether its content leaf also changed in the diff
   - For each component whose content leaf did NOT change, report a `DiffError`: "requirement Y added but component Z content leaf unchanged"
4. For each **removed** module-level requirement (`module/X/requirement/Y`, `Type == Removed`):
   - Read the current `module.json` for module X
   - For each component whose `implements` array still contains requirement Y, report a `DiffError`: "requirement Y removed but component Z still implements it"
5. For each **modified** project-level requirement (`project/requirement/Y`, `Type == Modified`):
   - Read the current `project.json` and all `module.json` files
   - Find all module requirements with `preq_id == Y`
   - If none exist, report a `DiffError`: "project requirement Y changed but no module requirement derives from it"
   - For each such module requirement, find all implementing components
   - For each component whose content leaf did NOT change in the diff, report a `DiffError`: "project requirement Y changed, derived to module requirement Z, but component W content leaf unchanged"
6. For each **added** project-level requirement (`project/requirement/Y`, `Type == Added`):
   - Find all module requirements with `preq_id == Y`
   - If none exist, report a `DiffError`: "project requirement Y added but no module requirement derives from it"
   - For each such module requirement, find all implementing components
   - For each component whose content leaf did NOT change, report a `DiffError`
7. For each **removed** project-level requirement (`project/requirement/Y`, `Type == Removed`):
   - For each module requirement that still has `preq_id == Y`, report a `DiffError`: "project requirement Y removed but module requirement Z still derives from it"
8. For component edge changes: when `module/X/meta` is modified but no requirement leaves in module X changed, the meta change is due to non-requirement modifications (component edges, module description, etc.). For each component in the current `module.json`, check whether its content leaf also changed. For each component whose content leaf did NOT change, report a `DiffError`.

The CompletenessChecker is the early gate — it runs during `spex diff` before impact or apply touch the bead system. `spex validate`'s RequirementCoverageChecker remains as a last-stand check for newly created specs or manual edits that bypassed the diff pipeline.

Update `arch_completeness_checker.md`, `impl_completeness_algorithm.md`, and `test_diff_classification.md`.

### 4. Merkle module: DiffCommand (component 7)

Update to call `CheckCompleteness` with the simplified signature (no snapshot path). Add `errors` array to JSON output alongside `changes`. Print errors in text output.

Update `arch_diff_command.md` and `impl_diff_command.md`.

### 5. Merkle module: SnapshotStore (component 3)

No format change needed. Requirement leaf nodes are regular leaf nodes — they are serialized to the snapshot flat map like any other leaf (`key → {hash, type, node_type, module}`). The snapshot format is generic and already supports arbitrary node types.

### 6. Merkle module: HashCommand (component 6)

No logic change. The TreeBuilder builds the tree with requirement leaves; the HashCommand just calls `BuildTree` and `Save` as before. The snapshot will naturally include requirement nodes.

### 7. Impact module: NodeMatcher (component 2)

No change needed. NodeMatcher already skips structural changes (per 2026-04-04 proposal, already implemented). Since requirement leaves are classified as `Structural`, they are automatically skipped.

### 8. Spec file updates

In `spec/merkle/module.json`:
- Update requirement 7 (ID-based tree keying) description to mention requirement nodes
- Update requirement 8 (change completeness validation) description to reflect the simplified algorithm
- Update TreeBuilder (component 2) to add `"requirement"` to its description
- Update CompletenessChecker (component 8) description to remove reference to snapshot comparison

In `spec/merkle/arch_tree_builder.md`:
- Add requirement leaves to the tree structure diagram
- Describe deterministic JSON serialization for requirement hashing

In `spec/merkle/impl_tree_construction.md`:
- Add requirement leaf construction step

In `spec/merkle/arch_completeness_checker.md`:
- Remove `snapshotPath` from function signature
- Simplify algorithm to use requirement leaf changes from diff

In `spec/merkle/impl_completeness_algorithm.md`:
- Remove JSON file comparison logic
- Replace with requirement leaf cross-referencing

In `spec/merkle/arch_impact_classifier.md`:
- Add `"requirement"` → `Structural` classification rule

In `spec/merkle/impl_impact_classification.md`:
- Add requirement classification rule

In `spec/merkle/test_diff_classification.md`:
- Add scenarios for requirement leaf changes

## Impact expectation

This proposal modifies the merkle module only. The impact module requires no changes (NodeMatcher already skips structural).

**Merkle module beads affected:**

- **TreeBuilder (component 2)**: content leaf must be updated to describe requirement leaf nodes. Existing bead for TreeBuilder may need reopening or a new bead.
- **ImpactClassifier (component 5)**: content leaf updated with new classification rule. Small change, likely a bead update.
- **CompletenessChecker (component 8)**: this is the bead currently being implemented (spexmachina-2aw). The spec change simplifies the implementation. The bead scope stays the same but the implementation approach changes.
- **DiffCommand (component 7)**: content leaf updated to wire CompletenessChecker and add errors to output. Separate bead.
- **HashCommand (component 6)**: no content change needed.
- **SnapshotStore (component 3)**: no content change needed.
- **DiffEngine (component 4)**: no content change needed (it already handles any leaf type).

**Estimated scope:** 2-3 sessions:
- Session 1: Spec updates (module.json, arch/impl/test content leaves) + TreeBuilder implementation (add requirement leaves)
- Session 2: ImpactClassifier update + CompletenessChecker implementation
- Session 3: DiffCommand integration + end-to-end testing

**Note on dogfooding:** Since `spex diff`, `spex impact`, and `spex apply` are not fully functional yet (this proposal is part of fixing them), spec changes and bead management for this work must be done manually with `br` commands. Once the pipeline is complete, future proposals will flow through the full tool chain.
