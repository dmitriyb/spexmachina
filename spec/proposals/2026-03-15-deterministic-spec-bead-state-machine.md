# Change Proposal: Deterministic Spec-Bead State Machine

## Context

The current pipeline (diff → impact → apply) creates beads from spec changes, but the relationship is loosely coupled:

1. **All beads are `type: task`.** Every component, impl_section, test_section, and data_flow creates the same bead type. There is no structural hierarchy in the issue tracker that mirrors the spec hierarchy. A module with 5 components and 3 test sections produces 8 undifferentiated tasks — no epic grouping, no type-based triage.

2. **The "review" action is a metadata patch, not a lifecycle event.** When a spec node changes and has an existing bead, the current ActionClassifier action is "review" — update `spec_hash` on the bead. This creates invisible mutations: the bead looks the same but the spec behind it changed. There is no audit trail. If the bead was already closed (work done), the closed bead silently becomes stale — nothing signals that the completed work may be invalidated. Note: this "review" action has nothing to do with the `/review` skill which reviews PRs — it is purely the impact module's classification label for in-place metadata updates.

3. **No predecessor/successor lineage.** When a spec node changes, the old bead and new bead have no link between them. History is fragmented.

4. **Bead status is ignored in classification.** A closed bead (work done) and an open bead (work not started) both get the same "review" action. These are fundamentally different situations.

Meanwhile, the building blocks for a tighter model already exist:

- The **spec graph** has a clear hierarchy: project → module → {components, impl_sections, test_sections, data_flows}
- The **merkle diff** identifies exactly which nodes changed, by ID
- The **mapping file** links spec nodes to beads with full metadata
- **`br`** supports typed issues (`--type epic/feature/task/bug`) and dependency links (`--deps type:id`, `--parent`)

The gap is not in tooling but in **rules** — no deterministic function maps spec node types to bead types, and no state machine governs how beads transition when their underlying spec changes.

Existing proposals that touch adjacent concerns:
- `2026-03-10-mapping-layer.md` defined the `.bead-map.json` format and the Map module's MappingStore. This proposal extends that format (adds `bead_type` field) and changes how records are updated.
- `2026-03-10-cli-infrastructure.md` defined the Apply module's commands (BeadCreator, BeadCloser, BeadUpdater, ApplyCommand). This proposal changes the behavior of all four, including eliminating BeadUpdater.

## Proposed change

### 1. Deterministic type assignment

Define a fixed mapping from spec node type to bead issue type:

| Spec Node Type | Gets a bead? | Bead Type | Rationale |
|---------------|-------------|-----------|-----------|
| module (in project.json) | yes | `epic` | Grouping container for a module's work |
| component (in module.json) | yes, one bead covering arch + impl | `feature` | Each component is a distinct capability to build |
| impl_section | no — covered by component bead | — | Impl tightens with component; they are inseparable work |
| data_flow | no — documentation only | — | Shows how components interact; not a work item |
| test_section | yes | `task` | Distinct verification effort (end-to-end testing) |

The type is a pure function of the spec node type, regardless of history. A component bead is always `feature` whether it's created for the first time or replacing an obsoleted predecessor. No history queries needed for type assignment.

When `spex apply` creates a bead, it reads the `spec_node_id` from the mapping record (e.g., `impact/component/3`), extracts the node type (`component`), and uses the table above to set `--type feature`. This replaces the current hardcoded `--type task`.

**Hierarchy via `--parent`:** Component beads (features) are created with `--parent <module-epic-bead-id>`. Test_section beads (tasks) are created with `--parent <component-feature-bead-id>`. This mirrors the spec graph in the issue tracker.

**Creation order:** Modules (epics) first, then components (features), then test_sections (tasks). The Apply flow processes creates sequentially respecting this ordering.

### 2. Simplified lifecycle: obsolete + create

Replace the current three-action model (create/close/review) with a uniform flow. Only one lifecycle label: `spex:obsolete`. No tombstone, no superseded-by — dead is dead.

**State transition table:**

| Spec Change | Existing Bead? | Action |
|-------------|---------------|--------|
| added | no | **create** new bead |
| modified | yes (any status) | **obsolete** old bead, **create** new bead |
| removed | yes (any status) | **obsolete** old bead |

The "review" action is eliminated entirely. Any spec change to an existing node always obsoletes the old bead and creates a fresh one (if the node still exists). No in-place metadata patching.

**Obsoleting a bead:**
```
br close <bead-id> --add-label spex:obsolete --add-label commit:<HEAD>
```

The `commit:<HEAD>` label stamps the last commit where the bead's spec was valid. Active beads carry no commit label — the spec is the current HEAD. The commit label only appears at the moment of obsolescence, providing a precise pointer for historical lookup: `git show <that-commit>:spec/...` recovers the exact spec state.

**Lineage via `br` deps:** New beads are created with `--deps blocks:<old-bead-id>`. History is a dependency chain walk in `br`: bead-v3 → bead-v2 → bead-v1.

### 3. `.bead-map.json` is current state only

The mapping file is always a clean snapshot of active beads. One record per active spec node, no history.

- **Changed node**: obsolete old bead, create new bead, **update** the existing mapping record with the new `bead_id` and `spec_hash`.
- **Removed node**: obsolete old bead, **delete** the mapping record.
- **Added node**: create bead, **insert** new mapping record.

