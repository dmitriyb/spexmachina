# Change Proposal: Coupled Sections

## Context

Spex defines specs as a typed graph: project-level requirements, module declarations, and within each module — requirements, components, impl_sections, data_flows, and test_sections. Testing was added as a first-class schema concept (`test_plan` in project.json, `test_sections` in module.json) because it crosscuts every component.

Other project-level concerns share this pattern. Delivery (CI, releases, distribution), performance budgets, accessibility standards — each is a cross-cutting concern that deserves a place in the spec graph so that spex's impact pipeline (merkle tracking, change detection, bead creation) works automatically. Today there is no generic mechanism for this. Adding delivery or any other concern would require hardcoding new top-level keys in the project schema and special-casing them throughout the pipeline.

### The coupled sections pattern

Three approaches were considered:

1. **Project-level section only** — declares the strategy, but spex's impact pipeline has no module/components to map changes to. Beads can't be created through the standard pipeline. Would require special-case logic.

2. **Module only** — has components and creates beads normally, but without a project-level schema section, the concern isn't a first-class concept. Each project would reinvent the structure.

3. **Sections array with type-based coupling** (chosen) — project.json gets a `sections` array where each entry has an `id`, `name`, and `type`. The type drives behavior: `type: "coupled"` tells spex this section requires a corresponding module. A coupled module must include a `section.schema.json` that defines validation rules for its section's content. Spex enforces the coupling and delegates content validation to the module's schema.

This is type-driven, not name-driven. Spex doesn't hardcode knowledge of "delivery" or "performance" — it knows about the `coupled` type. Adding a new project-level concern means adding an entry to the `sections` array with `type: "coupled"`, creating a module, and providing a `section.schema.json`. No spex binary changes. The coupling, validation, merkle tracking, impact mapping, and bead creation all work automatically from the existing pipeline.

### Module-owned schema validation

Each coupled module must provide a `section.schema.json` file in its spec directory (e.g., `spec/delivery/section.schema.json`). This is a standard JSON Schema that defines the valid structure for the module's project-level section content. During `spex validate`, for each section with `type: "coupled"`, spex:

1. Checks that a module with matching `name` exists in the modules array
2. Loads `spec/<module-path>/section.schema.json`
3. Validates the section's freeform content against that schema

This reuses spex's existing JSON Schema validation infrastructure (~20 lines of new code in the validator). The coupled module owns its own validation rules — spex core only validates the envelope (`id`, `name`, `type`).

### Follow-up proposals

These proposals build on the coupled sections framework introduced here:

1. **Tool delivery** — A coupled section for delivery: CI pipelines, release automation, and package distribution. First consumer of the coupled sections system. Uses `type: "coupled"` with a delivery module that provides `section.schema.json`.

2. **test_plan migration** — The existing `test_plan` section in project.json could be migrated into the `sections` array to unify the model. However, `test_plan` is lightly coupled (scenarios reference module IDs) while `test_sections` in each module are deeply interconnected with components (via `describes`). A thorough examination of the full connection graph is needed before migrating. This should be a dedicated proposal that: (a) maps all test_plan <-> module <-> test_sections connections, (b) determines if test_plan can become a `type: "coupled"` section with a test module, or needs a different type (e.g., `"distributed"`) reflecting its cross-module nature, (c) handles the breaking change for existing specs.

3. **Project initialization** — `spex init` command and `.spexmachina/` project directory convention. Independent of coupled sections but should support sections in initialized projects. Should cover: directory structure, `spex init` command, project bootstrapping UX, migration path for existing specs.

## Proposed change

### 1. Add `sections` array to project.json schema

Add a generic `sections` array to the project.json schema. Each entry has a required envelope and freeform content validated externally:

**Envelope schema** (validated by spex core):
- `id` (required, integer >= 1, unique within sections)
- `name` (required, string) — must match the coupled module's name
- `type` (required, string) — `"coupled"` enforces module existence + schema validation. Future types possible.

Additional properties are allowed on each section entry. These are freeform content validated by the coupled module's `section.schema.json`, not by spex core.

### 2. Add project requirement

