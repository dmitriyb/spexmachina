package delivery

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Acceptance coverage for spec/delivery/test_self_update.md, driven end
// to end against the fake-origin harness in self_update_harness_test.go.
// Test-section scope: only the cases test_self_update.md lists.

func TestSelfUpdate_GoldenUpgrade(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v2.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v2.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}

	out, err := exec.Command(target, "version").Output()
	if err != nil {
		t.Fatalf("run upgraded target: %v", err)
	}
	if !strings.Contains(string(out), "v2.0.0") {
		t.Errorf("upgraded target reports %q, want it to contain v2.0.0", out)
	}

	backup := target + ".bak"
	out, err = exec.Command(backup, "version").Output()
	if err != nil {
		t.Fatalf("run backup binary: %v", err)
	}
	if !strings.Contains(string(out), "v1.0.0") {
		t.Errorf("backup reports %q, want it to contain the previous v1.0.0", out)
	}

	assertExactEntries(t, targetDir, "spex", "spex.bak")
}

func TestSelfUpdate_RollbackRestoresBackup(t *testing.T) {
	_, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	backup := target + ".bak"
	writeFakeBinary(t, target, "v2.0.0")
	writeFakeBinary(t, backup, "v1.0.0")

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--rollback"}, nil)
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}

	out, err := exec.Command(target, "version").Output()
	if err != nil {
		t.Fatalf("run rolled-back target: %v", err)
	}
	if !strings.Contains(string(out), "v1.0.0") {
		t.Errorf("rolled-back target reports %q, want the restored v1.0.0", out)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Errorf("backup file still exists after rollback (expected an atomic rename to consume it): stat err=%v", err)
	}
	assertExactEntries(t, targetDir, "spex")
}

func TestSelfUpdate_TamperedArchiveFailsClosed(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	archivePath := fabricateSignedArchive(t, assetsDir, "v2.0.0", goos, goarch, privKey)
	tamperArchive(t, archivePath)
	origin := newFakeOrigin(t, "v2.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for a tampered archive\nstdout:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stderr, "signature") {
		t.Errorf("stderr = %q, want it to mention signature verification failure", res.Stderr)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after failure: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("target was modified despite a failed signature verification")
	}
	assertExactEntries(t, targetDir, "spex")
}

func TestSelfUpdate_DryRunChangesNothing(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v2.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v2.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0", "--check"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 for --check even though a newer release exists\nstderr:\n%s", res.ExitCode, res.Stderr)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after --check: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("--check modified the target")
	}
	assertExactEntries(t, targetDir, "spex")
}

func TestSelfUpdate_AlreadyUpToDateChangesNothing(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v1.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v1.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 when already up to date\nstderr:\n%s", res.ExitCode, res.Stderr)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after already-up-to-date run: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("an already-up-to-date run modified the target")
	}
	assertExactEntries(t, targetDir, "spex")
}

func TestSelfUpdate_ForwardOnlyGuardRefusesOlderLatest(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v1.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v1.0.0", assetsDir) // "latest" is older than installed

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v2.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v2.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero when the resolved latest is older than installed")
	}
	if !strings.Contains(res.Stderr, "forward-only") {
		t.Errorf("stderr = %q, want a forward-only refusal message", res.Stderr)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after refusal: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("target was modified despite the forward-only guard")
	}
	assertExactEntries(t, targetDir, "spex")
	if got := atomic.LoadInt32(&origin.Requests); got != 1 {
		t.Errorf("origin saw %d requests, want exactly 1 (resolve-latest only, no download attempt)", got)
	}
}

func TestSelfUpdate_ForceIsRejectedAsUnknownOption(t *testing.T) {
	_, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	// A closed port: if arg parsing somehow let --force through, the
	// script would fail loudly at the network stage rather than by luck
	// contacting a real origin.
	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0", "--force"}, []string{
		"SPEX_INSTALL_API_ORIGIN=http://127.0.0.1:1",
		"SPEX_INSTALL_RELEASE_ORIGIN=http://127.0.0.1:1",
	})
	if res.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for an unrecognized --force flag")
	}
	if !strings.Contains(res.Stderr, "unknown option") {
		t.Errorf("stderr = %q, want it to report --force as an unknown option", res.Stderr)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after rejection: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("target was modified despite --force being rejected")
	}
}

