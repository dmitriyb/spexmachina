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

// --- S12: BeadCloser obsoletes bead with correct labels ---

func TestREQ2_S12_CloseBeads_CorrectLabels(t *testing.T) {
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
	if len(got.Labels) != 2 {
		t.Fatalf("want 2 labels, got %d: %v", len(got.Labels), got.Labels)
	}
	if got.Labels[0] != "spex:obsolete" {
		t.Errorf("labels[0]: want %q, got %q", "spex:obsolete", got.Labels[0])
	}
	if !strings.HasPrefix(got.Labels[1], "commit:") {
		t.Errorf("labels[1]: want commit:<HEAD> prefix, got %q", got.Labels[1])
	}
	// Commit hash should be a hex string of at least 7 chars.
	commitHash := strings.TrimPrefix(got.Labels[1], "commit:")
	if len(commitHash) < 7 {
		t.Errorf("commit hash too short: %q", commitHash)
	}
}

// --- S13: BeadCloser treats individual close errors as warnings and continues ---

func TestREQ2_S13_CloseBeads_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	var called []string
	cli.closeFn = func(id string, labels []string) error {
		called = append(called, id)
		if id == "bead-2" {
			return fmt.Errorf("already closed")
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
	if !strings.Contains(err.Error(), "already closed") {
		t.Errorf("want error containing cause, got %v", err)
	}
	if len(called) != 3 {
		t.Fatalf("want 3 Close calls (all attempted), got %d", len(called))
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

	// Both should have spex:obsolete and commit:<HEAD> labels.
	for i, c := range cli.closed {
		if len(c.Labels) < 2 {
			t.Errorf("call %d: want at least 2 labels, got %v", i, c.Labels)
			continue
		}
		if c.Labels[0] != "spex:obsolete" {
			t.Errorf("call %d labels[0]: want %q, got %q", i, "spex:obsolete", c.Labels[0])
		}
		if !strings.HasPrefix(c.Labels[1], "commit:") {
			t.Errorf("call %d labels[1]: want commit: prefix, got %q", i, c.Labels[1])
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
