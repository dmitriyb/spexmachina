package apply

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockCLI implements BeadCLI for testing without external binaries.
type mockCLI struct {
	created  []CreateOpts      // recorded Create calls
	existing map[string]string // label key → bead ID for FindExisting
	createFn func(CreateOpts) (string, error)
	closeFn  func(id, reason string) error
	updateFn func(id string, metadata map[string]string) error
	closed   []closedBead    // recorded Close calls
	updated  []updatedBead   // recorded Update calls
	nextID   int
}

type closedBead struct {
	ID     string
	Reason string
}

type updatedBead struct {
	ID       string
	Metadata map[string]string
}

func newMockCLI() *mockCLI {
	return &mockCLI{
		existing: make(map[string]string),
	}
}

func (m *mockCLI) Create(_ context.Context, opts CreateOpts) (string, error) {
	if m.createFn != nil {
		return m.createFn(opts)
	}
	m.created = append(m.created, opts)
	m.nextID++
	return fmt.Sprintf("mock-%d", m.nextID), nil
}

func (m *mockCLI) FindExisting(_ context.Context, labels []string) (string, error) {
	key := strings.Join(labels, ",")
	if id, ok := m.existing[key]; ok {
		return id, nil
	}
	return "", nil
}

func (m *mockCLI) Close(_ context.Context, id string, reason string) error {
	if m.closeFn != nil {
		return m.closeFn(id, reason)
	}
	m.closed = append(m.closed, closedBead{ID: id, Reason: reason})
	return nil
}

func (m *mockCLI) Update(_ context.Context, id string, metadata map[string]string) error {
	if m.updateFn != nil {
		return m.updateFn(id, metadata)
	}
	m.updated = append(m.updated, updatedBead{ID: id, Metadata: metadata})
	return nil
}

func (m *mockCLI) setExisting(labels []string, id string) {
	key := strings.Join(labels, ",")
	m.existing[key] = id
}

func TestREQ1_CreateBeads_NewBeads(t *testing.T) {
	cli := newMockCLI()
	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", NodeType: "component", SpecHash: "abc123"},
		{Module: "merkle", Node: "hashing", NodeType: "impl_section", SpecHash: "def456"},
	}

	ids, err := CreateBeads(context.Background(), cli, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 2 {
		t.Fatalf("want 2 IDs, got %d", len(ids))
	}
	if len(cli.created) != 2 {
		t.Fatalf("want 2 Create calls, got %d", len(cli.created))
	}

	// Verify first bead
	got := cli.created[0]
	if got.Title != "validator: SchemaChecker" {
		t.Errorf("want title %q, got %q", "validator: SchemaChecker", got.Title)
	}
	if got.Type != "task" {
		t.Errorf("want type %q, got %q", "task", got.Type)
	}
	wantLabels := "spec_module:validator,spec_hash:abc123,spec_component:SchemaChecker"
	gotLabels := strings.Join(got.Labels, ",")
	if gotLabels != wantLabels {
		t.Errorf("want labels %q, got %q", wantLabels, gotLabels)
	}

	// Verify second bead uses impl_section label
	got2 := cli.created[1]
	if !containsLabel(got2.Labels, "spec_impl_section:hashing") {
		t.Errorf("want spec_impl_section label, got %v", got2.Labels)
	}
	if containsLabel(got2.Labels, "spec_component:") {
		t.Errorf("impl_section should not have spec_component label, got %v", got2.Labels)
	}
}

func TestREQ1_CreateBeads_Idempotency(t *testing.T) {
	cli := newMockCLI()
	cli.setExisting(
		[]string{"spec_module:validator", "spec_component:SchemaChecker"},
		"existing-1",
	)

	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", NodeType: "component", SpecHash: "abc123"},
		{Module: "merkle", Node: "TreeBuilder", NodeType: "component", SpecHash: "def456"},
	}

	ids, err := CreateBeads(context.Background(), cli, actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ids) != 2 {
		t.Fatalf("want 2 IDs, got %d", len(ids))
	}
	if ids[0] != "existing-1" {
		t.Errorf("want existing ID %q, got %q", "existing-1", ids[0])
	}
	if len(cli.created) != 1 {
		t.Errorf("want 1 Create call (second action only), got %d", len(cli.created))
	}
}

func TestREQ1_CreateBeads_CreateError(t *testing.T) {
	cli := newMockCLI()
	cli.createFn = func(opts CreateOpts) (string, error) {
		return "", fmt.Errorf("connection refused")
	}

	actions := []Action{
		{Module: "validator", Node: "SchemaChecker", NodeType: "component", SpecHash: "abc123"},
	}

	_, err := CreateBeads(context.Background(), cli, actions)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("want error containing %q, got %v", "connection refused", err)
	}
	if !strings.Contains(err.Error(), "validator/SchemaChecker") {
		t.Errorf("want error containing action path, got %v", err)
	}
}

func TestREQ1_SpecLabels(t *testing.T) {
	tests := []struct {
		name       string
		action     Action
		wantLabels []string
	}{
		{
			name:   "component",
			action: Action{Module: "apply", Node: "BeadCreator", NodeType: "component", SpecHash: "h1"},
			wantLabels: []string{
				"spec_module:apply",
				"spec_hash:h1",
				"spec_component:BeadCreator",
			},
		},
		{
			name:   "impl_section",
			action: Action{Module: "merkle", Node: "hashing", NodeType: "impl_section", SpecHash: "h2"},
			wantLabels: []string{
				"spec_module:merkle",
				"spec_hash:h2",
				"spec_impl_section:hashing",
			},
		},
		{
			name:   "unknown node type",
			action: Action{Module: "schema", Node: "Foo", NodeType: "other", SpecHash: "h3"},
			wantLabels: []string{
				"spec_module:schema",
				"spec_hash:h3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := specLabels(tt.action)
			if len(got) != len(tt.wantLabels) {
				t.Fatalf("want %d labels, got %d: %v", len(tt.wantLabels), len(got), got)
			}
			for i, want := range tt.wantLabels {
				if got[i] != want {
					t.Errorf("label[%d]: want %q, got %q", i, want, got[i])
				}
			}
		})
	}
}

func TestREQ1_IdempotencyLabels_ExcludeHash(t *testing.T) {
	a := Action{Module: "validator", Node: "SchemaChecker", NodeType: "component", SpecHash: "abc123"}
	labels := idempotencyLabels(a)

	for _, l := range labels {
		if strings.HasPrefix(l, "spec_hash:") {
			t.Errorf("idempotency labels should not include spec_hash, got %v", labels)
		}
	}
}

func TestREQ1_CreateBeads_Empty(t *testing.T) {
	cli := newMockCLI()
	ids, err := CreateBeads(context.Background(), cli, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("want 0 IDs for empty input, got %d", len(ids))
	}
}

func containsLabel(labels []string, prefix string) bool {
	for _, l := range labels {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}
