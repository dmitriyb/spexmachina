package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dmitriyb/spexmachina/cli"
	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/merkle"
)

// runInit executes `spex init` with the given args and returns stdout,
// stderr and the process exit code, mirroring runDoctor's harness.
func runInit(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newInitCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"init"}, args...))

	var execErr error
	stdout = captureStdout(t, func() {
		execErr = rootCmd.Execute()
	})

	if execErr != nil {
		fmt.Fprintln(errBuf, execErr)
		exitCode = 1
		var ec interface{ ExitCode() int }
		if errors.As(execErr, &ec) {
			exitCode = ec.ExitCode()
		}
	}
	return stdout, errBuf.String(), exitCode
}

// TestInit_SeedsEmptyTree covers "Init seeds the empty tree, not the
// current spec": run spex init in a directory that already contains a
// populated spec/ tree. The written snapshot must encode the canonical
// empty tree, not a snapshot of the present spec — the check that fails
// in the dangerous direction, since a snapshot seeded from the current
// spec makes the first diff clean and no work is ever born.
func TestInit_SeedsEmptyTree(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeValidSpecFiles(t, specDir)

	stdout, stderr, exitCode := runInit(t, "--spec-dir", specDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s stderr=%s", exitCode, stdout, stderr)
	}

	snapshotPath := filepath.Join(root, lifecycle.StateDirName, lifecycle.SnapshotFileName)
	tree, err := merkle.Load(snapshotPath)
	if err != nil {
		t.Fatalf("load written snapshot: %v", err)
	}

	empty := merkle.EmptyTree()
	if tree.Hash != empty.Hash {
		t.Errorf("snapshot root hash = %q, want the empty tree's %q", tree.Hash, empty.Hash)
	}
	if tree.Key != empty.Key {
		t.Errorf("snapshot root key = %q, want %q", tree.Key, empty.Key)
	}
	if len(tree.Children) != 0 {
		t.Errorf("snapshot has %d children, want 0 (the populated spec/ tree must not leak in)", len(tree.Children))
	}
}

// TestInit_EmptyJournalNoEvent covers "Init writes an empty journal, no
// init event": after spex init, the journal file exists and contains zero
// lines/bytes. An event written at birth would make "no cycle has
// completed" permanently false.
func TestInit_EmptyJournalNoEvent(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")

	stdout, stderr, exitCode := runInit(t, "--spec-dir", specDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s stderr=%s", exitCode, stdout, stderr)
	}

	journalPath := filepath.Join(root, lifecycle.StateDirName, lifecycle.JournalFileName)
	data, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("journal has %d bytes, want 0 (zero lines, no init event): %q", len(data), data)
	}
}

// TestInit_RefusesInitializedDirectory covers "Init refuses an
// initialised directory": spex init where .spex/ exists → non-zero exit,
// and every byte under .spex/ is unchanged — the journal survives.
func TestInit_RefusesInitializedDirectory(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now().UTC())

	before := dirBytes(t, root)

	_, stderr, exitCode := runInit(t, "--spec-dir", specDir)
	if exitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero; stderr=%s", stderr)
	}

	assertDirByteIdentical(t, root, before)
}

// TestInit_RetiredPathsIgnored covers the edge case: spex init in a
// directory carrying state files at the retired pre-lifecycle in-spec
// locations but no .spex/ proceeds as in any uninitialised directory and
// writes nothing outside .spex/. The retired paths are not a layout and
// are neither read nor migrated.
func TestInit_RetiredPathsIgnored(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	retiredSnapshot := filepath.Join(specDir, ".snapshot.json")
	retiredJournal := filepath.Join(specDir, ".history.jsonl")
	if err := os.WriteFile(retiredSnapshot, []byte(`{"root_hash":"x","root_key":"project","created_at":"2026-01-01T00:00:00Z","nodes":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retiredJournal, nil, 0644); err != nil {
		t.Fatal(err)
	}
	beforeRetired := dirBytes(t, specDir)

	_, stderr, exitCode := runInit(t, "--spec-dir", specDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, stderr)
	}

	stateDir := filepath.Join(root, lifecycle.StateDirName)
	if _, err := os.Stat(filepath.Join(stateDir, lifecycle.SnapshotFileName)); err != nil {
		t.Errorf(".spex/ snapshot not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, lifecycle.JournalFileName)); err != nil {
		t.Errorf(".spex/ journal not written: %v", err)
	}

	assertDirByteIdentical(t, specDir, beforeRetired)
}
