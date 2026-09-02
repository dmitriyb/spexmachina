package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileBeadMapSchema compiles the embedded journal-line schema for validation.
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

// validateLine validates one journal line document against the schema.
func validateLine(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

// Fixtures, verbatim from test_bead_map_schema.md.
const (
	fixtureChangeEvent = `{"event":"added","eid":"cafe1234:op-7","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`

	fixtureTaskReceipt = `{"event":"task_created","for":"cafe1234:op-7","task_id":"spexmachina-abc"}`

	fixtureTaskRetargeted = `{"event":"task_retargeted","for":"cafe1234:op-9","task_id":"spexmachina-abc"}`

	fixtureRegisteredEvent = `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage",
 "proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`

	fixtureEpicReceipt = `{"event":"task_created","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-0lk"}`

	fixtureRefreshReceipt = `{"event":"refresh","git_head":"cafe1234","absorbed":["cafe1234:op-7"]}`
)

// --- S1: Each fixture line passes validation ---

func TestFR7_S1_FixturesPass(t *testing.T) {
	sch := compileBeadMapSchema(t)

	fixtures := []struct {
		name string
		doc  string
	}{
		{"change event", fixtureChangeEvent},
		{"task receipt", fixtureTaskReceipt},
		{"task retargeted receipt", fixtureTaskRetargeted},
		{"registered event", fixtureRegisteredEvent},
		{"epic receipt", fixtureEpicReceipt},
		{"refresh receipt", fixtureRefreshReceipt},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			if err := validateLine(t, sch, f.doc); err != nil {
				t.Fatalf("%s should pass validation: %v", f.name, err)
			}
		})
	}
}

// --- S2: Unknown event value fails ---

func TestFR7_S2_UnknownEventFails(t *testing.T) {
	sch := compileBeadMapSchema(t)
	doc := `{"event":"renamed","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	err := validateLine(t, sch, doc)
	if err == nil {
		t.Fatal("expected validation error for unknown event value")
	}
}

// --- S3: Change event missing node fails ---

func TestFR7_S3_ChangeEventMissingNodeFails(t *testing.T) {
	sch := compileBeadMapSchema(t)
	doc := `{"event":"added","eid":"9f2c41a0b7d3","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	err := validateLine(t, sch, doc)
	if err == nil {
		t.Fatal("expected validation error for missing node")
	}
}

// --- S4: node must match the identity-hash pattern ---

func TestFR7_S4_NodePattern(t *testing.T) {
	sch := compileBeadMapSchema(t)

	changeEventWithNode := func(node string) string {
		return `{"event":"added","eid":"9f2c41a0b7d3","node":"` + node + `","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	}

	t.Run("not a hash", func(t *testing.T) {
		if err := validateLine(t, sch, changeEventWithNode("not-a-hash")); err == nil {
			t.Fatal("expected validation error for non-hash node")
		}
	})
	t.Run("uppercase", func(t *testing.T) {
		if err := validateLine(t, sch, changeEventWithNode("A1B2C3D4E5F6")); err == nil {
			t.Fatal("expected validation error for uppercase node")
		}
	})
}

// --- S5: before/after admit null but not absence ---

func TestFR7_S5_BeforeAfterNullVsAbsent(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("null before passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureChangeEvent); err != nil {
			t.Fatalf("null before should pass: %v", err)
		}
	})

	t.Run("omitted before fails", func(t *testing.T) {
		doc := `{"event":"added","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for omitted before")
		}
	})
}

// --- S6: Task receipt requires exactly one referent ---

func TestFR7_S6_TaskReceiptExactlyOneReferent(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("both for and proposal fails", func(t *testing.T) {
		doc := `{"event":"task_created","for":"9f2c41a0b7d3","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-abc"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error when both for and proposal are present")
		}
	})

	t.Run("neither for nor proposal fails", func(t *testing.T) {
		doc := `{"event":"task_created","task_id":"spexmachina-abc"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error when neither for nor proposal is present")
		}
	})
}

// --- S6c: task_retargeted takes the strict shape only ---

func TestFR7_S6c_TaskRetargetedStrictShape(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureTaskRetargeted); err != nil {
			t.Fatalf("task_retargeted fixture should pass: %v", err)
		}
	})

	t.Run("omitting for fails", func(t *testing.T) {
		doc := `{"event":"task_retargeted","task_id":"spexmachina-abc"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for task_retargeted omitting for")
		}
	})

	t.Run("proposal in place of for fails", func(t *testing.T) {
		doc := `{"event":"task_retargeted","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-abc"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for task_retargeted carrying proposal instead of for")
		}
	})
}

// --- S6b: Registered event shape ---

