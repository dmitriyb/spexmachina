---
name: spec-review
description: "Audit the spec against itself for internal inconsistencies, present findings, and fix the spec in-session after explicit approval"
argument-hint: "[module-name]"
---

# /spec-review — Audit Spec for Internal Inconsistencies

Find inconsistencies WITHIN the spec itself: prose that contradicts a JSON declaration, a declared
edge no leaf explains, a link that resolves to the wrong node, an arch leaf that has drifted back
into describing Go. **No code reading** — the audit runs over `spec/` and the two gates, never over
the implementation. Where a rule below cites a `.go` path it is recording where that rule lives, not
handing you a file to open; a finding is never derived from source. No skill in this repo audits
code-vs-spec alignment; if that is what the user wants, say so and stop.

If findings exist, present them and stop; after the user's explicit go, fix the spec in this same
session. If nothing is actionable, exit with a one-line confirmation.

## Step 1: Resolve scope

- `$ARGUMENTS` is a module name → audit that module's leaves.
- `$ARGUMENTS` is empty → audit every module listed in `spec/project.json`.

Step 2 always runs over the whole spec regardless of scope. Both gates are corpus-wide and neither
takes a module filter.

## Step 2: Run both gates, and know which one sees what

`spex` has two gates. They do not overlap, and a spec can pass one while failing the other. Run both
before reading a word of prose, and surface everything they report as findings — they are the
highest-confidence signals and the cheapest to interpret.

```bash
bin/spex validate      # JSON report on stdout. exit 0 valid, 1 invalid
bin/spex diff --json   # exit 0 no errors (changes are fine), 2 errors found, 1 the tree did not build, 3 not a spex project
```

**`spex validate` is snapshot-free and corpus-local.** It always writes a report to stdout —
`{valid, error_count, warning_count, errors}` — indented on a TTY, one line when piped. There is no
`--json` flag; the output is always JSON. **`warning_count` is a stable contract field and is always
0**: no checker emits any severity but `error`. Do not triage warnings; do not ask whether a finding
is "only a warning". There is no such thing.

**`spex diff` is history-relative.** It compares the current tree against the baseline snapshot at
`.spex/snapshot.json` (the location the lifecycle pre-flight resolves), and
it is the only place two whole classes of error appear:

- **exit 2, `errors[].type == "incomplete_change"`** — a requirement change, or a `meta` change (the
  `module.json` / `project.json` envelope), with no matching content-leaf change. **`spex validate`
  cannot see this and never will**: every checker takes a spec directory and nothing else, so none of
  them has a baseline to compare against. A spec can report `"valid": true, "error_count": 0` and
  still fail `diff` with exit 2. That combination is the most common way a correction run
  stalls — check for it explicitly rather than assuming a clean `validate` means a clean tree.
- **exit 2, `errors[].type == "surviving_name"`** — a removed `api` or `component` whose declared
  name is still written somewhere in the spec corpus. It fires *only while the removal is still in
  the diff*; once the removal has been ingested the leftover mentions are invisible forever. A
  removal that already merged is a manual audit (Step 5).
- **`notes[]`** — non-gating disclosures, present in the JSON only when non-empty:
  `suppressed_by_live_name` (mentions discarded because a live node of the same or longer name covers
  them) and `unverifiable_module` (a removed node whose module is gone and whose name could not be
  recovered). Read them. Each marks a place where "no errors" means less than it looks.
- **exit 1** — the tree did not build (a missing or unreadable content file, malformed JSON).
  **stdout is empty**; the reason is on stderr. Nothing else is reported until that is fixed.
- **exit 3** — not a spex project: the lifecycle pre-flight refused before any tree was built —
  never initialised (stderr names `spex init`), or the snapshot or journal is missing or
  unparseable (broken; stderr names `spex doctor`). Fix the project state first; the audit
  cannot start without it.

`spex diff` reads the task journal (`.spex/history.jsonl`) as a second hash→name source for the removal check. It
never writes anything.

