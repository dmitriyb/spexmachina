---
name: spec
description: "Read a proposal and author spec files: project.json, module.json, and markdown content leaves"
argument-hint: "<proposal-path-or-name>"
hooks:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: scripts/hooks/deny-commit.sh spec
        - type: command
          command: scripts/hooks/deny-br-close.sh spec
        - type: command
          command: scripts/hooks/assert-single-skill.sh spec
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
- Requirements: `id` (identity hash), `type` ("functional" | "non_functional"), `title` (required); `description`, `depends_on` (optional)
- Modules: `id` (identity hash), `name`, `path` (required); `description`, `requires_module` (optional). **Module `name` must be lowercase and must match the `name` field in the corresponding `module.json` exactly** (e.g., `"impact"`, not `"Impact"`)
- Milestones: `id` (identity hash), `title` (required); `description`, `groups` (optional)
- Test plan: `scenarios` array; each scenario has `id` (identity hash), `name` (required); `description`, `content` (path to `test_*.md`), `modules` (optional)

### module.json

- **Required**: `name`
- **Optional**: `description`, `requirements`, `components`, `impl_sections`, `data_flows`, `test_sections`
- Requirements: same as project, plus **required `preq_id`** (identity hash of the project requirement this module requirement derives from)
- Components: `id`, `name` (required); `description`, `content`, `implements`, `uses` (optional). If users or other systems invoke the module externally, that entry point is itself a component.
- Impl sections: `id`, `name` (required); `content`, `describes` (optional)
- Data flows: `id`, `name` (required); `description`, `content`, `uses` (optional)
- Test sections: `id`, `name` (required); `content` (path to `test_*.md`), `describes` (component IDs, optional)

### IDs

All IDs in `project.json` and `module.json` are 12-character lowercase hex identity hashes (pattern `^[a-f0-9]{12}$`). IDs are computed deterministically from the node's position in the spec graph — never assigned manually, never sequential. Same identity string always produces the same hash.

The hash is `SHA256(identity_string)` truncated to the first 6 bytes (12 hex chars), where `identity_string` is built per node type:

| Node type | Identity string |
|-----------|-----------------|
| Project requirement | `project/requirement/<title>` |
| Module | `module/<name>` |
| Module requirement | `<module>/requirement/<title>` |
| Component | `<module>/component/<name>` |
| Impl section | `<module>/impl_section/<name>` |
| Data flow | `<module>/data_flow/<name>` |
| Test section | `<module>/test_section/<name>` |
| Milestone | `milestone/<title>` |
| Test scenario | `test_plan/scenario/<name>` |

Prefer the `spex hash-id` subcommand — it calls the same `schema.IdentityHash` helper that the rest of the pipeline uses, so any future change to the hashing scheme stays consistent across authoring and validation:

```bash
spex hash-id --type requirement --name "Spec test strategy"
spex hash-id --type requirement --module merkle --name "Classify impact"
spex hash-id --type component    --module merkle --name "ImpactClassifier"
spex hash-id --type data_flow    --module merkle --name "Hash computation flow"
spex hash-id --type test_section --module apply  --name "Snapshot tests"
spex hash-id --type module       --name merkle
spex hash-id --type milestone    --name "Bootstrap"
spex hash-id --type scenario     --name "Coupled sections integration"
```

Valid `--type` values: `requirement`, `component`, `impl_section`, `data_flow`, `test_section`, `module`, `milestone`, `scenario`. `--module` is required for every type that lives inside a module (requirement, component, impl_section, data_flow, test_section); it is omitted for `module`, `milestone`, `scenario`, and project-level requirements.

When `spex` is not yet built, use `printf '%s' "<identity_string>" | sha256sum | head -c 12` as a manual fallback — but build the binary and re-verify at the first opportunity, because a silent divergence between manual hashing and the Go helper would poison the spec.

Changing a node's `name` or `title` changes its identity hash — the pipeline treats it as delete + create. Rename with care.

The only integer ID in the system is the `.bead-map.json` record's internal `id` field — used only for the `spex:<id>` bead label, never referenced from the spec graph.

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

### 0. Maintain an authoring log

Throughout this run, keep an in-memory append-only log of every spec node you create or modify. Each entry records:

- Module name
- Node type (`module`, `requirement`, `component`, `impl_section`, `data_flow`, `test_section`, `milestone`, `scenario`)
- Identity hash (computed via `spex hash-id`)
- Node name
- File path of any content leaf written
- Source: which proposal section / requirement drove this node

The log is the input to the per-node check (steps 3–5) and the end-of-session sanity gate (step 6). It does NOT need to be persisted to disk — it lives in your reasoning context for this run only.

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
- **Per-node check**: after each `module.json` write, append every node it declares (requirements, components, impl_sections, data_flows, test_sections) to the log AND run the per-node check defined in step 6's "Per-node check" subsection on each one. Fix anything before moving to the next file.

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
- **Per-node check**: after writing each `test_*.md`, append the test_section to the log AND run the per-node check on it. The `describes >= 2` ⇔ scenarios-actually-span-multiple-components rule is the highest-value check here.

### 5. Write implementation content leaves

