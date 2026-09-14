# Change Proposal: Authoring commands

## Context

Every stage of the pipeline downstream of the spec is deterministic and owned by a component: `spex validate`, `spex diff`, `spex plan`, `spex ingest`, `spex register`. The stage upstream of all of them — writing the spec — is not. An agent authoring a change edits `project.json` and `module.json` by hand, picks the plural array from memory, invents the content path, derives the id with [[6c378662424f|`spex hash-id`]], writes the leaf with the headings the review lenses expect, and learns afterwards from `spex validate` what it got wrong. The project's overview says the LLM does the creative work and calls spex for everything mechanical. Authoring is the one place that division has not been applied.

The cost is visible in `skills/`. The five authoring skills total 1,570 lines; `/spec` alone is 586, and most of it is structural rules a command could hold instead: which array a type lives in, the `arch_` / `flow_` / `test_` filename convention — stated nowhere but that skill, enforced by nothing — the six-word name tokenization rule, how a rename is really a delete-plus-create with every inbound link rewritten, which completeness rule a one-line requirement edit triggers. Those rules are restated in prose because nothing refuses their violation at the moment it happens. A skill that has to pre-empt every structural mistake is a skill that cannot be short, and a skill that long is one harness's private discipline rather than a loop anyone can run.

Two facts make this the moment. First, the taxonomy is data now: since `2026-08-20-declarative-profile` and `2026-08-30-profile-v2` the resolved profile declares every node type, its plural key, scope, fields, reference kinds and targets, coverage chains, plan-relevance and absorbability. A command that reads the profile can place any node of any declared type without knowing the type's name. Second, two adopters are waiting on exactly this path. `faber` carries a spex-shaped spec that fails validation with 188 errors, all of them format age — 73 requirement entries still carry `title` where spec format version 1 says `name`, and 19 `impl_sections` entries name a leaf kind the profile no longer knows. `portitor` carries hand-written seeds with no `project.json` and no identity hashes. [[b1baa51bd7a9|The format-version contract]] already promises that a pre-versioning spec is "migrated by one mechanical rename" and that an out-of-range document gets a "migrate-before-using-this-spex signal"; nothing in the binary performs the migration the signal points at.

One gap is worth stating on its own. `ProfileLoader`'s leaf records that "the resolved profile has no dedicated command" — it is observable only through the node table `spex render` emits. A skill that must never name a node type has nothing to read the types from. The printer is the precondition for every generic skill, and it is missing.

This proposal is the first of the series remainder. `2026-08-20-spex-check` follows it and re-pins in Go the parity the commands here are tested against on the bash lenses; `2026-08-20-ingest-adopt-mode` depends on the migration and node commands here for an adopter to reach its mode at all.

## Proposed change

### A write path over three layers, owned by a new `author` module

A new module, `author`, owns every command that writes under `spec/`. It is the mirror of `validator`: where the validator reads the spec and reports, `author` changes the spec and refuses. The two share the checkers, so a refusal at write is the same predicate as a failure at validate.

The commands cover the three layers a spec has:

| Layer | Command | Does |
|---|---|---|
| project and module | `spex node add` | declares a node of a profile-declared type: places it under the type's plural key in `project.json` or the module's `module.json` by scope, derives the id, requires the profile's required fields, and for a content-bearing type sets `content` to the conventional path and scaffolds the leaf. `--type module` registers the module in `project.json` and creates its `module.json` skeleton |
| project and module | `spex node remove` | removes a node and reports every inbound reference that still names it, refusing while any remains unless `--force` is given, in which case the dangling references are listed so the operator can retarget them |
| project and module | `spex node rename` | performs the rename as one transaction: derives the new id, rewrites the node, every reference field naming the old id, every `[[old-id|…]]` link in every leaf, and moves the content file; prints the retired name so the vocabulary sweep gets it |
| module | `spex edge add`, `spex edge remove` | edits one reference field entry — `implements`, `uses`, `describes`, `preq_id`, `depends_on`, `provided_by`, `requires_module` — checking that the target exists, that the profile permits the source type to carry that field and that target type, and that `uses` and `requires_module` stay acyclic |
| leaf | `spex leaf scaffold` | writes a content leaf skeleton for an existing node: the title heading, the section headings the profile declares for that type, and one placeholder line per declared edge the leaf owes a link for; refuses to overwrite a non-empty leaf |
| profile | `spex profile show` | prints the resolved profile as JSON — the default or `spec/profile.json`, after validation — so a caller can read the types, fields, reference kinds, chains and conventions instead of memorising them |
| whole spec | `spex migrate` | rewrites a spec from an earlier format version to the current one and stamps `spec_version`: the `title`-to-`name` rename on every requirement, removal of arrays the profile does not declare, with each removed entry and its orphaned file reported by path so a person folds the content into an arch leaf; re-running on a current spec changes nothing |

Names are the ones declared: `spex node add`, `spex node remove`, `spex node rename`, `spex edge add`, `spex edge remove`, `spex leaf scaffold`, `spex profile show`, `spex migrate` — eight apis, each within the six-word rule. `spex node` and `spex edge` are groupings in the way `spex map` is; the children are the apis.

