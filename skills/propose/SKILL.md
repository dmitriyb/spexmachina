---
name: propose
description: "Research the spec and draft a structured proposal in plan mode"
argument-hint: "[proposal-name]"
---

# /propose — Create a Spec Proposal

Draft a structured proposal by researching the spec, code, and existing proposals, then present the full draft in plan mode for user approval.

## Step 1: Detect Proposal Type

Check whether `spec/project.json` exists:

- **Does not exist** → this is a **project proposal** (bootstrapping a new project)
- **Exists** → this is a **change proposal** (modifying an existing spec)

## Step 2: Enter Plan Mode

Call `EnterPlanMode`. The system assigns a plan file path — you will write the full proposal draft there.

## Step 3: Clarify Intent

If `$ARGUMENTS` is empty and the user's intent is unclear, use `AskUserQuestion` to ask **one focused question** about what the proposal should cover. Do not present a checklist or menu. If $ARGUMENTS or prior conversation make the intent clear, skip this step.

## Step 4: Research

Read relevant files silently — do not narrate each file you read. Go straight to drafting after research.

### For change proposals, read:

1. `spec/project.json` — project requirements, modules, milestones
2. All `spec/*/module.json` — module requirements, components, edges
3. All markdown files (`*.md`) in affected module directories — every content leaf (arch, impl, test, flow, etc.)
4. All `spec/proposals/*.md` — prior proposals (avoid duplication/contradiction)
5. `CLAUDE.md` at the repo root — language, frameworks, build tools, conventions
6. Relevant source code if the proposal involves implementation changes

### For project proposals, read:

1. `spec/proposals/*.md` if the directory exists
2. `CLAUDE.md` at the repo root — language, frameworks, build tools, conventions
3. Existing source code to understand what already exists
4. Any existing `spec/` content

**Language/framework discovery:** Do NOT hardcode any programming language. Read `CLAUDE.md` at the repo root to determine the project's language, frameworks, build tools, and conventions. Use that info to guide which source files to read.

## Step 5: Draft Proposal as the Plan File

Write the FULL proposal text to the plan file using the appropriate template below. This is the exact text that will become the proposal file — not an outline, not a summary.

The draft should:
- Reference specific modules, components, and requirements by name and ID
- Note what existing proposals have already covered (avoid duplication)
- For change proposals: identify which spec nodes are affected
- Be substantive prose, not placeholders

### Project Proposal — address these sections

1. **Vision** — What problem does this project solve? What is the core idea in one paragraph?
2. **Modules** — What are the major components? For each: name, purpose, and what it depends on.
3. **Key requirements** — Functional requirements (what it does) and non-functional requirements (how well it does it).
4. **Design decisions** — What are the important choices and why? What alternatives were considered?

### Change Proposal — address these sections

1. **Context** — What is the current state? What triggered this change?
2. **Proposed change** — What specifically will change in the spec? Which modules, requirements, or components are affected?
3. **Impact expectation** — What beads will be created, modified, or closed? What is the expected scope of work?

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

1. **<Short name>** — <Description.>
2. ...

### Non-functional

1. **<Short name>** — <Description.>
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

<What specifically will change? Which modules, requirements, components are affected?>

## Impact expectation

<What beads will be created, modified, or closed? Estimated scope.>
```

## Step 6: Exit Plan Mode

Call `ExitPlanMode`. The user reviews the full proposal draft in the plan UI and approves or requests changes.

If the user requests changes, revise the draft and re-present — this happens naturally in the conversation flow after plan mode exits. Re-enter plan mode if substantial revisions are needed.

## Step 7: Write Proposal File

After the user approves:

1. Create `spec/proposals/` directory if it does not exist.
2. Write the approved draft to `spec/proposals/YYYY-MM-DD-<name>.md` where `YYYY-MM-DD` is today's date and `<name>` is a short kebab-case slug. If the user provided `$ARGUMENTS`, use that as the name slug. If `$ARGUMENTS` is empty, derive the slug from the proposal title (e.g. "Add user auth" → `add-user-auth`).
3. Tell the user the file path.
4. Remind them to review and commit to git.
