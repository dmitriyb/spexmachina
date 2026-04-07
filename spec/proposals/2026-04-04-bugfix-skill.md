# Change Proposal: Bugfix Skill

## Context

The current skill set (`/propose`, `/spec`, `/implement`, `/review`, `/fix`) covers the spec-driven pipeline: proposal → spec change → bead creation → implementation → review. But when a real bug is found — where the spec correctly describes the intended behavior and only the code is wrong — there is no skill to handle it.

The `/implement` skill expects beads created by the spex pipeline (feature/task types from spec changes). A bug doesn't start with a spec change. It needs a different entry point: create a bug-type bead via `br create`, link it to the relevant component, guide the fix with TDD, and close the bead. No spex pipeline involvement.

This gap was discovered when a bug was found in the impact module's structural change handling. The spec was partially wrong (Case B: spec change needed), but it surfaced the question: what do we do when the spec is correct and the code is simply broken?

### Two bug cases

1. **Spec is wrong** — the spec describes incorrect behavior. This is a normal spec change: update the spec → diff → impact → apply → beads created → implement. Not a code bug per se — it's a spec correction that flows through the standard pipeline.

2. **Spec is correct, code is wrong** — a true bug. The spec and content leaves already describe the right behavior, the implementation just doesn't match. No spec change needed. Create a bug bead manually, fix the code, close the bead. This is what `/bugfix` handles.

## Proposed change

### 1. New `/bugfix` skill

Create `skills/bugfix/SKILL.md` (and symlink in `.claude/skills/`) with this workflow:

**Input:** Bug description, optionally a component reference or bead ID.

**Step 1: Locate the bug.** Read the relevant code, tests, and spec content leaves. Identify which component has the bug and confirm the spec correctly describes the intended behavior. If the spec is also wrong, stop and tell the user to use the spec change pipeline instead (proposal → /spec → pipeline → /implement).

**Step 2: Create a bug bead.** Run `br create` with:
- Title: `Bug: <module>/<component> — <short description>`
- Type: `bug`
- Link to the original component's bead via `br dep add` (depends on the component bead)
- Label with the spex mapping ID if applicable

**Step 3: TDD — write the failing test first.** Write a test that reproduces the bug and fails. Run it to confirm failure. This ensures the fix is verifiable and the bug is well-understood before code changes.

**Step 4: Implement the fix.** Modify the code to make the failing test pass. Run the full test suite to confirm no regressions.

**Step 5: Close.** Close the bead with `br close` and PR reference.

### 2. No spec module changes

This skill is a workflow instruction file only. No changes to project.json, module.json, or any spec content. No beads created through the spex pipeline — the skill itself creates beads via `br` directly.

### 3. Skill file location

- `skills/bugfix/SKILL.md` — the skill definition
- `.claude/skills/bugfix.md` — symlink for Claude Code discovery

## Impact expectation

**New beads:** None through the spex pipeline. This is a skill file addition only.

**Modified beads:** None.

**Closed beads:** None.

Estimated scope: 1 session to write the skill file and symlink.
