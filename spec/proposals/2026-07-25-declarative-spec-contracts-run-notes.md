# Run notes — declarative spec contracts migration

Companion to `2026-07-25-declarative-spec-contracts.md`. Written at the W5 boundary, after five waves
committed, for whoever picks this up next.

**Read this before re-reading the proposal.** The proposal is still the plan; this file records where it
is wrong, what it does not say, and what has been measured since it was written.

---

## 1. Where the run stands

| wave | parts | commit |
|---|---|---|
| W1 tooling | 1a, 1b, 1c, 1d, 1e, 1f, 1h | `f7368c0` |
| W1½ vestigial nodes | 1i, 1j | `0acc9d1` |
| W2 skills | 2a, 2b, 2c, 2d | `94e7472` |
| W3 debt sweep | 3 | `ce3669c` |
| W4 apis | 4 | `bc8121c` |
| W5 project requirement | 5 | `166dbf3` |
| **W6 module migrations** | 6a–6k-2 (**12 parts**) | **not started** |
| **W7 cutover** | 7 | **not started** |
| **W8 refresh** | 8 | **not started** |

**1g (CI) was withdrawn by the author** — CI belongs in the tool-delivery proposal, not here. Commit
chunk #2 in the proposal's table is dropped; W1 produced one commit, not two. The consequence: the
proposal's fourth root cause (*"Nothing runs the checks"*) is **unaddressed by this migration** and moves
to `spec/proposals/2026-07-25-tool-delivery.md` on branch `proposal/delivery`, whose CI pipeline
component currently gates only build/vet/test and needs `spex validate` plus the diff-errors check.

**State at this boundary:** tree clean, `validate` 0 errors / 0 warnings, `diff` rc=0 with `errors: []`
and **93 accumulated changes** (the snapshot is not refreshed until W8, so every wave's changes stay in
the diff). `scripts/no-rename-check.sh` rc=0.

## 2. Resuming

Do **not** run the proposal's pre-flight — it demands an empty `git status`, which a mid-migration tree
can have only between waves. Confirm the baseline instead:

```bash
git log --oneline origin/main..HEAD     # six migration commits, listed above
git status --porcelain                  # empty at a wave boundary
go build -o bin/ ./cmd/spex/
bin/spex validate | jq -e '.valid == true and .warning_count == 0'
bin/spex diff --json | jq -e '.errors == []'
bash scripts/no-rename-check.sh
```

`commit-plan.md` and `commit-plan.d/` at the repo root are the orchestrator's scratch state, excluded via
`.git/info/exclude`. They carry per-part write sets and revert recipes. **This file carries everything
that outlives them.**

**Commits are attended.** The signing key is a hardware token; an unattended commit fails with
`agent refused operation` and leaves the wave in limbo. This happened once, at the W3 boundary. Ask
before every commit — approval at one boundary is not approval at the next.

## 3. Where the proposal is wrong

### 3.1 The W6 gate invocation cannot run as written

The proposal's W6 recipe builds an isolated tree at `/tmp/spex-gate` — a plain directory outside any git
repo — then invokes `heading-check.sh "$GATE_DIR/<MOD>"` with the default base `origin/main`. That exits
**2**: *"not inside a git repository and base 'origin/main' is not a directory."* Same for
`link-spread.sh`, `ladder-check.sh` and `no-rename-check.sh`. Every one of the twelve W6 parts would hit
it on its first gate.

Worse, the obvious operator fix — passing `/tmp/spex-base` (a spec **root**) as `<base>` — used to
return **0 while masking real violations**. That hole is now closed (a spec root handed where a module
directory is expected exits 2 on either side), but the correct invocation is per-module on **both** sides:

```bash
heading-check.sh "$GATE_DIR/$MOD" /tmp/spex-base/$MOD
link-spread.sh   "$GATE_DIR/$MOD" /tmp/spex-base/$MOD
ladder-check.sh  "$GATE_DIR/$MOD" commit-plan.d/<part>.md /tmp/spex-base/$MOD
link-check.sh    "$GATE_DIR" /tmp/diff.json "$GATE_DIR/$MOD"
```

### 3.2 The gate allowlist is missing `added/requirement`

The gate snippet (`:218-220`) and the prose rule (`:181`) both omit it, and `:181` never mentions 1j at
all — though 1j's entire brief is to **add** a project requirement plus a derived module requirement. A
part briefed to add requirements cannot avoid emitting `added/requirement`. It was added to the allowlist
for W1½ only. No later wave should emit it; if one does, that is a real finding.

### 3.3 The reference `link-check.sh` in the proposal is buggy

