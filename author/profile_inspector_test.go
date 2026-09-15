package author

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// failingWriter always fails, for exercising ShowProfile's encode-error path.
type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestShowProfile_NoProfileJSON_ReturnsDefaultProfile(t *testing.T) {
	specDir := t.TempDir()

	var buf bytes.Buffer
	profile, err := ShowProfile(specDir, &buf, false)
	if err != nil {
		t.Fatalf("ShowProfile: unexpected error: %v", err)
	}

	def := schema.DefaultProfile()
	defJSON, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("marshal default profile: %v", err)
	}
	gotJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal returned profile: %v", err)
	}
	if string(gotJSON) != string(defJSON) {
		t.Fatalf("ShowProfile did not resolve to the default profile\ngot:  %s\nwant: %s", gotJSON, defJSON)
	}

	if buf.String() != string(defJSON)+"\n" {
		t.Fatalf("compact output mismatch\ngot:  %q\nwant: %q", buf.String(), string(defJSON)+"\n")
	}
}

func TestShowProfile_PrettyIndentsWhenTrue(t *testing.T) {
	specDir := t.TempDir()

	var compact, pretty bytes.Buffer
	if _, err := ShowProfile(specDir, &compact, false); err != nil {
		t.Fatalf("ShowProfile (compact): %v", err)
	}
	if _, err := ShowProfile(specDir, &pretty, true); err != nil {
		t.Fatalf("ShowProfile (pretty): %v", err)
	}

	if strings.Contains(compact.String(), "\n  ") {
		t.Fatalf("compact output should carry no indentation, got: %s", compact.String())
	}
	if !strings.Contains(pretty.String(), "\n  ") {
		t.Fatalf("pretty output should be indented, got: %s", pretty.String())
	}

	// Both encode the same JSON value.
	var compactVal, prettyVal any
	if err := json.Unmarshal(compact.Bytes(), &compactVal); err != nil {
		t.Fatalf("unmarshal compact: %v", err)
	}
	if err := json.Unmarshal(pretty.Bytes(), &prettyVal); err != nil {
		t.Fatalf("unmarshal pretty: %v", err)
	}
	compactAgain, _ := json.Marshal(compactVal)
	prettyAgain, _ := json.Marshal(prettyVal)
	if string(compactAgain) != string(prettyAgain) {
		t.Fatalf("pretty and compact output carry different JSON values")
	}
}

func TestShowProfile_MalformedProfile_PropagatesSchemaError(t *testing.T) {
	specDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(specDir, "profile.json"), []byte(`{"profile_version": 3}`), 0644); err != nil {
		t.Fatalf("write profile.json: %v", err)
	}

	var buf bytes.Buffer
	profile, err := ShowProfile(specDir, &buf, false)
	if err == nil {
		t.Fatal("want an error for an out-of-range profile_version")
	}
	if profile != nil {
		t.Fatalf("want a nil profile on error, got %+v", profile)
	}
	if buf.Len() != 0 {
		t.Fatalf("want nothing written on resolution failure, got: %s", buf.String())
	}
	// ShowProfile does not re-implement the version check; it surfaces
	// schema.ResolveProfile's own error unchanged.
	if !strings.Contains(err.Error(), "unsupported (supported: 1-2)") {
		t.Fatalf("want schema's own out-of-range message, got: %v", err)
	}
}

func TestShowProfile_EncodeFailure_Wrapped(t *testing.T) {
	specDir := t.TempDir()
	wantErr := errors.New("boom")

	_, err := ShowProfile(specDir, failingWriter{err: wantErr}, false)
	if err == nil {
		t.Fatal("want an error when the writer fails")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("want wrapped writer error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "author: encode profile:") {
		t.Fatalf("want the author module's own encode-error prefix, got: %v", err)
	}
}

func TestShowProfile_NeedsNoInitialisedProject(t *testing.T) {
	// specDir carries only project.json, no .spex/ anywhere — ShowProfile
	// must still succeed, per arch_profile_inspector.md "What it does not
	// do": "needs no initialised project and reads nothing under .spex/".
	specDir := t.TempDir()
	proj := `{"name": "p", "modules": [{"id": "000000000001", "name": "alpha", "path": "alpha"}]}`
	if err := os.WriteFile(filepath.Join(specDir, "project.json"), []byte(proj), 0644); err != nil {
		t.Fatalf("write project.json: %v", err)
	}

	var buf bytes.Buffer
	if _, err := ShowProfile(specDir, &buf, false); err != nil {
		t.Fatalf("ShowProfile should not require .spex/: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("want profile JSON written")
	}
}
