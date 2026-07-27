# Classification and Reporting Tests

Integration and acceptance tests for ActionClassifier (component 3) and ReportGenerator (component 4). These tests verify that matched, unmatched, and orphaned node results are correctly classified into create/obsolete actions, and that the structured JSON impact report is correctly generated with accurate summary statistics.

## Setup

All scenarios build on the output of NodeMatcher. The fixture data represents a typical diff cycle:

**Matched entries (modified spec nodes with existing beads):**

```go
matches := []Match{
    {
        Change: ClassifiedChange{
            Path:   "validator/arch_schema_checker.md",
            Type:   "modified",
            Impact: "arch_impl",
            Module: "validator",
        },
        Beads: []BeadSpec{
            {ID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
        },
    },
    {
        Change: ClassifiedChange{
            Path:   "merkle/impl_hash_computation.md",
            Type:   "modified",
            Impact: "impl_only",
            Module: "merkle",
        },
        Beads: []BeadSpec{
            {ID: "spex-003", Module: "merkle", ImplSection: "Hash computation", SpecHash: "ghi789"},
        },
    },
}
```

**Unmatched entries (new spec nodes without beads):**

```go
unmatched := []Unmatched{
    {
        Change: ClassifiedChange{
            Path:   "validator/arch_coupled_section_checker.md",
            Type:   "added",
            Impact: "arch_impl",
            Module: "validator",
        },
    },
}
```

**Orphaned entries (beads referencing removed spec nodes):**

```go
orphaned := []Orphaned{
    {
        Bead: BeadSpec{ID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000"},
    },
}
```

## Scenarios

### S1: ActionClassifier produces correct actions for each category

Call `ClassifyActions(matches, unmatched, orphaned)`. Assert the returned `[]Action` contains exactly six actions:

| # | Type | BeadID | Module | Node | Reason |
|---|------|--------|--------|------|--------|
| 1 | obsolete | spex-001 | validator | SchemaChecker | Spec node modified: validator/SchemaChecker |
| 2 | create | (empty) | validator | SchemaChecker | Spec node modified (new): validator/SchemaChecker |
| 3 | obsolete | spex-003 | merkle | Hash computation | Spec node modified: merkle/Hash computation |
| 4 | create | (empty) | merkle | Hash computation | Spec node modified (new): merkle/Hash computation |
| 5 | create | (empty) | validator | CoupledSectionChecker | New spec node: validator/CoupledSectionChecker |
| 6 | obsolete | spex-010 | merkle | LegacyHasher | Spec node removed: merkle/LegacyHasher |

Modified nodes produce TWO actions (obsolete old + create new). Added nodes produce one create. Removed nodes produce one obsolete.

### S2: ActionClassifier handles modified node without a matching bead

Provide a Match-like scenario where a spec node is modified but has no bead (this arrives as an Unmatched entry with type "modified"):

```go
unmatched := []Unmatched{
    {
        Change: ClassifiedChange{
            Path:   "render/impl_markdown_rendering.md",
            Type:   "modified",
            Impact: "impl_only",
            Module: "render",
        },
    },
}
```

Assert the action type is `"create"` — a modified spec node with no tracking bead needs a new bead created. No obsolete action is generated since there is no old bead to obsolete.

### S3: ActionClassifier handles added node with an existing bead (unexpected case)

Provide a Match entry where the change type is "added" but a bead already exists:

```go
matches := []Match{
    {
        Change: ClassifiedChange{
            Path:   "proposal/arch_registrar.md",
            Type:   "added",
            Impact: "arch_impl",
            Module: "proposal",
        },
        Beads: []BeadSpec{
            {ID: "spex-020", Module: "proposal", Component: "Registrar", SpecHash: "old111"},
        },
    },
}
```

Assert the actions are `"obsolete"` (old bead) + `"create"` (new bead) — even for an "added" change type, the presence of an existing bead triggers the obsolete+create flow for consistency.

