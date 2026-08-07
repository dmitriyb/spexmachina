package mapping

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- fixture helpers ---

// writeJournal writes spec/.history.jsonl in dir, one line per entry.
func writeJournal(t *testing.T, dir string, lines []string) {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	path := filepath.Join(dir, ".history.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

// jsonField renders v as a quoted JSON string, or the literal null when v
// is empty — change events never legitimately carry an empty-string hash,
// so this doubles as the before/after null marker in fixtures.
func jsonField(v string) string {
	if v == "" {
		return "null"
	}
	return fmt.Sprintf("%q", v)
}

// changeLine builds one change-event journal line (added/removed/modified).
func changeLine(event, eid, node, name, nodeType, module, before, after, gitHead, proposal string) string {
	return fmt.Sprintf(
		`{"event":%q,"eid":%q,"node":%q,"name":%q,"node_type":%q,"module":%q,"before":%s,"after":%s,"git_head":%q,"proposal":%q}`,
		event, eid, node, name, nodeType, module, jsonField(before), jsonField(after), gitHead, proposal)
}

// taskCreatedLine builds a task_created receipt. Pass proposal for an epic
// receipt (no change event referent) or forEID for a node-pairing receipt.
func taskCreatedLine(forEID, proposal, taskID string) string {
	if proposal != "" {
		return fmt.Sprintf(`{"event":"task_created","proposal":%q,"task_id":%q}`, proposal, taskID)
	}
	return fmt.Sprintf(`{"event":"task_created","for":%q,"task_id":%q}`, forEID, taskID)
}

// taskClosedLine builds a task_closed receipt pairing to a change event's eid.
func taskClosedLine(forEID, taskID string) string {
	return fmt.Sprintf(`{"event":"task_closed","for":%q,"task_id":%q}`, forEID, taskID)
}

// --- Parse a well-formed journal ---

func TestREQ_934d627f0e90_ParseWellFormedJournal(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "hash-after-1", "head1", "prop1"),
		changeLine("removed", "e2", "bbbbbbbbbbbb", "Bar", "component", "modB", "hash-before-2", "", "head2", "prop2"),
		taskCreatedLine("e1", "", "task-a"),
	})

	store := NewMappingStore(dir)
	events, err := store.Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("want 3 events, got %d", len(events))
	}

	e0 := events[0]
	if e0.Event != "added" || e0.EID != "e1" || e0.Node != "aaaaaaaaaaaa" || e0.Name != "Foo" ||
		e0.NodeType != "component" || e0.Module != "modA" || e0.Before != nil ||
		e0.After == nil || *e0.After != "hash-after-1" || e0.GitHead != "head1" || e0.Proposal != "prop1" {
		t.Fatalf("event 0 fields did not round-trip: %+v", e0)
	}

	e1 := events[1]
	if e1.Event != "removed" || e1.EID != "e2" || e1.Node != "bbbbbbbbbbbb" || e1.Name != "Bar" ||
		e1.Before == nil || *e1.Before != "hash-before-2" || e1.After != nil || e1.GitHead != "head2" || e1.Proposal != "prop2" {
		t.Fatalf("event 1 fields did not round-trip: %+v", e1)
	}

	e2 := events[2]
	if e2.Event != "task_created" || e2.For != "e1" || e2.TaskID != "task-a" {
		t.Fatalf("event 2 fields did not round-trip: %+v", e2)
	}
}

// --- Fold yields the latest task-bearing event per node ---

func nodeXYJournal() []string {
	return []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "CompX", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-A"),
		changeLine("modified", "e2", "aaaaaaaaaaaa", "CompX", "component", "modA", "h1", "h2", "g2", "p2"),
		taskCreatedLine("e2", "", "task-B"),
		changeLine("added", "e3", "bbbbbbbbbbbb", "CompY", "component", "modA", "", "h3", "g3", "p3"),
	}
}

func TestREQ_934d627f0e90_FoldLatestTaskBearingEventPerNode(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(dir)
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var x *FoldEntry
	for i := range f.Entries {
		if f.Entries[i].Key == "aaaaaaaaaaaa" {
			x = &f.Entries[i]
		}
		if f.Entries[i].Key == "bbbbbbbbbbbb" {
			t.Fatalf("node Y should have no fold entry (no receipt), got %+v", f.Entries[i])
		}
	}
	if x == nil {
		t.Fatal("node X missing from fold")
	}
	if x.TaskID != "task-B" {
		t.Fatalf("X.TaskID: want task-B (latest wins), got %s", x.TaskID)
	}
	if x.Source.EID != "e2" {
		t.Fatalf("X.Source: want the modified event e2, got eid %s", x.Source.EID)
	}
}

