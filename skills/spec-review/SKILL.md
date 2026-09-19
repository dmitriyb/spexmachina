---
name: spec-review
description: "Audit the spec against itself for internal inconsistencies, present findings, and fix the spec in-session after explicit approval"
argument-hint: "[<module-or-id> ...] | all"
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

The audit runs once, in one context, over a scope a script computes and the audit itself widens
as findings demand. It never splits by module across several reviewers: a contradiction lives on
an edge between two nodes, and a reviewer who holds only one end of the edge cannot see it. Every
scoped review before this rule missed exactly that class.

## Step 1: Resolve scope

Before the first staging of a session, run the scripts' self-test once — `python3
skills/spec-review/selftest.py` — a few seconds, no model; a review is the most expensive place to
find a script bug, and every one found so far was findable here.

The scope is a set of nodes, not a list of modules, and a script decides it:

```bash
STAGE=$(mktemp -d)
python3 skills/spec-review/scope.py --stage "$STAGE" [--hops N] [--free-preq] [--diff <diff.json>] [<seed> ...]
python3 skills/spec-review/scope.py --stage "$STAGE" --all
```

`--stage` copies every file the scope names — `project.json`, a `module.slim.json` for every module
in scope (its in-scope requirements in full, its other nodes as id, name, description and reference
fields; never a full `module.json`), every in-scope, seam and mention leaf — into `$STAGE` under
its spec-relative path, appends the rows to `$STAGE/SCOPE.tsv`, writes `$STAGE/PAIRS.tsv`, the pair
list the judgement lenses decide (Step 5), writes `$STAGE/UNDECLARED.tsv`, the declared-versus-on-disk
leaf check over the whole spec (Step 4's check, done for you — never recompute it), and creates
`$STAGE/FINDINGS.tsv` and `$STAGE/CELLS.tsv` with their headers for you to append to (Step 7). With
`--stage` the script prints its summary lines only; the rows are in the files. **The stage is the only
place the audit reads spec files from.** Reading `spec/` directly, whether by path, by glob, by
loop or through a variable, is a failed review: the caller audits the transcript against the
stage afterwards (`skills/spec-review/audit-reads.py`) — by path and by file content, so the
form of the command does not matter — and rejects a run that read outside it, whatever its
report says. Commands that *run* over the spec — `bin/spex validate`, `bin/spex diff`, the slim
render, the deterministic lens scripts — are not reads and stay as they are; their output is a
report, and a report is read in full.

**How to read, because the cost of a review is turns times context.** Follow `$STAGE/READ-PLAN.tsv`:
one `cat` per row, the row's files together, in row order, and nothing else — the plan is packed
under the harness limit in a fixed order, so every reviewer of the same change reads the same
bytes in the same sequence and the reading phase is served from the prompt cache for all but the
first. Read the whole plan first, then judge. An expansion appends rows; read those the same way. Never read a persisted tool-result file. Never read a
staged file twice; if you need it again, you still have it. The audit counts overflows,
re-reads and duplicates — of leaves and of the slim JSON files alike — and fails the run on any
of them. Do not run `scripts/lens-requirements.sh`: `PAIRS.tsv` is that worksheet, and the script
prints descriptions the slim files already carry. Run `scripts/lens-counts.sh` once and read only
its lines naming staged paths.

- `$ARGUMENTS` empty → the **seed** is every node `spex diff --json` reports changed against the
  baseline (a `meta/<hash>` change seeds that module's node). This is the per-change default.
- `$ARGUMENTS` lists identity hashes and/or module names → the seed is those nodes (a module name
  seeds every node of that module). An explicit seed is still expanded; naming a module never
  confines the audit to it.
- `$ARGUMENTS` is `all` → the seed is every node.

**The `all` contract.** A full audit ends in a review proposal, never in spec edits, and Step 8
does not apply. The reviewer stops at the Step 7 report. The caller presents the audit results,
the cost figures and the findings table, then discusses the open questions one per message —
what the spec says today, the possible edits, each assessed on correctness rather than on size,
and a lean — until every finding that holds is accepted or declined. The caller then writes
`.spex/runs/review/` (run scratch, announced first): `FINDINGS.tsv`, `VERDICTS.tsv`, and
`DECISIONS.tsv` with one row per finding — `id`, `accepted` / `declined` / the chosen option,
one sentence of reason. That directory is `/propose`'s input, and the run ends there.

The script's summary ends with a `size` line, `large` or `small` from the thresholds in
`skills/spec-review/review.json` (seam reach and staged share of the corpus), and the number of
independent reviewers that size calls for. How many reviews run, and how their findings are
unioned, is the caller's orchestration (`/spec-workflow`); this skill describes one review, and a
reviewer never sees another reviewer's stage. This is the periodic full audit, not the
  per-change run; the walk below still applies, so each node is read against its neighbours,
  which is the edge count and readable, rather than "everything against everything", which is
  not.

The script prints, tab separated, every node within `--hops` (default 1) of the seed: `hop`, `id`,
`type`, `module`, `name`, `leaf`, and the edge and node it was reached through. Every declared
reference edge counts, in both directions — `uses`, `implements`, `describes`, `provided_by`,
`depends_on`, `preq_id`, `requires_module` — with two free moves: a node reaches its own module
node at no cost, so the modules that require a changed module, and the ones it requires, are
always in scope at hop 1; and a module requirement reaches its project requirement at no cost,
because that parent is what the requirement is read against. `--free-preq` makes the downward
move free too, so sibling requirements under a shared project requirement enter at the seed's
hop; on this corpus that is most of the spec, so it is off by default and the siblings sit in
the frontier instead.

Rows labelled `seam` are the arch and flow leaves of every module that requires a module in
scope. Those modules consume the changed contract, and the reliance is written in their leaves —
linked, named, or paraphrased — so the leaves are read whether or not a finding points at them.
Rows labelled `mention` are leaves outside the scope that link a seed id or name a seed component
or api as a whole word: a reliance the graph does not declare, visible only because the author
named the node. They are staged and read, and each is a lens 11 candidate. A paraphrase is not
caught, and nothing mechanical catches it. Rows labelled `cites` are the reverse of `mention`: nodes outside the scope that a seed leaf links
or names. A claim a seed makes about another node is read because the seed made it, so the first
hop of what used to be finding-driven expansion is now deterministic; expansion beyond it is still
the walk's. Rows labelled `frontier` are the nodes one edge beyond
`--hops`: reachable, not in scope, not staged. Keep that list. It is what the walk expands into,
and what the report must print if the walk stops short.

**What "in scope" means for reading.** Every leaf the stage holds, `seam` and `mention` rows
included, is read in full. A module node in scope means its `module.json` is read — the
requirements and descriptions, which are the module's contract. A project requirement in scope
means its entry in the staged `project.json`. Nothing in the stage is skimmed, and nothing outside
it exists to the audit until an expansion call stages it.

**The walk.** The audit is a graph walk, and it expands on findings, inside this one run:

1. Read the scope. Run every lens over it (Step 5). For each finding, locate the node whose
   contract the finding is about and read its **requirement edge first** — `implements` for a
   component, `preq_id` for a module requirement — because the requirement decides who is wrong.
2. If the finding is settled inside the node — the leaf contradicts its own requirement, or
   itself — propose a correction on that node alone.
3. If the finding involves another node — a claim the leaf makes about a neighbour, a neighbour
   that relies on the claim — go to that node: read its leaf and its requirement edge, assess the
   contradiction against both sides, and decide which side the correction lands on (the rule
   below). That node is now a seed: run the scope script again with its id, the same
   `--stage` and `--expand`, which copies its neighbours and the leaves it cites into the stage
   and appends them to `SCOPE.tsv` and the read plan — never the seam rows, which belong to the
   change, not to a walk step — and read them the same way. Every expansion is one such call; the report lists them.
4. Repeat until no proposed correction changes a claim on a node whose neighbours have not been
   read. That is the fixed point, and the run ends there — one coupled correction set, not a
   chain of re-reviews that each start from a summary of the last.

**Who wins.** The walk decides where to go, not which side to correct:

- a leaf that contradicts its requirement is corrected toward the requirement;
- two leaves that contradict each other are corrected toward the requirement they share, or
  toward the requirement of the node that owns the claim (the arch leaf of the component that
  `implements` it), never toward whichever was read first;
- two requirements that contradict each other stop the walk at that edge and become a question
  for the user — that is a proposal-level change, not a correction.

**Expand on claims, not on edits.** A correction that changes a claim another node could rely on
— an exit code, a field, an ordering, a precondition, a file written — makes its node a seed. A
correction that changes wording and leaves every statement's meaning intact does not. Without
this the walk grows on every typo.

**The cap.** Stop expanding at `--hops` plus 3 seeds-of-seeds — a walk that goes deeper is a
change larger than its proposal said, and the user decides whether to follow it. When the cap is
hit, the report carries a **Frontier** section (Step 7) listing every node the walk did not enter,
with the edge that reached it. A report with a non-empty frontier never prints the no-findings
line: a truncated walk that reads as clean is the exact failure this rule removes.

Step 2 always runs over the whole spec regardless of scope. Both gates are corpus-wide and neither
takes a node filter; so do the deterministic lens scripts.

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

For every node the scope lists, read what its row names, **from the stage**:

1. its leaf — `arch_*.md`, `flow_*.md`, `test_*.md` — in full, never an excerpt
2. for a module node, `$STAGE/<module>/module.json`; for a project requirement, its entry in
   `$STAGE/project.json`
3. the `implements → preq_id → project requirement` chain of every component in scope — the
   module requirement at hop 1 and its project requirement with it, since that move is free

Keep a name→hash table open. You need it to check link display text and to cite nodes in the
findings report:

```bash
bin/spex render --format json --slim | jq -r '.nodes[] | "\(.type)\t\(.module // "-")\t\(.id)\t\(.name)"'
```

Slim output lists `module` rows too. **Module nodes are not link targets** — only leaves resolve — so
an id copied from a `module` row into a `[[…]]` is an error, not a shortcut.

**A `.md` under `spec/` that no `module.json` declares is invisible** to the merkle tree and to the
link check — nothing reports it. The scope script computes the check at staging time and writes
`$STAGE/UNDECLARED.tsv`; an empty file is clean, and every row is a finding. Never recompute it
over `spec/`.

## Step 5: The audit — what no gate can see

This is LLM judgment: every question below is one neither gate can answer.

### Mandatory lenses — run all ten, count what you check

These lenses exist because implementation keeps catching what linear reading misses. Each is a
forced enumeration: build the pair list first, then check every pair. The final report must state
the pair count per lens (see Step 9) — a verdict without coverage numbers is not a verdict.

For lenses 3 and 8, and for the seam and mention rows, the pair list is already built:
`$STAGE/PAIRS.tsv`, one row per pair with both ends staged — component × implemented requirement
with the project requirement behind it, data flow × used component, seam leaf × the changed
contracts, mention leaf × the seed it names. **Write one verdict per row in the last column
before writing the report**: `clean`, or the finding number it produced. Verdicts are data, not
prose: fill the column with a script or one edit per batch of rows and do not narrate clean pairs
— reasoning is spent only where a row becomes a finding. The audit rejects a run with a blank
verdict, and every finding from these lenses cites its row. A pair listed and not judged is how
the last review missed a critical finding on a leaf it held; the file is what makes "check every
pair" checkable. Expansion calls append rows; judge those too.

For lens 6, `$STAGE/CELLS.tsv` is pre-filled with the candidate cells the script can see: every pair
of `--flags` a staged arch or flow leaf names. Decide each one — the verbatim sentence that decides
the intersection, `INDEPENDENT` when the two flags never interact (neither names the other, no
shared mode, no exclusion), or `UNDECIDED` with the finding number it produced — and append the
cells the script cannot see, modes that are not flags, the same way. A blank cell fails the audit;
most candidate pairs are `INDEPENDENT`, and writing that is the enumeration doing its job. An intersection count in a report becomes a list the audit can see; an undecided
cell is a finding by construction.

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
   adjectival compound ("impact analysis"), or in a `<module>: <Component>` task title matches
   nothing and must still be read for. The kfem class: nine sites lens 4 reported clean over.
   A hit that is *correct* — a leaf documenting the journal must quote a removal record, and a
   removal record names the dead module permanently — is excused by name in
   `scripts/lens-dissolved-modules.allow` (`<path-relative-to-spec-dir><TAB><substring>`, with the
   reason as a comment), never by rewriting the example into something that could not occur.
   Both halves must match, so a new stale reference in an already-listed file still fires;
   suppressions are counted in the output and a dead entry is reported.
8. **Requirements vs the leaves that implement them** — `$STAGE/PAIRS.tsv`, the `lens8` rows: for
   every component in scope, each requirement in its `implements`, and the project requirement
   behind it, paired with the component's arch leaf (the test leaves that describe it are in the
   slim module file). Read each requirement claim by claim against the **arch leaf**: the arch leaf is
   what honours a requirement, a test leaf only asserts it, and a claim carried by a test scenario
   alone is contract that leaked out of the arch leaf (the 2026-09-06 class). A claim the arch
   leaf no longer honours is a **critical** finding: it goes first in the Step 7 table, and its fix
   direction — bend the requirement to the leaves, or the leaves back to the requirement — is a
   question put to the user, never a silent edit of the requirement. The "with their dates"
   class: an arch leaf rewritten for a different purpose dropped a field the requirement still
   asks for; the code followed the leaf, a later review corrected the test leaf against the arch
   leaf, and nothing in three passes read one level up. Lens 3 reads flows against arch leaves;
   this lens is the same read one level higher, and the only place the requirement is re-read.
9. **Unit-shaped scenarios in test leaves** — `scripts/lens-test-shape.sh` (deterministic, exit 1
   on hits): a scenario in a `test_*.md` leaf that calls a Go identifier in code (`Name(...)`) and
   never invokes a `spex` subcommand. Test leaves hold module integration/acceptance scenarios
   only; unit cases live in Go `_test.go` files (`spec/proposals/2026-03-09-test-strategy.md`,
   Level 1). The 2026-09-06 class: 412 unit-shaped scenarios accumulated across 26 leaves, and
   every task born from such a leaf closed empty because the Go test already existed. The lens
   sees only the code-formatted call; a unit case written in prose passes it. So for every
   scenario in the audit scope, judge by shape as well: UNIT when one component is exercised in
   isolation — one function or one component's input→output on a specific case, the assertion on
   its return value or error, a fixture directory being that component's input, not an external
   system; KEEP when a `spex` subcommand is invoked and its exit code, stdout or files asserted,
   when two or more components act together, when a module boundary is crossed, or when an
   external system the process does not own is driven (sandbox tracker, git repository, HTTP
   server, CI harness). A unit-shaped scenario is a finding: the fix is deletion, never a
   rewrite into acceptance language. A kept scenario that legitimately calls a Go identifier is
   excused in `scripts/lens-test-shape.allow` (`<path><TAB><heading substring>`, reason as a
   comment); a dead entry is reported. The same script holds the leaf's shape: every test leaf
   carries a `## Setup` section (NO-SETUP otherwise) and every heading scenario opens with a bold
   `**Given**` line (NO-GIVEN otherwise); bullet cases carry their state inline.
