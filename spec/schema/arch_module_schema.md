# ModuleSchema

The module.json JSON Schema (`schema/module.schema.json`) defines the structure of each module within a spec.

## Structure

```
module.json
├── name (string, required)
├── description (string)
├── requirements[]
│   ├── id (int >= 1)
│   ├── preq_id (int >= 1, traces to project requirement)
│   ├── type ("functional" | "non_functional")
│   ├── title (string, required)
│   ├── description (string)
│   └── depends_on (int[])
├── components[]
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── description (string)
│   ├── content (string, path to arch_*.md)
│   ├── implements (int[], requirement IDs)
│   └── uses (int[], component IDs)
├── impl_sections[]
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── content (string, path to impl_*.md)
│   └── describes (int[], component IDs)
├── data_flows[]
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── description (string)
│   ├── content (string, path to flow_*.md)
│   └── uses (int[], component IDs)
└── test_sections[]
    ├── id (int >= 1)
    ├── name (string, required)
    ├── content (string, path to test_*.md)
    └── describes (int[], component IDs)
```

## Edge Types

- `preq_id`: module requirement → project requirement (traceability)
- `depends_on`: requirement → requirement (within module requirements)
- `implements`: component → requirement (fulfillment)
- `uses`: component → component (dependency)
- `describes`: impl_section → component (implementation detail)
- `uses` (data_flow): data_flow → component (involvement)
- `describes` (test_section): test_section → component (test coverage)
- `content`: any node → markdown leaf (described_in edge)

## Design Rationale

Only `name` is required at the module level. All arrays are optional, enabling incremental authoring. The `preq_id` field on requirements creates the traceability chain: project requirement → module requirement → component → impl_section. The `test_sections` array adds a parallel verification chain: component → test_section, enabling test coverage analysis.

Content paths are relative to the module directory, keeping file references local and relocatable.
