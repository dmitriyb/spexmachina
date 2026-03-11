# Bead Matching Tests

Integration and acceptance tests for BeadReader (component 1) and NodeMatcher (component 2). These tests verify that bead metadata is correctly read from the bead CLI and that changed spec nodes are deterministically correlated with existing beads.

## Setup

All scenarios share a common fixture layout:

- A spec tree with two modules: `validator` (components: SchemaChecker, DagChecker; impl_sections: Schema validation, Cycle detection) and `merkle` (components: Hasher, DiffEngine; impl_sections: Hash computation, Diff algorithm).
- A set of beads returned by the mock bead CLI, each carrying spec labels in the `labels` array as `key:value` strings:

```json
[
  {
    "id": "spex-001",
    "title": "Implement SchemaChecker",
    "labels": [
      "spec_module:validator",
      "spec_component:SchemaChecker",
      "spec_hash:abc123"
    ]
  },
  {
    "id": "spex-002",
    "title": "Implement Hasher",
    "labels": [
      "spec_module:merkle",
      "spec_component:Hasher",
      "spec_hash:def456"
    ]
  },
  {
    "id": "spex-003",
    "title": "Implement hash computation",
    "labels": [
      "spec_module:merkle",
      "spec_impl_section:Hash computation",
      "spec_hash:ghi789"
    ]
  },
  {
    "id": "spex-004",
    "title": "Unrelated task",
    "labels": ["team:backend"]
  }
]
```

- A merkle diff with classified changes, provided as the second input to NodeMatcher:

```json
[
  {"path": "validator/arch_schema_checker.md", "type": "modified", "impact": "arch_impl", "module": "validator"},
  {"path": "merkle/impl_hash_computation.md", "type": "modified", "impact": "impl_only", "module": "merkle"},
  {"path": "validator/arch_orphan_detector.md", "type": "added", "impact": "arch_impl", "module": "validator"},
  {"path": "merkle/arch_diff_engine.md", "type": "removed", "impact": "arch_impl", "module": "merkle"}
]
```

Tests use a mock `exec.CommandContext` that returns the fixture bead JSON instead of calling a real bead CLI binary.

## Scenarios

### S1: BeadReader extracts spec labels correctly

Call `ReadBeads(ctx, "br")` with the fixture JSON. Assert the returned `[]BeadSpec` contains exactly three entries (spex-001, spex-002, spex-003). The fourth bead (spex-004) has no spec labels and must be excluded. Verify each field:

| BeadSpec field | spex-001 | spex-002 | spex-003 |
|---|---|---|---|
| ID | "spex-001" | "spex-002" | "spex-003" |
| Module | "validator" | "merkle" | "merkle" |
| Component | "SchemaChecker" | "Hasher" | "" |
| ImplSection | "" | "" | "Hash computation" |
| SpecHash | "abc123" | "def456" | "ghi789" |

### S2: BeadReader returns empty slice when no beads have spec labels

Mock CLI returns beads with only non-spec labels (e.g., `["team:backend", "priority:high"]`). Assert `ReadBeads` returns `([]BeadSpec{}, nil)` — an empty slice, not an error.

### S3: NodeMatcher produces correct matched, unmatched, and orphaned lists

Call `MatchNodes(changes, beads, modules)` with the fixture data. Expected results:

- **Matched (1 entry):** `validator/arch_schema_checker.md` (modified) matches bead spex-001 because `spex-001.Module == "validator"` and `spex-001.Component == "SchemaChecker"` corresponds to filename `arch_schema_checker.md`.
- **Matched (1 entry):** `merkle/impl_hash_computation.md` (modified) matches bead spex-003 because `spex-003.Module == "merkle"` and `spex-003.ImplSection == "Hash computation"` corresponds to filename `impl_hash_computation.md`.
- **Unmatched (1 entry):** `validator/arch_orphan_detector.md` (added) has no bead with `Module == "validator"` and `Component == "OrphanDetector"`.
- **Orphaned (1 entry):** bead spex-002 references `merkle/Hasher`, but `merkle/arch_diff_engine.md` was removed (not `arch_hasher.md`). However, since no change removes `arch_hasher.md`, spex-002 is not orphaned in this scenario. Instead, `merkle/arch_diff_engine.md` (removed) has no matching bead and produces no orphan. Correct the expectation: no orphaned beads in this fixture because no bead references DiffEngine.

Revised expected results:
- Matched: 2 entries (SchemaChecker, Hash computation)
- Unmatched: 1 entry (OrphanDetector — added node, no bead)
- Unmatched-removed: 1 entry (DiffEngine — removed node, no bead referencing it, so no close action either)

### S4: NodeMatcher handles multiple beads per spec node

Add a second bead referencing the same spec node:

```json
{
  "id": "spex-005",
  "title": "Review SchemaChecker",
  "labels": [
    "spec_module:validator",
    "spec_component:SchemaChecker",
    "spec_hash:abc123"
  ]
}
```

Call `MatchNodes` with the modified change for `validator/arch_schema_checker.md`. Assert the resulting `Match.Beads` slice contains both spex-001 and spex-005.

### S5: NodeMatcher resolves filename slugs to component names via module.json

The filename `arch_schema_checker.md` must resolve to component name "SchemaChecker" by looking up the module's component list. Provide a module where the component name uses mixed case ("SchemaChecker") that does not trivially match the snake_case filename. Assert the match succeeds by confirming the module.json lookup correctly maps `arch_schema_checker.md` to the component whose `content` field equals `"arch_schema_checker.md"`.

### S6: Structural changes flag all beads in affected module

Add a structural change to the diff:

```json
{"path": "validator/module.json", "type": "modified", "impact": "structural", "module": "validator"}
```

Assert that all beads with `spec_module:validator` appear in the matched list, regardless of which specific component or impl_section they reference. Bead spex-001 (SchemaChecker) should be matched to this structural change.

### S7: Deterministic matching — identical inputs always produce identical output

Run `MatchNodes` twice with the same inputs (shuffling the order of the beads slice between runs). Assert the output slices (matched, unmatched, orphaned) are identical in both content and order. This validates requirement 5 (deterministic matching).

## Edge Cases

### E1: Bead CLI not installed

Mock `exec.CommandContext` to return `exec.ErrNotFound`. Assert `ReadBeads` returns an error wrapping the message `"impact: read beads: ..."`.

### E2: Bead CLI returns invalid JSON

Mock CLI returns `"not json at all"`. Assert `ReadBeads` returns a JSON parse error wrapped with `"impact: read beads: ..."`.

### E3: Bead CLI returns empty list

Mock CLI returns `[]`. Assert `ReadBeads` returns `([]BeadSpec{}, nil)`.

### E4: Labels with colons in values

A bead label `"spec_component:Foo:Bar"` should parse the key as `"spec_component"` and the value as `"Foo:Bar"` (split on first colon only). Assert the `Component` field is `"Foo:Bar"`.

### E5: Change path with no corresponding module in the modules map

A change path referencing a module name not present in the `modules` parameter. Assert `MatchNodes` treats this as an unmatched change and does not panic.

### E6: Bead references a module that has no changes

A bead with `spec_module:render` exists, but no changes affect the `render` module. Assert this bead does not appear in any output list (matched, unmatched, or orphaned) — it is simply unaffected.
