# Impact Classification Rules

## Classification Logic

Each changed leaf is classified based on its node metadata, not its filename pattern or path:

```
if change.NodeType == "meta":
    impact = "structural"
elif change.NodeType == "component":
    impact = "arch_impl"
elif change.NodeType in ("impl_section", "data_flow", "test_section"):
    impact = "impl_only"
```

The NodeType and Module fields are carried in each Change from the DiffEngine, so no path parsing is required.

## Module-Level Aggregation

When multiple leaves change within the same module, the module's overall impact is the maximum:

```
structural > arch_impl > impl_only
```

For example, if both an impl_section and a component change in the merkle module, the module impact is `arch_impl`.

## Impact Propagation

A structural change in module A may affect modules that depend on A (via `requires_module`). The Impact module (downstream) handles this propagation — the Merkle module only classifies individual changes.

## Output

The classified changes are the input to the Impact module. Each change carries:
- The spec ID key (e.g., `module/3/component/2`)
- The change type (added/removed/modified)
- The impact level
- The owning module ID
