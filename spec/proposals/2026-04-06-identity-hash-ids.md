# Change Proposal: Identity Hash IDs

## Context

Spec nodes (requirements, components, impl_sections, data_flows, test_sections, modules, milestones) use incremental integer IDs. These IDs are assigned sequentially within each array and referenced across edges (`implements`, `uses`, `describes`, `depends_on`, `requires_module`, `preq_id`, `groups`).

Incremental integers break under parallel development. When two branches independently add nodes to the same module, they assign the same next ID to different things. On merge, IDs collide and all cross-references break. This was first encountered when the structural validation proposal (RequirementCoverageChecker, validator requirement 14, component 10) and the coupled sections proposal (CoupledSectionChecker, validator requirement 14, component 10) both targeted the validator module from separate branches.

The root cause: integers are order-dependent. They require coordination (strict sequential work) to avoid collisions. This is incompatible with parallel branches.

### Two hashes, two purposes

The fix separates two concerns that are currently conflated:

**Identity hash** — a deterministic hash of the node's position in the spec graph. Computed from the node's module name, node type, and node name (e.g., `SHA256("impact/component/NodeMatcher")`, truncated to 12 hex characters). This replaces the integer ID in all JSON structures and cross-references. It's stable — it only changes if the node is renamed or moved to a different module. Two branches adding different nodes produce different identity hashes by definition. No collisions, no coordination needed.

**Content hash** — the existing merkle tree hash of the node's content leaf file. Used for change detection (diff, impact). When content is edited, the content hash changes, triggering obsolete+create in the pipeline. The identity hash stays the same — the bead-map record keeps its identity, only the `bead_id` and `spec_hash` are updated.

This mirrors the current architecture exactly. Integer IDs are stable references; content hashes drive change detection. The identity hash is a collision-free replacement for the integer. Nothing changes in the pipeline logic.

### Identity hash computation

For each node type, the identity string is:

| Node type | Identity string | Example |
|-----------|----------------|---------|
| Project requirement | `project/requirement/<title>` | `project/requirement/Compute impact` |
| Module | `module/<name>` | `module/impact` |
| Module requirement | `<module>/requirement/<title>` | `impact/requirement/Classify actions` |
| Component | `<module>/component/<name>` | `impact/component/NodeMatcher` |
| Impl section | `<module>/impl_section/<name>` | `impact/impl_section/Node matching algorithm` |
| Data flow | `<module>/data_flow/<name>` | `impact/data_flow/Impact analysis flow` |
| Test section | `<module>/test_section/<name>` | `impact/test_section/Bead matching tests` |
| Milestone | `milestone/<title>` | `milestone/Change Tracking` |
| Test scenario | `test_plan/scenario/<name>` | `test_plan/scenario/Cross-module mapping integration` |

The identity hash is `SHA256(identity_string)` truncated to 12 hex characters. Collision probability at 48 bits is negligible for spec-sized graphs (hundreds of nodes, not millions). The `name` or `title` field is the natural human-readable identifier that already exists on every node. If you know the module and component name, you can compute the ID yourself.

### Renaming is destructive

Renaming a node (changing its `name` or `title`) changes its identity hash. This is equivalent to deleting the old node and creating a new one — the old bead is obsoleted, a new bead is created. This matches the current behavior: changing a component's name today effectively creates a new integer ID.

## Proposed change

### 1. Schema changes

In `schema/project.schema.json` and `schema/module.schema.json`:

- Change all `id` fields from `"type": "integer", "minimum": 1` to `"type": "string", "pattern": "^[a-f0-9]{12}$"`.
- Change all cross-reference fields (`implements`, `uses`, `describes`, `depends_on`, `requires_module`, `preq_id`, `groups`, `modules`) from integer arrays/values to string arrays/values with the same hex pattern.

In `schema/bead-map.schema.json`:

- Update `spec_node_id` pattern to match the new identity hash format.

In `schema/schema.go`:

- Change all `ID int` struct fields to `ID string`.
- Change all `[]int` cross-reference fields (DependsOn, RequiresModule, Uses, Implements, Describes, Groups, Modules) to `[]string`.
- Change `PreqID int` to `PreqID string`.

### 2. Schema module spec changes

In `spec/schema/module.json`:

- Update requirement 1 ("Define project schema") description to reflect string hash IDs.
- Update requirement 2 ("Define module schema") description to reflect string hash IDs.
- Update requirement 4 ("Numeric ID constraints") — rename to "Identity hash ID constraints". IDs are 12-character hex strings, unique within their containing array.
- Update ProjectSchema (component 1) and ModuleSchema (component 2) content leaves.