- Create each markdown file referenced by `content` fields in components, impl_sections, and data_flows
- Write substantive content synthesized from the proposal — not stubs
- If the proposal lacks detail for a particular node, write what you can and mark gaps with `<!-- TODO: detail needed -->` comments
- **Per-node check**: after writing each content leaf (`arch_*.md`, `impl_*.md`, `flow_*.md`), append it to the log AND run the per-node check on it.

### 6. Sanity gates

Two-phase gate: per-node check (already run inline during steps 3–5) and end-of-session full review.

#### Per-node check

Run immediately after a single node is written (during steps 3–5). Cheap, scoped to one node and its content leaf:

- **Schema/structural**: defer to `bin/spex validate` for whole-graph checks; for one node, just confirm the JSON entry has its required fields (id, name) and any referenced IDs (in `implements`, `uses`, `describes`) point at nodes that exist or are about to be written in this run.
- **Prose-vs-JSON**: read what you just wrote in the content leaf. Does the prose describe what the JSON declaration claims?
  - `component / arch_*.md`: prose describes behavior fulfilling the requirements in `implements`
  - `impl_section / impl_*.md`: prose describes implementation of the components in `describes`, not other ones
  - `test_section / test_*.md`: scenarios match the structural shape declared by `describes`. For `describes >= 2`, scenarios actually span multiple of the described components (a scenario that names one component, one method is a unit test and does NOT belong here). For `describes == 1`, scenarios are bundled with that component's TDD work.
  - `data_flow / flow_*.md`: prose describes shapes/contracts moving between the components in `uses`
- If a check fails: fix the JSON entry or rewrite the content leaf before moving to the next node. Do not log unresolved findings forward — handle them inline.

#### End-of-session full review

Run once after step 5 completes, before step 7.

1. **Deterministic schema pass**: `bin/spex validate`. Exit code 0 with `"valid": true` is required. If errors, fix them and re-run before moving to the diff pass — schema/DAG failures will produce uninterpretable diff output.
2. **Deterministic completeness pass**: `bin/spex diff --json`. Read the top-level `errors` array. A non-empty array is a hard failure even though the CLI's bare text output labels these as "warnings" — the JSON is authoritative. Common findings:
   - `incomplete_change` with message `module X meta changed but component Y content leaf unchanged` — meta hash of module X changed (e.g., a component was added/removed or a test_section's `describes` was reshaped) but component Y's content leaf was not touched in this session. Fix by editing `arch_<Y>.md` (or whichever leaf belongs to Y) to acknowledge the structural shift in the module — what changed about Y's call sites, test surface, or relationships. The edits should be real, not cosmetic. If multiple components are flagged, every one of them needs an edit; the rule is intentionally strict.
   - Other `incomplete_change` shapes (requirement → component coverage, project requirement chain, etc.): fix per the message — usually the implementing component's content needs to change.
   This pass MUST end with `errors: []` before moving on. The pipeline (`spex impact`, `spex emit`) refuses to consume a diff with errors, so leaving them unresolved blocks the user.
3. **Cross-node prose-vs-JSON pass**: walk the authoring log. For each touched node, re-run the per-node check now that ALL nodes exist (catches issues where a node referenced something only written later in this session). Also check cross-cutting consistency:
   - No orphan content files (every `*.md` in a touched module dir is referenced by `module.json`)
   - Every component is covered by at least one `test_section.describes` entry (this is what `bin/spex validate`'s `test_coverage` checker enforces — confirm it passed)
   - Cross-references in `implements`, `uses`, `describes`, `requires_module`, `preq_id` resolve to existing nodes

#### Loop and escape

If the end-of-session review surfaces findings:

- Attempt 1: revise the spec to address findings; re-run the review.
- Attempt 2: revise again; re-run.
- After attempt 2 fails: HALT auto-correction. Present the user with two choices:
  - `skip` — leave the findings unresolved; user will adjust manually before running the pipeline.
  - `one more round` — try one more auto-correction attempt; on failure, present the same two choices again.

If all gates pass: print `spec: sanity gates passed (N nodes touched across M modules)` and proceed to step 7.

### 7. Report

Tell the user:
- What files were created or modified (list them)
- Any `<!-- TODO -->` markers that need follow-up
- Remind them to review the spec and commit it to git

## Alter Mode Details

When modifying an existing spec:

1. Read all existing JSON files first
2. Preserve existing IDs — never renumber, never recompute for unchanged nodes
3. Add new nodes by computing their identity hash from the identity string (see IDs section). IDs are content-derived, not assigned
4. When removing nodes, delete the JSON entry and its content file — do not leave orphans
5. When modifying a node, update the JSON fields and the content markdown as needed. **Changing a node's `name` or `title` changes its identity hash** — the old ID is gone, a new one appears. The pipeline treats this as delete + create
6. Update all edges affected by the change (e.g., if a component is removed, remove its ID from any `uses` or `describes` arrays)
7. **Module-level supersession: delete old, create new.** When a proposal restructures modules — splitting one into multiple, merging several into one, or reshaping a module's components significantly — delete the old module entry from `project.json`, delete the old module directory and all its content files, and create the new module(s) as entirely fresh structures. Do not keep the old directory for the new module, do not reuse IDs, and do not leave component shells pointing forward. Step 5's identity-hash rule handles individual rename cases; this extends the same intent to whole-module restructuring so ActionClassifier sees clean `removed → obsolete + cleanup` signals for the old nodes and `added → create` for the new ones, with no false lineage between old and new component beads.
