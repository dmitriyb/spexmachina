// Acceptance-level coverage for spec/delivery/test_release.md's "Cases —
// artifacts" and its edge case, exercising the real mechanisms
// .goreleaser.yaml and .github/workflows/release.yml wire up — ldflags
// stamping, byte-identical builds, SSHSIG signing/verification, and
// per-artifact checksums — with `go build`, `ssh-keygen`, and `sha256sum`,
// the same commands GoReleaser and the workflow themselves shell out to.
//
// test_release.md allows a local GoReleaser snapshot build as the
// alternative to exercising against a real pre-release tag; neither is
// available here (no `goreleaser` binary, no live repository to tag), so
// this harness goes one level lower, at the underlying commands, rather
// than fabricating a result. Commands and templates are extracted from the
// actual config/workflow text (readRepoFile, extractYAMLList — both in
// release_test.go) so a drifted file breaks these tests instead of a stale
// hardcoded copy silently passing.
//
// "Cases — manifest" are covered by delivery/manifest_test.go, added with
// the ReleaseManifest component itself — not duplicated here.
//
// Non-Responsibilities (need the goreleaser binary or a live tag/repository,
// named or implied by test_release.md itself):
//   - "The tag triggers exactly one release" with four archives, as an
//     end-to-end fact — the build matrix and archive naming template are
//     already pinned structurally by TestReleaseConfig_BuildMatrixAndStamping.
//   - The SLSA build-provenance attestation actually verifying — that's
//     GitHub's Sigstore-backed actions/attest-build-provenance step, which
//     cannot execute outside a real Actions run; its presence in the
//     workflow is already pinned by TestReleaseWorkflow_TagTriggered.
package delivery

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return dir
}

// buildSpex cross-compiles cmd/spex the same way .goreleaser.yaml's build
// stanza does: CGO disabled, paths trimmed, version metadata injected only
// through ldflags.
func buildSpex(t *testing.T, goos, goarch string, ldflags []string, outPath string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", strings.Join(ldflags, " "), "-o", outPath, "./cmd/spex")
	cmd.Dir = repoRootDir(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+goarch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build GOOS=%s GOARCH=%s: %v\n%s", goos, goarch, err, out)
	}
}

var ldflagsXVarRe = regexp.MustCompile(`-X (main\.\w+)=`)

