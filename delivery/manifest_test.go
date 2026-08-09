package delivery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeArchive writes a fake archive file under dir and returns its
// contents' sha256 and size, computed independently of the package under
// test so tests assert against a value the generator did not produce.
func writeArchive(t *testing.T, dir, name string, contents []byte) (sha256hex string, size int64) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), contents, 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:]), int64(len(contents))
}

// fourArchiveArtifacts returns the artifacts.json-shaped records for the
// linux/darwin x amd64/arm64 build matrix arch_release.md and
// test_release.md describe, plus the non-archive artifact types
// GoReleaser also records (intermediate binaries, checksum, signature)
// that a manifest must filter out.
func fourArchiveArtifacts(t *testing.T, dir string) ([]GoReleaserArtifact, map[string]string, map[string]int64) {
	t.Helper()
	targets := []struct{ goos, goarch string }{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	}
	var artifacts []GoReleaserArtifact
	wantSHA := map[string]string{}
	wantSize := map[string]int64{}
	for _, tg := range targets {
		name := "spex_1.2.3_" + tg.goos + "_" + tg.goarch + ".tar.gz"
		contents := []byte("archive contents for " + name)
		sum, size := writeArchive(t, dir, name, contents)
		wantSHA[tg.goos+"/"+tg.goarch] = sum
		wantSize[tg.goos+"/"+tg.goarch] = size

		// The intermediate per-target binary GoReleaser builds before
		// archiving: same os/arch, but not an Archive, and filtered out.
		binName := "spex_" + tg.goos + "_" + tg.goarch + "/spex"
		artifacts = append(artifacts,
			GoReleaserArtifact{Name: binName, Type: "Binary", Goos: tg.goos, Goarch: tg.goarch},
			GoReleaserArtifact{Name: name, Type: goreleaserArchiveType, Goos: tg.goos, Goarch: tg.goarch},
		)
	}
	// GoReleaser's own bookkeeping records, also filtered out.
	artifacts = append(artifacts,
		GoReleaserArtifact{Name: "spex_1.2.3_checksums.txt", Type: "Checksum"},
	)
	return artifacts, wantSHA, wantSize
}

func TestManifest_EntriesCoverPublishedArchivesOnly(t *testing.T) {
	dir := t.TempDir()
	artifacts, _, _ := fourArchiveArtifacts(t, dir)
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	if len(m.Artifacts) != 4 {
		t.Fatalf("manifest has %d entries, want 4 (intermediate binaries and metadata records must be filtered out): %+v", len(m.Artifacts), m.Artifacts)
	}
	for _, e := range m.Artifacts {
		if e.Filename == "" {
			t.Errorf("entry has empty filename: %+v", e)
		}
	}
}

func TestManifest_SHA256AndSizeRecomputedFromBytes(t *testing.T) {
	dir := t.TempDir()
	artifacts, wantSHA, wantSize := fourArchiveArtifacts(t, dir)
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	for target, want := range wantSHA {
		e, ok := m.Find(splitTarget(target))
		if !ok {
			t.Fatalf("manifest missing entry for target %s", target)
		}
		if e.SHA256 != want {
			t.Errorf("target %s: sha256 = %s, want %s (recomputed from archive bytes)", target, e.SHA256, want)
		}
		if e.Size != wantSize[target] {
			t.Errorf("target %s: size = %d, want %d", target, e.Size, wantSize[target])
		}
	}

	// A manifest entry disagreeing with its artifact is a failure even if
	// some other bookkeeping (never consulted here) would agree with it:
	// tamper with an archive after generation and confirm a fresh
	// recompute — the only source of truth this package trusts — no
	// longer matches the stale entry.
	target := "darwin/arm64"
	entry, _ := m.Find(splitTarget(target))
	if err := os.WriteFile(filepath.Join(dir, entry.Filename), []byte("tampered bytes"), 0o644); err != nil {
		t.Fatalf("tamper archive: %v", err)
	}
	m2, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest after tamper: %v", err)
	}
	entry2, _ := m2.Find(splitTarget(target))
	if entry2.SHA256 == entry.SHA256 {
		t.Fatalf("tampering the archive did not change its recomputed sha256")
	}
}

func TestManifest_ReleaseIdentityFields(t *testing.T) {
	dir := t.TempDir()
	artifacts, _, _ := fourArchiveArtifacts(t, dir)
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeefcafe", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	if m.Tool != "spex" {
		t.Errorf("Tool = %q, want %q", m.Tool, "spex")
	}
	if m.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", m.Version, "1.2.3")
	}
	if m.Commit != "deadbeefcafe" {
		t.Errorf("Commit = %q, want %q", m.Commit, "deadbeefcafe")
	}
	if m.Date != "2026-08-09T00:00:00Z" {
		t.Errorf("Date = %q, want %q", m.Date, "2026-08-09T00:00:00Z")
	}
	if m.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", m.SchemaVersion, SchemaVersion)
	}
	if m.Status == "" {
		t.Error("Status is empty")
	}
}

