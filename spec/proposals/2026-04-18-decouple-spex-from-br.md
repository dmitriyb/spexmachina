# Change Proposal: Decouple spex binary from br/bd

## Context

The `spex` binary shells out to external subprocesses at runtime in five call sites, all tied to the bead tracker (`br`/`bd`) or to git:

- **apply** (module 5): `apply/bead_creator.go` runs `br create`, `br list --json`, `br show <id> --json`, `br update`, `br close`, and `br --version` across the BeadCreator (component 1) and the `NewBeadCLI` probe. `apply/bead_closer.go:16` runs `git rev-parse HEAD` to build the `commit:<HEAD>` label on obsoleted beads. `cmd/spex/apply.go` exposes a `--bead-cli` flag and constructs a probing `BeadCLI` at runtime.
- **impact** (module 4): `impact/bead_reader.go` runs `br list --json` and is intended to populate `mapping.Record.BeadStatus` for the ActionClassifier (component 3). The function is modeled, tested, and compiled, but `cmd/spex/impact.go` never calls it. No other code path writes `bead_status` into `.bead-map.json`. The cleanup-bead gate at `impact/action_classifier.go:114` (`if o.Record.BeadStatus == "closed"`) therefore always evaluates false in production: when a spec module is removed, its obsolete actions fire, but the task beads that would track deletion of orphaned source code are never generated. The bug is latent today because no proposal has so far removed a whole module.
- **proposal** (module 6): `proposal/exec.go` and `proposal/history.go:55` run `br list --json` behind the HistoryViewer component to filter beads by `spec_proposal:<ref>` label. `cmd/spex/log.go:28` hardcodes `Bin: "br"` with no override.

A related and concrete bug surfaces in the same data path: the dep-graph wiring produced by the current `impact → apply` flow is broken whenever two dependent components are replaced in the same run. `impact/action_classifier.go:ResolveDeps` resolves spec-graph dependencies against mapping records at impact time. When a dep component is itself being obsoleted + recreated in the batch, the resolver picks up its OLD bead ID (the only one in the mapping at that moment). `apply/bead_creator.go:CreateBeads` passes that OLD ID to `br create --deps blocked-by:<old>`. The old bead is then closed by the obsolete phase, leaving the new bead "blocked-by" a dead bead with no edge to the new replacement. Visible in commit `21defea` (the 2026-04-12-data-flow-contract-layer apply run): all five new feature beads (`spexmachina-7hb`, `ec7`, `r4o`, `lln`, `p6z`) show `dependency_count: 1` — the single dep is always the OLD closed predecessor from lineage, and no spec-graph edges connect the new beads to each other despite `spec/merkle/module.json` saying e.g. DiffCommand uses SnapshotStore + DiffEngine + ImpactClassifier + CompletenessChecker. `spec/impact/impl_dependency_resolution.md:87` documents this as a "limitation" and claims ApplyCommand's topological sort handles it; the topo sort only fixes creation order, not edge targets. This proposal's `changeset.json` three-shape reference scheme (see §2) is the structural fix — forward references let same-run deps resolve to the new bead IDs at adapter-exec time instead of to stale pre-resolved values from impact time.

The consequences are concrete:

1. The spex binary cannot run, or be tested, without `br` (or `bd`) present on `PATH`. Integration tests under `apply/exec_cli_test.go` skip when the binary is missing.
2. spex is coupled to br's specific subcommand and flag surface, and to br's exact JSON output shapes (`list` returns `{"issues": [...]}`, `show` returns an array).
3. The `--bead-cli bd` override is incomplete: `spex log` ignores it, and `apply.NewBeadCLI` probes br-specific flag combinations at construction time.
4. The spex binary is not a pure `(inputs) → (outputs)` tool. Its behavior depends on the state of an external process, not solely on its arguments and the files it reads.

Proposal `2026-04-12-data-flow-contract-layer.md` (deferred until the current 16 open beads ship) extends apply's type mapping with `data_flow → task`. That change is consistent with the current shape of apply and does not conflict with this proposal — the `data_flow → task` entry simply moves into the new emit module's type table once this proposal lands after 2026-04-12.

## Proposed change

### 1. Project-level principle

Add a new non-functional requirement to `spec/project.json`: **the spex binary invokes no external subprocesses at runtime.** All interaction with external tools (bead trackers, git) happens via input and output data artifacts. A user-owned adapter script — outside the spex binary — bridges those artifacts to the bead tracker.

