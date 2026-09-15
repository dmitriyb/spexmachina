package author

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/merkle"
)

// fieldOrder asserts that the named JSON keys appear in body in the given
// order. It tolerates other keys interleaved but rejects any out-of-order
// pair.
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

func TestExitCodeConstants(t *testing.T) {
	if ExitOK != 0 {
		t.Fatalf("ExitOK: got %d want 0", ExitOK)
	}
	if ExitInputError != 1 {
		t.Fatalf("ExitInputError: got %d want 1", ExitInputError)
	}
	if ExitRefusal != 2 {
		t.Fatalf("ExitRefusal: got %d want 2", ExitRefusal)
	}
}

func TestExitCodes_Distinct(t *testing.T) {
	seen := map[int]string{}
	for name, code := range map[string]int{
		"ExitOK":         ExitOK,
		"ExitInputError": ExitInputError,
		"ExitRefusal":    ExitRefusal,
	} {
		if other, dup := seen[code]; dup {
			t.Fatalf("exit codes collide: %s and %s both = %d", name, other, code)
		}
		seen[code] = name
	}
}

func TestNodeAddInput_FieldsKeyedByDeclaredName(t *testing.T) {
	in := NodeAddInput{
		TypeName: "requirement",
		Name:     "R2",
		Module:   "alpha",
		Fields: map[string]string{
			"type":    "functional",
			"preq_id": "P1",
		},
	}
	if in.Fields["type"] != "functional" || in.Fields["preq_id"] != "P1" {
		t.Fatalf("NodeAddInput.Fields not keyed by declared field name: %+v", in.Fields)
	}
}

func TestNodeRemoveInput_Force(t *testing.T) {
	in := NodeRemoveInput{ID: "abc123def456", Force: true}
	if !in.Force || in.ID != "abc123def456" {
		t.Fatalf("NodeRemoveInput fields not carried: %+v", in)
	}
}

func TestRenameInput_IDAndNewName(t *testing.T) {
	in := RenameInput{ID: "abc123def456", NewName: "Comp2"}
	if in.ID != "abc123def456" || in.NewName != "Comp2" {
		t.Fatalf("RenameInput fields not carried: %+v", in)
	}
}

func TestEdgeInput_SourceFieldTarget(t *testing.T) {
	in := EdgeInput{SourceID: "src000000001", Field: "implements", TargetID: "tgt000000001"}
	if in.SourceID != "src000000001" || in.Field != "implements" || in.TargetID != "tgt000000001" {
		t.Fatalf("EdgeInput fields not carried: %+v", in)
	}
}

func TestScaffoldInput_ID(t *testing.T) {
	in := ScaffoldInput{ID: "abc123def456"}
	if in.ID != "abc123def456" {
		t.Fatalf("ScaffoldInput.ID not carried: %+v", in)
	}
}

func TestRefusalEntry_CanonicalFieldOrder(t *testing.T) {
	e := RefusalEntry{
		Check:   "id",
		Message: "duplicate component name \"Comp1\" in module alpha",
		Path:    "alpha/module.json:/components/0",
		Fix:     "existing node abc123def456 in alpha/module.json",
	}
	got := encode(t, e)
	fieldOrder(t, got, "refusal entry", "check", "message", "path", "fix")
}

func TestRefusalEntry_RoundTrip(t *testing.T) {
	original := RefusalEntry{
		Check:   "dag",
		Message: "cycle detected: a -> b -> a",
		Path:    "alpha/module.json:/components/1/uses/0",
		Fix:     "remove one edge in the cycle",
	}
	wire := encode(t, original)
	var got RefusalEntry
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != original {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, original)
	}
}

func TestRefusalEntry_FixAlwaysPresent(t *testing.T) {
	// Every refusal carries a fix (arch_obligation_reporter.md, "Every
	// refusal names its fix") — Fix has no omitempty, so it serializes
	// even at its zero value, which would otherwise silently vanish and
	// look indistinguishable from "no fix".
	e := RefusalEntry{Check: "id", Message: "m", Path: "p"}
	got := encode(t, e)
	if !strings.Contains(got, `"fix":""`) {
		t.Fatalf("fix key must serialize even when empty: %s", got)
	}
}

func TestWriteReport_CanonicalFieldOrder(t *testing.T) {
	r := WriteReport{
		Written: []string{"alpha/module.json", "alpha/arch_comp1.md"},
		Obligations: []merkle.DiffError{
			{Type: "incomplete_change", Message: "m", Path: "p", Related: []string{"r1"}},
		},
		RetiredName: "Comp1",
	}
	got := encode(t, r)
	fieldOrder(t, got, "write report", "written", "obligations", "retired_name")
}

func TestWriteReport_RetiredNameOmittedWhenEmpty(t *testing.T) {
	// Only spex node rename sets retired_name; every other writing
	// surface must omit the key entirely rather than emit "".
	r := WriteReport{
		Written:     []string{"alpha/module.json"},
		Obligations: []merkle.DiffError{},
	}
	got := encode(t, r)
	if strings.Contains(got, "retired_name") {
		t.Fatalf("retired_name must be omitted when empty: %s", got)
	}
}

func TestWriteReport_ObligationsIsMerkleDiffError(t *testing.T) {
	// Obligations must reuse the completeness checker's own entry type
	// unchanged, never a reimplementation
	// (arch_obligation_reporter.md, "Obligations are printed, not
	// discovered").
	r := WriteReport{
		Obligations: []merkle.DiffError{
			{Type: "incomplete_change", Message: "requirement R2 added but not implemented by any component", Path: "r2hash", Related: nil},
		},
	}
	got := encode(t, r)
	if !strings.Contains(got, `"type":"incomplete_change"`) {
		t.Fatalf("obligations must carry merkle.DiffError's own fields verbatim: %s", got)
	}
}

func TestWriteReport_NoObligationsIsEmptyArrayNotNull(t *testing.T) {
	r := WriteReport{Written: []string{"f"}, Obligations: []merkle.DiffError{}}
	got := encode(t, r)
	if !strings.Contains(got, `"obligations":[]`) {
		t.Fatalf("obligations should serialize as [] when the write incurs none: %s", got)
	}
}

func TestWriteReport_RoundTrip(t *testing.T) {
	original := WriteReport{
		Written: []string{"alpha/module.json"},
		Obligations: []merkle.DiffError{
			{Type: "incomplete_change", Message: "m", Path: "p", Related: []string{"r1", "r2"}},
		},
		RetiredName: "Comp1",
	}
	wire := encode(t, original)
	var got WriteReport
	if err := json.Unmarshal([]byte(wire), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RetiredName != original.RetiredName || len(got.Written) != len(original.Written) || len(got.Obligations) != len(original.Obligations) {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", got, original)
	}
}
