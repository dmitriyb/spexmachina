# Change Proposal: Spec Contract Integrity and Declarative Arch Leaves

## How to run this document

**You are the orchestrator.** This work does not go through the bead pipeline. There are no beads, no
`/spec`, no `/propose`, no `spex emit`, no `scripts/apply-br.sh`. You hold the plan in this file, you
delegate every part to subagents, and you commit each chunk as it completes. **`go-expert` is the only
skill anyone may invoke**, and only for Go — you never invoke it yourself, because you write no code;
the subagent doing the work invokes it as its first action, in parts 1a–1e and 7 only, for implement,
fix and review alike.

**Every part runs implement → gate → review → fix.** Implement is a subagent. **Review is always a
fresh subagent that did not implement the part** — you are the orchestrator and cannot review your own
delegation. Fix is a subagent given the findings verbatim plus the same briefing the implementer got.
A part is done when a review pass returns no findings.

**Parallelism is by disjoint write sets, never by convenience.** Disjointness is re-derived per wave,
never assumed: a part that only "adds a checker" still edits the chain in `cmd/spex/validate.go`, and
a part that only "adds a property" still rewrites `schema/module.schema.json`. Before spawning a
fan-out group, list each part's write set in `commit-plan.md` and confirm the intersection is empty.

**Your state is `commit-plan.md` plus `commit-plan.d/`**, both at the repo root and both added to
`.git/info/exclude` before wave 1. `commit-plan.md` records intent; `git log` records fact. Every part
gets an entry **before** its subagent is spawned:

```
### 6e — migrate `ingest`
status: IN_PROGRESS            # TODO → IN_PROGRESS → DONE → QUARANTINED
writes:  spec/ingest/
revert:  git checkout HEAD -- spec/ingest/ && git clean -fd spec/ingest/
```

`writes` and `revert` are what a fresh session needs to undo an interrupted part; without them a
`TODO` part and a half-applied part look identical on disk. Per-part ladder records go in
`commit-plan.d/<part>.md`, **never** in `commit-plan.md` — eleven W6 parts would hold one file at once
and last-writer-wins would destroy ten records.

**Run every gate through `bash -c`.** The session shell is fish, which rejects `$?`, `<<<` and
`< <(…)` — all of which appear in the gates and in `link-check.sh`.

### Pre-flight

```bash
git fetch origin main
git rev-parse --abbrev-ref HEAD                 # must NOT be main
git rev-list --count HEAD..origin/main          # must be 0
git status --porcelain                          # must be empty
go build -o bin/ ./cmd/spex/                    # bin/ is gitignored; it is never there
bin/spex validate                               # valid true, error_count 0, warning_count 0
bin/spex diff                                   # "no changes"
command -v jq
```

On `main`, branch — portitor rejects pushes to `main`. Dirty tree: **stop**; never `git stash` and
proceed, because the gates compare against `spec/.snapshot.json` and stashed spec edits produce a diff
belonging to nobody. `validate` red or `diff` not "no changes": **stop**, the baseline is broken.

**Pre-flight is for a fresh run only.** A resumed run uses the check under "If you are resuming"; a
mid-migration tree can never have an empty `git status`.

### The wave plan

| wave | parts | concurrency | blocked by |
|---|---|---|---|
| **W1** | **1a** api node type · **1b** typed-link resolver + removal-time name check + API name uniqueness · **1c** refresh type filter · **1d** `render --slim` + api support + the `test_section` omission + the `dot.go` node-ID fix · **1e** delete the orphan checks, add the `content` constraint · **1f** `scripts/link-check.sh` + the four mechanical checks · **1g** CI workflow · **1h** retire the `OrphanDetector` node | **1a alone**, then **1e alone**, then **1h alone**, then 1b/1c/1d/1f/1g in parallel. 1a and 1e both edit `schema/module.schema.json`; 1b and 1e both edit the checker chain in `cmd/spex/validate.go`; 1h depends on 1e. | — |
| **W1½** | **1i** delete milestones, `test_plan.scenarios`, requirement `61f3238dfd74`, and `spec/test_end_to_end_pipeline.md` after relocating its diagram · **1j** the stack non-functional project requirement | 1i then 1j — both touch `spec/project.json` | W1 |
| **W2** | **2a** `/spec` · **2b** `/spec-review` · **2c** `/propose` · **2d** CLAUDE.md `map context` field list | all four in parallel — four separate files | W1 |
| **W3** | **3** debt sweep | single part | W1 |
| **W4** | **4** declare 15 apis across **nine** modules | single part | W1, W3 |
| **W5** | **5** project requirement `9120788210c9` — 7 arch leaves across 5 modules | single part | W4 |
| **W6** | **6a** adapters · **6b** cli · **6c** emit · **6d** impact · **6e** ingest · **6f** map · **6g** merkle · **6h** proposal · **6i** render · **6j** schema · **6k-1**/**6k-2** validator | **all concurrent** — each writes only `spec/<MOD>/`. 6k-2 waits on 6k-1. | W1, W5 |
| **W7** | **7** cutover | single part | W6 |
| **W8** | **8** refresh and final commit | you, not a subagent | W7 |

**W2 and W3 may run concurrently with each other. W1 may not run concurrently with either** — their
gates rebuild `bin/spex` from the same working tree 1b–1g are editing, so a gate landing mid-write
builds a broken tree and reports a failure belonging to nobody. W3 must precede W6, because the sweep
rewrites `spec/map/flow_bead_mapping.md` and `spec/proposal/flow_proposal_lifecycle.md`, which 6f and
6h also touch.

**W4 precedes W6** not because links cross modules — all 205 declared links are module-local — but
because declaring `apis` moves each module's `meta` hash, and doing that inside a W6 part would
compound two meta obligations in one gate.

The eleven module directories are `adapters, cli, emit, impact, ingest, map, merkle, proposal, render,
schema, validator`. **The spec directory is `spec/map/`; `mapping/` is the Go package.** `validator` is
split because it is 48 ladder verdicts across 11 leaves against 14 for `cli`: **6k-1** takes
`IDValidator`, `RefValidator`, `DAGValidator`, `ContentResolver` and their impl leaves; **6k-2** takes
the rest and is the only one that edits `spec/validator/module.json`.

### The per-part protocol

1. **Implement.** Spawn one subagent with the briefing packet below. It edits files, reports what it
   did, and never runs `git` in write mode.
2. **Gate.** You run it, per "Gates". Serial waves gate `spec/` directly. **W6 gates an isolated
   tree** — see Gates. On failure, hand the verbatim output to a fix subagent; do not fix it yourself
   and do not restart the implementer.
3. **Review.** Spawn a **fresh** subagent per "Review criteria" and "Reviewer independence".
4. **Fix.** Spawn a fix subagent with the findings verbatim **and the same part specification and
   reference sections the implementer received** — findings alone are not a briefing. Re-gate, then
   review again with another fresh subagent. **Two fix rounds maximum.** A third failing review
   QUARANTINES the part: record the findings, revert only that part's files
   (`git checkout HEAD -- <its writes>`), let every sibling in the wave finish, and stop before the
   wave gate. Never carry a quarantined part into W8 — refresh baselines whatever is on disk.
5. Mark the part `DONE` in `commit-plan.md` with the gate output pasted under it, then commit its
   chunk. The commit makes the part durable; the marker only makes it findable.

### The briefing packet

Every subagent — implement, review or fix — gets exactly this. Nothing is implied and nothing is
"obvious from the repo".

**Common preamble, verbatim in every packet:**

> You are one part of a larger migration. Other subagents are editing other files right now.
> - Touch only the files this packet names. Never run `git` in write mode — no `checkout`, `stash`,
>   `reset`, `add`, `commit`, `clean`. `git show`, `git diff` and `git log` are permitted.
> - Never run `bin/spex ingest`, `bin/spex emit` or `scripts/apply-br.sh`. `validate`, `diff`,
>   `render` and `hash-id` are permitted. Never create, close or edit a bead; there is no bead
>   lifecycle in this work.
> - Never change a `name`, `title` or `content` field of any node that survives your part, and never
>   invent an `id`. Renames are what this migration is built to avoid.
> - If you run short of context, stop and report where you got to. Do not compact and continue.
> - Report: every file you touched, and anything you could not do.

**Per-part reference sections to include:**

| part | packet |
|---|---|
| 1a | §1.1 in full · files: `schema/schema.go`, `schema/module.schema.json`, `merkle/tree_builder.go`, `merkle/impact_classifier.go`, `cmd/spex/hashid.go` · `go-expert` |
| 1b | §1.2 **and §1.4 in full** — 1b's own text omits that `README.md`, `docs/` and `skills/` are outside the gate · files: `validator/`, `cmd/spex/validate.go` · `go-expert` |
| 1c | §1.5 in full · file: `ingest/refresh.go` · `go-expert` |
| 1d | §1.3 in full · files: `render/*.go`, `cmd/spex/render.go` · `go-expert` |
| 1e | §1.8's orphan paragraph · files: `validator/orphan_detector*.go`, `validator/testdata/orphan_*`, `cmd/spex/validate.go`, `cmd/spex/validate_test.go`, `schema/module.schema.json` · `go-expert` |
| 1f | the four scripts under "The mechanical checks" · files: `scripts/` |
| 1g | §1.11 · file: `.github/workflows/spec.yml` |
| 1h | §1.8 · files: `spec/validator/module.json`, `spec/validator/arch_orphan_detector.md`, `.bead-map.json` |
| 1i, 1j | §1.9, §1.10 in full · files as the part names |
| 2a–2c | §W2 · **§1.1, §1.2, §1.6, §1.7, §1.8, §1.12 in full** · file: that one skill directory |
| 2d | §W2's last sentence · file: `CLAUDE.md` |
| 3 | §W3 in full including both greps and the disposition table · files: `spec/**/*.md` except `spec/proposals/`, plus `README.md` |
| 4 | §W4 and §1.1 in full · files: the nine `module.json` plus the arch leaves it names |
| 5 | §W5 · files: `spec/project.json` and the seven arch leaves |
| 6a–6k | §W6 in full · **§1.6 AND §1.7** · **§"Segment 2 — diagrams and links" in full** · §1.2 · the module name · files: `spec/<MOD>/` only |
| 7 | §W7 in full, in the order it names · `go-expert` |

### Reviewer independence

The reviewer sees the implementer's *output*; it must never see the implementer's *case for it*, or
yours. **Do not put in a review packet:** the implement subagent's report, summary or rationale (the
one exception is the W6 ladder record, which criterion 4 requires — label it *"this is the
implementer's claim about its own work; it is the object of your audit, not evidence for it"*); your
assessment in any form, including "this looks clean" or "minor part" — give gate output raw, never a
verdict; the identity of the implement or fix subagent; how many fix rounds have been used or anything
about schedule or remaining context; an expected finding count.

On rounds 2 and 3 the packet **does** carry prior findings, in a separated block: *"Complete your own
independent pass against all eight criteria FIRST and write those findings down. Only then check each
item below and mark it CLOSED or STILL-OPEN. Label every finding NEW or CARRIED."*

A reviewer that edits any file has broken its remit: discard its report, revert its edits, re-review
with a new subagent. That round does not count against the cap.

### Review criteria

Reports findings, never edits. Each finding names a file, a line or heading, and the criterion.
"No findings" is a permitted and expected outcome.

1. **Scope.** Every hunk falls inside the part's write set. The delta is `git diff HEAD -- <writes>`;
   because each completed chunk is committed, that is the part's work alone.
2. **No renames.** `no-rename-check.sh` exits 0, and no `name`/`title`/`content` value changed for any
   surviving node. Removing a whole `impl_sections` entry is not a rename; editing one is.
3. **No unexpected nodes.** Only `added/api` in W4; only `removed/impl_section` in W6; only
   `removed/component` and `removed/requirement` in 1h and 1i.
4. **Ladder fidelity** (W6). `ladder-check.sh` exits 0. Then spot-check **five** verdicts against
   `git show origin/main:<impl leaf>` — the two longest sections, one `arm 2`, one `arm 6`, one
   `arm 4`/`arm 5`. An `arm 2` requires you to point at the arch sentence that already said it, in the
   leaf's **current** text. Any recorded arm 0 appears in your report as an unresolved finding.
5. **No lost structure** (W6). `heading-check.sh` exits 0, or every lost `##` heading is named in the
   implementer's report with a reason.