## Step 3: What the gates already cover — do not re-derive any of it

| check | catches |
|---|---|
| `schema` | JSON Schema conformance of `project.json` and every `module.json`; **`content` required and non-empty** (`minLength: 1`) on component, data_flow and test_section; unknown fields rejected (`additionalProperties: false`); id shape `^[a-f0-9]{12}$` |
| `content` | every declared content file exists, is not an absolute path, and contains no `..` |
| `link` | `[[<12-hex>\|<display text>]]` resolves to a live leaf; a name-based target is rejected; a link with no display text is rejected; an unterminated `[[` is rejected; a module node as target is rejected. **Only a fenced code block is skipped** — a link inside an HTML comment, inside a 4-space-indented block, or wrapped entirely in backticks is still scanned and must resolve. A bare 12-hex token is ignored everywhere except inside a `dot` fence, where it is a DOT node ID and is resolved |
| `id` | per-array id uniqueness; **api names are globally unique across modules**; api/component name declarability; referential integrity of `implements`, `uses`, `describes`, `provided_by` (module-local), `depends_on` and `preq_id`; project requirement **`priority` present and in 0–4** |
| `id_derivation` | every module-scoped id equals `IdentityHash(module, type, name)` |
| `dag` | cycles in module `requires_module`, requirement `depends_on`, component `uses` |
| `name_consistency` | `project.json` module name equals `module.json` name, lowercase |
| `test_coverage` | every component is described by at least one test_section |
| `requirement_coverage` | **two phases**: every project requirement is derived into ≥1 module requirement via `preq_id`, and every module requirement is implemented by ≥1 component |
| `coupled_section` | the `project.json` `sections` envelope, section id/name uniqueness, and coupled sections validated against their module's `section.schema.json` |

**There is no orphan check.** `validator/orphan_detector.go`, `CheckOrphans`, `detectOrphanComponents`
and `detectOrphanRequirements` were deleted. The non-empty `content` constraint replaced the
component / data_flow / test_section half of it, and `requirement_coverage` phase 2 already covered
the requirement half. Do not look for orphan output, and do not report its absence as a defect.

Three facts that change how the report reads:

- The `id` check runs uniqueness and name-declarability first and **returns early** if either fails.
  Everything in the deferred half goes unreported in that run: the cross-reference checks
  (`implements`, `uses`, `describes`, `provided_by`, `depends_on`, `preq_id`) **and the project
  requirement `priority` check**, which runs in the same deferred half. So a spec with a duplicate id
  can also be missing a `priority` and never say so. After fixing a duplicate id or a rejected name,
  re-run: more errors may appear.
- `id_derivation` exempts project-level requirement ids only. 15 project requirements in
  `spec/project.json` predate the convention and do **not** reproduce under `spex hash-id`. They are
  exempt, not correct. **Never propose "fixing" one** — it would rewrite the snapshot and every
  journal event keyed off it. Every module-scoped id derives and is enforced.
- **Name declarability** is new and its message is long. An `api` or `component` `name` is rejected
  unless corpus tokenization reproduces it exactly, in at least one and at most six whitespace-
  separated words. `spex validate [--json]`, `Validator (core)`, `Widget.` and `Bob's` all fail. The
  error names the replacement it will accept — quote that, do not invent one.

The valid node types are `requirement`, `component`, `data_flow`, `test_section`,
`api` and `module`; `spex hash-id --type` accepts exactly those seven and rejects anything else.

## Step 4: Read the spec

For each module in scope, read:

1. `spec/<module>/module.json`
2. every content leaf it declares — `arch_*.md`, `flow_*.md`, `test_*.md`
3. `spec/project.json`, for the `implements → preq_id → project requirement` chain

Keep a name→hash table open. You need it to check link display text and to cite nodes in the
findings report:

```bash
bin/spex render --format json --slim | jq -r '.nodes[] | "\(.type)\t\(.module // "-")\t\(.id)\t\(.name)"'
```

