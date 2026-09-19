# Change Proposal: Full review 2026-09-19

## Context

The first full-corpus `/spec-review all` ran over 300 nodes in 14 modules on 2026-09-19, with both gates green and every deterministic lens clean. It produced 38 findings; an independent verifier held 37 and rejected one (F17, a fold rule read against a journal-sourced biography — not a contradiction). Five open questions were discussed and decided; the findings, verdicts and decisions are the run's record under `.spex/runs/review/`. This proposal is that record turned into one epic. Nothing here is new design; every item is either a contradiction the spec carries against itself or the structural change needed to make a declared dependency graph tell the truth.

Four things the audit found are larger than a correction:

**The dependency graph cannot hold a reliance every pipeline module has.** The diff command in merkle (`c8b958ec310d`), the map command in map (`08909d62930b`), the plan command in plan (`92ae9dab6d6d`) and `spex register` in proposal (`21bc124a46c0`) all run the lifecycle pre-flight, `ProjectResolver` (`a9aa93774cc2`), and take its not-a-spex-project exit code. None declares `requires_module` on lifecycle. Two of them cannot: lifecycle requires merkle, for the empty-tree snapshot `spex init` seeds, and map, for the journal it creates and doctor reads, so merkle → lifecycle and map → lifecycle would each close a cycle the DAG check refuses. `2026-08-20-project-lifecycle` placed the resolver in lifecycle because "no existing module can take them"; that was true of the commands and remains true, but the resolver is a foundation the whole pipeline stands on, and it sits above the modules that call it.

**The render module's requirement and its leaves disagree on what reaches which output.** `Composable output` (`8d441659a190`) says every node type the reader surfaces reaches all three outputs. `SpecReader` (`7d1150c19724`), `RenderCommand` (`c56eefd05f42`) and `DOTRenderer` (`45331bdc0bd0`) say test sections reach the JSON output alone, and `MarkdownRenderer` (`b4b4eba6b551`) has no place for them — while the same leaves say a profile-declared content-bearing type gets its own markdown section. Excluding the built-in test section is a per-type special case in a renderer that "branches on no type name".

**`spex upgrade` decides none of its mode intersections.** The command leaf (`ab15d9e23c4a`) declares four modes and decides only that `--check` equals `--dry-run` and `--force` is unknown. Read against the installer (authorised for this one question): the script's rollback branch runs before it reads `--check`, so `spex upgrade --check --rollback` restores the backup instead of reporting, and `--rollback --version vX` silently ignores the pin. Faber's and portitor's installers carry the same ordering; that is theirs to fix. Spex also lacks the root `--version` / `-v` flag both siblings offer beside their `version` subcommand, and its own spec pins the absence in a test leaf with no reason given.

**The refresh absorbable set is stated four ways.** Requirement `Refresh mode for impl_only drift` (`e68653819f38`) declares every default-profile type absorbable in both directions, and `IngestCommand`'s leaf and the journal-line schema leaf agree. `RefreshHandler`'s description (`f9033352c13f`), a paragraph and a wiring step in its own leaf, the ingest flow (`6fd1f0cbb76c`) and a rationale in `Refresh mode tests` (`a483524c406c`) still carry the retired component-removal-only table and the pairing gate the requirement removed.

The rest are counts, stale claims, a missing field, a wrong fixture name, a dangling cross-reference, two mutually inconsistent test harness sketches, and module descriptions that drifted apart between `project.json` and `module.json`.

## Proposed change

### A. A new module: `state`

The state-directory contract and its one reader move out of lifecycle into a module below every pipeline module. `state` requires `schema` only.

