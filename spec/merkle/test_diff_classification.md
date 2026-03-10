# Diff and Classification Tests

Integration and acceptance tests for the DiffEngine (component 4) and ImpactClassifier (component 5). Validates that tree comparison correctly identifies added, removed, and modified nodes, and that impact classification assigns the right level based on filename patterns and module aggregation.

## Setup

Scenarios construct two in-memory merkle trees — a "snapshot" tree (representing the previous state) and a "current" tree (representing the working spec). Helper functions:

- `makeTree(nodes map[string]NodeInfo) *Node` — builds a tree from a flat map of `path -> {hash, type, children}`, mirroring the snapshot format for easy construction.
- `findChange(changes []Change, path string) *Change` — locates a change by path in the diff output for targeted assertions.
- `findClassified(classified []ClassifiedChange, path string) *ClassifiedChange` — same for classified changes.

Base fixture trees:

**Snapshot tree** (the "before" state):
```
project.json             hash=pj1  type=leaf
alpha/module.json        hash=am1  type=leaf
alpha/arch_widget.md     hash=aw1  type=leaf
alpha/impl_logic.md      hash=al1  type=leaf
alpha/flow_data.md       hash=af1  type=leaf
beta/module.json         hash=bm1  type=leaf
beta/arch_service.md     hash=bs1  type=leaf
beta/impl_handler.md     hash=bh1  type=leaf
```

**Current tree** is cloned from the snapshot and then selectively mutated per scenario.

## Scenarios

### S1: No changes — empty diff

**Given** the current tree is identical to the snapshot tree (all hashes match)
**When** `Diff(current, snapshot)` is called
**Then** the result is an empty slice (no changes)

**Rationale**: Baseline correctness — when nothing changed, no changes should be reported. This validates the map comparison logic handles the "all keys present, all hashes equal" case.

### S2: Single leaf modified

**Given** the current tree has `alpha/impl_logic.md` with `hash=al2` (changed from `al1`)
**When** `Diff(current, snapshot)` is called
**Then** the result contains exactly one change
**And** that change has Path=`alpha/impl_logic.md`, Type=`modified`, OldHash=`al1`, NewHash=`al2`

**Rationale**: The simplest mutation case. Validates that DiffEngine detects a hash difference for an existing leaf.

### S3: New leaf added

**Given** the current tree has all snapshot nodes plus a new leaf `alpha/arch_gadget.md` with `hash=ag1`
**When** `Diff(current, snapshot)` is called
**Then** the result contains one change with Path=`alpha/arch_gadget.md`, Type=`added`, OldHash=`""`, NewHash=`ag1`

**Rationale**: Validates the set-difference logic: keys in current but not in snapshot are reported as added.

### S4: Leaf removed

**Given** the current tree is the snapshot tree minus `beta/impl_handler.md`
**When** `Diff(current, snapshot)` is called
**Then** the result contains one change with Path=`beta/impl_handler.md`, Type=`removed`, OldHash=`bh1`, NewHash=`""`

**Rationale**: Validates the reverse set-difference: keys in snapshot but not in current are reported as removed.

### S5: Multiple changes across modules

**Given** the current tree has:
- `alpha/impl_logic.md` modified (hash changed)
- `alpha/arch_new.md` added
- `beta/arch_service.md` removed
**When** `Diff(current, snapshot)` is called
**Then** the result contains exactly 3 changes
**And** changes are sorted by path: `alpha/arch_new.md`, `alpha/impl_logic.md`, `beta/arch_service.md`

**Rationale**: Validates that DiffEngine handles mixed change types across multiple modules and returns results sorted by path for deterministic output (per `impl_diff_algorithm.md`).

### S6: First diff with no snapshot (all nodes added)

**Given** the snapshot is an empty tree (no nodes)
**And** the current tree has the full base fixture
**When** `Diff(current, snapshot)` is called
**Then** every leaf in the current tree appears as Type=`added`
**And** the number of changes equals the number of leaves in the current tree

**Rationale**: Per `impl_diff_algorithm.md`, the first run (no previous snapshot) reports everything as added. This is the baseline for future diffs.

### S7: Classify impl_only change

**Given** changes: `[{Path: "alpha/impl_logic.md", Type: "modified"}]`
**When** `Classify(changes)` is called
**Then** the result contains one ClassifiedChange with Impact=`impl_only` and Module=`alpha`

**Rationale**: Files matching `impl_*.md` are implementation-only changes per `impl_impact_classification.md`.

### S8: Classify flow as impl_only

**Given** changes: `[{Path: "alpha/flow_data.md", Type: "modified"}]`
**When** `Classify(changes)` is called
**Then** the result contains one ClassifiedChange with Impact=`impl_only` and Module=`alpha`

