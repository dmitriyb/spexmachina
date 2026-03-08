package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// initSandbox creates a temporary br workspace for integration tests.
// Uses t.Chdir so br discovers the sandbox .beads/ database.
// Skips the test if br is not on PATH.
func initSandbox(t *testing.T) *execCLI {
	t.Helper()

	bin, err := exec.LookPath("br")
	if err != nil {
		t.Skip("br not on PATH, skipping integration test")
	}

	dir := t.TempDir()

	// br init needs to run inside the workspace directory.
	cmd := exec.Command(bin, "init", "--prefix", "test", "--no-auto-flush", "--no-auto-import")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("br init: %v\n%s", err, out)
	}

	// Change to sandbox dir so br discovers .beads/ here, not in the project root.
	// t.Chdir restores the original dir on cleanup.
	t.Chdir(dir)

	return &execCLI{bin: bin}
}

// brShow returns the JSON object for a bead in the sandbox.
func brShow(t *testing.T, bin, id string) map[string]interface{} {
	t.Helper()
	out, err := exec.Command(bin, "show", id, "--format", "json", "--no-auto-flush", "--no-auto-import").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("br show %s: %v\n%s", id, err, ee.Stderr)
		}
		t.Fatalf("br show %s: %v", id, err)
	}
	var beads []map[string]interface{}
	if err := json.Unmarshal(out, &beads); err != nil {
		t.Fatalf("parse br show output: %v\n%s", err, out)
	}
	if len(beads) == 0 {
		t.Fatalf("br show %s: no results", id)
	}
	return beads[0]
}

func TestIntegration_NewBeadCLI(t *testing.T) {
	if _, err := exec.LookPath("br"); err != nil {
		t.Skip("br not on PATH, skipping integration test")
	}

	cli, err := NewBeadCLI(context.Background(), "br")
	if err != nil {
		t.Fatalf("NewBeadCLI(br): %v", err)
	}
	if cli == nil {
		t.Fatal("NewBeadCLI returned nil")
	}
}

