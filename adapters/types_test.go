package adapters

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fieldOrder asserts that the named JSON keys appear in body in the given
// order. It tolerates other keys interleaved but rejects any out-of-order pair.
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

func TestReceipts_VersionConstantIsOne(t *testing.T) {
	if ReceiptsVersion != 1 {
		t.Fatalf("ReceiptsVersion: got %d want 1", ReceiptsVersion)
	}
}

func TestReceipts_V1Schema(t *testing.T) {
	r := Receipts{
		Version: ReceiptsVersion,
		Status:  StatusComplete,
		Ops: []OpReceipt{
			{OpID: "op-0001", Status: OpStatusOk, BeadID: "spexmachina-abc", WasExisting: false},
		},
	}
	got := encode(t, r)
	if !strings.Contains(got, `"version":1`) {
		t.Fatalf("receipts must declare version 1: %s", got)
	}
	fieldOrder(t, got, "receipts", "version", "status", "ops")
}

func TestStatusConstants(t *testing.T) {
	cases := map[string]string{
		"StatusComplete": StatusComplete,
		"StatusPartial":  StatusPartial,
	}
	want := map[string]string{
		"StatusComplete": "complete",
		"StatusPartial":  "partial",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s: got %q want %q", k, v, want[k])
		}
	}
}

func TestOpStatusConstants(t *testing.T) {
	cases := map[string]string{
		"OpStatusOk":      OpStatusOk,
		"OpStatusSkipped": OpStatusSkipped,
		"OpStatusError":   OpStatusError,
	}
	want := map[string]string{
		"OpStatusOk":      "ok",
		"OpStatusSkipped": "skipped",
		"OpStatusError":   "error",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s: got %q want %q", k, v, want[k])
		}
	}
}

