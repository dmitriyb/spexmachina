package apply

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestREQ3_UpdateBeads_Success(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", BeadID: "bead-1", SpecHash: "hash-new-1"},
		{Module: "merkle", Node: "TreeBuilder", BeadID: "bead-2", SpecHash: "hash-new-2"},
	}

	err := UpdateBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated) != 2 {
		t.Fatalf("want 2 Update calls, got %d", len(cli.updated))
	}
	if cli.updated[0].ID != "bead-1" {
		t.Errorf("want bead ID %q, got %q", "bead-1", cli.updated[0].ID)
	}
	if cli.updated[0].Metadata["spec_hash"] != "hash-new-1" {
		t.Errorf("want spec_hash %q, got %q", "hash-new-1", cli.updated[0].Metadata["spec_hash"])
	}
	if cli.updated[1].ID != "bead-2" {
		t.Errorf("want bead ID %q, got %q", "bead-2", cli.updated[1].ID)
	}
	if cli.updated[1].Metadata["spec_hash"] != "hash-new-2" {
		t.Errorf("want spec_hash %q, got %q", "hash-new-2", cli.updated[1].Metadata["spec_hash"])
	}
}

func TestREQ3_UpdateBeads_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	var called []string
	cli.updateFn = func(id string, metadata map[string]string) error {
		called = append(called, id)
		if id == "bead-1" {
			return fmt.Errorf("not found")
		}
		return nil
	}

	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", BeadID: "bead-1", SpecHash: "h1"},
		{Module: "merkle", Node: "TreeBuilder", BeadID: "bead-2", SpecHash: "h2"},
	}

	err := UpdateBeads(context.Background(), cli, actions, testLogger())
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "bead-1") {
		t.Errorf("want error mentioning bead-1, got %v", err)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("want error containing cause, got %v", err)
	}
	if len(called) != 2 {
		t.Fatalf("want 2 Update calls, got %d", len(called))
	}
	if called[0] != "bead-1" || called[1] != "bead-2" {
		t.Errorf("want calls [bead-1 bead-2], got %v", called)
	}
}

func TestREQ3_UpdateBeads_AllErrors(t *testing.T) {
	cli := newMockCLI()
	cli.updateFn = func(id string, metadata map[string]string) error {
		return fmt.Errorf("failed for %s", id)
	}

	actions := []Action{
		{Module: "validator", Node: "A", BeadID: "bead-1", SpecHash: "h1"},
		{Module: "merkle", Node: "B", BeadID: "bead-2", SpecHash: "h2"},
	}

	err := UpdateBeads(context.Background(), cli, actions, testLogger())
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

func TestREQ3_UpdateBeads_Empty(t *testing.T) {
	cli := newMockCLI()
	err := UpdateBeads(context.Background(), cli, nil, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.updated) != 0 {
		t.Errorf("want 0 Update calls for empty input, got %d", len(cli.updated))
	}
}

func TestREQ3_UpdateBeads_OnlySpecHash(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "apply", Node: "BeadUpdater", BeadID: "bead-x", SpecHash: "abc123"},
	}

	err := UpdateBeads(context.Background(), cli, actions, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated[0].Metadata) != 1 {
		t.Fatalf("want exactly 1 metadata key, got %d", len(cli.updated[0].Metadata))
	}
	if _, ok := cli.updated[0].Metadata["spec_hash"]; !ok {
		t.Errorf("want spec_hash key in metadata, got %v", cli.updated[0].Metadata)
	}
}