- **Moved in:** component `ProjectResolver`; module requirements `Project state directory` (`44b8c5de0d37`) and `Uninitialized project is an error` (`055444aee4c7`), both deriving from `Project initialization and health` (`2574f0fdcef2`) as now. The resolver implements both. Moving a module-scoped node changes its identity hash, so each reaches the diff as removed plus added; the resolver's task history stays reachable through the journal.
- **Added:** test section `State resolver tests`, describing the resolver alone (no task of its own): the pre-flight refusals driven through `spex diff` and `spex map list` — no `.spex/`, a missing snapshot, a snapshot that is not JSON, a journal line failing the schema — asserting the not-a-spex-project code and the command each error names.
- **One condition that makes the split hold:** the resolver judges the snapshot as a file — present, parseable JSON — and never decodes it as a merkle tree. A snapshot that parses but whose tree is inconsistent fails later, in merkle's own loader. The resolver leaf says so; `IngestCommand`'s leaf (`db90eb607bcb`), which today says the pre-flight "loads the snapshot", says the same; `DiffCommand`'s exit-code list (F14) splits accordingly: exit 1 covers a corrupted `--snapshot` override and a tree merkle cannot load, the pre-flight code covers a resolved snapshot that does not parse.
- **Lifecycle keeps** `InitCommand`, `DoctorCommand`, `Initialize a project`, `Diagnose project health`, both apis and `Lifecycle command tests`. Its `uses` edges on the resolver and the two `depends_on` edges on `Project state directory` are dropped (both are module-local); the reliance travels through `requires_module` on `state`. Init still seeds the empty tree through merkle's encoder, so the one-implementation rule for the snapshot format survives.
- **Edges:** `state` → `schema`; `lifecycle`, `merkle`, `map`, `plan`, `proposal`, `ingest` → `state`. Every link to `a9aa93774cc2` in the corpus (eight leaves) is repointed at the new resolver id.

### B. Render: declared projections, and test sections in the collated document

- `Composable output` (`8d441659a190`) is reworded: every node type the reader surfaces reaches the JSON graph; every content-bearing type reaches the markdown document; DOT draws the dependency graph its renderer declares; an omission is a declared rule of the format, never a silent drop; an api reaches every output through its JSON fields alone.
- `Render markdown` (`8828685278e9`) drops the retired "implementation" section from its structure sentence and names the actual order: requirements, external surface, architecture, data flows, tests.
- `MarkdownRenderer` gains a `### Tests` section per module, each test leaf inlined exactly as architecture and data flows are; a module declaring no test sections emits no heading. `Render command tests` (`6aad73fe1bc2`) S2 and S4 follow.
- `DOTRenderer` replaces "given no shape here" with the rule: DOT draws the nodes that carry dependency edges (`uses`, `implements`, `provided_by`, `preq_id`, `depends_on`, `requires_module`); a coverage edge is not a dependency, so `describes` and the type that owns it stay out; a profile-declared type is drawn only when it carries a reference field of a dependency kind.
- `SpecReader` and `RenderCommand` replace "test sections reach the JSON output alone" with the two rules above.

### C. Upgrade modes and the root version flag

- The check flag reports on every path, rollback included, and changes nothing: with `--rollback` it reports whether a backup exists and what version it would restore. `--rollback` ignores a version pin. Both rules land in `UpgradeCommand` (`ab15d9e23c4a`) and in delivery's `SelfUpdate` (`a1e437df5c01`), whose installer reads the check flag before its rollback branch; `Upgrade Command Tests` (`ccfae4f355b3`) and `Self-Update Tests` (`24a49bd622f7`) gain the cases; the command test that today combines all three flags asserts the report.
- `Version command` (`e15f238e9534`) gains the root's built-in `--version` / `-v` flag, printing the same four lines as `spex version`. `RootCommand` (`b6758cdfabc4`) lists it under Global Flags as cobra's built-in; `VersionCommand` (`bbdb70e6f9f7`) says the flag shares its printer; `Version Command Tests` (`1d4fca99754e`) flips its "not supported" edge case.

### D. Declared dependencies

- `delivery` → `merkle` and `validator`: `SpecGate` (`4153dbd38133`) relies on the diff exit-code contract and the validate report's notes.
- `cli` → `delivery`: `UpgradeCommand` drives the embedded installer. No `delivery` → `cli` edge: `ReleasePipeline`'s leaf (`649f5268a2b2`) is reworded to say ldflags stamp the three variables the binary's entry package declares, the same three the version command reads — the names are the entry package's, owned by neither module.
- Author declares nothing new: `ObligationReporter` (`b9e7b96f6aa7`) states its own fix-entry shape (`check`, `message`, `path`, `fix`) instead of citing `spex doctor`'s; `AuthorCommands` (`1f9fec7c42f6`) keeps its three exit codes under `Composable` and drops "with the values `spex plan` documents".

### E. The refresh absorbable set, one statement

Toward requirement `e68653819f38`, unchanged: `RefreshHandler`'s description states that an added or removed entry refuses unless the resolved profile declares its type absorbable in that direction (every declared type, both directions, under the default) and that meta and module leaves refuse unconditionally, and drops the unclosed-pairing refusal; `arch_refresh.md` loses the "`component` is removal-only" paragraph and wiring step 4; the ingest flow's refresh paragraph and the bootstrap-guard rationale in `Refresh mode tests` say the same as the requirement.

