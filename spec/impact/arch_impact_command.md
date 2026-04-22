# ImpactCommand

CLI entry point for `spex impact`. Reads a merkle diff, validates it for errors, optionally enriches mapping records with live bead status from a caller-supplied `--beads` file, and maps changed spec nodes to affected beads.

## Responsibilities

- Parse CLI flags: diff input (stdin or file), `--beads` file (optional), bead CLI binary name (legacy; retained for backward compatibility during the decouple-spex-from-br transition)
- Validate diff input: if the diff JSON contains a non-empty `errors` array (from change completeness validation in `spex diff`), refuse to proceed — print errors to stderr and exit non-zero
- If `--beads <file>` is provided: read the file, pass its bytes to BeadReader to parse into `[]BeadSpec`, enrich each mapping record with its live `bead_status` before ActionClassifier runs. This makes the cleanup-bead gate fire correctly for removed spec nodes whose beads are already closed
- Wire NodeMatcher to correlate changed nodes with beads using direct identity-hash lookup
- Wire ActionClassifier to determine actions (create/obsolete) and collect `DepSpecNodeIDs`
- Wire ReportGenerator to output the impact report as JSON

## No rekeying

Earlier versions of this command kept a `buildMerkleIndex` helper that rewrote each bead-map record into a synthetic merkle key (`module/<id>/component/<id>`) before passing the records to `NodeMatcher`. That helper exists only because merkle and the bead-map used different key formats. Both formats are now identity hashes, so `buildMerkleIndex` is deleted. Records flow from the mapping store into `NodeMatcher` unchanged, and changes flow from the diff into `NodeMatcher` unchanged. The two streams join on identity hash equality.

## Diff Input Validation

The diff JSON may contain an `errors` array alongside `changes`. This array is populated by `spex diff` when structural meta changes are not accompanied by corresponding leaf-level changes (e.g., a requirement changed but no implementing component content leaf changed).

If `errors` is present and non-empty, ImpactCommand:
1. Prints each error message to stderr
2. Exits with code 1
3. Does NOT proceed to matching, classification, or report generation

This ensures no bead actions are created from an incomplete or inconsistent spec edit.

## `--beads` Input (new)

Impact accepts an optional `--beads <file>` flag carrying the JSON output of a tracker's `list --json` command. Typical usage:

```
br list --json > /tmp/beads.json
spex impact --diff diff.json --beads /tmp/beads.json
```

Or piped via a file descriptor:

```
spex impact --diff diff.json --beads <(br list --json)
```

When `--beads` is supplied:
- BeadReader parses the file into `[]BeadSpec` (pure function — no subprocess invocation inside impact).
- For each BeadSpec, its live `status` field is joined onto the corresponding mapping record by `spex:<record-id>` label.
- The enriched records flow into ActionClassifier; the cleanup-bead classification at `impact/action_classifier.go:114` (`if o.Record.BeadStatus == "closed"`) then fires correctly.

When `--beads` is omitted, ImpactCommand proceeds with mapping records as-is. The cleanup-bead gate defaults closed (no cleanup actions emitted) for safety — callers who want cleanup classification must supply `--beads`.

## Interface

```
spex impact [--diff file] [--beads file] [--bead-cli br] [--json]
```

The `--bead-cli` flag is retained for backward compatibility — older pipelines that haven't been migrated to the data-artifact flow still rely on it. Once the `--beads` input is universal, `--bead-cli` will be retired in a follow-up change.
