# Bead Map Schema Tests

Integration and acceptance tests for BeadMapSchema (component 4). These tests verify that the `.bead-map.json` JSON Schema correctly accepts valid mapping files, rejects invalid ones, and enforces all field constraints.

## Setup

The JSON Schema validator is initialized with the bead-map schema loaded from `BeadMapSchema()`.

**Fixture: valid minimal .bead-map.json:**
```json
{
  "next_id": 1,
  "records": []
}
```

**Fixture: valid full .bead-map.json:**
```json
{
  "next_id": 3,
  "records": [
    {
      "id": 1,
      "spec_node_id": "79946d618829",
      "bead_id": "abc-123",
      "bead_type": "feature",
      "module": "schema",
      "component": "ProjectSchema",
      "content_file": "spec/schema/arch_project_schema.md",
      "spec_hash": "a1b2c3d4"
    },
    {
      "id": 2,
      "spec_node_id": "651d5315eebf",
      "bead_id": "def-456",
      "bead_type": "task",
      "module": "validator",
      "component": "SchemaChecker",
      "content_file": "spec/validator/arch_schema_checker.md",
      "spec_hash": "e5f6g7h8",
      "bead_status": "closed"
    }
  ]
}
```

## Scenarios

### S1: Minimal .bead-map.json passes validation

**Input:** `{ "next_id": 1, "records": [] }`
**Expected:** Validation passes. Empty records array is valid.

### S2: Full .bead-map.json passes validation

**Input:** The full fixture above.
**Expected:** Validation passes. All required and optional fields are correctly typed.

### S3: Missing next_id fails

**Input:** `{ "records": [] }`
**Expected:** Validation fails. Missing required `next_id`.

### S4: Missing records fails

**Input:** `{ "next_id": 1 }`
**Expected:** Validation fails. Missing required `records`.

### S5: Record missing required field fails

**Input:** Record with `id` but missing `spec_node_id`:
```json
{
  "next_id": 1,
  "records": [{ "id": 1, "bead_id": "x", "bead_type": "task", "module": "m", "component": "C", "content_file": "f.md", "spec_hash": "h" }]
}
```
**Expected:** Validation fails. Error references `records/0`, missing `spec_node_id`.

### S6: Empty spec_node_id fails

**Input:** Record with `spec_node_id: ""`.
**Expected:** Validation fails on `minLength`. That is the only constraint the schema puts on the field — there is no pattern.

### S7: spec_node_id accepts both identities it has to carry

**Input:** Records with:
- `"79946d618829"` — a 12-character identity hash, what a component, data_flow or test_section record carries
- `"2026-07-25-declarative-spec-contracts"` — a proposal reference, what a proposal epic record carries

**Expected:** Both pass. One array holds records keyed two different ways, so the field is typed as a non-empty string and the shape is left to whoever writes the record.

### S8: next_id below minimum fails

**Input:** `{ "next_id": 0, "records": [] }`
**Expected:** Validation fails. `next_id` must be >= 1.

### S9: bead_status is optional

**Input:** A valid record without `bead_status`.
**Expected:** Validation passes. `bead_status` is not in the required array.

### S10: Extra fields rejected

**Input:** Record with an extra field `"priority": "high"`.
**Expected:** Validation fails. `additionalProperties: false` on record objects rejects unknown fields.

## Edge Cases

### E1: Empty string for required string fields

**Input:** Record with `bead_id: ""`.
**Expected:** Validation fails. `minLength: 1` is set on `spec_node_id`, `bead_id`, `bead_type` and `component`; the remaining string fields carry no minimum and accept the empty string, which is how a proposal epic record leaves `module`, `content_file` and `spec_hash` blank.

### E2: node_type outside the enum fails

**Input:** Record with `node_type: "requirement"`.
**Expected:** Validation fails. The enum is `proposal`, `component`, `data_flow`, `test_section` and nothing else — this is the one place the schema names which spec node types a record may stand for.

### E3: node_type absent passes

**Input:** A record with no `node_type` at all.
**Expected:** Validation passes. The property is optional, so a record can omit it; it is present only where a proposal epic has to be told apart from a node record.