Slim output lists `module` rows too. **Module nodes are not link targets** — only leaves resolve — so
an id copied from a `module` row into a `[[…]]` is an error, not a shortcut.

**A `.md` under `spec/` that no `module.json` declares is invisible** to the merkle tree and to the
link check — nothing reports it, so check by hand:

```bash
for m in spec/*/module.json; do d=$(dirname "$m"); jq -r --arg d "$d" \
  '[.components[]?, .data_flows[]?, .test_sections[]?] | .[].content | $d + "/" + .' "$m"; done \
  | sort -u > /tmp/declared
find spec -name '*.md' -not -path 'spec/proposals/*' | sort > /tmp/ondisk
comm -3 /tmp/declared /tmp/ondisk   # empty output = clean
```

## Step 5: The audit — what no gate can see

This is LLM judgment: every question below is one neither gate can answer.

### Mandatory lenses — run all seven, count what you check

These lenses exist because implementation keeps catching what linear reading misses. Each is a
forced enumeration: build the pair list first, then check every pair. The final report must state
the pair count per lens (see Step 9) — a verdict without coverage numbers is not a verdict.

1. **Usage strings vs flag vocabulary** — `scripts/lens-usage-strings.sh` (deterministic, exit 1
   on mismatch): every `--flag` in a module.json component/api description must appear in the
   owning arch leaf. The PR #224 class: a description carrying a retired flag.
2. **Counts and enumerations vs the graph** — `scripts/lens-counts.sh` (worksheet): every written
   count of graph objects ("twelve constructors", "fifteen apis") paired with the actual number
   from the slim render. Judge each pairing.
3. **Flow claims vs arch authority** — for every pair (flow leaf, component in its `uses`):
   extract each claim the flow makes about that component and verify it against the component's
   arch leaf, which is authoritative. The PR #196 class: five flow claims contradicting
   `arch_reconciler.md`.
4. **Retired lexicon sweep** — `scripts/lens-lexicon.sh` (deterministic, exit 1 on hits): every
   term declared under a proposal's "## Retired vocabulary" heading, swept across the corpus
   outside `spec/proposals/`. The PR #201 class: sibling fields the migration never updated.
   Its blind spot is the bare name of a dissolved module — lens 7.
5. **New surfaces vs project constraints** — every api or component added or materially changed
   in the audit scope, checked against every project-level non-functional requirement (the
   no-subprocess-vs-`spex upgrade` class: a new surface that collides with a standing constraint
   nobody re-read).
6. **Intersection cells** — for every leaf declaring two or more modes or interacting flags:
   enumerate the mode/flag pairs and confirm the leaf decides each intersection (the
   `--check`-during-anomaly class: two sections each valid alone, contradictory on one cell).
7. **Dissolved module names** — `scripts/lens-dissolved-modules.sh` (deterministic, exit 1 on
   hits, exit 2 if it cannot sweep): the class lens 4 cannot cover, because declaring a bare
   module name as a retired term would match every ordinary use of the word. It derives its own
   terms — every module a `removed` event names that `project.json` no longer lists — and matches
   only forms in which the name is an actor or a location: possessive, arrow chain (including the
   `\u2192` escape a `module.json` string carries), parenthesised qualifier, path fragment,
   `"module": "<name>"`, and "the <name> module", each tolerating backticks around the name.
   Precision over recall by design: a clean run does **not** mean the corpus is free of stale
   module references. A module named as a bare subject ("so impact never sees it"), as an
   adjectival compound ("impact analysis"), or in a `<module>: <Component>` bead title matches
   nothing and must still be read for. The kfem class: nine sites lens 4 reported clean over.
   A hit that is *correct* — a leaf documenting the journal must quote a removal record, and a
   removal record names the dead module permanently — is excused by name in
   `scripts/lens-dissolved-modules.allow` (`<path-relative-to-spec-dir><TAB><substring>`, with the
   reason as a comment), never by rewriting the example into something that could not occur.
   Both halves must match, so a new stale reference in an already-listed file still fires;
   suppressions are counted in the output and a dead entry is reported.

