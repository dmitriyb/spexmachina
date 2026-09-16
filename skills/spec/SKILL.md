---
name: spec
description: "Read a proposal and author the spec through the authoring commands: declare, link, scaffold, write prose, validate"
argument-hint: "<proposal-path-or-name> [<module>]"
---

# /spec — Author Spec from Proposal

Read a proposal from `spec/proposals/drafts/` (or `spec/proposals/` once `/mint` has registered it) and turn it into spec: the structure through `spex`'s authoring commands, the prose by hand. This is the LLM interface for spec authoring, and the division is exact: **every change to `project.json` or a `module.json` goes through a command; the only thing this skill writes directly is prose inside a content leaf.**

The commands read the resolved profile, refuse a change with the validator's own error and a `fix` that names the correction, and print what the change obliges. That is why this skill carries no table of node types, arrays, id rules, name rules or file conventions: the binary holds them, and a refusal tells you what it holds. Build it first (`go build -o bin/ ./cmd/spex/`).

## Arguments

```
/spec <proposal-path-or-name> [<module>]
```

### First argument — the proposal

1. If it is a path to an existing file, use it directly.
2. If it is a name (no path separator), look for `spec/proposals/drafts/*-<name>.md`, then `spec/proposals/*-<name>.md`.
3. If it is empty, list `spec/proposals/drafts/` and `spec/proposals/` and ask the user which proposal to use.

Read the proposal fully before proceeding.

### Second argument — module scope (optional)

When present it names one module: the `name` of an entry in `spec/project.json`'s `modules` array, which is also that module's directory under `spec/`. A large proposal is normally authored one module per run, so that each run's diff is reviewable on its own.

A scoped run:

- **writes only inside `spec/<module>/`** — every command it issues names that module, and every leaf it writes is in that directory;
- **never changes `spec/project.json`**: no project requirement, no module entry, no `requires_module` edge. If the proposal needs one, say exactly what is needed and stop short of making it. Another run owns it;
- **cannot bring a new module into existence**, because `spex node add --type module` writes `project.json`. A module `project.json` already declares whose directory is missing is in scope: the entry exists, only the files are owed;
- **still runs the gates over the whole tree**, because every `spex` subcommand reads the whole spec directory and neither gate takes a module filter. "Which findings are yours" below says what a scoped run does with findings outside it.

With no second argument the run owns the whole tree.

## The commands

| Command | Does | Report |
|---|---|---|
| `spex profile show` | prints the resolved profile: types, plural keys, fields with kinds and reference targets, coverage chains, content prefix and leaf sections per content-bearing type | the profile |
| `spex node add <name> --type <t> [--module <m>] [--field k=v]…` | declares a node: file, array, id, required fields and content path all decided from the profile; a content-bearing node gets its leaf skeleton; `--type module` registers a module and writes its `module.json` | `written`, `obligations` |
| `spex node remove <id> [--force]` | removes a node and its leaf; refuses while anything still references it, listing each reference with the `spex edge remove` that retargets it; `--force` removes anyway and lists what it left dangling | `written`, `obligations`, `retired_name` |
| `spex node rename <id> <new-name>` | one transaction: new id, every reference field and every typed link repointed, the leaf moved | `written`, `obligations`, `retired_name` |
| `spex edge add <source-id> <field> <target-id>` | one entry on one reference field, with target, field, target-type and cycle checks; on a cardinality-one field it replaces the held target | `written`, `obligations`, `replaced_target` |
| `spex edge remove <source-id> <field> <target-id>` | the inverse; clearing a required cardinality-one field is refused | `written`, `obligations` |
| `spex leaf scaffold <id>` | writes a leaf skeleton — title, the profile's headings, one placeholder link per owed edge — for a node whose leaf is absent or empty; never overwrites prose | `written` |
| `spex migrate` | brings a spec from an earlier format version to the current one; adopters only | `written`, orphaned files |

Every command honours `--spec-dir`, reads and writes only under it, needs no initialised project, and exits `0` on success, `1` on an input error (stderr names the input), `2` on a refusal (the error document on stdout, each entry with a `fix`). A refusal writes nothing.

Reference fields — `implements`, `uses`, `describes`, `provided_by`, `preq_id`, `depends_on` — can be given at add time as `--field name=id` (comma-separated for lists) or wired afterwards with `spex edge add`. Field names are the profile's, read from `spex profile show`, never remembered.

Two things the commands do not do. **Editing a scalar field of an existing node** — a requirement's `description`, a `priority`, a `type` — has no command yet; it is the one JSON edit made by hand, on the entry alone, and `spex diff` reports what it obliges. **Writing prose** is yours.

