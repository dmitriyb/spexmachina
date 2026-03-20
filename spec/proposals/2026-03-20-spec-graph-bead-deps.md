# Change Proposal: Spec-Graph Bead Dependencies

## Context

When `spex apply` creates new beads (or when beads are created manually following the apply pattern), the only dependency set is `--deps blocks:<old-bead-id>` for lineage — linking a replacement bead to its predecessor. No dependencies are derived from the spec's structural relationships: component `uses` edges (intra-module) and module `requires_module` edges (inter-module).

This means every newly created bead appears immediately workable in `br ready` and `bv`, even when its spec-level dependencies have open (unimplemented) beads. For example, Apply: BeadCreator (`spexmachina-y57`) shows as ready because its only dep is the old closed bead (`spexmachina-sn6`). But the apply module `requires_module: [4, 9]` (impact, map), and Map: MappingStore (`spexmachina-pzp`) is a new open bead that must be implemented first. `br ready` and `bv` cannot see this constraint.

The PreflightChecker (map/component/2) already implements this exact dependency walk at runtime — checking `requires_module` transitively and component `uses` edges against closed beads in the mapping file. But this is a runtime gate for `/implement`, not a structural property of the bead graph. The result: tools that consume the bead graph (`br ready`, `bv`, capacity planning) show incorrect readiness.

The root cause: the spec-to-bead pipeline (ActionClassifier → BeadCreator) does not propagate spec-graph dependencies into bead-graph dependencies.

## Proposed change

### Approach: derive bead deps from spec graph at creation time

BeadCreator already resolves parent hierarchy from the spec (module epic → component feature → test task). The same pattern extends to peer dependencies: resolve `uses` and `requires_module` from the spec, look up current beads from the mapping file, and add `--deps depends:<bead-id>` for each open dependency.

### Impact module changes

**ActionClassifier (impact/component/3):**

Add a dependency resolution step after action classification. For each `create` action:

1. Resolve the spec node's module from the spec graph
2. Collect `requires_module` dependencies (transitive, with cycle detection — reuse PreflightChecker's algorithm)
3. For component nodes: collect `uses` dependencies (direct component IDs within the same module)
4. For each dependency, resolve the current bead ID from the mapping file
5. Attach resolved dependency bead IDs to the Action

**Action struct extension:**

```go
type Action struct {
    // ... existing fields ...
    DepBeadIDs []string // bead IDs this action's bead should depend on (from spec graph)
}
```

The `DepBeadIDs` field carries the resolved bead IDs for spec-graph dependencies. This is separate from `OldBeadID` (lineage).

**Impact module requirement:**

Add requirement 7 to impact/module.json: "Resolve spec-graph dependencies for create actions" — for each create action, resolve `uses` and `requires_module` edges to current bead IDs via the mapping file. The ActionClassifier needs access to the mapping store for this resolution.

Update ActionClassifier (component 3) to implement requirement 7. Update its `uses` to include BeadReader or a new dependency on the mapping store interface.

### Apply module changes

**BeadCreator (apply/component/1):**

For each create action, in addition to `--deps blocks:<old-bead-id>` (lineage), also pass `--deps depends:<dep-bead-id>` for each entry in `DepBeadIDs`. Multiple `--deps` flags are supported by `br create`.

The `depends` relationship type (vs `blocks`) is semantically correct: "this bead depends on that bead" means "don't start this until that is done." The `blocks` type is for lineage: "this bead replaces that bead."

**ApplyCommand (apply/component/6):**

Creation ordering must be extended. Currently: epics → features → tasks (type-based). This ensures parents exist before children. With spec-graph deps, we also need topological ordering within each type level to ensure dependency beads are created before their dependents. If bead A depends on bead B and both are being created in the same apply run, B must be created first so its bead ID is available for A's `--deps`.

### What does NOT change

- **PreflightChecker**: Continues to work as-is. It checks readiness at runtime by reading the mapping file. The bead-graph deps are a parallel mechanism for tool visibility, not a replacement.
- **Mapping file format**: No schema changes to `.bead-map.json`.
- **BeadCloser**: Obsolete actions don't need spec-graph deps.
- **Lineage deps**: `--deps blocks:<old-bead-id>` continues unchanged.

### Dependency resolution semantics

**Module-level (`requires_module`):** When creating a component bead in module A, and module A `requires_module: [B]`, add a `depends` edge to every **open** component bead in module B. If all of B's component beads are closed, no edge is needed (dependency already satisfied). This matches PreflightChecker's algorithm.

**Component-level (`uses`):** When creating a component bead, and the component `uses: [X, Y]`, add a `depends` edge to X and Y's current beads if they are open.

**Transitive module deps:** If A requires B and B requires C, A's beads depend on B's open beads AND C's open beads (transitive). Component `uses` are NOT transitive (same as PreflightChecker).

**Closed beads are skipped:** If a dependency's bead is already closed, no edge is added — the work is done.

## Impact expectation

This is a spec-only change (proposal). No beads are created or modified by the proposal itself.

When the spec changes are applied, the following beads will be affected:

- **Impact: ActionClassifier** (impact/component/3, `spexmachina-ycy`) — modified: new requirement, dependency resolution logic
- **Impact: ReportGenerator** (impact/component/4, `spexmachina-szt`) — modified: Action struct gains `DepBeadIDs` field, report must include it
- **Apply: BeadCreator** (apply/component/1, `spexmachina-y57`) — modified: pass `--deps depends:` from `DepBeadIDs`
- **Apply: ApplyCommand** (apply/component/6, `spexmachina-iup`) — modified: topological ordering within type levels

Estimated scope: 4 component beads obsoleted + 4 new beads created. The impact and apply modules' test sections will also need updates for the new dependency resolution scenarios.
