package apply

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestREQ4_TagWithProposal_Success(t *testing.T) {
	cli := newMockCLI()
	ids := []string{"bead-1", "bead-2", "bead-3"}
	proposal := "2026-02-23-spex-machina.md"

	err := TagWithProposal(context.Background(), cli, ids, proposal, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated) != 3 {
		t.Fatalf("want 3 Update calls, got %d", len(cli.updated))
	}
	for i, id := range ids {
		if cli.updated[i].ID != id {
			t.Errorf("call %d: want bead ID %q, got %q", i, id, cli.updated[i].ID)
		}
		wantRef := "2026-02-23-spex-machina" // .md stripped
		if cli.updated[i].Metadata["spec_proposal"] != wantRef {
			t.Errorf("call %d: want spec_proposal %q, got %q", i, wantRef, cli.updated[i].Metadata["spec_proposal"])
		}
	}
}

func TestREQ4_TagWithProposal_ErrorContinuesBatch(t *testing.T) {
	cli := newMockCLI()
	var called []string
	cli.updateFn = func(id string, metadata map[string]string) error {
		called = append(called, id)
		if id == "bead-1" {
			return fmt.Errorf("not found")
		}
		return nil
	}

	ids := []string{"bead-1", "bead-2"}
	err := TagWithProposal(context.Background(), cli, ids, "proposal.md", testLogger())
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

func TestREQ4_TagWithProposal_AllErrors(t *testing.T) {
	cli := newMockCLI()
	cli.updateFn = func(id string, metadata map[string]string) error {
		return fmt.Errorf("failed for %s", id)
	}

	ids := []string{"bead-1", "bead-2"}
	err := TagWithProposal(context.Background(), cli, ids, "proposal.md", testLogger())
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

func TestREQ4_TagWithProposal_Empty(t *testing.T) {
	cli := newMockCLI()
	err := TagWithProposal(context.Background(), cli, nil, "proposal.md", testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cli.updated) != 0 {
		t.Errorf("want 0 Update calls for empty input, got %d", len(cli.updated))
	}
}

func TestREQ4_TagWithProposal_MetadataKey(t *testing.T) {
	cli := newMockCLI()
	ids := []string{"bead-x"}
	proposal := "2026-03-01-feature.md"

	err := TagWithProposal(context.Background(), cli, ids, proposal, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cli.updated[0].Metadata) != 1 {
		t.Fatalf("want exactly 1 metadata key, got %d", len(cli.updated[0].Metadata))
	}
	if _, ok := cli.updated[0].Metadata["spec_proposal"]; !ok {
		t.Errorf("want spec_proposal key in metadata, got %v", cli.updated[0].Metadata)
	}
}