### 3. New utility: IdentityHash function

A shared utility function:

```go
func IdentityHash(parts ...string) string {
    identity := strings.Join(parts, "/")
    h := sha256.Sum256([]byte(identity))
    return hex.EncodeToString(h[:6]) // 12 hex chars = 6 bytes
}
```

Lives in the schema package since it defines what IDs look like. Used by TreeBuilder, MappingStore, validator, and the migration script.

### 4. Merkle module changes

In `spec/merkle/module.json`:

- Update TreeBuilder (component 2) content: `nodeKey()` uses identity hashes directly instead of `fmt.Sprintf("module/%d/%s/%d", ...)`.
- Update requirement 7 ("ID-based tree keying") description to reflect identity hash keying.

### 5. Validator module changes

In `spec/validator/module.json`:

- Update IDValidator (component 5) content: uniqueness checks and cross-reference validation switch from integer to string comparison. `map[int]bool` becomes `map[string]bool`.
- Update requirement 5 ("ID uniqueness") and requirement 6 ("Cross-reference integrity") descriptions.

### 6. Impact module changes

In `spec/impact/module.json`:

- Update ActionClassifier (component 3) content: `ResolveDeps()` uses identity hashes instead of `fmt.Sprintf("%s/component/%d", ...)`.
- `nodeTypeFromSpecNodeID()` — identity hashes don't embed the node type. The node type must be carried separately (already available in `ClassifiedChange.NodeType` and `Record.BeadType`).

### 7. Apply module changes

In `spec/apply/module.json`:

- Update `deriveSpecNodeID()`: use identity hashes directly from the diff change or mapping record instead of parsing merkle keys to extract integer IDs.

### 8. Map module changes

In `spec/map/module.json`:

- Update MappingStore (component 1): `spec_node_id` values become identity hashes.
- Update ContextResolver (component 4): looks up identity hash in the module's component list instead of parsing integer from spec_node_id.
- Update `mapping/spec_graph.go`: translation between spec_node_id and merkle keys uses identity hashes throughout.

### 9. Spec file migration

All `spec/project.json` and `spec/*/module.json` files:

- Replace every integer `id` with its computed identity hash.
- Replace every cross-reference integer with the corresponding identity hash.
- Mechanical transformation: compute `IdentityHash("<module>", "<node_type>", "<name>")` for each node, replace the integer.

### 10. Bead-map migration

In `.bead-map.json`:

- Replace `spec_node_id` values (currently like `"impact/component/2"`) with identity hashes.
- The internal record `id` field remains an integer — it's auto-incremented and internal to the map, not referenced in the spec graph.

### 11. Migration tooling

A `spex migrate-ids` command (or standalone script) that:

1. Reads all spec JSON files
2. Computes identity hashes for every node
3. Builds an integer → hash mapping table
4. Replaces all IDs and cross-references in all JSON files
5. Updates `.bead-map.json` spec_node_id values
6. Validates the result with `spex validate`

### 12. Skill changes

**`/spec` skill:** Update the IDs section from "All IDs are integers >= 1, unique within their array" to "All IDs are 12-character hex identity hashes computed from the node's module, type, and name. IDs are computed automatically — never assigned manually."

## Impact expectation

This is a cross-cutting change affecting all modules. However, the conceptual change is small — integer → string — and the pipeline logic is unchanged.

**Affected modules (content leaf updates):**
- Schema: ProjectSchema, ModuleSchema, SchemaLoader content + requirement descriptions
- Validator: IDValidator content + requirement descriptions
- Merkle: TreeBuilder content + requirement 7 description
- Impact: ActionClassifier content
- Apply: BeadCreator / ApplyCommand content
- Map: MappingStore, ContextResolver content

**New beads:** Beads for each updated component's content leaf (via normal obsolete+create pipeline). Plus a bead for the migration tooling.

**Modified beads:** None structurally — only content leaf updates.

**Closed beads:** None.

**Estimated scope:** 4-5 sessions:
- Session 1: Schema changes (JSON schemas, schema.go structs, IdentityHash utility)
- Session 2: Merkle TreeBuilder + Validator IDValidator updates
- Session 3: Impact + Apply + Map updates
- Session 4: Migration script + run migration + validate + update bead-map + save snapshot
- Session 5: Integration testing across the full pipeline
