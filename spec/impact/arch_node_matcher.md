# NodeMatcher

Matches changed spec nodes from the merkle diff to existing beads by direct identity-hash lookup.

## Responsibilities

- Take classified changes (from merkle diff) and mapping records
- Match each changed spec node to the bead(s) that reference its identity hash
- Identify unmatched changes (new spec nodes without beads)
- Identify unmatched mapping records (records referencing removed spec nodes)
- Skip structural changes — they do not participate in matching

## Interface

```go
type Match struct {
    Change  ClassifiedChange
    Records []Record // mapping records linking this spec node to beads
}

type Unmatched struct {
    Change ClassifiedChange // new spec node, no mapping record
}

type Orphaned struct {
    Record Record // mapping record references removed spec node
}

func MatchNodes(changes []ClassifiedChange, records []Record) ([]Match, []Unmatched, []Orphaned)
```

## Matching Logic

The merkle diff key (`change.Key`) and the mapping record `SpecNodeID` are both identity hash strings — the same 12-character hex value the spec author wrote in the JSON `id` field. A record matches a change when the two strings are equal:

```go
if change.Key == record.SpecNodeID { /* match */ }
```

There is nothing else: no path parsing, no module name resolution, no case manipulation, no `fmt.Sprintf` to rebuild a key. Earlier versions kept a `buildMerkleIndex` helper that converted bead-map records into a synthetic merkle key format like `module/3/component/2`. That helper is deleted: both sides already speak the same language.

### Structural changes are skipped

Changes with `impact: "structural"` (the synthetic `meta/project` and `meta/<module-hash>` leaves from the merkle tree) are not matched against bead-map records. They produce no matches, no unmatched entries, and no orphans. MatchNodes filters them out before the matching loop.

Structural changes signal that the JSON envelope changed (a requirement was added, a module dependency was modified, etc.). Bead impact comes from leaf-level changes — components, impl_sections, test_sections, data_flows — which merkle detects independently as `arch_impl` or `impl_only` changes. When requirements change, affected components are expected to be updated too, and those component-level changes are the actual triggers for bead obsolete+create.

The consistency between structural and leaf changes is enforced upstream by `spex validate` (state checks) and `spex diff` (change completeness checks). By the time MatchNodes runs, structural consistency is already guaranteed.

## Why direct lookup matters

The previous architecture had two key formats — merkle keys (`module/N/component/M`) and bead-map `spec_node_id` values (`<module-name>/component/<id>`) — and a translation layer in between. The translation layer was the source of five pipeline bugs found during the first real run (PR #99): every conversion was a chance to drift the formats, and the apply tests used a mock store that skipped schema validation, so format mismatches passed CI. Identity hashes collapse the two formats into one. There is no rekeying, so there is nothing to drift.
