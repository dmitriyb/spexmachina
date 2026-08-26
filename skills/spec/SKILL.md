---
name: spec
description: "Read a proposal and author spec files: project.json, module.json, and markdown content leaves"
argument-hint: "<proposal-path-or-name> [<module>]"
---

# /spec — Author Spec from Proposal

Read a proposal from `spec/proposals/` and create or modify the spec: `project.json`, `module.json` files, and markdown content leaves. This is the LLM interface for spec authoring.

`bin/spex validate` is the arbiter of whether the structural output is correct. Build it first (`go build -o bin/ ./cmd/spex/`) and run it often — several rules below are enforced by the validator and not by the JSON Schema, so a file that looks schema-clean can still fail.

## Arguments

```
/spec <proposal-path-or-name> [<module>]
```

### First argument — the proposal

1. If it is a path to an existing file, use it directly.
2. If it is a name (no path separator), look for `spec/proposals/*-<name>.md`.
3. If it is empty, list `spec/proposals/` and ask the user which proposal to use.

Read the proposal fully before proceeding.

### Second argument — module scope (optional)

When present it names one module: the `name` of an entry in `spec/project.json`'s `modules` array, which is also that module's directory under `spec/`. A large proposal is normally authored one module per run, so that each run's diff is reviewable on its own.

A scoped run:

- **writes only inside `spec/<module>/`** — that module's `module.json` and its content leaves;
- **never writes `spec/project.json`** and never writes another module's directory. If the proposal requires a project-level change (a new project requirement, a new module entry, a `requires_module` edge) or an edit in a sibling module, say exactly what is needed and stop short of making it. Another run owns it;
- **cannot bring a new module into existence**, because bringing one into existence means writing `spec/project.json`. A module directory `project.json` does not name is invisible to every gate: over a `spec/widgets/` holding a schema-clean `module.json` and its content leaves, `bin/spex validate` reports `{"valid":true,"error_count":0}`, and the module appears in neither `bin/spex diff` nor `bin/spex render --format json --slim`. Creating one without the `project.json` entry produces a module no gate can see. See "Detect Mode";
- **still runs the gates over the whole tree**, because every `spex` subcommand reads the whole spec directory — and neither gate can be scoped down, so a finding in another module holds them red no matter what this run does. Fix every finding whose `path` or `related` names a node inside the scoped module, re-run, then judge only what is left. Report the rest verbatim and leave it alone; never edit outside `spec/<module>/` to turn a gate green. "Sanity gates" says exactly what a scoped run does when the residue is non-empty.

With no second argument the run owns the whole tree.

## Detect Mode

Check the current state of `spec/`:

| Condition | Mode | What to do |
|-----------|------|------------|
| `spec/project.json` does not exist | **New project** | Create `project.json` + all module dirs, `module.json` files, and markdown leaves |
| `spec/project.json` exists, proposal adds new modules | **New module** | Add module entries to `project.json`, create new module dirs with `module.json` and markdown leaves |
| `spec/project.json` exists, proposal modifies existing nodes | **Alter** | Modify existing JSON and markdown files in place |
| `spec/project.json` exists, proposal adds new modules AND modifies existing nodes | **New module + Alter** | Both actions apply — add new modules and modify existing nodes in a single pass |

**A module-scoped run is always Alter.** It can never be **New project**, and it can never be **New module** in the sense of the table's New module row: adding a module entry is a `spec/project.json` write, and the scope argument can only name a module `project.json` already lists, so a scoped run has nothing to add there.

The one case that looks like new-module work and *is* in scope: a module `project.json` already declares whose `spec/<module>/` directory does not exist yet. Creating that directory, its `module.json` and its leaves stays inside the scope, because the entry that makes them visible is already committed. If instead the proposal asks for a module `project.json` does not name, stop and say so — a whole-tree run has to add the entry first. Create the directory without it and nothing downstream can see it (see "Second argument — module scope").

Tell the user which mode was detected and why before proceeding.

## Node types

| Type | Declared in | Content leaf | Produces a bead |
|------|-------------|--------------|-----------------|
| project requirement | `project.json` → `requirements` | no | no |
| module | `project.json` → `modules` | no — it surfaces in a diff as a `meta/<hash>` leaf hashed from `module.json` | no |
| module requirement | `module.json` → `requirements` | no | no |
| component | `module.json` → `components` | `arch_*.md`, **required** | yes |
| api | `module.json` → `apis` | **no** | no |
| data flow | `module.json` → `data_flows` | `flow_*.md`, **required** | yes |
| test section | `module.json` → `test_sections` | `test_*.md`, **required** | only when `describes` has ≥ 2 entries |

**Node types that no longer exist:** `impl_sections`, `milestones` and `test_plan` (with its `scenarios`). Impl sections are gone from `module.schema.json`, from `bin/spex hash-id` and from the corpus — every fact one used to carry belongs in the component's `arch_*.md` or nowhere; see "Deciding what belongs in an arch leaf". Milestones and the test plan are gone from `project.json`. `project.schema.json` declares `additionalProperties: false`, so writing either one back is a hard schema failure — one JSON error entry with `check: "schema"`, `path: "project.json"`, and a combined message:

```
additional properties 'milestones', 'test_plan' not allowed
```