func TestFR7_S6b_RegisteredEventShape(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureRegisteredEvent); err != nil {
			t.Fatalf("registered fixture should pass: %v", err)
		}
	})

	t.Run("omitting proposal fails", func(t *testing.T) {
		doc := `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event omitting proposal")
		}
	})

	t.Run("omitting eid fails", func(t *testing.T) {
		doc := `{"event":"registered","proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event omitting eid")
		}
	})

	t.Run("carrying node fails", func(t *testing.T) {
		doc := `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage",
 "proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234","node":"a1b2c3d4e5f6"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event carrying node")
		}
	})
}

// --- S7: node_type is a closed enum ---

func TestFR7_S7_NodeTypeClosedEnum(t *testing.T) {
	sch := compileBeadMapSchema(t)

	changeEventWithNodeType := func(nodeType string) string {
		return `{"event":"added","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"` + nodeType + `","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	}

	t.Run("retired kind rejected", func(t *testing.T) {
		if err := validateLine(t, sch, changeEventWithNodeType("impl_section")); err == nil {
			t.Fatal("expected validation error for retired node_type impl_section")
		}
	})

	t.Run("requirement admitted", func(t *testing.T) {
		if err := validateLine(t, sch, changeEventWithNodeType("requirement")); err != nil {
			t.Fatalf("node_type requirement should pass: %v", err)
		}
	})
}

// --- S8: Refresh receipt shape ---

func TestFR7_S8_RefreshReceiptShape(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureRefreshReceipt); err != nil {
			t.Fatalf("refresh fixture should pass: %v", err)
		}
	})

	t.Run("empty absorbed passes", func(t *testing.T) {
		doc := `{"event":"refresh","git_head":"cafe1234","absorbed":[]}`
		if err := validateLine(t, sch, doc); err != nil {
			t.Fatalf("empty absorbed should pass: %v", err)
		}
	})

	t.Run("omitted absorbed fails", func(t *testing.T) {
		doc := `{"event":"refresh","git_head":"cafe1234"}`
		if err := validateLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for omitted absorbed")
		}
	})
}

// --- S8b: Optional path on change events ---

func TestFR7_S8b_OptionalPath(t *testing.T) {
	sch := compileBeadMapSchema(t)

	t.Run("with path passes", func(t *testing.T) {
		doc := `{"event":"added","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal","path":"impact/arch_action_classifier.md"}`
		if err := validateLine(t, sch, doc); err != nil {
			t.Fatalf("change event with path should pass: %v", err)
		}
	})

	t.Run("without path passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureChangeEvent); err != nil {
			t.Fatalf("change event without path should pass: %v", err)
		}
	})
}

// --- S8c: Optional v on every line shape ---

func TestFR7_S8c_OptionalFormatVersion(t *testing.T) {
	sch := compileBeadMapSchema(t)

	withVersion := func(doc, version string) string {
		return strings.TrimSuffix(strings.TrimSpace(doc), "}") + `,"v":` + version + `}`
	}

	fixtures := []struct {
		name string
		doc  string
	}{
		{"change event", fixtureChangeEvent},
		{"task receipt", fixtureTaskReceipt},
		{"task retargeted receipt", fixtureTaskRetargeted},
		{"registered event", fixtureRegisteredEvent},
		{"epic receipt", fixtureEpicReceipt},
		{"refresh receipt", fixtureRefreshReceipt},
	}
	for _, f := range fixtures {
		t.Run(f.name+" with v:1 passes", func(t *testing.T) {
			if err := validateLine(t, sch, withVersion(f.doc, "1")); err != nil {
				t.Fatalf("%s with v:1 should pass: %v", f.name, err)
			}
		})
	}

	t.Run("v as string fails", func(t *testing.T) {
		if err := validateLine(t, sch, withVersion(fixtureChangeEvent, `"1"`)); err == nil {
			t.Fatal("expected validation error for v as string")
		}
	})

	t.Run("v above 1 passes", func(t *testing.T) {
		if err := validateLine(t, sch, withVersion(fixtureChangeEvent, "2")); err != nil {
			t.Fatalf("v:2 should pass — the schema pins no upper bound: %v", err)
		}
	})

	t.Run("absent v passes", func(t *testing.T) {
		if err := validateLine(t, sch, fixtureChangeEvent); err != nil {
			t.Fatalf("absent v should pass: %v", err)
		}
	})
}

// --- S9: No integer ids anywhere ---

func TestFR7_S9_NoIntegerIDs(t *testing.T) {
	sch := compileBeadMapSchema(t)
	doc := `{"id": 42, "spec_node_id": "a1b2c3d4e5f6"}`
	if err := validateLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for retired bead-map record shape")
	}
}

// --- E1: Extra properties rejected ---

func TestFR7_E1_ExtraPropertiesRejected(t *testing.T) {
	sch := compileBeadMapSchema(t)
	doc := `{"event":"added","eid":"9f2c41a0b7d3","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal","color":"red"}`
	if err := validateLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for undeclared field")
	}
}

// --- E2: Empty object ---

func TestFR7_E2_EmptyObjectFails(t *testing.T) {
	sch := compileBeadMapSchema(t)
	if err := validateLine(t, sch, `{}`); err == nil {
		t.Fatal("expected validation error for empty object")
	}
}

// --- E3: The schema file itself is embedded ---

func TestFR7_E3_SchemaCompiles(t *testing.T) {
	// BeadMapSchema() returns a compilable JSON Schema document — the embed
	// is validated by compiling it, which catches a truncated or stale
	// bead-map.schema.json at test time rather than on first use.
	compileBeadMapSchema(t)
}

// --- Loading / idempotency ---

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

func TestFR7_BeadMapSchemaSelfContainedRefs(t *testing.T) {
	data, err := BeadMapSchema()
	if err != nil {
		t.Fatalf("BeadMapSchema(): %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	oneOf, ok := raw["oneOf"].([]any)
	if !ok || len(oneOf) != 5 {
		t.Fatalf("top-level oneOf should list 5 line shapes, got %v", raw["oneOf"])
	}
	for _, entry := range oneOf {
		m := entry.(map[string]any)
		ref, _ := m["$ref"].(string)
		if !strings.HasPrefix(ref, "#/$defs/") {
			t.Fatalf("oneOf entry $ref should point into #/$defs/, got %q", ref)
		}
		defName := strings.TrimPrefix(ref, "#/$defs/")
		defs := raw["$defs"].(map[string]any)
		if defs[defName] == nil {
			t.Fatalf("$defs/%s missing", defName)
		}
	}
}
