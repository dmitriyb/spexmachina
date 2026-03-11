---
name: spec
description: "Read a proposal and author spec files: project.json, module.json, and markdown content leaves"
argument-hint: "<proposal-path-or-name>"
---

# /spec — Author Spec from Proposal

Read a proposal from `spec/proposals/` and create or modify the spec: `project.json`, `module.json` files, and markdown content leaves. This is the LLM interface for spec authoring — all structural output must conform to the JSON Schema.

## Resolve Proposal

1. If `$ARGUMENTS` is a path to an existing file, use it directly.
2. If `$ARGUMENTS` is a name (no path separator), look for `spec/proposals/*-$ARGUMENTS.md`.
3. If `$ARGUMENTS` is empty, list `spec/proposals/` and ask the user which proposal to use.

Read the proposal fully before proceeding.

## Detect Mode

Check the current state of `spec/`:

| Condition | Mode | What to do |
|-----------|------|------------|
| `spec/project.json` does not exist | **New project** | Create `project.json` + all module dirs, `module.json` files, and markdown leaves |
| `spec/project.json` exists, proposal adds new modules | **New module** | Add module entries to `project.json`, create new module dirs with `module.json` and markdown leaves |
| `spec/project.json` exists, proposal modifies existing nodes | **Alter** | Modify existing JSON and markdown files in place |
| `spec/project.json` exists, proposal adds new modules AND modifies existing nodes | **New module + Alter** | Both actions apply — add new modules and modify existing nodes in a single pass |

Tell the user which mode was detected and why before proceeding.

## Schema Reference

All JSON output must conform to the schemas in `schema/project.schema.json` and `schema/module.schema.json`. Read these files before writing any JSON. Key rules:

### project.json

- **Required**: `name`, `modules` (at least one)
- **Optional**: `description`, `version`, `requirements`, `milestones`, `test_plan`
- Requirements: `id` (int ≥1), `type` ("functional" | "non_functional"), `title` (required); `description`, `depends_on` (optional)
- Modules: `id` (int ≥1), `name`, `path` (required); `description`, `requires_module` (optional). **Module `name` must be lowercase and must match the `name` field in the corresponding `module.json` exactly** (e.g., `"impact"`, not `"Impact"`)
- Milestones: `id` (int ≥1), `title` (required); `description`, `groups` (optional)
- Test plan: `scenarios` array; each scenario has `id` (int ≥1), `name` (required); `description`, `content` (path to `test_*.md`), `modules` (optional)

### module.json

- **Required**: `name`
- **Optional**: `description`, `requirements`, `components`, `impl_sections`, `data_flows`, `test_sections`
- Requirements: same as project, plus optional `preq_id` (traces to project requirement)
- Components: `id`, `name` (required); `description`, `content`, `implements`, `uses` (optional). If users or other systems invoke the module externally, that entry point is itself a component.
- Impl sections: `id`, `name` (required); `content`, `describes` (optional)
- Data flows: `id`, `name` (required); `description`, `content`, `uses` (optional)
- Test sections: `id`, `name` (required); `content` (path to `test_*.md`), `describes` (component IDs, optional)

### IDs

- All IDs are integers ≥1, unique within their array (requirements, components, etc.)
- Assign IDs sequentially starting from 1 within each array
- In alter mode, never reuse an ID that was previously assigned — append with the next available ID

### Edges

| Edge | From | To | Field |
|------|------|----|-------|
| `depends_on` | requirement | requirement | `depends_on: [id, ...]` |
| `requires_module` | module | module | `requires_module: [id, ...]` |
| `preq_id` | module requirement | project requirement | `preq_id: id` |
| `groups` | milestone | module | `groups: [id, ...]` |
| `implements` | component | requirement | `implements: [id, ...]` |
| `uses` | component | component | `uses: [id, ...]` |
| `describes` | impl_section | component | `describes: [id, ...]` |
| `uses` | data_flow | component | `uses: [id, ...]` |
| `describes` | test_section | component | `describes: [id, ...]` |
| `modules` | test_scenario | module | `modules: [id, ...]` |
| `described_in` | node | markdown leaf | `content: "path.md"` |

## Interface Mapping

The schema has no explicit interface node type. When migrating legacy specs that have an Interfaces section:

- **Behavioral contracts** (C ABI stability, serialization format, protocol guarantees) → model as **functional requirements**
- **Structural interfaces** (data loaders, visualization, export) → model as **components**

## File Layout

### Directory structure