## Reading a report

`obligations` is the completeness checker's list for *this* write, against the tree as the previous command left it: which leaves this change now owes a real edit. It is not a running total — a later command can close an earlier one's obligation (adding a test section closes a `test_coverage` entry; editing an obliged leaf closes an `incomplete_change` entry), and only `spex diff` against the session's starting snapshot reports the session as a whole. Read each report as the cost of that step; read `spex diff` at the end as the truth.

A `fix` is literal: a declared-type list, a field name and kind, the array that was searched, the declarable form of a name, the `edge remove` that unblocks a removal. Apply it and reissue. If a refusal names no fix you can apply — the command cannot express the edit the proposal needs — stop and report it as a gap in `spex`, never work around it by editing the JSON.

## Workflow

### Before anything: the working branch

The spec is authored on a dedicated working branch, cut as the first tool call of the run — before the log, before reading the proposal:

```bash
git fetch origin && git switch -c spec/<slug> origin/main
```

`<slug>` is the proposal stem without its date prefix (`2026-08-20-reconciler-split` → `spec/reconciler-split`). A failed fetch is a hard stop: report it and wait. A session already on a non-main branch carrying this proposal's work stays on it.

The expected end state: every edit of this run sits on that branch, and `main` receives it through a PR.

### 0. Keep the write reports

Every command's report is the authoring log: keep each one, in order, in the run's context. The `written` lists are what you report at the end; the `obligations` are what you still owe; the `retired_name` entries are what the vocabulary sweep starts from. Nothing is persisted; the reports live in this run only.

### 1. Read the proposal and the profile

- Read the resolved proposal file.
- Run `bin/spex profile show` and read it: the declared types and their plural keys tell you what a proposal's "component" or "endpoint" maps to; a type's `fields` tell you which values `spex node add` will require and which reference fields it may carry; the `coverage_chains` tell you what every new node owes before the gate is green — under the default profile, a project requirement owes a module requirement deriving from it, a module requirement owes an implementing component, a component owes a describing test section.
- Read `bin/spex render --format json --slim | jq -r '.nodes[] | "\(.type)\t\(.name)\t\(.id)\t\(.module)"'` for the ids of every node the proposal names. Existing ids are never recomputed, and an existing project requirement is never renamed: most of this project's project requirement ids predate the identity convention and a rename would orphan every journal event keyed off them. Change their fields, keep their ids.
- If a module scope was given, confirm it names a real module and read that directory in full.

### 2. Plan the spec graph

Before the first command, present the user with the graph the proposal implies, in `spex` terms:

- **Project requirements** — new ones with type and priority; existing ones touched, with a note that their ids are preserved.
- **Modules** — new entries, and the `requires_module` edges they need.
- **Per module**: requirements, components, apis, data flows, test sections, with their edges — which requirement each component implements, which components each test section describes, which components each api is provided by.
- **Apis** — the exact surface strings, as callers type them. An api is the invocation, never a signature; flags live behind it. Names are globally unique across modules and belong to the module owning the entry point.
- **Removals and renames** the proposal asks for, each with the sweep it implies.

Ask: "These nodes will be declared: `<list>`. Is anything missing?" Confirm or adjust before issuing a command. In a scoped run, also state the project-level or sibling-module edits the proposal needs and that this run will not make them.

### 3. Declare and link

Issue the commands in dependency order, so that each target exists before the edge that names it:

1. modules (`--type module`), then `requires_module` edges;
2. project requirements, then module requirements with their `preq_id`;
3. components with their `implements`, then `uses` edges;
4. apis, then their `provided_by` edges;
5. data flows with their `uses`;
6. test sections with their `describes`. A test section owes its own task only when it describes two or more components; a single-component section is bundled into that component's work, so group components whose scenarios naturally span them.

For each removal the proposal asks for: `spex node remove <id>`; read the inbound list it refuses with, retarget each reference with the `edge remove` and `edge add` it names, then remove again. Use `--force` only when the dangling references are ones this same run will retarget next. For each rename: `spex node rename`. Both print a `retired_name`; note it for step 6.

Read every report before the next command. A refusal's `fix` is the next command. Do not proceed past a refusal you have not resolved.

### 4. Write test leaves

Write tests before architecture prose, so the scenarios derive from requirements and contracts rather than from implementation decisions.

A test leaf holds module integration and acceptance scenarios and nothing else. Unit cases — one function or one component's input → output on a specific case, its return value or error asserted — are Go `_test.go` files beside the component and never appear in the spec, even when they read a fixture directory. `scripts/lens-test-shape.sh` rejects the code-formatted form of a unit case and a leaf without a Setup section; `/spec-review` judges the prose form.

