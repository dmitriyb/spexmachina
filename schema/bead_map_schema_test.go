package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileBeadMapSchema compiles the embedded bead-map schema for validation.
func compileBeadMapSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal bead-map schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("bead-map.schema.json", doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("bead-map.schema.json")
	if err != nil {
		t.Fatalf("compile bead-map schema: %v", err)
	}
	return sch
}

// validateBeadMap validates a JSON document against the bead-map schema.
func validateBeadMap(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

// --- Schema Loading Tests ---

func TestFR7_BeadMapSchemaLoadsValidJSON(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("BeadMapSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("BeadMapSchema() is not valid JSON: %v", err)
	}
}

func TestFR7_BeadMapSchemaMetaProperties(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tests := []struct {
		name  string
		check func() bool
	}{
		{"$schema is 2020-12", func() bool { return raw["$schema"] == "https://json-schema.org/draft/2020-12/schema" }},
		{"$id is correct", func() bool { return raw["$id"] == "https://spexmachina.dev/schema/bead-map.json" }},
		{"title is correct", func() bool { return raw["title"] == "Spex Machina Bead Map" }},
		{"type is object", func() bool { return raw["type"] == "object" }},
		{"additionalProperties is false", func() bool { return raw["additionalProperties"] == false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check() {
				t.Fatal("check failed")
			}
		})
	}

	// Check required array
	req := raw["required"].([]any)
	reqSet := make(map[string]bool)
	for _, v := range req {
		reqSet[v.(string)] = true
	}
	for _, key := range []string{"next_id", "records"} {
		if !reqSet[key] {
			t.Fatalf("required array should contain %q", key)
		}
	}

	// Check properties keys
	props := raw["properties"].(map[string]any)
	for _, key := range []string{"next_id", "records"} {
		if props[key] == nil {
			t.Fatalf("bead-map schema missing property %q", key)
		}
	}

	// Check $defs
	defs := raw["$defs"].(map[string]any)
	if defs["record"] == nil {
		t.Fatal("bead-map schema missing $def 'record'")
	}
}

func TestFR7_BeadMapSchemaRecordDefinesAllFields(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	defs := raw["$defs"].(map[string]any)
	recordDef := defs["record"].(map[string]any)
	recordProps := recordDef["properties"].(map[string]any)

	// All required fields from requirement 7
	for _, key := range []string{"id", "spec_node_id", "bead_id", "bead_type", "module", "component", "content_file", "spec_hash"} {
		if recordProps[key] == nil {
			t.Fatalf("bead-map record missing required property %q", key)
		}
	}

	// Optional field
	if recordProps["bead_status"] == nil {
		t.Fatal("bead-map record missing optional property 'bead_status'")
	}

	// Verify required array matches
	reqArr := recordDef["required"].([]any)
	reqSet := make(map[string]bool)
	for _, v := range reqArr {
		reqSet[v.(string)] = true
	}
	for _, key := range []string{"id", "spec_node_id", "bead_id", "bead_type", "module", "component", "content_file", "spec_hash"} {
		if !reqSet[key] {
			t.Fatalf("record required array should contain %q", key)
		}
	}
	if reqSet["bead_status"] {
		t.Fatal("bead_status should not be in required array")
	}
}

func TestFR7_BeadMapSchemaSpecNodeIDIsNonEmpty(t *testing.T) {
	// Historically spec_node_id was restricted to a 12-char hex pattern.
	// With proposal epic records it now also carries human-readable proposal
	// references, so the schema only enforces a non-empty string here.
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	defs := raw["$defs"].(map[string]any)
	recordDef := defs["record"].(map[string]any)
	recordProps := recordDef["properties"].(map[string]any)
	specNodeDef := recordProps["spec_node_id"].(map[string]any)

	if _, hasPattern := specNodeDef["pattern"]; hasPattern {
		t.Fatalf("spec_node_id pattern was relaxed for proposal epic records; did not expect a pattern constraint")
	}
	minLen, ok := specNodeDef["minLength"].(float64)
	if !ok {
		t.Fatalf("spec_node_id should have minLength; got %v", specNodeDef["minLength"])
	}
	if int(minLen) != 1 {
		t.Fatalf("spec_node_id minLength: want 1, got %d", int(minLen))
	}
}

func TestFR7_BeadMapSchemaRejectsAdditionalPropertiesAtAllLevels(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Top level
	if raw["additionalProperties"] != false {
		t.Fatal("top-level additionalProperties should be false")
	}

	// Record level
	defs := raw["$defs"].(map[string]any)
	recordDef := defs["record"].(map[string]any)
	if recordDef["additionalProperties"] != false {
		t.Fatal("record additionalProperties should be false")
	}
}

// --- Validation Tests ---

func TestFR7_MinimalBeadMapPasses(t *testing.T) {
	sch := compileBeadMapSchema(t)
	data := readTestdata(t, "minimal_bead_map.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("minimal bead-map should pass validation: %v", err)
	}
}

func TestFR7_FullBeadMapPasses(t *testing.T) {
	sch := compileBeadMapSchema(t)
	data := readTestdata(t, "valid_bead_map.json")
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := sch.Validate(v); err != nil {
		t.Fatalf("full bead-map should pass validation: %v", err)
	}
}

func TestFR7_MissingRequiredFields(t *testing.T) {
	sch := compileBeadMapSchema(t)

	tests := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{
			"missing next_id",
			`{"records": []}`,
			"next_id",
		},
		{
			"missing records",
			`{"next_id": 1}`,
			"records",
		},
		{
			"record missing id",
			`{"next_id": 1, "records": [{"spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
			"id",
		},
		{
			"record missing spec_node_id",
			`{"next_id": 1, "records": [{"id": 1, "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
			"spec_node_id",
		},
		{
			"record missing bead_id",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
			"bead_id",
		},
		{
			"record missing bead_type",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
			"bead_type",
		},
		{
			"record missing module",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
			"module",
		},
		{
			"record missing component",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "content_file": "f.md", "spec_hash": "h"}]}`,
			"component",
		},
		{
			"record missing content_file",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "spec_hash": "h"}]}`,
			"content_file",
		},
		{
			"record missing spec_hash",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md"}]}`,
			"spec_hash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBeadMap(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for missing %s, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error should reference %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestFR7_SpecNodeIDNonEmptyValidation(t *testing.T) {
	sch := compileBeadMapSchema(t)

	validRecord := func(specNodeID string) string {
		return `{"next_id": 1, "records": [{"id": 1, "spec_node_id": "` + specNodeID + `", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`
	}

	t.Run("valid values", func(t *testing.T) {
		valid := []string{
			"a1b2c3d4e5f6",                       // typical identity hash
			"2026-04-12-data-flow-contract-layer", // proposal reference used on epic records
			"123456789012",                       // numeric hex
		}
		for _, id := range valid {
			t.Run(id, func(t *testing.T) {
				err := validateBeadMap(t, sch, validRecord(id))
				if err != nil {
					t.Fatalf("spec_node_id %q should pass: %v", id, err)
				}
			})
		}
	})

	t.Run("empty rejected", func(t *testing.T) {
		err := validateBeadMap(t, sch, validRecord(""))
		if err == nil {
			t.Fatal("empty spec_node_id should fail minLength validation")
		}
	})
}

func TestFR7_NextIDMinimum(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("next_id 1 passes", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 1, "records": []}`)
		if err != nil {
			t.Fatalf("next_id=1 should pass: %v", err)
		}
	})

	t.Run("next_id 0 fails", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 0, "records": []}`)
		if err == nil {
			t.Fatal("next_id=0 should fail")
		}
	})

	t.Run("negative next_id fails", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": -1, "records": []}`)
		if err == nil {
			t.Fatal("negative next_id should fail")
		}
	})
}