### 2. Split apply into emit + ingest with an external adapter

The current apply module (module 5) mixes three kinds of work, two of which are pure Go over local files and one of which is the subprocess coupling:

- **Decide** — topological sort within type levels, parent bead-ID resolution from `.bead-map.json`, priority propagation through `project.json`, idempotency label construction, content-file path resolution. Pure.
- **Do** — shell out to `br` to label, create, close, and tag beads. Subprocess.
- **Record** — insert, update, delete records in `.bead-map.json`; save the new merkle snapshot to `spec/.snapshot.json`. Pure.

The "do" step leaves the spex binary. The "decide" and "record" steps become two new spex modules.

**emit** (new module). Command: `spex emit --proposal <ref> --git-head <sha>`. Pure function over the impact report (stdin or `--impact`), `.bead-map.json`, the spec directory, and the caller-supplied git HEAD. Emits `changeset.json` to stdout — an ordered, tool-agnostic list of operations (label, create, close, tag) that an external adapter consumes. Inherits from apply: topological ordering within type levels (`2026-03-20-spec-graph-bead-deps.md`), priority propagation, parent-resolution from the mapping store, idempotency labeling (`spex:<record-id>`), and the identity-hash flow established in `2026-04-06-identity-hash-ids.md`.

**ingest** (new module). Command: `spex ingest --changeset changeset.json --receipts receipts.json`. Pure function over the two artifacts. Updates `.bead-map.json` (insert new records, update modified, delete removed) and writes the new merkle snapshot. Handles partial receipts: if the adapter stopped mid-run, ingest applies whatever succeeded; unfinished operations resurface on the next `spex emit` through the idempotency path.

**changeset.json** is versioned (`"version": 1`). Operations appear in the exact order the adapter must execute. Forward references in parent and dependency fields use three shapes: `{"ref":"bead","bead_id":"<id>"}` for existing beads, `{"ref":"op","op_id":"<id>"}` for the result of another create op in this changeset, `{"ref":"spec_node","spec_node_id":"<id>"}` for lookups into the mapping store at exec time. Each `create` op carries an `idempotency.label` the adapter matches against before creating so re-runs are safe. A `git_head` field carries the SHA the caller passed to `spex emit`.

**Impact → emit dep-flow contract.** The impact report carries spec-graph dependencies at the spec_node_id level, not pre-resolved bead IDs. The `DepBeadIDs` field on `impact.Action` is retired; impact emits `DepSpecNodeIDs []string` per create action, populated from component `uses` (direct) and transitive `requires_module` edges in the spec graph. Emit's Resolver walks each create's `DepSpecNodeIDs` and classifies each dep spec_node_id into one of the three `changeset.json` ref shapes: `ref:op` when another create in the batch targets that spec_node_id (the common same-run case — this is what fixes the broken-dep-graph bug noted in the Context section); `ref:bead` when the mapping store already has an open bead for it (unchanged upstream dep); `ref:spec_node` as the adapter-time fallback. Closed existing beads are dropped (dep is already satisfied). This removes impact's dependence on live `bead_status` for dep resolution — the cleanup-bead gate in §3 remains the only consumer of `bead_status`.

**receipts.json** is versioned (`"version": 1`). Per-op status is `ok` | `skipped` | `error`; top-level status is `complete` | `partial`. Adapters record `was_existing: true/false` on each create op so ingest distinguishes a newly created bead from an idempotent re-match.

**Module dependency graph**: emit `requires_module` [impact (4), map (9)]. ingest `requires_module` [emit, map (9), merkle (3)].

### 3. Wire bead-state as an input — fix the latent cleanup-bead gap

`spex impact` gains an optional `--beads <file>` flag. The file carries the JSON array produced by `br list --json` (or any tracker whose output conforms to the same shape). Impact enriches each mapping record with its live status before passing to the ActionClassifier (component 3). The cleanup-bead gate at `impact/action_classifier.go:114` then fires correctly: when a spec node is removed and its bead is closed, a task bead titled "Code cleanup: `<module>/<node>`" is generated to track deletion of orphaned source.

`impact/bead_reader.go` is rewritten as a pure function over input JSON — no subprocess. It becomes a parser for the bead tracker's `list --json` output, taking `[]byte` or `io.Reader` and producing `[]BeadSpec`. The existing tests in `impact/bead_reader_test.go` already rely on a stub binary; they are rewritten to exercise the parser directly against canned JSON fixtures.