10. **Requirement carried by an arch leaf** — `scripts/lens-requirement-links.sh` (deterministic,
   exit 1 on hits): every module requirement is linked by id (`[[<id>|...]]`) from the arch leaf of
   at least one component that implements it. The structural half of lens 8: a requirement whose
   only carrier is a test leaf fails here before anyone has to read for it. Whether the linking
   leaf actually states the claim is lens 8's judgment.
11. **Undeclared module dependency** — for every arch or flow leaf in scope, one question: does
   it rely on a contract outside its module — another module's command, exit code, file, pre-flight,
   ordering — whether it links the node, names it, or paraphrases it? If yes, `project.json` must
   declare `requires_module` from this module to that one. A reliance with no declared edge is a
   finding, and the fix is the edge (`spex edge add <module-id> requires_module <module-id>`), not a
   rewrite of the sentence. The edge is what makes the scope walk reach the dependency next time;
   a paraphrase is invisible to every mechanical check, so this lens is the only place it is
   caught, and the 2026-09-17 class is why it exists: a validator leaf relying on the lifecycle
   pre-flight with no edge to lifecycle, and no reviewer reaching lifecycle. Count the leaves read.

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
  at this shape. This shape produces a task; the other does not.
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

Task-producing node types are **`component`, `data_flow`, and `test_section` with `describes >= 2`**
(contract in `spec/plan/arch_action_classifier.md`). Everything else produces no task: `requirement`
(project or module), **`api`**, `test_section` with `describes == 1`, `meta`, and
**`module`**. A module is not a leaf, so no change ever reaches the classifier carrying node type
`module`; a `module.json` edit surfaces as that module's `meta/<hash>` leaf, and `meta` produces no
task.

