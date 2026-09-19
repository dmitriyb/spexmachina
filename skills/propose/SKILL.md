---
name: propose
description: "Research the spec and draft a structured proposal in plan mode"
argument-hint: "[proposal-name]"
---

# /propose — Create a Spec Proposal

Draft a structured proposal by researching the spec, code, and existing proposals, then present the full draft in plan mode for user approval.

A proposal is prose, but it is prose `/spec` must turn into spec files through `spex`'s authoring commands. Step 5 is the set of things those commands can declare, and how to read what a change costs off them instead of estimating it. A proposal that ignores it proposes work nobody can carry out — a name that cannot be declared, a link that cannot resolve, a Go signature that cannot live in an arch leaf.

## Step 1: Detect Proposal Type

Check whether `spec/project.json` exists:

- **Does not exist** → this is a **project proposal** (bootstrapping a new project)
- **Exists** → this is a **change proposal** (modifying an existing spec)

## Step 2: Enter Plan Mode

Call `EnterPlanMode`. The system assigns a plan file path — you will write the full proposal draft there.

## Step 3: Clarify Intent

If `$ARGUMENTS` is empty and the user's intent is unclear, use `AskUserQuestion` to ask **one focused question** about what the proposal should cover. Do not present a checklist or menu. If `$ARGUMENTS` or prior conversation make the intent clear, skip this step.

If `$ARGUMENTS` names a review decisions directory (`.spex/runs/review/`, written by `/spec-review all`), that is the whole intent: a review proposal carrying every accepted finding as decided, nothing else. Ask nothing.

## Step 4: Research

Read relevant files silently — do not narrate each file you read. Go straight to drafting after research.

### For change proposals, read:

1. `spec/project.json` — project requirements and the module list
2. All `spec/*/module.json` — module requirements, apis, components, data flows, test sections, and the edges between them
3. The markdown content leaves (`arch_*.md`, `flow_*.md`, `test_*.md`) in affected module directories
4. All `spec/proposals/*.md` — prior proposals (avoid duplication/contradiction)
5. `CLAUDE.md` at the repo root — language, frameworks, build tools, conventions
6. Relevant source code if the proposal involves implementation changes
7. For a review proposal, the decisions directory first: `FINDINGS.tsv` (two verbatim quotes, the contradiction and the replacement text per finding), `VERDICTS.tsv` (only `holds` rows count), `DECISIONS.tsv` (`accepted` / `declined` / the chosen option, with its reason). The proposed change is the decisions, node by node; a declined finding is not proposed

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

Nodes only — `{id, type, name, module}` — with bare identity hashes. Roughly 24 KB against 790 KB for the full `--format json` graph. Edges are omitted; read those from `module.json`. Every id in that table except the `module` rows is a legal link target (Step 5.3).

## Step 5: Constrain the proposal to what the format allows

`/spec` authors the spec through `spex`'s authoring commands, which read the resolved profile and refuse anything the format cannot hold. A proposal therefore does not need to restate the format; it needs to propose things the commands can declare, and to say what they cost.

### 5.1 The vocabulary is the profile's

Run `bin/spex profile show`. Its `node_types` are the only things a proposal can ask for — under the default profile: project and module `requirement`, `component`, `data_flow`, `test_section`, `api`, plus the frame's `module`. Each type's `fields` say what a new node must carry (a project requirement's `type` and `priority`, a module requirement's `preq_id`); its `requires_content` says whether it owns a leaf; the `coverage_chains` say what one new node drags in: a project requirement needs a module requirement deriving from it, a module requirement an implementing component, a component a describing test section. A test section produces its own task only when it describes two or more components. Say these consequences in the Impact expectation rather than letting `/spec` discover them.

**`api` is how a proposal declares an external surface** — the exact string callers write (`spex diff`, `GET /v1/specs/{id}`), never a signature, with no content file, `provided_by` module-local, and a name globally unique across modules. **Never propose an impl section**; the type no longer exists.

### 5.2 Names and ids

A component or api name must be its own tokenization in at most six words — no brackets, parentheses, trailing periods or possessives; `spex node add` refuses anything else and prints the declarable form. Write the declarable form in the proposal. Ids are derived by the commands and never appear in a proposal as new values; an existing node is referred to by the id the slim render shows, and an existing project requirement's id is never recomputed or renamed.

### 5.3 Cross-references are hash links

Content leaves reference other nodes as `[[<12-hex identity hash>|<display text>]]`; module nodes are not linkable. When a proposal wants a specific cross-reference made, name the target node and let `/spec` fetch its hash.

### 5.4 Arch leaves describe behaviour, not code

