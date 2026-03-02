# ProjectSchema

The project.json JSON Schema (`schema/project.schema.json`) defines the top-level structure of a spex-machina spec.

## Structure

```
project.json
├── name (string, required)
├── description (string)
├── version (string)
├── requirements[]
│   ├── id (int >= 1)
│   ├── type ("functional" | "non_functional")
│   ├── title (string, required)
│   ├── description (string)
│   └── depends_on (int[])
├── modules[] (required, minItems: 1)
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── path (string, required)
│   ├── description (string)
│   └── requires_module (int[])
└── milestones[]
    ├── id (int >= 1)
    ├── title (string, required)
    ├── description (string)
    └── groups (int[])
```

## Edge Types

- `depends_on`: requirement → requirement (within project-level requirements)
- `requires_module`: module → module (inter-module dependency)
- `groups`: milestone → module (milestone grouping)

## Design Rationale

Only `name` and `modules` are required at the project level. Requirements and milestones are optional — a minimal spec needs only a name and at least one module declaration. This supports incremental spec authoring: start with structure, add requirements later.

`additionalProperties: false` ensures strict conformance — no extra fields allowed. This makes the schema the single source of truth for what fields exist.
