// Package lifecycle owns the on-disk contract of what a spex project is:
// the .spex/ state directory holding the snapshot and the task journal,
// and the single pre-flight — Resolve — that every subcommand touching
// project state calls before reading or writing it. See
// spec/lifecycle/arch_project_resolver.md.
package lifecycle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
)

const (
	// StateDirName is the only state-directory layout: everything spex
	// writes lives here, at the project root, alongside spec/. Authored
	// content is never resolved here — a person, not the tool, owns it.
	StateDirName = ".spex"

	// SnapshotFileName and JournalFileName are the two files Resolve
	// answers locations for, and the only two spex init ever writes into
	// a fresh state directory.
	SnapshotFileName = "snapshot.json"
	JournalFileName  = "history.jsonl"

	// ExitNotAProject is the stable, documented exit code a resolver
	// refusal carries — distinct from the input-error (1) and invariant
	// (2) exit codes used elsewhere in the pipeline. Both refusal
	// branches share it: from a caller's perspective (CI, a script) both
	// mean "this pipeline cannot proceed here"; the typed error's
	// message is what tells never-initialised and broken apart.
	ExitNotAProject = 3
)

// ProjectContext is what a successful Resolve hands back: the resolved
// snapshot and journal locations inside .spex/. Callers thread these
// locations through — no other component computes a state path.
type ProjectContext struct {
	SnapshotPath string
	JournalPath  string
}

// UninitializedError is Resolve's answer when no .spex/ exists at all:
// the project was never initialised, and spex init is the fix.
type UninitializedError struct {
	ProjectRoot string
}

func (e *UninitializedError) Error() string {
	return fmt.Sprintf("lifecycle: not a spex project (no %s at %s); run 'spex init'",
		StateDirName, e.ProjectRoot)
}

// ExitCode implements the interface cmd/spex/main.go inspects to set the
// process exit status.
func (e *UninitializedError) ExitCode() int { return ExitNotAProject }

// BrokenError is Resolve's answer when .spex/ exists but its state is
// missing or unparseable: the project is broken, and spex doctor is the
// fix — never spex init, which would destroy a journal that might still
// be salvageable.
type BrokenError struct {
	ProjectRoot string
	Err         error
}

func (e *BrokenError) Error() string {
	return fmt.Sprintf("lifecycle: project state at %s is broken: %v; run 'spex doctor'",
		filepath.Join(e.ProjectRoot, StateDirName), e.Err)
}

func (e *BrokenError) Unwrap() error { return e.Err }

// ExitCode implements the interface cmd/spex/main.go inspects to set the
// process exit status. It shares ExitNotAProject with UninitializedError:
// both refusals mean the pipeline cannot proceed, and the message is what
// distinguishes them.
func (e *BrokenError) ExitCode() int { return ExitNotAProject }

// Resolve is the single pre-flight every subcommand that touches project
// state calls before reading or writing it. Given the project root — the
// directory .spex/ lives under, a sibling of spec/ — it returns the
// resolved snapshot and journal locations, or a typed error distinguishing
// "never initialised" (no .spex/) from "broken" (.spex/ present, but the
// snapshot or journal is missing or unparseable).
//
// Resolution is read-only: Resolve never creates or repairs .spex/ or
// anything inside it. There is no fallback to any other layout — .spex/
// is the only place Resolve ever looks.
func Resolve(projectRoot string) (*ProjectContext, error) {
	stateDir := filepath.Join(projectRoot, StateDirName)

	info, err := os.Stat(stateDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &UninitializedError{ProjectRoot: projectRoot}
		}
		return nil, &BrokenError{ProjectRoot: projectRoot, Err: err}
	}
	if !info.IsDir() {
		return nil, &BrokenError{ProjectRoot: projectRoot, Err: fmt.Errorf("%s is not a directory", stateDir)}
	}

	snapshotPath := filepath.Join(stateDir, SnapshotFileName)
	journalPath := filepath.Join(stateDir, JournalFileName)

	if _, err := merkle.Load(snapshotPath); err != nil {
		return nil, &BrokenError{ProjectRoot: projectRoot, Err: fmt.Errorf("snapshot: %w", err)}
	}

	if err := checkJournalReadable(journalPath); err != nil {
		return nil, &BrokenError{ProjectRoot: projectRoot, Err: fmt.Errorf("journal: %w", err)}
	}

	return &ProjectContext{SnapshotPath: snapshotPath, JournalPath: journalPath}, nil
}

// checkJournalReadable reports whether the journal at path exists and
// parses cleanly. Unlike mapping.MappingStore.Parse, a missing file is
// itself a failure here: once .spex/ exists, its absence is damage, not
// the "never ingested" state that Parse tolerates for callers outside the
// lifecycle pre-flight.
func checkJournalReadable(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	_, err := mapping.NewMappingStore(path).Parse()
	return err
}
