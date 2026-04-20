# ImpactClassifier

Classifies diff changes by their impact level. Uses node metadata (type, module association) instead of parsing path prefixes.

## Responsibilities

- Analyze each change's node metadata to determine the spec layer affected
- Classify into four impact levels
- Attach impact classification to each change for downstream consumption

## Impact Levels

| Level | Condition | Meaning |
|-------|-----------|---------|
| `impl_only` | Node type is `impl_section` or `test_section` | Implementation detail changed, architecture stable |
| `contract` | Node type is `data_flow` | Cross-component shape changed — all components in the flow's `uses` array must be updated in lockstep |
| `arch_impl` | Node type is `component` | Architecture changed, dependent modules may be affected |
| `structural` | Node type is `meta` or `requirement` | Spec structure changed — new/removed nodes, changed edges, changed requirements |

The `contract` level sits between `impl_only` and `arch_impl`: contract changes are not purely local to one component (so `impl_only` is wrong), but they also do not rewire graph topology (so `arch_impl` is wrong). Downstream node matchers skip `structural` changes but forward `contract` changes so a dedicated data_flow task bead is produced.

## Interface

```go
type ClassifiedChange struct {
    Change
    Impact string // "impl_only", "contract", "arch_impl", "structural"
}

func Classify(changes []Change) []ClassifiedChange
```

The module association is already carried in `Change.Module` from the DiffEngine, so no path parsing is needed to determine which module a change belongs to.

## Rules

Classification uses the node metadata (NodeType, Module) attached to each change by the DiffEngine, not path parsing:

1. If change.NodeType is `"impl_section"` or `"test_section"` → `impl_only`
2. If change.NodeType is `"data_flow"` → `contract`
3. If change.NodeType is `"component"` → `arch_impl`
4. If change.NodeType is `"meta"` (module.json or project.json) → `structural`
5. If change.NodeType is `"requirement"` → `structural`
6. If a module has changes at multiple levels, the highest level wins (structural > arch_impl > contract > impl_only)