func TestFR7_RecordIDMinimum(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("record id 0 fails", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 1, "records": [{"id": 0, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`)
		if err == nil {
			t.Fatal("record id=0 should fail")
		}
	})
}

func TestFR7_EmptyStringFieldsFail(t *testing.T) {
	// module, content_file, and spec_hash are allowed to be empty on proposal
	// epic records, so they are not asserted here.
	sch := compileBeadMapSchema(t)

	fields := []struct {
		name string
		doc  string
	}{
		{
			"empty bead_id",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
		},
		{
			"empty bead_type",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`,
		},
		{
			"empty component",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "", "content_file": "f.md", "spec_hash": "h"}]}`,
		},
	}
	for _, tt := range fields {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBeadMap(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
		})
	}
}

func TestFR7_ExtraFieldsRejected(t *testing.T) {
	sch := compileBeadMapSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{
			"extra field at top level",
			`{"next_id": 1, "records": [], "version": "1.0"}`,
		},
		{
			"extra field at record level",
			`{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h", "priority": 1}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBeadMap(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for extra field in %s, got nil", tt.name)
			}
		})
	}
}

func TestFR7_BeadStatusOptional(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("without bead_status passes", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h"}]}`)
		if err != nil {
			t.Fatalf("record without bead_status should pass: %v", err)
		}
	})

	t.Run("with bead_status passes", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 1, "records": [{"id": 1, "spec_node_id": "a1b2c3d4e5f6", "bead_id": "abc", "bead_type": "task", "module": "m", "component": "c", "content_file": "f.md", "spec_hash": "h", "bead_status": "closed"}]}`)
		if err != nil {
			t.Fatalf("record with bead_status should pass: %v", err)
		}
	})
}

func TestFR7_WrongTopLevelType(t *testing.T) {
	sch := compileBeadMapSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{"array", `[]`},
		{"string", `"just a string"`},
		{"number", `42`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tt.doc), &v); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if err := sch.Validate(v); err == nil {
				t.Fatalf("expected validation error for %s top-level type, got nil", tt.name)
			}
		})
	}
}

func TestFR7_NextIDWrongType(t *testing.T) {
	sch := compileBeadMapSchema(t)

	tests := []struct {
		name string
		doc  string
	}{
		{"string next_id", `{"next_id": "one", "records": []}`},
		{"float next_id", `{"next_id": 1.5, "records": []}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBeadMap(t, sch, tt.doc)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
		})
	}
}