6. **Link obligation** (W6). `link-check.sh` and `link-spread.sh` exit 0, and each link sits in the
   sentence that discusses the target.
7. **Contract prose.** Grep the rewritten leaves for `func `, `struct`, `interface`,
   `ctx context.Context`, `err != nil`, `map[`, `[]byte`, ```` ```go ````. Every hit is a finding
   unless the implementer cites §1.7 and names the requirement whose description carries the
   algorithm. Then: two `##` sections asserting the same thing about the same subject is a finding.
8. **Spec-vs-code contradictions** noticed in passing are findings in your report. You do not fix
   them, do not file them anywhere, and do not edit `commit-plan.md`.

**The pre-image is available.** `origin/main` never moves during the run, so
`git show origin/main:<path>` returns the original bytes of any file, including deleted ones. You are
expected to read them.

### Gates

```bash
go build -o bin/ ./cmd/spex/
GATE_DIR=${GATE_DIR:-spec}

bin/spex --spec-dir "$GATE_DIR" validate | jq -e '.valid == true and .warning_count == 0'

bin/spex --spec-dir "$GATE_DIR" diff --json > /tmp/diff.json; rc=$?
if [ ! -s /tmp/diff.json ]; then
  echo "GATE HARD FAIL: diff exited $rc and wrote no JSON — the tree does not build"; exit 1
fi

jq -e '.errors == []' /tmp/diff.json

jq -e '([.changes[] | "\(.type)/\(.node_type)"] | unique)
  - ["removed/impl_section","modified/impl_section","removed/requirement","removed/component",
     "modified/component","modified/data_flow","modified/test_section",
     "modified/requirement","modified/meta","added/api"]
  | length == 0' /tmp/diff.json

scripts/no-rename-check.sh
```

W6 parts add, scoped to their module:
`link-check.sh "$GATE_DIR" /tmp/diff.json "$GATE_DIR/<MOD>"`, `heading-check.sh "$GATE_DIR/<MOD>"`,
`ladder-check.sh "$GATE_DIR/<MOD>" commit-plan.d/<part>.md`, `link-spread.sh "$GATE_DIR/<MOD>"`.

Go-only or `skills/`-only parts gate on `go build ./...`, `go vet ./...`, `go test ./...` plus their
acceptance lines. Parts touching only `.github/`, `CLAUDE.md` or `README.md` record
`NO GATE — documentation-only part`; for 1g, run the workflow's own steps locally in declared order
and paste the transcript.

**`rc` is checked, not printed.** `diff` exits 0 clean, 2 on completeness errors, 1 when the tree does
not build — and on exit 1 it writes nothing to stdout, so an unchecked `jq -e` fails with an opaque
parse error naming no file.

**`modified/impl_section` stays on the allowlist until W7.** W3 rewrites three impl leaves and,
because refresh is deferred to W8, they remain `modified` until their owning W6 part deletes them.

**`warning_count == 0` presupposes 1e.** Before it lands, the first W6 part to delete impl leaves
fires one orphan warning per component in that module — four for `schema`, measured. That is why W6 is
blocked on W1.

**`link-check.sh` runs in W6 and W8 only.** It reads `touched` from the whole accumulated diff, so run
earlier it demands links from every component W3, W4 and W5 touch — measured, 52 `MISSING_LINK` across
18 leaves at the W3 boundary, four waves before links are authored.

**W6 gates an isolated tree, never `spec/`.** Eleven concurrent agents mean a global gate reports
siblings' half-finished work as this part's failure; worse, a module caught between `rm impl_*.md` and
its `module.json` edit makes `diff` abort before emitting JSON.

