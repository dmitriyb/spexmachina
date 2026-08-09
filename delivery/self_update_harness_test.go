package delivery

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// This file holds the fake-origin harness shared by the self-update
// integration tests (self_update_upgrade_test.go): a modified copy of the
// embedded install script with only the trust anchor swapped for a
// throwaway key, a local HTTP server shaped like the production release
// endpoints, and helpers to fabricate a signed fake release. Per
// spec/delivery/test_self_update.md: "The harness runs the real script (a
// copy with only the signing public key swapped for a throwaway test key,
// origins redirected through the script's test-only origin variables)
// against a local server shaped like the release endpoints."
//
// The sandbox this suite runs in has neither curl/wget nor tar, tools the
// real script (and any real machine it runs on) assumes. runScript builds
// tiny Go stand-ins from testdata/shim and prepends them to PATH so the
// script's own, unmodified curl/tar invocations resolve to something that
// works here; this is purely a test fixture and never ships.

const repoSlug = "dmitriyb/spexmachina"

var pubkeyLineRE = regexp.MustCompile(`SPEX_RELEASE_PUBKEY="[^"]*"`)

// scriptCopyWithKey writes a copy of the embedded install script with its
// baked-in trust anchor replaced by pubKeyValue ("<type> <base64>", no
// comment) and returns its path. The origin variables are left alone —
// those are overridden per-invocation via environment, exactly as
// arch_self_update.md's test-only origin hooks describe.
func scriptCopyWithKey(t *testing.T, pubKeyValue string) string {
	t.Helper()
	replaced := pubkeyLineRE.ReplaceAllString(InstallScript, `SPEX_RELEASE_PUBKEY="`+pubKeyValue+`"`)
	if replaced == InstallScript {
		t.Fatal("SPEX_RELEASE_PUBKEY line not found in embedded install script; harness key swap would be a no-op")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(path, []byte(replaced), 0o755); err != nil {
		t.Fatalf("write script copy: %v", err)
	}
	return path
}

// generateThrowawayKey creates a fresh Ed25519 SSH keypair for the
// harness's own use, distinct from the production key baked into
// install.sh. Returns the private key path (for signing fabricated
// releases) and the public key's "<type> <base64>" value (for swapping
// into a script copy via scriptCopyWithKey).
func generateThrowawayKey(t *testing.T) (privPath, pubKeyValue string) {
	t.Helper()
	dir := t.TempDir()
	priv := filepath.Join(dir, "throwaway")
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "spex-release-test", "-f", priv)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate throwaway key: %v\n%s", err, out)
	}
	pubBytes, err := os.ReadFile(priv + ".pub")
	if err != nil {
		t.Fatalf("read throwaway pubkey: %v", err)
	}
	fields := strings.Fields(string(pubBytes))
	if len(fields) < 2 {
		t.Fatalf("unexpected ssh-keygen pubkey format: %q", pubBytes)
	}
	return priv, fields[0] + " " + fields[1]
}

// fakeBinaryScript is the content installed at a fake release's "spex"
// binary path and at any pre-existing target/backup fixture: a shell
// script whose `version` subcommand output matches the shape
// probe_version and cmd/spex's real `spex version` share ("spex
// <version>" as the first line's first two fields).
func fakeBinaryScript(version string) []byte {
	return []byte("#!/bin/sh\necho \"spex " + version + "\"\n")
}

// writeFakeBinary writes fakeBinaryScript(version) to path, executable.
func writeFakeBinary(t *testing.T, path, version string) {
	t.Helper()
	if err := os.WriteFile(path, fakeBinaryScript(version), 0o755); err != nil {
		t.Fatalf("write fake binary %s: %v", path, err)
	}
}

// archiveName mirrors install.sh's own naming: <bin>_<version-no-v>_<goos>_<goarch>.tar.gz
func archiveName(version, goos, goarch string) string {
	return fmt.Sprintf("spex_%s_%s_%s.tar.gz", strings.TrimPrefix(version, "v"), goos, goarch)
}

// fabricateSignedArchive builds a tar.gz containing a single executable
// file "spex" with fakeBinaryScript(version) as its content, signs it
// with privKeyPath via `ssh-keygen -Y sign -n file`, and returns the
// archive path (the signature lands alongside it as archive+".sig",
// exactly where install.sh looks for it).
func fabricateSignedArchive(t *testing.T, dir, version, goos, goarch, privKeyPath string) string {
	t.Helper()
	path := filepath.Join(dir, archiveName(version, goos, goarch))
	writeTarGz(t, path, "spex", fakeBinaryScript(version))
	signArchive(t, path, privKeyPath)
	return path
}

func writeTarGz(t *testing.T, path, entryName string, contents []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive %s: %v", path, err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: entryName, Mode: 0o755, Size: int64(len(contents))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(contents); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
}

func signArchive(t *testing.T, archivePath, privKeyPath string) {
	t.Helper()
	cmd := exec.Command("ssh-keygen", "-Y", "sign", "-n", "file", "-f", privKeyPath, archivePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sign archive %s: %v\n%s", archivePath, err, out)
	}
}

