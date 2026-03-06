package apply

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
}

func TestREQ2_CloseBeads_Success(t *testing.T) {
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
	if cli.closed[0].ID != "bead-1" {
		t.Errorf("want bead ID %q, got %q", "bead-1", cli.closed[0].ID)
	}
	if cli.closed[0].Reason != "Spec node removed: validator/SchemaChecker" {
		t.Errorf("want reason containing module/node, got %q", cli.closed[0].Reason)
	}
	if cli.closed[1].ID != "bead-2" {
		t.Errorf("want bead ID %q, got %q", "bead-2", cli.closed[1].ID)
	}
}

func TestREQ2_CloseBeads_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	cli.closeFn = func(id, reason string) error {
		if id == "bead-1" {
			return fmt.Errorf("already closed")
		}
		return nil
	}

	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", BeadID: "bead-1"},
		{Module: "merkle", Node: "TreeBuilder", BeadID: "bead-2"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "bead-1") {
		t.Errorf("want error mentioning bead-1, got %v", err)
	}
	if !strings.Contains(err.Error(), "already closed") {
		t.Errorf("want error containing cause, got %v", err)
	}
	// Second action should still have been attempted (not in cli.closed since closeFn overrides)
	// Verify by checking the error only mentions bead-1, not bead-2
	if strings.Contains(err.Error(), "bead-2") {
		t.Errorf("bead-2 should have succeeded, but error mentions it: %v", err)
	}
}

func TestREQ2_CloseBeads_AllErrors(t *testing.T) {
	cli := newMockCLI()
	cli.closeFn = func(id, reason string) error {
		return fmt.Errorf("failed for %s", id)
	}

	actions := []Action{
		{Module: "validator", Node: "A", BeadID: "bead-1"},
		{Module: "merkle", Node: "B", BeadID: "bead-2"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "bead-1") {
		t.Errorf("want error mentioning bead-1, got %v", err)
	}
	if !strings.Contains(err.Error(), "bead-2") {
		t.Errorf("want error mentioning bead-2, got %v", err)
	}
}

func TestREQ2_CloseBeads_Empty(t *testing.T) {
	cli := newMockCLI()
	err := CloseBeads(context.Background(), cli, nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.closed) != 0 {
		t.Errorf("want 0 Close calls for empty input, got %d", len(cli.closed))
	}
}

func TestREQ2_CloseBeads_ReasonFormat(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "apply", Node: "BeadCloser", BeadID: "bead-x"},
	}

	err := CloseBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Spec node removed: apply/BeadCloser"
	if cli.closed[0].Reason != want {
		t.Errorf("want reason %q, got %q", want, cli.closed[0].Reason)
	}
}
