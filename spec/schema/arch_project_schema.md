# ProjectSchema

The project.json JSON Schema (`schema/project.schema.json`) defines the top-level structure of a spex-machina spec.

## Structure

```
project.json
├── name (string, required)
├── description (string)
├── version (string)
├── requirements[]
│   ├── id (string, 12-char hex identity hash)
│   ├── type ("functional" | "non_functional")
│   ├── title (string, required)
│   ├── description (string)
│   ├── priority (int, 0-4, optional)
│   └── depends_on (string[], identity hashes)
├── modules[] (required, minItems: 1)
│   ├── id (string, 12-char hex identity hash)
│   ├── name (string, required)
│   ├── path (string, required)
│   ├── description (string)
│   └── requires_module (string[], identity hashes)
├── milestones[]
│   ├── id (string, 12-char hex identity hash)
│   ├── title (string, required)
│   ├── description (string)
│   └── groups (string[], identity hashes)
├── sections[]
│   ├── id (string, 12-char hex identity hash)
│   ├── name (string, required)
│   ├── type (string, required)
│   └── ... (additional properties allowed — freeform content)
└── test_plan
    └── scenarios[]
        ├── id (string, 12-char hex identity hash)
        ├── name (string, required)
        ├── description (string)
        ├── content (string, path to test_*.md)
        └── modules (string[], identity hashes)
```

All `id` and cross-reference fields use the identity hash string format `^[a-f0-9]{12}$`. IDs are computed deterministically from each node's identity string by `schema.IdentityHash` — see `impl_identity_hash.md` for the algorithm and the table of identity strings per node type. IDs are never assigned manually.

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
