# Spex Machina

*Spec ex machina — no deus required.*

[![release](https://img.shields.io/github/v/release/dmitriyb/spexmachina)](https://github.com/dmitriyb/spexmachina/releases)
[![go](https://img.shields.io/github/go-mod/go-version/dmitriyb/spexmachina)](go.mod)
[![license](https://img.shields.io/github/license/dmitriyb/spexmachina)](LICENSE)
[![ci](https://github.com/dmitriyb/spexmachina/actions/workflows/main.yml/badge.svg)](https://github.com/dmitriyb/spexmachina/actions/workflows/main.yml)

<!-- terminal recording: spex init → validate → diff → plan on a tiny spec. Placeholder until recorded. -->

`spex` owns the structural half of spec-driven development. You define your
project as a typed DAG — a JSON skeleton with markdown content leaves — and
`spex` tracks it with a merkle tree, computes which tasks a change invalidates,
and emits a tool-agnostic changeset an adapter applies to your tracker.

```
spec change → validate → diff → plan → adapter → ingest
                 │         │      │       │         │
                 │         │      │       │         └─ appends the journal,
                 │         │      │       │            writes the snapshot
                 │         │      │       └─ executes task actions
                 │         │      │          against the tracker
                 │         │      └─ decides the changeset: which tasks the
                 │         │         change creates, closes and retargets
                 │         └─ compares the merkle tree against the snapshot
                 └─ confirms the spec is a valid DAG
```

## What it does

- **Typed DAG spec.** Requirements, components, data flows, tests and apis with identity hashes and typed edges; `spex validate` refuses anything that is not an acyclic, fully covered graph.
- **Merkle diff.** `spex diff` hashes the spec bottom-up and compares it against the committed snapshot, grading every change by impact.
- **Deterministic changeset.** `spex plan` turns a diff plus live task state into an ordered list of `create` / `close` / `retarget` operations. No LLM in the loop: same spec, same snapshot, same output.
- **Adapter, outside the binary.** The changeset is tool-agnostic; an adapter applies it to your tracker and returns receipts. `scripts/apply-br.sh` is the reference, for `br`.
- **Journal.** `spex ingest` reconciles changeset and receipts, appends the task journal and moves the baseline. `spex map context` answers for any node, live or long removed.

Everything is pipeable, exits with documented codes, and lives in files committed to git. See [`docs/architecture.md`](docs/architecture.md).

[OpenSpec](https://github.com/Fission-AI/OpenSpec) makes the same bet on spec-driven work and trusts the model with the mechanical half. spex makes that half a program: what changed, what it invalidates and which tasks that means are computed, never judged.

## Install

Download the install script, verify it, then run it. Never `curl | sh`: a piped script cannot verify itself before it runs. Details, other shells and the trust model are in [`docs/install.md`](docs/install.md).

```bash
curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh     -o install.sh \
&& curl -fsSL https://github.com/dmitriyb/spexmachina/releases/latest/download/install.sh.sig -o install.sh.sig \
&& ssh-keygen -Y verify -f <(printf 'dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing\n') \
     -I dvbozhko@gmail.com -n file -s install.sh.sig < install.sh \
&& bash install.sh \
&& rm -f install.sh install.sh.sig
```

Public key, pin it once:

```
dvbozhko@gmail.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIhmCWVDP/Tcm3CqXNjTQTChbKxr223xMob9zc56Uuny release signing
```

<details>
<summary>fish, plain sh, verifying the archive directly, upgrading</summary>

- **fish** and **plain `sh`** variants of the block above: [`docs/install.md`](docs/install.md#primary-verified-install-script).
- **Maximal**: skip the script and verify the release archive itself by SSHSIG, SLSA attestation or the Go checksum database: [`docs/install.md`](docs/install.md#maximal-verify-the-binary-archive-directly).
- **Upgrading**: `spex upgrade` runs the same signed installer against its own path, forward-only; `--check`, `--version`, `--rollback`: [`docs/install.md`](docs/install.md#upgrading).

</details>

## Quick start

```sh
spex init                                            # .spex/: empty-tree snapshot, empty journal
spex validate                                        # the structural gate
spex diff                                            # what changed since the snapshot, by impact
spex register --git-head "$(git rev-parse --short HEAD)" spec/proposals/drafts/<stem>.md
scripts/export-br.sh tasks.json                      # in-flight tasks, from the tracker
spex diff --json | spex plan --proposal <stem> --git-head "$(git rev-parse --short HEAD)" \
                             --tasks tasks.json --out changeset.json
scripts/apply-br.sh changeset.json receipts.json     # the adapter, outside the binary
spex ingest --changeset changeset.json --receipts receipts.json   # move the baseline
spex map context <node-id>                           # the spec behind one task, live or removed
```

That is one full cycle: validate, find what changed, decide which tasks it creates, closes and retargets, let an adapter apply them, then record the result and move the baseline. Every flag and exit code is in [`docs/commands.md`](docs/commands.md).

## Learn more

- [`docs/architecture.md`](docs/architecture.md) — the pipeline, the merkle model, impact, the journal, and the terms.
- [`docs/configuration.md`](docs/configuration.md) — the spec format: `project.json`, `module.json`, leaves, node types, edges.
- [`docs/commands.md`](docs/commands.md) — every subcommand, flag and exit code.
- [`docs/skills.md`](docs/skills.md) — the authoring loop: `/propose`, `/spec`, `/spec-review`, `/mint`, `/drift`.
- [`docs/install.md`](docs/install.md) — install channels, the trust model, the public key, upgrading.
- `spec/**` — the authoritative, requirement-level specification, in spexmachina's own format.

## License

Apache-2.0, see [`LICENSE`](LICENSE).