### S4: ActionClassifier handles removed node without a matching bead

Provide an Unmatched entry with type "removed":

```go
unmatched := []Unmatched{
    {
        Change: ClassifiedChange{
            Path:   "schema/arch_deprecated_loader.md",
            Type:   "removed",
            Impact: "arch_impl",
            Module: "schema",
        },
    },
}
```

Assert no action is generated — removed + no bead = no action (nothing to obsolete).

### S5: ActionClassifier handles multiple beads per matched node

Provide a Match entry with two beads:

```go
matches := []Match{
    {
        Change: ClassifiedChange{
            Path:   "validator/arch_schema_checker.md",
            Type:   "modified",
            Impact: "arch_impl",
            Module: "validator",
        },
        Beads: []BeadSpec{
            {ID: "spex-001", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
            {ID: "spex-005", Module: "validator", Component: "SchemaChecker", SpecHash: "abc123"},
        },
    },
}
```

Assert four actions are generated: two obsolete actions (one for each old bead) and two create actions (new beads to replace them). Each old bead is independently obsoleted.

### S5b: ActionClassifier handles removed node with closed bead (cleanup)

Provide an Orphaned entry where the bead status is "closed":

```go
orphaned := []Orphaned{
    {
        Bead: BeadSpec{ID: "spex-010", Module: "merkle", Component: "LegacyHasher", SpecHash: "zzz000", Status: "closed"},
    },
}
```

Assert two actions are generated:
1. `"obsolete"` with BeadID `"spex-010"` and reason `"Spec node removed: merkle/LegacyHasher"`
2. `"create"` with reason `"Code cleanup: merkle/LegacyHasher"` — a cleanup bead for deleting shipped code

### S5c: ActionClassifier handles removed node with open bead (no cleanup)

Provide an Orphaned entry where the bead status is "open":

```go
orphaned := []Orphaned{
    {
        Bead: BeadSpec{ID: "spex-011", Module: "merkle", Component: "DraftHasher", SpecHash: "yyy000", Status: "open"},
    },
}
```

Assert only one action is generated: `"obsolete"` with BeadID `"spex-011"`. No cleanup bead is created because no code has shipped to main.

### S6: Impact level appears in action metadata but does not change action type

Classify the same matched change three times, varying only the impact level (`impl_only`, `arch_impl`, `structural`). Assert all three produce the same action types (obsolete + create) regardless of impact level. Verify the reason string includes the impact level for traceability.

### S7: ReportGenerator produces valid JSON with correct structure

Call `GenerateReport(actions, &buf)` with the six actions from S1. Parse the output JSON and assert:

```json
{
  "creates": [
    {"type": "create", "module": "validator", "node": "SchemaChecker", "reason": "Spec node modified (new): validator/SchemaChecker"},
    {"type": "create", "module": "merkle", "node": "Hash computation", "reason": "Spec node modified (new): merkle/Hash computation"},
    {"type": "create", "module": "validator", "node": "CoupledSectionChecker", "reason": "New spec node: validator/CoupledSectionChecker"}
  ],
  "obsoletes": [
    {"type": "obsolete", "bead_id": "spex-001", "module": "validator", "node": "SchemaChecker", "reason": "Spec node modified: validator/SchemaChecker"},
    {"type": "obsolete", "bead_id": "spex-003", "module": "merkle", "node": "Hash computation", "reason": "Spec node modified: merkle/Hash computation"},
    {"type": "obsolete", "bead_id": "spex-010", "module": "merkle", "node": "LegacyHasher", "reason": "Spec node removed: merkle/LegacyHasher"}
  ],
  "summary": {
    "create_count": 3,
    "obsolete_count": 3
  }
}
```

### S8: ReportGenerator uses 2-space indentation

Call `GenerateReport` and inspect the raw bytes written to the writer. Assert the output uses 2-space indentation (not tabs, not 4 spaces). Verify by checking that the second line starts with two spaces.

