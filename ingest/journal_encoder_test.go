package ingest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/schema"
)

// Unit tests for spec/ingest/arch_journal_encoder.md, calling
// JournalEncoder.Encode and JournalEncoder.Validate directly to pin the
// encoder's own wire shapes and validation behavior, independent of any
// caller. The spec's "Invariant 5" scenarios live at other call sites:
// "schema-invalid line refused" is TestCheckInvariant5_SchemaInvalidLine
// in reconciler_test.go, calling checkInvariant5 directly; "the encoder
// refuses at its own boundary" is
// TestConsistencyInvariants_Invariant5_EncoderRefusesAtOwnBoundary in
// consistency_invariants_test.go, which — like these tests — calls
// NewJournalEncoder().Validate directly, with no changeset or
// reconciliation run around it.

// TestJournalEncoder_Encode_ChangeEvent covers the added/modified/removed
// wire shape: all ten required keys present, before/after admitting null.
func TestJournalEncoder_Encode_ChangeEvent(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "Comp",
		NodeType: "component", Module: "m", Before: nil, After: strPtr("h1"),
		GitHead: "g1", Proposal: "p1", Path: "m/comp.md",
	}
	raw, err := NewJournalEncoder().Encode(ev)
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Encode output not valid JSON: %v", err)
	}
	want := map[string]any{
		"event": "added", "eid": "e1", "node": "aabbccddeeff", "name": "Comp",
		"node_type": "component", "module": "m", "before": nil, "after": "h1",
		"git_head": "g1", "proposal": "p1", "path": "m/comp.md",
	}
	for k, v := range want {
		if got[k] != v && !(v == nil && got[k] == nil) {
			t.Errorf("field %q = %v, want %v", k, got[k], v)
		}
	}
}

// TestJournalEncoder_Encode_ChangeEvent_OmitsPathWhenEmpty covers
// requirement/api change events, which carry no content-leaf path.
func TestJournalEncoder_Encode_ChangeEvent_OmitsPathWhenEmpty(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "Req",
		NodeType: "requirement", Module: "m", After: strPtr("h1"),
		GitHead: "g1", Proposal: "p1",
	}
	raw, err := NewJournalEncoder().Encode(ev)
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	if strings.Contains(string(raw), `"path"`) {
		t.Errorf("Encode output = %s, want no path key when Path is empty", raw)
	}
}

// TestJournalEncoder_Encode_TaskReceipt covers task_created/task_closed,
// which carry for OR proposal but never both.
func TestJournalEncoder_Encode_TaskReceipt(t *testing.T) {
	raw, err := NewJournalEncoder().Encode(mapping.Event{Event: "task_created", TaskID: "t1", For: "e1"})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	if strings.Contains(string(raw), `"proposal"`) {
		t.Errorf("Encode output = %s, want no proposal key when unset", raw)
	}

	raw, err = NewJournalEncoder().Encode(mapping.Event{Event: "task_created", TaskID: "t1", Proposal: "stem"})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	if strings.Contains(string(raw), `"for"`) {
		t.Errorf("Encode output = %s, want no for key when unset", raw)
	}
}

// TestJournalEncoder_Encode_TaskRetargeted covers task_retargeted, whose
// for is always required and which admits no proposal field at all.
func TestJournalEncoder_Encode_TaskRetargeted(t *testing.T) {
	raw, err := NewJournalEncoder().Encode(mapping.Event{Event: "task_retargeted", TaskID: "t1", For: "e1"})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Encode output not valid JSON: %v", err)
	}
	if _, ok := got["proposal"]; ok {
		t.Errorf("Encode output = %s, want no proposal key at all", raw)
	}
	if got["for"] != "e1" {
		t.Errorf("for = %v, want e1", got["for"])
	}
}

// TestJournalEncoder_Encode_Refresh covers the refresh receipt shape:
// git_head serialises as null when absent and absorbed always serialises
// as an array, even when empty.
func TestJournalEncoder_Encode_Refresh(t *testing.T) {
	raw, err := NewJournalEncoder().Encode(mapping.Event{Event: "refresh", Absorbed: nil})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Encode output not valid JSON: %v", err)
	}
	if got["git_head"] != nil {
		t.Errorf("git_head = %v, want null when GitHead is empty", got["git_head"])
	}
	absorbed, ok := got["absorbed"].([]any)
	if !ok || len(absorbed) != 0 {
		t.Errorf("absorbed = %v, want an empty array", got["absorbed"])
	}

	raw, err = NewJournalEncoder().Encode(mapping.Event{Event: "refresh", GitHead: "g1", Absorbed: []string{"e1", "e2"}})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Encode output not valid JSON: %v", err)
	}
	if got["git_head"] != "g1" {
		t.Errorf("git_head = %v, want g1", got["git_head"])
	}
}

// TestJournalEncoder_Encode_Registered covers the registered event shape
// — no writer in this package, but a legacy/seeded line JournalEncoder
// must still be able to encode.
func TestJournalEncoder_Encode_Registered(t *testing.T) {
	raw, err := NewJournalEncoder().Encode(mapping.Event{Event: "registered", EID: "e1", Proposal: "stem", GitHead: "g1"})
	if err != nil {
		t.Fatalf("Encode: unexpected error %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Encode output not valid JSON: %v", err)
	}
	if got["eid"] != "e1" || got["proposal"] != "stem" || got["git_head"] != "g1" {
		t.Errorf("Encode output = %s, want eid/proposal/git_head round-tripped", raw)
	}
}

