# Spex Machina

*Spec ex machina — no deus required.*

A CLI (`spex`) that owns the structural half of spec-driven development.
You define your project as a typed DAG — a JSON skeleton with markdown content leaves — and `spex` tracks it with a merkle tree, computes which tasks a change invalidates, and emits a tool-agnostic changeset an adapter applies to your tracker.

```
spec change → validate → diff → impact → emit → adapter → ingest
                 │         │      │        │        │        │
                 │         │      │        │        │        └─ appends the journal,
                 │         │      │        │        │           writes the snapshot
                 │         │      │        │        └─ executes task actions
                 │         │      │        │           against the tracker
                 │         │      │        └─ composes the next changeset
                 │         │      └─ maps changed nodes to affected tasks
                 │         └─ compares the merkle tree against the snapshot
                 └─ confirms the spec is a valid DAG
```

**No LLM in the loop** — just deterministic graph operations: the same spec state plus the same snapshot always produce the same diff, impact and changeset.
Every subcommand reads stdin or files, writes stdout or files, and exits with documented codes, so the pipeline above is literally a pipeline.
Snapshots, proposals and the task journal are files committed to git; there is no external state and no database.
Nothing in that pipeline shells out — the git SHA is caller-supplied and the tracker is reached only through an external adapter — which is why `spex` stays a single static binary with no runtime dependencies (`spex upgrade` is the one exception: it runs its own embedded, signed installer under `bash`).
See [`docs/architecture.md`](docs/architecture.md) for the full model.

---

## Install

One binary is published on the [GitHub Releases page][releases]: `spex` (linux/darwin, amd64/arm64).
Every release archive is signed with SSHSIG (`ssh-keygen -Y sign`), verifiable with the `ssh-keygen` that already ships with OpenSSH on essentially every machine — no extra tool to install just to verify.

[releases]: https://github.com/dmitriyb/spexmachina/releases

### Primary: verified install script

**bash / zsh:**

```bash
curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh     -o install.sh \
&& curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh.sig -o install.sh.sig \
&& ssh-keygen -Y verify -f <(printf 'dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing\n') \
     -I dvbozhko@gmail.com -n file -s install.sh.sig < install.sh \
&& bash install.sh \
&& rm -f install.sh install.sh.sig
```

**fish:**

```fish
curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh -o install.sh
and curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh.sig -o install.sh.sig
and ssh-keygen -Y verify -f (printf 'dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing\n' | psub) -I dvbozhko@gmail.com -n file -s install.sh.sig < install.sh
and bash install.sh
and rm -f install.sh install.sh.sig
```

This downloads `install.sh`, verifies **the script itself** against the public key below, and only then runs it — never `curl | sh`.
`install.sh` then resolves the latest release, detects your OS/arch, downloads the matching `spex` archive and its signature, and verifies the **binary** with the same key (embedded in the script, trusted because the script was just verified) before installing it.
The script is bash, not POSIX `sh` — run it with `bash`, as above.
Set `SPEX_INSTALL_VERSION=v0.1.0` before the final `bash install.sh` to install a specific release instead of the latest, and pass `--dir DIR` to choose the install directory (default `$HOME/.local/bin`).

The block above needs bash or zsh (`<(…)` process substitution).
Under a plain `sh`, write the allowed-signers line to a file first:

```sh
printf 'dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing\n' > allowed_signers
ssh-keygen -Y verify -f allowed_signers -I dvbozhko@gmail.com -n file -s install.sh.sig < install.sh
```

### Maximal: verify the binary archive directly

No install script — download the archive for your platform from the [Releases page][releases], then verify it by any one of:

```bash
# SSHSIG, against the same pinned key as above
ssh-keygen -Y verify -f allowed_signers -I dvbozhko@gmail.com -n file \
  -s spex_<version>_<os>_<arch>.tar.gz.sig < spex_<version>_<os>_<arch>.tar.gz

# SLSA provenance via Sigstore/Rekor — identity-anchored, no key to manage
gh attestation verify spex_<version>_<os>_<arch>.tar.gz --repo dmitriyb/spexmachina

# Go users: the Go module checksum database
go install github.com/dmitriyb/spexmachina/cmd/spex@<tag>
```