func TestSelfUpdate_PinnedOlderVersionInstallsAnyDirection(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v1.0.0", goos, goarch, privKey)
	// The "latest" tag is irrelevant once a version is pinned; it is
	// never even resolved.
	origin := newFakeOrigin(t, "v9.9.9", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v2.0.0")

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v2.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_VERSION=v1.0.0",
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 for an explicit older pin\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "explicitly requested") {
		t.Errorf("stdout = %q, want the as-explicitly-requested notice", res.Stdout)
	}

	out, err := exec.Command(target, "version").Output()
	if err != nil {
		t.Fatalf("run pinned target: %v", err)
	}
	if !strings.Contains(string(out), "v1.0.0") {
		t.Errorf("target reports %q, want it installed at the pinned v1.0.0", out)
	}
}

func TestSelfUpdate_DevCurrentVersionWarnsAndProceeds(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v1.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v1.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "dev")

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "dev"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 for a dev current version\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "cannot be ordered") {
		t.Errorf("stderr = %q, want a cannot-be-ordered warning", res.Stderr)
	}

	out, err := exec.Command(target, "version").Output()
	if err != nil {
		t.Fatalf("run upgraded target: %v", err)
	}
	if !strings.Contains(string(out), "v1.0.0") {
		t.Errorf("target reports %q, want it upgraded to v1.0.0", out)
	}
}

func TestSelfUpdate_UnwritableTargetDirStopsAndReportsElevation(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v2.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v2.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	if err := os.Chmod(targetDir, 0o555); err != nil {
		t.Fatalf("chmod target dir read-only: %v", err)
	}
	t.Cleanup(func() { os.Chmod(targetDir, 0o755) })

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
	})
	if res.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for an unwritable target directory")
	}
	if !strings.Contains(res.Stderr, "re-run elevated") {
		t.Errorf("stderr = %q, want the re-run-elevated message", res.Stderr)
	}
	// Upgrade mode must never self-invoke sudo (unlike first-install
	// mode): a self-invoked sudo would either prompt (hanging the test)
	// or, with no real sudo binary in this sandbox, fail with a
	// "command not found" error instead of the re-run-elevated message
	// asserted above — so reaching that message at all is itself
	// evidence sudo was never attempted.

	if err := os.Chmod(targetDir, 0o755); err != nil {
		t.Fatalf("restore target dir permissions: %v", err)
	}
	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after failure: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("target was modified despite an unwritable target directory")
	}
}

// TestSelfUpdate_FailedCopyDuringSwapLeavesTargetUntouched drives
// replace_target's staging cp through a stub that writes a truncated file
// and fails, the way a real cp can die part-way through (e.g. ENOSPC). The
// swap must not proceed on that partial file: the target stays exactly as
// it was, no backup is created, and no staged residue is left behind.
func TestSelfUpdate_FailedCopyDuringSwapLeavesTargetUntouched(t *testing.T) {
	goos, goarch := hostPlatform(t)
	privKey, pubKey := generateThrowawayKey(t)
	script := scriptCopyWithKey(t, pubKey)

	assetsDir := t.TempDir()
	fabricateSignedArchive(t, assetsDir, "v2.0.0", goos, goarch, privKey)
	origin := newFakeOrigin(t, "v2.0.0", assetsDir)

	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "spex")
	writeFakeBinary(t, target, "v1.0.0")
	wantBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target fixture: %v", err)
	}

	failingCPDir := t.TempDir()
	failingCP := filepath.Join(failingCPDir, "cp")
	// $2 is the destination cp was asked to write; write a truncated
	// payload there and exit non-zero, mirroring a cp that dies mid-copy.
	stub := "#!/bin/sh\nprintf 'NEW' > \"$2\"\nexit 1\n"
	if err := os.WriteFile(failingCP, []byte(stub), 0o755); err != nil {
		t.Fatalf("write failing cp stub: %v", err)
	}

	res := runScript(t, script, []string{"--upgrade", "--target", target, "--current-version", "v1.0.0"}, []string{
		"SPEX_INSTALL_API_ORIGIN=" + origin.URL,
		"SPEX_INSTALL_RELEASE_ORIGIN=" + origin.URL,
		"PATH=" + failingCPDir + ":" + buildShimBinDir(t) + ":" + os.Getenv("PATH"),
	})
	if res.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero when the staging cp fails\nstdout:\n%s", res.Stdout)
	}

	gotBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after failure: %v", err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Error("target was modified despite a failed staging cp")
	}
	assertExactEntries(t, targetDir, "spex")
}