// --- Lookup by identity hash ---

func TestREQ_934d627f0e90_LookupByIdentityHash(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(dir)
	entry, err := store.Get("aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.TaskID != "task-B" {
		t.Fatalf("TaskID: want task-B, got %s", entry.TaskID)
	}
	if entry.Source.Name != "CompX" || entry.Source.NodeType != "component" || entry.Source.Module != "modA" {
		t.Fatalf("Source name/node_type/module: got %+v", entry.Source)
	}
}

// --- Lookup by task id ---

func TestREQ_934d627f0e90_LookupByTaskID(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(dir)
	byHash, err := store.Get("aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get by hash: %v", err)
	}
	byTask, err := store.Get("task-B")
	if err != nil {
		t.Fatalf("Get by task id: %v", err)
	}
	if !reflect.DeepEqual(byHash, byTask) {
		t.Fatalf("hash and task-id lookups diverged: %+v vs %+v", byHash, byTask)
	}
}

// --- Removed node retains its biography ---

func TestREQ_934d627f0e90_RemovedNodeRetainsBiography(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("added", "e1", "cccccccccccc", "CompZ", "component", "modC", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-Z"),
		changeLine("removed", "e2", "cccccccccccc", "CompZ", "component", "modC", "h1", "", "g2-removed", "remove-prop"),
		taskClosedLine("e2", "task-Z"),
	})

	store := NewMappingStore(dir)
	entry, err := store.Get("cccccccccccc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !entry.Removed {
		t.Fatal("want Removed=true")
	}
	if entry.TaskID != "" {
		t.Fatalf("removed entry should carry biography instead of a live task, got TaskID=%q", entry.TaskID)
	}
	if entry.Source.Name != "CompZ" || entry.Source.NodeType != "component" || entry.Source.Module != "modC" {
		t.Fatalf("biography name/node_type/module: got %+v", entry.Source)
	}
	if entry.Source.Proposal != "remove-prop" {
		t.Fatalf("biography proposal: want remove-prop, got %s", entry.Source.Proposal)
	}
	if entry.Source.GitHead != "g2-removed" {
		t.Fatalf("biography git_head: want g2-removed, got %s", entry.Source.GitHead)
	}
}

func TestREQ_934d627f0e90_RemovedNodeWithCleanupTaskStaysRemoved(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("removed", "e1", "dddddddddddd", "CompY", "component", "modD", "h1", "", "g1-removed", "remove-prop"),
		taskClosedLine("e1", "task-Y"),
		taskCreatedLine("e1", "", "br-cleanup"),
	})

	store := NewMappingStore(dir)
	entry, err := store.Get("dddddddddddd")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !entry.Removed {
		t.Fatal("want Removed=true even after a cleanup task_created adopts the node's task id")
	}
	if entry.TaskID != "br-cleanup" {
		t.Fatalf("want TaskID=br-cleanup, got %q", entry.TaskID)
	}
	if entry.Source.Name != "CompY" || entry.Source.NodeType != "component" || entry.Source.Module != "modD" {
		t.Fatalf("biography name/node_type/module: got %+v", entry.Source)
	}
	if entry.Source.Proposal != "remove-prop" {
		t.Fatalf("biography proposal: want remove-prop, got %s", entry.Source.Proposal)
	}
}

// --- Epic receipts fold without a change event ---

func TestREQ_934d627f0e90_EpicReceiptFoldsWithoutChangeEvent(t *testing.T) {
	dir := t.TempDir()
	slug := "2026-04-18-decouple-spex-from-br"
	writeJournal(t, dir, []string{
		taskCreatedLine("", slug, "spexmachina-0lk"),
	})

	store := NewMappingStore(dir)
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("want 1 epic entry, got %d", len(f.Entries))
	}
	entry := f.Entries[0]
	if entry.Key != slug {
		t.Fatalf("Key: want %s, got %s", slug, entry.Key)
	}
	if entry.TaskID != "spexmachina-0lk" {
		t.Fatalf("TaskID: want spexmachina-0lk, got %s", entry.TaskID)
	}
	if entry.Source.Node != "" || entry.Source.Name != "" {
		t.Fatalf("epic entry should carry no invented change event, got Source=%+v", entry.Source)
	}
}

// --- Deterministic order ---

