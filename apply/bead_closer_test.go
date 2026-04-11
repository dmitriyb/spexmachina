package apply

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
}

// --- S12: BeadCloser closes bead without labels (labels applied in label phase) ---

func TestREQ2_S12_CloseBeads_NoLabels(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "validator", Node: "LegacyChecker", BeadID: "spexmachina-42"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.closed) != 1 {
		t.Fatalf("want 1 Close call, got %d", len(cli.closed))
	}
	got := cli.closed[0]
	if got.ID != "spexmachina-42" {
		t.Errorf("ID: want %q, got %q", "spexmachina-42", got.ID)
	}
	if len(got.Labels) != 0 {
		t.Errorf("want 0 labels (close phase only closes), got %d: %v", len(got.Labels), got.Labels)
	}
}

// --- S13: BeadCloser treats individual close errors as errors and continues ---

func TestREQ2_S13_CloseBeads_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	var closeCalled []string
	cli.closeFn = func(id string, labels []string) error {
		closeCalled = append(closeCalled, id)
		if id == "bead-2" {
			return fmt.Errorf("network timeout")
		}
		return nil
	}

	actions := []Action{
		{Module: "validator", Node: "A", BeadID: "bead-1"},
		{Module: "validator", Node: "B", BeadID: "bead-2"},
		{Module: "validator", Node: "C", BeadID: "bead-3"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "bead-2") {
		t.Errorf("want error mentioning bead-2, got %v", err)
	}
	if !strings.Contains(err.Error(), "network timeout") {
		t.Errorf("want error containing cause, got %v", err)
	}
	// All 3 beads are open (default mock), so all 3 get Close calls.
	if len(closeCalled) != 3 {
		t.Fatalf("want 3 Close calls (all attempted), got %d", len(closeCalled))
	}
}

// --- S13b: Already-closed beads are skipped without calling Close ---

func TestREQ2_S13b_CloseBeads_SkipsAlreadyClosed(t *testing.T) {
	cli := newMockCLI()
	cli.statusFn = func(id string) (string, error) {
		if id == "bead-2" {
			return "closed", nil
		}
		return "open", nil
	}

	actions := []Action{
		{Module: "validator", Node: "A", BeadID: "bead-1"},
		{Module: "validator", Node: "B", BeadID: "bead-2"},
		{Module: "validator", Node: "C", BeadID: "bead-3"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// bead-2 is already closed, so only bead-1 and bead-3 get Close calls.
	if len(cli.closed) != 2 {
		t.Fatalf("want 2 Close calls (bead-2 skipped), got %d", len(cli.closed))
	}
	for _, c := range cli.closed {
		if c.ID == "bead-2" {
			t.Error("Close should not be called for already-closed bead-2")
		}
	}
}

// --- S14: BeadCloser returns nil when all closes succeed ---

func TestREQ2_S14_CloseBeads_AllSucceed(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", BeadID: "bead-1"},
		{Module: "merkle", Node: "TreeBuilder", BeadID: "bead-2"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.closed) != 2 {
		t.Fatalf("want 2 Close calls, got %d", len(cli.closed))
	}

	// Close phase passes nil labels (labeling was done in label phase).
	for i, c := range cli.closed {
		if len(c.Labels) != 0 {
			t.Errorf("call %d: want 0 labels, got %v", i, c.Labels)
		}
	}
}

// --- E2: Obsolete action with bead_id that no longer exists ---

func TestREQ2_E2_CloseBeads_BeadNotFound(t *testing.T) {
	cli := newMockCLI()
	cli.closeFn = func(id string, labels []string) error {
		return fmt.Errorf("bead not found")
	}

	actions := []Action{
		{Module: "validator", Node: "Gone", BeadID: "spexmachina-gone"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "bead not found") {
		t.Errorf("want 'bead not found' in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "spexmachina-gone") {
		t.Errorf("want bead ID in error, got %v", err)
	}
}

// --- E4: Empty obsoletes list ---

func TestREQ2_E4_CloseBeads_Empty(t *testing.T) {
	cli := newMockCLI()
	err := CloseBeads(context.Background(), cli, nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.closed) != 0 {
		t.Errorf("want 0 Close calls for empty input, got %d", len(cli.closed))
	}
}

// --- LabelObsoletes tests ---

func TestREQ2_LabelObsoletes_AddsLabels(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	actions := []Action{
		{Module: "validator", Node: "LegacyChecker", BeadID: "bead-1", ChangeType: "modified"},
	}

	err := LabelObsoletes(context.Background(), cli, store, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated) != 1 {
		t.Fatalf("want 1 Update call, got %d", len(cli.updated))
	}
	got := cli.updated[0]
	if got.ID != "bead-1" {
		t.Errorf("Update ID: want %q, got %q", "bead-1", got.ID)
	}
	if got.Metadata["spex"] != "obsolete" {
		t.Errorf("want spex:obsolete label, got spex:%s", got.Metadata["spex"])
	}
	commitVal, ok := got.Metadata["commit"]
	if !ok {
		t.Fatal("want commit label in metadata")
	}
	if len(commitVal) < 7 {
		t.Errorf("commit hash too short: %q", commitVal)
	}
}

func TestREQ2_LabelObsoletes_DeletesMappingForRemovedNodes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		ID:         10,
		SpecNodeID: "validator/component/1",
		BeadID:     "bead-1",
		Module:     "validator",
		Component:  "LegacyChecker",
	})

	actions := []Action{
		{Module: "validator", Node: "LegacyChecker", BeadID: "bead-1", ChangeType: "removed"},
	}

	err := LabelObsoletes(context.Background(), cli, store, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mapping record should be deleted for removed nodes.
	recs, _ := store.List()
	if len(recs) != 0 {
		t.Errorf("want 0 records after removing, got %d", len(recs))
	}
}

func TestREQ2_LabelObsoletes_LeavesMappingForModifiedNodes(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	store.addRecord(mapping.Record{
		ID:         10,
		SpecNodeID: "validator/component/1",
		BeadID:     "bead-1",
		Module:     "validator",
		Component:  "ContentResolver",
	})

	actions := []Action{
		{Module: "validator", Node: "ContentResolver", BeadID: "bead-1", ChangeType: "modified"},
	}

	err := LabelObsoletes(context.Background(), cli, store, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mapping record should be left intact for modified nodes.
	recs, _ := store.List()
	if len(recs) != 1 {
		t.Errorf("want 1 record (unchanged) for modified node, got %d", len(recs))
	}
}

func TestREQ2_LabelObsoletes_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	callCount := 0
	cli.updateFn = func(id string, metadata map[string]string) error {
		callCount++
		if id == "bead-2" {
			return fmt.Errorf("update failed")
		}
		return nil
	}

	actions := []Action{
		{Module: "validator", Node: "A", BeadID: "bead-1", ChangeType: "modified"},
		{Module: "validator", Node: "B", BeadID: "bead-2", ChangeType: "modified"},
		{Module: "validator", Node: "C", BeadID: "bead-3", ChangeType: "modified"},
	}

	err := LabelObsoletes(context.Background(), cli, store, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if callCount != 3 {
		t.Errorf("want 3 Update calls (all attempted), got %d", callCount)
	}
}

func TestREQ2_LabelObsoletes_Empty(t *testing.T) {
	cli := newMockCLI()
	store := newMockStore()
	err := LabelObsoletes(context.Background(), cli, store, nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.updated) != 0 {
		t.Errorf("want 0 Update calls for empty input, got %d", len(cli.updated))
	}
}