func TestManifest_ReleaseStatusFromVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"1.2.3", "release"},
		{"1.2.3-rc1", "prerelease"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := releaseStatus(tt.version); got != tt.want {
				t.Errorf("releaseStatus(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestManifest_SingleTargetResolutionSufficesToVerify(t *testing.T) {
	dir := t.TempDir()
	artifacts, wantSHA, wantSize := fourArchiveArtifacts(t, dir)
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}

	// A consumer resolving darwin/arm64 alone must get everything needed
	// to verify that one archive's bytes without consulting the other
	// three entries.
	e, ok := m.Find("darwin", "arm64")
	if !ok {
		t.Fatal("manifest missing darwin/arm64 entry")
	}
	if e.SHA256 != wantSHA["darwin/arm64"] || e.Size != wantSize["darwin/arm64"] {
		t.Fatalf("darwin/arm64 entry = %+v, want sha256=%s size=%d", e, wantSHA["darwin/arm64"], wantSize["darwin/arm64"])
	}
	data, err := os.ReadFile(filepath.Join(dir, e.Filename))
	if err != nil {
		t.Fatalf("read resolved archive: %v", err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != e.SHA256 {
		t.Fatal("resolved entry's sha256 does not verify the archive it names")
	}
}

func TestManifest_MissingArchiveFileIsError(t *testing.T) {
	dir := t.TempDir()
	artifacts := []GoReleaserArtifact{
		{Name: "spex_1.2.3_linux_amd64.tar.gz", Type: goreleaserArchiveType, Goos: "linux", Goarch: "amd64"},
	}
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	_, err := GenerateManifest(dir, artifacts, meta)
	if err == nil {
		t.Fatal("GenerateManifest with an Archive record whose file is absent returned nil error, want a release-blocking failure")
	}
}

func TestManifest_ChecksumDisagreementIsError(t *testing.T) {
	dir := t.TempDir()
	name := "spex_1.2.3_linux_amd64.tar.gz"
	writeArchive(t, dir, name, []byte("archive contents"))
	artifacts := []GoReleaserArtifact{
		{
			Name: name, Type: goreleaserArchiveType, Goos: "linux", Goarch: "amd64",
			Extra: goreleaserExtra{Checksum: "sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"},
		},
	}
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	_, err := GenerateManifest(dir, artifacts, meta)
	if err == nil {
		t.Fatal("GenerateManifest with a recomputed sha256 disagreeing with GoReleaser's recorded checksum returned nil error, want a release-blocking failure")
	}
}

func TestManifest_ChecksumAgreementDoesNotError(t *testing.T) {
	dir := t.TempDir()
	name := "spex_1.2.3_linux_amd64.tar.gz"
	sum, _ := writeArchive(t, dir, name, []byte("archive contents"))
	artifacts := []GoReleaserArtifact{
		{
			Name: name, Type: goreleaserArchiveType, Goos: "linux", Goarch: "amd64",
			Extra: goreleaserExtra{Checksum: "sha256:" + sum},
		},
	}
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest with an agreeing recorded checksum returned an error: %v", err)
	}
	if len(m.Artifacts) != 1 {
		t.Fatalf("manifest has %d entries, want 1", len(m.Artifacts))
	}
}

func TestManifest_LoadGoReleaserRecords(t *testing.T) {
	dir := t.TempDir()

	artifactsJSON := `[
		{"name":"spex_1.2.3_linux_amd64.tar.gz","type":"Archive","goos":"linux","goarch":"amd64"},
		{"name":"spex_1.2.3_checksums.txt","type":"Checksum"}
	]`
	if err := os.WriteFile(filepath.Join(dir, "artifacts.json"), []byte(artifactsJSON), 0o644); err != nil {
		t.Fatalf("write artifacts.json: %v", err)
	}
	metaJSON := `{"project_name":"spex","version":"1.2.3","commit":"deadbeef","date":"2026-08-09T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(metaJSON), 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	artifacts, err := LoadGoReleaserArtifacts(filepath.Join(dir, "artifacts.json"))
	if err != nil {
		t.Fatalf("LoadGoReleaserArtifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(artifacts))
	}

	meta, err := LoadGoReleaserMetadata(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("LoadGoReleaserMetadata: %v", err)
	}
	if meta.ProjectName != "spex" || meta.Version != "1.2.3" {
		t.Errorf("meta = %+v, want project_name=spex version=1.2.3", meta)
	}
}

func TestManifest_LoadGoReleaserArtifactsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifacts.json")
	if err := os.WriteFile(path, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("write artifacts.json: %v", err)
	}

	if _, err := LoadGoReleaserArtifacts(path); err == nil {
		t.Fatal("LoadGoReleaserArtifacts with malformed JSON returned nil error")
	}
}

func TestManifest_LoadGoReleaserMetadataMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metadata.json")
	if err := os.WriteFile(path, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	if _, err := LoadGoReleaserMetadata(path); err == nil {
		t.Fatal("LoadGoReleaserMetadata with malformed JSON returned nil error")
	}
}

func TestManifest_MarshalsToJSON(t *testing.T) {
	dir := t.TempDir()
	artifacts, _, _ := fourArchiveArtifacts(t, dir)
	meta := GoReleaserMetadata{ProjectName: "spex", Version: "1.2.3", Commit: "deadbeef", Date: "2026-08-09T00:00:00Z"}

	m, err := GenerateManifest(dir, artifacts, meta)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if _, err := json.Marshal(m); err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
}

func splitTarget(target string) (goos, goarch string) {
	for i := 0; i < len(target); i++ {
		if target[i] == '/' {
			return target[:i], target[i+1:]
		}
	}
	return target, ""
}