`spex ingest --mode refresh` **has shipped.** It absorbs content-only edits to any leaf, plus
structural additions and removals of an explicit type list, read from the resolved profile's
`absorbable` declarations (`ingest/refresh.go`, `schema.Profile.Absorbable`); the default profile
declares the table below:

| node type | added | removed |
|---|---|---|
| `requirement` | absorbed | absorbed |
| `api` | absorbed | absorbed |
| `component` | **refused** | absorbed |
| anything else (`data_flow`, `test_section`, `module`, `meta`) | refused | refused |

So:

- Findings on task-producing leaves → the fix owes tasks; say so in the Step 7 report. What
  each owes is decided at mint time from the task-state artifact: a node whose earlier task is
  finished (absent from the artifact) takes one plain create, an open one is retargeted, a
  claimed one refuses the run — never a close-and-recreate.
- Findings on both kinds → the task-producing changes drive the lifecycle; the fix owes tasks.
- Findings confined to the absorbable set → the fix owes none; say so in the Step 7 report.

## Step 7: Present findings and stop (if findings)

**A finding is two verbatim quotes and one sentence.** Before the report, append every finding
to `$STAGE/FINDINGS.tsv`: `id`, the two node ids, the two staged files, the quote from each side
copied verbatim (for a requirement or description, the quote from the staged `module.slim.json`
or `project.json`), the one-sentence contradiction, and the replacement text. The audit checks
that both quotes occur verbatim in the files they name and that every finding a pair or cell
cites exists here; a finding whose quotes are not in the corpus is rejected without discussion.
A finding that rests on one side only — a stale count, a dangling reference — quotes that side and
leaves the other quote empty.