func TestFR7_NullFieldFails(t *testing.T) {
	sch := compileBeadMapSchema(t)
	err := validateBeadMap(t, sch, `{"next_id": 1, "records": null}`)
	if err == nil {
		t.Fatal("expected validation error for null records, got nil")
	}
}

// --- Go Type Tests ---

func TestFR7_BeadMapRoundTrip(t *testing.T) {
	data := readTestdata(t, "valid_bead_map.json")
	var bm BeadMap
	if err := json.Unmarshal(data, &bm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if bm.NextID < 1 {
		t.Fatalf("NextID should be >= 1, got %d", bm.NextID)
	}
	if len(bm.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(bm.Records))
	}
	if bm.Records[0].BeadType != "task" {
		t.Fatalf("expected bead_type 'task', got %q", bm.Records[0].BeadType)
	}
	if bm.Records[1].BeadStatus != "closed" {
		t.Fatalf("expected bead_status 'closed', got %q", bm.Records[1].BeadStatus)
	}

	// Round-trip
	out, err := json.Marshal(&bm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var bm2 BeadMap
	if err := json.Unmarshal(out, &bm2); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}

	if bm.NextID != bm2.NextID {
		t.Fatalf("round-trip NextID mismatch: want %d, got %d", bm.NextID, bm2.NextID)
	}
	if len(bm.Records) != len(bm2.Records) {
		t.Fatalf("round-trip records length mismatch: want %d, got %d", len(bm.Records), len(bm2.Records))
	}
	for i, r := range bm.Records {
		r2 := bm2.Records[i]
		if r.ID != r2.ID {
			t.Fatalf("record[%d] ID mismatch: want %d, got %d", i, r.ID, r2.ID)
		}
		if r.SpecNodeID != r2.SpecNodeID {
			t.Fatalf("record[%d] SpecNodeID mismatch: want %q, got %q", i, r.SpecNodeID, r2.SpecNodeID)
		}
		if r.BeadID != r2.BeadID {
			t.Fatalf("record[%d] BeadID mismatch: want %q, got %q", i, r.BeadID, r2.BeadID)
		}
		if r.BeadType != r2.BeadType {
			t.Fatalf("record[%d] BeadType mismatch: want %q, got %q", i, r.BeadType, r2.BeadType)
		}
		if r.Module != r2.Module {
			t.Fatalf("record[%d] Module mismatch: want %q, got %q", i, r.Module, r2.Module)
		}
		if r.Component != r2.Component {
			t.Fatalf("record[%d] Component mismatch: want %q, got %q", i, r.Component, r2.Component)
		}
		if r.ContentFile != r2.ContentFile {
			t.Fatalf("record[%d] ContentFile mismatch: want %q, got %q", i, r.ContentFile, r2.ContentFile)
		}
		if r.SpecHash != r2.SpecHash {
			t.Fatalf("record[%d] SpecHash mismatch: want %q, got %q", i, r.SpecHash, r2.SpecHash)
		}
		if r.BeadStatus != r2.BeadStatus {
			t.Fatalf("record[%d] BeadStatus mismatch: want %q, got %q", i, r.BeadStatus, r2.BeadStatus)
		}
	}
}