### F. Corrections carried as decided

Each with its exact replacement text in `.spex/runs/review/FINDINGS.tsv`:

| Finding | Node | Correction |
|---|---|---|
| F09 | `Bounded third-party CLI surface` (`293b27f73924`) | four `spex node` children, twenty-six api names |
| F10 | `RootCommand` (`b6758cdfabc4`) | fifteen of nineteen constructors declared |
| F11 | `TreeBuilder` (`dfe1467b7a4b`) | three content-bearing node types |
| F12 | `Diff and classification flow` (`45a52ae06265`) | no skip-the-load branch for an absent snapshot |
| F13 | `DiffEngine` (`cb262b280963`) | Interface compares against the init-seeded empty tree, never a missing baseline |
| F15 | `Task creation mapping flow` (`ef335db5e961`) | retarget: `br update` for the label, `br dep add` per missing dep |
| F16 | `Proposal lifecycle` (`1d33b4c6c36e`) | ProposalLog gains `date` |
| F18 | `Plan command tests` (`7f529376c087`) | malformed journal line is the pre-flight's code, not exit 1 |
| F19 | `Task matching tests` (`187f7044bd5f`) | cite `spec/map/arch_mapping_store.md` |
| F20 | `Br integration test` (`e62fbe481413`) | the PATH gate guards the integration loop only |
| F21 | `ModuleSchema` (`78883b84c32d`) | journal `node_type` membership is the profile's, checked at ingest's write boundary |
| F22, F25 | `Merkle command tests` (`49a61e0d5737`) | S7 rationale claims no per-module view; second S8 becomes S9 |
| F23, F24 | `Diff and classification tests` (`95a279cbdcbc`) | E2 claims no combined level; S14 names `COMP2_HASH` |
| F26 | `Snapshot tests` (`4ff940b743d7`) | the dangling "Sketched, that shape is:" |
| F36 | cli module (`430f8b037415`) | one description in both files, naming upgrade |
| F37 | schema module (`f03774b40bd5`) | one description in both files, naming the journal-line and task-state schemas, the profile and the identity hash |
| F38 | map module (`fb20a21b62f1`) | `project.json` takes `module.json`'s description |

## Impact expectation

Read off a dry run on a scratch copy: the `state` module declared, the three nodes removed from lifecycle and re-added under `state` with the new test section and its edges, the nine `requires_module` edges wired, the five requirement and component descriptions set, the dangling references retargeted, then `spex diff --json` against the copied snapshot: 27 changes (5 added, 19 modified, 3 removed) and 13 completeness errors, every one accounted for below.

### New nodes — state module

| Node | Type | Task |
|---|---|---|
| `state` | module | no |
| `Project state directory` | module requirement | no |
| `Uninitialized project is an error` | module requirement | no |
| `ProjectResolver` | component | yes |
| `State resolver tests` | test section, describes one | no |

### Removed nodes — lifecycle module

`ProjectResolver` (`a9aa93774cc2`), `Project state directory` (`44b8c5de0d37`), `Uninitialized project is an error` (`055444aee4c7`). The resolver's finished task makes `spex plan` mint a cleanup create for it; the removal is the old half of a move, so that task's work is the package move rather than a deletion, and it waits for the batch's last layer by the layer-edge rule. The removal-time name sweep reports nothing: the live `state` node of the same name covers every mention, and the diff's note says so.

### Modified nodes and the leaves they oblige