and `bin/spex hash-id --type milestone` / `--type scenario` exit 1. Milestones were superseded by proposal epics; scenarios by test sections whose `describes` spans two or more components.

## Schema Reference

All JSON output must conform to `schema/project.schema.json` and `schema/module.schema.json`. Read both before writing any JSON. Key rules, including the ones only the validator enforces:

### project.json

- **Required**: `name`, `modules` (at least one)
- **Optional**: `description`, `version`, `requirements`, `sections`
- Requirements: `id` (identity hash), `type` (`functional` | `non_functional`), `title` — required by schema; **`priority` (integer 0–4) is required by the validator**, which is the one mandatory field the schema calls optional. Omitting it yields an error entry with `check: "id"`, `path: "project.json:/requirements/<id>"` and message `project requirement <id> missing priority`.
  `description` and `depends_on` are optional.
- Modules: `id`, `name`, `path` required; `description`, `requires_module` optional. **Module `name` must be lowercase and must match the `name` field in the corresponding `module.json` exactly** (e.g. `"plan"`, not `"Plan"`).
- `sections`: project-level typed envelopes (`id`, `name`, `type`). A section of type `coupled` must name an existing module and validate against that module's `section.schema.json`. This project declares none; do not add one unless a proposal asks for it.

### module.json

- **Required**: `name`
- **Optional**: `description`, `requirements`, `components`, `apis`, `data_flows`, `test_sections`
- Requirements: `id`, `type`, `title`, **`preq_id`** all required — `preq_id` is the identity hash of the project requirement this one derives from. `description`, `depends_on` optional. A module requirement **must not** carry `priority`: the module `requirement` definition sets `additionalProperties: false` and has no such property, so it is a schema error.
- Components: `id`, `name`, **`content`** required, `content` non-empty (`minLength: 1`); `description`, `implements`, `uses` optional. If users or other systems invoke the module externally, that entry point is a component **and** should also be declared as an `api`.
- APIs: `id`, `name` required; `description`, `provided_by`, `group` optional. **No `content`.**
- Data flows: `id`, `name`, **`content`** required and non-empty; `description`, `uses` optional
- Test sections: `id`, `name`, **`content`** required and non-empty (path to `test_*.md`); `describes` (component IDs) optional

`content` is required and non-empty on component, data_flow and test_section. A node with empty `content` is silently skipped by the merkle tree builder, which means it is not a leaf, produces no diff, and cannot be linked to — the schema now rejects it outright rather than letting it disappear.

### `api` — the external surface node

An `api` is one entry point exactly as callers write it: `spex diff`, `GET /v1/specs/{id}`, `schema.IdentityHash`. **Never a signature.**

**The external surface is fully declared: 9 of the 11 modules carry an `apis` array, 15 api names in total, covering every CLI subcommand** (`schema` and `adapters` own no entry point and correctly declare none). Before adding an api, check it does not already exist in another module — names are globally unique and the identity belongs to the module owning the entry point. The block below is a quotation of `spec/merkle/module.json`, declaring `spex diff` whose component is `DiffCommand` (`c8b958ec310d`):

```json
"apis": [
  { "id": "d487fc9c4fa5", "name": "spex diff",
    "description": "Compute changes between snapshot and current spec",
    "provided_by": ["c8b958ec310d"], "group": "cli" }
]
```

`d487fc9c4fa5` is `bin/spex hash-id --type api --module merkle --name "spex diff"`. Note that the api is declared in the module that owns the entry point, not in `cli`: `provided_by` cannot reach across modules.

- Identity is `<module>/api/<name>` — `bin/spex hash-id --type api --module <mod> --name "<name>"`.
- **No content file.** An api hashes from its JSON fields alone, exactly as a project requirement does. It is still a merkle leaf, so it is a valid link target.
- **Not bead-producing.** An added, modified or removed api yields zero create/obsolete actions; its impact level is `contract`, and the components behind it carry the work.
- **`provided_by` is module-local** and referentially checked: every hash in it must be a component declared in *this* `module.json`. A real component id from another module is as wrong as one that exists nowhere —
  `provided_by references non-existent component <id> (provided_by is module-local)`.
  Another module's involvement is already expressed by component `uses` edges.
- **`group` is freeform** and `spex` never branches on it. It exists so renderers can group a surface that is part CLI and part HTTP.
- **API names are globally unique across every module** — the one uniqueness rule in the spec that is not per-array:
  `duplicate api name "demo run"; api names are globally unique, declared by: demo, other`.
- Renaming an api is delete-plus-create.
- **The blind spot is surface drift nobody writes down — not the `description`.** The leaf hashes `description`, `group`, `id`, `name` and `provided_by`, so a *description-only* edit does move the hash: `bin/spex diff` reports the api as `modified   contract   <module>   <id>`. What is genuinely invisible is a change to the external surface that never reaches `module.json` at all — an added `--flag`, a new query parameter, a changed response field. The api has no content file and nothing in the spec records its surface beyond the name, so such a change moves no hash and produces no diff entry. The `description` is therefore the one field where that drift can be *made* visible, and writing it there is a tracked change, not a silent one.

### IDs