// TestJournalEncoder_Encode_UnknownKind covers the refusal for a journal
// line kind the schema does not declare.
func TestJournalEncoder_Encode_UnknownKind(t *testing.T) {
	_, err := NewJournalEncoder().Encode(mapping.Event{Event: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("Encode: got %v, want error naming the unknown kind", err)
	}
}

// TestJournalEncoder_Validate_ValidLines is the control: a well-formed
// line of every kind must validate cleanly.
func TestJournalEncoder_Validate_ValidLines(t *testing.T) {
	enc := NewJournalEncoder()
	lines := []mapping.Event{
		{Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "x", NodeType: "component", Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p"},
		{Event: "modified", EID: "e2", Node: "112233445566", Name: "x", NodeType: "component", Module: "m", Before: strPtr("h0"), After: strPtr("h1"), GitHead: "g", Proposal: "p"},
		{Event: "removed", EID: "e3", Node: "aabbccddeeff", Name: "x", NodeType: "component", Module: "m", Before: strPtr("h0"), GitHead: "g", Proposal: "p"},
		{Event: "registered", EID: "e4", Proposal: "stem", GitHead: "g"},
		{Event: "task_created", TaskID: "t1", For: "e1"},
		{Event: "task_created", TaskID: "t1", Proposal: "stem"},
		{Event: "task_closed", TaskID: "t1", For: "e1"},
		{Event: "task_retargeted", TaskID: "t1", For: "e2"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e2"}},
	}
	for _, ev := range lines {
		if err := enc.Validate(ev); err != nil {
			t.Errorf("Validate(%s): unexpected error %v", ev.Event, err)
		}
	}
}

// TestJournalEncoder_Validate_InvalidNodeHash covers a change event whose
// node fails the schema's 12-hex-char pattern.
func TestJournalEncoder_Validate_InvalidNodeHash(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "not-a-hash", Name: "x", NodeType: "component",
		Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}
	err := NewJournalEncoder().Validate(ev)
	if err == nil {
		t.Fatal("Validate: want error for malformed node hash, got nil")
	}
}

// TestJournalEncoder_Validate_MissingNode covers the empty-node case
// (schema-invalid line refused, per test_consistency_invariants.md
// "Invariant 5: schema-invalid line refused").
func TestJournalEncoder_Validate_MissingNode(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "", Name: "x", NodeType: "component",
		Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}
	if err := NewJournalEncoder().Validate(ev); err == nil {
		t.Fatal("Validate: want error for empty node, got nil")
	}
}

// TestJournalEncoder_Validate_TaskReceiptBothForAndProposal covers the
// schema's oneOf constraint: a task_created carrying both for and
// proposal is invalid — exactly one of the two arms may apply.
func TestJournalEncoder_Validate_TaskReceiptBothForAndProposal(t *testing.T) {
	ev := mapping.Event{Event: "task_created", TaskID: "t1", For: "e1", Proposal: "stem"}
	if err := NewJournalEncoder().Validate(ev); err == nil {
		t.Fatal("Validate: want error for task_created carrying both for and proposal, got nil")
	}
}

// TestJournalEncoder_Validate_UnknownKind covers Validate's own refusal
// path when Encode itself fails — the line never reaches schema
// validation at all.
func TestJournalEncoder_Validate_UnknownKind(t *testing.T) {
	err := NewJournalEncoder().Validate(mapping.Event{Event: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("Validate: got %v, want error naming the unknown kind", err)
	}
}

// TestJournalEncoder_Validate_NodeTypeNotDeclared covers the encoder's
// own membership check: a change event that is schema-valid in every
// other respect is still refused when its node_type names a kind the
// resolved profile does not declare — the default profile, here, since
// Profile is left unset.
func TestJournalEncoder_Validate_NodeTypeNotDeclared(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "x",
		NodeType: "endpoint", Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}
	err := NewJournalEncoder().Validate(ev)
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("Validate: got %v, want error naming the undeclared kind %q", err, "endpoint")
	}
}

// TestJournalEncoder_Validate_NodeTypeDeclaredByCustomProfile is the
// control: the same otherwise-refused event validates once handed an
// encoder resolved against a profile that declares "endpoint".
func TestJournalEncoder_Validate_NodeTypeDeclaredByCustomProfile(t *testing.T) {
	ev := mapping.Event{
		Event: "added", EID: "e1", Node: "aabbccddeeff", Name: "x",
		NodeType: "endpoint", Module: "m", After: strPtr("h"), GitHead: "g", Proposal: "p",
	}
	enc := &JournalEncoder{Profile: &schema.Profile{NodeTypes: []schema.NodeType{
		{Name: "endpoint", PluralKey: "endpoints", Scope: "module"},
	}}}
	if err := enc.Validate(ev); err != nil {
		t.Fatalf("Validate under a profile declaring %q: unexpected error %v", "endpoint", err)
	}
}

// TestJournalEncoder_Validate_ProfileCheckExemptsNonChangeEvents covers
// every journal-line kind that carries no node_type at all: registered,
// task receipts, task_retargeted and refresh must all validate against
// an encoder whose profile declares nothing, since the membership check
// only ever applies to added/modified/removed.
func TestJournalEncoder_Validate_ProfileCheckExemptsNonChangeEvents(t *testing.T) {
	enc := &JournalEncoder{Profile: &schema.Profile{}}
	lines := []mapping.Event{
		{Event: "registered", EID: "e1", Proposal: "stem", GitHead: "g"},
		{Event: "task_created", TaskID: "t1", For: "e1"},
		{Event: "task_closed", TaskID: "t1", For: "e1"},
		{Event: "task_retargeted", TaskID: "t1", For: "e1"},
		{Event: "refresh", GitHead: "g", Absorbed: []string{"e1"}},
	}
	for _, ev := range lines {
		if err := enc.Validate(ev); err != nil {
			t.Errorf("Validate(%s): unexpected error %v", ev.Event, err)
		}
	}
}