**The file is the deliverable, not the report.** `FINDINGS.tsv`, `PAIRS.tsv`, `CELLS.tsv` and the
expansion calls are what the caller reads; the reply is a pointer to them. The reply carries: the
scope summary lines; the gate results; one line per finding — id, node, lens, and a ten-word
defect; the open questions, each with its candidate texts by finding id; the expansions and the
frontier; the per-lens counts. It does not repeat the quotes or the replacement texts, which are
in the file. When the caller presents the findings to the user it renders this table from the
file, one row per finding:

| # | Node | File | Lens/gate | Defect | Path | Bucket |
|---|------|------|-----------|--------|------|--------|

`Node` carries the full node name with the identity hash in parentheses — `Action Classifier
(72ab19c303f1)`. `Path` is how the walk reached the node from the seed — `seed`, or
`Store (5f09dc1a5d0c) → requires_module → beta → implements → QueryCommand` — so every finding
outside the seed carries its own reason for being in the report. `Bucket` is the Step 6 lifecycle
bucket (task-producing or absorbable), so the user sees what each fix will owe downstream.

Below the table, per finding: **the exact replacement text**, per leaf and per field — the
sentence or paragraph as it is and as it should be, or the field value as it is and as it should
be — never a description of the edit. The reviewer had the wording in front of it; whoever
applies the fix after the go does not, and a chain of coupled corrections must land as one set
without being re-derived from a summary. Where the correct resolution needs a decision the spec
does not record, state the open question as a question for the user to settle, with the two
candidate texts.

