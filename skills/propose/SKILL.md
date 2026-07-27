---
name: propose
description: "Research the spec and draft a structured proposal in plan mode"
argument-hint: "[proposal-name]"
---

# /propose — Create a Spec Proposal

Draft a structured proposal by researching the spec, code, and existing proposals, then present the full draft in plan mode for user approval.

A proposal is prose, but it is prose `/spec` must turn into spec files. Step 5 is the set of things `/spec` can actually author. A proposal that ignores it proposes work nobody can carry out — a name that cannot be declared, a link that cannot resolve, a Go signature that cannot live in an arch leaf. Draft against the format, not against a memory of it.

## Step 1: Detect Proposal Type

Check whether `spec/project.json` exists:

- **Does not exist** → this is a **project proposal** (bootstrapping a new project)
- **Exists** → this is a **change proposal** (modifying an existing spec)

## Step 2: Enter Plan Mode

Call `EnterPlanMode`. The system assigns a plan file path — you will write the full proposal draft there.

## Step 3: Clarify Intent

If `$ARGUMENTS` is empty and the user's intent is unclear, use `AskUserQuestion` to ask **one focused question** about what the proposal should cover. Do not present a checklist or menu. If `$ARGUMENTS` or prior conversation make the intent clear, skip this step.

## Step 4: Research

Read relevant files silently — do not narrate each file you read. Go straight to drafting after research.

### For change proposals, read:

1. `spec/project.json` — project requirements and the module list
2. All `spec/*/module.json` — module requirements, apis, components, data flows, test sections, and the edges between them
3. The markdown content leaves (`arch_*.md`, `flow_*.md`, `test_*.md`) in affected module directories
4. All `spec/proposals/*.md` — prior proposals (avoid duplication/contradiction)
5. `CLAUDE.md` at the repo root — language, frameworks, build tools, conventions
6. Relevant source code if the proposal involves implementation changes

### For project proposals, read:

1. `spec/proposals/*.md` if the directory exists
2. `CLAUDE.md` at the repo root — language, frameworks, build tools, conventions
3. Existing source code to understand what already exists
4. Any existing `spec/` content

**Language/framework discovery:** Do NOT hardcode any programming language. Read `CLAUDE.md` at the repo root to determine the project's language, frameworks, build tools, and conventions. Use that info to guide which source files to read.

### Get the name→hash table once

Every node this project already has, as one compact table, instead of re-grepping `module.json`:

```bash
bin/spex render --format json --slim | jq -r '.nodes[] | "\(.type)\t\(.name)\t\(.id)"'
```

Nodes only — `{id, type, name, module}` — with bare identity hashes. Roughly 24 KB against 790 KB for the full `--format json` graph. Edges are omitted; read those from `module.json`. Every id in that table except the `module` rows is a legal link target (Step 5.4).

## Step 5: Constrain the proposal to what the format allows

### 5.1 The node vocabulary

These are the only things a proposal can ask for. Identity is `<module>/<type>/<name>` for module-scoped nodes and `project/requirement/<title>` for project requirements.

| Node | Declared in | Content leaf | Produces a bead |
|---|---|---|---|
| project requirement | `project.json` → `requirements[]` | none — hashes from its JSON fields | no |
| module | `project.json` → `modules[]` + `<mod>/module.json` | none — a synthetic `meta/<hash>` leaf hashes the whole `module.json` | no — the `meta` leaf is what changes, and `meta` produces none |
| module requirement | `module.json` → `requirements[]` | none — hashes from its JSON fields | no |
| **api** | `module.json` → `apis[]` | none — hashes from its JSON fields | no |
| component | `module.json` → `components[]` | required, non-empty (`arch_*.md`) | yes |
| data flow | `module.json` → `data_flows[]` | required, non-empty (`flow_*.md`) | yes |
| test section | `module.json` → `test_sections[]` | required, non-empty (`test_*.md`) | only when `describes` names ≥ 2 components |

