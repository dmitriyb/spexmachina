package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/merkle"
)

// writeFile writes content to <dir>/<name>, creating parent directories as
// needed.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// initialisedProject builds a fresh project root whose .spex/ holds a
// parseable empty-tree snapshot and an empty journal — the state spex
// init leaves behind.
func initialisedProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, StateDirName)
	if err := merkle.Save(merkle.EmptyTree(), filepath.Join(stateDir, SnapshotFileName), time.Now().UTC()); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	writeFile(t, stateDir, JournalFileName, "")
	return root
}

// snapshotDirBytes reads every regular file under root and returns its
// content keyed by path, for the read-only assertion: Resolve must never
// change what is on disk.
func snapshotDirBytes(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func assertByteIdentical(t *testing.T, root string, before map[string][]byte) {
	t.Helper()
	after := snapshotDirBytes(t, root)
	if len(after) != len(before) {
		t.Fatalf("file count changed: before=%d after=%d", len(before), len(after))
	}
	for path, want := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file disappeared: %s", path)
		}
		if string(got) != string(want) {
			t.Fatalf("file %s changed: before=%q after=%q", path, want, got)
		}
	}
}

// TestResolve_InitialisedResolves covers "Initialised resolves": a
// project whose .spex/ carries a parseable snapshot and journal resolves
// to a context whose paths sit inside .spex/.
func TestResolve_InitialisedResolves(t *testing.T) {
	root := initialisedProject(t)

	ctx, err := Resolve(root)
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}

	stateDir := filepath.Join(root, StateDirName)
	if !strings.HasPrefix(ctx.SnapshotPath, stateDir+string(filepath.Separator)) {
		t.Errorf("SnapshotPath = %q, want under %q", ctx.SnapshotPath, stateDir)
	}
	if !strings.HasPrefix(ctx.JournalPath, stateDir+string(filepath.Separator)) {
		t.Errorf("JournalPath = %q, want under %q", ctx.JournalPath, stateDir)
	}
	if _, err := os.Stat(ctx.SnapshotPath); err != nil {
		t.Errorf("SnapshotPath does not exist: %v", err)
	}
	if _, err := os.Stat(ctx.JournalPath); err != nil {
		t.Errorf("JournalPath does not exist: %v", err)
	}
}

// TestResolve_OnlyStateDirIsConsulted covers "Only .spex/ is consulted":
// state files planted at the retired pre-lifecycle in-spec locations do
// not resolve — no .spex/ means uninitialised, full stop, since the
// retired paths are not a layout Resolve ever looks at.
func TestResolve_OnlyStateDirIsConsulted(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, filepath.Join("spec", ".snapshot.json"), `{"root_hash":"x","root_key":"project","created_at":"2026-01-01T00:00:00Z","nodes":{}}`)
	writeFile(t, root, filepath.Join("spec", ".history.jsonl"), "")

	_, err := Resolve(root)
	if err == nil {
		t.Fatal("Resolve: want error, got nil")
	}
	var uninit *UninitializedError
	if !errors.As(err, &uninit) {
		t.Fatalf("want *UninitializedError, got %T: %v", err, err)
	}
}

// TestResolve_UninitialisedIsInitsError covers "Uninitialised is init's
// error": no .spex/ at all names spex init and carries the stable
// not-a-spex-project exit code.
func TestResolve_UninitialisedIsInitsError(t *testing.T) {
	root := t.TempDir()

	_, err := Resolve(root)
	if err == nil {
		t.Fatal("Resolve: want error, got nil")
	}

	var uninit *UninitializedError
	if !errors.As(err, &uninit) {
		t.Fatalf("want *UninitializedError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "spex init") {
		t.Errorf("error %q does not name spex init", err.Error())
	}
	if strings.Contains(err.Error(), "spex doctor") {
		t.Errorf("error %q wrongly names spex doctor", err.Error())
	}

	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement ExitCode()")
	}
	if got := ec.ExitCode(); got != ExitNotAProject {
		t.Errorf("ExitCode() = %d, want %d", got, ExitNotAProject)
	}
}

// TestResolve_BrokenIsDoctorsError covers "Broken is doctor's error":
// .spex/ present but the snapshot or the journal damaged names spex
// doctor, never spex init — the asymmetry the whole component exists to
// enforce, since telling a bad-merge user "run init" destroys the
// journal.
func TestResolve_BrokenIsDoctorsError(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(root string)
	}{
		{
			name: "snapshot deleted",
			break_: func(root string) {
				if err := os.Remove(filepath.Join(root, StateDirName, SnapshotFileName)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "journal malformed",
			break_: func(root string) {
				writeFile(t, filepath.Join(root, StateDirName), JournalFileName, "{not valid json\n")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := initialisedProject(t)
			tc.break_(root)

			_, err := Resolve(root)
			if err == nil {
				t.Fatal("Resolve: want error, got nil")
			}

			var broken *BrokenError
			if !errors.As(err, &broken) {
				t.Fatalf("want *BrokenError, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "spex doctor") {
				t.Errorf("error %q does not name spex doctor", err.Error())
			}
			if strings.Contains(err.Error(), "spex init") {
				t.Errorf("error %q wrongly names spex init", err.Error())
			}

			var uninit *UninitializedError
			if errors.As(err, &uninit) {
				t.Fatalf("broken state must never surface as UninitializedError")
			}
		})
	}
}

// TestResolve_EmptyStateDirIsBroken covers the edge case: .spex/ exists
// but is empty → broken, not uninitialised. The directory's presence is
// the initialisation marker.
func TestResolve_EmptyStateDirIsBroken(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, StateDirName), 0755); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(root)
	if err == nil {
		t.Fatal("Resolve: want error, got nil")
	}
	var broken *BrokenError
	if !errors.As(err, &broken) {
		t.Fatalf("want *BrokenError, got %T: %v", err, err)
	}
}

// TestResolve_StateDirIsAFile covers the edge case: .spex/ is a file, not
// a directory → broken, error names spex doctor.
func TestResolve_StateDirIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, StateDirName), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve(root)
	if err == nil {
		t.Fatal("Resolve: want error, got nil")
	}
	var broken *BrokenError
	if !errors.As(err, &broken) {
		t.Fatalf("want *BrokenError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "spex doctor") {
		t.Errorf("error %q does not name spex doctor", err.Error())
	}
}

// TestResolve_IsReadOnly covers the edge case spanning every scenario
// above: after any Resolve call, the directory's contents are
// byte-identical to its setup state — success or refusal.
func TestResolve_IsReadOnly(t *testing.T) {
	t.Run("initialised", func(t *testing.T) {
		root := initialisedProject(t)
		before := snapshotDirBytes(t, root)
		if _, err := Resolve(root); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		assertByteIdentical(t, root, before)
	})

	t.Run("uninitialised", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "sentinel.txt", "untouched")
		before := snapshotDirBytes(t, root)
		if _, err := Resolve(root); err == nil {
			t.Fatal("Resolve: want error, got nil")
		}
		assertByteIdentical(t, root, before)
	})

	t.Run("broken", func(t *testing.T) {
		root := initialisedProject(t)
		if err := os.Remove(filepath.Join(root, StateDirName, SnapshotFileName)); err != nil {
			t.Fatal(err)
		}
		before := snapshotDirBytes(t, root)
		if _, err := Resolve(root); err == nil {
			t.Fatal("Resolve: want error, got nil")
		}
		assertByteIdentical(t, root, before)
	})
}