The implementation at `:947-961` counts a `[[id|Name]]` **inside a fence** as a link, while the Links
paragraph says "anything inside a fence" is out of scope. Demonstrated end-to-end: dumping all eight of
`spec/render/arch_render_command.md`'s declared edges into a ```text fence turned `link-check.sh`,
`link-spread.sh`, `heading-check.sh` and `no-rename-check.sh` **all green** while satisfying the
completeness checker — the exact "pass every gate while doing nothing" the mechanical checks exist to
prevent. The shipped script fixes this; the proposal's snippet should not be copied.

Note the proposal's "byte-identical" clause means the *optional third argument* must not change behaviour
when unused. It does **not** mean fidelity to that reference.

### 3.4 Stale measurements

Measured against the corpus during W1 and W2:

| proposal claim | measured |
|---|---|
| §1.4 `Hash computation` survives **13** times | **16** |
| §1.4 `spex map` leaves **5** bare | **4** (the fifth grep hit is `spex mapping store`, correctly not a match) |
| §1.2 **144** bare 12-hex occurrences | **113** in declared leaves, 169 including proposals |
| §W3 expect **33** token hits | **32** — 1i deleted the spec-root file that held the 33rd |
| §1.12 `priority` check at `id_validator.go:163` | `checkProjectPriority` at **241-261**; that line is a closure |
| 1b "one line **appended**" to the checker chain | `CheckLinks` is inserted **third**, before `CheckIDs` |

The mechanisms are right; the counts drifted. Do not change behaviour to match the numbers.

### 3.5 Two smaller ones

- §1.11 read literally ("a CI job that runs `spex validate`") is satisfied by a step the 1g paragraph
  calls insufficient. Moot now that 1g is withdrawn.
- §1.5's rationale for the explicit refresh list does not support its conclusion: `requirement` is *on*
  the list, so both formulations absorb the requirement changes it warns about. The only thing the
  explicit list actually excludes relative to `!beadProducingTypes` is `meta`. The list is still right.

## 4. Constraints the proposal does not state

**Removing a component obliges a corpus sweep in the same part.** 1b's removal-time name check errors on
surviving prose mentions of a removed `api` or `component` name. 1h removed component `OrphanDetector`;
its 24 surviving mentions across 8 files had to be swept **inside 1h**, or 1b — the very next batch of the
same wave — could not have passed its own gate. Any later part that removes a component inherits this.

**Any byte change to a `module.json` moves `meta/<module>`** and obliges a changed content leaf on
**every** component in that module — unless a requirement in that module also changed, which puts it in
`reqModules` and skips the whole-module obligation (`completeness_checker.go:126-128`). This is the
single most common source of surprise `incomplete_change` errors.

**`meta/project` is structurally obligation-free.** `completeness_checker.go` only collects meta nodes
with `Module != ""`. So `spec/project.json` edits that touch no requirement are free — which is how W3
reconciled four `project.json`↔`module.json` description drifts at zero cost.

