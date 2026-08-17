# PlanCommand

[[ca0f477fdded|`spex plan`]] is entered here. One invocation covers what used to be a two-command seam: it reads a merkle diff, refuses it if `spex diff` flagged it as incomplete, enriches the journal fold's pairings with live bead status from a caller-supplied `--beads` file, maps changed spec nodes to actions, and composes `changeset.json` — there is no intermediate document and no second command between the diff and the changeset.

## Usage

```
spex plan --proposal <ref> --git-head <sha> [--diff <file>] [--beads <file>] [--absorb <file>] [--out <file>]
```

Flags:

| Flag | Required | Description |
|------|----------|-------------|
| `--proposal` | yes | Proposal ref (filename stem, e.g., `2026-04-18-decouple-spex-from-br`). Embedded in the changeset and used for the proposal epic's title. |
| `--git-head` | yes | Git HEAD SHA. Threaded into changeset.json as `git_head`; half of every eid the run derives, and required by the adapter's pre-flight as document provenance. |
| `--diff`     | no (stdin default) | Path to the diff document `spex diff --json` writes; `-` selects stdin explicitly. |
| `--beads`    | no | Tracker listing (the JSON of `br list --json` or a compatible shape), parsed by BeadReader. |
| `--absorb`   | no | Git-committed JSON list of `{node, reason}` entries marking cosmetic modifications to absorb. |
| `--out`      | no (stdout default) | Path to write changeset.json. If omitted, writes to stdout. |

The root's persistent `--spec-dir` is read too: it is what the spec graph is loaded from and where the task journal (`<spec-dir>/.history.jsonl`) lives.

## Declared surface

`spex plan` is this module's only external entry point, declared as an api node in `spec/plan/module.json` with `provided_by` naming PlanCommand. The declared name is the invocation string alone — the flags above are not part of it.

That boundary decides what the pipeline can see. Renaming the subcommand changes the api's identity hash and so reads as a removal plus an addition, and the removal-time name check then reports every place in the spec corpus that still says `spex plan`. Changing a flag — adding one, renaming one, changing what `--out` accepts — moves no name and no hash, and the diff cannot see it. Flag-level contract changes are therefore documented here, in the table above, and are caught by review rather than by the tool. The `--json` flag died with the machine report it rendered: it is not parsed, and supplying it is an unknown-flag error. The flags retired alongside it are recorded in the proposals that retired them, not carried here — this table is the live surface, and a leaf that also catalogued dead spellings would age into a list of things that are not true.

## Pre-flight

Before running the builder:

1. Require `--proposal`, and require `--git-head` to match `^[0-9a-f]{7,40}$`.
2. Read the diff from stdin or `--diff` and [[dbe693b25c8d|refuse it if its `errors` array is non-empty]]: print each entry to stderr and exit 1 without matching, classifying or composing anything. `spex diff` fills that array with `incomplete_change` entries (structural meta changes not accompanied by corresponding leaf-level changes) and `surviving_name` entries (a removed api or component still named in the corpus); leaving one unresolved means the spec edit is unfinished, and no bead actions may be derived from an unfinished edit. This is the pipeline's one gate on the diff — no downstream stage re-checks it, because no downstream stage exists between here and the adapter.
3. Parse and fold the task journal at `<spec-dir>/.history.jsonl` — an absent journal folds empty. From the same parsed events, resolve the run's registration: the `registered` event whose proposal is `--proposal`, or nothing if the journal holds none. One parse answers both — the fold lists task-bearing pairings, the registration is a lifecycle fact that has no pairing until the epic's task lands.
   The fold reaches the run as two projections, and **a removed node's entry — its tombstone — belongs to neither**. It is withheld from the pairings handed to [[972faea162a6|NodeMatcher]], because a node removed in an earlier epic and later re-added under the same name carries the same identity hash: matched against its own tombstone it would produce a close against an already-closed task and a successor create bearing an old bead id, where the correct answer is a plain create. It is withheld again from the lookup [[e9a3b1b85953|Resolver]] reads for epic and parent resolution, because a dead epic's tombstone would otherwise answer as that epic and parent every op in the run at a closed task. The epic-pairing entries the fold also carries are the mirror case — kept in the Resolver lookup, which keys on the proposal slug, and withheld from matching, which keys on a spec node hash they do not have. Neither projection is the fold verbatim; each drops what it cannot answer for.
