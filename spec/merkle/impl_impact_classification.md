# Impact Classification Rules

## Classification Logic

Each changed leaf is classified based on its node metadata, not its filename pattern or path:

```
if change.NodeType in ("meta", "requirement"):
    impact = "structural"
elif change.NodeType == "component":
    impact = "arch_impl"
elif change.NodeType == "data_flow":
    impact = "contract"
elif change.NodeType in ("impl_section", "test_section"):
    impact = "impl_only"
```

The NodeType and Module fields are carried in each Change from the DiffEngine, so no path parsing is required.

## Why `contract` is distinct from `impl_only`

A data_flow leaf describes the shapes moving between the components in its `uses` array. When those shapes change, every participant is affected — but the graph topology (which components exist, what they implement, who depends on whom) does not change. Treating the change as `impl_only` would tell downstream matchers "skip this, no bead needed," which is wrong because the consumers need a coordinated update. Treating it as `arch_impl` would imply a component-shape change, which is also wrong because the components themselves are unchanged.

The `contract` level captures this precisely: "cross-component shape inside a module, no topology change." It is the signal for the action classifier to produce a single data_flow task bead that owns the shared types, with the participating component beads then picking up per-component logic.

## Module-Level Aggregation

When multiple leaves change within the same module, the module's overall impact is the maximum:

```
structural > arch_impl > contract > impl_only
```

For example, if both an impl_section and a component change in the merkle module, the module impact is `arch_impl`. If a data_flow and an impl_section change, the module impact is `contract`.

## Impact Propagation

A structural change in module A may affect modules that depend on A (via `requires_module`). The Impact module (downstream) handles this propagation — the Merkle module only classifies individual changes.

## Output

The classified changes are the input to the Impact module. Each change carries:
- The identity hash key (e.g., `a1b2c3d4e5f6`), or `meta/project` / `meta/<module-hash>` for envelope leaves
- The change type (added/removed/modified)
- The impact level (`impl_only` | `contract` | `arch_impl` | `structural`)
- The owning module's identity hash (empty for project-level nodes)
- The node type (component, impl_section, data_flow, test_section, requirement, meta)