```bash
cp -a spec /tmp/spex-base                      # ONCE, before spawning any W6 subagent
# per part:
MOD=<module>
rm -rf /tmp/spex-gate && cp -a /tmp/spex-base /tmp/spex-gate
rm -rf "/tmp/spex-gate/$MOD" && cp -a "spec/$MOD" "/tmp/spex-gate/$MOD"
GATE_DIR=/tmp/spex-gate
```

`cp -a` carries `spec/.snapshot.json`, so the isolated tree diffs against the same baseline. **Never
refresh `/tmp/spex-base` mid-wave** — a stale baseline is the point. When all eleven are `DONE`, run
the gate once with `GATE_DIR=spec`; that global run is the wave's acceptance. If it fails while all
eleven isolated runs passed, two parts edited the same file — find it with `git diff --name-only`.

**`errors: []` is a part-completion property, not a mid-part one.** Removing a `module.json`'s
`impl_sections` array and inserting that module's links cannot be atomic; between them the diff
carries `module <MOD> meta changed but component <C> content leaf unchanged`. Measured green at the
W3, W4, W5, every W6 and the W7 boundary; red at every mid-part instant.

**Never run `ingest --mode normal`, `spex emit` or `scripts/apply-br.sh`.**

### The mechanical checks

Four scripts, built in 1f, that turn four unfalsifiable review bullets into exit codes. Each catches a
failure that otherwise passes `validate`, `diff` and `link-check` while doing none of the work.

- **`no-rename-check.sh`** — every `id` present at `origin/main` and now carries the same
  `name`/`title`. Catches the one rename the allowlist cannot see: a `title` edit leaving `id` alone
  produces `modified/requirement`, which is allowlisted, and desyncs the identity hash.
- **`heading-check.sh <spec/MOD>`** — P2's frozen `##` list may grow, never shrink. Catches a
  wholesale arch-leaf rewrite, which is otherwise just `modified/component`.
- **`ladder-check.sh <spec/MOD> <record>`** — every `##` section of every deleted impl leaf, plus each
  leaf's `[preamble]`, has a verdict line. The only thing tying the implementer's record to leaves
  that no longer exist.
- **`link-spread.sh <spec/MOD>`** — fails a leaf that puts more than half its links in its last 20% of
  lines, or that grows a `## References`/`## Links`/`## See also`/`## Related` heading. **Appending
  every link to a trailing block satisfies `link-check.sh`, changes the leaf's bytes, satisfies the
  completeness checker, and represents zero migration work** — this is the cheapest way to pass every
  other gate while doing nothing.

`link-check.sh` takes an optional trailing module filter so the W6 part gate can scope it; with no
third argument its behaviour is byte-identical to the two-argument form.

### Committing

**Commit each chunk when its work is `DONE` and reviewed. Do not defer commits to the end.** Thirty-five
files are required by more than one chunk — every `spec/<MOD>/module.json` carries W4's `apis` array
and W6's `impl_sections` removal; `spec/project.json` carries W3's and W5's edits — and three impl
leaves (`spec/ingest/impl_refresh.md`, `spec/map/impl_crud_operations.md`,
`spec/impact/impl_action_classification.md`) are *edited* by W3 and *deleted* by W6, so at the end of
the run their W3 content exists nowhere on disk. `git add` stages whole files and `git add -p` is
unavailable here, so a deferred plan cannot produce these commits at all. Committing on completion
yields the same chunks in the same order with no staging gymnastics, and it makes resumption trivial.

| # | commit when | message |
|---|---|---|
| 1 | W1 boundary (1a–1f, 1h) | `tooling: api node type, typed links, refresh filter, render slim, content constraint` |
| 2 | W1 boundary (1g) | `ci: run spex validate and the diff-errors gate on PRs` |
| 3 | W1½ boundary | `spec: retire milestones and scenarios, declare the stack requirement` |
| 4 | W2 boundary | `skills: rewrite /spec, /spec-review, /propose for the new format` |
| 5 | W3 `DONE` | `sweep: retire 33 stale references and four contract defects` |
| 6 | W4 `DONE` | `declare: 15 api nodes across nine modules` |
| 7 | W5 `DONE` | `spec: retire impl_section from project requirement 9120788210c9` |
| 8–18 | each W6 part `DONE` | `migrate: <module> arch leaves to contracts, drop impl sections` |
| 19 | W7 `DONE` | `cutover: remove impl_section from code and schema` |
| 20 | after refresh | `state: refresh snapshot and bead-map` |

**Always `git add` explicit paths; never `git commit -a` and never `git add .`** — another wave's
uncommitted work may be in the tree, and `commit-plan.md` is excluded but `bin/` and `.spex/` are only
gitignored. Chunks 8–18 are safe to commit while other W6 parts run, because the eleven write sets are
disjoint. Chunk 20 must be last: the snapshot reflects the whole final tree. Commit messages are at
most two sentences. Push the branch, open one PR for the whole migration, do not merge it yourself.

### If you are resuming

```bash
git log --oneline origin/main..HEAD     # every part that reached a commit — fact
git status --porcelain                  # every part that did not
```

Each committed chunk is a completed part. Every uncommitted path belongs to an interrupted part;
`commit-plan.md` names which, because its entry was written before the subagent was spawned.

**Revert interrupted parts; do not resume them.** A subagent that died mid-part left a state no gate
accepts and no reviewer can bound:

```bash
git checkout HEAD -- <the part's writes>
git clean -fd <the part's directories>
```

Then re-run that part from `TODO`. **Do not re-run pre-flight** — it demands an empty `git status`,
which a mid-migration tree can never have. Confirm the baseline instead with: empty `git status` after
the reverts, `go build`, `validate` clean, and `diff --json | jq -e '.errors == []'`.

If `commit-plan.md` is missing but `git log origin/main..HEAD` is non-empty, the file was lost, not
the run: rebuild it from the commit subjects, mark every committed chunk `DONE`, revert everything
uncommitted, continue. If both are empty and the tree is dirty, the run was abandoned:
`git checkout HEAD -- . && git clean -fd`, then pre-flight.

**Never compress context and continue.** Finish the current part, commit it, mark it `DONE`, stop.


## Part specifications

### W1 — tooling

**1a — the `api` node type.** Lands alone and first. `schema/schema.go` gains an `API` struct and
`APIs []API` on `ModuleSpec`; `schema/module.schema.json` gains `properties.apis` and `$defs/api`
(mandatory — root `additionalProperties: false` rejects the array otherwise); `merkle/tree_builder.go`
gains `hashAPI`, a structural clone of `hashRequirement` (sorted map → `json.Marshal` → `HashBytes`,
`Type: "leaf"`, `NodeType: "api"`, no file I/O); `merkle/impact_classifier.go` gains an explicit
`case "api"`; `cmd/spex/hashid.go` accepts `--type api`. **Acceptance:** `go test ./...` green;
`hash-id --type api --module merkle --name "spex diff"` prints a 12-hex id; a hand-added `apis` array
validates and appears as `added/api` with a real impact level. Omitting the classifier case makes
`diff` report `"impact":"unknown"` and hard-fails `cmd/spex/impact.go:202`.