All IDs in `project.json` and `module.json` are 12-character lowercase hex identity hashes (pattern `^[a-f0-9]{12}$`). They are computed deterministically from the node's position in the spec graph — never assigned manually, never sequential. The same identity string always produces the same hash.

The hash is `SHA256(identity_string)` truncated to the first 6 bytes (12 hex chars), where `identity_string` is the parts below joined with `/`:

| Node type | Identity string |
|-----------|-----------------|
| Project requirement | `project/requirement/<title>` |
| Module | `module/<name>` |
| Module requirement | `<module>/requirement/<title>` |
| Component | `<module>/component/<name>` |
| API | `<module>/api/<name>` |
| Data flow | `<module>/data_flow/<name>` |
| Test section | `<module>/test_section/<name>` |

Use `bin/spex hash-id`. It calls the same `schema.IdentityHash` helper the rest of the pipeline uses:

```bash
bin/spex hash-id --type requirement  --name "Spec test strategy"
bin/spex hash-id --type requirement  --module merkle --name "Classify impact"
bin/spex hash-id --type component    --module merkle --name "ImpactClassifier"
bin/spex hash-id --type api          --module merkle --name "spex diff"
bin/spex hash-id --type data_flow    --module merkle --name "Hash computation flow"
bin/spex hash-id --type test_section --module ingest --name "Snapshot tests"
bin/spex hash-id --type module       --name merkle
```

Valid `--type` values, and the whole list:

```
requirement, component, data_flow, test_section, api, module
```

Anything else exits 1 with `hash-id: unknown type "<x>"; valid types: …`. `--module` is required for `component`, `data_flow`, `test_section` and `api`; it is omitted for `module` and for project-level requirements, and supplying it to `--type requirement` is what makes the hash module-scoped.

**Never hand-write a module-scoped id.** The `id_derivation` checker recomputes every one of them and errors when a declared id is not its own identity hash:

```
component "Widget" declares id aaaaaaaaaaaa but its identity hash is e44c856b0ff8;
a module-scoped id must equal IdentityHash("demo", "component", "Widget") or the node
cannot be recovered from its hash after removal
```

This covers module requirements, components, data_flows, test_sections and apis. Project-level **requirement** ids are the sole exemption, for the reason immediately below. Module ids in `project.json` are not checked, but do derive them anyway.

> **Legacy project requirement hashes — never recompute them.**
> 15 of the 18 requirements in `spec/project.json` predate the identity-hash convention and carry ids `bin/spex hash-id` cannot reproduce (`Render spec` declares `6b00623735ac` where the computed hash is `060ca1db054d`). They are **exempt, not correct**. Recomputing one rewrites the snapshot and orphans every task-journal event keyed off it, and destroys the lineage of every bead already filed against it. When you touch an existing project requirement, **keep its `id` byte-for-byte** and change only the fields the proposal asks for. Only a genuinely new project requirement gets `bin/spex hash-id --type requirement --name "<title>"`.

Changing a node's `name` or `title` changes its identity hash — the pipeline treats it as delete + create. Rename with care, and remember the id must be regenerated to match the new name.

There are no integer IDs in the system. The task journal (`.spex/history.jsonl`) files every change event under the changed node's own identity hash, and keys the event itself by an `eid`: `<git_head>:<op_id>` for an event a mint produced from an op, `refresh:<node>:<before>:<after>` (plus a `#N` suffix on the rare collision) for one produced without an op behind it — a whole-run refresh, or a node absorbed inside a normal run, which are indistinguishable on the wire. A tracker label carries that eid — `spex:<eid>` — but the label is optional insurance an adapter may stamp, not identity: the journal is what every downstream stage reads. The older `spex:<spec_node_id>` labels still on beads predate the eid scheme.

### Declarable names — component and api

A component or api `name` is rejected unless **tokenizing it reproduces it exactly, in at most 6 whitespace-separated words.** This is not a style rule. When a node is removed, its name exists nowhere any more; the removal-time sweep recovers it by hashing corpus phrases against the removed identity hash, and every phrase it can build is a join of corpus tokens. A name that is not itself such a join can never be matched, so the removal could never be swept — the validator refuses it at declaration time.

Tokenization strips `` ` `` `*` `_` `"` `'` `(` `)` `[` `]` `,` `;` `:` `!` `?` and quotation marks from both ends of each word, plus trailing periods and a trailing possessive `'s`. It does **not** strip `/`, `-`, `{`, `}`, `<`, `>` or `|`.

Rejected, with what the validator tells you to declare instead:

| Rejected | Reduces to | Why |
|----------|-----------|-----|
| `spex validate [--json]` | `spex validate --json` | brackets stripped |
| `Validator (core)` | `Validator core` | parentheses stripped |
| `Widget.` | `Widget` | trailing period stripped |
| `Bob's` | `Bob` | possessive stripped |
| `a b c d e f g` | — | 7 words; at most 6 |

Accepted: `Widget`, `spex diff`, `spex render --format json --slim`, `GET /v1/specs/{id}`, `schema.IdentityHash` — an interior dot is part of the name, a trailing one is not.

Write the name the way a caller types it, then check it before committing to it: if `bin/spex validate` accepts it, the removal sweep can find it.

### Edges

