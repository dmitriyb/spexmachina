# NodeMatcher

[[d165e2fe215e|Correlating changed spec nodes with the beads that already track them]] is a single equality test here: the merkle diff and the bead-map name a node with the same identity hash, so a match is a direct lookup and nothing else.

## Responsibilities

- Take classified changes (from merkle diff) and mapping records
- Match each changed spec node to the bead(s) that reference its identity hash
- Return **every** record that stores a matched node's identity hash, not just the first — the lookup yields a list and the whole list is carried forward. The mapping store refuses a second record for a `spec_node_id` unless that record's `node_type` is `proposal`, so for every other node type the list holds a single record; matching neither relies on that nor enforces it
- Identify unmatched changes (new spec nodes without beads)
- Identify unmatched mapping records (records referencing removed spec nodes)
- Skip structural changes — they do not participate in matching
- Order matched records and orphaned records alike by bead id: the orphan list is collected out of a lookup keyed by record, so the sort is what keeps [[755ded242c8a|the same diff over the same bead state]] returning the same three lists in the same order on every run

The records arriving here have already had each bead's live status joined onto them from [[bec96486c6b2|BeadReader]]'s output. Matching neither reads that status nor alters it; it is carried through untouched, for ActionClassifier's cleanup gate to consult further down.

## Interface

One call, over the classified changes and the mapping records, returning three lists and no failure. **Matched** pairs a change with every record that stores its identity hash. **Unmatched** holds an added or modified spec node no record refers to — nothing tracks it yet; a *removed* node no record refers to reaches none of the three lists, since there is no bead to obsolete and nothing left to build. **Orphaned** holds a record whose spec node the diff reports as removed, carried alongside the node type of that removed change, because an identity hash does not embed one and the classifier needs it downstream.

## Matching Logic

The key merkle puts on a changed leaf and the `spec_node_id` a mapping record stores are the same 12-character hex identity hash — the value the spec author wrote in the node's `id` field. A record matches a change when the two are equal, and that equality is the whole of the match: no path parsing, no module name resolution, no case manipulation, no key rebuilt out of parts. Earlier versions kept a `buildMerkleIndex` helper that converted bead-map records into a synthetic merkle key format like `module/3/component/2`. That helper is deleted: both sides already speak the same language.

A worked example: a component's `id` field is `abc123def456`. The merkle tree builds a leaf under that same key, the bead-map record for that component stores `spec_node_id: "abc123def456"`, and when the component's content file changes the diff reports the change under it. The record is found by asking for that one string. There is no intermediate format and nothing to translate.

### Structural changes are skipped

Changes with `impact: "structural"` (the synthetic `meta/project` and `meta/<module-hash>` leaves from the merkle tree) are not matched against bead-map records. They produce no matches, no unmatched entries, and no orphans. They are filtered out before any lookup runs.

Structural changes signal that the JSON envelope changed (a requirement was added, a module dependency was modified, etc.). Bead impact comes from leaf-level changes — components, impl_sections, test_sections, data_flows — which merkle detects independently as `arch_impl` or `impl_only` changes. When requirements change, affected components are expected to be updated too, and those component-level changes are the actual triggers for bead obsolete+create.

The consistency between structural and leaf changes is enforced upstream by `spex validate` (state checks) and `spex diff` (change completeness checks). By the time matching runs, structural consistency is already guaranteed.

## Why direct lookup matters

The previous architecture had two key formats — merkle keys (`module/N/component/M`) and bead-map `spec_node_id` values (`<module-name>/component/<id>`) — and a translation layer in between. The translation layer was the source of five pipeline bugs found during the first real run (PR #99): every conversion was a chance to drift the formats, and the apply tests used a mock store that skipped schema validation, so format mismatches passed CI. Identity hashes collapse the two formats into one. There is no rekeying, so there is nothing to drift.