// tamperArchive flips a byte in the archive after signing, so its
// signature (computed over the pre-tamper bytes) no longer verifies —
// the harness's stand-in for "an attacker altered the archive after it
// was signed."
func tamperArchive(t *testing.T, archivePath string) {
	t.Helper()
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive to tamper: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("archive is empty, cannot tamper")
	}
	data[0] ^= 0xFF
	if err := os.WriteFile(archivePath, data, 0o644); err != nil {
		t.Fatalf("write tampered archive: %v", err)
	}
}

// fakeOrigin is a local HTTP server shaped like the two production
// routes install.sh talks to: the GitHub "latest release" API and a
// release's download URLs. Both SPEX_INSTALL_API_ORIGIN and
// SPEX_INSTALL_RELEASE_ORIGIN point at it in tests; the routes don't
// overlap so one server serves both roles.
type fakeOrigin struct {
	URL      string
	Requests int32 // atomic
}

// newFakeOrigin serves latestTag as the resolved "latest" release and
// serves any file under assetsDir at
// /<repoSlug>/releases/download/<tag>/<filename> regardless of which tag
// is named in the path — tests fabricate exactly the assets a given case
// needs and don't otherwise distinguish tags by directory.
func newFakeOrigin(t *testing.T, latestTag, assetsDir string) *fakeOrigin {
	t.Helper()
	fo := &fakeOrigin{}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/"+repoSlug+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fo.Requests, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"tag_name": %q}`, latestTag)
	})
	downloadPrefix := "/" + repoSlug + "/releases/download/"
	mux.HandleFunc(downloadPrefix, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fo.Requests, 1)
		rest := strings.TrimPrefix(r.URL.Path, downloadPrefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 || parts[1] == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(assetsDir, filepath.Base(parts[1])))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	fo.URL = srv.URL
	return fo
}

var (
	shimOnce sync.Once
	shimDir  string
	shimErr  error
)

// buildShimBinDir builds (once per test binary run) the curl/tar stand-ins
// testdata/shim/{curl,tar} provide and returns the directory containing
// them, meant to be prepended to a script subprocess's PATH.
func buildShimBinDir(t *testing.T) string {
	t.Helper()
	shimOnce.Do(func() {
		dir, err := os.MkdirTemp("", "spex-selfupdate-shim-")
		if err != nil {
			shimErr = err
			return
		}
		for name, pkg := range map[string]string{
			"curl": "github.com/dmitriyb/spexmachina/delivery/testdata/shim/curl",
			"tar":  "github.com/dmitriyb/spexmachina/delivery/testdata/shim/tar",
		} {
			out := filepath.Join(dir, name)
			cmd := exec.Command("go", "build", "-o", out, pkg)
			if output, err := cmd.CombinedOutput(); err != nil {
				shimErr = fmt.Errorf("build %s shim: %w\n%s", name, err, output)
				return
			}
		}
		shimDir = dir
	})
	if shimErr != nil {
		t.Fatalf("build test shims: %v", shimErr)
	}
	return shimDir
}

// scriptResult is what runScript reports about one script invocation.
type scriptResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runScript runs scriptPath under bash with args and env (in addition to
// a minimal PATH/HOME base and the shim bin dir), capturing stdout,
// stderr, and the exit code separately.
func runScript(t *testing.T, scriptPath string, args []string, env []string) scriptResult {
	t.Helper()
	shim := buildShimBinDir(t)

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	baseEnv := []string{
		"PATH=" + shim + ":" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + os.Getenv("TMPDIR"),
	}
	cmd.Env = append(baseEnv, env...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run script: %v", err)
		}
	}
	return scriptResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

// hostPlatform mirrors install.sh's own uname-based detection so tests
// fabricate archives under the name the script will actually request.
func hostPlatform(t *testing.T) (goos, goarch string) {
	t.Helper()
	out, err := exec.Command("uname", "-s").Output()
	if err != nil {
		t.Fatalf("uname -s: %v", err)
	}
	switch strings.TrimSpace(string(out)) {
	case "Linux":
		goos = "linux"
	case "Darwin":
		goos = "darwin"
	default:
		t.Fatalf("unsupported test host OS: %s", out)
	}
	out, err = exec.Command("uname", "-m").Output()
	if err != nil {
		t.Fatalf("uname -m: %v", err)
	}
	switch strings.TrimSpace(string(out)) {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "arm64", "aarch64":
		goarch = "arm64"
	default:
		t.Fatalf("unsupported test host arch: %s", out)
	}
	return goos, goarch
}

// dirEntries lists the base names of files directly under dir, for
// asserting a directory holds exactly the files a test expects (e.g. "no
// staged residue left in the target directory").
func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// assertExactEntries fails the test unless dir's entries are exactly
// want, order ignored.
func assertExactEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := dirEntries(t, dir)
	sort.Strings(got)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if strings.Join(got, ",") != strings.Join(wantSorted, ",") {
		t.Errorf("dir %s entries = %v, want exactly %v", dir, got, wantSorted)
	}
}
