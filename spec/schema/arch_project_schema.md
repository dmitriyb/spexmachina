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
│   ├── priority (int, 0-4, optional)
│   └── depends_on (int[])
├── modules[] (required, minItems: 1)
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── path (string, required)
│   ├── description (string)
│   └── requires_module (int[])
├── milestones[]
│   ├── id (int >= 1)
│   ├── title (string, required)
│   ├── description (string)
│   └── groups (int[])
├── sections[]
│   ├── id (int >= 1)
│   ├── name (string, required)
│   ├── type (string, required)
│   └── ... (additional properties allowed — freeform content)
└── test_plan
    └── scenarios[]
        ├── id (int >= 1)
        ├── name (string, required)
        ├── description (string)
        ├── content (string, path to test_*.md)
        └── modules (int[])
```

## Edge Types

- `depends_on`: requirement → requirement (within project-level requirements)
- `requires_module`: module → module (inter-module dependency)
- `groups`: milestone → module (milestone grouping)
- `modules` (test_plan): test_scenario → module (cross-module test coverage)

## Sections and Coupled Modules

The `sections` array is a generic extension point for project-level concerns. Each section entry has a required envelope (`id`, `name`, `type`) and allows additional properties for freeform content.

The section entry itself does NOT use `additionalProperties: false` — this is intentional. The envelope fields are validated by spex core, while the additional content fields are validated by the coupled module's `section.schema.json`. This two-layer validation allows each module to define its own content structure without requiring changes to the project schema.

When `type` is `"coupled"`, the validator enforces that a module with the same `name` exists and provides a `section.schema.json` file. See the CoupledSectionChecker in the validator module for enforcement details.

### section.schema.json Convention

Each coupled module provides a `section.schema.json` file in its spec directory (e.g., `spec/delivery/section.schema.json`). This is a standard JSON Schema (draft 2020-12) that validates the freeform content fields of the section entry (everything except `id`, `name`, `type`). Spex loads this schema at validation time and applies it to the section content.

## Edge Types

- `depends_on`: requirement → requirement (within project-level requirements)
- `requires_module`: module → module (inter-module dependency)
- `groups`: milestone → module (milestone grouping)
- `modules` (test_plan): test_scenario → module (cross-module test coverage)
- `sections[].name` → `modules[].name`: coupled section references its implementing module by name match

## Design Rationale

Only `name` and `modules` are required at the project level. Requirements and milestones are optional — a minimal spec needs only a name and at least one module declaration. This supports incremental spec authoring: start with structure, add requirements later.

`additionalProperties: false` ensures strict conformance at the project level — no extra top-level fields allowed. However, section entries within the `sections` array allow additional properties to support freeform content validated externally.