Arch leaves carry no language: no `func`, no signatures, no ```go fences. The one exception is pseudocode where a requirement's *description* is itself the algorithm or the bound. Propose observable behaviour — stdout, exit codes, files written, ordering, error conditions, naming conventions — and the unrecoverable why: rejected alternatives, constraints imposed from elsewhere, historical reasons.

### 5.5 Say what the change costs — by running it

The completeness rules make proposals larger than they look: a requirement description change obliges a changed leaf on every implementing component; a project requirement change walks down to every component under it; any `module.json` change moves the module's `meta` leaf and obliges every component in the module unless a requirement in it also changed; a rename is a removal plus an addition with a corpus-wide sweep for the old name.

Do not compute the impact expectation by hand. Apply the proposal's structural changes to a scratch copy and read the cost off the commands:

```bash
S=$(mktemp -d) && cp -r spec .spex "$S"/ && P="$S/spec"
bin/spex node add <name> --type <t> --module <m> -s "$P" ...    # one per new node
bin/spex edge add <source-id> <field> <target-id> -s "$P"       # one per edge
bin/spex diff -s "$P" --json | jq '.changes, .errors'           # the cumulative cost
```

Each command's `obligations` is the cost of that step; `spex diff` against the copied snapshot is the cost of the whole change, entry for entry what `/spec` will have to discharge. The tables in the Impact expectation — new nodes, modified nodes and the leaves they oblige, tasks — are read from that output. Nothing under the real `spec/` is touched.

`spex validate` has no warnings. Every finding is an error, and a spec either validates or does not.

### Say which way the baseline moves

A proposal whose whole effect is corrective — spec text converged onto shipped, test-pinned
behaviour, no work born — should declare it: prepend `mode: refresh` YAML frontmatter (the
convention `/spec-review`'s correction proposals already use) and name the pinning evidence in
the Impact expectation. `/mint` then executes it as a refresh instead of minting tasks that owe
nothing. A proposal that births any work omits the frontmatter; mixed changes stay a mint, with
the yielding nodes argued per `/mint`'s absorb rules.

### Retired vocabulary (required when anything is retired)

If the proposal retires a term, flag, file, command, or concept, the draft MUST carry a
`## Retired vocabulary` section listing each retired token on its own line as `- ` + backticked
term. This feeds `scripts/lens-lexicon.sh`: every future spec-review sweeps the corpus for these
terms, which is what keeps migration shadows (sibling text nobody updated) from surviving review.
A proposal that retires nothing omits the section.

## Step 6: Draft Proposal as the Plan File

The plan file has TWO parts: **instructions header** then **proposal content**, separated by `---`. Both are required.

### Part 1: Instructions header

The plan file MUST start with:

1. **Output statement**: `The result of this session is a proposal draft YYYY-MM-DD-<slug>.md placed in spec/proposals/drafts/; /mint registers it into spec/proposals/.`
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
3. **Impact expectation** — What tasks will be created, retargeted or closed? Which content leaves must change to satisfy the completeness rules of 5.5? What is the expected scope of work?

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

<What tasks will be created, retargeted, or closed? Which content leaves must change? Estimated scope.>
```

The H2 headings in both templates are the ones `spex register` requires. Renaming or dropping one makes the proposal unregisterable.

## Step 7: Exit Plan Mode

Call `ExitPlanMode`. The user reviews the full proposal draft in the plan UI and approves or requests changes.

If the user requests changes, revise the draft and re-present — this happens naturally in the conversation flow after plan mode exits. Re-enter plan mode if substantial revisions are needed.

## Step 8: Write Proposal File

After the user approves:

1. Create `spec/proposals/drafts/` if it does not exist.
2. Write the approved draft to `spec/proposals/drafts/YYYY-MM-DD-<name>.md` where `YYYY-MM-DD` is today's date and `<name>` is a short kebab-case slug. If the user provided `$ARGUMENTS`, use that as the name slug. If `$ARGUMENTS` is empty, derive the slug from the proposal title (e.g. "Add user auth" → `add-user-auth`).
   **The slug is capped at 26 characters**, making the stem (`YYYY-MM-DD-<name>`) at most 37. `spex register` keys the proposal's `registered` event as `<git_head>:<stem>`, and that eid becomes the epic task's idempotency label, `spex:<git_head>:<stem>`; br rejects any label over 50 characters (`Error: Validation failed: label: exceeds 50 characters`) — with the 7-character short SHA the pipeline uses, 37 is exactly the budget left for the stem. An over-long slug does not degrade: it fails the `br create` partway through the mint, long after this session ended. Shorten it here: if the slug the user gave (or the one the title implies) is longer, propose a shortened one and say why — never trim it silently.
3. Tell the user the file path.
4. Remind them to review and commit to git. The draft stays under `drafts/`
   until `/mint` runs `spex register`, which copies it to
   `spec/proposals/<stem>.md` and refuses a destination that already exists —
   never write the draft to the destination yourself.

**STOP after writing the file.** Do NOT explore code, do NOT attempt implementation, do NOT edit anything under `spec/` other than the new proposal file. The proposal skill produces exactly one artifact: the proposal file.
