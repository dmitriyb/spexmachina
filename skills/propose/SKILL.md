---
name: propose
description: "Guide a free-form conversation into a structured spec proposal (project or change)"
argument-hint: "[proposal-name]"
---

# /propose — Create a Spec Proposal

Guide the user through a conversation to produce a structured proposal document in `spec/proposals/`.

## Detect Proposal Type

Check whether `spec/project.json` exists:

- **Does not exist** → this is a **project proposal** (bootstrapping a new project)
- **Exists** → this is a **change proposal** (modifying an existing spec)

## Conversation Flow

Start by telling the user which proposal type was detected and why. Then gather information through conversation — ask questions, clarify scope, surface trade-offs. Do not dump a template and ask the user to fill it in. Instead, have a natural conversation and build the proposal from the answers.

### Project Proposal — gather these sections

1. **Vision** — What problem does this project solve? What is the core idea in one paragraph?
2. **Modules** — What are the major components? For each: name, purpose, and what it depends on.
3. **Key requirements** — Functional requirements (what it does) and non-functional requirements (how well it does it).
4. **Design decisions** — What are the important choices and why? What alternatives were considered?

### Change Proposal — gather these sections

1. **Context** — What is the current state? What triggered this change?
2. **Proposed change** — What specifically will change in the spec? Which modules, requirements, or components are affected?
3. **Impact expectation** — What beads will be created, modified, or closed? What is the expected scope of work?

## Writing the Proposal

Once the conversation has covered all sections, write the proposal:

1. **Filename**: `spec/proposals/YYYY-MM-DD-<name>.md` where `YYYY-MM-DD` is today's date and `<name>` is a short kebab-case slug. If the user provided $ARGUMENTS, use that as the name slug. If $ARGUMENTS is empty, derive the slug from the proposal title (e.g. "Add user auth" → `add-user-auth`).
2. **Format**: Use the appropriate template below.
3. **Content**: Synthesize the conversation into clear, concise prose. Do not include the raw Q&A — distill it.

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

## After Writing

- Create `spec/proposals/` if it does not exist.
- Tell the user the file path and summarize what was written.
- Remind them to review the proposal and commit it to git.
- Note: once `spex register` is available, it will validate the proposal structure and link it to the spec. For now, the proposal is a plain markdown file committed to `spec/proposals/`.
