# Change Proposal: Tool Delivery

## Context

Every spex module is implemented, all beads are closed, and the spec is fully covered — yet the
tool only exists for someone who clones the repo and runs `go build`. There is no CI, no release
process, no published binaries, no install path, and no way to verify that a given `spex` binary
corresponds to a given commit. `cmd/spex/version.go` already declares `version`, `commit`, and
`date` for ldflags injection, but nothing wires them up; `.github/` holds only a PR template.

Two sibling tools — faber and portitor — already run this subsystem in production, and this proposal
adopts their model. The defining decision is where delivery lives in the spec: **delivery is a
standalone module, not spec data.** CI, release automation, and distribution are described by an
ordinary `delivery` module — requirements, components, content leaves — that flows through the
normal spex pipeline: merkle tracks it, impact maps it, beads are cut from it. The executable
artifacts themselves — `.goreleaser.yaml`, the GitHub Actions workflows, `install.sh` — are the
single source of truth for how a release is actually built. The module documents intent; it defines
**no** bespoke delivery-metadata schema and adds nothing to `project.json` beyond an ordinary module
registration.

This supersedes the earlier idea of modeling delivery as a coupled *section* of `project.json`. The
proposal therefore drops its dependency on the *Coupled Sections* proposal
(`2026-04-02-coupled-sections.md`): there is no `sections` array, no `section.schema.json`, and no
delivery content validated by spex core. Coupled Sections remains a standalone idea for other future
concerns.

### Reference implementations

faber and portitor are sibling tools written by the same author as spex, and are other parts of the
larger flow-orchestration system it belongs to. Both already implement this delivery subsystem, so
their specs are the starting point for `/spec` and the implementation pass. They live in the public
repositories `github.com/dmitriyb/faber` and `github.com/dmitriyb/portitor`, which should be cloned
into `/tmp` to read the details below. Portitor is the closer analog (a single binary, like spex).

- **Module structure** — each repo's `spec/delivery/` (`module.json`, `arch_ci.md`, `arch_release.md`,
  `arch_self_update.md`, `test_delivery.md`).
- **`spex upgrade` front-end** — portitor's `spec/cli/arch_upgrade_command.md` and
  `test_upgrade_command.md`.

### Deferred to follow-up proposals

- **Homebrew and AUR distribution.** The pipeline ships binaries and a verified install script only.
  Package-manager channels are deferred to a single later proposal that adds them consistently across
  faber, portitor, and spex at once, so the three tools stay in lockstep.
- **Windows target.** The build matrix is linux/darwin × amd64/arm64 to match the sibling tools. A
  `windows/amd64` target is deferred to its own proposal.

## Proposed change

### A new `delivery` module

Register a `delivery` module in `project.json`'s `modules[]` and author `spec/delivery/` through
`/spec`. At the level of components, the module covers:

- **CI pipeline** — trigger-tiered GitHub Actions: a fast gate on every PR (build, vet, test), the
  same gate plus `-race` on push to main, and a nightly fuzz run that discovers `Fuzz*` targets at
  run time (no hardcoded list). Docs-only changes skip the runner; each workflow cancels its own
  superseded runs.
- **Release pipeline** — a GoReleaser build triggered by a `v*` tag, cross-compiling linux/darwin ×
  amd64/arm64 with `CGO_ENABLED=0`, `-trimpath`, and ldflags stamping `main.version/commit/date`,
  publishing a GitHub Release.
- **Signing and provenance** — every archive SSHSIG-signed (`ssh-keygen -Y sign -n file`, the
  Ed25519 release key already shared with faber and portitor), plus a SLSA build-provenance
  attestation and sha256 checksums: three independent ways to trust a downloaded binary.
- **Release manifest** — a self-verifying `manifest.json` derived from GoReleaser's own output,
  recomputing each artifact's sha256/size from the bytes on disk, so a downstream consumer can pin a
  per-target checksum programmatically.
- **Self-update mechanism** — a signed, verify-then-run `install.sh` that also carries an upgrade
  mode; it is embedded byte-for-byte into the binary and kept identical to the released copy by a
  build-failing test. This is what `spex upgrade` drives.

The module's requirements trace to the two project requirements below; its concrete requirement,
component, and content-leaf structure is authored by `/spec`, not enumerated here.

### New project requirements and a milestone

- **Delivery pipeline** (functional) — spex has an automated, reproducible release pipeline that
  turns a version tag into signed, checksummed, provenance-attested, cross-architecture binaries,
  with `.goreleaser.yaml` and the workflow YAML as the source of truth.
- **Installable and self-updating** (non-functional) — pre-built signed binaries for linux and macOS
  (amd64, arm64), installable via a verified `install.sh` and `go install`, and updatable in place
  with `spex upgrade`.
- A **Delivery** milestone grouping the module.

### `spex upgrade` in the `cli` module

Add an `UpgradeCommand` to the existing `cli` module. `spex upgrade` self-updates the installed
binary by driving the embedded, already-signed `install.sh` in an upgrade mode against the running
binary's own path — it reimplements none of the resolve/download/verify/replace logic. It is
forward-only and has no `--force`:

```
spex upgrade                    # resolve the latest signed release and move to it
spex upgrade --version vX.Y.Z   # install an exact release, in any direction
spex upgrade --check            # resolve + verify, report only, change nothing (alias: --dry-run)
spex upgrade --rollback         # restore the previous binary from its .bak backup
```

Forward-only means a resolved *latest* that is older than what is installed is a rollback anomaly the
command hard-refuses, non-overridably; an explicitly named `--version` installs in any direction. The
mechanism it drives is the delivery module's self-update component. (`spex version` already exists in
the `cli` module.)

### Installation and upgrade documentation

Update `README.md` with an Installation section — verified download of `install.sh` (verify the
script's signature, then run it; never `curl | sh`), plus `go install
github.com/dmitriyb/spexmachina/cmd/spex@latest` and build-from-source — and an Upgrading section
documenting the `spex upgrade` surface above and the published release-signing public key.

## Impact expectation

**New beads:**
- An epic for the `delivery` module.
- Feature beads per delivery component: CI pipeline, release pipeline, signing/provenance, release
  manifest, self-update mechanism.
- A feature bead for the `cli` module's `UpgradeCommand`.
- Task beads for each component's impl and test content leaves, cut through the normal pipeline.

**Modified beads:** The `cli` module gains one new component (`UpgradeCommand`); its existing beads
are otherwise unchanged.

**Closed beads:** None.

**Estimated scope:** ~2 sessions — (1) author the `delivery` module and `cli/UpgradeCommand` spec via
`/spec`; (2) implement the infrastructure (`.goreleaser.yaml`, CI/release workflows, `install.sh` +
upgrade mode, `manifest.json`, `spex upgrade`, README), starting from the proven faber/portitor
artifacts.
