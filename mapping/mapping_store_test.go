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

// writeJournal writes a journal file named .history.jsonl in dir, one line
// per entry, and returns its path — the resolved location a fixture hands
// to NewMappingStore, since the store computes no location of its own.
func writeJournal(t *testing.T, dir string, lines []string) string {
	t.Helper()
	content := ""
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	path := filepath.Join(dir, ".history.jsonl")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return path
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

// taskRetargetedLine builds a task_retargeted receipt pairing to the
// retarget's own modified event.
func taskRetargetedLine(forEID, taskID string) string {
	return fmt.Sprintf(`{"event":"task_retargeted","for":%q,"task_id":%q}`, forEID, taskID)
}

// refreshLine builds a refresh receipt naming the eids of the change
// events absorbed. gitHead of "" serialises as JSON null (a run with no
// --git-head).
func refreshLine(gitHead string, absorbed []string) string {
	items := make([]string, len(absorbed))
	for i, a := range absorbed {
		items[i] = fmt.Sprintf("%q", a)
	}
	return fmt.Sprintf(`{"event":"refresh","git_head":%s,"absorbed":[%s]}`, jsonField(gitHead), strings.Join(items, ","))
}

// registeredLine builds a registered event opening a proposal's lifecycle.
func registeredLine(eid, proposal, gitHead string) string {
	return fmt.Sprintf(`{"event":"registered","eid":%q,"proposal":%q,"git_head":%q}`, eid, proposal, gitHead)
}

// strPtr returns a pointer to s, for populating Event.Before/After in
// Append test fixtures.
func strPtr(s string) *string {
	return &s
}

// --- Parse a well-formed journal ---

func TestREQ_934d627f0e90_ParseWellFormedJournal(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "hash-after-1", "head1", "prop1"),
		changeLine("removed", "e2", "bbbbbbbbbbbb", "Bar", "component", "modB", "hash-before-2", "", "head2", "prop2"),
		taskCreatedLine("e1", "", "task-a"),
	})

	store := NewMappingStore(path)
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
	path := writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(path)
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
	path := writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(path)
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
	path := writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(path)
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

// --- task_retargeted is task-bearing and moves the sourcing event ---

func TestREQ_76fe608c3a40_FoldTaskRetargetedMovesSourcingEvent(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "eeeeeeeeeeee", "CompW", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-C"),
		changeLine("modified", "e2", "eeeeeeeeeeee", "CompW", "component", "modA", "h1", "h2", "g2", "p2"),
		taskRetargetedLine("e2", "task-C"),
		changeLine("modified", "e3", "eeeeeeeeeeee", "CompW", "component", "modA", "h2", "h3", "g3", "p3"),
		taskRetargetedLine("e3", "task-C"),
		refreshLine("g4", []string{"e3"}),
	})

	store := NewMappingStore(path)
	entry, err := store.Get("eeeeeeeeeeee")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.TaskID != "task-C" {
		t.Fatalf("TaskID: want task-C unchanged across retargets, got %s", entry.TaskID)
	}
	if entry.Source.EID != "e3" {
		t.Fatalf("Source: want sourcing event moved forward to the latest modified e3, got eid %s", entry.Source.EID)
	}
	if entry.Source.After == nil || *entry.Source.After != "h3" {
		t.Fatalf("Source.After: want h3 (the retargeted state's hash), got %+v", entry.Source.After)
	}
}

// --- Removed node retains its biography ---

func TestREQ_934d627f0e90_RemovedNodeRetainsBiography(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "cccccccccccc", "CompZ", "component", "modC", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-Z"),
		changeLine("removed", "e2", "cccccccccccc", "CompZ", "component", "modC", "h1", "", "g2-removed", "remove-prop"),
		taskClosedLine("e2", "task-Z"),
	})

	store := NewMappingStore(path)
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
	path := writeJournal(t, dir, []string{
		changeLine("removed", "e1", "dddddddddddd", "CompY", "component", "modD", "h1", "", "g1-removed", "remove-prop"),
		taskClosedLine("e1", "task-Y"),
		taskCreatedLine("e1", "", "br-cleanup"),
	})

	store := NewMappingStore(path)
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