**`api` is how a proposal declares an external surface** — a CLI invocation, an HTTP route, a library entry point. `name` is the exact string callers write (`spex diff`, `GET /v1/specs/{id}`, `schema.IdentityHash`), never a signature. It has no content file. `provided_by` lists identity hashes of components **in the same module** and is referentially checked; involvement from other modules is already expressed by component `uses` edges. `group` is a freeform renderer label — spex never branches on it. **API names are globally unique across every module**; component names only need to be unique inside their own module.

**Never propose a new impl section.** The `impl_sections` array still exists while the migration that is deleting it runs, but it produces no bead, duplicates what the code already carries, and binds the spec to a language. Content that used to go there belongs in the arch leaf of the component implementing the relevant requirement — or nowhere.

**Required fields a proposal has to supply values for:**

- Project requirement: `id`, `type` (`functional` | `non_functional`), `title`, and **`priority` 0–4**. The JSON Schema calls `priority` optional; the validator rejects a project requirement without it. Propose a priority for every new project requirement.
- Module requirement: `id`, `type`, `title`, **`preq_id`** — every module requirement derives from a project requirement.
- Component / data flow / test section: `id`, `name`, `content` (non-empty; an empty string is a schema error, not a skipped node).
- API: `id`, `name`.

**Coverage obligations that turn one proposed node into several:**

- A new project requirement needs at least one module requirement deriving from it, or `spex validate` fails.
- A new module requirement needs at least one component whose `implements` names it.
- A new component needs at least one test section whose `describes` names it.
- `uses` edges between components and `requires_module` edges between modules must stay acyclic.

So "add a component" is really "add a component, its arch leaf, and its test coverage" — say so in the Impact expectation.

### 5.2 Do not name things that cannot be named

An api or component `name` is rejected unless **tokenizing it reproduces it exactly, in at most 6 whitespace-separated words**. Tokenization strips `` ` * _ " ' ( ) [ ] , ; : ! ? “ ” ‘ ’ `` from both ends of each word, plus a trailing `.` and a trailing possessive — both the straight `'s` and the curly `’s`. It deliberately keeps `/ - { } < > |`.

The rule exists because a removed node's name is recovered by hashing corpus phrases against its identity hash. A name no phrase can equal is a name whose removal can never be swept.

| Rejected | Why | Declarable form |
|---|---|---|
| `spex validate [--json]` | brackets are stripped | `spex validate --json` |
| `Validator (core)` | parens are stripped | `Validator core` |
| `Widget.` | trailing period is stripped | `Widget` |
| `Bob's` | possessive suffix is stripped | `Bob` |
| `_private` | leading underscore is stripped | `private` |
| `This component name is seven words long` | 7 words, limit is 6 | shorter |

Accepted, verified: `spex diff`, `GET /v1/specs/{id}`, `schema.IdentityHash`, `spex render --format json --slim`, `WidgetLister`.

A proposal that says "declare an api called `spex diff [--json]`" is proposing something unauthorable. Write the declarable form in the proposal, and if the optional-argument shape matters to a reader, put it in the prose rather than the name.

### 5.3 Ids are derived, never invented

Every module-scoped id must equal `IdentityHash(module, node_type, name)`. The validator errors on any that does not, because three mechanisms read the hash backwards to recover the name.

```bash
bin/spex hash-id --type requirement --name "Serve widgets"                 # project requirement
bin/spex hash-id --type module --name widgets                              # module
bin/spex hash-id --module widgets --type component --name WidgetLister
bin/spex hash-id --module widgets --type api --name "widgetctl list"
```

Valid `--type` values: `requirement`, `component`, `impl_section`, `data_flow`, `test_section`, `api`, `module`. `--module` is required for everything except a project requirement and a module. Anything else exits 1.

