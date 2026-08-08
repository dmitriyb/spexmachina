package ingest

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/adapters"
)

// fieldOrder asserts that the named JSON keys appear in body in the
// given order. It tolerates other keys interleaved but rejects any
// out-of-order pair.
func fieldOrder(t *testing.T, body, label string, keys ...string) {
	t.Helper()
	last := -1
	for _, k := range keys {
		needle := `"` + k + `":`
		idx := strings.Index(body, needle)
		if idx < 0 {
			t.Fatalf("%s: missing key %q in %s", label, k, body)
		}
		if idx <= last {
			t.Fatalf("%s: key %q at %d is not after previous key (idx %d) in %s",
				label, k, idx, last, body)
		}
		last = idx
	}
}

func encode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return strings.TrimRight(buf.String(), "\n")
}

func TestReasonPrefixConstants(t *testing.T) {
	cases := map[string]string{
		"ReasonRemovedPrefix":  ReasonRemovedPrefix,
		"ReasonModifiedPrefix": ReasonModifiedPrefix,
	}
	want := map[string]string{
		"ReasonRemovedPrefix":  "Spec node removed",
		"ReasonModifiedPrefix": "Spec node modified",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s: got %q want %q", k, v, want[k])
		}
	}
}

func TestReasonPrefixes_DiscriminateClassifierOutput(t *testing.T) {
	// Impact's action_classifier.go produces close-op Reason strings as
	// "<prefix>: <module>/<node>" — see impl_action_classification.md.
	// The reconciler discriminates on prefix-match, so the prefixes must
	// be left-anchored substrings of the full reason text.
	removed := "Spec node removed: merkle/LegacyHasher"
	if !strings.HasPrefix(removed, ReasonRemovedPrefix) {
		t.Fatalf("removed reason %q must start with %q", removed, ReasonRemovedPrefix)
	}
	modified := "Spec node modified: validator/SchemaChecker"
	if !strings.HasPrefix(modified, ReasonModifiedPrefix) {
		t.Fatalf("modified reason %q must start with %q", modified, ReasonModifiedPrefix)
	}
	// "Spec node modified (new)" is the create-side reason from the
	// modified pair — also matches the modified prefix, by design, since
	// any "modified" close is paired with a same-record-id create.
	modifiedNew := "Spec node modified (new): validator/SchemaChecker"
	if !strings.HasPrefix(modifiedNew, ReasonModifiedPrefix) {
		t.Fatalf("modified-new reason %q must start with %q", modifiedNew, ReasonModifiedPrefix)
	}
}

func TestReasonPrefixes_AreDistinct(t *testing.T) {
	// The discriminator collapses if one prefix is a prefix of the other.
	if strings.HasPrefix(ReasonRemovedPrefix, ReasonModifiedPrefix) ||
		strings.HasPrefix(ReasonModifiedPrefix, ReasonRemovedPrefix) {
		t.Fatalf("reason prefixes must not nest: removed=%q modified=%q",
			ReasonRemovedPrefix, ReasonModifiedPrefix)
	}
}

func TestExitCodeConstants(t *testing.T) {
	if ExitOK != 0 {
		t.Fatalf("ExitOK: got %d want 0", ExitOK)
	}
	if ExitInputError != 1 {
		t.Fatalf("ExitInputError: got %d want 1", ExitInputError)
	}
	if ExitInvariant != 2 {
		t.Fatalf("ExitInvariant: got %d want 2", ExitInvariant)
	}
}

func TestExitCodes_Distinct(t *testing.T) {
	seen := map[int]string{}
	for name, code := range map[string]int{
		"ExitOK":         ExitOK,
		"ExitInputError": ExitInputError,
		"ExitInvariant":  ExitInvariant,
	} {
		if other, dup := seen[code]; dup {
			t.Fatalf("exit codes collide: %s and %s both = %d", name, other, code)
		}
		seen[code] = name
	}
}