// ldflagsVarNames returns the deduplicated main.<var> names .goreleaser.yaml
// injects via -X, in first-seen order.
func ldflagsVarNames(t *testing.T, cfg string) []string {
	t.Helper()
	matches := ldflagsXVarRe.FindAllStringSubmatch(cfg, -1)
	if len(matches) == 0 {
		t.Fatal(".goreleaser.yaml: no -X main.<var>= ldflags found")
	}
	seen := map[string]bool{}
	var names []string
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// TestReleaseBuild_LdflagsStampVersionCommitDate exercises test_release.md's
// "Each archive's binary reports the tag's version, the release commit, and
// the build date via `spex version` — the ldflags stamp, not the `dev`
// defaults." It builds the real binary with the exact -X variable names
// .goreleaser.yaml declares and runs it, rather than injecting the package
// vars in-process (cmd/spex/version_test.go already covers that).
func TestReleaseBuild_LdflagsStampVersionCommitDate(t *testing.T) {
	cfg := readRepoFile(t, ".goreleaser.yaml")
	varNames := ldflagsVarNames(t, cfg)
	want := []string{"main.version", "main.commit", "main.date"}
	if len(varNames) != len(want) {
		t.Fatalf("ldflags -X variable names = %v, want %v", varNames, want)
	}
	for _, w := range want {
		found := false
		for _, v := range varNames {
			if v == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("ldflags -X variable names = %v, missing %q", varNames, w)
		}
	}

	ldflags := []string{
		"-X main.version=v9.9.9-test",
		"-X main.commit=deadbeefcafefeed",
		"-X main.date=2026-08-09T00:00:00Z",
	}
	outPath := filepath.Join(t.TempDir(), "spex")
	buildSpex(t, runtime.GOOS, runtime.GOARCH, ldflags, outPath)

	out, err := exec.Command(outPath, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("run ldflags-stamped binary: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"v9.9.9-test", "deadbeefcafefeed", "2026-08-09T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("stamped binary output missing %q:\n%s", want, got)
		}
	}
	for _, notWant := range []string{"spex dev", "unknown"} {
		if strings.Contains(got, notWant) {
			t.Errorf("stamped binary output still carries a dev default (%q):\n%s", notWant, got)
		}
	}
}

// TestReleaseBuild_ByteIdenticalAcrossRepeatBuilds exercises
// test_release.md's "Building the same tag twice yields byte-identical
// binaries (CGO off, trimmed paths, no timestamp leakage outside the
// stamped date)" across the full build matrix .goreleaser.yaml declares.
func TestReleaseBuild_ByteIdenticalAcrossRepeatBuilds(t *testing.T) {
	cfg := readRepoFile(t, ".goreleaser.yaml")
	goos := extractYAMLList(t, cfg, "goos")
	goarch := extractYAMLList(t, cfg, "goarch")

	ldflags := []string{
		"-X main.version=v9.9.9-test",
		"-X main.commit=deadbeefcafefeed",
		"-X main.date=2026-08-09T00:00:00Z",
	}

	for _, goosVal := range goos {
		for _, goarchVal := range goarch {
			t.Run(goosVal+"_"+goarchVal, func(t *testing.T) {
				dir := t.TempDir()
				first := filepath.Join(dir, "spex-1")
				second := filepath.Join(dir, "spex-2")
				buildSpex(t, goosVal, goarchVal, ldflags, first)
				buildSpex(t, goosVal, goarchVal, ldflags, second)

				a, err := os.ReadFile(first)
				if err != nil {
					t.Fatalf("read first build: %v", err)
				}
				b, err := os.ReadFile(second)
				if err != nil {
					t.Fatalf("read second build: %v", err)
				}
				if !bytes.Equal(a, b) {
					t.Fatalf("two builds of %s/%s with identical ldflags/trimpath/CGO_ENABLED=0 differ (%d vs %d bytes)", goosVal, goarchVal, len(a), len(b))
				}
			})
		}
	}
}

// TestReleaseSigning_SSHSIGRoundTripAndTamperDetection exercises
// test_release.md's "Each archive verifies against the release-signing
// public key with the SSHSIG file-namespace check; a flipped byte in the
// archive makes the same check fail" — against the literal command
// .goreleaser.yaml's signs block shells out to.
func TestReleaseSigning_SSHSIGRoundTripAndTamperDetection(t *testing.T) {
	cfg := readRepoFile(t, ".goreleaser.yaml")
	for _, want := range []string{"-Y", "sign", "-n", "file"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf(".goreleaser.yaml signs block missing %q — sign command shape changed, update this test", want)
		}
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	genCmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-C", "release-test", "-f", keyPath)
	if out, err := genCmd.CombinedOutput(); err != nil {
		t.Fatalf("generate signing key: %v\n%s", err, out)
	}

	archive := filepath.Join(dir, "spex_1.2.3_linux_amd64.tar.gz")
	if err := os.WriteFile(archive, []byte("archive contents"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	signCmd := exec.Command("ssh-keygen", "-Y", "sign", "-n", "file", "-f", keyPath, archive)
	if out, err := signCmd.CombinedOutput(); err != nil {
		t.Fatalf("sign archive: %v\n%s", err, out)
	}
	sigPath := archive + ".sig"
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("signature file not produced: %v", err)
	}

	pub, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}
	allowedSigners := filepath.Join(dir, "allowed_signers")
	if err := os.WriteFile(allowedSigners, []byte("release@spex "+string(pub)), 0o644); err != nil {
		t.Fatalf("write allowed_signers: %v", err)
	}

	verify := func(archivePath string) error {
		data, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("read archive for verify: %v", err)
		}
		cmd := exec.Command("ssh-keygen", "-Y", "verify", "-n", "file", "-f", allowedSigners, "-I", "release@spex", "-s", sigPath)
		cmd.Stdin = bytes.NewReader(data)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errWithOutput(err, out)
		}
		return nil
	}

	if err := verify(archive); err != nil {
		t.Fatalf("archive fails the SSHSIG file-namespace check against its own signature: %v", err)
	}

	tampered := append([]byte(nil), []byte("archive contents")...)
	tampered[0] ^= 0xFF
	tamperedPath := filepath.Join(dir, "tampered.tar.gz")
	if err := os.WriteFile(tamperedPath, tampered, 0o644); err != nil {
		t.Fatalf("write tampered archive: %v", err)
	}
	if err := verify(tamperedPath); err == nil {
		t.Fatal("a flipped byte in the archive still passed the SSHSIG check against the original signature")
	}
}

