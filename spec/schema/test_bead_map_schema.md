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
      "spec_node_id": "schema/component/1",
      "bead_id": "abc-123",
      "bead_type": "feature",
      "module": "schema",
      "component": "ProjectSchema",
      "content_file": "spec/schema/arch_project_schema.md",
      "spec_hash": "a1b2c3d4"
    },
    {
      "id": 2,
      "spec_node_id": "validator/component/1",
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

### S6: Invalid spec_node_id pattern fails

**Input:** Record with `spec_node_id: "invalid-format"`.
**Expected:** Validation fails. The pattern `^[a-z_]+/(component|impl_section|data_flow|test_section)/[0-9]+$` rejects the value.

### S7: Valid spec_node_id patterns pass

**Input:** Records with:
- `"schema/component/1"` — passes
- `"map/impl_section/3"` — passes
- `"render/data_flow/1"` — passes
- `"validator/test_section/2"` — passes

**Expected:** All pass validation.

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
**Expected:** Depends on schema — if `minLength: 1` is set, fails. If not, passes. Documents the boundary.

### E2: spec_node_id with uppercase module name

**Input:** `spec_node_id: "Schema/component/1"`.
**Expected:** Validation fails. Pattern requires lowercase `[a-z_]+`.

### E3: spec_node_id with unknown type segment

**Input:** `spec_node_id: "schema/requirement/1"`.
**Expected:** Validation fails. Pattern enum only allows `component|impl_section|data_flow|test_section`.
