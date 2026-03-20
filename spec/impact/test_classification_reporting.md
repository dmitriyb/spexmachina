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
            Path:   "validator/arch_orphan_detector.md",
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
| 5 | create | (empty) | validator | OrphanDetector | New spec node: validator/OrphanDetector |
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
    {"type": "create", "module": "validator", "node": "OrphanDetector", "reason": "New spec node: validator/OrphanDetector"}
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
