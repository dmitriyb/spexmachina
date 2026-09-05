package plan

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// --- Unit tests: canned fixtures exercising each extraction path and each
// refusal (arch_task_reader.md, "Testing"). ---

func TestReadTasksBytes_CarriesIDAndStatusInOrder(t *testing.T) {
	data := []byte(`{"version": 1, "tasks": [
		{"task_id": "spex-001", "status": "open"},
		{"task_id": "spex-002", "status": "in_progress"},
		{"task_id": "spex-003", "status": "open"}
	]}`)

	got, err := ReadTasksBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Task{
		{ID: "spex-001", Status: "open"},
		{ID: "spex-002", Status: "in_progress"},
		{ID: "spex-003", Status: "open"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestReadTasksBytes_EmptyArrayReturnsEmptySlice(t *testing.T) {
	got, err := ReadTasksBytes([]byte(`{"version": 1, "tasks": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got %d: %#v", len(got), got)
	}
}

func TestReadTasksBytes_MalformedJSONError(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`not json`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks: parse:") {
		t.Errorf("want parse error prefix, got: %v", err)
	}
}

func TestReadTasksBytes_WrongVersionRefused(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`{"version": 2, "tasks": []}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
		t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
	}
}

func TestReadTasksBytes_ClosedStatusRefused(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`{"version": 1, "tasks": [{"task_id": "spex-004", "status": "closed"}]}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
		t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
	}
}

func TestReadTasksBytes_UndeclaredEntryPropertyRefused(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`{"version": 1, "tasks": [{"task_id": "spex-005", "status": "open", "labels": []}]}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
		t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
	}
}

func TestReadTasksBytes_RawTrackerListingRefused(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`{"issues": [{"id": "spex-006", "status": "open"}]}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
		t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
	}
}

func TestReadTasksBytes_MissingTaskIDRefused(t *testing.T) {
	_, err := ReadTasksBytes([]byte(`{"version": 1, "tasks": [{"status": "open"}]}`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
		t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
	}
}

func TestReadTasks_ReaderEquivalentToBytes(t *testing.T) {
	data := []byte(`{"version": 1, "tasks": [{"task_id": "spex-001", "status": "open"}]}`)

	fromBytes, err := ReadTasksBytes(data)
	if err != nil {
		t.Fatalf("ReadTasksBytes: %v", err)
	}
	fromReader, err := ReadTasks(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadTasks: %v", err)
	}
	if !reflect.DeepEqual(fromBytes, fromReader) {
		t.Errorf("reader/bytes mismatch: bytes=%#v reader=%#v", fromBytes, fromReader)
	}
}

// --- Integration scenarios from spec/plan/test_task_matching.md ---
//
// S1, S2 and S2b exercise TaskReader alone, against the shape it actually
// consumes: the task-state artifact, not the task journal — TaskReader
// starts no process, contacts no tracker, and never touches the journal.
// S3 onward exercise NodeMatcher against journal-derived pairings and
// belong to that component's own bead.

// S1: TaskReader carries id and status, parses nothing else. Each entry
// carries exactly the two fields the interface promises (ID, Status), in
// input order, and no entry exposes anything beyond them. The in_progress
// status on spex-002 is carried through verbatim.
func TestS1_TaskReaderCarriesIDAndStatus_ParsesNothingElse(t *testing.T) {
	data := []byte(`{
		"version": 1,
		"tasks": [
			{"task_id": "spex-001", "status": "open"},
			{"task_id": "spex-002", "status": "in_progress"},
			{"task_id": "spex-003", "status": "open"}
		]
	}`)

	got, err := ReadTasksBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Task{
		{ID: "spex-001", Status: "open"},
		{ID: "spex-002", Status: "in_progress"},
		{ID: "spex-003", Status: "open"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	// Task's only fields are ID and Status: verify reflectively that no
	// third field sneaks in.
	typ := reflect.TypeOf(Task{})
	if typ.NumField() != 2 {
		t.Fatalf("Task has %d fields, want 2 (ID, Status)", typ.NumField())
	}
}

// S2: TaskReader returns an empty slice, not an error, on an empty
// artifact — nothing in flight is the normal state between epics.
func TestS2_TaskReaderEmptySliceOnEmptyArtifact(t *testing.T) {
	got, err := ReadTasksBytes([]byte(`{"version": 1, "tasks": []}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty slice, got %d: %#v", len(got), got)
	}
}

// S2b: TaskReader refuses every document that is not a version-1
// task-state artifact — each refused with an error beginning
// "plan: read tasks:" and naming the constraint, none returning entries.
func TestS2b_TaskReaderRefusesNonTaskStateDocuments(t *testing.T) {
	cases := []struct {
		name         string
		doc          string
		wantContains string
	}{
		{
			name:         "unspoken version",
			doc:          `{"version": 2, "tasks": []}`,
			wantContains: "/version",
		},
		{
			name:         "closed status the format has no value for",
			doc:          `{"version": 1, "tasks": [{"task_id": "spex-004", "status": "closed"}]}`,
			wantContains: "'open', 'in_progress'",
		},
		{
			name:         "undeclared property on an entry",
			doc:          `{"version": 1, "tasks": [{"task_id": "spex-005", "status": "open", "labels": []}]}`,
			wantContains: "'labels' not allowed",
		},
		{
			name:         "raw tracker listing, the retired input shape",
			doc:          `{"issues": [{"id": "spex-006", "status": "open"}]}`,
			wantContains: "missing properties 'version', 'tasks'",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ReadTasksBytes([]byte(tc.doc))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.HasPrefix(err.Error(), "plan: read tasks:") {
				t.Errorf("want 'plan: read tasks:' prefix, got: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("want error naming the constraint (%q), got: %v", tc.wantContains, err)
			}
			if got != nil {
				t.Errorf("want no entries on refusal, got %#v", got)
			}
		})
	}
}