**Component (`arch_*.md`)**

- Does the prose describe behaviour that satisfies every requirement in `implements`? Does it claim
  behaviour no requirement promises?
- Every declared edge should be *explained* in the leaf, in the sentence that already discusses the
  target, carrying a `[[<hash>|<display text>]]` link. A link appended to a trailing list, or under a
  `## References`-style heading, is a finding: the link is meant to sit inside the explanation, not
  next to it. `scripts/link-check.sh spec <diff.json> [<module>]` reports the missing ones
  mechanically (component leaf owes `uses` ∪ `implements` ∪ every api whose `provided_by` names it;
  data_flow leaf owes `uses`) — but it is diff-scoped and only inspects leaves the diff reports as
  touched. `scripts/link-spread.sh spec/<module>` catches the dump-at-the-end shape.
- **Link display text is free-form and unchecked.** A link can resolve perfectly and still read as
  the wrong node. Check every display string against the slim table.
- **Arch leaves name no language.** A Go fence, a `func` signature, a `ctx context.Context`
  parameter, or a sentence whose subject is a Go identifier belongs in code, not in an arch leaf. The
  single exception: the component implements a requirement **whose description names the algorithm or
  the bound the fence encodes** — then it stays, as language-neutral pseudocode. The test is the
  requirement's *description*, not its `type`.

  ````
  grep -nE '^```go|^func |\(ctx context\.Context' spec/<module>/arch_*.md
  ````

  The Go-stripping migration is in flight, so a leaf that has not been migrated yet will light this
  up heavily. Report **one finding per leaf**, naming the offending sections — never one per line.

**API (declared in `module.json`; there is no api content file)**

- `name` is the exact external surface string as callers write it — `spex diff`,
  `GET /v1/specs/{id}`, `schema.IdentityHash`. **Never a signature.**
- `provided_by` names components in this module only; another module's involvement rides on component
  `uses` edges, not here.
- `group` is freeform and `spex` never branches on it.
- **An api's *identity* moves only when its name moves — its leaf hash covers more than that.** The
  leaf hashes `description`, `group`, `id`, `name` and `provided_by`, so a description-only edit does
  move the hash and `spex diff` reports the api as `modified` at impact level `contract`. Do not
  treat the description as machine-invisible.
- **The real blind spot is surface drift nobody wrote down.** An added flag, a new query parameter, a
  changed response field moves no hash and produces no diff entry — an api has no content file and
  nothing in `module.json` records the external surface beyond the name. The `description` is the one
  field where such drift *can* be made visible, which makes it the audit target: compare each api's
  description against the arch leaves of the components in `provided_by`, and against the surface the
  name claims.
- Renaming an api is delete-plus-create, so a proposed rename must also survive the removal-time name
  sweep. Say so in the findings report.

**Data flow (`flow_*.md`)**

- Does the prose describe the shapes and contracts moving between the components in `uses`, naming
  each one correctly?
- `uses` is not a decorative reference list. Every component listed in it gains the data_flow as a
  dependency so the sorter sequences the flow's op ahead of theirs (`plan/action_classifier.go`,
  the data_flow add-on beside `depsFor`). A component the narrative walks through but `uses` omits
  is a real defect.

**Test section (`test_*.md`)**

- `len(describes) >= 2` — the scenarios must exercise behaviour spanning at least two of the
  described components. One component, one method, one assertion is a unit test and does not belong
  at this shape. This shape produces a bead; the other does not.
- `len(describes) == 1` — unit/component tests bundled with that component's work. Appropriate here.

**Requirements (`project.json` + `module.json`)**

- Are the descriptions specific enough to be testable?
- Does each `preq_id` chain lead to a project requirement with consistent semantics?
- A description naming a node type, a file path or a command that no longer exists is a finding.

**Corpus-wide**

- A name still written in prose for something removed in an *already-ingested* change. `diff`'s
  `surviving_name` check cannot see it any more (Step 2), so this is the one removal audit that is
  entirely manual.

