// Package delivery holds spex's release-pipeline tooling: build-failing
// tests that pin the non-Go release artifacts (.goreleaser.yaml, the
// release workflow) to the contract spec/delivery/arch_release.md states,
// and the release manifest generator that turns GoReleaser's own output
// into the self-verifying manifest.json spec/delivery/arch_manifest.md
// describes.
package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SchemaVersion pins the release manifest's shape, independent of the spex
// version a given manifest describes.
const SchemaVersion = 1

// goreleaserArchiveType is the artifacts.json "type" value GoReleaser
// assigns to a published archive, as opposed to an intermediate per-target
// binary, a checksum file, or a signature.
const goreleaserArchiveType = "Archive"

// GoReleaserArtifact is the subset of a dist/artifacts.json record this
// package reads. GoReleaser writes one entry per intermediate binary,
// archive, checksum file, and signature; only entries whose Type is
// "Archive" become manifest entries.
type GoReleaserArtifact struct {
	Name   string          `json:"name"`
	Type   string          `json:"type"`
	Goos   string          `json:"goos"`
	Goarch string          `json:"goarch"`
	Extra  goreleaserExtra `json:"extra"`
}

// goreleaserExtra is the subset of an artifacts.json record's "extra" map
// this package reads: GoReleaser's own recorded checksum, shaped
// "sha256:<hex>". It is used only as a cross-check against the sha256 this
// package recomputes from the archive bytes — never as the source of
// truth — so a future GoReleaser upgrade dropping or reshaping other
// "extra" fields does not affect this package.
type goreleaserExtra struct {
	Checksum string `json:"Checksum"`
}

// GoReleaserMetadata is the subset of a dist/metadata.json record this
// package reads.
type GoReleaserMetadata struct {
	ProjectName string `json:"project_name"`
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	Date        string `json:"date"`
}

// Target names the os/arch pair a manifest entry was built for, using the
// same os and arch strings the archive naming convention does.
type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// ManifestEntry describes one published archive. SHA256 and Size are
// recomputed from the archive bytes on disk — never copied from
// GoReleaser's own checksum bookkeeping — so they say what the archive
// actually hashes to.
type ManifestEntry struct {
	Target   Target `json:"target"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
}

// Manifest is the self-verifying description of a release: the release
// identity plus one entry per published archive, letting a downstream
// consumer resolve and verify a single target without scraping the
// release page or downloading every archive.
type Manifest struct {
	SchemaVersion int             `json:"schema_version"`
	Tool          string          `json:"tool"`
	Version       string          `json:"version"`
	Commit        string          `json:"commit"`
	Date          string          `json:"date"`
	Status        string          `json:"status"`
	Artifacts     []ManifestEntry `json:"artifacts"`
}

// Find returns the manifest entry for the given os/arch target, sufficient
// on its own to resolve and verify that one archive.
func (m *Manifest) Find(os, arch string) (ManifestEntry, bool) {
	for _, e := range m.Artifacts {
		if e.Target.OS == os && e.Target.Arch == arch {
			return e, true
		}
	}
	return ManifestEntry{}, false
}

// GenerateManifest reads GoReleaser's own artifact records, recomputes
// each published archive's sha256 and size from the bytes at distDir, and
// returns the resulting manifest. GoReleaser's intermediate per-target
// binaries and its own metadata records are not "Archive"-typed and are
// filtered out. A missing or unreadable archive file is a release-blocking
// failure, so it is returned as an error rather than skipped. When an
// artifact record carries GoReleaser's own recorded checksum, the
// recomputed hash is cross-checked against it; a disagreement between the
// two is also a release-blocking failure. A record with no checksum, or
// one not shaped "sha256:<hex>", skips the cross-check rather than
// failing, so a future GoReleaser upgrade changing or omitting that field
// does not break generation.
func GenerateManifest(distDir string, artifacts []GoReleaserArtifact, meta GoReleaserMetadata) (*Manifest, error) {
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		Tool:          meta.ProjectName,
		Version:       meta.Version,
		Commit:        meta.Commit,
		Date:          meta.Date,
		Status:        releaseStatus(meta.Version),
	}
	for _, a := range artifacts {
		if a.Type != goreleaserArchiveType {
			continue
		}
		sum, size, err := hashFile(filepath.Join(distDir, a.Name))
		if err != nil {
			return nil, fmt.Errorf("delivery: generate manifest: archive %s: %w", a.Name, err)
		}
		if recorded, ok := parseGoReleaserChecksum(a.Extra.Checksum); ok && recorded != sum {
			return nil, fmt.Errorf("delivery: generate manifest: archive %s: recomputed sha256 %s disagrees with GoReleaser's recorded checksum %s", a.Name, sum, recorded)
		}
		m.Artifacts = append(m.Artifacts, ManifestEntry{
			Target:   Target{OS: a.Goos, Arch: a.Goarch},
			Filename: a.Name,
			SHA256:   sum,
			Size:     size,
		})
	}
	return m, nil
}

// parseGoReleaserChecksum extracts the hex digest from a GoReleaser
// "extra.Checksum" value shaped "sha256:<hex>". A value with no such
// prefix — including the empty string a record with no checksum
// unmarshals to — returns ok=false, telling GenerateManifest to skip the
// disagreement check rather than treat an absent or reshaped field as a
// mismatch.
func parseGoReleaserChecksum(checksum string) (hexDigest string, ok bool) {
	const prefix = "sha256:"
	if !strings.HasPrefix(checksum, prefix) {
		return "", false
	}
	return strings.TrimPrefix(checksum, prefix), true
}

// releaseStatus derives a release's status from its tag-derived version:
// a semver prerelease component (a hyphen after the numeric core) marks
// the release a prerelease, so the manifest reflects it without needing
// any state beyond the version string itself.
func releaseStatus(version string) string {
	if strings.Contains(version, "-") {
		return "prerelease"
	}
	return "release"
}

func hashFile(path string) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("read %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// LoadGoReleaserArtifacts reads and parses a GoReleaser dist/artifacts.json
// file.
func LoadGoReleaserArtifacts(path string) ([]GoReleaserArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("delivery: load artifacts: %w", err)
	}
	var artifacts []GoReleaserArtifact
	if err := json.Unmarshal(data, &artifacts); err != nil {
		return nil, fmt.Errorf("delivery: load artifacts: parse %s: %w", path, err)
	}
	return artifacts, nil
}

// LoadGoReleaserMetadata reads and parses a GoReleaser dist/metadata.json
// file.
func LoadGoReleaserMetadata(path string) (GoReleaserMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return GoReleaserMetadata{}, fmt.Errorf("delivery: load metadata: %w", err)
	}
	var meta GoReleaserMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return GoReleaserMetadata{}, fmt.Errorf("delivery: load metadata: parse %s: %w", path, err)
	}
	return meta, nil
}
