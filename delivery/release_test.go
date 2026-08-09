// Package delivery holds build-failing tests that pin the release
// pipeline's non-Go artifacts (.goreleaser.yaml, the release workflow) to
// the contract spec/delivery/arch_release.md states, so a drifted config
// breaks CI instead of silently diverging from the spec.
package delivery

import (
	"os"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestReleaseConfig_BuildMatrixAndStamping(t *testing.T) {
	cfg := readRepoFile(t, ".goreleaser.yaml")

	for _, want := range []string{
		"main: ./cmd/spex",
		"CGO_ENABLED=0",
		"-trimpath",
		"-X main.version={{ .Tag }}",
		"-X main.commit={{ .Commit }}",
		"-X main.date={{ .Date }}",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf(".goreleaser.yaml missing %q", want)
		}
	}

	goos := extractYAMLList(t, cfg, "goos")
	for _, want := range []string{"linux", "darwin"} {
		if !contains(goos, want) {
			t.Errorf("goos list %v missing %q", goos, want)
		}
	}
	if len(goos) != 2 {
		t.Errorf("goos list %v: want exactly linux and darwin", goos)
	}

	goarch := extractYAMLList(t, cfg, "goarch")
	for _, want := range []string{"amd64", "arm64"} {
		if !contains(goarch, want) {
			t.Errorf("goarch list %v missing %q", goarch, want)
		}
	}
	if len(goarch) != 2 {
		t.Errorf("goarch list %v: want exactly amd64 and arm64", goarch)
	}

	if !strings.Contains(cfg, "name_template") {
		t.Fatal(".goreleaser.yaml: archive missing name_template")
	}
	for _, want := range []string{"spex", "{{ .Version }}", "{{ .Os }}", "{{ .Arch }}"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("archive name_template missing %q component", want)
		}
	}
}

func TestReleaseConfig_TrustChain(t *testing.T) {
	cfg := readRepoFile(t, ".goreleaser.yaml")

	if !strings.Contains(cfg, "algorithm: sha256") {
		t.Error(".goreleaser.yaml: checksum algorithm is not sha256")
	}

	if !strings.Contains(cfg, "signs:") {
		t.Fatal(".goreleaser.yaml: no signs block configured")
	}
	for _, want := range []string{"ssh-keygen", "-Y", "sign", "-n", "\"file\"", "artifacts: archive"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("signs block missing %q", want)
		}
	}
}

func TestReleaseWorkflow_TagTriggered(t *testing.T) {
	wf := readRepoFile(t, ".github/workflows/release.yml")

	if !strings.Contains(wf, "tags:") || !strings.Contains(wf, "v*") {
		t.Error("release workflow does not trigger on version tags")
	}
	if strings.Contains(wf, "pull_request") {
		t.Error("release workflow must not trigger on pull_request")
	}

	for _, want := range []string{
		"goreleaser-action",
		"release --clean",
		"attest-build-provenance",
		"RELEASE_SSH_SIGNING_KEY",
	} {
		if !strings.Contains(wf, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
}

func TestReleaseWorkflow_PerArtifactChecksums(t *testing.T) {
	wf := readRepoFile(t, ".github/workflows/release.yml")

	if !strings.Contains(wf, "sha256sum") {
		t.Error("release workflow does not generate per-artifact sha256 checksums")
	}
}

// extractYAMLList reads a simple flow-style-free YAML list under `key:` of
// the shape:
//
//	key:
//	  - a
//	  - b
//
// This is a narrow scanner for this repo's own .goreleaser.yaml, not a
// general YAML parser — a third-party YAML module is not in the declared
// stack (spec/project.json, requirement 96c6c15ecc3e).
func extractYAMLList(t *testing.T, doc, key string) []string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	var items []string
	inList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == key+":" {
			inList = true
			continue
		}
		if inList {
			if strings.HasPrefix(trimmed, "- ") {
				items = append(items, strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
				continue
			}
			break
		}
	}
	if len(items) == 0 {
		t.Fatalf(".goreleaser.yaml: no %q list found", key)
	}
	return items
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