| Edge | From | To | Field |
|------|------|----|-------|
| `depends_on` | requirement | requirement (same scope) | `depends_on: [id, ...]` |
| `requires_module` | module | module | `requires_module: [id, ...]` |
| `preq_id` | module requirement | project requirement | `preq_id: id` |
| `implements` | component | module requirement | `implements: [id, ...]` |
| `uses` | component | component (same module) | `uses: [id, ...]` |
| `provided_by` | api | component (**same module**) | `provided_by: [id, ...]` |
| `uses` | data flow | component | `uses: [id, ...]` |
| `describes` | test section | component | `describes: [id, ...]` |
| `described_in` | node | markdown leaf | `content: "path.md"` |

`requires_module`, `depends_on` and component `uses` must each stay acyclic; the `dag` checker reports cycles.

### Typed links between content leaves

Prose in a content leaf refers to another spec node with

```
[[<12-hex identity hash>|<display text>]]
```

- The target is an **identity hash**, never a name. A name carries no `<type>` segment, so it cannot be turned back into a hash, and names are ambiguous across types within one module.
- **Display text is free-form and never checked.** It may contain a code span: ``[[3f9a1c7b2e04|`spex validate`]]``. Put the backticks *inside* the display half, never around the whole link — **inline code spans are not tracked by the scanner**, so wrapping a `[[…]]` in backticks does not stop it being resolved. Prose that needs to show the link syntax literally belongs in a fence.
- **Module nodes are not linkable.** Only merkle leaves are link targets, and a module is not a leaf: `link target <id> is a module node; module nodes are not linkable`.
- A **bare 12-hex token in ordinary prose is ignored** — the corpus quotes hashes as data all the time. The one exception is inside a ```` ```dot ```` fence, where a bare hash is a DOT node ID and *is* resolved.
- ``[[…]]`` inside a fenced code block is skipped entirely (this is what keeps bash `[[ -f x ]]` tests from being read as links). **HTML comments and 4-space-indented code blocks are *not* skipped** — a link written inside either one is scanned and must resolve.
- A link may wrap across a hard line break. It must close before the next blank line, fenced block or end of file, or it is reported as `unterminated link …`.

Get the hash for a link from the slim render, which is exactly a name → hash table:

```bash
bin/spex render --format json --slim | jq -r '.nodes[] | "\(.name)\t\(.id)\t\(.type)"'
```

The four link failures, all reported by the `link` checker at `<file>:<line>`:

```
link [[Widget|the widget]] target "Widget" is not a 12-character identity hash; links are hash-based, not name-based
link [[e44c856b0ff8]] has no display text; write [[e44c856b0ff8|<display text>]]
link target 8615746fb371 is a module node; module nodes are not linkable
link target deadbeefdead does not resolve to any spec node
```

Nothing forces an arbitrary mention to be a link — but **touched leaves owe their declared edges**, and `scripts/link-check.sh` enforces it: a component leaf that is `added` or `modified` in the diff must carry one link for every id in its `uses`, its `implements`, and every api whose `provided_by` names it; a touched data-flow leaf owes its `uses`. A link inside a fenced block, an HTML comment, a 4-space-indented block or a whole-link code span does **not** satisfy the obligation — the obligation scanner skips all four even where the validator's resolver scans them, so the safe form is the link in running prose with backticks inside the display half. Place each link in the sentence that discusses the target, replacing or wrapping the existing mention: `scripts/link-spread.sh` fails a leaf that appends links without changing a line of prose (`LINK_APPENDED`), grows a `## References`-style heading, or writes link-only lines. Run both scripts before calling a leaf done — `/spec-review` runs them regardless. Beyond the obligation, link when a reader would otherwise have to guess which node you mean.

## Interface Mapping

The schema has three ways to model an interface. When migrating a legacy spec that has an Interfaces section:

- **External entry points** (CLI subcommands, HTTP routes, exported library functions) → declare an **`api`**, and a component for the code behind it
- **Behavioral contracts** (ABI stability, serialization format, protocol guarantees) → model as **requirements**
- **Structural interfaces** (data loaders, visualization, export) → model as **components**

## File Layout

### Directory structure

```
spec/
  project.json
  proposals/
    YYYY-MM-DD-name.md
  <module-path>/          ← path from project.json module entry
    module.json
    arch_<name>.md        ← component content
    flow_<name>.md        ← data_flow content
    test_<name>.md        ← test_section content
```

### Content path conventions

Content paths in `module.json` are relative to the module directory:

| Node type | Filename pattern | Example |
|-----------|-----------------|---------|
| component | `arch_<snake_name>.md` | `arch_schema_checker.md` |
| data_flow | `flow_<snake_name>.md` | `flow_validation_pipeline.md` |
| test_section | `test_<snake_name>.md` | `test_schema_validation.md` |

Use lowercase snake_case for the `<name>` portion — a short slug derived from the node name. APIs have no file. Content paths may not be absolute and may not contain `..`.

## Deciding what belongs in an arch leaf

### Arch leaves carry no Go

