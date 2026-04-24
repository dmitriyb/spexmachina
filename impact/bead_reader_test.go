package impact

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestReadBeadsBytes_WrappedShape verifies that ReadBeadsBytes accepts the
// {"issues": [...]} envelope form produced by br list --json.
func TestReadBeadsBytes_WrappedShape(t *testing.T) {
	data := []byte(`{"issues":[{"id":"sm-1","status":"open","labels":["spex:42"]}]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []BeadSpec{{
		ID:       "sm-1",
		RecordID: 42,
		Status:   "open",
		Labels:   []string{"spex:42"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestReadBeadsBytes_BareArrayShape verifies that ReadBeadsBytes accepts the
// bare [...] form for adapter-produced JSON.
func TestReadBeadsBytes_BareArrayShape(t *testing.T) {
	data := []byte(`[{"id":"sm-2","status":"closed","labels":["spex:7","team:x"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []BeadSpec{{
		ID:       "sm-2",
		RecordID: 7,
		Status:   "closed",
		Labels:   []string{"spex:7", "team:x"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestReadBeadsBytes_OrderPreserved verifies BeadSpec output preserves the
// input JSON order (determinism requirement).
func TestReadBeadsBytes_OrderPreserved(t *testing.T) {
	data := []byte(`{"issues":[
		{"id":"a","status":"open","labels":["spex:3"]},
		{"id":"b","status":"open","labels":["spex:1"]},
		{"id":"c","status":"open","labels":["spex:2"]}
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

// TestReadBeadsBytes_SkipsNonSpecManaged verifies beads without a spex: label
// are dropped from the output (not errors).
func TestReadBeadsBytes_SkipsNonSpecManaged(t *testing.T) {
	data := []byte(`[
		{"id":"spec-bead","status":"open","labels":["spex:1"]},
		{"id":"plain-bead","status":"open","labels":["priority:high"]},
		{"id":"no-labels","status":"open","labels":[]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead, got %d", len(got))
	}
	if got[0].ID != "spec-bead" {
		t.Errorf("want spec-bead, got %s", got[0].ID)
	}
}

// TestReadBeadsBytes_EmptyArrayReturnsEmptySlice verifies an empty valid
// array returns an empty slice, not an error.
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

// TestReadBeadsBytes_ValidButNoSpecManaged verifies a valid array with no
// spex-labeled beads returns an empty slice, not an error.
func TestReadBeadsBytes_ValidButNoSpecManaged(t *testing.T) {
	data := []byte(`[{"id":"plain","status":"open","labels":["priority:high"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 beads, got %d", len(got))
	}
}

// TestReadBeadsBytes_MalformedJSONError verifies malformed JSON returns an
// error wrapped with "impact: read beads: parse:".
func TestReadBeadsBytes_MalformedJSONError(t *testing.T) {
	_, err := ReadBeadsBytes([]byte(`not json`))
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "impact: read beads: parse:") {
		t.Errorf("want parse error prefix, got: %v", err)
	}
}

// TestReadBeadsBytes_MissingBeadID verifies a bead object without an id
// returns a positional error.
func TestReadBeadsBytes_MissingBeadID(t *testing.T) {
	data := []byte(`[
		{"id":"a","status":"open","labels":["spex:1"]},
		{"status":"open","labels":["spex:2"]}
	]`)

	_, err := ReadBeadsBytes(data)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "impact: read beads: missing bead id at index 1") {
		t.Errorf("want missing-id error at index 1, got: %v", err)
	}
}

// TestReadBeadsBytes_FirstSpexLabelWins verifies the first spex: label wins
// when multiple are present.
func TestReadBeadsBytes_FirstSpexLabelWins(t *testing.T) {
	data := []byte(`[{"id":"a","status":"open","labels":["spex:42","spex:99"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead, got %d", len(got))
	}
	if got[0].RecordID != 42 {
		t.Errorf("want RecordID 42 (first wins), got %d", got[0].RecordID)
	}
}

// TestReadBeadsBytes_NonNumericSpexLabelSkipped verifies beads whose spex:
// label has a non-numeric value are dropped (cannot be matched by RecordID).
func TestReadBeadsBytes_NonNumericSpexLabelSkipped(t *testing.T) {
	data := []byte(`[
		{"id":"bad","status":"open","labels":["spex:abc"]},
		{"id":"good","status":"open","labels":["spex:5"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead (bad dropped), got %d", len(got))
	}
	if got[0].ID != "good" || got[0].RecordID != 5 {
		t.Errorf("want good/5, got %s/%d", got[0].ID, got[0].RecordID)
	}
}

// TestReadBeadsBytes_CarriesLiveStatus verifies that bead status is carried
// through for downstream cleanup-bead gating.
func TestReadBeadsBytes_CarriesLiveStatus(t *testing.T) {
	data := []byte(`[
		{"id":"a","status":"open","labels":["spex:1"]},
		{"id":"b","status":"in_progress","labels":["spex:2"]},
		{"id":"c","status":"closed","labels":["spex:3"]}
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

// TestReadBeads_ReaderEquivalentToBytes verifies ReadBeads and ReadBeadsBytes
// produce the same output.
func TestReadBeads_ReaderEquivalentToBytes(t *testing.T) {
	data := []byte(`{"issues":[{"id":"a","status":"open","labels":["spex:1"]}]}`)

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

// TestExtractRecordID covers the internal label-parsing helper.
func TestExtractRecordID(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		wantID int
		wantOK bool
	}{
		{"valid spex label", []string{"spex:42"}, 42, true},
		{"spex label among others", []string{"team:backend", "spex:7", "priority:high"}, 7, true},
		{"no spex label", []string{"priority:high"}, 0, false},
		{"empty labels", []string{}, 0, false},
		{"nil labels", nil, 0, false},
		{"non-numeric spex value", []string{"spex:abc"}, 0, false},
		{"spex zero", []string{"spex:0"}, 0, true},
		{"first wins", []string{"spex:1", "spex:2"}, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := extractRecordID(tt.labels)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("extractRecordID(%v) = (%d, %v), want (%d, %v)",
					tt.labels, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}
