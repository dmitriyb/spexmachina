# The authoring loop

`spex` owns the structural half of spec-driven development. The creative half
— deciding what the spec should say — happens in an interactive session, and
five Claude Code skills under `skills/` drive it. They call `spex` subcommands
for everything mechanical, which keeps the two halves cleanly separated: the
skills never guess at structure, and `spex` never guesses at intent.

Each skill is a `SKILL.md` invoked as a slash command.

The split runs through authoring itself. The structure of a spec — which
array a node lives in, its id, its content path, its edges, the headings its
leaf carries — is written by the authoring commands (`spex node`, `spex edge`,
`spex leaf scaffold`, driven by `spex profile show`), which refuse a change the
validator would reject and print what a change obliges. The skills therefore
carry no table of node types or file conventions; they hold the judgement —
what belongs in a leaf, what a scenario is, what a proposal should say — and
one loop: read the profile, declare, link, scaffold, write prose, validate,
apply the fix a refusal names, repeat. Because the loop never names a node
type, it is the same loop under any profile. That claim is verified for the
default profile and exercised by a test-only custom profile, and by nothing
else until a project with its own profile runs it.

## The doctrine

Three rules govern the whole loop. Everything below is an expression of them.

**The spec is the truth, and it changes only here.** Only the authoring loop
writes `spec/`. Automated implementer contexts are structurally denied write
access to it — an implementer that finds a spec defect files a drift report,
never a spec edit.

**The baseline moves only deliberately.** `.spex/snapshot.json` advances by a
*mint* when work is born, or a *refresh* when a correction owes no task work.
Never automatically, never as a side effect. Every refresh states its reason.

**An epic with an untriaged `drifts/` is not closed.** Reports accumulate
during implementation and are cleared by `/drift`, not before.

## `/propose`

*Research the spec and draft a structured proposal in plan mode.*

The entry point for any change. It detects whether you are proposing a new
project or a change to an existing one, enters plan mode, clarifies intent,
then researches the current spec before drafting anything — so the proposal
argues against what actually exists rather than what it assumes exists.

It deliberately constrains the draft to what the authoring commands can
declare, and it reads the change's cost off them rather than estimating it:
the structural changes are applied to a scratch copy of the spec and the
impact expectation is taken from the commands' `obligations` and from
`spex diff` over the copy. The output is a proposal draft committed to
`spec/proposals/drafts/`; `/mint` registers it into `spec/proposals/`, which is
what makes every later change traceable to a stated reason.

## `/spec`

*Read a proposal and author spec files: `project.json`, `module.json`, and
markdown content leaves.*

Takes a proposal and writes the spec through the commands: every node is a
`spex node add`, every edge a `spex edge add`, every removal or rename the
command that performs it as one transaction, and every leaf starts from the
skeleton the commands scaffold. The skill writes prose and nothing else
directly. A refusal's `fix` is the next command; a report's `obligations` are
the leaves that now owe an edit.

The bulk of the skill is judgment about *where content belongs* — what earns
an architecture leaf versus a description field versus a test section, what a
scenario is and is not — which is exactly the part a schema cannot enforce and
`spex validate` cannot check.

## `/spec-review`

*Audit the spec for internal inconsistencies (no code reading) and draft a
correction proposal in plan mode if findings exist.*

A read-only audit of the spec against itself. It runs both gates first and
then explicitly does **not** re-derive what they already cover — the point is
to find what no gate can see: contradictions between leaves, requirements that
no longer mean what their components implement, coverage that is technically
satisfied but semantically empty.

Findings are bucketed by lifecycle, and if there are any, the skill drafts a
correction proposal rather than editing the spec directly. No findings is a
clean exit, not a failure.

It never reads implementation code. A mismatch between spec and code is drift,
and drift has its own path.

## `/mint`

*Move the baseline for a committed spec change: decide mint-vs-refresh per
node, assess absorb marks, then run the pipeline.*

The loop's last phase and the only place the baseline moves. It owns the
doctrine the other skills reference: the direction axis (does the corrected
spec change what code must do, or record what code already provably does),
the per-node absorb assessment with its four rules, the plan → adapter →
ingest mechanics with their label budgets, the refresh pathway, and the
gate-asymmetry override. `/spec` ends at green gates and a commit; `/mint`
starts from that commit.

## `/drift`

*Triage drift reports filed by implementers: validate, verdict, and apply the
accepted corrections; the audit is `/spec-review`'s, the baseline decision
`/mint`'s, and the user-level `/drift-workflow` sequences all three.*

Implementers file `drifts/drift-<task-id>.json` (schema:
`schema/drift.schema.json`) when they find a spec defect. A report names the
claim, the authoritative source that contradicts it, and the evidence. It is
**non-blocking** when the defect does not gate the implementer's own contract
— it rides along in that task's PR and is triaged after the epic. It is
**blocking** when the task's own contract is ambiguous, in which case it
travels as its own PR and stops the epic.

`/drift` collects and validates the reports, classifies each, fixes the
spec, and hands over; the baseline call is made explicitly on `/mint`'s
direction axis — mint if the correction births work, refresh if the spec is
converging on shipped, test-pinned behaviour — and executes it through
`/mint`. That decision is the reason the skill exists, and it is made by a
human-supervised session, never by a box.

## How they fit together

```
/propose ──▶ proposal draft in spec/proposals/drafts/
              │
           /spec ──▶ spec/ written, gates green, committed
                                          │
                         /mint ──▶ diff → plan → adapter → ingest
                                   (or refresh; baseline moves here)
                                          │
                            implementation happens; defects found
                            become drifts/drift-<task-id>.json
                                          │
                                    /drift ─────▶ spec corrected,
                                                   baseline decided,
                                                   reports cleared

/spec-review runs at any time against the spec alone
```

See [`architecture.md`](architecture.md) for what happens on the structural
side of that diagram, and [`configuration.md`](configuration.md) for the
format the skills author.