**Rationale**: `flow_*.md` files are classified as `impl_only`, same as `impl_*.md` — they describe data flows, not architecture contracts.

### S9: Classify arch_impl change

**Given** changes: `[{Path: "beta/arch_service.md", Type: "modified"}]`
**When** `Classify(changes)` is called
**Then** the result contains one ClassifiedChange with Impact=`arch_impl` and Module=`beta`

**Rationale**: Files matching `arch_*.md` represent architecture changes that may affect dependent modules.

### S10: Classify structural change — module.json

**Given** changes: `[{Path: "alpha/module.json", Type: "modified"}]`
**When** `Classify(changes)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`alpha`

**Rationale**: `module.json` changes are structural — they alter the spec graph itself (added/removed components, changed edges).

### S11: Classify structural change — project.json

**Given** changes: `[{Path: "project.json", Type: "modified"}]`
**When** `Classify(changes)` is called
**Then** the result contains one ClassifiedChange with Impact=`structural` and Module=`""` (project-level, no specific module)

**Rationale**: `project.json` changes affect the entire project structure (modules added or removed).

### S12: Module-level aggregation — highest impact wins

**Given** changes within the same module alpha:
- `alpha/impl_logic.md` modified (impl_only)
- `alpha/arch_widget.md` modified (arch_impl)
**When** `Classify(changes)` is called
**Then** both changes have Module=`alpha`
**And** the per-module aggregated impact for alpha is `arch_impl` (the higher of the two)

**Rationale**: Per `impl_impact_classification.md`, when multiple leaves change within a module, the maximum impact level applies: `structural > arch_impl > impl_only`.

### S13: Module-level aggregation — structural overrides all

**Given** changes within module alpha:
- `alpha/impl_logic.md` modified (impl_only)
- `alpha/arch_widget.md` modified (arch_impl)
- `alpha/module.json` modified (structural)
**When** `Classify(changes)` is called
**Then** the per-module aggregated impact for alpha is `structural`

**Rationale**: Structural is the highest impact level and overrides both arch_impl and impl_only.

### S14: Diff then Classify end-to-end integration

**Given** a snapshot tree and a current tree where:
- `alpha/impl_logic.md` hash changed (impl modification)
- `beta/arch_service.md` hash changed (arch modification)
- `gamma/module.json` added (new module)
**When** `Diff(current, snapshot)` is called, then `Classify(changes)` is called on the result
**Then** alpha's change is classified as `impl_only`
**And** beta's change is classified as `arch_impl`
**And** gamma's module.json change is classified as `structural`

**Rationale**: Full pipeline integration: DiffEngine produces raw changes, ImpactClassifier annotates them. This is the exact data path described in `flow_diff_classification.md`.

## Edge Cases

### E1: Diff with identical trees returns empty, Classify with empty returns empty

**Given** an empty changes slice
**When** `Classify([]Change{})` is called
**Then** the result is an empty slice

**Rationale**: No changes means no impact. Classify must not panic or inject synthetic entries.

### E2: Added leaf in new module

**Given** the snapshot has modules alpha and beta, current tree adds a new module gamma with `gamma/module.json` and `gamma/arch_api.md`
**When** `Diff` then `Classify` are called
**Then** `gamma/module.json` is classified as `structural`
**And** `gamma/arch_api.md` is classified as `arch_impl` with Module=`gamma`

**Rationale**: A brand-new module means both structural changes (new module.json) and arch changes (new component files). The module-level aggregate should be `structural`.

### E3: Removed entire module

**Given** the snapshot has alpha and beta, current tree has only alpha (beta entirely removed)
**When** `Diff` then `Classify` are called
**Then** all of beta's nodes appear as `removed` changes
**And** `beta/module.json` removal is classified as `structural`

### E4: File renamed (appears as add + remove pair)

**Given** the snapshot has `alpha/arch_widget.md`, current tree has `alpha/arch_component.md` (same content, different name)
**When** `Diff(current, snapshot)` is called
**Then** it reports two changes: `alpha/arch_widget.md` removed and `alpha/arch_component.md` added
**And** DiffEngine does not attempt rename detection (it compares paths, not content)

**Rationale**: Rename detection is out of scope for the merkle module. The diff is purely path-and-hash based.

### E5: Deterministic ordering across runs

**Given** a diff with changes across 5 modules
**When** `Diff` is called 100 times with the same inputs
**Then** the change list order is identical every time (sorted by path)

**Rationale**: Per `impl_diff_algorithm.md`, deterministic output ordering is a hard requirement. No randomness or map iteration order leakage.