### Every command is a pure function of the resolved profile

No command names a node type, a plural key, a field or a file prefix. Each reads the resolved profile through `ProfileLoader` (`cd726f8b088b`) and refuses a type, field or target the profile does not declare, with the refusal naming what the profile does declare. That is what makes the commands, and any skill written over them, indifferent to the ontology: a project whose profile declares `endpoint` and `resource` gets the same commands with the same behaviour.

Two conventions the commands need are not in the profile today and move into it, as profile format version 2:

- **`content_prefix`** per content-bearing type — `arch_` for components, `flow_` for data flows, `test_` for test sections under the default profile. Today this convention exists only in `skills/spec/SKILL.md`; the validator resolves whatever path `content` names and enforces no prefix. Declaring it lets `spex node add` compute the path and lets a project choose its own.
- **`leaf_sections`** per content-bearing type — the ordered `##` headings a leaf of that type carries, empty when a type declares none. The default profile declares the headings the arch, flow and test leaves in this repository already share. `spex leaf scaffold` writes them, and `2026-08-20-spex-check`'s `HeadingChecker` gains a declared target to check against instead of only a baseline.

The profile document is decoded strictly — unknown top-level or per-type keys are rejected — so a version 1 binary cannot read a profile carrying these keys without a clear signal, and that is exactly the case the format-version contract exists for. The supported `profile_version` range becomes 1 to 2; a version 1 profile, or an absent file, yields the default conventions. The alternative — treating the two keys as optional extensions of version 1 — was rejected because a version 1 reader would fail on unknown fields with a decode error instead of the range message the contract promises.

### Write-time refusal is the validator's own predicate

Before writing, a command applies the change to an in-memory copy of the spec and runs the checkers the change can violate — schema conformance through `SchemaChecker` (`651d5315eebf`), id derivation and reference integrity through `IDValidator` (`00beeeda5ddd`), acyclicity through `DAGChecker` (`c6c770a59d68`), name declarability through the same tokenization the validator applies. A refusal is the validator's error, emitted before the file is touched, never a re-implementation of the rule. Where the validator would pass, the write proceeds; the commands add no rule the validator does not hold.

**Every refusal names its fix.** Each error carries the same `fix` field [[c318df455dcc|`spex doctor`]] already uses — the command, flag or value that resolves it: an undeclared type lists the declared ones; a missing required field names it and its kind; a target that does not exist says which array was searched; a name the tokenizer would not reproduce shows the declarable form. This is the property a short skill depends on. A skill stays long when the tool's errors are terse and the skill has to pre-empt them; with the fix in the error, the skill's loop is "run the command, apply the fix it names".

### The obligation a change creates is printed, not discovered

The completeness rules — a requirement description change obliges a changed leaf on every implementing component, a `module.json` change moves the module's `meta` leaf and obliges every component in the module unless a requirement in it also changed, a component edge change obliges its own leaf — are what make proposals larger than they look, and today they surface only when `spex diff` runs at the end. Every writing command ends by running `CompletenessChecker` (`de3309dfbd3c`) over the before-and-after pair it just produced and prints the leaves the change now obliges, as data under an `obligations` key. The agent sees the cost when it is incurred, and the impact expectation of a proposal can be read off the commands instead of computed by hand.

### Direct edits stay legal

The files are files. `spex validate` remains the trust boundary, and a hand edit, an external tool or a merge fix all remain possible and all meet the same check; nothing here makes the commands the only door. What changes is the skills: the authoring skills get one write path for JSON and leaf skeletons, and no fallback clause that re-imports the rules the commands hold. Prose inside a scaffolded leaf is the only thing the agent writes directly, because prose is the creative half.

### Determinism and formatting

Same spec plus same command plus same arguments produces the same files; a second run of an idempotent command — `migrate` on a current spec, `scaffold` on a leaf it already wrote, `edge add` of an edge that exists — changes nothing and says so. JSON is written in one canonical form: two-space indent, the profile's key order for node fields, arrays in declaration order. A hand-formatted file is reformatted on its first write. Identity hashes and merkle leaves are computed from field values, not bytes, so a reformat moves no hash and shows up in `spex diff` as nothing.

### What the commands do not touch

Nothing under `.spex/`. The authoring commands read and write `spec/` only and need no initialised project: `faber` must be migrated and validated before it is ever initialised, and the lifecycle requirement that a read command must not write is untouched because these commands write only what a person would otherwise write by hand. They call no subprocess and add no dependency to [[96c6c15ecc3e|the declared stack]].

### Work riding with the epic, outside the graph

`/spec` and `/propose` are rewritten as short loops over these commands for this repository: read the profile, declare, link, scaffold, write prose, validate, apply the fix the refusal names, repeat. The sections of `/spec` that restate array placement, id derivation, declarable names, edge rules and the file layout are deleted, since the commands refuse what those sections warned against. `docs/commands.md` documents the eight apis. The generalisation claim is stated as far as it is verified: the loops are ontology-free by construction, and both waiting adopters use the default profile, so behaviour under a custom profile is exercised by the golden tests over a second, test-only profile and by nothing else until a custom-profile adopter appears.

