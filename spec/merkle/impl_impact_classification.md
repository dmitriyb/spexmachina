# Impact Classification Rules

## Classification Logic

Each changed leaf is classified based on its filename pattern and location:

```
if path ends with "project.json" or "module.json":
    impact = "structural"
elif filename matches "arch_*.md":
    impact = "arch_impl"
elif filename matches "impl_*.md" or "flow_*.md":
    impact = "impl_only"
```

## Module-Level Aggregation

When multiple leaves change within the same module, the module's overall impact is the maximum:

```
structural > arch_impl > impl_only
```

For example, if both `impl_hash_computation.md` and `arch_hasher.md` change in the merkle module, the module impact is `arch_impl`.

## Impact Propagation

A structural change in module A may affect modules that depend on A (via `requires_module`). The Impact module (downstream) handles this propagation — the Merkle module only classifies individual changes.

## Output

The classified changes are the input to the Impact module. Each change carries:
- The file path
- The change type (added/removed/modified)
- The impact level
- The owning module name