**1b — link resolution, removal-time name check, API name uniqueness.** In `validator/`, plus one
line appended to the checker chain in `cmd/spex/validate.go`. This is the first validator that reads
leaf bytes — `content_resolver.go` only stats paths — so it needs a markdown scanner with fence
tracking, a resolver keyed on merkle leaf hashes, and a new error class. Module nodes are not
linkable (`Type == "module"`). The removal check needs the snapshot, which `validator/` cannot reach
today: state whether it loads `spec/.snapshot.json` itself or the check lives in `cmd/spex/diff.go`.
**Acceptance:** a link to a live node passes; to a deleted node errors; a link with no
`|<display text>` errors; two modules declaring the same api name error; a bare 12-hex token outside
`[[…]]` and outside a ```dot fence is ignored.

**1c — refresh type filter.** `ingest/refresh.go:121-134` refuses any `Added` or `Removed` regardless
of type. Admit exactly **`requirement`, `impl_section`, `api`, `component`** — explicit, never
`!beadProducingTypes`, which also admits `meta` and would let refresh baseline a spec `validate`
rejects. `component` is on the list solely because 1e deletes the code behind spec component
`OrphanDetector` and 1h then removes the node. **Acceptance:** deleting an impl_section refreshes
clean; adding an api refreshes clean; **adding** a component still refuses; deleting a component whose
`.bead-map.json` record was removed refreshes clean; deleting one whose record survives refuses with
`orphan_record`.

**1d — render.** `--slim` on `spex render --format json`: drop inlined `content` (326,736 of today's
484,586 bytes), drop `description` (43,364 more), emit bare identity hashes rather than
`module:schema:comp:79946d618829`, include `test_section` (omitted entirely today — a real bug). Nodes
only, `{id, type, name, module}`. Add api nodes to all three renderers, and fix `render/dot.go:55` to
emit the bare identity hash its own spec declares. **Acceptance:** `--slim` is ~24 KB compact and
contains every `test_section`; `render --format dot` node IDs match `hash-id`.

**1e — orphan checks out, `content` constraint in.** Delete `validator/orphan_detector.go` and
`validator/orphan_detector_test.go` outright, remove `validator.CheckOrphans(specDir)` from
`cmd/spex/validate.go:32`, delete `validator/testdata/orphan_*`, and delete
`cmd/spex/validate_test.go`'s `TestFR7_ValidateCommand_WarningsDoNotFail` and its
`setupSpecWithOrphans` helper — that test asserts the validator emits a warning, which this part makes
impossible. Deleting only the two functions leaves `CheckOrphans` with unused variables and an unused
import; `go build` fails. Add to `schema/module.schema.json`, on `$defs/component`, `$defs/data_flow`
**and** `$defs/test_section`: `content` in `required`, `minLength: 1` — all three are silently skipped
by `merkle/tree_builder.go` when `content` is empty (lines 122, 144, 155), so constraining only
`component` leaves two thirds of defect 3 in place. **Acceptance:** `go build ./... && go vet ./... &&
go test ./...` green; zero migration (all 52 components, 11 data_flows, 37 test_sections already
comply); an empty-string `content` is a schema **error**;
`grep -rn 'Severity:\s*"warning"' --include='*.go' .` returns nothing.

**1f — the checking scripts.** `scripts/link-check.sh` per "Links" below, plus `no-rename-check.sh`,
`heading-check.sh`, `ladder-check.sh` and `link-spread.sh` per "The mechanical checks".
**Acceptance:** each exits 0 on the current corpus and 1 on a seeded violation; `link-check.sh` is
unaffected by `if [[ -z "$x" ]]` inside a ```bash fence; a trailing module argument scopes it.

**1g — CI.** `.github/workflows/spec.yml`, on `pull_request` and `push` to `main`: `go vet ./...`,
`go test ./...`, `go build -o bin/ ./cmd/spex/`, `bin/spex validate`, and
`bin/spex diff --json | jq -e '.errors == []'`. **The last step is the one that matters** —
`cmd/spex/validate.go:43-47` returns success on warnings, and `incomplete_change` surfaces only in the
diff `errors` array. Required for merge. First CI in the repository.

**1h — retire the `OrphanDetector` node.** After 1e. Remove component `bbb1ae43d00d` and requirement
`a34f8aa47fad` ("Orphan detection", whose text is false once impl sections go) from
`spec/validator/module.json`, delete `spec/validator/arch_orphan_detector.md`, drop `a34f8aa47fad`
from any `implements` array, and delete the `.bead-map.json` record pointing at `bbb1ae43d00d`.
Reword requirement `707094f8868b`, which names `impl_sections`, `milestones` and `test_plan
scenarios`. Leaving the node ships exactly defect class 2 — a declared interface the code no longer
has. **Acceptance:** `validate` clean; `diff` shows `removed/component` and `removed/requirement` with
no `incomplete_change`; a refresh accepts both. Only part before W8 that edits `.bead-map.json`.

### W1½ — retire the vestigial nodes

**1i — milestones and scenarios.** Remove `milestones` and `test_plan` from `spec/project.json`,
`schema/schema.go` and `schema/project.schema.json`; drop `milestone` and `scenario` from
`cmd/spex/hashid.go` and `validator/id_validator.go`. Remove module requirement `61f3238dfd74`
("Define test_plan in project schema") from `spec/schema/module.json` and from every `implements`
array declaring it. Relocate the pipeline diagram from `spec/test_end_to_end_pipeline.md` into a
tracked flow leaf, then delete the file — it sits at `spec/` root, is referenced by no `module.json`,
and never enters the merkle tree, which is why it rotted. **Acceptance:** `go test ./...` green;
`validate` clean; `diff` shows `removed/requirement` with no `incomplete_change`;
`hash-id --type milestone` errors.

**1j — the stack requirement.** Add one `non_functional` project requirement declaring the language,
the standard-library-first policy and permitted third-party tooling.
`merkle/completeness_checker.go:226-235` fires unless a module requirement derives from it via
`preq_id` and an implementing component's leaf changes, so the same edit adds a derived module
requirement in `cli` and changes `spec/cli/arch_root_command.md`. **Acceptance:** `diff` exit 0 with
`errors: []`.

### W2 — the skills

Rewrite `/spec`, `/spec-review` and `/propose` for hash links, `api` nodes, Go-free arch leaves, the
§1.6 ladder and the absence of impl sections. `/spec` gains a **module-scope argument** it lacks today
(`argument-hint: "<proposal-path-or-name>"`). Correct in the same pass: the undocumented mandatory
`priority` field (`validator/id_validator.go:163`); the two undocumented `requirement_coverage` error
phases; the stale "warnings" terminology and missing exit-2 contract for `spex diff`;
`/spec-review`'s claim that refresh has not shipped; and a warning that 15 project requirements carry
legacy identity hashes `hash-id` cannot reproduce and must never be recomputed. **2d** corrects
CLAUDE.md's `spex map context` field list, which documents the `impl_files` W7 removes.

Note these three subagents document W1's output while writing prose only; nothing they write is
verifiable until W1 has merged, so their review is prose-only.

### W3 — the debt sweep

```bash
grep -rnE '\bspex apply\b|\bBeadCreator\b|\bBeadCloser\b|\bApplyCommand\b|\bBeadUpdater\b|\bPreflightChecker\b|\bspex check\b|/spec-drift|\bspex merkle diff\b' \
  spec/ --include='*.md' | grep -v '^spec/proposals/'

grep -rnoE '\b[a-z][a-z0-9_]*(/[a-z0-9_]+)+\.go\b' spec/ --include='*.md' \
  | grep -v '^spec/proposals/' \
  | while IFS= read -r l; do p="${l##*:}"; [ -f "$p" ] || echo "$l"; done
```

A single-segment path regex matches `spex/diff.go` inside `cmd/spex/diff.go` and yields 25 false
positives; dropping the existence test cannot tell stale from live. **Expect 33 and 9** — `spex apply`
8, `BeadCreator` 14, `BeadCloser` 3, `ApplyCommand` 2, `BeadUpdater` 2, `PreflightChecker` 1,
`spex check` 1, `/spec-drift` 1, `spex merkle diff` 1, distributed **flow 21, arch 5, impl 3, test 2,
spec-root 1**. The spec-root hit is `spec/test_end_to_end_pipeline.md`, which lives outside every
`spec/<MOD>/` and therefore has no W6 owner — **W3 owns it**. If the counts differ, the corpus has
moved; stop and re-measure.

**A hit is not automatically an edit.**