func TestFR7_BeadMapMinimalRoundTrip(t *testing.T) {
	data := readTestdata(t, "minimal_bead_map.json")
	var bm BeadMap
	if err := json.Unmarshal(data, &bm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if bm.NextID != 1 {
		t.Fatalf("expected NextID=1, got %d", bm.NextID)
	}
	if len(bm.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(bm.Records))
	}
}

func TestFR7_BeadMapOmitsEmptyBeadStatus(t *testing.T) {
	bm := BeadMap{
		NextID: 1,
		Records: []BeadMapRecord{
			{
				ID:          1,
				SpecNodeID:  "a1b2c3d4e5f6",
				BeadID:      "abc",
				BeadType:    "task",
				Module:      "m",
				Component:   "c",
				ContentFile: "f.md",
				SpecHash:    "h",
			},
		},
	}
	out, err := json.Marshal(&bm)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), "bead_status") {
		t.Fatal("empty bead_status should be omitted from JSON output")
	}
}

// --- Idempotency and Concurrency ---

func TestFR7_BeadMapSchemaIdempotent(t *testing.T) {
	data1, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	data2, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("BeadMapSchema() not idempotent")
	}
}

func TestFR7_BeadMapSchemaConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = BeadMapSchema()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}
	for i := 1; i < 10; i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatalf("goroutine %d returned different content", i)
		}
	}
}

func TestFR7_BeadMapSchemaNonTrivialSize(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	if len(data) <= 100 {
		t.Fatalf("bead-map schema too small (%d bytes), expected > 100", len(data))
	}
}

func TestFR7_BeadMapSchemaValidatesFixture(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("valid fixture passes", func(t *testing.T) {
		data := readTestdata(t, "valid_bead_map.json")
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if err := sch.Validate(v); err != nil {
			t.Fatalf("valid fixture should pass validation: %v", err)
		}
	})

	t.Run("invalid document rejected", func(t *testing.T) {
		err := validateBeadMap(t, sch, `{"next_id": 1}`)
		if err == nil {
			t.Fatal("expected validation error for missing records, got nil")
		}
	})
}

func TestFR7_BeadMapSchemaSelfContainedRefs(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Check records.items.$ref points to #/$defs/record
	props := raw["properties"].(map[string]any)
	records := props["records"].(map[string]any)
	items := records["items"].(map[string]any)
	ref := items["$ref"].(string)
	if ref != "#/$defs/record" {
		t.Fatalf("records $ref should be #/$defs/record, got %q", ref)
	}

	// Verify $defs/record exists
	defs := raw["$defs"].(map[string]any)
	if defs["record"] == nil {
		t.Fatal("$defs/record missing")
	}
}