### 4. Rework spex log to read bead data from stdin

`spex log --proposal <ref>` reads `br list --json` output from stdin. The HistoryViewer (module 6 proposal, component 1) owns grouping, proposal-file resolution, and rendering; the bead tracker owns source of truth. Usage:

```
br list --json | spex log --proposal <ref>
```

`proposal/exec.go` is deleted. `proposal/history.go`'s `CLIBeadLister` type is deleted. The viewer accepts parsed `[]BeadRecord` directly.

### 5. Replace the git subprocess with a caller-supplied argument

`spex emit --git-head <sha>` requires the caller to pass the git HEAD SHA. The value is embedded in `changeset.json` and used by the adapter to build `commit:<HEAD>` labels on obsoleted beads. Callers supply `$(git rev-parse HEAD)` from their shell. The spex binary never invokes git.

### 6. Ship a reference adapter script

Add `scripts/apply-br.sh` — a bash + jq reference adapter for br users. It reads `changeset.json`, maintains the `op_id → bead_id` substitution table for forward references, looks up idempotency labels before creating, runs the five br subcommands the current apply runs (`create`, `list --json`, `show --json`, `update`, `close`), and writes `receipts.json`. Marked clearly as a reference implementation — "vet for your own use before production." Users of other trackers (bd, GitHub Issues, Jira) can author adapters against the `changeset.json` contract without changes to spex.

An integration test `scripts/apply-br_test.sh` exercises the adapter end-to-end against a real `br` sandbox; it is gated on `br` being present and lives outside the Go test surface.

### 7. Affected spec nodes

**New modules**:

- `spec/emit/module.json` + markdown leaves (arch, impl, flow, test). Components: ChangesetBuilder, Resolver (parent, dep, priority), TopologicalSorter, IdempotencyLabeler, EmitCommand. Requirements cover: topological ordering within type levels, priority propagation from project requirements through modules and components, parent bead-ID resolution from the mapping store, forward-reference encoding for in-batch creates, idempotency label construction, deterministic changeset schema, tool-agnostic operation vocabulary, inclusion of `git_head` from the caller-supplied flag.

- `spec/ingest/module.json` + markdown leaves. Components: Reconciler, SnapshotSaver, IngestCommand. Requirements cover: mapping record upsert, update, delete from per-op outcomes; partial-receipt tolerance; re-run idempotency; merkle snapshot save; no external subprocess invocation.

**Deleted**:

- `spec/apply/` — entire module (13 files). The apply module is superseded by emit + ingest.
- `spec/impact/arch_bead_reader.md`, `spec/impact/impl_bead_reading.md` — BeadReader (impact component 3) is repurposed from a shell-out pattern to a pure parser; the existing content files are replaced.
- `spec/impact/impl_dependency_resolution.md` — the impact-time dep-to-bead-ID resolution is retired. Emit owns dep resolution via the three-shape ref scheme; impact only collects `DepSpecNodeIDs` from the spec graph. The former file's "limitation" paragraph that claimed topological sort handled same-run deps was factually wrong and drove the broken-dep-graph bug noted in the Context section.

**Modified**:

- `spec/project.json` — rename module 5 from `apply` to `emit`; add new module `ingest`; add a new non-functional project requirement stating that the spex binary invokes no external subprocesses at runtime; update the module dependency edges (emit requires impact + map; ingest requires emit + map + merkle).
- `spec/impact/module.json` — rewrite BeadReader (component 3) description to reflect pure-function input parsing; add a requirement for `--beads` input ingestion; update ImpactCommand and ActionClassifier descriptions to note that bead status is sourced from enriched mapping records; retire the dep-to-bead-ID resolution requirement (req 7 from `2026-03-20-spec-graph-bead-deps.md`) and replace it with "emit `DepSpecNodeIDs` per create action" — resolution moves to emit.
- `spec/impact/arch_action_classifier.md` — remove the "Spec-Graph Dependency Resolution" section; the Action struct drops `DepBeadIDs` in favor of `DepSpecNodeIDs`.
- `spec/impact/arch_bead_reader.md`, `spec/impact/impl_bead_reading.md` — content rewritten for the pure-parser role.
- `spec/proposal/arch_history_viewer.md` — update to describe stdin input; remove references to the CLIBeadLister exec path.

**Unaffected by this proposal but noted**: the `data_flow → task` mapping from proposal `2026-04-12-data-flow-contract-layer.md` lands in emit's type mapping (instead of apply's BeadCreator) when 2026-04-12 ships before this proposal. A single entry in the type table moves.