Findings that belong to one coupled correction — a claim and every neighbour that relies on it —
are grouped and numbered together, in walk order.

**Expansions.** Below the findings, one line per expansion call the walk made: the seed id, the
finding that caused it, and the rows it staged. The caller checks these against the transcript.

**Frontier.** When the walk stopped at the cap with nodes it did not enter, a section headed
`Frontier` follows the table: one line per node, with the edge and node that reached it and the
finding that pushed the walk there. It is mandatory whenever non-empty, and a report that carries
it is not a clean report whatever the table says.

**Then stop and wait for the user's explicit go** — fixing is a second, separately authorized act.
Partial approval is normal: fix what was approved and list the rest as declined-or-deferred in the
final report.

## Step 7b: Verification (the caller, before discussion)

Accepting a run has three mechanical parts, all before anyone reads a finding: the audit
(`audit-reads.py`, below under Step 8), the cost figures (`python3 skills/spec-review/cost.py
<transcript>` — calls, turns, final context, output, turns times context, and the cache split, which
is how a second reviewer's shared reading shows), and the verification here.

Findings are candidates until verified. The caller first cuts the passages —
`python3 skills/spec-review/passages.py "$STAGE"` writes `PASSAGES.tsv`, the block around each
quote with its neighbours and heading — then runs one fresh subagent with `PASSAGES.tsv` and
`FINDINGS.tsv`, using `skills/spec-review/verify.md` as its instructions: per finding it reads the
two passages and answers one question — does the contradiction exist as stated. It opens a staged
file only when a passage is not enough, and says so. It writes `$STAGE/VERDICTS.tsv`: `id`, `holds` / `does-not-hold` /
`mistake`, and one sentence of reason quoting what decides it. Only `holds` reaches the user; the
rest are listed at the end of the report with the verifier's reason, so a wrong rejection can be
appealed by quote rather than by re-review. A finding also holds without verification when two
independent runs produced it with the same quotes.

## Step 8: Fix in-session (after the go) — per-change mode only

Before the go, the caller audits the run:

```bash
python3 skills/spec-review/audit-reads.py <transcript> "$STAGE"
```

It lists every spec file the reviewer read, by path or by content, and exits 1 when any was
outside the stage, when a command globbed or looped over `spec/`, when a staged leaf was read
more than once or a read overflowed the harness limit, when a persisted tool-result file was read
back, when `PAIRS.tsv` has a row without a verdict, when a quote in `FINDINGS.tsv` or `CELLS.tsv`
is not verbatim in the staged file it names, or when a verdict cites a finding the file does not
carry. A run that fails on reading is discarded and rerun;
its findings are not discussed, because a reviewer that read outside the walk cannot say which of
its findings the walk would have produced. A run that fails only on cost — duplicates, batches —
keeps its findings and the failure is reported beside them.

- The fixes land on the current working branch: spec-review runs after `/spec` or `/drift`
  and rides that run's branch. A standalone audit whose approved findings warrant structural
  rework ends in a review proposal — hand the findings to `/propose`.
- Apply the approved corrections with `/spec` discipline: ids via `bin/spex hash-id`, the
  per-node check after each edited leaf.
- Re-run both Step 2 gates. The run ends with both green, or with every residual finding reported
  verbatim, with the command that produced it, named as another run's work.
- Report what changed, then remind the user to review and commit on the branch, landing via PR.

The run ends there.

## Step 9: No-findings exit

If Step 5 produced nothing actionable AND both Step 2 gates are clean AND the walk left no
frontier, print the coverage contract — a bare "clean" is not a valid exit:

```
spec-review: no actionable findings (N nodes audited across M modules, seed S nodes, hops H, frontier 0)
  lens 1 usage-strings: <pairs checked> (script exit 0)
  lens 2 counts: <claims judged>
  lens 3 flow-vs-arch: <claim pairs checked>
  lens 4 lexicon: <terms swept> (script exit 0)
  lens 5 surfaces-vs-constraints: <surface x constraint pairs>
  lens 6 intersections: <cells checked>
  lens 7 dissolved-modules: <modules swept> (script exit 0)
  lens 8 requirements-vs-leaves: <component x requirement pairs read>
  lens 9 test-shape: <scenarios judged> (script exit 0)
  lens 10 requirement-links: <requirements checked> (script exit 0)
  lens 11 undeclared-dependency: <leaves read>
```

Where N, M, S and H come from the scope script's summary line and the walk. A clean verdict asserts only "nothing found at these
counts and depth" — never "no defects exist". If a lens was inapplicable, say so with a reason. Exit there; the audit leaves every spec file as it found it.

## Out of scope

- Code reading. Findings come from `spec/` and the two gates only — the `.go` paths cited above are
  provenance for a rule, never a reading assignment — and no skill in this repo audits code-vs-spec
  alignment.
- Anything `spex validate` or `spex diff` already reports (Step 3). Surface it; never re-derive it.