func TestSummary_CanonicalFieldOrder(t *testing.T) {
	s := Summary{
		Ok:               12,
		Skipped:          1,
		Errors:           0,
		EventsAppended:   8,
		ReceiptsAppended: 10,
		SnapshotSaved:    true,
		Status:           adapters.StatusComplete,
	}
	got := encode(t, s)
	fieldOrder(t, got, "summary",
		"ok", "skipped", "errors",
		"events_appended", "receipts_appended",
		"snapshot_saved", "status",
	)
}

func TestSummary_MatchesFlowSpecExample(t *testing.T) {
	// The flow_ingest.md "Summary output" example is the canonical wire
	// shape. Pin byte-for-byte equality so any future field rename or
	// reorder breaks the test loudly.
	s := Summary{
		Ok:               10,
		Skipped:          1,
		Errors:           0,
		EventsAppended:   8,
		ReceiptsAppended: 10,
		SnapshotSaved:    true,
		Status:           adapters.StatusComplete,
	}
	got := encode(t, s)
	want := `{"ok":10,"skipped":1,"errors":0,"events_appended":8,"receipts_appended":10,"snapshot_saved":true,"status":"complete"}`
	if got != want {
		t.Fatalf("flow spec summary mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestSummary_PartialRunShape(t *testing.T) {
	// Partial top-level status surfaces verbatim and snapshot_saved is
	// false. Both fields must serialize even at their zero values.
	s := Summary{
		Ok:             2,
		Errors:         1,
		EventsAppended: 2,
		Status:         adapters.StatusPartial,
	}
	got := encode(t, s)
	if !strings.Contains(got, `"snapshot_saved":false`) {
		t.Fatalf("partial summary must keep snapshot_saved:false: %s", got)
	}
	if !strings.Contains(got, `"status":"partial"`) {
		t.Fatalf("partial summary must echo status:partial: %s", got)
	}
}

func TestSummary_AllZeroFieldsPresent(t *testing.T) {
	// All count fields, snapshot_saved, and status are part of the v1
	// contract — they must serialize even when zero/empty so consumers
	// can rely on field presence.
	s := Summary{}
	got := encode(t, s)
	for _, key := range []string{
		`"ok":0`,
		`"skipped":0`,
		`"errors":0`,
		`"events_appended":0`,
		`"receipts_appended":0`,
		`"snapshot_saved":false`,
		`"status":""`,
	} {
		if !strings.Contains(got, key) {
			t.Fatalf("zero summary missing %s: %s", key, got)
		}
	}
}

func TestSummary_DeterministicEncoding(t *testing.T) {
	s := Summary{
		Ok:             7,
		EventsAppended: 5,
		SnapshotSaved:  true,
		Status:         adapters.StatusComplete,
	}
	first := encode(t, s)
	second := encode(t, s)
	if first != second {
		t.Fatalf("encoding non-deterministic:\n%s\nvs\n%s", first, second)
	}
}

func TestSummary_RoundTrip(t *testing.T) {
	// Tooling consumes the stdout JSON — Unmarshal must recover the
	// same Summary so callers can pipe `spex ingest` through `jq` or
	// re-read it programmatically.
	original := Summary{
		Ok:               10,
		Skipped:          1,
		Errors:           2,
		EventsAppended:   7,
		ReceiptsAppended: 9,
		SnapshotSaved:    false,
		Status:           adapters.StatusPartial,
	}
	wire := encode(t, original)
	var got Summary
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != original {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, original)
	}
}

func TestSummary_StatusValuesAreFromAdaptersPackage(t *testing.T) {
	// Status field MUST carry an adapters package status value verbatim
	// — there is no separate ingest-side vocabulary. This pins the
	// cross-package contract so a future drift in adapters.Status* would
	// fail at compile time here too (via the constant references above).
	if adapters.StatusComplete != "complete" || adapters.StatusPartial != "partial" {
		t.Fatalf("adapters status drift: complete=%q partial=%q",
			adapters.StatusComplete, adapters.StatusPartial)
	}
}