| verdict | test | action |
|---|---|---|
| REWRITE | presents the removed thing as current behaviour | rewrite to name what does the job now |
| KEEP | explicitly historical — "pre-decouple", "legacy", "previous", "no longer", "there is no" — and the history explains the current design | leave the bytes alone |
| FIXTURE | inside sample output or example JSON a reader or test compares against | leave alone |

Measured: **seven of the nine Go paths are KEEP** — six `pre-decouple apply/bead_creator.go` citations
plus `spec/proposal/arch_proposal_commands.md:53` — and **two are REWRITE**: `merkle/cmd.go`
(`spec/cli/arch_root_command.md:16`) and `internal/atomic.go` (`spec/ingest/impl_refresh.md:167`). Of
the token sites, three are KEEP or FIXTURE: `spec/adapters/impl_br_subprocess.md:113`,
`spec/proposal/arch_history_viewer.md:51` (sample `spex log` output),
`spec/emit/arch_changeset_builder.md:113` (example changeset JSON). Sweeping a KEEP deletes the
rationale trail §1.6 arm 5 protects.

Also: the wrong `Register` signature in `spec/proposal/arch_registrar.md`, corrected to
`func Register(proposalPath, specDir string) (string, error)`; `CommandRegistrar`
(`spec/cli/flow_command_dispatch.md:93`); the empty-`content` skip, into
`spec/merkle/arch_tree_builder.md`; the narrow refresh contract at
`spec/ingest/arch_refresh.md:18-24`, which also cites the deleted `/review`; and `README.md:42`.

**`spec/render/flow_render_pipeline.md:72` is deliberately NOT swept.** The spec declares bare identity
hashes; `render/dot.go:55` emits `<mod>_comp_<hash>`. **The spec is right and the code is wrong** — 1d
fixes it. Sweeping it would delete the convention W6's `dot` diagrams are defined against.

**Three stale references live in JSON envelopes**, invisible to the token sweep:
`spec/schema/module.json` requirement `cdc9c58ba097` and `spec/map/module.json`'s bead-mapping
data_flow both name the removed `apply` module, and project requirement `81f8102ae1b5` is titled
"Apply changes" — **re-describe it, never retitle it**; it carries a legacy identity hash.

**Those three envelope edits carry nine mandatory arch-leaf touches, and they belong to this part.**
Measured: without them `diff` exits 2 with **14 `incomplete_change` errors**, and because refresh is
deferred to W8 those errors persist through W4 and W5, failing their gates too. `cdc9c58ba097` obliges
`spec/schema/arch_schema_loader.md`. `81f8102ae1b5` derives into impact, emit, ingest and adapters and
obliges `spec/impact/arch_bead_reader.md`, `spec/impact/arch_action_classifier.md`,
`spec/emit/arch_topological_sorter.md`, `spec/emit/arch_resolver.md`,
`spec/ingest/arch_reconciler.md` and `spec/adapters/arch_br_reference_adapter.md`. The
`spec/map/module.json` data_flow description moves `meta/map` with no map requirement changing, so the
whole-module obligation applies: `spec/map/arch_map_command.md` and
`spec/map/arch_context_resolver.md` in addition to `arch_mapping_store.md`, which the token sweep
already reaches. Each takes a real edit — the contract sentence the envelope change implies — not a
manufactured touch. Measured with them: exit 0, `errors: []`, 28 changes.

**Two leaves are rewrites, not sweeps.** `spec/map/flow_bead_mapping.md` and
`spec/proposal/flow_proposal_lifecycle.md` document a lifecycle (`spex apply` → BeadCreator →
BeadUpdater → BeadCloser) that no longer exists. Rewrite both against the shipped pipeline, whose
authoritative leaves are `spec/emit/arch_changeset_builder.md`,
`spec/adapters/arch_br_reference_adapter.md` and `spec/ingest/arch_reconciler.md`; the executable
reference is `scripts/apply-br.sh`. The replacement lifecycle is
`diff → impact → emit → <adapter> → ingest`. Read those three before writing a word, and do not rename
either data_flow node.

### W4 — declare 15 apis across nine modules

15 invocations: the 12 top-level commands plus `map get`, `map list`, `map context`. They resolve to
**nine** modules — `validator, merkle, impact, emit, ingest, proposal, render, cli, map`. **`schema`
and `adapters` own no command entry point and gain no `apis` array.** Whether the bare `spex` root
also gets a node is this part's call, recorded in `commit-plan.md`.

Adding `apis` moves `meta/<module>`, which makes `merkle/completeness_checker.go` demand a changed
content leaf for **every** component in that module — measured, a meta-only pass over the nine fires
**47** `incomplete_change` and `diff` exits 2.

**The escape, measured green:** in every module that gains an api, also touch one requirement
description **and every arch leaf of every component implementing that requirement**. The requirement
change puts the module into `reqModules` and `merkle/completeness_checker.go:126-128` skips the
whole-module obligation, leaving only the per-implementor one. Choose the requirement with the fewest
implementors: **eight of the nine have one with exactly one; `render` has none** — its cheapest is
`8d441659a190` *Composable output* with two, and touching only one fails with `requirement 'Composable
output' (render) description changed but component RenderCommand content leaf unchanged`.

**`adapters` and `schema` are unprotected going into W6.** Neither enters `reqModules` here. `schema`
recovers in W6 via its two `impl_section`-naming requirements; `adapters` has none, so when its W6
part removes `impl_sections` its meta moves with no requirement change and the whole-module obligation
fires. `adapters` has one component carrying 8 declared edges, so W6 step 6's link obligation
satisfies it — do not skip step 6 for `adapters`.

### W5 — project requirement `9120788210c9`

*Map spec nodes to beads* names `impl_section`. Eight module requirements derive from it across
schema, impact, emit, ingest and map, implemented by **seven distinct components** — `MappingStore`
implements two. Measured green: `validate` valid, `warning_count 0`; `diff` exit 0, `errors: []`, nine
changes — 7 `modified/component`, 1 `modified/requirement`, 1 `modified/meta`, **the last two carrying
`module: ""`**, because project-level changes have no module. The gate's module clause does not apply
to this part.

### W6 — the eleven module migrations

**Step 1 — load.** Read `spec/<MOD>/module.json`. **Before reading any impl leaf**, write down every
arch leaf's `##` heading list (§1.6 P2). That frozen list is the destination map.

**Step 2 — fix destinations by requirement, not `describes`** (§1.6 P1). Only `schema` has
multi-`describes` impl sections.

**Step 3 — run the ladder.** One line per `##` section, in file order, in
`commit-plan.d/<part>.md` — **never `commit-plan.md`**. Format, parsed by `ladder-check.sh`:

```
<impl-leaf-basename> :: <heading text without "## "> :: arm <n> <ARM NAME> :: <destination or ->
```

Each leaf's H1 and any prose before its first `##` is one line whose heading is `[preamble]`. A
section split at a fence boundary (P3) produces two lines with the same heading and a `(split 1/2)`
suffix on the arm name.

**Arm 0 files nothing.** There are no beads. Record the line with destination `-`, describe both
statements and the code you checked in your report, and do not resolve it. The orchestrator copies it
into `commit-plan.md` under `## Findings`. Arm 0's "file a drift bead" wording is superseded for this
migration. You may read Go to establish which statement the code supports; never edit it.