// TestREQ_934d627f0e90_RemovedNodeReachableByCleanupTaskID keys a removed
// node by its cleanup task id — the half of "the two keys are
// interchangeable ways to reach one node" that removed nodes were missing.
// Both file orders are exercised because pairing is by eid, not by
// position: ingest lands a batch in which a cleanup's task_created can sit
// either side of the removal event it names, and the fold must answer the
// same either way.
func TestREQ_934d627f0e90_RemovedNodeReachableByCleanupTaskID(t *testing.T) {
	removal := changeLine("removed", "e9", "ffffffffffff", "CompV", "component", "modE", "h1", "", "g-removed", "remove-prop")
	cleanup := taskCreatedLine("e9", "", "task-cleanup")

	orders := map[string][]string{
		"receipt after removal":  {removal, cleanup},
		"receipt before removal": {cleanup, removal},
	}

	for name, lines := range orders {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeJournal(t, dir, lines)
			store := NewMappingStore(path)

			f, err := store.List()
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(f.Dangling) != 0 {
				t.Fatalf("cleanup receipt names a removal the journal carries; want no dangling, got %+v", f.Dangling)
			}

			byTask, err := store.Get("task-cleanup")
			if err != nil {
				t.Fatalf("Get by cleanup task id: %v", err)
			}
			byHash, err := store.Get("ffffffffffff")
			if err != nil {
				t.Fatalf("Get by hash: %v", err)
			}
			if !reflect.DeepEqual(byTask, byHash) {
				t.Fatalf("keys must be interchangeable: by task %+v, by hash %+v", byTask, byHash)
			}
			if byTask.Key != "ffffffffffff" || byTask.TaskID != "task-cleanup" {
				t.Fatalf("want key ffffffffffff / task-cleanup, got %+v", byTask)
			}
			if !byTask.Removed {
				t.Fatal("want Removed=true")
			}
			if byTask.Source.Name != "CompV" || byTask.Source.NodeType != "component" ||
				byTask.Source.Module != "modE" || byTask.Source.Proposal != "remove-prop" ||
				byTask.Source.GitHead != "g-removed" {
				t.Fatalf("cleanup task must answer with the biography, got %+v", byTask.Source)
			}
		})
	}
}

// TestREQ_934d627f0e90_HistoryPairsReceiptAheadOfItsEvent covers the same
// position-independence on the lineage side: a receipt naming an eid that
// appears later in the file still belongs to that node's history.
func TestREQ_934d627f0e90_HistoryPairsReceiptAheadOfItsEvent(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "ffffffffffff", "CompV", "component", "modE", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-V"),
		taskCreatedLine("e9", "", "task-cleanup"),
		changeLine("removed", "e9", "ffffffffffff", "CompV", "component", "modE", "h1", "", "g-removed", "remove-prop"),
		taskClosedLine("e9", "task-V"),
	})

	store := NewMappingStore(path)
	history, err := store.History("ffffffffffff")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	wantEvents := []string{"added", "task_created", "task_created", "removed", "task_closed"}
	if len(history) != len(wantEvents) {
		t.Fatalf("want %d events, got %d: %+v", len(wantEvents), len(history), history)
	}
	for i, want := range wantEvents {
		if history[i].Event != want {
			t.Fatalf("event %d: want %s, got %s", i, want, history[i].Event)
		}
	}
	if history[2].TaskID != "task-cleanup" {
		t.Fatalf("cleanup receipt missing from lineage, got %+v", history[2])
	}
}

// --- Epic receipts fold without a change event ---

func TestREQ_934d627f0e90_EpicReceiptFoldsWithoutChangeEvent(t *testing.T) {
	dir := t.TempDir()
	slug := "2026-04-18-decouple-spex-from-br"
	path := writeJournal(t, dir, []string{
		taskCreatedLine("", slug, "spexmachina-0lk"),
	})

	store := NewMappingStore(path)
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

// --- Registered event folds the epic ---

func TestREQ_934d627f0e90_RegisteredEventFoldsEpic(t *testing.T) {
	dir := t.TempDir()
	eid := "cafe1234:2026-08-11-event-keyed-linkage"
	slug := "2026-08-11-event-keyed-linkage"
	path := writeJournal(t, dir, []string{
		registeredLine(eid, slug, "cafe1234"),
		taskCreatedLine(eid, "", "spexmachina-hdkq"),
	})

	store := NewMappingStore(path)
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("want 1 epic entry, got %d: %+v", len(f.Entries), f.Entries)
	}
	entry := f.Entries[0]
	if entry.Key != slug {
		t.Fatalf("Key: want the registered event's slug %s, got %s", slug, entry.Key)
	}
	if entry.TaskID != "spexmachina-hdkq" {
		t.Fatalf("TaskID: want spexmachina-hdkq, got %s", entry.TaskID)
	}
	if entry.Source.Event != "registered" || entry.Source.EID != eid {
		t.Fatalf("Source: want the registered event, got %+v", entry.Source)
	}

	byTask, err := store.Get("spexmachina-hdkq")
	if err != nil {
		t.Fatalf("Get(task id): %v", err)
	}
	if !reflect.DeepEqual(entry, byTask) {
		t.Fatalf("List entry and Get-by-task-id diverged: %+v vs %+v", entry, byTask)
	}
}