**Requirement 15 — Coupled sections:**
```json
{
  "id": 15,
  "type": "functional",
  "title": "Coupled sections",
  "description": "Project.json supports a sections array with typed entries. Sections with type 'coupled' require a corresponding module and a section.schema.json in the module's spec directory. Spex validates the envelope generically and delegates content validation to the module's schema.",
  "priority": 1
}
```

### 3. Schema module changes

In `spec/schema/module.json`:

- Add requirement 8: "Define sections array in project schema" (`preq_id: 15`). The project.json JSON Schema adds a `sections` array. Each entry requires `id` (integer >= 1), `name` (string), `type` (string). Additional properties allowed (freeform content validated externally by module schemas).
- Add requirement 9: "Define section.schema.json convention" (`preq_id: 15`). Document and enforce the convention that coupled modules provide `section.schema.json` in their spec directory.
- Update ProjectSchema component (component 1) `implements` to include requirements 8 and 9 (currently implements [1, 4, 6], becomes [1, 4, 6, 8, 9]).

### 4. Validator module changes

In `spec/validator/module.json`:

- Add requirement 14: "Validate sections and coupled modules" (`preq_id: 15`). Three concerns:
  - **Envelope validation**: Section IDs unique, required fields present.
  - **Coupled enforcement**: For each section with `type: "coupled"`, a module with matching `name` must exist. Bidirectional: a module claiming to be coupled must have a corresponding section.
  - **Schema delegation**: For each coupled section, load `spec/<module-path>/section.schema.json` and validate the section content against it. Error if the schema file is missing.
- Add a CoupledSectionChecker component to implement this validation generically (not delivery-specific — works for any future coupled section).

### 5. Render module changes

In `spec/render/module.json`:

The render command currently has hardcoded handling in `render/spec_reader.go` for Components, ImplSections, DataFlows, and TestSections. All three renderers (markdown, DOT, JSON) have explicit blocks for each section type. TestSections and test_plan are read but never rendered (existing gap).

- Add requirement (next available ID): "Render sections" (`preq_id: 15`). The spec reader and all three renderers (markdown, DOT, JSON) must handle the `sections` array generically — iterate over sections by name, render each section's content, and pull related content from the coupled module.
- Update SpecReader component to read the `sections` array from project.json.
- Update MarkdownRenderer, DOTRenderer, and JSONRenderer components to render sections.

This should be generic: iterate `sections`, render by name, pull coupled module content. No hardcoded "if delivery" logic.

### 6. Skill changes

**`/propose` skill** (`skills/propose/SKILL.md`):

In **Step 4: Research** -> "For change proposals, read" section, add after item 2:
- "All sections in `project.json` `sections` array — understand existing coupled sections and their modules."

In the **Change Proposal Template** -> "Proposed change" guidance, add:
- "If the change involves a coupled section, specify: which section is affected, whether its `section.schema.json` needs updates, and whether the coupled module's components change."

**`/spec` skill** (`skills/spec/SKILL.md`):

In the **Schema Reference** section, add to project.json listing:
- `sections` — array of typed project-level sections. Each has `id`, `name`, `type`. Type `"coupled"` requires a module with matching name and a `section.schema.json` in the module's spec directory.

In the **File Layout** section, add:
- `spec/<module>/section.schema.json` — JSON Schema for coupled section content validation. Required for modules that implement a coupled section.

In the **Edge table**, add:
- `sections[].name` -> `modules[].name` — coupled section references its implementing module by name match.

In **"Plan the spec graph"** step, add to the summary:
- Include any sections changes and whether coupled modules need creation or updates.

## Impact expectation

This proposal touches three existing spec modules (schema, validator, render) with targeted additions. No new modules are created — the delivery module that consumes this framework is in a separate proposal.

**New beads:**
- Beads for schema module requirements 8-9 and ProjectSchema component update
- Beads for validator module requirement 14 and CoupledSectionChecker component
- Beads for render module section rendering requirement and renderer component updates

**Modified beads:** None — existing requirements and components are unchanged in behavior.

**Closed beads:** None.

**Estimated scope:** 3 sessions:
- Session 1: Schema changes (sections array in project.schema.json, section.schema.json convention)
- Session 2: Validator changes (CoupledSectionChecker, schema delegation)
- Session 3: Render changes (generic section rendering) + skill updates