**Step 4 — strip the arch leaves.** `grep -n '```go\|^func \|(ctx context.Context' spec/<MOD>/arch_*.md`
prints nothing when done, unless §1.7's exception applies. `adapters` and `cli` have zero Go fences in
arch — a no-op there; their fold and link obligations are unchanged.

**Step 5 — re-represent diagrams** per the table under "Segment 2 — diagrams and links".

**Step 6 — insert links.** A link goes **in the sentence that already discusses the target**,
replacing or wrapping the existing mention. If a leaf mentions a declared edge nowhere, write the
sentence that explains the relationship and link inside it — do not append it to a list.
`link-spread.sh` fails a leaf that puts more than half its links in its last 20% of lines or grows a
`## References`-style heading.

**Step 7 — delete impl sections.** `rm spec/<MOD>/impl_*.md`, remove the `impl_sections` array from
`spec/<MOD>/module.json`. **Leave `schema/module.schema.json` alone** — that is W7. Then edit **every**
requirement, component, data_flow and test_section `description` in this `module.json` that names
`impl_section`: merkle, schema and validator have two requirements each, and `ActionClassifier`,
`ContextResolver`, `ImpactClassifier`, `ModuleSchema` and `OrphanDetector` carry it in component
descriptions. Verify with `grep -c impl_section spec/<MOD>/module.json` → 0. A requirement edit fires
`incomplete_change` against its implementor, which step 6's link already satisfies.

### W7 — cutover

Remove `impl_section` from the **17** non-test Go files — `render/{dot,json,markdown,spec_reader}.go`,
`emit/sorter.go`, `cmd/spex/{emit,impact,ingest,hashid}.go`,
`validator/{content_resolver,id_validator}.go`, `merkle/{tree_builder,diff_engine,impact_classifier}.go`,
`impact/action_classifier.go`, `mapping/context_resolver.go`, `schema/schema.go`
(`validator/orphan_detector.go` is already gone, deleted in 1e) — then **24 `_test.go` files and 43
JSON fixtures** under `validator/testdata/`, `merkle/testdata/` and `cmd/spex/testdata/`, and only
then `schema/module.schema.json`, **in that order**. `encoding/json` ignores unknown fields, so
removing the Go struct field first fails *open* while removing the schema property fails *closed*.
`ingest/refresh.go` keeps its `impl_section` entry: the snapshot still carries impl_section leaves
until W8.

**Acceptance:** `go build ./...`, `go vet ./...`, `go test ./...` green; `validate` clean;
`grep -rn impl_section spec/*/module.json spec/project.json` empty — **`spec/.snapshot.json` is
excluded, because it describes the pre-migration tree until W8 rewrites it**; `spex map context <id>`
succeeds and no longer emits an `impl_files` key at all, the field being removed from `ContextResult`.
This part's spec diff is legitimately empty, so `diff` reporting no new changes is a pass.

### W8 — refresh and final commit

```bash
mkdir -p .spex
printf '%s' '{"version":1,"git_head":"","proposal":"","ops":[]}' > .spex/changeset.json
printf '%s' '{"version":1,"ops":[]}'                             > .spex/receipts.json
bin/spex ingest --mode refresh --changeset .spex/changeset.json --receipts .spex/receipts.json
bin/spex diff        # must print "no changes"
bin/spex validate    # valid true, 0 errors, 0 warnings
```

Measured on the full end state with the 1c filter: `records_updated: 55, records_unchanged: 6,
snapshot_saved: true, status: complete`, exit 0, then `diff` → no changes. Unpatched, it refuses with
`added_entries` — 1c is load-bearing.

`ingest` is atomic. If it refuses, a part renamed something; the 15 `added/api` and 56
`removed/impl_section` entries are expected and admitted, so filter them out:

```bash
jq '.changes[] | select(.type != "modified" and (["api","impl_section","requirement","component"] | index(.node_type) | not))' /tmp/diff.json
```

Then commit chunk 20 and open the PR.


## Context

Three unrelated-looking defects share one root cause.

**1. Deleted things still documented as current.** `spex apply` was removed in `a74dca3`
(2026-04-25), a commit touching nine files — eight code, one `.beads/issues.jsonl` — and zero spec. An
exhaustive sweep finds **32 live stale references**: `spex apply` 8, `BeadCreator` 14, `BeadCloser` 3,
`ApplyCommand` 2, `BeadUpdater` 2, `PreflightChecker` 1, `spex check` 1, `/spec-drift` 1. Distribution
**flow 22, arch 5, impl 3, test 2**. One further stale command: `spex merkle diff`. Six `spex hash`
mentions are *not* stale — they document its absence.

**2. Declared interfaces the code never had.** `spec/proposal/arch_registrar.md` has been edited
exactly once — `087a231`, the bootstrap — and declares
`func Register(ctx context.Context, proposalPath, specDir string) error`; the implementation
(`0efb835`, zero spec files touched) is `func Register(proposalPath, specDir string) (string, error)`.
`git log -S` confirms the code never had the declared form. Three more: `render/dot.go:55` emits
`<mod>_comp_<hash>` while `spec/render/flow_render_pipeline.md:72` declares bare identity hashes;
`CommandRegistrar` and `merkle/cmd.go` / `NewCmd()` exist nowhere.

**3. Undocumented behaviour with authoring consequences.** `merkle/tree_builder.go` skips any
component (122), impl_section (133), data_flow (144) or test_section (155) whose `content` *field* is
empty — the node never enters the tree. `spec/merkle/arch_tree_builder.md` does not mention it. A
zero-byte *file* is different: it does enter the tree.

**4. A declared contract narrower than the code.** `spec/ingest/arch_refresh.md:18-24` restricts
refresh to non-bead-producing leaves while `ingest/refresh.go:111-134` applies no node-type filter and
gates only on `Added` and `Removed`.

### Why the pipeline cannot see any of them

*Prose is not an edge.* Declared references are graph edges and `validator/id_validator.go` errors
when one points at a removed node. The stale references are untyped prose; nothing changed their
bytes, so no hash moved, so no diff, so no bead.

*The CLI surface is not in the graph.* `ApplyCommand` was a component, but `spex apply` — the
invocation every document names — was nothing.

*Code is not in the tree.* `/spec-drift` covered this once; deleted in `2a13728`, and
`docs/enforcement-migration.md` does not list it in the disposition table.

*Nothing runs the checks.* `.github/` contains only `pull_request_template.md`.
`cmd/spex/validate.go:43-47` exits 0 on warnings. On the day `apply` was deleted nothing would have
invoked `spex validate`.

### Corrections to earlier reasoning

Drift does **not** concentrate in arch leaves: 22 stale references are in flow leaves against 5 in
arch, and of 15 `Bead*` references in `spec/map/flow_bead_mapping.md` only 3 are in the ASCII diagram
— 12 are running prose. The arch-vs-impl Go gap is narrow: 67% of arch leaves carry ```go fences
against 57% of impl leaves. The case for changing arch rests on stack-agnosticism and defect 2, not
drift frequency.

## Proposed change

### Segment 0 — U1. Segment 1 — U2 and U3, plus U4 and U5. Segment 2 — U6 through U17.

**1.1 A new node type: `api`.** `module.json` gains an `apis` array:

```json
"apis": [
  { "id": "3f9a1c7b2e04", "name": "spex diff",
    "description": "Compute changes between snapshot and current spec",
    "provided_by": ["c8b958ec310d"], "group": "cli" }
]
```

`name` is the exact external surface string as callers write it — `spex diff`, or `GET /v1/specs/{id}`
for an HTTP project, or `schema.IdentityHash` for a library entry point. Never a signature. Identity is
`<module>/api/<name>`, so `hash-id --type api` joins the existing types (eight today; six after 1i
removes `milestone` and `scenario`).

**No content file** — APIs hash from their JSON fields exactly as project requirements do. Verified:
that path returns `Type: "leaf"`, so `collectLeafHashes` sees api nodes and link resolution works. An
`api_*.md` leaf would recreate the impl-section mistake.

**Not bead-producing**, by omission from `impact.beadProducingTypes`; verified, an added api yields
zero create/obsolete actions. **`provided_by` is module-local** — the api belongs to the module owning
the command's entry point; other modules' involvement is already expressed by component `uses` edges.
**`group` is freeform and spex never branches on it**; it exists so `spex render` can group a
project's surface when it serves both a CLI and HTTP. **API names are globally unique across modules**,
a new explicit check — all existing uniqueness is per-array within one file.

Renaming an API is delete-plus-create. Only identity changes are caught: an added query parameter, a
changed response field or a dropped `--flag` moves no name.

Feasibility is verified: ~70 lines across `schema/schema.go`, `schema/module.schema.json`,
`merkle/tree_builder.go`, `merkle/impact_classifier.go`, `cmd/spex/hashid.go`, full test suite passing.
`merkle/impact_classifier.go` needs an explicit `case "api"` — without it `diff` reports
`"impact":"unknown"` and the next stage **hard-fails** at `cmd/spex/impact.go:202`.
`render/{dot,json,markdown}.go` silently drop api nodes today and must be extended.

**1.2 Typed links carrying identity hashes.** `[[<identity-hash>|<display text>]]`, resolving against
the merkle leaf keys as `ingest/refresh.go:155` already does. Name-based links are rejected: they drop
the `<type>` segment so they cannot compute an `IdentityHash`; they are ambiguous
(`spec/schema/module.json` carries `Identity hash algorithm` as both a requirement and an
impl_section); and they make every rename a multi-leaf edit minting a bead pair per site.

Display text is **free-form and unchecked**. Links sit **outside** existing code spans —
`` [[3f9a1c7b2e04|`spex validate`]] ``. **Module nodes are not linkable**: `collectLeafHashes` collects
`Type == "leaf"` only, and modules are `Type == "module"`.

Verified non-issue: the 15 bash `[[` occurrences in `spec/adapters/` are all inside ```bash fences and
every one is followed by a space, so `\[\[[0-9a-f]{12}\|` cannot match.

**1.3 `spex render --format json --slim`.** Four changes to the shipped renderer: drop inlined
`content` (326,736 of today's 484,586 bytes), drop `description` (a further 43,364), emit bare identity
hashes instead of `module:schema:comp:79946d618829`, and include `test_section`, which the renderer
omits entirely — a real, unrelated bug. `--slim` emits nodes only, `{id, type, name, module}`, because
that is the name→hash table `/spec` and the validator need; edges come from `module.json`, which the
authoring agent is editing anyway. **Measured 35,337 bytes pretty, 24,221 compact**, dropping to ~22 KB
once impl sections leave and apis arrive.

**1.4 Removal-time name checking.** When `diff` reports a node removed, search the corpus for its
declared name and error on survivors. What was rejected earlier was searching for *guessed* terms: the
bare word `apply` gives 74 case-insensitive hits for 8 real; `spex apply` and `ApplyCommand` give 8 for
8 and 2 for 2. Two scoping rules make it safe. **Only `api` and `component` names are searched** — the
56 impl_section names are generic noun phrases, and `Hash computation` alone survives 13 times in
another module's fixtures. **Longest-match-first**, subtracting hits consumed by a longer live name:
`spex map` occurs 34 times, 29 belonging to `spex map get|list|context`, leaving 5 bare.
`spec/proposals/`, `README.md`, `docs/` and `skills/` are outside the gate.

No rule forces live mentions to be links. 1.2 protects the future; 1.4 protects the past. **The
residual gap:** a paraphrase that names nothing is invisible to every mechanism here, permanently.

**1.5 Refresh absorbs additions and removals of an explicit type list** — exactly `requirement`,
`impl_section`, `api`. It must be explicit, not `!beadProducingTypes`: that also admits `meta`, and
refresh runs neither `validate` nor the completeness checker, so a literal implementation absorbs a
project-requirement addition `diff` rejects with exit 2 and a module-requirement removal `validate`
rejects with exit 1.

**1.5 ships before the first segment-1 unit, not merely before segment 2.** U4 adds api nodes and ends
with refresh; unpatched, that is exit 2 `added_entries` and 23 bead operations. Measured on the
strip-and-delete half: unpatched refuses, normal pipeline costs **102 bead operations**; patched,
refresh exits 0 with **zero**.

No completeness-checker change is proposed: the snapshot stores hashes only and cannot know what
changed inside `module.json`. It is also unnecessary — ordering satisfies it.

**1.6 The arch redesign procedure.** Impl content is judged **per `##` section**, each getting exactly
one recorded verdict. Judging whole leaves does not reproduce between agents.

*Preamble, once per component:*

- **P1 — destination by requirement, not `describes`.** The destination is the arch leaf of the
  component that `implements` the requirement the content documents.
  `spec/schema/impl_identity_hash.md` `describes` ProjectSchema and ModuleSchema, but its requirement
  `cdc9c58ba097` sits on `SchemaLoader`.
- **P2 — read the arch leaf to completion first and write down its `##` headings.** Freezing that list
  before reading the impl leaf is what makes arm 2 mechanical.
- **P3 — the unit of verdict is a `##` section.** The H1 and any prose before the first `##` is one
  section named `[preamble]`; in every sampled leaf it restates the component name and takes arm 2. If
  a section contains a language fence and prose that would take a different arm, **split it at the
  fence boundary and record one verdict per part** — splitting is recorded, never silent.

*The ladder, first match wins:*

| # | Arm | Test | Verdict |
|---|---|---|---|
| 0 | CONTRADICTS | asserts what the arch leaf denies | resolve against the code, keep one statement, file a drift bead for the loser. Never silently pick. |
| 1 | SYNTAX | load-bearing content is a language fence, or a sentence whose subject is a language identifier | Delete — but first: does it assert something a caller can observe (stdout, exit code, file written, ordering, a name convention)? If yes, restate it language-neutrally **into the arch section whose subject it shares, creating one if none exists**, then delete. |
| — | *1.7 exception* | the destination component implements a requirement **whose description names the algorithm or bound the fence encodes** | keep as language-neutral pseudocode |
| 2 | ALREADY SAID | the arch leaf has a section or sentence whose **subject** is the same node, artifact, field or condition | Delete. If the impl wording is more precise, replace the arch words **in place**; section count does not grow. |
| 3 | CODE'S JOB | a fact about a git-tracked file, or an instruction to a future implementer | Delete. |
| 3.5 | COST CLAIM | an asymptotic complexity, benchmark or resource figure | Delete, unless the destination implements a requirement naming a bound — then move as one sentence. Cost claims never create a section. |
| 4 | FALSIFIABLE | a reader with the built binary could prove it wrong | Move, rephrased to name no language. New section allowed. |
| 5 | UNRECOVERABLE WHY | a rejected alternative, a constraint from elsewhere, a historical reason | Move, compressed. |
| 6 | otherwise | — | Delete. |

Arm 2 sits above arm 3 deliberately: measured across nine leaves, the dominant reason to delete is
"the arch leaf already says this", not "this duplicates code", and asking the duplication question
first produces arch leaves stating the same thing twice. Aggregate outcome ~68–74% deleted. The top
arm found three genuine spec-vs-code contradictions in the sample.

The real judgment surface is **80 KB** — 79,808 bytes of impl prose with all language fences removed —
not the 41 KB of Go fences inside those leaves and not the full 136 KB. (12.7 KB is the Go-fence total
in the *arch* leaves, which is what step 4 strips.)

**1.7 Pseudocode where the algorithm is the requirement**, per the arm-1 exception. **The test is the
requirement's description, not its `type`.** `cdc9c58ba097` "Identity hash algorithm" is typed
`functional` and its description *is* the algorithm, while `SchemaLoader` implements no
`non_functional` requirement at all — so a type-based filter would delete the one fence the rule exists
to keep.

**1.8 Impl sections are removed.** They produce no beads, duplicate what is already trackable, and bind
the spec to a language. **No bead-producing nodes are added in segment 2.** An earlier draft proposed
converting the four multi-`describes` impl sections to data flows; withdrawn, because `data_flow.uses`
is a sequencing edge `impact/action_classifier.go:196-220` consumes to order emit ops. All four fold
per P1.

**The orphan check is deleted, not rewritten.** Replace `detectOrphanComponents` with JSON Schema on
`$defs/component`, `$defs/data_flow` **and** `$defs/test_section` — `content` in `required`,
`minLength: 1`. All three are silently skipped by `merkle/tree_builder.go` when `content` is empty
(122, 144, 155); constraining only `component` leaves two thirds of defect 3 in place. Measured: all 52
components, 11 data_flows and 37 test_sections already comply — zero migration — and it catches the
empty-string case as an **error**, not a warning. `detectOrphanRequirements` also goes:
`requirement_coverage_checker.go` already errors on the identical condition. **The validator ends with
zero warning sites**, making `warning_count == 0` trivially true.

**`impl_files` is removed from `spex map context`.** **CLAUDE.md documents that field and must be
updated in the same change.**

Scope: 18 non-test Go files reference `impl_section`. In the spec, 142 occurrences across 56 leaves in
8 modules — 17 of those leaves are `impl_*.md` the migration deletes, so the real surface is **39
leaves**.

**1.9 Milestones and `test_plan.scenarios` are deleted.** Both are vestigial: declared in
`schema/schema.go`, ID-checked by the validator, and that is all. No renderer touches them. Milestones
predate proposal epics; scenarios were superseded by test sections with `describes >= 2`, and three of
four never got a content file. Also remove module requirement `61f3238dfd74`, which exists solely for
`test_plan`, and correct two requirement descriptions naming them. The pipeline diagram in
`test_end_to_end_pipeline.md` relocates into a tracked flow leaf and the file is deleted.

**1.10 Stack becomes an explicit non-functional project requirement.** An added project requirement
triggers `merkle/completeness_checker.go:226-235` unless a module requirement derives from it and an
implementing component's leaf changes; that chain must be designed.

**1.11 A CI job that runs `spex validate`** — see U3.

**1.12 The authoring skills are rewritten** for hash links, api nodes, Go-free arch leaves, the 1.6
ladder and the absence of impl sections; `/spec` gains a module-scope argument; the CLAUDE.md
`map context` field list is corrected. Audit defects fixed in the same pass: the undocumented mandatory
`priority` field (`validator/id_validator.go:163`); the two undocumented `requirement_coverage` error
phases; the stale "warnings" terminology and missing exit-2 contract for `diff`; `/spec-review`'s claim
that refresh has not shipped; and a warning that 15 project requirements, 6 milestones and 3 scenarios
carry legacy hashes `hash-id` cannot reproduce.

### Segment 2 — diagrams and links

**Diagrams**, decided by one test — *does every arrow connect two named things?* Scope is 22 untagged
fences carrying arrows or box characters, 19,244 bytes across 16 `arch_*`/`flow_*` files. The other 27
untagged fences in those files are CLI usage strings and sample output: **left alone**, since the test
presupposes arrows.

| class | count | target |
|---|---|---|
| GRAPH | 12 | ```dot fence |
| CONTROL_FLOW — arrows connect steps, or the block branches | 7 | numbered markdown list, or a two-column condition→outcome table |
| FIELD_TREE — no arrows, containment of field names | 3 | ```json skeleton, or a `key pattern / node type / hashed from` table |

No rule is written for shapes that do not occur — there is not a single `+---+` grid in the corpus.
Prose is never a node: a box holding a sentence gets a short label and the sentence moves below the
fence.

**DOT node IDs** adopt the convention the spec already declares at
`spec/render/flow_render_pipeline.md:72`, so hand-written diagrams are byte-compatible with
`spex render --format dot`. **A 12-hex token resolves only inside a ```dot fence or inside `[[…]]`;
everywhere else it is text.** Measured: 144 bare 12-hex occurrences exist, 99 of them fake fixtures, and
**zero are in a `dot` fence** — day-one false positives are zero. Illustrative graphs use non-hex IDs.
**Non-node participants are decided by ID shape**: `^[0-9a-f]{12}$` resolves and must exist, anything
else is a literal identifier, never resolved, rendered dashed. So `scripts/apply-br.sh`,
`changeset.json` and `br` are legal and unchecked.

**Links.** A unit links exactly the declared edges of the nodes whose leaves it rewrites. For a
**component leaf**: `uses ∪ implements ∪ {api.id | this component ∈ api.provided_by}`. For a
**data_flow leaf**: `uses` only. That is **220 links across 63 files** — 55 component `uses`, 109
`implements`, 41 data_flow `uses`, 15 api links — against a raw surface of 1,157 mentions.

Out of scope permanently: all 38 test leaves; second and later mentions of an already-linked target;
anything inside a fence; module names; file paths, env vars, flags and external tools; a component's
mentions of itself.

**Done test.** "Touched" is read from `diff --json`: changes of type `modified` or `added` whose
`node_type` is `component` or `data_flow`.

```bash
#!/usr/bin/env bash
# scripts/link-check.sh <spec-dir> <diff.json> — exit 1 and name every missing link.
set -uo pipefail
spec=${1:-spec}; diffjson=${2:-diff.json}; fail=0
touched=$(jq -r '.changes[] | select((.type=="modified" or .type=="added") and (.node_type=="component" or .node_type=="data_flow")) | .path' "$diffjson")
for mj in "$spec"/*/module.json; do d=$(dirname "$mj")
  while IFS=$'\t' read -r id file want; do
    grep -qxF "$id" <<<"$touched" || continue
    have=$(grep -oE '\[\[[0-9a-f]{12}\|' "$d/$file" | tr -d '[|')
    for w in $want; do grep -qxF "$w" <<<"$have" || { echo "MISSING_LINK $d/$file ($id) -> $w"; fail=1; }; done
  done < <(jq -r '(.apis//[]) as $a | ((.components//[])[] | . as $c
        | [$c.id, $c.content, ((($c.uses//[])+($c.implements//[])+[$a[]|select((.provided_by//[])|index($c.id))|.id])|join(" "))]),
        ((.data_flows//[])[] | [.id, .content, ((.uses//[])|join(" "))]) | @tsv' "$mj")
done
exit $fail
```

The `|` in the pattern is load-bearing: bash test syntax can neither count as a link nor mask a
missing one.

**This obligation is load-bearing.** All 52 components carry at least one declared edge, so linking
guarantees every arch leaf's bytes change — exactly what `merkle/completeness_checker.go:126-144`
demands of every component in a meta-changed module. It removes any need for manufactured touches and
rescues the case where an impl leaf is fully deletable and the arch leaf would otherwise not change.

**Per-unit allowlist.** `removed/{impl_section,requirement}`,
`modified/{component,data_flow,test_section,requirement,meta}`, `added/api`. The module-scoping clause
binds U6–U17 only: **U4 is cross-module by construction, and U5 carries two rows whose `module` is the
empty string.** `validate` has no `--json` flag because it always emits JSON; `diff --json` keys each
change as `path` and carries `module`.

## Impact expectation

**U1** produces no beads, run through refresh.

**U2/U3** are ordinary pipeline work producing normal beads across `schema`, `validator`, `ingest`,
`render`, `merkle`, `impact`, `map` and `cli`. The CI job and the skill rewrites produce **no beads and
no diff**, because neither `.github/` nor `skills/` enters the merkle tree; they are created by hand as
U2's last step, `spex:cleanup`-labelled and parented to the epic, and both must close before U6.

**U4–U17**, with 1.5 landed and no bead-producing nodes added, are refresh-mode content migration with
**zero bead lifecycle**. The one exception is arm-0 drift beads, filed by hand; the sample found three
across nine leaves, so expect single digits. If a unit produces more than five, stop — it is too large
or the ladder is being misapplied.

**Net surface.** This removes impl sections, milestones, scenarios, two orphan checks, the `impl_files`
field, and a subcommand that was never built; it adds one node type, one link syntax, one renderer flag
and one CI job.

**Rollback.** Under cutover-last a stall leaves the corpus green and resumable. What does not roll back
is `.beads/issues.jsonl` once `scripts/apply-br.sh` has run, since portitor requires the
`reviewer`/`owner` role to add a `"status":"closed"` line. Landing 1.5 first eliminates the exposure.

**Known limits, stated rather than implied.** Only identity changes are caught: a renamed flag, an
added query parameter or a changed response field produces no signal. A paraphrase that names nothing
is invisible. Links cover declared spec nodes only — scripts, Go file paths, env vars and external
tools remain prose. And ~900 live mentions are deliberately left unlinked, because forcing them would
cost more than it protects.