An `arch_*.md` leaf states behaviour a caller can observe. It carries **no ```` ```go ```` fence, no function signature, no `func` keyword, and no sentence whose subject is a language identifier.** The code is the source of truth for how; the leaf is the source of truth for what and why. A leaf that restates a signature goes stale the first time the signature changes, and nothing catches it.

Diagrams are welcome — an ASCII or DOT diagram whose every arrow connects two named things earns its place.

### The one exception: pseudocode where the algorithm *is* the requirement

Keep a fence, rewritten as **language-neutral pseudocode**, when the destination component implements a requirement **whose description names the algorithm or the bound the fence encodes**.

The test is the requirement's **description**, not its `type`. `schema`'s `cdc9c58ba097` "Identity hash algorithm" is typed `functional`, and its description *is* the algorithm ("joins them with `/`, computes SHA256, and returns the first 6 bytes as a 12-character lowercase hex string"). A type-based filter would delete the one fence this exception exists to keep.

When you keep pseudocode, name the requirement it belongs to in the surrounding prose.

### The ladder — judging existing implementation prose

This is the standing test for whether a paragraph belongs in an arch leaf at all (the impl-leaf folding it was built for is complete — no `impl_*.md` exists in the corpus). The unit of judgement is a `##` section, and every section gets exactly one recorded verdict; judging whole leaves does not reproduce between runs.

**Preamble, once per component:**

- **P1 — the destination is chosen by requirement, not by `describes`.** The content belongs in the arch leaf of the component that `implements` the requirement the content documents, which was often not a component the retired impl section named.
- **P2 — read the destination arch leaf to completion first and write down its `##` headings.** Freezing that list before reading the source is what makes arm 2 mechanical.
- **P3 — the unit of verdict is a `##` section.** The H1 and any prose before the first `##` is one section, `[preamble]`. If a section mixes a language fence with prose that would take a different arm, split it at the fence boundary and record one verdict per part. Splitting is recorded, never silent.

**The ladder — first match wins:**

| # | Arm | Test | Verdict |
|---|-----|------|---------|
| 0 | CONTRADICTS | asserts what the arch leaf denies | Resolve against the code, keep one statement, record the loser in a `drifts/` report for `/drift-fix`. Never silently pick. |
| 1 | SYNTAX | the load-bearing content is a language fence, or a sentence whose subject is a language identifier | Delete — but first ask: does it assert something a caller can observe (stdout, exit code, a file written, an ordering, a naming convention)? If yes, restate it language-neutrally **into the arch section whose subject it shares**, creating one only if none exists, then delete. |
| — | *exception* | the destination component implements a requirement **whose description names the algorithm or bound the fence encodes** | Keep, as language-neutral pseudocode. |
| 2 | ALREADY SAID | the arch leaf has a section or sentence whose **subject** is the same node, artifact, field or condition | Delete. If the impl wording is more precise, replace the arch words **in place**; the section count does not grow. |
| 3 | CODE'S JOB | a fact about a git-tracked file, or an instruction to a future implementer | Delete. |
| 3.5 | COST CLAIM | an asymptotic complexity, benchmark or resource figure | Delete, unless the destination implements a requirement naming a bound — then move it as one sentence. A cost claim never creates a section. |
| 4 | FALSIFIABLE | a reader with the built binary could prove it wrong | Move, rephrased to name no language. A new section is allowed. |
| 5 | UNRECOVERABLE WHY | a rejected alternative, a constraint imposed from elsewhere, a historical reason | Move, compressed. |
| 6 | otherwise | — | Delete. |

Arm 2 sits above arm 3 deliberately: the dominant reason to delete is "the arch leaf already says this", not "this duplicates code", and asking the duplication question first produces arch leaves that state the same thing twice. Expect roughly 70% of sections to be deleted.

### Markdown content leaves

Each markdown file is a content leaf. Write substantive content — these are the documents implementers read.

- **Component (`arch_*.md`)**: what this component is, its responsibilities, the behaviour a caller can observe, its contracts with the components in `uses`, and the rationale that is not recoverable from the code. No Go.
- **Data flow (`flow_*.md`)**: how data moves between the components in `uses` — input shape, transformations, output shape, error paths.
- **Test section (`test_*.md`)**: module integration/acceptance coverage for the components in `describes` — setup, inputs, expected outputs, edge cases. NOT unit tests (those are Go `_test.go` files).

## Workflow

### Before anything: the working branch

The spec is authored on a dedicated working branch, cut as the first tool call of the run —
before the log, before reading the proposal:

```bash
git fetch origin && git switch -c spec/<slug> origin/main
```

`<slug>` is the proposal stem without its date prefix (`2026-08-20-reconciler-split` →
`spec/reconciler-split`). A failed fetch is a hard stop: report it and wait — the branch is cut
from a freshly fetched `origin/main`. A session already on a non-main branch carrying this
proposal's work stays on it.

The expected end state: every edit of this run sits on that branch, and `main` receives it
through a PR. In an interactive session this step is the sole carrier of that state — the
portitor gate binds the autonomous boxes — so the branch exists before the first spec file is
written.

### 0. Maintain an authoring log

Throughout this run, keep an in-memory append-only log of every spec node you create or modify. Each entry records:

- Module name
- Node type (`module`, `requirement`, `component`, `api`, `data_flow`, `test_section`)
- Identity hash (computed via `bin/spex hash-id`)
- Node name
- File path of any content leaf written
- Source: which proposal section / requirement drove this node

