---
name: spec-drift
description: "Audit shipped code against its spec, identify drift, and draft a correction proposal in plan mode if found"
argument-hint: "[module-name]"
---

# /spec-drift — Audit Code Against Spec

Read the spec content for shipped components and compare it against the actual code. Identify cases where spec and code have diverged: spec describes behavior X, code does behavior Y. Draft a correction proposal in plan mode if drift is found.

This skill complements `/spec-review` (which audits internal spec consistency, no code reading). Use `/spec-drift` when shipped code has evolved past its spec OR when reviewers/implementers suspect drift between what the spec claims and what the codebase actually does.

## Step 1: Resolve scope

- If `$ARGUMENTS` is a module name, scope the audit to that module only.
- If `$ARGUMENTS` is empty, audit every module that has both a spec directory AND shipped code.

## Step 2: Detect modules with shipped code

For each module in `spec/project.json`:

- Spec directory: `spec/<module>/` exists with content leaves.
- Code directory: `<module>/` exists at the repo root (project convention; verify by reading `CLAUDE.md` for any project-specific layout notes).

If a module has spec but no code directory, skip it — there's nothing to audit yet (the components are pre-implementation). Report skipped modules in the final summary.

## Step 3: Read spec + code per module

For each module in scope, read in this order:

1. `spec/<module>/module.json` to get the component list and content paths
2. Spec content leaves: `arch_*.md`, `impl_*.md`, `test_*.md`, `flow_*.md`
3. Code in `<module>/`:
   - All `*.go` source files (skip `*_test.go` for the first pass — those are TDD artifacts and may legitimately exceed what arch describes)
   - For each component named in `module.json`, find the Go type/function with the same name

Do NOT modify any files. Read-only audit.

## Step 4: Drift detection (per component)

For each component in the module:

1. **Arch claim vs code surface**: does the code define a type/struct/function matching the component's name? Are its public methods consistent with what `arch_<component>.md` claims it does?
2. **Impl claim vs code body**: does the impl_section's described algorithm match the implemented one? Are the data structures named in impl present in code?
3. **Test claim vs test code**: does the test_section's scenarios correspond to actual `_test.go` cases? (One-way check: every spec scenario should have a corresponding test; the reverse — test cases without spec — is normal TDD and not drift.)
4. **Data flow claim vs interfaces**: do the shapes described in flow_*.md exist as Go types/interfaces in the participating components' code?

For each discrepancy found, record:

- Spec file path
- Code file path(s)
- The mismatch in one sentence
- Suggested resolution: "spec wrong" (code is correct, spec needs updating), "code wrong" (code drifted from spec, code needs fixing), or "both wrong" (genuine intent change required).

## Step 5: Bucket findings by lifecycle impact

Same rule as `/spec-review`:

- **Bead-producing-leaf drift** (spec changes to component, data_flow, test_section with `describes >= 2`, module): correction proposal goes through normal pipeline.
- **Non-bead-producing-leaf drift only** (spec changes to impl_section, test_section with `describes == 1`, requirement): candidate for `mode: refresh` once that pathway exists.

If "code wrong" findings exist, those become a normal change proposal regardless of leaf type — fixing code is regular implementation work, not refresh.

## Step 6: Draft proposal in plan mode (if findings)

Enter plan mode. Write a plan file with the instructions header and proposal content using the Change Proposal template from `/propose`. The proposal should clearly separate:

- **Spec corrections** (where code is right, spec is wrong) — in the "Proposed change" section, list spec edits needed.
- **Code corrections** (where spec is right, code drifted) — in the "Impact expectation" section, list which beads will need re-implementation work.
- **Mixed cases** — flag for human resolution; do not auto-decide.

If all findings are spec-only corrections AND all touched leaves are non-bead-producing AND refresh mode has shipped, declare `mode: refresh` in the proposal frontmatter:

```yaml
---
mode: refresh
---
```

If refresh mode has not shipped yet AND all findings are non-bead-producing-leaf-only, hold the proposal: report findings and exit without drafting (avoid stalled epics).

## Step 7: Exit plan mode and write proposal file

After approval, write to `spec/proposals/YYYY-MM-DD-spec-drift-<scope>.md`. Tell the user the path; remind them to review + commit + run `/spec` on it.

**STOP after writing the file.** Do not run `/spec` automatically.

## Step 8: No-findings exit

If no drift is found, print exactly:

```
spec-drift: no drift found (N components audited across M modules, K modules skipped pre-implementation)
```

Exit without entering plan mode.

## Out of scope

- Internal spec consistency. Use `/spec-review`.
- Bug-fixing the code itself. Use `/fix` with the bead the code corresponds to.
- Authoring new spec for unimplemented behavior. Use `/propose` + `/spec`.
