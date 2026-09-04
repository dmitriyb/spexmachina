package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileJournalLineSchema compiles the embedded journal-line schema for validation.
func compileJournalLineSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("JournalLineSchema(): %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal journal-line schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("journal-line.schema.json", doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("journal-line.schema.json")
	if err != nil {
		t.Fatalf("compile journal-line schema: %v", err)
	}
	return sch
}

// validateJournalLine validates one journal line document against the schema.
func validateJournalLine(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

// Fixtures, verbatim from test_journal_line_schema.md.
const (
	jlFixtureChangeEvent = `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`

	jlFixtureTaskCreated = `{"event":"task_created","for":"cafe1234:op-component-a1b2c3d4e5f6","task_id":"spexmachina-abc"}`

	jlFixtureTaskRetargeted = `{"event":"task_retargeted","for":"cafe1234:op-retarget-a1b2c3d4e5f6","task_id":"spexmachina-abc"}`

	jlFixtureRegisteredEvent = `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage",
 "proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`

	jlFixtureLegacyEpicReceipt = `{"event":"task_created","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-0lk"}`

	jlFixtureRefreshReceipt = `{"event":"refresh","git_head":"cafe1234","absorbed":["cafe1234:op-component-a1b2c3d4e5f6"]}`
)

// --- S1: Each fixture line passes validation ---

func TestJournalLineSchema_S1_FixturesPass(t *testing.T) {
	sch := compileJournalLineSchema(t)

	fixtures := []struct {
		name string
		doc  string
	}{
		{"change event", jlFixtureChangeEvent},
		{"task_created receipt", jlFixtureTaskCreated},
		{"task_retargeted receipt", jlFixtureTaskRetargeted},
		{"registered event", jlFixtureRegisteredEvent},
		{"legacy epic receipt", jlFixtureLegacyEpicReceipt},
		{"refresh receipt", jlFixtureRefreshReceipt},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			if err := validateJournalLine(t, sch, f.doc); err != nil {
				t.Fatalf("%s should pass validation: %v", f.name, err)
			}
		})
	}
}

// --- S2: Unknown event value fails ---

func TestJournalLineSchema_S2_UnknownEventFails(t *testing.T) {
	sch := compileJournalLineSchema(t)
	doc := `{"event":"renamed","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	if err := validateJournalLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for unknown event value")
	}
}

// --- S3: Change event missing node fails ---

func TestJournalLineSchema_S3_ChangeEventMissingNodeFails(t *testing.T) {
	sch := compileJournalLineSchema(t)
	doc := `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	if err := validateJournalLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for missing node")
	}
}

// --- S4: node must match the identity-hash pattern ---

func TestJournalLineSchema_S4_NodePattern(t *testing.T) {
	sch := compileJournalLineSchema(t)

	changeEventWithNode := func(node string) string {
		return `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"` + node + `","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	}

	t.Run("not a hash", func(t *testing.T) {
		if err := validateJournalLine(t, sch, changeEventWithNode("not-a-hash")); err == nil {
			t.Fatal("expected validation error for non-hash node")
		}
	})
	t.Run("uppercase", func(t *testing.T) {
		if err := validateJournalLine(t, sch, changeEventWithNode("A1B2C3D4E5F6")); err == nil {
			t.Fatal("expected validation error for uppercase node")
		}
	})
}

// --- S5: before/after admit null but not absence ---

func TestJournalLineSchema_S5_BeforeAfterNullVsAbsent(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("null before passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureChangeEvent); err != nil {
			t.Fatalf("null before should pass: %v", err)
		}
	})

	t.Run("omitted before fails", func(t *testing.T) {
		doc := `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for omitted before")
		}
	})
}

// --- S6: Task receipt requires exactly one referent ---

func TestJournalLineSchema_S6_TaskReceiptExactlyOneReferent(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("both for and proposal fails", func(t *testing.T) {
		doc := `{"event":"task_created","for":"cafe1234:op-component-a1b2c3d4e5f6","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-abc"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error when both for and proposal are present")
		}
	})

	t.Run("neither for nor proposal fails", func(t *testing.T) {
		doc := `{"event":"task_created","task_id":"spexmachina-abc"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error when neither for nor proposal is present")
		}
	})
}

// --- S6b: Registered event shape ---

