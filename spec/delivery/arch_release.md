# Release Pipeline

A tag-triggered GoReleaser build turning a version tag into a GitHub Release, satisfying [[1a614dd66ebb|Cross-architecture binary release]]. The GoReleaser configuration and the release workflow are the source of truth for how a release is built; this leaf states the contract they must keep. The pipeline is adopted from the faber and portitor releases — same shape, same key, so the three tools stay in lockstep.

## Build matrix and stamping

The build cross-compiles linux and darwin on amd64 and arm64 — four archives, named by tool, version, os, and arch. CGO is disabled and paths are trimmed; version, commit, and date are injected only through ldflags into the variables the version command already declares. That stamping discipline is what [[6956c1a875ae|Deterministic ldflags-stamped builds]] requires: a given tag yields byte-stable binaries, and the stamped version is the authoritative input to the self-update forward-only guard — the upgrade path never has to re-execute the about-to-be-replaced binary to learn what it is.

## Trust chain

[[1cbfffdeb281|Signed and attested artifacts]] demands three independent ways to trust a download, and the release publishes all three:

- an SSHSIG signature per archive, made with the shared Ed25519 release key (the same key faber, portitor, and spex publish), verifiable with the standard file-namespace check;
- sha256 checksums — consolidated for the release, plus a per-artifact checksum file so one target verifies without downloading the other three;
- a SLSA build-provenance attestation binding each archive to the repository and the tag's commit.

A failed signing step aborts the release; unsigned archives are never published.

## Release assets

Beyond the archives, the release carries the checksum files, the signatures, the manifest produced by the manifest generator, and the canonical install script with its signature — the verified download path the README documents.
