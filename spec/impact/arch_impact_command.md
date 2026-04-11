# ImpactCommand

CLI entry point for `spex impact`. Reads a merkle diff, validates it for errors, and maps changed spec nodes to affected beads.

## Responsibilities

- Parse CLI flags: diff input (stdin or file), bead CLI binary name
- Validate diff input: if the diff JSON contains a non-empty `errors` array (from change completeness validation in `spex diff`), refuse to proceed — print errors to stderr and exit non-zero
- Wire BeadReader to load existing bead metadata
- Wire NodeMatcher to correlate changed nodes with beads using direct identity-hash lookup
- Wire ActionClassifier to determine actions (create/obsolete)
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

## Interface

```
spex impact [--diff file] [--bead-cli br] [--json]
```
