---
name: spec-review
description: "Audit the spec for internal inconsistencies (no code reading) and draft a correction proposal in plan mode if findings exist"
argument-hint: "[module-name]"
---

**Commits and pushes:** no. This skill audits and drafts a proposal in plan mode; the user reviews and commits. Enforcement hook `check-skill-commit-allowed.sh` blocks `git commit` when the active skill is `spec-review`.

## Step 0: Declare skill identity to enforcement hooks

Before any other action, run this command verbatim so the hook layer knows the active skill (see CLAUDE.md "## Enforcement"):

```bash
mkdir -p .claude && printf '{"skill":"spec-review","started_at":"%s","pid":%d}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" > .claude/skill-context.json
```

# /spec-review — Audit Spec for Internal Inconsistencies

Read the current spec and identify inconsistencies WITHIN the spec itself: prose that contradicts JSON declarations, JSON that references nonexistent nodes, content that doesn't match the structural shape its parent claims. **No code reading.** Code-vs-spec alignment is a separate skill (`/spec-drift`).

If findings exist, draft a correction proposal in plan mode using the `/propose` change-proposal template. If nothing actionable, exit silently with a one-line confirmation.

## Step 1: Resolve scope

- If `$ARGUMENTS` is a module name, scope the audit to that module only.
- If `$ARGUMENTS` is empty, audit every module listed in `spec/project.json`.

## Step 2: Run deterministic checks first

Run `bin/spex validate` and parse its output. Surface any errors as findings BEFORE doing prose-level review — they are the highest-confidence signals and cheapest to interpret.

The validator already covers: schema conformance, ID uniqueness, DAG acyclicity, orphan detection, broken cross-references, requirement coverage, test coverage. Do not re-derive any of these.

## Step 3: Read spec content

For each module in scope, read:

1. `spec/<module>/module.json`
2. Every content leaf referenced by `module.json` (`arch_*.md`, `impl_*.md`, `flow_*.md`, `test_*.md`)
3. Read `spec/project.json` for project-level requirements (needed for the `implements → preq_id → project_requirement` chain)

Do NOT read source code. This skill audits spec internal consistency only.

## Step 4: Prose-vs-JSON consistency review

For each touched node, check that the prose in the content leaf matches the structural claims in the JSON. The check is LLM judgment — what `spex validate` cannot mechanically verify.

Per node type:

- **Component (`arch_*.md`)**: Does the architecture description actually describe behavior fulfilling the requirements in the component's `implements` array? Does it reference the components in its `uses` array (or the module dependencies via `requires_module`)? Does it claim behaviors the requirements don't promise?
- **Impl section (`impl_*.md`)**: Does the prose describe implementation of the components in `describes`, not other components? Are the algorithms/data structures consistent with what the corresponding arch leaves claim?
- **Test section (`test_*.md`)**:
  - For `len(describes) >= 2`: do the scenarios actually exercise behavior that spans ≥2 of the described components? A scenario that names one component, one method, one assertion is a unit test and does NOT belong here.
  - For `len(describes) == 1`: scenarios should be unit/component tests bundled with that component's TDD work — appropriate at this shape.
- **Data flow (`flow_*.md`)**: Does the prose describe shapes/contracts moving between the components in `uses`? Are the participating components named correctly in the flow narrative?
- **Requirements (project.json + module.json)**: Are descriptions specific enough to be testable? Do module requirements' `preq_id` chains lead to project requirements with consistent semantics?

## Step 5: Bucket findings by lifecycle impact

Per the impact module's gating rules (`spec/impact/impl_action_classification.md`), classify each finding by the spec node it touches:

- **Bead-producing leaves**: `component`, `data_flow`, `test_section` with `describes >= 2`, `module`. A correction proposal touching these will go through normal pipeline (obsolete + create lifecycle).
- **Non-bead-producing leaves**: `impl_section`, `test_section` with `describes == 1`, `requirement`. A correction proposal touching only these is a candidate for `mode: refresh` once that pathway exists in ingest.

If findings exist on both kinds, the proposal goes through normal pipeline (the bead-producing changes drive the lifecycle, refresh mode would be wrong).

If findings exist only on non-bead-producing leaves AND the ingest refresh-mode pathway has shipped, the proposal frontmatter declares `mode: refresh`. If refresh mode has NOT shipped yet, the proposal is held: report findings to the user and exit without drafting (drafting a normal-mode proposal for impl-only drift would create a stalled epic).

## Step 6: Draft proposal in plan mode (if findings)

If actionable findings exist, follow the `/propose` Step 5 pattern: enter plan mode, write a plan file with an instructions header (Part 1) and proposal content (Part 2) separated by `---`.

Use the Change Proposal template from `/propose`:

```markdown
# Change Proposal: <Title>

## Context

<Cite each finding with file path and what's inconsistent. Reference the proposal/PR or commit that introduced the drift if known.>

## Proposed change

<For each finding, the spec edit that resolves it. Be specific: which fields, which prose paragraphs.>

## Impact expectation

<List the spec nodes touched. State the lifecycle: normal-mode (bead lifecycle expected) or refresh-mode (no bead lifecycle, hash-only update).>
```

If `mode: refresh` applies, include this as YAML frontmatter at the top of the proposal file:

```yaml
---
mode: refresh
---
```

The plan file's instructions header MUST contain the post-write step: `After writing the file, tell the user the file path and remind them to review and commit to git.`

## Step 7: Exit plan mode and write proposal file

After user approval in plan mode, write the proposal to `spec/proposals/YYYY-MM-DD-spec-review-<scope>.md`. Use today's date and a slug derived from the audit scope (e.g., `spec-review-emit` if scoped to one module, `spec-review-all` for full audit).

Tell the user the file path and remind them to review + commit + run `/spec` on it to apply.

**STOP after writing the file.** Do not run `/spec` automatically. The author reviews first.

## Step 8: No-findings exit

If Step 4 produced no actionable findings AND Step 2 produced no validator errors, print exactly:

```
spec-review: no actionable findings (N nodes audited across M modules)
```

Where N and M are the counts from the audit scope. Exit without entering plan mode and without writing any file.

## Out of scope

- Code reading. Use `/spec-drift` for code-vs-spec alignment.
- Schema/structural validation. Use `bin/spex validate` (already run in Step 2).
- Applying corrections. Use `/spec` on the drafted proposal.