### S9: ReportGenerator groups actions correctly

Provide five actions: 2 creates, 3 obsoletes. Assert:
- `report.Creates` has length 2
- `report.Obsoletes` has length 3
- `report.Summary.CreateCount == 2`
- `report.Summary.ObsoleteCount == 3`

### S10: Full pipeline — ClassifyActions into GenerateReport

Wire the two components together: pass NodeMatcher output through `ClassifyActions`, then pass the resulting actions through `GenerateReport`. Parse the JSON output and verify end-to-end correctness. This is the integration point between components 3 and 4.

## Edge Cases

### E1: Empty inputs produce empty report

Call `ClassifyActions(nil, nil, nil)`. Assert the result is an empty `[]Action`. Pass the empty actions to `GenerateReport`. Assert the JSON output has empty arrays and zero counts:

```json
{
  "creates": [],
  "obsoletes": [],
  "summary": {"create_count": 0, "obsolete_count": 0}
}
```

### E2: ReportGenerator handles nil writer

Call `GenerateReport(actions, nil)`. Assert a non-nil error is returned rather than a panic.

### E3: Actions with empty strings in fields

Create an action where Module and Node are empty strings. Assert `GenerateReport` still produces valid JSON (the fields appear as empty strings, not null or omitted).

### E4: Very large action list

Generate 10,000 actions (mix of creates and obsoletes). Assert `ClassifyActions` completes without error and `GenerateReport` produces valid JSON with correct summary counts. This validates that no O(n^2) algorithms are hiding in the pipeline.

### E5: Duplicate actions are preserved, not deduplicated

If the same spec node change appears in both `matches` and `unmatched` due to upstream bugs, assert that `ClassifyActions` produces actions for both entries. Deduplication is not the classifier's responsibility — it faithfully translates its inputs.

### E6: Report JSON is parseable by standard JSON parsers

Write the report output to a buffer, then unmarshal it back into an `ImpactReport` struct. Assert the round-trip produces identical data. This validates that the JSON is well-formed and the struct tags are correct.

## Dependency Collection Scenarios

These scenarios test the spec-graph dependency collection that runs alongside action classification. For each create action with `node_type=component`, ActionClassifier walks the spec graph and records identity hashes of spec nodes the new bead will depend on in `DepSpecNodeIDs`. No mapping-store lookup and no bead-status filtering happens here — emit's Resolver classifies each identity hash into a `ref:op` / `ref:bead` / `ref:spec_node` at emit time. This defers bead-ID resolution to the point where the current batch's op IDs are known, fixing the broken-dep-graph bug where an obsoleted-in-the-same-batch bead was picked up as a dependency.

### D1: Component `uses` edge collects the sibling's identity hash

Given a create action for component X in module A, where X `uses: [Y]` and Y is another component in module A with identity hash `id_Y`.

When `ClassifyActions` runs:

Then X's `DepSpecNodeIDs` contains `id_Y`. A self-reference (X's own identity hash) is filtered out.

### D2: Bead status is irrelevant to collection

Given a create action for component X whose `uses: [Y]`, and the mapping records for Y show a closed bead.

When `ClassifyActions` runs:

Then X's `DepSpecNodeIDs` still contains `id_Y`. The classifier does not peek at `Records.BeadStatus` — filtering "already-satisfied" deps is emit's responsibility.

### D3: Transitive `requires_module` collects every reachable module's components

Given a create action for a component in module A, where A `requires_module: [B]` and B `requires_module: [C]`. Module B has component CompB; module C has component CompC.

When `ClassifyActions` runs:

Then the create action's `DepSpecNodeIDs` contains both `id_CompB` and `id_CompC`. Transitive module reachability pulls in every component identity hash along the closure.

### D4: Component `uses` edges are NOT transitive

Given component X `uses: [Y]` and Y `uses: [Z]`.

When a create action for X is classified:

Then `DepSpecNodeIDs` contains only `id_Y`. Z is not included — component `uses` walks one hop only, matching PreflightChecker semantics.