// --- Append validates and lands atomically ---

func TestREQ_934d627f0e90_AppendLandsBatchAtomically(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, ".history.jsonl")
	store := NewMappingStore(journalPath)

	batch := []Event{
		{Event: "added", EID: "e1", Node: "aaaaaaaaaaaa", Name: "Foo", NodeType: "component", Module: "modA", After: strPtr("h1"), GitHead: "g1", Proposal: "p1"},
		{Event: "task_created", For: "e1", TaskID: "task-a"},
	}
	if err := store.Append(batch); err != nil {
		t.Fatalf("Append: %v", err)
	}

	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines after first append, got %d: %q", len(lines), raw)
	}

	// A second batch parses and folds exactly as the first, proving the
	// write went through the store's own read path.
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Entries) != 1 || f.Entries[0].TaskID != "task-a" {
		t.Fatalf("want 1 entry with TaskID task-a, got %+v", f.Entries)
	}
}

func TestREQ_934d627f0e90_AppendRefusedBatchChangesNothing(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, ".history.jsonl")
	store := NewMappingStore(journalPath)

	seed := []Event{
		{Event: "added", EID: "e1", Node: "aaaaaaaaaaaa", Name: "Foo", NodeType: "component", Module: "modA", After: strPtr("h1"), GitHead: "g1", Proposal: "p1"},
	}
	if err := store.Append(seed); err != nil {
		t.Fatalf("Append seed: %v", err)
	}
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}

	// Second line's node fails the identity-hash pattern — a schema
	// violation, not invalid JSON.
	badBatch := []Event{
		{Event: "added", EID: "e2", Node: "bbbbbbbbbbbb", Name: "Bar", NodeType: "component", Module: "modA", After: strPtr("h2"), GitHead: "g2", Proposal: "p2"},
		{Event: "added", EID: "e3", Node: "not-a-hash", Name: "Baz", NodeType: "component", Module: "modA", After: strPtr("h3"), GitHead: "g3", Proposal: "p3"},
	}
	err = store.Append(badBatch)
	if err == nil {
		t.Fatal("want error for batch with schema-violating line")
	}
	var ae *AppendError
	if !errors.As(err, &ae) {
		t.Fatalf("want *AppendError, got %T: %v", err, err)
	}
	if ae.Line != 2 {
		t.Fatalf("want offending batch line 2, got %d", ae.Line)
	}

	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("refused batch changed the file:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestREQ_934d627f0e90_AppendLandsOnNonEmptyJournal(t *testing.T) {
	dir := t.TempDir()
	journalPath := filepath.Join(dir, ".history.jsonl")
	store := NewMappingStore(journalPath)
	node := "aaaaaaaaaaaa"

	first := []Event{
		{Event: "added", EID: "e1", Node: node, Name: "Foo", NodeType: "component", Module: "modA", After: strPtr("h1"), GitHead: "g1", Proposal: "p1"},
		{Event: "task_created", For: "e1", TaskID: "task-a"},
	}
	if err := store.Append(first); err != nil {
		t.Fatalf("Append first batch: %v", err)
	}

	second := []Event{
		{Event: "modified", EID: "e2", Node: node, Name: "Foo", NodeType: "component", Module: "modA", Before: strPtr("h1"), After: strPtr("h2"), GitHead: "g2", Proposal: "p1"},
		{Event: "task_created", For: "e2", TaskID: "task-b"},
	}
	if err := store.Append(second); err != nil {
		t.Fatalf("Append second batch: %v", err)
	}

	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("want 4 lines after both appends, got %d: %q", len(lines), raw)
	}

	events, err := store.Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	wantEvents := []string{"added", "task_created", "modified", "task_created"}
	for i, ev := range events {
		if ev.Event != wantEvents[i] {
			t.Fatalf("line %d: want event %q, got %q", i+1, wantEvents[i], ev.Event)
		}
	}

	// The fold must reflect both batches: node X's lineage runs
	// added+task-a then modified+task-b, folding to the second task — the
	// same lineage rule test_mapping_store.md pins for the read path, now
	// proven through the write path.
	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Entries) != 1 || f.Entries[0].TaskID != "task-b" {
		t.Fatalf("want 1 entry with TaskID task-b, got %+v", f.Entries)
	}
}