func TestREQ_934d627f0e90_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("added", "e1", "111111111111", "CompP", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-P"),
		changeLine("added", "e2", "222222222222", "CompQ", "component", "modA", "", "h2", "g2", "p2"),
		taskCreatedLine("e2", "", "task-Q"),
	})

	store := NewMappingStore(dir)
	first, err := store.List()
	if err != nil {
		t.Fatalf("List (1st): %v", err)
	}
	second, err := store.List()
	if err != nil {
		t.Fatalf("List (2nd): %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("List not deterministic:\n%+v\nvs\n%+v", first, second)
	}

	if len(first.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(first.Entries))
	}
	if first.Entries[0].Key != "111111111111" || first.Entries[1].Key != "222222222222" {
		t.Fatalf("entries not ordered by file position: %+v", first.Entries)
	}
}

// --- Edge cases ---

func TestREQ_934d627f0e90_MissingJournalFile(t *testing.T) {
	dir := t.TempDir()
	store := NewMappingStore(dir)

	events, err := store.Parse()
	if err != nil {
		t.Fatalf("Parse: want nil error for missing journal, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("want no events, got %d", len(events))
	}

	f, err := store.List()
	if err != nil {
		t.Fatalf("List: want nil error for missing journal, got %v", err)
	}
	if len(f.Entries) != 0 {
		t.Fatalf("want empty fold, got %d entries", len(f.Entries))
	}
}

func TestREQ_934d627f0e90_EmptyJournalFile(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nil)
	store := NewMappingStore(dir)

	events, err := store.Parse()
	if err != nil {
		t.Fatalf("Parse: want nil error for empty journal, got %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("want no events, got %d", len(events))
	}

	f, err := store.List()
	if err != nil {
		t.Fatalf("List: want nil error for empty journal, got %v", err)
	}
	if len(f.Entries) != 0 {
		t.Fatalf("want empty fold, got %d entries", len(f.Entries))
	}
}

func TestREQ_4aee62bd3c15_MalformedLineReportsLineNumber(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-a"),
		"not valid json {",
	})

	store := NewMappingStore(dir)
	_, err := store.Parse()
	if err == nil {
		t.Fatal("want error for malformed line")
	}
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Line != 3 {
		t.Fatalf("want line 3, got %d", pe.Line)
	}
}

func TestREQ_4aee62bd3c15_SchemaViolationReportsLineNumber(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "task receipt with neither for nor proposal",
			line: `{"event":"task_created","task_id":"spexmachina-abc"}`,
		},
		{
			name: "change event missing node",
			line: `{"event":"added","eid":"e1","name":"Foo","node_type":"component","module":"m","before":null,"after":"h","git_head":"g","proposal":"p"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeJournal(t, dir, []string{
				changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "h1", "g1", "p1"),
				tt.line,
			})

			store := NewMappingStore(dir)
			_, err := store.Parse()
			if err == nil {
				t.Fatal("want schema validation error")
			}
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("want *ParseError, got %T: %v", err, err)
			}
			if pe.Line != 2 {
				t.Fatalf("want line 2, got %d", pe.Line)
			}
		})
	}
}

func TestREQ_934d627f0e90_DanglingReceiptDoesNotPoisonFold(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "CompM", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-M"),
		taskCreatedLine("no-such-eid", "", "task-orphan"),
	})

	store := NewMappingStore(dir)
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Dangling) != 1 {
		t.Fatalf("want 1 dangling receipt, got %d", len(f.Dangling))
	}
	if f.Dangling[0].Receipt.TaskID != "task-orphan" {
		t.Fatalf("dangling receipt task id: want task-orphan, got %s", f.Dangling[0].Receipt.TaskID)
	}

	entry, err := store.Get("aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("Get: rest of the journal should fold normally, got error: %v", err)
	}
	if entry.TaskID != "task-M" {
		t.Fatalf("TaskID: want task-M, got %s", entry.TaskID)
	}
}

// --- History ---

func TestREQ_934d627f0e90_HistoryOldestFirst(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(dir)
	history, err := store.History("aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history) != 4 {
		t.Fatalf("want 4 events (added, task_created, modified, task_created), got %d: %+v", len(history), history)
	}
	wantEvents := []string{"added", "task_created", "modified", "task_created"}
	for i, want := range wantEvents {
		if history[i].Event != want {
			t.Fatalf("event %d: want %s, got %s", i, want, history[i].Event)
		}
	}
}

// --- Get not found ---

func TestREQ_934d627f0e90_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	writeJournal(t, dir, nodeXYJournal())
	store := NewMappingStore(dir)

	if _, err := store.Get("dddddddddddd"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for unknown hash, got %v", err)
	}
	if _, err := store.Get("no-such-task"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for unknown task id, got %v", err)
	}
}