**Added and modified requirements are governed by different functions** — `checkAddedRequirement`
(needs ≥1 implementor, all implementors' nodes in `changedPaths`) versus `checkModifiedRequirement`.
Do not assume which applies.

**The two link scanners deliberately differ.** `validator/markdown_scanner.go` (Go, `CheckLinks`) skips
**fenced blocks only** — a link inside an HTML comment, a 4-space indented block or an inline code span
is still scanned and must resolve. `scripts/check-lib.sh` skips fences, HTML comments, indented blocks
and whole-link-in-span. Both are defensible ("must this resolve?" vs "does this count as work?"), but the
combination is a trap: a stale link parked in an `<!-- TODO -->` is a hard `validate` error, while a valid
one there satisfies no obligation. The advice that satisfies both: put backticks **inside** the display
half, never around the whole link.

## 5. W6 — the next wave

### Workload, measured at this boundary

**55 impl leaves / 55 declared impl_sections across 11 modules.** Zero typed links exist in the gated
corpus, so W6 is `CheckLinks`'s first real exercise.

| module | impl leaves | arch leaves | arch leaves with Go |
|---|---|---|---|
| adapters | 4 | 1 | **0** |
| cli | 3 | 3 | **0** |
| emit | 5 | 5 | 4 |
| impact | 5 | 5 | 4 |
| ingest | 4 | 4 | 2 |
| map | 4 | 3 | 3 |
| merkle | 7 | 7 | 6 |
| proposal | 4 | 4 | 2 |
| render | 5 | 5 | 4 |
| schema | 4 | 4 | 1 |
| validator | 10 | 10 | 5 |

`adapters` and `cli` have **zero** Go in arch, as the proposal predicts — step 4 is a no-op there, but
their fold and link obligations are unchanged. `merkle` and `validator` are the heaviest.

### The ladder record format is now enforced

`ladder-check.sh` validates the arm **name** against the arm **number** against §1.6's ladder
(`0 CONTRADICTS, 1 SYNTAX, 2 ALREADY SAID, 3 CODE'S JOB, 3.5 COST CLAIM, 4 FALSIFIABLE,
5 UNRECOVERABLE WHY, 6 otherwise`). **Free-form arm names such as `arm 3 MOVE` will fail.** It tolerates
case, `'`/`’`, and `(split n/m)` / `(1.7 exception)` parentheticals. `arm 0` surfaces as `ARM_ZERO` with a
non-zero exit, as the plan requires. **This is tighter than the proposal's prose — tell every W6 part.**

### `link-spread.sh` no longer uses shape heuristics

Two adversarial reviews established that a link dump cannot be told from honest prose by line shape: the
two densest real leaves (8 links in 8 lines, 11 in 12) are indistinguishable from a dump by position or
ratio, and the presentation forms Segment 2 **mandates** (numbered list, condition→outcome table) look
exactly like one. `LINK_CLUSTER` was removed. The primary rule is now `LINK_APPENDED`: **links added
since `<base>` with not one line of the leaf's prose changed.** An honest migration rewrites the sentence
the link sits in; a dump does not.

One documented gap: a few links each wrapped in ~4 words of filler under *no heading at all* is not
caught. Backstops: any heading → `LINK_HEADING`; link-only lines → `LINK_DUMP`.

### Known pre-existing red

`heading-check.sh spec/validator` reports `LOST_HEADING spec/validator/impl_test_coverage.md ::
Relationship to OrphanDetector`. That is 1h correctly removing a section documenting a retired component
— an in-place heading shrink that `diff` reports as nothing more interesting than `modified/impl_section`,
i.e. exactly the class the script exists to catch. **It self-resolves**: the script skips deleted leaves,
and W6 deletes that impl leaf. Do not add an allowlist.

## 6. Work routed to specific waves

### → W6 6b (cli)
Three tracked leaves still declare the retired `hash-id --type` values: `arch_hash_id_command.md`,
`impl_hash_id.md:45-48,60` (pins the deleted `case "milestone"` arms and the pre-change error string),
`test_hash_id_command.md:35-39` (S5 "Milestone hash" asserts exit 0; the Go test now asserts the
opposite). **`test_hash_id_command.md:41-45` S6 has no implementing test at all** — `TestFR58_S6_ScenarioHash`
was deleted and not replaced. That is a coverage hole, not just stale prose.

### → W6 6e (ingest)
Requirement `e68653819f38` *Refresh mode for impl_only drift* is stale **three** ways: (a) "refused if the
diff contains any added or removed entries" — false since W1's absorb list; (b) it calls `data_flow` and
`test_section` non-bead-producing — both **are** in `beadProducingTypes` and are explicitly refused;
(c) "Activated via proposal frontmatter `mode: refresh`; pipeline routing reads the frontmatter" — **no
frontmatter routing exists anywhere in the Go tree**; activation is `--mode refresh`. W4 could not fix it
because its sole implementor's leaf is already api-aware; W6 touches every arch leaf anyway.
⚠️ Its **title** is also wrong but is identity-bearing — fixing that is delete-plus-create and must not
happen in this migration.

### → W6 6i (render)
`test_renderers.md:140-148` (D6) asserts "No node ID contains … characters that would require quoting in
DOT" — the shipped `TestFR2_D6_ValidNodeIDs` now asserts the **opposite**; `:124-125` (D4) names edge
`alpha_comp_2 -> alpha_comp_1`; `impl_dot_generation.md:17-27` still specifies the composite
`<module>_<type>_<id>` form. Also: `api` is rendered by all three renderers but declared nowhere in the
render spec, and `flow_render_pipeline.md:72-76` says DOT labels are `<name>\n<id[:8]>` (code emits
`label="<name>"` only) and that JSON nodes are `{id, type, name, module, content_hash}` (code emits
`content`, `description` and `group`). New test scenario labels J9–J15, D8–D10, M7, S11, E10 trace to
nothing. `render/json.go:271`'s docstring still mentions milestones. `arch_spec_reader.md`'s `ModuleGraph`
fence declares `{Module, Content}`; the code is `{Module, Spec, Content}`.

### → W6 6j (schema)
`spec/schema/module.json` requirement `237fd8ffb610` still names milestones and `test_plan` scenarios.
Two parts tried and correctly refused: its second implementor obliges `arch_module_schema.md`, which has
zero milestone content, so only a manufactured touch was available. **The unlock: `arch_module_schema.md`
documents neither the `apis` property nor `$defs/api`, though the schema has carried both since W1.**
Documenting `apis` is the real edit that lets the milestone reference go. Also `test_schema_validation.md`
still specifies scenarios S16/E9/E10 and `test_schema_loading.md` still lists `milestones`/`test_plan` as
expected keys.

### → W6 6k (validator)
Requirements `4b399b1c568f` and `530dc49d7135` name milestones/scenarios and are stale since 1i removed
those ref checks. Go-isms survive in five arch leaves — that is step 4, as designed.

### → a wave whose fan-out covers five specific leaves
`spec/project.json` requirement `d5a8407d38e1` still says the validator checks "…no orphans, no cycles…".
The orphan checks were deleted outright. Three parts have now probed it: editing the description fires
7 `incomplete_change` naming `ModuleSchema` (schema) and `SchemaChecker`, `ContentResolver`, `DAGChecker`,
`NameConsistencyChecker` (validator). **W6 6j + 6k together cover exactly that set** — it becomes free
if they coordinate.

### → W7
`ingest/doc.go` carries the same stale "any added or removed entry" claim as the ingest spec.
No Go test covers the `cmd/spex/diff.go` removal-check wiring (`cmd/spex/*_test.go` was outside 1b's
write set), so the `SurvivingName → DiffError` mapping and the exit-2 path are proven by CLI runs only.
`impact/action_classifier.go:31`'s `beadProducingTypes["module"]` is a dead entry — no change can carry
`node_type: "module"`, since the module node has an empty `NodeType` and `flattenLeaves` collects only
leaves.

### → the delivery proposal (branch `proposal/delivery`)
Add `spex validate` and the guarded diff-errors gate to the CI pipeline component of
`spec/proposals/2026-07-25-tool-delivery.md` (~line 55; its fast gate is build/vet/test only).
**Do not write the gate as `bin/spex diff --json | jq -e '.errors == []'`** — the pipe masks `diff`'s exit
code, and on exit 1 `diff` writes nothing to stdout, so `jq` dies with an opaque parse error naming no
file. Capture to a file, preserve the status, branch on all three cases. "Required for merge" is a
branch-protection rule; the status check name is `spec`.

## 7. What W1 actually shipped, in case the proposal is read alone

- **`api` node type** — `apis` array, identity `<module>/api/<name>`, no content file, not
  bead-producing, `provided_by` module-local and referentially checked, **names globally unique across
  modules**.
- **A name-declarability rule.** An api or component `name` is rejected unless tokenizing it reproduces it
  exactly, in ≤6 whitespace-separated words. `spex validate [--json]`, `Validator (core)`, `Widget.` and
  `Bob's` are all **rejected**. Live audit at W1: 51 component names, 0 rejected.
- **`CheckIDDerivation`** — a module-scoped declared `id` must equal `IdentityHash(module, nodeType, name)`.
  **24 project-level ids are exempt** (15 requirements + the retired 6 milestones and 3 scenarios); they
  are legacy, do not derive, and are load-bearing snapshot and bead-map keys. **Never recompute them.**
- **Typed links** `[[<12-hex>|<display text>]]`, resolving against merkle leaf hashes. Module nodes are
  not linkable. A bare 12-hex token is ignored except inside a ```dot fence — where it resolves whether or
  not it is quoted, which matters because the DOT renderer **must** quote (144 of 229 ids begin with a
  digit, and unquoted they silently split into two nodes under a real DOT parser).
- **Removal-time name checking.** The name of a removed node is stored nowhere, so the check **recovers**
  it — hashing every 1–6-word corpus phrase as `IdentityHash(module, nodeType, phrase)` against the
  removed keys. A match is cryptographic proof. Failure mode is false *negatives*. Emits non-gating
  `notes` (`suppressed_by_live_name`, `unverifiable_module`) in a `notes` array present only when
  non-empty. Cost ~77 ms per (module, node_type) group.
- **`content` required and non-empty** on component, impl_section, data_flow and test_section. This
  replaced the orphan checks, which were deleted outright — **the validator now has zero warning sites**,
  and `warning_count` is a stable contract field that every gate asserts is 0.
- **`spex diff --map`**, and **`spex render --format json --slim`** (~24 KB against ~790 KB; the
  name→hash table).
- **Five mechanical check scripts** plus a shared `scripts/check-lib.sh`.

## 8. Things that were nearly shipped wrong

Recorded because they are the failure modes this migration is prone to, not as history.

- A **total defeat of the link obligation** — links dumped into a fence turning four gates green while
  doing zero work. Inherited from the proposal's own reference implementation.
- **A module no gate can see** — `/spec` told a module-scoped run it could create a new module while never
  writing `project.json`. A module absent from `project.json` is invisible to `validate`, `diff` **and**
  `render --slim`.
- **A brand-new requirement carrying a false claim** — 1j asserted cobra "enters the binary through the
  root command alone" when 14 non-test files import it.
- **Four false claims introduced by the debt sweep itself**, in a part whose purpose is removing false
  claims.
- **Two authoring skills teaching a rule the tool cannot apply** — `module` listed as bead-producing.

Every one was caught by an independent review, and none by a gate. The gates verify structure; only a
reader verifies truth.