func TestOpReceipt_OkCanonicalFieldOrder(t *testing.T) {
	r := OpReceipt{
		OpID:        "op-0001",
		Status:      OpStatusOk,
		BeadID:      "spexmachina-abc",
		WasExisting: false,
	}
	got := encode(t, r)
	fieldOrder(t, got, "ok receipt",
		"op_id", "status", "bead_id", "was_existing",
	)
	for _, banned := range []string{`"reason"`, `"error"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("ok receipt leaked optional field %s: %s", banned, got)
		}
	}
}

func TestOpReceipt_SkippedShapeWithReason(t *testing.T) {
	r := OpReceipt{
		OpID:        "op-0003",
		Status:      OpStatusSkipped,
		BeadID:      "spexmachina-ghi",
		WasExisting: true,
		Reason:      "idempotent re-match",
	}
	got := encode(t, r)
	fieldOrder(t, got, "skipped receipt",
		"op_id", "status", "bead_id", "was_existing", "reason",
	)
	if strings.Contains(got, `"error"`) {
		t.Fatalf("skipped receipt must not emit error field: %s", got)
	}
}

func TestOpReceipt_ErrorShapeWithError(t *testing.T) {
	r := OpReceipt{
		OpID:        "op-0004",
		Status:      OpStatusError,
		BeadID:      "",
		WasExisting: false,
		Error:       "br create exited 1: invalid priority -1",
	}
	got := encode(t, r)
	fieldOrder(t, got, "error receipt",
		"op_id", "status", "bead_id", "was_existing", "error",
	)
	if strings.Contains(got, `"reason"`) {
		t.Fatalf("error receipt must not emit reason field: %s", got)
	}
	// bead_id "" must remain present per receipts schema
	if !strings.Contains(got, `"bead_id":""`) {
		t.Fatalf("error receipt must keep empty bead_id field: %s", got)
	}
}

func TestOpReceipt_BeadIDPresentEvenWhenEmpty(t *testing.T) {
	// The receipts contract requires bead_id to appear in every entry,
	// even when no bead was created (error / pre-create failure).
	r := OpReceipt{OpID: "op-0001", Status: OpStatusError}
	got := encode(t, r)
	if !strings.Contains(got, `"bead_id":""`) {
		t.Fatalf("bead_id must serialize even when empty: %s", got)
	}
}

func TestOpReceipt_WasExistingPresentEvenWhenFalse(t *testing.T) {
	// was_existing is part of the v1 contract: always present, never elided.
	r := OpReceipt{OpID: "op-0001", Status: OpStatusOk, BeadID: "spexmachina-abc"}
	got := encode(t, r)
	if !strings.Contains(got, `"was_existing":false`) {
		t.Fatalf("was_existing must serialize even when false: %s", got)
	}
}

func TestReceipts_DeterministicEncoding(t *testing.T) {
	r := Receipts{
		Version: ReceiptsVersion,
		Status:  StatusComplete,
		Ops: []OpReceipt{
			{OpID: "op-0001", Status: OpStatusOk, BeadID: "spexmachina-abc"},
			{OpID: "op-0002", Status: OpStatusOk, BeadID: "spexmachina-def", WasExisting: true},
		},
	}
	first := encode(t, r)
	second := encode(t, r)
	if first != second {
		t.Fatalf("encoding is non-deterministic:\n%s\nvs\n%s", first, second)
	}
}

func TestReceipts_RoundTrip(t *testing.T) {
	// Ingest reads receipts.json — verify Unmarshal recovers the same struct.
	original := Receipts{
		Version: ReceiptsVersion,
		Status:  StatusPartial,
		Ops: []OpReceipt{
			{OpID: "op-0001", Status: OpStatusOk, BeadID: "spexmachina-abc"},
			{OpID: "op-0002", Status: OpStatusSkipped, BeadID: "spexmachina-def", WasExisting: true, Reason: "idempotent re-match"},
			{OpID: "op-0003", Status: OpStatusError, Error: "br exited 1"},
		},
	}
	wire := encode(t, original)
	var got Receipts
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version != original.Version || got.Status != original.Status {
		t.Fatalf("top-level mismatch: got %+v want %+v", got, original)
	}
	if len(got.Ops) != len(original.Ops) {
		t.Fatalf("ops length: got %d want %d", len(got.Ops), len(original.Ops))
	}
	for i, op := range got.Ops {
		if op != original.Ops[i] {
			t.Fatalf("op %d mismatch: got %+v want %+v", i, op, original.Ops[i])
		}
	}
}

func TestIdempotencyLabelConstants(t *testing.T) {
	// These literals are part of the adapter contract: emit writes
	// idempotency.label = "spex:<n>"; the reference adapter applies
	// "spex:obsolete" and "commit:<HEAD>" labels on close.
	if IdempotencyLabelPrefix != "spex:" {
		t.Fatalf("IdempotencyLabelPrefix: got %q want %q", IdempotencyLabelPrefix, "spex:")
	}
	if ObsoleteLabel != "spex:obsolete" {
		t.Fatalf("ObsoleteLabel: got %q want %q", ObsoleteLabel, "spex:obsolete")
	}
	if CommitLabelPrefix != "commit:" {
		t.Fatalf("CommitLabelPrefix: got %q want %q", CommitLabelPrefix, "commit:")
	}
}

func TestReceipts_OmitsAllOptionalFieldsOnAllOk(t *testing.T) {
	// An all-ok receipt set should never carry any reason/error keys.
	r := Receipts{
		Version: ReceiptsVersion,
		Status:  StatusComplete,
		Ops: []OpReceipt{
			{OpID: "op-0001", Status: OpStatusOk, BeadID: "spexmachina-abc"},
			{OpID: "op-0002", Status: OpStatusOk, BeadID: "spexmachina-def"},
		},
	}
	got := encode(t, r)
	for _, banned := range []string{`"reason"`, `"error"`} {
		if strings.Contains(got, banned) {
			t.Fatalf("all-ok receipts leaked %s: %s", banned, got)
		}
	}
}
