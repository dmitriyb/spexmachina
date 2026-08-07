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
	data := []byte(`{"issues":[{"id":"sm-1","status":"open","labels":["spex:6a7b8c9d0e1f"]}]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []BeadSpec{{
		ID:         "sm-1",
		SpecNodeID: "6a7b8c9d0e1f",
		Status:     "open",
		Labels:     []string{"spex:6a7b8c9d0e1f"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestReadBeadsBytes_BareArrayShape verifies that ReadBeadsBytes accepts the
// bare [...] form for adapter-produced JSON.
func TestReadBeadsBytes_BareArrayShape(t *testing.T) {
	data := []byte(`[{"id":"sm-2","status":"closed","labels":["spex:0123456789ab","team:x"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []BeadSpec{{
		ID:         "sm-2",
		SpecNodeID: "0123456789ab",
		Status:     "closed",
		Labels:     []string{"spex:0123456789ab", "team:x"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

// TestReadBeadsBytes_ProposalSlugLabel verifies that a spex:<slug> label
// following the dated-stem proposal convention is recognized as a live
// spec node id.
func TestReadBeadsBytes_ProposalSlugLabel(t *testing.T) {
	data := []byte(`[{"id":"sm-epic","status":"open","labels":["spex:2026-04-18-decouple-spex-from-br"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead, got %d", len(got))
	}
	if got[0].SpecNodeID != "2026-04-18-decouple-spex-from-br" {
		t.Errorf("want proposal slug as SpecNodeID, got %q", got[0].SpecNodeID)
	}
}

// TestReadBeadsBytes_OrderPreserved verifies BeadSpec output preserves the
// input JSON order (determinism requirement).
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

// TestReadBeadsBytes_SkipsNonSpecManaged verifies beads without a live-form
// spex: label are dropped from the output (not errors).
func TestReadBeadsBytes_SkipsNonSpecManaged(t *testing.T) {
	data := []byte(`[
		{"id":"spec-bead","status":"open","labels":["spex:aaaaaaaaaaaa"]},
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
		{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa"]},
		{"status":"open","labels":["spex:bbbbbbbbbbbb"]}
	]`)

	_, err := ReadBeadsBytes(data)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "impact: read beads: missing bead id at index 1") {
		t.Errorf("want missing-id error at index 1, got: %v", err)
	}
}

// TestReadBeadsBytes_FirstLiveFormLabelWins verifies the first label whose
// suffix parses as a hash or slug wins when multiple spex: labels are
// present.
func TestReadBeadsBytes_FirstLiveFormLabelWins(t *testing.T) {
	data := []byte(`[{"id":"a","status":"open","labels":["spex:aaaaaaaaaaaa","spex:bbbbbbbbbbbb"]}]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead, got %d", len(got))
	}
	if got[0].SpecNodeID != "aaaaaaaaaaaa" {
		t.Errorf("want first label to win, got %q", got[0].SpecNodeID)
	}
}

// TestReadBeadsBytes_LegacyIntegerLabelInert verifies the legacy spex:<n>
// label form is recognized as inert history, not a live pairing.
func TestReadBeadsBytes_LegacyIntegerLabelInert(t *testing.T) {
	data := []byte(`[
		{"id":"legacy","status":"open","labels":["spex:42"]},
		{"id":"live","status":"open","labels":["spex:aaaaaaaaaaaa"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead (legacy dropped), got %d", len(got))
	}
	if got[0].ID != "live" {
		t.Errorf("want live, got %s", got[0].ID)
	}
}

// TestReadBeadsBytes_CleanupPrefixInert verifies the spex:cleanup-<hash>
// form and the bare markers spex:obsolete / spex:cleanup are passed over as
// inert rather than treated as a live spec node id.
func TestReadBeadsBytes_CleanupPrefixInert(t *testing.T) {
	data := []byte(`[
		{"id":"cleanup-hash","status":"closed","labels":["spex:cleanup-aaaaaaaaaaaa"]},
		{"id":"obsolete-marker","status":"closed","labels":["spex:obsolete"]},
		{"id":"bare-cleanup","status":"closed","labels":["spex:cleanup"]},
		{"id":"live","status":"open","labels":["spex:bbbbbbbbbbbb"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead (markers dropped), got %d: %#v", len(got), got)
	}
	if got[0].ID != "live" {
		t.Errorf("want live, got %s", got[0].ID)
	}
}

// TestReadBeadsBytes_NonLiveFormLabelSkipsToNextLabel verifies a bead whose
// spex: labels all fail to parse is dropped, but one that carries a
// non-live-form label ahead of a live one still resolves via the live one.
func TestReadBeadsBytes_NonLiveFormLabelSkipsToNextLabel(t *testing.T) {
	data := []byte(`[
		{"id":"bad-only","status":"open","labels":["spex:42"]},
		{"id":"bad-then-good","status":"open","labels":["spex:42","spex:cccccccccccc"]}
	]`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 bead, got %d", len(got))
	}
	if got[0].ID != "bad-then-good" || got[0].SpecNodeID != "cccccccccccc" {
		t.Errorf("want bad-then-good/cccccccccccc, got %s/%s", got[0].ID, got[0].SpecNodeID)
	}
}

// TestReadBeadsBytes_CarriesLiveStatus verifies that bead status is carried
// through for downstream cleanup-bead gating.
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

// TestReadBeads_ReaderEquivalentToBytes verifies ReadBeads and ReadBeadsBytes
// produce the same output.
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

// TestExtractSpecNodeID covers the internal label-parsing helper.
func TestExtractSpecNodeID(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		wantID string
		wantOK bool
	}{
		{"identity hash", []string{"spex:aaaaaaaaaaaa"}, "aaaaaaaaaaaa", true},
		{"proposal slug", []string{"spex:2026-04-18-decouple-spex-from-br"}, "2026-04-18-decouple-spex-from-br", true},
		{"label among others", []string{"team:backend", "spex:bbbbbbbbbbbb", "priority:high"}, "bbbbbbbbbbbb", true},
		{"no spex label", []string{"priority:high"}, "", false},
		{"empty labels", []string{}, "", false},
		{"nil labels", nil, "", false},
		{"legacy integer form", []string{"spex:42"}, "", false},
		{"cleanup hash form", []string{"spex:cleanup-aaaaaaaaaaaa"}, "", false},
		{"bare cleanup marker", []string{"spex:cleanup"}, "", false},
		{"obsolete marker", []string{"spex:obsolete"}, "", false},
		{"first live-form label wins", []string{"spex:aaaaaaaaaaaa", "spex:bbbbbbbbbbbb"}, "aaaaaaaaaaaa", true},
		{"inert label then live label", []string{"spex:42", "spex:aaaaaaaaaaaa"}, "aaaaaaaaaaaa", true},
		{"uppercase hash does not match", []string{"spex:AAAAAAAAAAAA"}, "", false},
		{"short hash does not match", []string{"spex:aaaa"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := extractSpecNodeID(tt.labels)
			if gotID != tt.wantID || gotOK != tt.wantOK {
				t.Errorf("extractSpecNodeID(%v) = (%q, %v), want (%q, %v)",
					tt.labels, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

// --- Integration scenarios from spec/impact/test_bead_matching.md ---
//
// S1 and S2 exercise BeadReader against the shape it actually consumes: the
// tracker listing handed to --beads (arch_bead_reader.md's "Input Shape").
// BeadReader never touches the task journal — that fold is
// mapping.MappingStore's job (see spec/map/test_mapping_store.md) — so these
// scenarios are framed on br-list-json fixtures rather than journal lines.

// S1: BeadReader extracts pairings correctly. Every entry carries the four
// fields the interface promises, and every spec node id matches the
// identity hash pattern ^[a-f0-9]{12}$.
func TestS1_BeadReaderExtractsPairingsCorrectly(t *testing.T) {
	schkHash := "5c6a1e2d3f4a"
	hasrHash := "1a2b3c4d5e6f"
	data := []byte(`{"issues":[
		{"id":"spex-001","status":"open","labels":["spex:` + schkHash + `","commit:deadbeef"]},
		{"id":"spex-002","status":"in_progress","labels":["spex:` + hasrHash + `"]}
	]}`)

	got, err := ReadBeadsBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 pairings, got %d", len(got))
	}

	for _, spec := range got {
		if !identityHashPattern.MatchString(spec.SpecNodeID) {
			t.Errorf("SpecNodeID %q does not match identity hash pattern", spec.SpecNodeID)
		}
	}

	if got[0].ID != "spex-001" || got[0].SpecNodeID != schkHash || got[0].Status != "open" {
		t.Errorf("entry 0: got %+v", got[0])
	}
	if len(got[0].Labels) != 2 {
		t.Errorf("entry 0: want labels carried through, got %v", got[0].Labels)
	}
	if got[1].ID != "spex-002" || got[1].SpecNodeID != hasrHash || got[1].Status != "in_progress" {
		t.Errorf("entry 1: got %+v", got[1])
	}
}

// S2: BeadReader returns an empty slice, not an error, when no pairings
// exist. Absence and emptiness are both first-class states.
func TestS2_BeadReaderEmptyWhenNoPairingsExist(t *testing.T) {
	cases := map[string][]byte{
		"no spec-managed beads": []byte(`{"issues":[{"id":"a","status":"open","labels":["priority:high"]}]}`),
		"empty issues array":    []byte(`{"issues":[]}`),
		"empty bare array":      []byte(`[]`),
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
