# NodeMatcher

[[a771b3c60d9d|Correlating changed spec nodes with the tasks that already track them]] is a single equality test here: the merkle diff and the journal fold name a node with the same identity hash, so a match is a direct lookup and nothing else — the fold supplies every pairing, and no tracker label is ever consulted.

## Responsibilities

- Take classified changes (from merkle diff) and the journal fold's pairings
- Match each changed spec node to the task(s) that reference its identity hash
- Return **every** pairing that stores a matched node's identity hash, not just the first — the lookup yields a list and the whole list is carried forward. The fold answers with one current pairing per node (latest task-bearing event wins, `task_created` and `task_retargeted` alike), so in practice the list holds a single entry; matching neither relies on that nor enforces it
- Identify unmatched changes (new spec nodes without tasks)
- Identify orphaned pairings (a pairing whose spec node the diff reports removed)
- Skip structural changes — they do not participate in matching
- Order matched pairings and orphaned pairings alike by task id: the orphan list is collected out of a lookup keyed by pairing, so the sort is what keeps [[5369b5ae363c|the same diff over the same task state]] returning the same three lists in the same order on every run

The pairings arriving here have already had each task's live status joined onto them from [[80afb22dab75|TaskReader]]'s output — or no status at all, when the task-state artifact does not list the task. Matching neither reads that status nor alters it, and it does not distinguish a pairing with a status from one without: both are carried through untouched, for ActionClassifier's two decisions to consult further down. A pairing whose task the artifact omits is still a pairing; its absence of status is the classifier's signal that the task is finished, not the matcher's cue to drop it.

## Interface

One call, over the classified changes and the fold's pairings, returning three lists and no failure. **Matched** pairs a change with every pairing that stores its identity hash. **Unmatched** holds an added or modified spec node no pairing refers to — nothing tracks it yet; a *removed* node no pairing refers to reaches none of the three lists, since there is no task to close and nothing left to build. **Orphaned** holds a pairing whose spec node the diff reports as removed, carried alongside the node type of that removed change, because an identity hash does not embed one and the classifier needs it downstream.

## Matching Logic

The key merkle puts on a changed leaf and the node key a journal pairing stores are the same 12-character hex identity hash — the value the spec author wrote in the node's `id` field. A pairing matches a change when the two are equal, and that equality is the whole of the match: no path parsing, no module name resolution, no case manipulation, no key rebuilt out of parts. Earlier versions kept a `buildMerkleIndex` helper that converted the retired map file's records into a synthetic merkle key format like `module/3/component/2`. That helper is deleted: both sides already speak the same language.

A worked example: a component's `id` field is `abc123def456`. The merkle tree builds a leaf under that same key, the journal pairs that same hash with the component's task, and when the component's content file changes the diff reports the change under it. The pairing is found by asking for that one string. There is no intermediate format and nothing to translate.

A retargeted task changes nothing here: the fold moves the pairing's sourcing event forward while the task id and the node key stay put, so the same lookup finds the same pairing — now carrying a newer `after` hash for the already-tracked cell downstream to compare against. A successor task changes nothing either: a node whose earlier task finished and was followed by a plain create has two `task_created` lines in its history and no `task_closed` between them, and the fold's latest-wins rule hands the matcher the successor.

### Structural changes are skipped

Changes with `impact: "structural"` (the synthetic `meta/project` and `meta/<module-hash>` leaves from the merkle tree) are not matched against journal pairings. They produce no matches, no unmatched entries, and no orphans. They are filtered out before any lookup runs.

Structural changes signal that the JSON envelope changed (a requirement was added, a module dependency was modified, etc.). Task impact comes from leaf-level changes — components, test_sections, data_flows — which merkle detects independently as `arch_impl` or `impl_only` changes. When requirements change, affected components are expected to be updated too, and those component-level changes are the actual triggers for task actions.

The consistency between structural and leaf changes is enforced upstream by `spex validate` (state checks) and `spex diff` (change completeness checks). By the time matching runs, structural consistency is already guaranteed.

## Why direct lookup matters

The previous architecture had two key formats — merkle keys (`module/N/component/M`) and the retired map file's `spec_node_id` values (`<module-name>/component/<id>`) — and a translation layer in between. The translation layer was the source of five pipeline bugs found during the first real run (PR #99): every conversion was a chance to drift the formats, and the apply tests used a mock store that skipped schema validation, so format mismatches passed CI. Identity hashes collapse the two formats into one. There is no rekeying, so there is nothing to drift.