Archive names carry the version without its leading `v` — `spex_0.1.0_linux_amd64.tar.gz` for tag `v0.1.0`.
Each release also carries a consolidated `spex_<version>_checksums.txt`, one `.sha256` per archive, and a machine-readable `manifest.json` (schema, target, sha256, size per artifact).

### What each channel protects, and what it doesn't

- **Primary** verifies both the install script and the binary it fetches, end to end — `download → verify → run`, never a piped script: a piped `curl … | sh` executes as it streams and cannot verify itself before running, so verification has to wrap the download from outside the stream, which is exactly why the primary path is not a one-liner pipe.
- **Maximal** gives you the strongest per-artifact check for a single file, with no script in between.
- The trust anchor in both cases is the public key **copied from this README** — that defeats tampering of the download in transit; the residual risk is being sent to a look-alike or phishing copy of this repository, closed by using the known repository URL and by pinning the public key **once** — copy it a single time, then verify every future release against that pinned copy rather than re-copying it from wherever you happen to land.
- Signatures and attestations give **authenticity, not freshness**: a channel attacker who can intercept your download could still steer you to a genuine-but-older, vulnerable release (a downgrade); this applies to every channel above equally at *first install*, where there is no installed version to floor against — note it as a residual risk rather than a solved one. For *updates* this is closed: `spex upgrade` is forward-only and hard-refuses (non-overridably) a resolved latest that is older than what is installed (see Upgrading).

### Public key

```
dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing
```

This is the same key across all three verification paths above (SSHSIG install script, SSHSIG archive, and the `allowed_signers` line either way), and the same line spex's sibling tools — [faber](https://github.com/dmitriyb/faber) and [portitor](https://github.com/dmitriyb/portitor) — publish, so one pinned copy serves all three.
It can also be pinned and cross-checked against GitHub's own copy at `https://api.github.com/users/dmitriyb/ssh_signing_keys`, once it is added under Settings → SSH and GPG keys → Signing keys — useful if this README itself is ever suspected of being tampered with in a fork or mirror.

### Upgrading

An installed binary updates itself with `spex upgrade`, which embeds the same signed `install.sh` above and runs it against the running binary's own path — same resolve → download → SSHSIG-verify, then a safe in-place swap (move-aside + `rename(2)`, never a write over the running file), keeping the previous binary as `<path>.bak`.
Upgrade is **forward-only**: it resolves the latest release and moves toward it, and hard-refuses (non-overridable) a resolved latest that is *older* than the installed version — a signature proves authenticity, not freshness, so a latest that moved backward is treated as a compromised-origin rollback anomaly.
`--check` reports the comparison without changing anything (and exits 0 even when the outcome is an anomaly), `--version vX.Y.Z` installs an exact release in any direction (the deliberate path to an older release), and `--rollback` restores the backup.
See [`docs/commands.md`](docs/commands.md) for the full flag reference.

---

## Usage sketch

```sh
spex validate                                   # DAG, refs, coverage — before anything else
spex diff --json | spex impact --beads beads.json > impact.json
spex emit --impact impact.json --proposal <stem> --git-head "$(git rev-parse HEAD)" \
  > changeset.json
scripts/apply-br.sh changeset.json > receipts.json   # the adapter — outside the binary
spex ingest --changeset changeset.json --receipts receipts.json
```

That is one full cycle: validate the spec, find what changed since the snapshot, decide which tasks it invalidates, compose the changeset, let an adapter apply it, then record the result and move the baseline.
`spex map context <node-id>` answers the other everyday question — the full spec context behind one task, live or long removed.
Everything is pipeable and every subcommand documents its exit codes; see [`docs/commands.md`](docs/commands.md).

## Learn more

- [`docs/architecture.md`](docs/architecture.md) — the module pipeline, the merkle model, how impact is classified, and where the snapshot fits.
- [`docs/configuration.md`](docs/configuration.md) — the spec format: `project.json`, `module.json`, markdown leaves, node types and edges.
- [`docs/commands.md`](docs/commands.md) — the full CLI reference: every subcommand, flag and exit code.
- [`docs/skills.md`](docs/skills.md) — the authoring loop: `/propose`, `/spec`, `/spec-review`, `/drift-fix`.
- [`docs/enforcement-migration.md`](docs/enforcement-migration.md) — migrating a spec to the declarative enforcement contracts.
- `spec/**` — the authoritative, requirement-level specification (spexmachina format).

## License

Apache-2.0 (see `LICENSE`).
