package schema

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// compileTaskStateSchema compiles the embedded task-state schema for validation.
func compileTaskStateSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("TaskStateSchema(): %v", err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal task-state schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("task-state.schema.json", doc); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	sch, err := c.Compile("task-state.schema.json")
	if err != nil {
		t.Fatalf("compile task-state schema: %v", err)
	}
	return sch
}

// validateTaskState validates one task-state document against the schema.
func validateTaskState(t *testing.T, sch *jsonschema.Schema, doc string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(doc), &v); err != nil {
		t.Fatalf("unmarshal test document: %v", err)
	}
	return sch.Validate(v)
}

// Fixtures, verbatim from test_task_state_schema.md.
const (
	tsFixtureTwoEntries = `{"version": 1, "tasks": [
  {"task_id": "spexmachina-abc", "status": "open"},
  {"task_id": "spexmachina-def", "status": "in_progress"}
]}`

	tsFixtureEmpty = `{"version": 1, "tasks": []}`
)

// --- S1: Both fixtures pass validation ---

func TestTaskStateSchema_S1_FixturesPass(t *testing.T) {
	sch := compileTaskStateSchema(t)

	fixtures := []struct {
		name string
		doc  string
	}{
		{"two entries", tsFixtureTwoEntries},
		{"empty tasks", tsFixtureEmpty},
	}
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			if err := validateTaskState(t, sch, f.doc); err != nil {
				t.Fatalf("%s should pass validation: %v", f.name, err)
			}
		})
	}
}

// --- S2: The status enum is exactly open and in_progress ---

func TestTaskStateSchema_S2_StatusEnumClosed(t *testing.T) {
	sch := compileTaskStateSchema(t)

	docWithStatus := func(status string) string {
		return `{"version": 1, "tasks": [
  {"task_id": "spexmachina-abc", "status": ` + status + `},
  {"task_id": "spexmachina-def", "status": "in_progress"}
]}`
	}

	variants := []string{`"closed"`, `"done"`, `"blocked"`, `""`, `"OPEN"`}
	for _, v := range variants {
		t.Run(v, func(t *testing.T) {
			if err := validateTaskState(t, sch, docWithStatus(v)); err == nil {
				t.Fatalf("expected validation error for status %s", v)
			}
		})
	}
}

// --- S3: Version is required and must be 1 ---

func TestTaskStateSchema_S3_VersionRequiredAndPinned(t *testing.T) {
	sch := compileTaskStateSchema(t)

	t.Run("version 2 fails", func(t *testing.T) {
		doc := `{"version": 2, "tasks": []}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for version 2")
		}
	})

	t.Run("version as string fails", func(t *testing.T) {
		doc := `{"version": "1", "tasks": []}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for version as string")
		}
	})

	t.Run("version absent fails", func(t *testing.T) {
		doc := `{"tasks": []}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for absent version")
		}
	})
}

// --- S4: An entry admits nothing but task_id and status ---

func TestTaskStateSchema_S4_EntryClosedShape(t *testing.T) {
	sch := compileTaskStateSchema(t)

	t.Run("extra labels array fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [
  {"task_id": "spexmachina-abc", "status": "open", "labels": []}
]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for entry carrying labels")
		}
	})

	t.Run("extra title fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [
  {"task_id": "spexmachina-abc", "status": "open", "title": "Do the thing"}
]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for entry carrying title")
		}
	})

	t.Run("id in place of task_id fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [
  {"id": "spexmachina-abc", "status": "open"}
]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for entry spelling id instead of task_id")
		}
	})
}

// --- S5: The envelope admits nothing but version and tasks ---

func TestTaskStateSchema_S5_EnvelopeClosedShape(t *testing.T) {
	sch := compileTaskStateSchema(t)

	t.Run("extra generated_at fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [], "generated_at": "2026-09-03T00:00:00Z"}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for envelope carrying generated_at")
		}
	})

	t.Run("tasks absent fails", func(t *testing.T) {
		doc := `{"version": 1}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for absent tasks")
		}
	})

	t.Run("only tasks key fails", func(t *testing.T) {
		doc := `{"tasks": []}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for document with only tasks key")
		}
	})
}

// --- S6: The raw tracker listing is refused, not recognised ---

func TestTaskStateSchema_S6_RawTrackerListingRefused(t *testing.T) {
	sch := compileTaskStateSchema(t)

	t.Run("issues envelope fails", func(t *testing.T) {
		doc := `{"issues": [{"id": "spexmachina-abc", "status": "open", "labels": []}]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for retired issues envelope")
		}
	})

	t.Run("bare array fails", func(t *testing.T) {
		doc := `[{"id": "spexmachina-abc", "status": "open", "labels": []}]`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for bare array")
		}
	})
}

// --- S7: task_id is a non-empty string ---

func TestTaskStateSchema_S7_TaskIDNonEmptyString(t *testing.T) {
	sch := compileTaskStateSchema(t)

	t.Run("empty string fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [{"task_id": "", "status": "open"}]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for empty task_id")
		}
	})

	t.Run("integer fails", func(t *testing.T) {
		doc := `{"version": 1, "tasks": [{"task_id": 42, "status": "open"}]}`
		if err := validateTaskState(t, sch, doc); err == nil {
			t.Fatal("expected validation error for integer task_id")
		}
	})
}

// --- E1: Empty object ---

func TestTaskStateSchema_E1_EmptyObjectFails(t *testing.T) {
	sch := compileTaskStateSchema(t)
	if err := validateTaskState(t, sch, `{}`); err == nil {
		t.Fatal("expected validation error for empty object")
	}
}

// --- E2: The schema file itself is embedded ---

func TestTaskStateSchema_E2_SchemaCompiles(t *testing.T) {
	// TaskStateSchema() returns a compilable JSON Schema document — the
	// embed is validated by compiling it, which catches a truncated or
	// stale task-state.schema.json at test time rather than on first use.
	compileTaskStateSchema(t)
}

// --- Loading / idempotency ---

func TestTaskStateSchema_LoadsValidJSON(t *testing.T) {
	data, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("TaskStateSchema(): %v", err)
	}
	if len(data) == 0 {
		t.Fatal("TaskStateSchema() returned empty bytes")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("TaskStateSchema() is not valid JSON: %v", err)
	}
}

func TestTaskStateSchema_Idempotent(t *testing.T) {
	data1, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	data2, err := TaskStateSchema()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatal("TaskStateSchema() not idempotent")
	}
}