func TestJournalLineSchema_S6b_RegisteredEventShape(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureRegisteredEvent); err != nil {
			t.Fatalf("registered fixture should pass: %v", err)
		}
	})

	t.Run("omitting proposal fails", func(t *testing.T) {
		doc := `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event omitting proposal")
		}
	})

	t.Run("omitting eid fails", func(t *testing.T) {
		doc := `{"event":"registered","proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event omitting eid")
		}
	})

	t.Run("carrying node fails", func(t *testing.T) {
		doc := `{"event":"registered","eid":"cafe1234:2026-08-11-event-keyed-linkage",
 "proposal":"2026-08-11-event-keyed-linkage","git_head":"cafe1234","node":"a1b2c3d4e5f6"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for registered event carrying node")
		}
	})
}

// --- S6c: task_retargeted takes the strict shape only ---

func TestJournalLineSchema_S6c_TaskRetargetedStrictShape(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureTaskRetargeted); err != nil {
			t.Fatalf("task_retargeted fixture should pass: %v", err)
		}
	})

	t.Run("omitting for fails", func(t *testing.T) {
		doc := `{"event":"task_retargeted","task_id":"spexmachina-abc"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for task_retargeted omitting for")
		}
	})

	t.Run("proposal in place of for fails", func(t *testing.T) {
		doc := `{"event":"task_retargeted","proposal":"2026-04-18-decouple-spex-from-br","task_id":"spexmachina-abc"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for task_retargeted carrying proposal instead of for")
		}
	})
}

// --- S6d: Two task_created lines for one node validate independently ---

func TestJournalLineSchema_S6d_TwoTaskCreatedLinesValidateIndependently(t *testing.T) {
	sch := compileJournalLineSchema(t)

	first := `{"event":"task_created","for":"cafe1234:op-component-a1b2c3d4e5f6","task_id":"spexmachina-abc"}`
	second := `{"event":"task_created","for":"cafe1235:op-3","task_id":"spexmachina-xyz"}`

	if err := validateJournalLine(t, sch, first); err != nil {
		t.Fatalf("first task_created should pass: %v", err)
	}
	if err := validateJournalLine(t, sch, second); err != nil {
		t.Fatalf("second task_created should pass: %v", err)
	}
}

// --- S7: node_type is shape-checked, not enumerated ---

func TestJournalLineSchema_S7_NodeTypeShapeCheckedNotEnumerated(t *testing.T) {
	sch := compileJournalLineSchema(t)

	changeEventWithNodeType := func(nodeType string) string {
		return `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"` + nodeType + `","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal"}`
	}

	for _, nodeType := range []string{"requirement", "endpoint", "impl_section"} {
		t.Run(nodeType+" admitted", func(t *testing.T) {
			if err := validateJournalLine(t, sch, changeEventWithNodeType(nodeType)); err != nil {
				t.Fatalf("node_type %q should pass: %v", nodeType, err)
			}
		})
	}

	t.Run("non-type-shaped value rejected", func(t *testing.T) {
		if err := validateJournalLine(t, sch, changeEventWithNodeType("Impl Section")); err == nil {
			t.Fatal("expected validation error for node_type \"Impl Section\" (fails the type-name pattern)")
		}
	})
}

// --- S8: Refresh receipt shape ---

func TestJournalLineSchema_S8_RefreshReceiptShape(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("fixture passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureRefreshReceipt); err != nil {
			t.Fatalf("refresh fixture should pass: %v", err)
		}
	})

	t.Run("empty absorbed passes", func(t *testing.T) {
		doc := `{"event":"refresh","git_head":"cafe1234","absorbed":[]}`
		if err := validateJournalLine(t, sch, doc); err != nil {
			t.Fatalf("empty absorbed should pass: %v", err)
		}
	})

	t.Run("omitted absorbed fails", func(t *testing.T) {
		doc := `{"event":"refresh","git_head":"cafe1234"}`
		if err := validateJournalLine(t, sch, doc); err == nil {
			t.Fatal("expected validation error for omitted absorbed")
		}
	})
}

// --- S8b: Optional path on change events ---

func TestJournalLineSchema_S8b_OptionalPath(t *testing.T) {
	sch := compileJournalLineSchema(t)

	t.Run("with path passes", func(t *testing.T) {
		doc := `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal","path":"impact/arch_action_classifier.md"}`
		if err := validateJournalLine(t, sch, doc); err != nil {
			t.Fatalf("change event with path should pass: %v", err)
		}
	})

	t.Run("without path passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureChangeEvent); err != nil {
			t.Fatalf("change event without path should pass: %v", err)
		}
	})
}