func TestIntegration_NewBeadCLI_BadBinary(t *testing.T) {
	_, err := NewBeadCLI(context.Background(), "nonexistent-bead-cli-xyz")
	if err == nil {
		t.Fatal("want error for missing binary, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("want 'not found' in error, got %v", err)
	}
}

func TestIntegration_Create(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	id, err := cli.Create(ctx, CreateOpts{
		Title:  "test: Widget",
		Type:   "task",
		Labels: []string{"spec_module:test", "spec_hash:abc123", "spec_component:Widget"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == "" {
		t.Fatal("Create returned empty ID")
	}

	bead := brShow(t, cli.bin, id)
	if got := bead["title"].(string); got != "test: Widget" {
		t.Errorf("title: want %q, got %q", "test: Widget", got)
	}
	if got := bead["issue_type"].(string); got != "task" {
		t.Errorf("type: want %q, got %q", "task", got)
	}

	labels := toStringSlice(t, bead["labels"])
	wantLabels := []string{"spec_module:test", "spec_hash:abc123", "spec_component:Widget"}
	for _, want := range wantLabels {
		if !containsStr(labels, want) {
			t.Errorf("want label %q in %v", want, labels)
		}
	}
}

func TestIntegration_FindExisting(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	// Create a bead to find.
	id, err := cli.Create(ctx, CreateOpts{
		Title:  "find: Target",
		Type:   "task",
		Labels: []string{"spec_module:find", "spec_component:Target"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Find by matching labels.
	found, err := cli.FindExisting(ctx, []string{"spec_module:find", "spec_component:Target"})
	if err != nil {
		t.Fatalf("FindExisting: %v", err)
	}
	if found != id {
		t.Errorf("FindExisting: want %q, got %q", id, found)
	}

	// Non-matching labels return empty.
	notFound, err := cli.FindExisting(ctx, []string{"spec_module:nonexistent"})
	if err != nil {
		t.Fatalf("FindExisting (no match): %v", err)
	}
	if notFound != "" {
		t.Errorf("FindExisting (no match): want empty, got %q", notFound)
	}
}

func TestIntegration_Close(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	id, err := cli.Create(ctx, CreateOpts{
		Title:  "close: Victim",
		Type:   "task",
		Labels: []string{"spec_module:close"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := cli.Close(ctx, id, "spec node removed"); err != nil {
		t.Fatalf("Close: %v", err)
	}

	bead := brShow(t, cli.bin, id)
	if got := bead["status"].(string); got != "closed" {
		t.Errorf("status after close: want %q, got %q", "closed", got)
	}
}

func TestIntegration_Update(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	id, err := cli.Create(ctx, CreateOpts{
		Title:  "update: Target",
		Type:   "task",
		Labels: []string{"spec_module:update"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := cli.Update(ctx, id, map[string]string{"spec_hash": "newhash999"}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	bead := brShow(t, cli.bin, id)
	labels := toStringSlice(t, bead["labels"])
	if !containsStr(labels, "spec_hash:newhash999") {
		t.Errorf("want label spec_hash:newhash999 after update, got %v", labels)
	}
}

func TestIntegration_CreateBeads_Idempotency(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	actions := []Action{
		{Module: "idem", Node: "Widget", NodeType: "component", SpecHash: "h1"},
	}

	ids1, err := CreateBeads(ctx, cli, actions)
	if err != nil {
		t.Fatalf("CreateBeads first call: %v", err)
	}
	if len(ids1) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids1))
	}

	ids2, err := CreateBeads(ctx, cli, actions)
	if err != nil {
		t.Fatalf("CreateBeads second call: %v", err)
	}
	if len(ids2) != 1 {
		t.Fatalf("want 1 ID, got %d", len(ids2))
	}

	if ids1[0] != ids2[0] {
		t.Errorf("idempotency: want same ID, got %q and %q", ids1[0], ids2[0])
	}
}

func TestIntegration_CloseBeads(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	// Create two beads to close.
	id1, err := cli.Create(ctx, CreateOpts{Title: "close: A", Type: "task", Labels: []string{"spec_module:cb"}})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	id2, err := cli.Create(ctx, CreateOpts{Title: "close: B", Type: "task", Labels: []string{"spec_module:cb"}})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	actions := []Action{
		{Module: "cb", Node: "A", BeadID: id1},
		{Module: "cb", Node: "B", BeadID: id2},
	}

	if err := CloseBeads(ctx, cli, actions, testLogger()); err != nil {
		t.Fatalf("CloseBeads: %v", err)
	}

	for _, id := range []string{id1, id2} {
		bead := brShow(t, cli.bin, id)
		if got := bead["status"].(string); got != "closed" {
			t.Errorf("bead %s: want status closed, got %q", id, got)
		}
	}
}

func TestIntegration_UpdateBeads(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	id, err := cli.Create(ctx, CreateOpts{
		Title:  "update: Comp",
		Type:   "task",
		Labels: []string{"spec_module:ub", "spec_hash:old"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	actions := []Action{
		{Module: "ub", Node: "Comp", BeadID: id, SpecHash: "newhash"},
	}

	if err := UpdateBeads(ctx, cli, actions, testLogger()); err != nil {
		t.Fatalf("UpdateBeads: %v", err)
	}

	bead := brShow(t, cli.bin, id)
	labels := toStringSlice(t, bead["labels"])
	if !containsStr(labels, "spec_hash:newhash") {
		t.Errorf("want label spec_hash:newhash, got %v", labels)
	}
}

func TestIntegration_TagWithProposal(t *testing.T) {
	cli := initSandbox(t)
	ctx := context.Background()

	id1, err := cli.Create(ctx, CreateOpts{Title: "tag: X", Type: "task", Labels: []string{"spec_module:tag"}})
	if err != nil {
		t.Fatalf("Create X: %v", err)
	}
	id2, err := cli.Create(ctx, CreateOpts{Title: "tag: Y", Type: "task", Labels: []string{"spec_module:tag"}})
	if err != nil {
		t.Fatalf("Create Y: %v", err)
	}

	proposal := "2026-02-23-spex-machina"
	if err := TagWithProposal(ctx, cli, []string{id1, id2}, proposal, testLogger()); err != nil {
		t.Fatalf("TagWithProposal: %v", err)
	}

	want := fmt.Sprintf("spec_proposal:%s", proposal)
	for _, id := range []string{id1, id2} {
		bead := brShow(t, cli.bin, id)
		labels := toStringSlice(t, bead["labels"])
		if !containsStr(labels, want) {
			t.Errorf("bead %s: want label %q, got %v", id, want, labels)
		}
	}
}

// toStringSlice extracts a []string from a JSON-decoded []interface{}.
func toStringSlice(t *testing.T, v interface{}) []string {
	t.Helper()
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("want []interface{}, got %T", v)
	}
	out := make([]string, len(arr))
	for i, elem := range arr {
		out[i] = elem.(string)
	}
	return out
}

// containsStr checks if s is in the slice.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
