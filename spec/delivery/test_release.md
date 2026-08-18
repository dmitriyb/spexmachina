# Release Pipeline Tests

Acceptance coverage for [[649f5268a2b2|ReleasePipeline]] and [[5f9a7e641c09|ReleaseManifest]] together: a tag becomes a trustworthy, machine-consumable release. Exercised against a pre-release tag on the real repository (or a GoReleaser snapshot build locally where a real tag is not warranted).

## Setup

- A signed pre-release tag pushed to the repository.
- The release-signing public key available out of band for verification (the same shared Ed25519 key faber and portitor publish).

## Cases — artifacts

- The tag triggers exactly one release with four archives: linux/darwin crossed with amd64/arm64, named by the archive convention carrying tool, version, os, and arch.
- Each archive's binary reports the tag's version, the release commit, and the build date via `spex version` — the ldflags stamp, not the `dev` defaults.
- Building the same tag twice yields byte-identical binaries (CGO off, trimmed paths, no timestamp leakage outside the stamped date).
- Each archive verifies against the release-signing public key with the SSHSIG file-namespace check; a flipped byte in the archive makes the same check fail.
- The consolidated checksum file matches independently recomputed sha256 sums, and each archive also carries its own per-artifact checksum file.
- The SLSA build-provenance attestation for each archive verifies against the repository and the tag's commit.

## Cases — manifest

- The release carries a manifest whose entries cover exactly the four published archives — not GoReleaser's intermediate per-target directories or the metadata file.
- Each manifest entry's sha256 and size equal values recomputed from the downloaded archive bytes; a manifest entry disagreeing with its artifact is a test failure even if GoReleaser's own bookkeeping agrees with the manifest.
- The manifest names the tool, the tag-derived version, the commit, the build date, and a schema version pinning the manifest shape itself.
- A consumer resolving a single target (e.g. darwin/arm64) from the manifest alone obtains a checksum sufficient to verify that one archive without downloading the other three.

## Edge cases

- A pre-release tag publishes a GitHub pre-release: the release configuration declares the automatic pre-release mark, and the manifest built for that tag records the prerelease status.
- A failed signing step aborts the release rather than publishing unsigned archives.