```
spec/
  project.json
  proposals/
    YYYY-MM-DD-name.md
  <module-path>/          ← path from project.json module entry
    module.json
    arch_<name>.md        ← component content
    impl_<name>.md        ← impl_section content
    flow_<name>.md        ← data_flow content
    test_<name>.md        ← test_section content
```

### Content path conventions

Content paths in `module.json` are relative to the module directory:

| Node type | Filename pattern | Example |
|-----------|-----------------|---------|
| component | `arch_<snake_name>.md` | `arch_schema_checker.md` |
| impl_section | `impl_<snake_name>.md` | `impl_cycle_detection.md` |
| data_flow | `flow_<snake_name>.md` | `flow_validation_pipeline.md` |
| test_section | `test_<snake_name>.md` | `test_schema_validation.md` |

Use lowercase snake_case for the `<name>` portion. The name should be a short, descriptive slug derived from the node name.

### Markdown content leaves

Each markdown file is a content leaf. Write substantive content — these are the detailed design documents that implementers will read. Structure:

- **Component (`arch_*.md`)**: What this component is, its responsibilities, key interfaces, and design rationale. Include ASCII diagrams where they clarify structure.
- **Impl section (`impl_*.md`)**: How the component is built — algorithms, data structures, error handling, key implementation decisions.
- **Data flow (`flow_*.md`)**: How data moves between components — input format, transformations, output format, error paths.
- **Test section (`test_*.md`)**: Module integration/acceptance test scenarios for components — setup, inputs, expected outputs, edge cases. NOT unit tests (those are Go `_test.go` files).

## Workflow

### 1. Read proposal and schemas

- Read the resolved proposal file
- Read `schema/project.schema.json` and `schema/module.schema.json`
- If in new-module or alter mode, read the existing `spec/project.json` and relevant `module.json` files

### 2. Plan the spec graph

Before writing files, present the user with a summary:

- **Project-level requirements** — list with IDs and titles
- **Modules** — list with IDs, names, paths, and inter-module dependencies
- **For each module**: requirements, components, impl_sections, data_flows, test_sections — with edges shown
- **Milestones** — if applicable

Ask the user:
- "These modules will be created: `<list>`. Is anything missing?" — the user may identify modules the proposal implies but that you overlooked (e.g., CLI, API, UI layers).
- Confirm or adjust before writing files. This is the spec review gate.

### 3. Write JSON files

- Write `spec/project.json` (or update it in alter/new-module mode)
- Create module directories under `spec/<module-path>/`
- Write `spec/<module-path>/module.json` for each module
- Use 2-space indentation for JSON

### 4. Write test sections

Write tests BEFORE implementation content to avoid confirmation bias — test scenarios should be derived from requirements and component contracts, not influenced by implementation decisions.

- For each module, create `test_sections` entries in `module.json` that cover all components
- Each test_section's `describes` array must reference component IDs — every component must be covered by at least one test_section
- Write `test_*.md` content leaves with substantive integration/acceptance test scenarios:
  - **Setup**: what fixtures, test data, or preconditions are needed
  - **Scenarios**: concrete input → expected output pairs
  - **Edge cases**: boundary conditions, error paths, invalid inputs
- Group related components into shared test_sections where they have natural testing affinity (e.g., components that form a pipeline)
- If applicable, add cross-module `test_plan` scenarios to `project.json`

### 5. Write implementation content leaves

- Create each markdown file referenced by `content` fields in components, impl_sections, and data_flows
- Write substantive content synthesized from the proposal — not stubs
- If the proposal lacks detail for a particular node, write what you can and mark gaps with `<!-- TODO: detail needed -->` comments

### 6. Validate

- Run `spex validate` if available (the binary may not exist yet during bootstrap)
- If `spex validate` is not available, manually verify:
  - All `content` paths in module.json resolve to files that were created
  - All cross-reference IDs (implements, uses, describes, depends_on, requires_module, groups) point to existing nodes
  - No duplicate IDs within any array
  - Every component is described by at least one test_section (test coverage check)

### 7. Report

Tell the user:
- What files were created or modified (list them)
- Any `<!-- TODO -->` markers that need follow-up
- Remind them to review the spec and commit it to git
- Note: once `spex validate` exists, run it to confirm structural validity

## Alter Mode Details

When modifying an existing spec:

1. Read all existing JSON files first
2. Preserve existing IDs — never renumber
3. Add new nodes with the next sequential ID after the current maximum
4. When removing nodes, delete the JSON entry and its content file — do not leave orphans
5. When modifying a node, update the JSON fields and the content markdown as needed
6. Update all edges affected by the change (e.g., if a component is removed, remove its ID from any `uses` or `describes` arrays)