func TestREQ_76fe608c3a40_AppendTaskRetargeted(t *testing.T) {
	dir := t.TempDir()
	store := NewMappingStore(filepath.Join(dir, ".history.jsonl"))

	batch := []Event{
		{Event: "added", EID: "e1", Node: "ffffffffffff", Name: "Foo", NodeType: "component", Module: "modA", After: strPtr("h1"), GitHead: "g1", Proposal: "p1"},
		{Event: "task_created", For: "e1", TaskID: "task-r"},
		{Event: "modified", EID: "e2", Node: "ffffffffffff", Name: "Foo", NodeType: "component", Module: "modA", Before: strPtr("h1"), After: strPtr("h2"), GitHead: "g2", Proposal: "p1"},
		{Event: "task_retargeted", For: "e2", TaskID: "task-r"},
	}
	if err := store.Append(batch); err != nil {
		t.Fatalf("Append: %v", err)
	}

	f, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(f.Entries) != 1 || f.Entries[0].TaskID != "task-r" || f.Entries[0].Source.EID != "e2" {
		t.Fatalf("want 1 entry TaskID task-r sourced from e2, got %+v", f.Entries)
	}
}

// --- Deterministic order ---

func TestREQ_934d627f0e90_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "111111111111", "CompP", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-P"),
		changeLine("added", "e2", "222222222222", "CompQ", "component", "modA", "", "h2", "g2", "p2"),
		taskCreatedLine("e2", "", "task-Q"),
	})

	store := NewMappingStore(path)
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
	store := NewMappingStore(filepath.Join(dir, ".history.jsonl"))

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
	path := writeJournal(t, dir, nil)
	store := NewMappingStore(path)

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
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-a"),
		"not valid json {",
	})

	store := NewMappingStore(path)
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
			path := writeJournal(t, dir, []string{
				changeLine("added", "e1", "aaaaaaaaaaaa", "Foo", "component", "modA", "", "h1", "g1", "p1"),
				tt.line,
			})

			store := NewMappingStore(path)
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
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "aaaaaaaaaaaa", "CompM", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-M"),
		taskCreatedLine("no-such-eid", "", "task-orphan"),
	})

	store := NewMappingStore(path)
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
	path := writeJournal(t, dir, nodeXYJournal())

	store := NewMappingStore(path)
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

func TestREQ_76fe608c3a40_HistoryIncludesTaskRetargeted(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, []string{
		changeLine("added", "e1", "eeeeeeeeeeee", "CompW", "component", "modA", "", "h1", "g1", "p1"),
		taskCreatedLine("e1", "", "task-C"),
		changeLine("modified", "e2", "eeeeeeeeeeee", "CompW", "component", "modA", "h1", "h2", "g2", "p2"),
		taskRetargetedLine("e2", "task-C"),
	})

	store := NewMappingStore(path)
	history, err := store.History("eeeeeeeeeeee")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	wantEvents := []string{"added", "task_created", "modified", "task_retargeted"}
	if len(history) != len(wantEvents) {
		t.Fatalf("want %d events, got %d: %+v", len(wantEvents), len(history), history)
	}
	for i, want := range wantEvents {
		if history[i].Event != want {
			t.Fatalf("event %d: want %s, got %s", i, want, history[i].Event)
		}
	}
}

// --- Get not found ---

func TestREQ_934d627f0e90_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	path := writeJournal(t, dir, nodeXYJournal())
	store := NewMappingStore(path)

	if _, err := store.Get("dddddddddddd"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for unknown hash, got %v", err)
	}
	if _, err := store.Get("no-such-task"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound for unknown task id, got %v", err)
	}
}
