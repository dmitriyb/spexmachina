package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/lifecycle"
	"github.com/dmitriyb/spexmachina/mapping"
	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// doctorFinding is one artifact's health, per spec/lifecycle/arch_doctor_command.md
// "Behaviour": what is present, what is missing, what is unreadable, and —
// only when a command exists that would fix it — the fix. Damage inside an
// existing .spex/ carries no Fix: re-initialising is how a journal dies, so
// doctor names no command for it and leaves the decision to a human.
type doctorFinding struct {
	Artifact string `json:"artifact"`
	Status   string `json:"status"` // present, missing, unreadable
	Detail   string `json:"detail,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// doctorReport is spex doctor's full JSON output: every finding in one
// pass, plus the overall verdict the exit code also reflects.
type doctorReport struct {
	Healthy  bool            `json:"healthy"`
	Findings []doctorFinding `json:"findings"`
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose project state health",
		Args:  cobra.NoArgs,
		RunE:  runDoctorE,
	}
}

// runDoctorE is DoctorCommand: it builds on the lifecycle pre-flight for
// the top-level classification (never initialised vs. broken vs. healthy),
// then — unlike the pre-flight, which stops at the first refusal — goes
// deeper for a broken project, examining the snapshot and the journal
// individually so every finding is reported in one pass. It never writes:
// after any run, the project directory is byte-identical to before.
func runDoctorE(cmd *cobra.Command, args []string) error {
	specDir, err := resolveSpecDir(cmd)
	if err != nil {
		return err
	}
	root := resolveProjectRoot(specDir)

	report, resultErr := diagnose(root)

	isTTY := term.IsTerminal(int(os.Stdout.Fd()))
	enc := json.NewEncoder(os.Stdout)
	if isTTY {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(&report); err != nil {
		return fmt.Errorf("doctor: encode report: %w", err)
	}

	return resultErr
}

// diagnose runs the full health check and returns the report to serialize
// alongside the error the caller should propagate (nil when healthy).
func diagnose(root string) (doctorReport, error) {
	_, resolveErr := lifecycle.Resolve(root)

	var uninit *lifecycle.UninitializedError
	switch {
	case resolveErr == nil:
		return doctorReport{
			Healthy: true,
			Findings: []doctorFinding{
				{Artifact: lifecycle.StateDirName, Status: "present"},
				{Artifact: snapshotArtifact(), Status: "present"},
				{Artifact: journalArtifact(), Status: "present"},
			},
		}, nil

	case errors.As(resolveErr, &uninit):
		// Never initialised: the state directory itself is the one and
		// only finding. A project that was never initialised points at
		// spex init — the only case in which doctor names a fix.
		return doctorReport{
			Healthy: false,
			Findings: []doctorFinding{
				{Artifact: lifecycle.StateDirName, Status: "missing", Fix: "spex init"},
			},
		}, resolveErr

	default:
		// Broken: .spex/ exists but something under it is missing or
		// unreadable. The pre-flight would stop at the first offender;
		// doctor examines both artifacts so every finding surfaces.
		findings := []doctorFinding{diagnoseSnapshot(root), diagnoseJournal(root)}
		healthy := true
		for _, f := range findings {
			if f.Status != "present" {
				healthy = false
			}
		}
		var err error
		if !healthy {
			err = fmt.Errorf("doctor: project state is unhealthy")
		}
		return doctorReport{Healthy: healthy, Findings: findings}, err
	}
}

func snapshotArtifact() string {
	return filepath.Join(lifecycle.StateDirName, lifecycle.SnapshotFileName)
}

func journalArtifact() string {
	return filepath.Join(lifecycle.StateDirName, lifecycle.JournalFileName)
}

// diagnoseSnapshot reports the snapshot's health: missing, unreadable (with
// the parse failure as detail), or present. No fix is named — repairing a
// snapshot inside an existing .spex/ is a human decision, never doctor's or
// init's.
func diagnoseSnapshot(root string) doctorFinding {
	path := filepath.Join(root, lifecycle.StateDirName, lifecycle.SnapshotFileName)
	artifact := snapshotArtifact()

	if _, err := merkle.Load(path); err != nil {
		if errors.Is(err, merkle.ErrSnapshotAbsent) {
			return doctorFinding{Artifact: artifact, Status: "missing"}
		}
		return doctorFinding{Artifact: artifact, Status: "unreadable", Detail: err.Error()}
	}
	return doctorFinding{Artifact: artifact, Status: "present"}
}

// diagnoseJournal reports the journal's health. mapping.MappingStore.Parse
// treats a missing file as the first-class "never ingested" state (nil,
// nil) rather than an error, so presence is checked directly first — once
// .spex/ exists, a missing journal is damage, not a bootstrap signal.
func diagnoseJournal(root string) doctorFinding {
	path := filepath.Join(root, lifecycle.StateDirName, lifecycle.JournalFileName)
	artifact := journalArtifact()

	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return doctorFinding{Artifact: artifact, Status: "missing"}
		}
		return doctorFinding{Artifact: artifact, Status: "unreadable", Detail: err.Error()}
	}
	if _, err := mapping.NewMappingStore(path).Parse(); err != nil {
		return doctorFinding{Artifact: artifact, Status: "unreadable", Detail: err.Error()}
	}
	return doctorFinding{Artifact: artifact, Status: "present"}
}