### D5: Mixed `uses` and `requires_module` are merged and deduplicated

Given component X in module A where X `uses: [Y]` (Y sits in module A), and A `requires_module: [B]` with B containing CompB.

When `ClassifyActions` runs:

Then `DepSpecNodeIDs` contains both `id_Y` and `id_CompB`. Duplicates are removed — if the walks hit the same identity hash twice, it appears once.

### D6: No edges yields empty `DepSpecNodeIDs`

Given a create action for a component with no `uses` edges in a module with no `requires_module` edges.

When `ClassifyActions` runs:

Then `DepSpecNodeIDs` is empty (length zero).

### D7: `requires_module` cycle terminates and collects each reachable module once

Given module A `requires_module: [B]` and module B `requires_module: [A]` (a cycle — invalid but must not hang).

When `ClassifyActions` runs for a component in A:

Then the walk terminates via visited-set tracking and `DepSpecNodeIDs` includes `id_CompB` (B is reachable from A). The cycle does not cause infinite recursion.

### D8: Data_flow add-on — component gains the flow's identity hash when both are in the same batch

Given a data_flow F in module M with `uses: [X]` (listing component X), and both F and X are in the current batch of changes (both produce create actions).

When `ClassifyActions` runs:

Then the create action for X has `DepSpecNodeIDs` containing `id_F`. This ensures emit's topological sort places the data_flow op first and the component ops gain a `ref:op` dependency on it (the contract-layer guarantee).

### D9: Data_flow add-on does not apply to components the flow does not list

Given data_flow F with `uses: [X]` and component Y (not listed in F's `uses`), both F and Y are in the same batch.

When `ClassifyActions` runs:

Then Y's `DepSpecNodeIDs` does NOT contain `id_F`. Only components explicitly listed in the flow's `uses` pick up the add-on.

### D10: Data_flow add-on fires only for flows in the current batch

Given data_flow F exists in the spec graph with `uses: [X]`, but F is not in the current batch of changes (it was created in a previous run). Only X is in the batch.

When `ClassifyActions` runs:

Then X's `DepSpecNodeIDs` does NOT contain `id_F`. Pre-existing data_flow dependencies are emit's concern — it resolves them to `ref:bead` or `ref:spec_node` from the mapping store. Only same-batch flows need the add-on.

### D11: Non-component creates do not walk `uses` / `requires_module`

Given a create action for a data_flow (or test_section) in module M, where M `requires_module: [A]`.

When `ClassifyActions` runs:

Then the create action's `DepSpecNodeIDs` is empty. The `uses` / `requires_module` walk is component-only. Data_flow and test_section creates carry no spec-graph deps from classification — their ordering inside the batch is driven by the data_flow add-on (D8) applied to the components on the other side.

### D12: Nil spec graph leaves `DepSpecNodeIDs` empty

Given `ClassifyActions(nil, ...)` — the caller does not supply a spec graph (e.g., a legacy call site).

When classification runs:

Then every create action has an empty `DepSpecNodeIDs`. No dependency walk happens. Classification itself (action types, reasons, bead IDs) still works.

### D13: Obsolete actions never carry `DepSpecNodeIDs`

Given any obsolete action (modified-with-bead, removed-with-bead, cleanup pair).

When `ClassifyActions` runs:

Then the obsolete action's `DepSpecNodeIDs` is empty. Dependency information belongs on creates only — obsolete actions describe work on an existing bead and inherit its existing graph position.

### D14: ReportGenerator serializes `dep_spec_node_ids` for creates

Given a create action with `DepSpecNodeIDs: ["abc123...", "def456..."]` (two identity hashes).

When `GenerateReport` serializes the action:

Then the JSON output includes:
```json
{
  "dep_spec_node_ids": ["abc123...", "def456..."],
  ...
}
```

When `DepSpecNodeIDs` is empty or nil, the field is omitted via `omitempty`.