// --- S8c: Optional v on every line shape ---

func TestJournalLineSchema_S8c_OptionalFormatVersion(t *testing.T) {
	sch := compileJournalLineSchema(t)

	withVersion := func(doc, version string) string {
		return strings.TrimSuffix(strings.TrimSpace(doc), "}") + `,"v":` + version + `}`
	}

	fixtures := []struct {
		name string
		doc  string
	}{
		{"change event", jlFixtureChangeEvent},
		{"task_created receipt", jlFixtureTaskCreated},
		{"task_retargeted receipt", jlFixtureTaskRetargeted},
		{"registered event", jlFixtureRegisteredEvent},
		{"legacy epic receipt", jlFixtureLegacyEpicReceipt},
		{"refresh receipt", jlFixtureRefreshReceipt},
	}
	for _, f := range fixtures {
		t.Run(f.name+" with v:1 passes", func(t *testing.T) {
			if err := validateJournalLine(t, sch, withVersion(f.doc, "1")); err != nil {
				t.Fatalf("%s with v:1 should pass: %v", f.name, err)
			}
		})
	}

	t.Run("v as string fails", func(t *testing.T) {
		if err := validateJournalLine(t, sch, withVersion(jlFixtureChangeEvent, `"1"`)); err == nil {
			t.Fatal("expected validation error for v as string")
		}
	})

	t.Run("absent v passes", func(t *testing.T) {
		if err := validateJournalLine(t, sch, jlFixtureChangeEvent); err != nil {
			t.Fatalf("absent v should pass: %v", err)
		}
	})
}

// --- S9: No integer ids anywhere ---

func TestJournalLineSchema_S9_NoIntegerIDs(t *testing.T) {
	sch := compileJournalLineSchema(t)
	doc := `{"id": 42, "spec_node_id": "a1b2c3d4e5f6"}`
	if err := validateJournalLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for retired record-file shape")
	}
}

// --- E1: Extra properties rejected ---

func TestJournalLineSchema_E1_ExtraPropertiesRejected(t *testing.T) {
	sch := compileJournalLineSchema(t)
	doc := `{"event":"added","eid":"cafe1234:op-component-a1b2c3d4e5f6","node":"a1b2c3d4e5f6","name":"ActionClassifier",
 "node_type":"component","module":"impact","before":null,"after":"e3b0c44298fc",
 "git_head":"cafe1234","proposal":"2026-08-01-task-journal","color":"red"}`
	if err := validateJournalLine(t, sch, doc); err == nil {
		t.Fatal("expected validation error for undeclared field")
	}
}

// --- E2: Empty object ---

func TestJournalLineSchema_E2_EmptyObjectFails(t *testing.T) {
	sch := compileJournalLineSchema(t)
	if err := validateJournalLine(t, sch, `{}`); err == nil {
		t.Fatal("expected validation error for empty object")
	}
}

// --- E3: The schema file itself is embedded ---

func TestJournalLineSchema_E3_SchemaCompiles(t *testing.T) {
	// JournalLineSchema() returns a compilable JSON Schema document — the
	// embed is validated by compiling it, which catches a truncated or
	// stale journal-line.schema.json at test time rather than on first use.
	compileJournalLineSchema(t)
}

// --- Loading / idempotency ---

func TestJournalLineSchema_LoadsValidJSON(t *testing.T) {
	data, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("JournalLineSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("JournalLineSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JournalLineSchema() is not valid JSON: %v", err)
	}
}

func TestJournalLineSchema_Idempotent(t *testing.T) {
	data1, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	data2, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("JournalLineSchema() not idempotent")
	}
}

func TestJournalLineSchema_ConcurrentAccess(t *testing.T) {
	var wg sync.WaitGroup
	results := make([][]byte, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = JournalLineSchema()
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

func TestJournalLineSchema_SelfContainedRefs(t *testing.T) {
	data, err := JournalLineSchema()
	if err != nil {
		t.Fatalf("JournalLineSchema(): %v", err)
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
