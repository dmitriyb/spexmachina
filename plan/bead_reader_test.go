package plan

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestReadBeadsBytes_WrappedShape(t *testing.T) {
	data := []byte(`{"issues":[{"id":"sm-1","status":"open","labels":["spex:6a7b8c9d0e1f"]}]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Bead{{ID: "sm-1", Status: "open"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestReadBeadsBytes_BareArrayShape(t *testing.T) {
	data := []byte(`[{"id":"sm-2","status":"closed","labels":["spex:0123456789ab","team:x"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Bead{{ID: "sm-2", Status: "closed"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestReadBeadsBytes_OrderPreserved(t *testing.T) {
	data := []byte(`{"issues":[
		{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa"]},
		{"id":"b","status":"open","labels":["spex:bbbbbbbbbbbb"]},
		{"id":"c","status":"open","labels":["spex:cccccccccccc"]}
	]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 beads, got %d", len(got))
	}
	wantIDs := []string{"a", "b", "c"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("index %d: got ID %q, want %q", i, got[i].ID, id)
		}
	}
}

// TestReadBeadsBytes_NoLabelParsing verifies every bead is carried through
// regardless of its labels' shape — legacy integers, cleanup- prefixes,
// unrelated markers — since BeadReader parses none of them (arch_bead_reader.md,
// "No Label Parsing").
func TestReadBeadsBytes_NoLabelParsing(t *testing.T) {
	data := []byte(`[
		{"id":"spec-bead","status":"open","labels":["spex:aaaaaaaaaaaa"]},
		{"id":"plain-bead","status":"open","labels":["priority:high"]},
		{"id":"no-labels","status":"open","labels":[]},
		{"id":"legacy-int","status":"open","labels":["spex:42"]},
		{"id":"cleanup-marker","status":"closed","labels":["spex:cleanup-aaaaaaaaaaaa"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Bead{
		{ID: "spec-bead", Status: "open"},
		{ID: "plain-bead", Status: "open"},
		{ID: "no-labels", Status: "open"},
		{ID: "legacy-int", Status: "open"},
		{ID: "cleanup-marker", Status: "closed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestReadBeadsBytes_EmptyArrayReturnsEmptySlice(t *testing.T) {
	cases := map[string][]byte{
		"bare":    []byte(`[]`),
		"wrapped": []byte(`{"issues":[]}`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ReadBeadsBytes(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("want 0 beads, got %d: %#v", len(got), got)
			}
		})
	}
}

func TestReadBeadsBytes_MalformedJSONError(t *testing.T) {
	_, err := ReadBeadsBytes([]byte(`not json`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "plan: read beads: parse:") {
		t.Errorf("want parse error prefix, got: %v", err)
	}
}

func TestReadBeadsBytes_MissingBeadID(t *testing.T) {
	data := []byte(`[
		{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa"]},
		{"status":"open","labels":["spex:bbbbbbbbbbbb"]}
	]`)

	_, err := ReadBeadsBytes(data)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "plan: read beads: missing bead id at index 1") {
		t.Errorf("want missing-id error at index 1, got: %v", err)
	}
}

func TestReadBeadsBytes_CarriesLiveStatus(t *testing.T) {
	data := []byte(`[
		{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa"]},
		{"id":"b","status":"in_progress","labels":["spex:bbbbbbbbbbbb"]},
		{"id":"c","status":"closed","labels":["spex:cccccccccccc"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStatus := []string{"open", "in_progress", "closed"}
	for i, s := range wantStatus {
		if got[i].Status != s {
			t.Errorf("index %d: want status %q, got %q", i, s, got[i].Status)
		}
	}
}

func TestReadBeads_ReaderEquivalentToBytes(t *testing.T) {
	data := []byte(`{"issues":[{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa"]}]}`)

	fromBytes, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("ReadBeadsBytes: %v", err)
	}
	fromReader, err := ReadBeads(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadBeads: %v", err)
	}
	if !reflect.DeepEqual(fromBytes, fromReader) {
		t.Errorf("reader/bytes mismatch: bytes=%#v reader=%#v", fromBytes, fromReader)
	}
}

// --- Integration scenarios from spec/plan/test_bead_matching.md ---
//
// S1 and S2 exercise BeadReader alone, against the shape it actually
// consumes: a tracker listing (arch_bead_reader.md's "Input Shape"), not
// the task journal — BeadReader starts no process and contacts no tracker,
// and never touches .history.jsonl. S3 onward exercise NodeMatcher and
// belong to that component's own bead.

// S1: BeadReader carries id and status, parses no labels. Every entry
// carries exactly the two fields the interface promises, in input order,
// and nothing derived from a label.
func TestS1_BeadReaderCarriesIDAndStatus_ParsesNoLabels(t *testing.T) {
	data := []byte(`{"issues":[
		{"id":"spex-001","status":"open",       "labels":["spex:<HEAD>:op-1","commit:deadbeef"]},
		{"id":"spex-002","status":"in_progress","labels":["spex:<HEAD>:op-2"]},
		{"id":"spex-003","status":"open",       "labels":["spex:<HEAD>:op-3"]}
	]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []Bead{
		{ID: "spex-001", Status: "open"},
		{ID: "spex-002", Status: "in_progress"},
		{ID: "spex-003", Status: "open"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}

	// Bead's only fields are ID and Status: verify reflectively that no
	// third field sneaks in carrying anything derived from a label.
	typ := reflect.TypeOf(Bead{})
	if typ.NumField() != 2 {
		t.Fatalf("Bead has %d fields, want 2 (ID, Status) — no label-derived field", typ.NumField())
	}
}

// S2: BeadReader returns an empty slice, not an error, when the input is a
// valid JSON array with no beads. Absence and emptiness are both
// first-class states.
func TestS2_BeadReaderEmptySliceOnEmptyInput(t *testing.T) {
	cases := map[string][]byte{
		"empty issues array": []byte(`{"issues":[]}`),
		"empty bare array":   []byte(`[]`),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ReadBeadsBytes(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("want empty slice, got %d: %#v", len(got), got)
			}
		})
	}
}