// TestReleaseSigning_FailedSigningAbortsRatherThanPublish exercises
// test_release.md's edge case: "A failed signing step aborts the release
// rather than publishing unsigned archives." A missing signing key is what
// a broken RELEASE_SSH_SIGNING_KEY_PATH looks like in CI: the exact command
// .goreleaser.yaml's signs block shells out to must fail loudly and never
// produce a signature, which is what makes GoReleaser abort the pipeline
// before the publish step runs.
func TestReleaseSigning_FailedSigningAbortsRatherThanPublish(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "spex_1.2.3_linux_amd64.tar.gz")
	if err := os.WriteFile(archive, []byte("archive contents"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	cmd := exec.Command("ssh-keygen", "-Y", "sign", "-n", "file", "-f", filepath.Join(dir, "missing-key"), archive)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("signing with a missing key exited cleanly, want a failure: %s", out)
	}
	if _, statErr := os.Stat(archive + ".sig"); statErr == nil {
		t.Fatal("a signature file was produced despite the signing command failing")
	}
}

// extractNamedRunBlock returns the dedented body of the `run: |` block scalar
// belonging to the named GitHub Actions step in wf, without retyping it.
func extractNamedRunBlock(t *testing.T, wf, stepName string) string {
	t.Helper()
	lines := strings.Split(wf, "\n")
	runHeaderRe := regexp.MustCompile(`^(\s*)run:\s*\|\s*$`)

	i := 0
	for ; i < len(lines); i++ {
		if strings.Contains(lines[i], "name: "+stepName) {
			break
		}
	}
	if i == len(lines) {
		t.Fatalf("step %q not found in workflow", stepName)
	}
	for ; i < len(lines); i++ {
		m := runHeaderRe.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent := len(m[1])
		var block []string
		for j := i + 1; j < len(lines); j++ {
			line := lines[j]
			if strings.TrimSpace(line) == "" {
				block = append(block, "")
				continue
			}
			curIndent := len(line) - len(strings.TrimLeft(line, " "))
			if curIndent <= indent {
				break
			}
			block = append(block, line[indent+2:])
		}
		return strings.Join(block, "\n")
	}
	t.Fatalf("step %q has no run: | block", stepName)
	return ""
}

// TestReleaseChecksums_PerArtifactMatchesRecomputedSHA256 exercises
// test_release.md's "each archive also carries its own per-artifact
// checksum file" against the literal script
// .github/workflows/release.yml's "Generate per-artifact checksums" step
// runs, extracted rather than retyped.
func TestReleaseChecksums_PerArtifactMatchesRecomputedSHA256(t *testing.T) {
	wf := readRepoFile(t, ".github/workflows/release.yml")
	script := extractNamedRunBlock(t, wf, "Generate per-artifact checksums")
	if !strings.Contains(script, "sha256sum") {
		t.Fatalf("extracted run block does not shell out to sha256sum:\n%s", script)
	}

	dir := t.TempDir()
	distDir := filepath.Join(dir, "dist")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}

	archives := map[string][]byte{
		"spex_1.2.3_linux_amd64.tar.gz":  []byte("linux amd64 archive contents"),
		"spex_1.2.3_darwin_arm64.tar.gz": []byte("darwin arm64 archive contents"),
	}
	wantSHA := map[string]string{}
	for name, contents := range archives {
		if err := os.WriteFile(filepath.Join(distDir, name), contents, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		sum := sha256.Sum256(contents)
		wantSHA[name] = hex.EncodeToString(sum[:])
	}

	cmd := exec.Command("bash", "-c", script)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run extracted checksum script: %v\n%s", err, out)
	}

	for name, want := range wantSHA {
		data, err := os.ReadFile(filepath.Join(distDir, name+".sha256"))
		if err != nil {
			t.Fatalf("read %s.sha256: %v", name, err)
		}
		fields := strings.Fields(string(data))
		if len(fields) == 0 || fields[0] != want {
			t.Errorf("%s.sha256 = %q, want sha256 %s (recomputed independently from the archive bytes)", name, data, want)
		}

		verify := exec.Command("sha256sum", "-c", name+".sha256")
		verify.Dir = distDir
		if out, err := verify.CombinedOutput(); err != nil {
			t.Errorf("sha256sum -c %s.sha256 failed: %v\n%s", name, err, out)
		}
	}
}

func errWithOutput(err error, out []byte) error {
	return &exitErrorWithOutput{err: err, out: out}
}

type exitErrorWithOutput struct {
	err error
	out []byte
}

func (e *exitErrorWithOutput) Error() string {
	return e.err.Error() + ": " + string(e.out)
}
