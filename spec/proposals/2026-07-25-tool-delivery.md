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

- **CI pipeline** — trigger-tiered GitHub Actions: a fast gate on every PR (build, vet, test, **plus
  the spec gate below**), the same gate plus `-race` on push to main, and a nightly fuzz run that
  discovers `Fuzz*` targets at run time (no hardcoded list). Docs-only changes skip the runner; each
  workflow cancels its own superseded runs.
- **Spec gate** — the half of CI that makes spex self-hosting rather than merely tested. On every PR,
  after `go build -o bin/ ./cmd/spex/`, run `bin/spex validate` and then assert that `spex diff`
  reports no completeness errors. **The second step is the one that matters.** `spex validate` returns
  success on warnings, and `incomplete_change` — the class that catches a spec edit whose obligations
  were not met — surfaces *only* in the diff `errors` array, never in `validate`. Without it the job
  is decorative: a spec change can merge having silently broken the graph it claims to describe.

  Two implementation constraints, both learned the hard way and both non-obvious:

  - **Do not write it as `bin/spex diff --json | jq -e '.errors == []'`.** The pipe discards `diff`'s
    own exit code. `spex diff` exits **0** clean, **2** on completeness errors and **1** when the tree
    does not build — and on exit 1 it writes *nothing* to stdout, so `jq` fails with an opaque parse
    error naming no file, while a `diff` that exited 1 having emitted `{"errors":[]}` would pass.
    Capture to a file, preserve the status, and branch on all three cases.
  - **"Required for merge" is a branch-protection rule**, not something the workflow can declare. The
    status check to mark required is the job name.

  This closes the root cause the *declarative spec contracts* proposal identifies as its fourth
  defect — *"Nothing runs the checks."* On the day `spex apply` was deleted, nothing would have
  invoked `spex validate`, and the spec drifted for three months undetected. That proposal originally
  carried the CI job itself; it was withdrawn from there and moved here, so this is now the only place
  the check is specified.
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
- ~~A **Delivery** milestone grouping the module.~~ **Not available — see below.**

### Depends on the declarative spec contracts migration

That migration (`2026-07-25-declarative-spec-contracts.md`, in flight on
`proposal/declarative-spec-contracts`) changes the spec format under this proposal. Two things written
above are no longer authorable, and one is new:

- **Milestones are deleted.** The node type is gone from `schema/schema.go`,
  `schema/project.schema.json`, `cmd/spex/hashid.go` and `validator/id_validator.go`;
  `spex hash-id --type milestone` now exits 1 and a `milestones` key in `project.json` is a hard schema
  failure. The **Delivery** milestone cannot be created. Grouping the module needs no node — the module
  registration is the grouping — so the simplest resolution is to drop the bullet.
- **Impl sections are being removed.** The *Impact expectation* section below says "Task beads for each
  component's **impl** and test content leaves". After the migration a component's contract lives wholly
  in its arch leaf; there are no impl leaves to cut beads from. Expect arch + test only.
- **Arch leaves carry no Go.** No `func` signatures, no ```go fences — the exception is narrow (a
  requirement whose *description* names the algorithm or bound the fence encodes). This matters here
  because delivery's arch leaves will want to describe workflow YAML and shell, and the same rule
  applies: describe the contract, not the implementation.
- **A new `api` node type exists**, declaring an external surface as callers write it. `spex upgrade`
  below is exactly that — it should be declared as an api in the `cli` module (`hash-id --type api
  --module cli --name "spex upgrade"`), not only as a component. Note names must be *declarable*:
  tokenizing must reproduce the name exactly, in at most six whitespace-separated words, so
  `spex upgrade` is fine and `spex upgrade [--check]` is rejected.

None of this blocks authoring; it changes what `/spec` may write. **Author `spec/delivery/` after that
migration merges**, or expect to redo the impl leaves.

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
