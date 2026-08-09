# Release Manifest

A machine-readable description of a release, satisfying [[99127d3a1b95|Machine-readable release manifest]]: one JSON document a downstream consumer reads to resolve and verify a single target programmatically, without scraping the release page or downloading every archive.

## Self-verification

The generator runs after [[649f5268a2b2|ReleasePipeline]] has produced its output and reads GoReleaser's own artifact and metadata records — but deliberately does not trust their checksum bookkeeping. For each published archive it recomputes the sha256 and size directly from the bytes on disk. The manifest therefore says what the artifacts actually hash to, not what the build tool's internal records claim; a disagreement between the two is a release-blocking failure, and the manifest survives a future GoReleaser upgrade changing its internal fields.

## Shape

The manifest names the tool, the tag-derived version, the commit, the build date, a release status, and one entry per published archive — target (os and architecture, matching the archive naming convention), filename, sha256, and size. A schema version pins the manifest shape itself, independent of the tool version it describes. Entries cover exactly the published archives: GoReleaser's intermediate per-target directories and its metadata records are filtered out.