## Impact expectation

**Prerequisites**: This proposal is strictly deferred until (a) the current 16 open beads ship — including `spexmachina-tjs` (apply: ApplyCommand), which must close through real completion, not through supersession — and (b) proposal `2026-04-12-data-flow-contract-layer.md` lands. PR #110 carries 2026-04-12, and its body already sets the 16-bead precondition. Landing 2026-04-12 first means its one touch on apply (the `data_flow → task` mapping) integrates cleanly, and this proposal then carries that mapping forward into emit's type table as part of the spec move.

**New beads** (generated by `spex apply` against the spec changes described above):

- emit module: one `epic` for the module; one `feature` per component (ChangesetBuilder, Resolver, TopologicalSorter, IdempotencyLabeler, EmitCommand). Multi-component test_sections become `task` beads; single-component test_sections are bundled with their component's feature bead per the rule established in `2026-04-12-data-flow-contract-layer.md`.
- ingest module: one `epic`; one `feature` per component (Reconciler, SnapshotSaver, IngestCommand). Same test_section coupling rules.
- adapters module: one `epic`; one `feature` for the BrReferenceAdapter component, carrying `scripts/apply-br.sh` and its br-gated integration test.
- impact: one `feature` bead for the BeadReader rewrite plus the `--beads` flag wiring in the ImpactCommand component.
- proposal: one `feature` bead for the HistoryViewer stdin rework.
- **Code cleanup beads** generated by the ActionClassifier for each removed apply node whose bead is closed by the time this proposal runs. Current apply has 4 components (BeadCreator, BeadCloser, SnapshotSaver, ApplyCommand — ProposalTagger was removed earlier on main), one multi-component test_section (Bead action tests, describing BeadCreator + BeadCloser), and one data_flow (Apply flow). Per the ActionClassifier gating rules on removed+closed nodes, that produces 4 component feature cleanup beads + 1 test_section task cleanup bead + 1 data_flow task cleanup bead = **6 cleanup beads** total, each carrying the work of deleting the corresponding Go source under `apply/` and the entrypoint in `cmd/spex/apply.go`.

**Modified spec nodes**:

- `spec/project.json` — module table (rename 5; add new module), new non-functional requirement, module dependency edges.
- `spec/impact/module.json` — BeadReader component, new `--beads` input requirement, ImpactCommand and ActionClassifier descriptions.
- `spec/impact/arch_bead_reader.md`, `spec/impact/impl_bead_reading.md` — content rewritten.
- `spec/proposal/arch_history_viewer.md` — stdin input.

**Closed beads**: All apply-module beads (per the mapping store at apply time) are obsoleted and closed via `spex:obsolete` + `commit:<HEAD>` labels. Their mapping records are deleted per the existing behavior at `apply/bead_closer.go:57` (`ChangeType == "removed"`). The cleanup task beads generated alongside them carry the source-deletion work.

**Skill changes (not tracked as beads)**: None. The implement skill already uses `bin/spex map context <record_id>` to resolve spec files deterministically; the new emit and ingest modules conform to that contract. Adapter-script authors using trackers other than br consult the `changeset.json` schema, which is part of the emit module spec.

**Migration**: No data migration. The `.bead-map.json` format is unchanged — `spec_node_id`, `bead_id`, `module`, `component`, `spec_hash`, `content_file`, `bead_type` fields survive verbatim. `bead_type` values (`epic`, `feature`, `task`) remain valid. Once the `--beads` input ships, mapping records get populated with `bead_status` on first write through ingest; records without it continue to read as empty-status, which the existing classifier handles. Existing bead IDs in the tracker are untouched.

**Estimated scope**: 5–6 implementation sessions following the beads generated from this proposal.

- Session 1: emit — types, ChangesetBuilder
- Session 2: emit — Resolver, TopologicalSorter, IdempotencyLabeler, EmitCommand CLI wiring
- Session 3: ingest — Reconciler, SnapshotSaver, IngestCommand CLI wiring
- Session 4: impact — BeadReader rewrite and `--beads` wiring; proposal — HistoryViewer stdin rework; reference adapter `scripts/apply-br.sh` plus its integration test
- Session 5: cleanup beads — delete `apply/`, `cmd/spex/apply.go`, `proposal/exec.go`
- Session 6: README and CLAUDE.md updates; dogfood end-to-end pipeline run against a small proposal to prove the new flow