The log is the input to the per-node check (steps 3–5) and the end-of-session gate (step 6). It does not need to be persisted — it lives in your reasoning context for this run only.

### 1. Read proposal and schemas

- Read the resolved proposal file
- Read `schema/project.schema.json` and `schema/module.schema.json`
- In new-module or alter mode, read the existing `spec/project.json` and the relevant `module.json` files
- If a module scope was given, confirm it names a real module and read that directory in full

### 2. Plan the spec graph

Before writing files, present the user with a summary:

- **Project-level requirements** — IDs and titles (and, for existing ones, a note that their ids are preserved verbatim)
- **Modules** — IDs, names, paths, inter-module dependencies
- **For each module**: requirements, components, apis, data_flows, test_sections — with edges shown
- **APIs** — the external surface strings, exactly as callers write them

Ask the user:

- "These modules will be created: `<list>`. Is anything missing?" — the user may identify modules the proposal implies but that you overlooked (CLI, API, UI layers).
- Confirm or adjust before writing files. This is the spec review gate.

In a module-scoped run, also state which project-level or sibling-module edits the proposal needs and that this run will not make them.

### 3. Write JSON files

- Write `spec/project.json` (or update it in alter/new-module mode) — **not in a module-scoped run**
- Create module directories under `spec/<module-path>/`
- Write `spec/<module-path>/module.json` for each module in scope
- Derive every module-scoped `id` with `bin/spex hash-id`; never type one by hand
- Give every project requirement a `priority` (0–4)
- Use 2-space indentation for JSON
- **Per-node check**: after each `module.json` write, append every node it declares to the log AND run the per-node check from step 6 on each one. Fix anything before moving to the next file.

### 4. Write test sections

Write tests BEFORE implementation content to avoid confirmation bias — test content should be derived from requirements and component contracts, not influenced by implementation decisions.

- For each module, create `test_sections` entries in `module.json` that cover all components
- Each test_section's `describes` must reference component IDs — **every component must be covered by at least one test_section**, which the `test_coverage` checker enforces:
  `component Widget (id:e44c856b0ff8) has no test_section coverage`
- Write `test_*.md` content leaves with substantive integration/acceptance coverage:
  - **Setup**: fixtures, test data, preconditions
  - **Cases**: concrete input → expected output pairs
  - **Edge cases**: boundary conditions, error paths, invalid inputs
- Group related components into shared test_sections where they have natural testing affinity (components forming a pipeline). A test_section produces its own bead only when `describes` has ≥ 2 entries; a single-component one is bundled into that component's work.
- **Per-node check**: after writing each `test_*.md`, append the test_section to the log AND run the per-node check on it. The `describes >= 2` ⇔ the-content-actually-spans-multiple-components rule is the highest-value check here.

### 5. Write architecture and data-flow content leaves

- Create every markdown file referenced by a `content` field on a component or data_flow
- Write substantive content synthesized from the proposal — not stubs. `content` is required and non-empty, so there is no such thing as a component without a leaf
- Apply "Deciding what belongs in an arch leaf": no Go, and the ladder for anything carried over from existing implementation prose
- Add typed links where a reader would otherwise have to guess which node is meant
- If the proposal lacks detail for a node, write what you can and mark gaps with `<!-- TODO: detail needed -->` — but note that an HTML comment is **not** invisible to the link checker, so do not park a `[[…]]` inside one
- **Per-node check**: after writing each leaf (`arch_*.md`, `flow_*.md`), append it to the log AND run the per-node check on it

### 6. Sanity gates

Two phases: the per-node check (run inline during steps 3–5) and the end-of-session review.

#### Per-node check

Run immediately after a single node is written. Cheap, scoped to one node and its content leaf:

- **Structural**: defer to `bin/spex validate` for whole-graph checks; for one node, confirm the JSON entry carries its required fields, that its `id` is what `bin/spex hash-id` returns for its name, and that every referenced id (`implements`, `uses`, `describes`, `provided_by`, `preq_id`) points at a node that exists or is about to be written in this run.
- **Prose-vs-JSON**: read what you just wrote in the content leaf. Does the prose describe what the JSON declaration claims?
  - `component / arch_*.md`: the prose describes behaviour fulfilling the requirements in `implements`
  - `test_section / test_*.md`: the content matches the shape declared by `describes`. For `describes >= 2`, it actually spans multiple described components (content that names one component and one method is a unit test and does NOT belong here). For `describes == 1`, it is bundled with that component's TDD work.
  - `data_flow / flow_*.md`: the prose describes shapes and contracts moving between the components in `uses`
  - `api`: the `name` is the exact surface string, `provided_by` names components in this same module, and the component behind it says what the entry point does
- If a check fails: fix the JSON entry or rewrite the content leaf before moving to the next node. Do not carry unresolved findings forward.

#### End-of-session full review

Run once after step 5, before step 7.

**1. Deterministic structural pass**

```bash
bin/spex validate
```

Ten checkers run: `schema`, `content`, `link`, `id`, `id_derivation`, `dag`, `name_consistency`, `test_coverage`, `requirement_coverage`, `coupled_section`. Output is always JSON — one line normally, pretty-printed when stdout is a terminal. There is **no `--json` flag**; passing one exits 1 with `unknown flag: --json`.