The skeleton is already on disk with the profile's headings. Fill them:

- **Setup**: the ground every scenario shares — the sample spec or sandbox the Givens build on, invocation conventions, the gate that skips the leaf. Only what two or more scenarios use.
- **Scenarios**, each opening with a **Given** line: the state this scenario starts from; **When**: a `spex` subcommand invoked, or a path through two or more components, or a module boundary crossed, or an external system driven; **Then**: exit code, stdout, files, journal or tracker state observed. A When that names one component alone is a unit case and does not belong here.
- **Edge cases**: the same shape, for boundary conditions, error paths and invalid inputs.

For `describes >= 2`, the content must actually span the described components; content that names one component and one method is a unit test in disguise.

### 5. Write architecture and data-flow leaves

Every content-bearing node the run declared has a skeleton. Fill each one with substantive content synthesized from the proposal — not stubs.

**A touched leaf owes its declared edges as typed links.** A component leaf carries one `[[<id>|<display>]]` link for every entry in its `uses`, its `implements`, and every api whose `provided_by` names it; a data-flow leaf owes its `uses`. `scripts/link-check.sh` enforces it over the diff. Place each link in the sentence that discusses the target, replacing the scaffold's placeholder line where there is one and writing it fresh where there is not — the scaffold written at `node add` time carries placeholders only for edges that existed when it was written. `scripts/link-spread.sh` fails a leaf that appends links without changing a line of prose, grows a `## References`-style heading, or writes link-only lines. Backticks go inside the display half, never around the whole link; a link inside a fence, an HTML comment or a 4-space-indented block does not satisfy the obligation. Get any other id you need from the slim render; module nodes are not linkable.

**An obliged leaf owes a real edit.** When a report or `spex diff` says `module X meta changed but component Y content leaf unchanged`, or `requirement R changed but component C content leaf unchanged`, the leaf must say what changed about that node's call sites, test surface, relationships or behaviour. Cosmetic edits do not discharge it.

#### Deciding what belongs in an arch leaf

**Arch leaves carry no Go.** An `arch_*.md` leaf states behaviour a caller can observe. It carries no ```` ```go ```` fence, no function signature, no `func` keyword, and no sentence whose subject is a language identifier. The code is the source of truth for how; the leaf is the source of truth for what and why. Diagrams are welcome — an ASCII or DOT diagram whose every arrow connects two named things earns its place.

**The one exception: pseudocode where the algorithm is the requirement.** Keep a fence, rewritten as language-neutral pseudocode, when the destination component implements a requirement whose *description* names the algorithm or the bound the fence encodes. The test is the description, not the requirement's `type`. When you keep pseudocode, name the requirement it belongs to in the surrounding prose.

**The ladder — judging existing implementation prose.** This is the standing test for whether a paragraph belongs in an arch leaf at all. The unit of judgement is a `##` section, and every section gets exactly one recorded verdict.

Preamble, once per component: P1 — the destination is chosen by requirement, not by `describes`: content belongs in the arch leaf of the component that `implements` the requirement the content documents. P2 — read the destination arch leaf to completion first and write down its `##` headings. P3 — the unit of verdict is a `##` section; the H1 and any prose before the first `##` is `[preamble]`; a section mixing a language fence with prose that would take a different arm is split at the fence boundary, one verdict per part, recorded.

First match wins:

| # | Arm | Test | Verdict |
|---|-----|------|---------|
| 0 | CONTRADICTS | asserts what the arch leaf denies | Resolve against the code, keep one statement, record the loser in a `drifts/` report for `/drift`. Never silently pick. |
| 1 | SYNTAX | the load-bearing content is a language fence, or a sentence whose subject is a language identifier | Delete — but first ask: does it assert something a caller can observe (stdout, exit code, a file written, an ordering, a naming convention)? If yes, restate it language-neutrally into the arch section whose subject it shares, creating one only if none exists, then delete. |
| — | *exception* | the destination component implements a requirement whose description names the algorithm or bound the fence encodes | Keep, as language-neutral pseudocode. |
| 2 | ALREADY SAID | the arch leaf has a section or sentence whose subject is the same node, artifact, field or condition | Delete. If the impl wording is more precise, replace the arch words in place; the section count does not grow. |
| 3 | CODE'S JOB | a fact about a git-tracked file, or an instruction to a future implementer | Delete. |
| 3.5 | COST CLAIM | an asymptotic complexity, benchmark or resource figure | Delete, unless the destination implements a requirement naming a bound — then move it as one sentence. A cost claim never creates a section. |
| 4 | FALSIFIABLE | a reader with the built binary could prove it wrong | Move, rephrased to name no language. A new section is allowed. |
| 5 | UNRECOVERABLE WHY | a rejected alternative, a constraint imposed from elsewhere, a historical reason | Move, compressed. |
| 6 | otherwise | — | Delete. |