## Step 6: Bucket findings by lifecycle

Bead-producing node types are **`component`, `data_flow`, and `test_section` with `describes >= 2`**
(contract in `spec/plan/arch_action_classifier.md`). Everything else produces no bead: `requirement`
(project or module), **`api`**, `test_section` with `describes == 1`, `meta`, and
**`module`**. A module is not a leaf, so no change ever reaches the classifier carrying node type
`module`; a `module.json` edit surfaces as that module's `meta/<hash>` leaf, and `meta` produces no
bead.

`spex ingest --mode refresh` **has shipped.** It absorbs content-only edits to any leaf, plus
structural additions and removals of an explicit type list (`ingest/refresh.go`,
`refreshAbsorbable`):

| node type | added | removed |
|---|---|---|
| `requirement` | absorbed | absorbed |
| `api` | absorbed | absorbed |
| `component` | **refused** | absorbed |
| anything else (`data_flow`, `test_section`, `module`, `meta`) | refused | refused |

So:

- Findings on bead-producing leaves → the fix owes beads; say so in the Step 7 report.
- Findings on both kinds → the bead-producing changes drive the lifecycle; the fix owes beads.
- Findings confined to the absorbable set → the fix owes none; say so in the Step 7 report.

## Step 7: Present findings and stop (if findings)

The findings report is the deliverable of the audit. Present it as a table, one row per finding:

| # | Node | File | Lens/gate | Defect | Bucket |
|---|------|------|-----------|--------|--------|

`Node` carries the full node name with the identity hash in parentheses — `Action Classifier
(72ab19c303f1)`. `Bucket` is the Step 6 lifecycle bucket (bead-producing or absorbable), so the
user sees what each fix will owe downstream.

Below the table, per finding: the exact spec edit that resolves it — which field, which prose
paragraph — and, where the correct resolution needs a decision the spec does not record, the
open question stated as a question for the user to settle.

**Then stop and wait for the user's explicit go** — fixing is a second, separately authorized act.
Partial approval is normal: fix what was approved and list the rest as declined-or-deferred in the
final report.

## Step 8: Fix in-session (after the go)

- The fixes land on the current working branch: spec-review runs after `/spec` or `/drift-fix`
  and rides that run's branch. A standalone audit whose approved findings warrant structural
  rework ends in a review proposal — hand the findings to `/propose`.
- Apply the approved corrections with `/spec` discipline: ids via `bin/spex hash-id`, the
  per-node check after each edited leaf.
- Re-run both Step 2 gates. The run ends with both green, or with every residual finding reported
  verbatim, with the command that produced it, named as another run's work.
- Report what changed, then remind the user to review and commit on the branch, landing via PR.

The run ends there.

## Step 9: No-findings exit

If Step 5 produced nothing actionable AND both Step 2 gates are clean, print the coverage
contract — a bare "clean" is not a valid exit:

```
spec-review: no actionable findings (N nodes audited across M modules)
  lens 1 usage-strings: <pairs checked> (script exit 0)
  lens 2 counts: <claims judged>
  lens 3 flow-vs-arch: <claim pairs checked>
  lens 4 lexicon: <terms swept> (script exit 0)
  lens 5 surfaces-vs-constraints: <surface x constraint pairs>
  lens 6 intersections: <cells checked>
  lens 7 dissolved-modules: <modules swept> (script exit 0)
```

Where N and M come from the audit scope. A clean verdict asserts only "nothing found at these
counts and depth" — never "no defects exist". If a lens was inapplicable, say so with a reason. Exit there; the audit leaves every spec file as it found it.

## Out of scope

- Code reading. Findings come from `spec/` and the two gates only — the `.go` paths cited above are
  provenance for a rule, never a reading assignment — and no skill in this repo audits code-vs-spec
  alignment.
- Anything `spex validate` or `spex diff` already reports (Step 3). Surface it; never re-derive it.
