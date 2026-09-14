# Change Proposal: Skills as a release artifact

## Context

The authoring loop is five skills under `skills/`. They are what turns `spex` from a set of subcommands into a way of working: `/propose`, `/spec`, `/spec-review`, `/mint`, `/drift`. What a user installs is the binary. The documented path — download, verify, run the installer — delivers `spex` and nothing else; `skills/` is a directory in this repository, symlinked into `.claude/skills/` for the people who cloned it, and absent for everyone who did not. A released user has the structural half of the loop and none of the creative half.

`2026-08-20-harness-agnostic-loop` saw this gap and proposed embedding the loop's prose in the binary behind a `spex guide` api. It was declined because the remedy duplicated the skill set that `2026-09-14-authoring-commands` reduces to short loops over commands, and because a prose guide inside the binary would freeze the structure that proposal removes. What survives of it is the gap itself, and the simplest reading of it: if the skills are the loop, ship the skills.

Two facts shape the shape of that. The skill format is a published one, `SKILL.md` per skill directory, and it is read by more than one harness; a converter is speculation until a harness proves incompatible, so this proposal ships the set as it is. And the binary already carries one artifact whose identity with a repository file is enforced by a build-failing test: [[d051819c3224|the embedded installer]] is byte-for-byte the released `install.sh`, asserted by [[5b33ea62c4e3|a test that fails the build on drift]]. The same mechanism carries a directory as well as it carries a file.

This proposal cannot be scheduled before its input exists. The skill set worth shipping is the one `2026-09-14-authoring-commands` produces — short, ontology-free, with no bash lens names once `2026-08-20-spex-check` lands and no tracker script names once `2026-08-20-adapter-bindings` lands — and it should have been run through one real adoption first. The draft is written now so the decision is recorded; its impact expectation is stated as far as it can be known and is amended when the set exists.

## Proposed change

### The skill set is embedded and versioned with the binary

The `delivery` module gains `SkillBundle`: the contents of `skills/` embedded in the binary, with a build-failing test asserting that the embedded copy is byte-identical to the repository's `skills/` directory — file set, names and bytes — exactly as the installer is asserted today. The binary can then never ship a loop that drifted from the one under review, and `spex version` identifies which loop a user is running, because the loop has no version of its own: it is the binary's.

Embedding rather than adding a second archive member is deliberate. An archive member reaches a first-time installer and misses everyone who arrives through `spex upgrade`; the embedded copy travels with every path the binary takes.

### One command writes them out

A new api `spex skills install`, provided by a new `SkillsCommand` in the `cli` module, in the way [[766b86a25cc2|`spex upgrade`]] is the cli front-end for the delivery module's self-update. It writes the bundled skill directories into a directory the caller names with `--to`. The flag is required: the conventional destinations differ per harness, and a default that names one would be policy in the binary; the documentation shows the common ones.

Behaviour that can be checked: it refuses to overwrite a file whose bytes differ from the bundle unless `--force` is given, and names each such file, so a locally edited skill is never silently replaced; a second run into the same directory writes nothing and says so; it prints the files it wrote as data, and exits non-zero having written nothing when any refusal occurs. It reads and writes only under `--to`; it touches neither `spec/` nor `.spex/` and needs no initialised project, since installing the loop precedes using it.

### What does not change

The skills remain source in `skills/`, edited and reviewed there; the bundle is derived from them at build time. `spex upgrade` is unchanged and needs no knowledge of the skills: upgrading the binary upgrades the bundle, and a user re-runs `spex skills install` to refresh a project's copy, which is the same refusal-and-force contract as a first install. No converter for any harness ships; the format is the published one.

## Impact expectation

### New nodes

| Module | Node | Type | Task |
|---|---|---|---|
| delivery | Ship the authoring skills with the binary | module requirement | no |
| delivery | `SkillBundle` | component | yes |
| delivery | Skill bundle tests — describes `SkillBundle` | test section | no (one component) |
| cli | Skills install command | module requirement | no |
| cli | `SkillsCommand` | component | yes |
| cli | `spex skills install` | api | no |
| cli | Skills Command Tests — describes `SkillsCommand` | test section | no (one component) |

Both new requirements derive from [[5b50178bf72e|Installable and self-updating]], the project requirement the installer and `spex upgrade` already derive from. `SkillBundle` implements the delivery requirement and is described by its own test section; `SkillsCommand` implements the cli requirement, is described by its own test section, and takes a `uses` edge on `RootCommand` as every cli subcommand does.

### Modified nodes

| Module | Node | Hash | Task | Why |
|---|---|---|---|---|
| cli | `RootCommand` | `b6758cdfabc4` | yes | the bounded-surface enumeration gains a constructor |
| cli | Root Command Tests | `476f594a2f5f` | no | one component |

### Modified requirements, and the leaves they oblige

| Module | Requirement | Hash | Obliges |
|---|---|---|---|
| cli | Bounded third-party CLI surface | `293b27f73924` | RootCommand — it counts the constructors |

`delivery` and `cli` both have `module.json` changes; each also adds or changes a requirement in the same module, so the meta rule is suppressed in both.

### The checks that can actually fail

The byte-identity test is the one that matters: a change to any file under `skills/` without the bundle following fails the build, as the installer's does today. Then: installing into an empty directory yields a tree identical to `skills/`; a second run writes nothing; one locally edited file makes the run refuse, name that file and write nothing else; `--force` replaces it; and the installed set under a fresh checkout of a project runs the loop end to end against that project, which is the property this artifact exists for and the one that cannot be asserted until the skill set exists.

### Scope

Three tasks under one epic: three component leaves — two new, one modified. The constructor count in `293b27f73924` and the line and file counts here are to be re-read when this is scheduled, after `2026-09-14-authoring-commands`, `2026-08-20-spex-check` and `2026-08-20-adapter-bindings` have landed and one adoption has run the set.