History reconstruction does not rely on the mapping file:
- **Lineage**: follow `deps` chain in `br` (bead-v3 → bead-v2 → bead-v1)
- **Spec at any point**: each obsolete bead has `commit:<hash>` label; the `.bead-map.json` at that commit shows the full mapping at that time

**Mapping record format** (add `bead_type` field for auditability):

```json
{
  "id": 42,
  "spec_node_id": "impact/component/3",
  "bead_id": "abc-123",
  "bead_type": "feature",
  "module": "impact",
  "component": "ActionClassifier",
  "content_file": "spec/impact/arch_action_classifier.md",
  "spec_hash": "e3b0c44..."
}
```

### 4. Apply pipeline

```
1. User modifies spec files
2. spex diff → list of changed spec node IDs (added, modified, removed)
3. spex apply:
   For each modified node:
     a. Look up spec_node_id in .bead-map.json → get old bead_id
     b. br close <old_bead_id> --add-label spex:obsolete --add-label commit:<HEAD>
     c. br create --type <from type table> --parent <parent-bead-id> --deps blocks:<old_bead_id> → new bead_id
     d. Update mapping record: set bead_id = new_bead_id, update spec_hash
     e. br update <new_bead_id> --add-label spex:<record-id>
   For each added node (no existing record):
     a. br create --type <from type table> --parent <parent-bead-id> → new bead_id
     b. Insert new mapping record
     c. br update <new_bead_id> --add-label spex:<record-id>
   For each removed node:
     a. Look up in .bead-map.json → get bead_id
     b. br close <bead_id> --add-label spex:obsolete --add-label commit:<HEAD>
     c. Delete mapping record
   Save new snapshot
4. User commits (spec changes + snapshot + .bead-map.json)
```

**Creation order within "added" and "modified":** epics first, then features, then tasks. Parent bead IDs are resolved from the mapping file — the module's epic must exist before component features can be created.

### 5. Module-level beads (epics)

Modules get epic beads. The module's `spec_node_id` is `<module-name>/module` (e.g., `impact/module`). The mapping record links this to the epic bead.

Epics enable:
- **Grouping**: `br list --parent <epic-id>` shows all work for a module
- **Progress**: Epic completion = all child features closed
- **Impact scope**: A structural change (module.json itself changed) obsoletes the epic and creates a new one

### 6. `preq_id` required on module requirements

Every module requirement must trace to a project requirement via `preq_id`. This is enforced by:

- **Schema**: add `preq_id` to the `required` array in the module requirement definition in `module.schema.json`
- **Validator**: check that every `preq_id` references an existing project requirement ID

No orphan module requirements. If a module requirement doesn't derive from a project goal, either the project goal is missing or the module requirement shouldn't exist.

### 7. Deterministic priority propagation

Add a `priority` field to project requirements in `project.schema.json` (integer, 0-4).

**Priority derivation for beads:**

```
component.implements[] → module requirement.preq_id → project requirement.priority
bead_priority = min(all project requirement priorities in that set)
```

The lowest number (highest urgency) wins. If a component implements requirements tracing to P0 and P2 project requirements, the bead gets P0. Passed via `--priority` on `br create`.

**Migration:** existing project requirements without a `priority` field default to P1 (high). The user reviews and adjusts manually.

### 8. Schema and validator tightening

**`module.schema.json`:** Add `preq_id` to the `required` array on the module requirement definition. Every module requirement must trace to a project requirement.

**`project.schema.json`:** Add `priority` field (integer, 0-4) to the project requirement definition.

**Validator:** Two new checks — `preq_id` on every module requirement must reference an existing project requirement ID; `priority` must be present on project requirements.

**Deferred:** Devops section (new section type in module.json for build/deployment activities, each getting its own `task` or `chore` bead) is deferred to a follow-up proposal.

## Impact expectation

**Impact module (module ID 4):**
- ActionClassifier (component 3) — replace three-row decision table with simplified state transition table (added→create, modified→obsolete+create, removed→obsolete)
- ReportGenerator (component 4) — new action values: `create`, `obsolete` (replacing `create`, `close`, `review`)

**Apply module (module ID 5):**
- BeadCreator (component 1) — type assignment from type table, `--parent` linking, `--deps blocks:<old-bead-id>` for lineage, `--priority` from deterministic derivation
- BeadCloser (component 2) — simplified to uniform path: `br close` + `spex:obsolete` + `commit:<HEAD>` labels
- BeadUpdater (component 3) — eliminated; "review" action replaced by obsolete+create flow
- ApplyCommand (component 6) — enforce creation ordering (epics → features → tasks)

**Map module (module ID 9):**
- MappingStore (component 1) — add `bead_type` field to records, support record updates (not just create/delete)

**Schema module (module ID 1):**
- ModuleSchema — `preq_id` becomes required on module requirements
- ProjectSchema — add `priority` field to project requirements

**Validator module (module ID 2):**
- IdValidator — new cross-reference check: `preq_id` must reference existing project requirement
- New check: `priority` field present on project requirements

**No new modules or components.** This is a behavioral change to existing components plus schema tightening. Devops section deferred to follow-up proposal.