## Impact expectation

### New project requirement

| Node | Type | Priority |
|---|---|---|
| Author spec structure | functional | 1 |

A new project requirement, so `project.json`'s envelope leaf moves. Every module requirement below derives from it.

### New module — `author`

| Node | Type | Task |
|---|---|---|
| Add and remove nodes by profile | module requirement | no |
| Rename a node as one transaction | module requirement | no |
| Edit reference fields with graph checks | module requirement | no |
| Scaffold content leaves | module requirement | no |
| Show the resolved profile | module requirement | no |
| Migrate a spec to the current format | module requirement | no |
| Refusals name the fix | module requirement | no |
| Report obligations before writing | module requirement | no |
| `NodeEditor` | component | yes |
| `NodeRenamer` | component | yes |
| `EdgeEditor` | component | yes |
| `LeafScaffolder` | component | yes |
| `ProfileInspector` | component | yes |
| `Migrator` | component | yes |
| `ObligationReporter` | component | yes |
| `AuthorCommands` | component | yes |
| `spex node add`, `spex node remove`, `spex node rename`, `spex edge add`, `spex edge remove`, `spex leaf scaffold`, `spex profile show`, `spex migrate` | api ×8 | no |
| Authoring flow — proposal to validated spec through the commands | data flow | yes |
| Node editing tests — describes `NodeEditor`, `NodeRenamer`, `EdgeEditor` | test section | yes |
| Leaf and profile tests — describes `LeafScaffolder`, `ProfileInspector` | test section | yes |
| Migration tests — describes `Migrator` | test section | no (one component) |
| Obligation tests — describes `ObligationReporter` | test section | no (one component) |
| Author command tests — describes `AuthorCommands` | test section | no (one component) |

Each component implements one requirement; `AuthorCommands` implements none of the eight directly and is the module's command component in the way `ProposalCommands` (`21bc124a46c0`) is, with `uses` edges on the seven workers and `provided_by` on every api. "Refusals name the fix" and "Report obligations before writing" are implemented by `ObligationReporter`, which the six writing components use. The module `requires_module` `schema` (profile, identity hash), `validator` (the checkers) and `merkle` (`CompletenessChecker`); none of those depends on `author`, so the module graph stays acyclic. Coverage holds: every requirement has an implementing component and every component a describing test section.

### Modified nodes

| Module | Node | Hash | Task | Why |
|---|---|---|---|---|
| schema | `ProfileLoader` | `cd726f8b088b` | yes | validates `content_prefix` and `leaf_sections`; its leaf's "no dedicated command" sentence becomes false |
| schema | `SchemaLoader` | `ee88263d6555` | yes | the supported `profile_version` range becomes 1 to 2 |
| schema | Schema loading tests | `8719672c7580` | yes | describes both, among four components |
| cli | `RootCommand` | `b6758cdfabc4` | yes | the bounded-surface enumeration grows from fourteen constructors to twenty-two |
| cli | Root Command Tests | `476f594a2f5f` | no | one component |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| schema | Declare the node taxonomy | `5392cca550c4` | ProfileLoader — the profile gains two per-type declarations |
| schema | Declare the format version | `b1baa51bd7a9` | SchemaLoader — "exactly version 1" becomes a range |
| cli | Bounded third-party CLI surface | `293b27f73924` | RootCommand — it counts the constructors |

Every obligation is discharged by the table above. The `author` module is new, so it has no existing leaves for the meta rule to reach; `schema` and `cli` each change a requirement in the same module, so the meta rule is suppressed in both. `project.json` gains a requirement and a module entry, which moves the project envelope leaf and nothing else.

### The checks that can actually fail

The parity claim is against the rules the skill states today and the bash lenses enforce at review: for each rule the `/spec` skill's ID, declarable-name, edge and file-layout sections state, a fixture that violates it must be refused by the command with a `fix` naming the correction, and the same fixture written by hand must fail `spex validate` with the same error — that equivalence is the property, and a command that refuses what validate accepts, or accepts what validate refuses, fails it.

Then: a rename over a fixture with links in three leaves and references in two arrays leaves `spex validate` green and `spex diff` reporting one removed and one added node and nothing dangling; `spex migrate` over a copy of `faber`'s spec yields a document `spex validate` accepts except for the orphaned impl files it reported, and a second run is a no-op; `spex profile show` over the default profile round-trips through `ProfileLoader` unchanged; a version 1 profile still resolves and a version 3 one is refused with the range message; every writing command run twice on the same input leaves the tree byte-identical after the first run; and a golden test over a second, test-only profile declaring types that share no name with the defaults exercises every command once, which is the only evidence of ontology independence this proposal claims.

### Scope

Fifteen tasks under one epic: eleven component leaves — eight new, three modified — one data flow, and three test sections, being the two new ones that describe two or more components plus the modified schema loading tests. One project requirement and one module are added. Nothing is renamed and no node is removed.