Arm 2 sits above arm 3 deliberately: the dominant reason to delete is "the arch leaf already says this", not "this duplicates code".

**What each leaf is for.** Component (`arch_`): what the component is, its responsibilities, the behaviour a caller can observe, its contracts with the components in `uses`, and the rationale not recoverable from the code. Data flow (`flow_`): how data moves between the components in `uses` — input shape, transformations, output shape, error paths. If the proposal lacks detail for a node, write what you can and mark the gap with `<!-- TODO: detail needed -->`; an HTML comment is not invisible to the link checker, so do not park a link inside one.

### 6. Gates

**Structural pass.**

```bash
bin/spex validate | jq -e '.valid == true and .warning_count == 0'
```

Every finding is an error; there are no warnings and no `--json` flag. Its entries are the same predicate the commands applied at each write, so what it can still find is what the prose step introduced or what an obligation left open. Fix everything in scope here before the completeness pass.

**Completeness pass.**

```bash
bin/spex diff --json
```

Exit `0` clean, `2` with a non-empty `errors` array, `1` when the tree did not build, `3` when the project is not initialised or is broken. This pass must end with `errors: []`: `spex plan` refuses a diff with errors outright, so an unresolved finding blocks the user's mint. Two entry types appear: `incomplete_change`, a structural change whose consequences were not written down — the obliged leaf owes a real edit; and `surviving_name`, a removed or renamed component or api whose name still appears in the corpus outside `spec/proposals/` — start from the `retired_name` entries in your reports, `git grep` the name across `spec/`, and rewrite every leaf that still presents it as current, keeping deliberate negative or historical mentions. `spex diff` may also print notes; they are disclosures, never violations, and never affect the exit code.

**Review pass.** Run `scripts/link-check.sh spec diff.json` over the saved `spex diff --json` output, and `scripts/link-spread.sh spec/<module>` for each touched module (it compares against `origin/main` by default, or a baseline directory given as its second argument); `/spec-review` runs both regardless. Then walk your reports: every leaf under a touched module is referenced by a `content` field, every api name is the string a caller types, and every test section that describes two or more components actually spans them.

**Which findings are yours.** Both gates read the whole tree. In a scoped run, fix every finding whose `path` or `related` names a node inside `spec/<module>/`, re-run, and judge only the residue. If the residue is empty the gates passed. If it is not, the gates did not pass and this run does not make them pass: do not edit `spec/project.json` or a sibling module to clear it; report each remaining finding verbatim with the command that produced it, as work another run owns. A scoped run ending with a residue is a correct outcome.

**Loop and escape.** If the gates surface findings inside the scope: attempt 1, revise and re-run; attempt 2, revise and re-run; after attempt 2 fails, halt auto-correction and offer the user `skip` (leave the findings; they adjust by hand before the pipeline) or `one more round`. Print `spec: sanity gates passed (N nodes touched across M modules)` only when both gates are green with no residue.

### 7. Report

Tell the user:

- The files written — the union of the `written` lists — and any `<!-- TODO -->` markers that need follow-up.
- Every `retired_name` and the sweep done for it.
- Any hand edit made under the scalar-field exception, and any refusal that exposed a command gap.
- In a scoped run, the project-level or sibling-module edits the proposal still needs, and any out-of-scope gate findings left in place.
- Which nodes look like absorb candidates and why — advisory only; the classification is `/mint`'s, made against the committed diff.
- Remind them to review and commit on the working branch, landing on `main` via PR. The mint runs against that commit.

## Handing the spec to the pipeline

`/spec` writes the spec and stops: gates green, edits committed by the user, baseline untouched. Everything after — the per-node mint-vs-absorb assessment, the adapter's export of the task-state artifact, `spex plan --tasks`, the adapter's apply half, `spex ingest`, the refresh pathway — is `/mint`'s. Author with it in view: a rename or removal reaches the pipeline as a removal plus an addition with no lineage edge, the journal is the lineage; a node whose earlier task is finished gets one plain create; a claimed (`in_progress`) task's node must not change, so mint a module's changes in one run rather than across several while tasks are in flight.
