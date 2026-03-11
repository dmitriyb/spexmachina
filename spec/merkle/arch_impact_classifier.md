# ImpactClassifier

Classifies diff changes by their impact level. Uses node metadata (type, module association) instead of parsing path prefixes.

## Responsibilities

- Analyze each change's node metadata to determine the spec layer affected
- Classify into three impact levels
- Attach impact classification to each change for downstream consumption

## Impact Levels

| Level | Condition | Meaning |
|-------|-----------|---------|
| `impl_only` | Node type is `impl_section` or `data_flow` | Implementation detail changed, architecture stable |
| `arch_impl` | Node type is `component` | Architecture changed, dependent modules may be affected |
| `structural` | Node type is `meta` (module.json or project.json) | Spec structure changed — new/removed nodes, changed edges |

## Interface

```go
type ClassifiedChange struct {
    Change
    Impact string // "impl_only", "arch_impl", "structural"
}

func Classify(changes []Change) []ClassifiedChange
```

The module association is already carried in `Change.Module` from the DiffEngine, so no path parsing is needed to determine which module a change belongs to.

## Rules

Classification uses the node metadata (NodeType, Module) attached to each change by the DiffEngine, not path parsing:

1. If change.NodeType is `"impl_section"` or `"data_flow"` → `impl_only`
2. If change.NodeType is `"component"` → `arch_impl`
3. If change.NodeType is `"meta"` (module.json or project.json) → `structural`
4. If a module has changes at multiple levels, the highest level wins (structural > arch_impl > impl_only)
