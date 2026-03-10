# Classification and Reporting Tests

Integration and acceptance tests for ActionClassifier (component 3) and ReportGenerator (component 4). These tests verify that matched, unmatched, and orphaned node results are correctly classified into create/close/review actions, and that the structured JSON impact report is correctly generated with accurate summary statistics.

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

Call `ClassifyActions(matches, unmatched, orphaned)`. Assert the returned `[]Action` contains exactly four actions:

| # | Type | BeadID | Module | Node | Impact | Reason |
|---|---|---|---|---|---|---|
| 1 | review | spex-001 | validator | SchemaChecker | arch_impl | Spec node modified (arch_impl): validator/SchemaChecker |
| 2 | review | spex-003 | merkle | Hash computation | impl_only | Spec node modified (impl_only): merkle/Hash computation |
| 3 | create | (empty) | validator | OrphanDetector | arch_impl | New spec node: validator/OrphanDetector |
| 4 | close | spex-010 | merkle | LegacyHasher | (empty) | Spec node removed: merkle/LegacyHasher |

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

Assert the action type is `"create"` — a modified spec node with no tracking bead needs a new bead created, per the decision table row: modified + no bead = create.

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

Assert the action type is `"review"` — per the decision table, added + existing bead = review for consistency.

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

Assert no action is generated — per the decision table, removed + no bead = no action (nothing to close).

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

Assert two separate `review` actions are generated, one for each bead ID (spex-001 and spex-005). Each bead that tracks the same spec node must be independently flagged for review.

### S6: Impact level appears in action metadata but does not change action type

Classify the same matched change three times, varying only the impact level (`impl_only`, `arch_impl`, `structural`). Assert all three produce action type `"review"` but with different `Impact` and `Reason` fields. Specifically verify the reason string includes the impact level:
- `"Spec node modified (impl_only): merkle/Hasher"`
- `"Spec node modified (arch_impl): merkle/Hasher"`
- `"Spec node modified (structural): merkle/Hasher"`

### S7: ReportGenerator produces valid JSON with correct structure

Call `GenerateReport(actions, &buf)` with the four actions from S1. Parse the output JSON and assert:

```json
{
  "creates": [
    {"type": "create", "module": "validator", "node": "OrphanDetector", "impact": "arch_impl", "reason": "New spec node: validator/OrphanDetector"}
  ],
  "closes": [
    {"type": "close", "bead_id": "spex-010", "module": "merkle", "node": "LegacyHasher", "reason": "Spec node removed: merkle/LegacyHasher"}
  ],
  "reviews": [
    {"type": "review", "bead_id": "spex-001", "module": "validator", "node": "SchemaChecker", "impact": "arch_impl", "reason": "Spec node modified (arch_impl): validator/SchemaChecker"},
    {"type": "review", "bead_id": "spex-003", "module": "merkle", "node": "Hash computation", "impact": "impl_only", "reason": "Spec node modified (impl_only): merkle/Hash computation"}
  ],
  "summary": {
    "create_count": 1,
    "close_count": 1,
    "review_count": 2
  }
}
```

### S8: ReportGenerator uses 2-space indentation

Call `GenerateReport` and inspect the raw bytes written to the writer. Assert the output uses 2-space indentation (not tabs, not 4 spaces). Verify by checking that the second line starts with two spaces.

### S9: ReportGenerator groups actions correctly

Provide six actions: 2 creates, 1 close, 3 reviews. Assert:
- `report.Creates` has length 2
- `report.Closes` has length 1
- `report.Reviews` has length 3
- `report.Summary.CreateCount == 2`
- `report.Summary.CloseCount == 1`
- `report.Summary.ReviewCount == 3`

### S10: Full pipeline — ClassifyActions into GenerateReport

Wire the two components together: pass NodeMatcher output through `ClassifyActions`, then pass the resulting actions through `GenerateReport`. Parse the JSON output and verify end-to-end correctness. This is the integration point between components 3 and 4.

## Edge Cases

### E1: Empty inputs produce empty report

Call `ClassifyActions(nil, nil, nil)`. Assert the result is an empty `[]Action`. Pass the empty actions to `GenerateReport`. Assert the JSON output has empty arrays and zero counts:

```json
{
  "creates": [],
  "closes": [],
  "reviews": [],
  "summary": {"create_count": 0, "close_count": 0, "review_count": 0}
}
```

### E2: ReportGenerator handles nil writer

Call `GenerateReport(actions, nil)`. Assert a non-nil error is returned rather than a panic.

### E3: Actions with empty strings in fields

Create an action where Module and Node are empty strings. Assert `GenerateReport` still produces valid JSON (the fields appear as empty strings, not null or omitted).

### E4: Very large action list

Generate 10,000 actions (mix of creates, closes, reviews). Assert `ClassifyActions` completes without error and `GenerateReport` produces valid JSON with correct summary counts. This validates that no O(n^2) algorithms are hiding in the pipeline.

### E5: Duplicate actions are preserved, not deduplicated

If the same spec node change appears in both `matches` and `unmatched` due to upstream bugs, assert that `ClassifyActions` produces actions for both entries. Deduplication is not the classifier's responsibility — it faithfully translates its inputs.

### E6: Report JSON is parseable by standard JSON parsers

Write the report output to a buffer, then unmarshal it back into an `ImpactReport` struct. Assert the round-trip produces identical data. This validates that the JSON is well-formed and the struct tags are correct.
