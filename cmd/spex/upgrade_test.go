package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/cli"
)

// Coverage for spec/cli/test_upgrade_command.md. The update mechanics
// themselves (resolve, verify, replace, rollback) are covered by the
// delivery module's self-update tests (delivery/self_update_upgrade_test.go);
// these cases assert only what the command adds on top: flag translation
// and verdict handling, driven against a stub installer script substituted
// through the installScript test seam.

// captureStubScript writes its full argv (space-joined) and every SPEX_-
// prefixed environment variable to the path named by
// SPEX_UPGRADE_TEST_CAPTURE, then exits 0.
const captureStubScript = "#!/bin/sh\n" +
	"printf 'ARGS:%s\\n' \"$*\" > \"$SPEX_UPGRADE_TEST_CAPTURE\"\n" +
	"env | grep '^SPEX_' >> \"$SPEX_UPGRADE_TEST_CAPTURE\" || true\n" +
	"exit 0\n"

// runUpgrade executes `spex upgrade` with the given args and returns
// stdout, stderr, and the process exit code, mirroring main.go's exit-code
// handling: 0 on success, the error's ExitCode() when it implements that
// interface, or 1 otherwise.
func runUpgrade(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	root := cli.NewRootCmd()
	root.AddCommand(newUpgradeCmd())

	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(append([]string{"upgrade"}, args...))

	if err := root.Execute(); err != nil {
		exitCode = 1
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			exitCode = ec.ExitCode()
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// withStubScript substitutes installScript for body, restoring the
// original (the production embedded script) after the test.
func withStubScript(t *testing.T, body string) {
	t.Helper()
	orig := installScript
	installScript = body
	t.Cleanup(func() { installScript = orig })
}

// withStampedVersion substitutes the compiled-in version stamp, restoring
// the original after the test.
func withStampedVersion(t *testing.T, v string) {
	t.Helper()
	orig := version
	version = v
	t.Cleanup(func() { version = orig })
}

func readCaptureFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capture file: %v", err)
	}
	return string(data)
}

func resolvedSelf(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return self
}

func TestUpgrade_BareInvocationPassesResolvedTargetAndStampedVersion(t *testing.T) {
	withStampedVersion(t, "v1.2.3")
	withStubScript(t, captureStubScript)

	captureFile := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv("SPEX_UPGRADE_TEST_CAPTURE", captureFile)

	_, stderr, exitCode := runUpgrade(t)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", exitCode, stderr)
	}

	captured := readCaptureFile(t, captureFile)
	wantArgs := "ARGS:--upgrade --target " + resolvedSelf(t) + " --current-version v1.2.3"
	if !strings.Contains(captured, wantArgs) {
		t.Errorf("captured = %q, want it to contain %q", captured, wantArgs)
	}
	if strings.Contains(captured, "SPEX_INSTALL_VERSION=") {
		t.Errorf("captured env unexpectedly sets SPEX_INSTALL_VERSION when no --version pin was given:\n%s", captured)
	}
}

func TestUpgrade_VersionPinTravelsAsEnvCheckAndRollbackAsGiven(t *testing.T) {
	withStampedVersion(t, "v1.2.3")
	withStubScript(t, captureStubScript)

	captureFile := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv("SPEX_UPGRADE_TEST_CAPTURE", captureFile)

	_, stderr, exitCode := runUpgrade(t, "--version", "v9.9.9", "--check", "--rollback")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", exitCode, stderr)
	}

	captured := readCaptureFile(t, captureFile)
	if strings.Contains(captured, "--version") {
		t.Errorf("captured args contain a --version flag, want the pin translated to the environment only:\n%s", captured)
	}
	if !strings.Contains(captured, "SPEX_INSTALL_VERSION=v9.9.9") {
		t.Errorf("captured env missing SPEX_INSTALL_VERSION=v9.9.9:\n%s", captured)
	}
	if !strings.Contains(captured, "--check") {
		t.Errorf("captured args missing --check:\n%s", captured)
	}
	if !strings.Contains(captured, "--rollback") {
		t.Errorf("captured args missing --rollback:\n%s", captured)
	}
}

func TestUpgrade_CheckAndDryRunBindSameFieldAnomalyWarningExitsZero(t *testing.T) {
	// Stands in for the script's own anomaly report: it only exits 0 when
	// the command actually forwarded a --check flag, whichever the
	// user spelled it as.
	withStubScript(t, "#!/bin/sh\n"+
		"code=9\n"+
		"for a in \"$@\"; do [ \"$a\" = \"--check\" ] && code=0; done\n"+
		"echo 'install.sh: warning: comparison: anomaly (resolved release is older than installed)' >&2\n"+
		"exit \"$code\"\n")

	for _, flag := range []string{"--check", "--dry-run"} {
		t.Run(flag, func(t *testing.T) {
			_, stderr, exitCode := runUpgrade(t, flag)
			if exitCode != 0 {
				t.Fatalf("exit code = %d, want 0 for %s (an anomaly is reported, not refused)\nstderr:\n%s", exitCode, flag, stderr)
			}
			if !strings.Contains(stderr, "anomaly") {
				t.Errorf("stderr = %q, want it to contain the installer's anomaly warning", stderr)
			}
		})
	}
}

func TestUpgrade_ForwardOnlyRefusalPropagatesAsNonZeroExit(t *testing.T) {
	withStubScript(t, "#!/bin/sh\n"+
		"echo 'install.sh: refusing to install: older than installed (forward-only guard)' >&2\n"+
		"exit 1\n")

	_, stderr, exitCode := runUpgrade(t)
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 (the installer's own exit code)\nstderr:\n%s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "forward-only") {
		t.Errorf("stderr = %q, want it to contain the installer's refusal message", stderr)
	}
}

func TestUpgrade_ForceRejectedAsUnknownFlagNothingInvoked(t *testing.T) {
	withStubScript(t, captureStubScript)

	captureFile := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv("SPEX_UPGRADE_TEST_CAPTURE", captureFile)

	_, _, exitCode := runUpgrade(t, "--force")
	if exitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for an unrecognized --force flag")
	}
	if _, err := os.Stat(captureFile); !os.IsNotExist(err) {
		t.Errorf("installer script ran despite --force being an unknown flag (capture file exists)")
	}
}

func TestUpgrade_DevBuildOmitsCurrentVersionArgument(t *testing.T) {
	withStampedVersion(t, "dev")
	withStubScript(t, captureStubScript)

	captureFile := filepath.Join(t.TempDir(), "capture.txt")
	t.Setenv("SPEX_UPGRADE_TEST_CAPTURE", captureFile)

	_, stderr, exitCode := runUpgrade(t)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", exitCode, stderr)
	}

	captured := readCaptureFile(t, captureFile)
	if strings.Contains(captured, "--current-version") {
		t.Errorf("captured args contain --current-version for a dev build, want it omitted so the script probes the target:\n%s", captured)
	}
}

func TestUpgrade_RegisteredOnRootCommandAndAppearsInHelp(t *testing.T) {
	root := cli.NewRootCmd()
	root.AddCommand(newUpgradeCmd())

	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "upgrade") {
		t.Errorf("root help missing 'upgrade':\n%s", out.String())
	}
}
