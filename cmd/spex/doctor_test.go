package main

import (
	"bytes"
	"encoding/json"
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

// runDoctor executes `spex doctor` with the given args and returns stdout,
// stderr and the process exit code, mirroring main.go's exit-code and
// stderr handling exactly as runDiff does for the diff command.
func runDoctor(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	rootCmd := cli.NewRootCmd()
	rootCmd.AddCommand(newDoctorCmd())

	errBuf := new(bytes.Buffer)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(append([]string{"doctor"}, args...))

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

// doctorWireFinding and doctorWireReport decode `spex doctor`'s JSON output
// as any consumer (CI, a script) would see it on the wire — independent of
// the unexported doctorReport/doctorFinding types the command builds.
type doctorWireFinding struct {
	Artifact string `json:"artifact"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Fix      string `json:"fix"`
}

type doctorWireReport struct {
	Healthy  bool                `json:"healthy"`
	Findings []doctorWireFinding `json:"findings"`
}

func decodeDoctorReport(t *testing.T, stdout string) doctorWireReport {
	t.Helper()
	var report doctorWireReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("decode doctor report: %v\nstdout: %s", err, stdout)
	}
	return report
}

func findingFor(t *testing.T, report doctorWireReport, artifact string) doctorWireFinding {
	t.Helper()
	for _, f := range report.Findings {
		if f.Artifact == artifact {
			return f
		}
	}
	t.Fatalf("no finding for artifact %q in %+v", artifact, report.Findings)
	return doctorWireFinding{}
}

// newProjectRoot returns a fresh temporary project root and the --spec-dir
// path doctor is invoked with (a sibling of .spex/, matching how every
// other CLI command resolves the project root).
func newProjectRoot(t *testing.T) (root, specDir string) {
	t.Helper()
	root = t.TempDir()
	return root, filepath.Join(root, "spec")
}

// healthyProject builds a project root whose .spex/ holds a parseable
// empty-tree snapshot and an empty journal — what spex init leaves behind,
// per lifecycle.initialisedProject's own fixture. InitCommand does not yet
// exist as a runnable CLI command in this codebase, so the fixture is built
// directly against the same on-disk contract doctor and the resolver share.
func healthyProject(t *testing.T) (root, specDir string) {
	t.Helper()
	root, specDir = newProjectRoot(t)
	seedProjectState(t, specDir, merkle.EmptyTree(), time.Now().UTC())
	return root, specDir
}

func dirBytes(t *testing.T, root string) map[string][]byte {
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

func assertDirByteIdentical(t *testing.T, root string, before map[string][]byte) {
	t.Helper()
	after := dirBytes(t, root)
	if len(after) != len(before) {
		t.Fatalf("file count changed: before=%d after=%d (before=%v after=%v)", len(before), len(after), before, after)
	}
	for path, want := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("file disappeared: %s", path)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("file %s changed: before=%q after=%q", path, want, got)
		}
	}
}

// TestDoctor_HealthyProject covers "Doctor on a healthy project": every
// artifact is reported present and readable, exit 0.
func TestDoctor_HealthyProject(t *testing.T) {
	_, specDir := healthyProject(t)

	stdout, stderr, exitCode := runDoctor(t, "--spec-dir", specDir)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, stderr)
	}
	report := decodeDoctorReport(t, stdout)
	if !report.Healthy {
		t.Fatalf("report.Healthy = false, want true: %+v", report)
	}
	for _, artifact := range []string{
		lifecycle.StateDirName,
		filepath.Join(lifecycle.StateDirName, lifecycle.SnapshotFileName),
		filepath.Join(lifecycle.StateDirName, lifecycle.JournalFileName),
	} {
		finding := findingFor(t, report, artifact)
		if finding.Status != "present" {
			t.Errorf("finding %s: status = %q, want present", artifact, finding.Status)
		}
	}
}

// TestDoctor_UninitializedProject covers the edge case: `spex doctor` in an
// uninitialised directory reports the project as never initialised, names
// spex init, and exits with the not-a-spex-project code rather than
// crashing on absent files.
func TestDoctor_UninitializedProject(t *testing.T) {
	_, specDir := newProjectRoot(t)

	stdout, stderr, exitCode := runDoctor(t, "--spec-dir", specDir)

	if exitCode != lifecycle.ExitNotAProject {
		t.Fatalf("exit code = %d, want %d (ExitNotAProject); stderr=%s", exitCode, lifecycle.ExitNotAProject, stderr)
	}
	report := decodeDoctorReport(t, stdout)
	if report.Healthy {
		t.Fatalf("report.Healthy = true, want false: %+v", report)
	}
	finding := findingFor(t, report, lifecycle.StateDirName)
	if finding.Status != "missing" {
		t.Errorf("finding status = %q, want missing", finding.Status)
	}
	if finding.Fix != "spex init" {
		t.Errorf("finding fix = %q, want %q", finding.Fix, "spex init")
	}
}

// TestDoctor_NamesTheFixPerFinding covers "Doctor names the fix, per
// finding": on damaged fixtures, doctor lists each missing or unreadable
// artifact together with the command that would fix it — a missing
// .spex/ names spex init (covered separately above); a damaged file
// inside .spex/ names none, because re-initialising is how a journal
// dies.
func TestDoctor_NamesTheFixPerFinding(t *testing.T) {
	t.Run("snapshot deleted", func(t *testing.T) {
		root, specDir := healthyProject(t)
		if err := os.Remove(filepath.Join(root, lifecycle.StateDirName, lifecycle.SnapshotFileName)); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, exitCode := runDoctor(t, "--spec-dir", specDir)

		if exitCode == 0 {
			t.Fatalf("exit code = 0, want non-zero; stdout=%s stderr=%s", stdout, stderr)
		}
		report := decodeDoctorReport(t, stdout)
		if report.Healthy {
			t.Fatalf("report.Healthy = true, want false: %+v", report)
		}
		snap := findingFor(t, report, filepath.Join(lifecycle.StateDirName, lifecycle.SnapshotFileName))
		if snap.Status != "missing" {
			t.Errorf("snapshot status = %q, want missing", snap.Status)
		}
		if snap.Fix != "" {
			t.Errorf("snapshot fix = %q, want empty — damage inside .spex/ names no command", snap.Fix)
		}
		journal := findingFor(t, report, filepath.Join(lifecycle.StateDirName, lifecycle.JournalFileName))
		if journal.Status != "present" {
			t.Errorf("journal status = %q, want present", journal.Status)
		}
	})

	t.Run("journal malformed", func(t *testing.T) {
		root, specDir := healthyProject(t)
		journalPath := filepath.Join(root, lifecycle.StateDirName, lifecycle.JournalFileName)
		if err := os.WriteFile(journalPath, []byte("{not valid json\n"), 0644); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, exitCode := runDoctor(t, "--spec-dir", specDir)

		if exitCode == 0 {
			t.Fatalf("exit code = 0, want non-zero; stdout=%s stderr=%s", stdout, stderr)
		}
		report := decodeDoctorReport(t, stdout)
		if report.Healthy {
			t.Fatalf("report.Healthy = true, want false: %+v", report)
		}
		journal := findingFor(t, report, filepath.Join(lifecycle.StateDirName, lifecycle.JournalFileName))
		if journal.Status != "unreadable" {
			t.Errorf("journal status = %q, want unreadable", journal.Status)
		}
		if journal.Detail == "" {
			t.Errorf("journal detail is empty, want the parse failure")
		}
		if journal.Fix != "" {
			t.Errorf("journal fix = %q, want empty — damage inside .spex/ names no command", journal.Fix)
		}
		snap := findingFor(t, report, filepath.Join(lifecycle.StateDirName, lifecycle.SnapshotFileName))
		if snap.Status != "present" {
			t.Errorf("snapshot status = %q, want present", snap.Status)
		}
	})
}

// TestDoctor_NeverRepairs covers "Doctor never repairs": run spex doctor
// against every damaged fixture; afterwards the directory is
// byte-identical to before. No flag, no mode, no exception mints or moves
// a baseline.
func TestDoctor_NeverRepairs(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, root string)
	}{
		{
			name: "snapshot deleted",
			damage: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, lifecycle.StateDirName, lifecycle.SnapshotFileName)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "journal malformed",
			damage: func(t *testing.T, root string) {
				journalPath := filepath.Join(root, lifecycle.StateDirName, lifecycle.JournalFileName)
				if err := os.WriteFile(journalPath, []byte("{not valid json\n"), 0644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, specDir := healthyProject(t)
			tc.damage(t, root)

			before := dirBytes(t, root)
			if _, _, exitCode := runDoctor(t, "--spec-dir", specDir); exitCode == 0 {
				t.Fatalf("exit code = 0, want non-zero for a damaged fixture")
			}
			assertDirByteIdentical(t, root, before)
		})
	}
}
