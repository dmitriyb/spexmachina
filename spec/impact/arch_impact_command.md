# ImpactCommand

[[62b47fdb7f2d|`spex impact`]] is entered here. It reads a merkle diff, refuses it if `spex diff` flagged it as incomplete, optionally enriches the journal fold's pairings with live bead status from a caller-supplied `--beads` file, and maps changed spec nodes to affected beads.

## Responsibilities

- Parse CLI flags: diff input (stdin or file), `--beads` file (optional), bead CLI binary name (legacy; retained for backward compatibility during the decouple-spex-from-br transition)
- [[9199f27cb1e2|Refuse a diff that carries errors]]: if the diff JSON holds a non-empty `errors` array (from the change-completeness and removed-name checks in `spex diff`), print those errors to stderr and exit non-zero without doing any of the work below
- If `--beads <file>` is provided: read the file and hand its bytes to [[bec96486c6b2|BeadReader]], then join each parsed bead's live status onto its journal pairing before classification runs. This makes the cleanup-bead gate fire correctly for removed spec nodes whose beads are already closed
- Hand the changes and the pairings to [[06035e7f0c39|NodeMatcher]], which correlates them by direct identity-hash lookup
- Hand its three lists to [[76d72cbe00f3|ActionClassifier]], which decides the create and obsolete actions and collects `DepSpecNodeIDs`
- Hand the actions to [[60d4747021ec|ReportGenerator]], which is where [[cc987600950e|the report reaches stdout]] as JSON

## No rekeying

Earlier versions of this command kept a `buildMerkleIndex` helper that rewrote each bead-map record into a synthetic merkle key (`module/<id>/component/<id>`) before passing the records to `NodeMatcher`. That helper existed only because merkle and the retired bead-map used different key formats. Both formats are now identity hashes, so `buildMerkleIndex` is deleted. Pairings flow from the journal fold into `NodeMatcher` unchanged, and changes flow from the diff into `NodeMatcher` unchanged. The two streams join on identity hash equality.

## Diff Input Validation

The diff JSON may contain an `errors` array alongside `changes`. `spex diff` fills it with two kinds of entry: `incomplete_change`, when structural meta changes are not accompanied by corresponding leaf-level changes (e.g., a requirement changed but no implementing component content leaf changed), and `surviving_name`, when an api or component removed from the spec is still named somewhere in the spec corpus. Every entry carries a `type`, a `message` and a `path`. A `related` field is always present too, but it is not always a list of values: two `incomplete_change` complaints name no second node — an added requirement that no component implements at all, and a project requirement from which no module requirement derives — and both serialize `related` as null. Where it is populated on an `incomplete_change`, its entries and the `path` alike are identity hashes, the same values that key the merkle tree and the journal — except for the complaint that a module's meta leaf changed while one of its component content leaves did not, which is filed against the synthetic path `meta/<module-hash>`. On a `surviving_name` both are `<file>:<line>` sites relative to the spec directory instead: the first site as the `path`, all of them in `related`.

If `errors` is present and non-empty, ImpactCommand:
1. Prints each error message to stderr
2. Exits with code 1
3. Does NOT proceed to matching, classification, or report generation

This ensures no bead actions are created from an incomplete or inconsistent spec edit. The way out is to finish the spec edit — update the content leaves the errors name — and re-run `spex diff` until it reports none.

## `--beads` Input (new)

[[116eb5f9906a|Bead state arrives as a file, never as a subprocess]]: `--beads <file>` carries the JSON output of a tracker's `list --json` command. Typical usage:

```
br list --json > /tmp/beads.json
spex impact --diff diff.json --beads /tmp/beads.json
```

Or piped via a file descriptor:

```
spex impact --diff diff.json --beads <(br list --json)
```

When `--beads` is supplied:
- BeadReader parses the file into one entry per spec-managed bead — a pure parse, with nothing started or contacted from inside impact.
- Each parsed bead's live status is joined onto the matching pairing by task id — the fold's pairing carries the task id the receipt recorded, and no label is parsed.
- The enriched pairings flow into ActionClassifier, and the removed-node gate that asks whether a bead is closed then has a real answer to read.

When `--beads` is omitted, ImpactCommand proceeds with the fold as-is. The cleanup-bead gate defaults closed (no cleanup actions emitted) for safety — callers who want cleanup classification must supply `--beads`.

## Interface

```
spex impact [--diff file] [--beads file] [--bead-cli br] [--json]
```

The `--bead-cli` flag is retained for backward compatibility — older pipelines that haven't been migrated to the data-artifact flow still rely on it. Once the `--beads` input is universal, `--bead-cli` will be retired in a follow-up change.