| Node | Hash | Task | Why |
|---|---|---|---|
| `Bounded third-party CLI surface` | `293b27f73924` | — | F09; obliges `RootCommand`, which changes for F10 and the version flag anyway |
| `Version command` | `e15f238e9534` | — | root flag; obliges `VersionCommand` |
| `Render markdown` | `8828685278e9` | — | F02; obliges `SpecReader`, `MarkdownRenderer`, `RenderCommand` |
| `Composable output` | `8d441659a190` | — | B; obliges `SpecReader`, `RenderCommand` |
| `Initialize a project`, `Diagnose project health` | `ac57844f66a6`, `1ddbd6e36681` | — | `depends_on` dropped; their leaves change anyway |
| `InitCommand`, `DoctorCommand` | `f64995aaeb56`, `3accd139a00d` | yes, yes | `uses` dropped, links repointed, the resolver described as `state`'s |
| `DiffCommand` | `c8b958ec310d` | yes | link repointed, F14 exit codes, pre-flight named as `state`'s |
| `SnapshotStore`, `MappingStore`, `IngestCommand` | `b2fcd9457a28`, `205e67ca4aad`, `db90eb607bcb` | yes ×3 | link repointed; `IngestCommand` also the file-level pre-flight sentence |
| `RefreshHandler` | `f9033352c13f` | yes | E: description, paragraph and wiring step |
| `Ingest flow` | `6fd1f0cbb76c` | yes | E |
| `Refresh mode tests`, `Lifecycle command tests`, `Snapshot tests` | `a483524c406c`, `7fb345b743f3`, `4ff940b743d7` | no | describe one component; E, the dropped `describes`, F26 |
| `MarkdownRenderer`, `DOTRenderer`, `SpecReader`, `RenderCommand` | `b4b4eba6b551`, `45331bdc0bd0`, `7d1150c19724`, `c56eefd05f42` | yes ×4 | B |
| `Render command tests` | `6aad73fe1bc2` | yes | describes four; S2, S4 |
| `UpgradeCommand`, `SelfUpdate`, `ReleasePipeline` | `ab15d9e23c4a`, `a1e437df5c01`, `649f5268a2b2` | yes ×3 | C, D |
| `RootCommand`, `VersionCommand` | `b6758cdfabc4`, `bbdb70e6f9f7` | yes ×2 | C, F10 |
| `Upgrade Command Tests`, `Self-Update Tests`, `Version Command Tests` | `ccfae4f355b3`, `24a49bd622f7`, `1d4fca99754e` | no | describe one component; C |
| `ObligationReporter`, `AuthorCommands` | `b9e7b96f6aa7`, `1f9fec7c42f6` | yes ×2 | D, wording only — absorb candidates for `/mint` |
| `TreeBuilder`, `DiffEngine`, `Diff and classification flow` | `dfe1467b7a4b`, `cb262b280963`, `45a52ae06265` | yes ×3 | F11–F13, wording only — absorb candidates |
| `Task creation mapping flow`, `Proposal lifecycle`, `ModuleSchema` | `ef335db5e961`, `1d33b4c6c36e`, `78883b84c32d` | yes ×3 | F15, F16, F21, wording only — absorb candidates |
| `Merkle command tests`, `Diff and classification tests`, `Task matching tests` | `49a61e0d5737`, `95a279cbdcbc`, `187f7044bd5f` | yes ×3 | describe two or more; F19, F22–F25, wording only — absorb candidates |
| `Plan command tests`, `Br integration test` | `7f529376c087`, `e62fbe481413` | no | describe one; F18, F20 |

### Meta-leaf accounting

The `requires_module` edges live in `project.json`, whose envelope leaf is inert: nine edges cost nothing. `RefreshHandler`'s description moves ingest's meta leaf, and no ingest requirement changes, so the dry run obliges every ingest component: `Reconciler`, `EventBuilder`, `InvariantChecker`, `JournalEncoder`, `SnapshotSaver` gain no content and are the epic's clearest absorb candidates, argued per `/mint`'s rules. The cli, lifecycle and render meta changes are covered by their modules' requirement changes. F37's `schema/module.json` description would oblige schema's six components the same way; F38 edits `project.json` only (`map/module.json` already holds the better text) and costs nothing. The `state` module's own meta leaf is new.

### The checks that can actually fail

`spex diff` on an uninitialised directory, on a state directory whose snapshot is not JSON, and on one whose snapshot parses but whose tree names a child no entry defines: the first two exit with the not-a-spex-project code naming `spex init` and `spex doctor`, the third exits 1 from merkle's loader — the split A introduces. `spex render --format markdown` over a module with two test sections carries a `### Tests` heading and both leaves' own `#` headings shifted to `####`; over a module with none it carries no such heading. `spex upgrade --check --rollback` against a target with a backup leaves both files byte-identical and exits 0. `spex --version` exits 0 with the four lines `spex version` prints. `spex validate` on a profile declaring `component` absorbable on removal only still refuses an added component under refresh, which is what proves E moved the policy into the profile and nowhere else.

### Scope

Mixed change, so a mint: roughly 26 component and flow tasks and 4 multi-component test-section tasks under one epic, plus one cleanup, of which about a third are wording-only corrections `/mint` may absorb. The code work is the `state` package and the call-site moves behind A, the markdown Tests section behind B, the root version flag and the installer's check-before-rollback behind C. Nothing is renamed; the only removals are the three moved nodes.

## Retired vocabulary

- `lifecycle/arch_project_resolver.md`