**Never recompute an existing project requirement id.** Fifteen of this project's eighteen project requirements carry hashes that predate the convention and `hash-id` cannot reproduce them. Project-level requirement ids are exempt from the derivation check for exactly that reason. If a proposal refers to one, copy the id out of `project.json`.

### 5.4 Cross-references are hash links

Content leaves reference other nodes as `[[<12-hex identity hash>|<display text>]]`. Display text is free-form and never checked, and it may contain a code span — but the backticks go *inside* the display half, never around the whole link: `` [[3f9a1c7b2e04|`spex validate`]] ``. Inline code spans are not tracked by the link scanner, so wrapping a `[[…]]` in backticks does not stop it being resolved; prose that needs to show the link syntax literally belongs in a fence.

- Name-based links are rejected: a name carries no type segment, so it cannot be turned back into a hash.
- **Module nodes are not linkable.** Only leaves are: requirements, apis, components, data flows, test sections.
- A `[[` must close with `]]` before the next blank line, fence or end of file.

When a proposal wants a specific cross-reference made, name the target node and let `/spec` fetch its hash from the slim render.

### 5.5 Arch leaves describe behaviour, not code

Arch leaves carry no language: no `func`, no signatures, no ```go fences. The one exception is pseudocode where a requirement's *description* is itself the algorithm or the bound — and then it is language-neutral pseudocode, not Go.

Nothing in `spex validate` checks this; it is authoring discipline, which means a proposal that promises "the arch leaf will document the `Validate(specDir string) []ValidationError` signature" will simply produce a leaf that has to be rewritten. Propose observable behaviour instead: stdout, exit codes, files written, ordering, error conditions, naming conventions.

What an arch leaf is *for*: falsifiable statements a reader with the built binary could prove wrong, and the unrecoverable why — rejected alternatives, constraints imposed from elsewhere, historical reasons. Facts about a git-tracked file, instructions to a future implementer, and anything the arch leaf already says belong nowhere.

### 5.6 Say what the change costs

`spex diff` exits **0** clean, **2** on completeness errors, **1** when the tree cannot be built. Two of those completeness rules make proposals larger than they look, and the Impact expectation section should account for them.

**Touching a requirement's description obliges a changed content leaf on every implementing component.** Changing one line of one module requirement:

```
modified   structural   widgets    5533b6eb0761
modified   structural   widgets    meta/3fe17d5b8078

1 error(s):
  error: [incomplete_change] requirement 'Widget listing' (widgets) description changed
         but component WidgetLister content leaf unchanged
diff: 1 completeness error(s) found     # exit 2
```

The project-requirement form is worse: it walks project requirement → every module requirement deriving from it → every component implementing those, and demands a changed leaf from each.

**Changing anything in a `module.json` moves that module's `meta` leaf**, which obliges a changed content leaf on **every** component in the module — unless a requirement in that module also changed, which suppresses the meta rule. Adding one api and touching nothing else:

```
added      contract     widgets    b707fba93042
modified   structural   widgets    meta/3fe17d5b8078

1 error(s):
  error: [incomplete_change] module widgets meta changed
         but component WidgetLister content leaf unchanged
