# ModuleSchema

The module.json JSON Schema (`schema/module.schema.json`) defines the structure of each module within a spec.

## Structure

```
module.json
├── name (string, required)
├── description (string)
├── requirements[]
│   ├── id (string, 12-char hex identity hash)
│   ├── preq_id (string, identity hash of project requirement, required)
│   ├── type ("functional" | "non_functional")
│   ├── title (string, required)
│   ├── description (string)
│   └── depends_on (string[], identity hashes)
├── components[]
│   ├── id (string, 12-char hex identity hash)
│   ├── name (string, required)
│   ├── description (string)
│   ├── content (string, path to arch_*.md)
│   ├── implements (string[], requirement identity hashes)
│   └── uses (string[], component identity hashes)
├── impl_sections[]
│   ├── id (string, 12-char hex identity hash)
│   ├── name (string, required)
│   ├── content (string, path to impl_*.md)
│   └── describes (string[], component identity hashes)
├── data_flows[]
│   ├── id (string, 12-char hex identity hash)
│   ├── name (string, required)
│   ├── description (string)
│   ├── content (string, path to flow_*.md)
│   └── uses (string[], component identity hashes)
└── test_sections[]
    ├── id (string, 12-char hex identity hash)
    ├── name (string, required)
    ├── content (string, path to test_*.md)
    └── describes (string[], component identity hashes)
```

All `id`, `preq_id`, and cross-reference fields use the identity hash string format `^[a-f0-9]{12}$`. The hash is computed by `schema.IdentityHash` from the node's module name, type, and human identifier (`name` or `title`). See `impl_identity_hash.md` for the full algorithm and identity-string table.

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

Only `name` is required at the module level. All arrays are optional, enabling incremental authoring. The `preq_id` field on requirements is required (not optional) — every module requirement must trace to a project requirement. This creates the traceability chain: project requirement → module requirement → component → impl_section. The `test_sections` array adds a parallel verification chain: component → test_section, enabling test coverage analysis.

Content paths are relative to the module directory, keeping file references local and relocatable.
