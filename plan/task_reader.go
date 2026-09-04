package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/dmitriyb/spexmachina/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Task is TaskReader's per-entry output: the tracker's own task id and the
// task's live status, exactly as the task-state artifact spelled it.
// Nothing else survives the parse — the artifact carries no label, no
// title and no other tracker field, and TaskReader reads none
// (arch_task_reader.md, "Interface").
type Task struct {
	ID     string
	Status string
}

// taskStateEntry mirrors one entry of the task-state artifact's wire shape.
type taskStateEntry struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

// taskStateDoc mirrors the task-state artifact's envelope: version 1, a
// tasks array, nothing else (schema/task-state.schema.json).
type taskStateDoc struct {
	Version int              `json:"version"`
	Tasks   []taskStateEntry `json:"tasks"`
}

var (
	taskStateSchema     *jsonschema.Schema
	taskStateSchemaErr  error
	taskStateSchemaOnce sync.Once
)

// getTaskStateSchema compiles the embedded task-state schema
// (schema.TaskStateSchema) once and caches it.
func getTaskStateSchema() (*jsonschema.Schema, error) {
	taskStateSchemaOnce.Do(func() {
		raw, err := schema.TaskStateSchema()
		if err != nil {
			taskStateSchemaErr = fmt.Errorf("load task-state schema: %w", err)
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			taskStateSchemaErr = fmt.Errorf("parse task-state schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("task-state.schema.json", doc); err != nil {
			taskStateSchemaErr = fmt.Errorf("add task-state schema: %w", err)
			return
		}
		taskStateSchema, taskStateSchemaErr = c.Compile("task-state.schema.json")
	})
	return taskStateSchema, taskStateSchemaErr
}

// ReadTasks validates and decodes the task-state artifact read from r into
// one Task per listed entry, in input order. It is a pure parser: no
// subprocess invocation, no live tracker call (arch_task_reader.md, "No
// Subprocess") — callers supply the bytes of the adapter-produced tasks.json
// via --tasks file input. An empty tasks array decodes to an empty slice,
// not an error: nothing in flight is the normal state between epics
// (arch_task_reader.md, "Responsibilities").
//
// Every failure — malformed JSON or a document that fails the task-state
// schema (wrong or missing version, a status outside the enum, an
// undeclared property, a missing task_id) — is returned to the caller with
// a message beginning "plan: read tasks:", naming the violated constraint.
func ReadTasks(r io.Reader) ([]Task, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("plan: read tasks: %w", err)
	}
	return ReadTasksBytes(data)
}

// ReadTasksBytes is ReadTasks for callers that already hold the artifact's
// bytes in memory.
func ReadTasksBytes(data []byte) ([]Task, error) {
	sch, err := getTaskStateSchema()
	if err != nil {
		return nil, fmt.Errorf("plan: read tasks: %w", err)
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("plan: read tasks: parse: %w", err)
	}
	if err := sch.Validate(doc); err != nil {
		return nil, fmt.Errorf("plan: read tasks: %w", err)
	}

	var ts taskStateDoc
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, fmt.Errorf("plan: read tasks: parse: %w", err)
	}

	out := make([]Task, 0, len(ts.Tasks))
	for _, e := range ts.Tasks {
		out = append(out, Task{ID: e.TaskID, Status: e.Status})
	}
	return out, nil
}