Required:

```bash
bin/spex validate | jq -e '.valid == true and .warning_count == 0'
```

Every finding is severity `error`. **The validator has no warning sites at all**: `warning_count` is always 0 and is kept in the JSON only because CI and downstream gates assert on it. Exit is 0 when `valid` is true, 1 otherwise. Fix everything in scope here before running the diff pass — a structural failure makes diff output uninterpretable.

Two error families are worth knowing before you meet them, both from `requirement_coverage`, which runs in two phases:

- *Phase 1, project requirement → module requirement (via `preq_id`)*:
  `project requirement <id> "<title>" is not derived into any module requirement`
  Every project requirement must be derived by at least one module requirement somewhere in the tree. Adding a project requirement without a derivation is a validation failure, not a warning.
- *Phase 2, module requirement → component (via `implements`)*:
  `<module> requirement <id> "<title>" is not implemented by any component`
  Every module requirement must be named in some component's `implements` array in the same module.

**2. Deterministic completeness pass**

```bash
bin/spex diff          # or: bin/spex diff --json
```

Exit contract:

| Exit | Meaning |
|------|---------|
| 0 | the tree built and the `errors` array is empty |
| 1 | the tree did not build — missing or unparseable `project.json` / `module.json`, unreadable content leaf |
| 2 | the tree built and `errors` is non-empty |
| 3 | not a spex project — the lifecycle pre-flight refused before any tree was built: never initialised (stderr names `spex init`), or the snapshot or journal missing or unparseable (broken; stderr names `spex doctor`). An *empty* journal is fine — `spex init` seeds it empty and a never-ingested project simply contributes no names to the removal sweep |

The bare text output reports these as **errors**, not warnings — `N error(s):` followed by `error: [<type>] <message>`, `path:` and `related:` lines. The `--json` form puts the same items in the top-level `errors` array. Either way a non-empty `errors` array is a hard failure and the command exits 2. `bin/spex plan` refuses to consume such a diff — `plan: diff contains N error(s), refusing to proceed`, exit 1 — so leaving one unresolved blocks the user's mint outright.

Two entry types appear in `errors`:

- **`incomplete_change`** — a structural change whose consequences were not written down. The eight shapes:
  - `module X meta changed but component Y content leaf unchanged` — `module.json` changed (a node added, removed, or a `describes`/`uses` array reshaped) but Y's leaf was not touched. Fix by editing `arch_<Y>.md` to say what changed about Y's call sites, test surface or relationships. Real edits, not cosmetic. If several components are flagged, every one needs an edit; the rule is intentionally strict. **One exemption:** when a module requirement in the same module also changed, the whole-module obligation is skipped and only the finer-grained requirement rules below apply (`merkle/completeness_checker.go:126`) — so a `module.json` edit that includes a requirement change does not oblige every component, only the requirement's implementors.
  - `requirement R (…) added but not implemented by any component`
  - `requirement R (…) added but component C content leaf unchanged`
  - `requirement R (…) description changed but component C content leaf unchanged`
  - `component C still implements removed requirement R (…)`
  - `no module requirement derives from project requirement P`
  - `project requirement P changed but component C (…) content leaf unchanged`
  - `module requirement R (…) still derives from removed project requirement P`
- **`surviving_name`** — `removed <api|component> "<name>" (<hash>) is still named in the spec corpus at N site(s); sweep the mentions or restore the node`. When you remove an api or component, its declared name must stop appearing anywhere in `spec/` outside `spec/proposals/`. Rewrite every site the error lists, or put the node back. This is why declarable names matter: a name the sweep cannot rebuild is a removal nobody can verify.

`bin/spex diff` may also print a **notes** block (`note: [unverifiable_module] …`, `note: [suppressed_by_live_name] …`). Notes are disclosures, never violations: they never affect the exit code. Read them — each one marks a place where "no errors" means less than it looks — but do not try to make them go away.

This pass must end with `errors: []`. In a module-scoped run, resolve only the errors naming nodes inside the scoped module and report the rest — see "A scoped run and the tree-wide gates" below.

**3. Cross-node prose-vs-JSON pass**

Walk the authoring log. For each touched node, re-run the per-node check now that all nodes exist (this catches nodes that referenced something written later in the session). Also check what no checker checks:

- Every `*.md` in a touched module directory is referenced by a `content` field in `module.json`. Nothing validates this — an unreferenced leaf is invisible to the merkle tree and will never be diffed.
- Every cross-reference in `implements`, `uses`, `describes`, `provided_by`, `requires_module`, `preq_id` resolves (confirm the `id` checker passed).
- Every api name is the string a caller actually types, and no two modules claim the same one.

#### A scoped run and the tree-wide gates

Both gates read the whole spec directory and neither takes a module filter, so a finding that belongs to another module holds them red however clean the scoped module is. There is no version of the gate a scoped run may weaken: `bin/spex validate | jq -e '.valid == true and .warning_count == 0'` and `errors: []` are exactly what they say, whatever the scope.

The rule is therefore about *which findings are yours*, not about the gate:

1. Fix every finding whose `path` or `related` names a node inside `spec/<module>/`. Re-run both gates.
2. Judge only the residue. If it is empty, the gates passed — print the success line and go to step 7.
3. If the residue is non-empty, **the gates did not pass, and this run does not make them pass.** Do not edit `spec/project.json` or a sibling module's files to clear it. Do not print `spec: sanity gates passed`. Go to step 7 anyway and report each remaining finding verbatim, together with the command that produced it, named as work another run owns.

A scoped run that ends with a non-empty residue is a correct outcome, not a failure to be retried: the "Loop and escape" ladder below applies only to findings inside the scope.

#### Loop and escape

If the end-of-session review surfaces findings inside the scope (or, in a whole-tree run, any findings at all):

- Attempt 1: revise the spec to address findings; re-run the review.
- Attempt 2: revise again; re-run.
- After attempt 2 fails: HALT auto-correction. Present the user with two choices:
  - `skip` — leave the findings unresolved; the user will adjust manually before running the pipeline.
  - `one more round` — one more auto-correction attempt; on failure, present the same two choices again.

If both gates are green — or, in a scoped run, green apart from a residue that names no node inside the scope — proceed to step 7. Print `spec: sanity gates passed (N nodes touched across M modules)` only in the first case; with a residue, say instead how many findings are being left to another run.

### 7. Report

Tell the user:

- What files were created or modified (list them)
- Any `<!-- TODO -->` markers that need follow-up
- In a module-scoped run, the project-level or sibling-module edits the proposal still needs, and any out-of-scope gate findings left in place
- Which nodes look like absorb candidates and why — advisory only; the classification itself is `/mint`'s Step 2, made against the committed diff
- Remind them to review the spec and commit it to git **on the working branch, landing on `main` via PR**. The mint runs against that commit, so the SHA the user hands `spex plan --git-head` is the one carrying these edits

## Alter Mode Details

When modifying an existing spec:

1. Read all existing JSON files first.
2. **Preserve existing IDs.** Never renumber. Never recompute an id for an unchanged node — and never recompute a project requirement id at all (see *Legacy project requirement hashes* under IDs).
3. Add new nodes by deriving their identity hash with `bin/spex hash-id`. IDs are content-derived, not assigned.
4. When removing a node, delete the JSON entry and its content file. Then sweep its **name** out of the corpus: `bin/spex diff` reports a `surviving_name` error for every removed api or component whose declared name still appears anywhere under `spec/` except `spec/proposals/`, which is historical and deliberately exempt.
5. When modifying a node, update the JSON fields and the content markdown together. **Changing a `name` or `title` changes the identity hash** — the old id is gone, a new one appears, and the new id must be regenerated with `bin/spex hash-id`. The pipeline treats this as delete + create, so the rename is also a removal for rule 4's purposes.
6. **Update everything the change touches — graph edges *and* prose.**
   - *Edges*: update every graph edge the change affects — if a component is removed, drop its id from every `uses`, `describes` and `provided_by` array.
   - *Links*: a `[[<hash>|…]]` pointing at a removed or renamed node no longer resolves and is a `link` error. Find them with `git grep '\[\[<old-hash>' spec/` and repoint or remove each one.
   - *Prose footprint*: a removed or renamed component, api, command, flag or identifier can be named as prose in content leaves of **any** module — pipeline diagrams, narrative, examples — not only the module the proposal names. `git grep` the old name across the whole `spec/` tree. Rewrite every leaf that still presents the removed or renamed thing as current; keep deliberate negative or historical mentions ("there is no `spex hash` command"). Scope the edits to what the grep finds, not to the modules the proposal happened to enumerate.
7. **Module-level supersession: delete old, create new.** When a proposal restructures modules — splitting one, merging several, or reshaping a module's components significantly — delete the old module entry from `project.json`, delete the old module directory and all its content files, and create the new module(s) as entirely fresh structures. Do not keep the old directory for the new module, do not reuse IDs, and do not leave component shells pointing forward. Rule 5 handles individual renames; this extends the same intent to whole-module restructuring, so the pipeline sees clean `removed → obsolete + cleanup` signals for the old nodes and `added → create` for the new ones, with no false lineage between old and new component beads. Note that retiring a whole module is the case the removal-time sweep is weakest at: with the module name gone from `project.json`, it may report `unverifiable_module` and leave the prose sweep entirely to you.

## Handing the spec to the pipeline

`/spec` writes the spec and stops: gates green, edits committed by the user, baseline untouched.
Everything after that — the per-node mint-vs-absorb assessment, `spex plan`, the adapter,
`spex ingest`, the label budgets, and the refresh pathway — is `/mint`'s, the one skill that
moves the baseline and the canonical home of that doctrine. Author with it in view:

- The mint runs against the commit carrying these edits — the SHA the user hands
  `spex plan --git-head` (as a 7-character short SHA; `/mint` carries the label budget).
- `spex plan` refuses a diff with errors outright, which is why step 6's completeness pass must
  end `errors: []` — an unresolved finding blocks the user's mint, not merely annotates it.
- A run that carries sweep-only leaves (a retired name rewritten across modules, say) should
  name them in the step 7 report as absorb candidates, so `/mint`'s assessment starts from the
  author's own knowledge of what changed.
- A claimed (`in_progress`) task's node must not change — mint a module's changes in one run
  rather than dribbling them across several while beads are in flight.