4. Load the spec graph rooted at the spec directory's `project.json`.
5. If `--beads <file>` is given, hand its bytes to [[9f1578d7af6d|BeadReader]] and [[d027a902f414|join each parsed bead's live status onto its journal pairing by task id]]. When the flag is omitted, the fold is used as-is: the cleanup gate defaults closed and no pairing is known-open, so nothing is retargeted and nothing invents work from a missing input.
6. If `--absorb <file>` is given, parse it and [[049e8ae9cc51|validate every entry against the diff]]: each marked node must appear as `modified`, and a mark on an `added` or `removed` node, or on a node absent from the diff, is exit 2 naming the node. Each valid mark then removes its node's change from the stream handed to matching, and the command composes that node's absorbed entry — identity hash, before/after hashes off the diff entry, the authored reason. Absorption happens here, before any pairing or status is consulted, so a cosmetic edit never retargets, never refuses, and never recreates, whatever the state of the task tracking it; the builder only writes the finished entries.

## Wiring

PlanCommand assembles the run and then gets out of the way. The enriched pairings and the diff's changes — marked ones already withheld — go to [[972faea162a6|NodeMatcher]], which correlates them by direct identity-hash lookup; its three lists go to [[8aa1ab5ac102|ActionClassifier]], which decides the create, obsolete and retarget actions and collects `DepSpecNodeIDs` — or refuses the run, exit 2, naming every claimed (`in_progress`) task whose node changed; the actions, the fold, the registration, the spec graph, the composed absorbed entries, the `--git-head` SHA and the `--proposal` ref go to [[4c1146bb7287|ChangesetBuilder]], which owns everything from there.

The registration is resolved here rather than inside the builder for the same reason the fold is: the command is the one place permitted to know where the journal lives and how it parses, and everything downstream of it receives finished answers. Neither the builder nor its three subordinates opens `.history.jsonl`.

Resolver, TopologicalSorter and IdempotencyLabeler are reached only through the builder. PlanCommand neither builds nor calls them, which is why this module's `uses` graph runs command → builder → the three rather than command → all seven: there is exactly one place a change to the composition has to be made.

Once the builder answers, the command has one job left: serialize the changeset in canonical form and write it to the configured sink.

## Exit Codes

- `0` — success; changeset written.
- `1` — input validation error (bad flags, malformed JSON, diff carries errors, unreadable `--beads` or journal). Stderr names the flag or the input that failed.
- `2` — contract refusal (a claimed task's node changed, an invalid absorb entry, a dep cycle, an unresolvable dep or parent). Stderr carries the error with the spec_node_ids or task ids implicated.

Failure modes never write a partial changeset.

## Composability

- stdin input + stdout output makes `spex diff | spex plan ...` pipeline-friendly — the diff document flows straight in, with no stage between.
- `--out` + a specific path lets callers capture the changeset for git review before handing to the adapter.
- `--out` writes atomically: the changeset lands beside the target and is moved into place only once it is complete. A failure part-way through leaves no half-written file for the adapter to pick up, and the target path holds either the previous run's changeset or the new one, never a splice of the two.

## Non-Responsibilities

- Does not run the adapter.
- Does not append to the journal or save a snapshot — those belong to ingest, including the `modified` events for absorbed nodes, which ingest mints off the changeset's `absorbed` array.
- Does not invoke git — `--git-head` is caller-supplied.
- Does not judge whether an edit is cosmetic — the absorb file is an authored, reviewed input, and the command only checks its entries are structurally markable.

Those absences are one property: [[cf4f1ab8264a|the run is a pure function of the files and flags it is handed]]. It starts no subprocess, opens no connection and asks no tracker anything — the project prohibition's one sanctioned exception, the upgrade command's installer drive, never enters a plan run — so the same diff, tracker listing, task journal, spec directory, absorb list, proposal ref and SHA produce the same bytes on every machine and at every hour.

## Test surface

PlanCommand's CLI-level tests (flag validation, stdin/stdout wiring, exit
codes, diff-with-errors rejection, the claimed-task refusal, absorb-entry
validation) live in the `Plan command tests` test_section and ship with
this component's implementation bead. The four-component composition that
produces the changeset itself (ChangesetBuilder + Resolver +
TopologicalSorter + IdempotencyLabeler) is covered by the consolidated
`test_changeset_builder` test_section — PlanCommand wires that composition
but the cross-component integration assertions live with the builder
tests, not here — and the classifier/resolver retarget contract by
`test_classification`.