diff: 1 completeness error(s) found     # exit 2
```

**Renaming a node is delete-plus-create.** The name is the identity, so a rename destroys one node and mints another: a new hash, an obsolete bead and a create bead, every inbound link rewritten, and a corpus-wide sweep for surviving mentions of the old name. A proposal that renames things is proposing expensive work — name the cost in the Impact expectation rather than letting `/spec` discover it.

`spex validate` has no warnings. Every finding is an error, `warning_count` is always 0, and a spec either validates or does not.

## Step 6: Draft Proposal as the Plan File

The plan file has TWO parts: **instructions header** then **proposal content**, separated by `---`. Both are required.

### Part 1: Instructions header

The plan file MUST start with:

1. **Output statement**: `The result of this session is a proposal file YYYY-MM-DD-<slug>.md placed in spec/proposals/.`
2. **Proposal type**: State whether this is a project proposal or change proposal.
3. **Template**: Copy the FULL template for the detected proposal type (from the templates below) into the plan file. State: "Write the proposal file using this exact markdown structure."
4. **Conformance rule**: "The proposal MUST contain ONLY the sections defined in the template. No extra top-level sections."
5. **Quality requirements**: Reference specific modules, apis, components and requirements by name and identity hash. Note what existing proposals have already covered. For change proposals: identify which spec nodes are affected, and which nodes are added, modified or removed. Be substantive prose, not placeholders.
6. **Post-write instructions**: "After writing the file, tell the user the file path and remind them to review and commit to git."
7. A `---` separator before Part 2.

### Part 2: Proposal content

Write the FULL proposal text using the appropriate template. The proposal MUST contain ONLY the sections defined in the template — no extra top-level sections (no "Assessment notes", no "What doesn't change", no "Deferred" as standalone sections). Subsections within the template's sections are fine for organizing content.

### Revisions

If the user discusses changes to the draft, update the proposal content in Part 2 but NEVER remove or reduce the instructions in Part 1. The instructions header is what allows a future session to correctly produce the output file.

### Project Proposal — address these sections

1. **Vision** — What problem does this project solve? What is the core idea in one paragraph?
2. **Modules** — What are the major components? For each: name, purpose, and what it depends on.
3. **Key requirements** — Functional requirements (what it does) and non-functional requirements (how well it does it). Give each a priority 0–4.
4. **Design decisions** — What are the important choices and why? What alternatives were considered?

### Change Proposal — address these sections

1. **Context** — What is the current state? What triggered this change?
2. **Proposed change** — What specifically will change in the spec? Which modules, requirements, apis or components are affected, and are they added, modified or removed?
3. **Impact expectation** — What beads will be created, modified or closed? Which content leaves must change to satisfy the completeness rules of 5.6? What is the expected scope of work?

### Project Proposal Template

```markdown
# Project Proposal: <Title>

*<One-line tagline.>*

## Vision

<1-2 paragraphs describing the problem and the solution.>

## Modules

### 1. <Module Name>

<Purpose and scope. What it depends on.>

### 2. <Module Name>

...

## Key requirements

### Functional

1. **<Short name>** — <Description.> (priority <0-4>)
2. ...

### Non-functional

1. **<Short name>** — <Description.> (priority <0-4>)
2. ...

## Design decisions

### <Decision title>

<What was decided, why, and what alternatives were rejected.>
```

### Change Proposal Template

```markdown
# Change Proposal: <Title>

## Context

<What is the current state? What triggered this change?>

## Proposed change

<What specifically will change? Which modules, requirements, apis, components are affected?>

## Impact expectation

<What beads will be created, modified, or closed? Which content leaves must change? Estimated scope.>
```

The H2 headings in both templates are the ones `spex register` requires. Renaming or dropping one makes the proposal unregisterable.

## Step 7: Exit Plan Mode

Call `ExitPlanMode`. The user reviews the full proposal draft in the plan UI and approves or requests changes.

If the user requests changes, revise the draft and re-present — this happens naturally in the conversation flow after plan mode exits. Re-enter plan mode if substantial revisions are needed.

## Step 8: Write Proposal File

After the user approves:

1. Create `spec/proposals/` directory if it does not exist.
2. Write the approved draft to `spec/proposals/YYYY-MM-DD-<name>.md` where `YYYY-MM-DD` is today's date and `<name>` is a short kebab-case slug. If the user provided `$ARGUMENTS`, use that as the name slug. If `$ARGUMENTS` is empty, derive the slug from the proposal title (e.g. "Add user auth" → `add-user-auth`).
3. Tell the user the file path.
4. Remind them to review and commit to git.

**STOP after writing the file.** Do NOT explore code, do NOT attempt implementation, do NOT edit anything under `spec/` other than the new proposal file. The proposal skill produces exactly one artifact: the proposal file.
