# DiffCommand

CLI entry point for [[d487fc9c4fa5|`spex diff`]]. It [[f223179a540a|compares the current merkle tree against the stored snapshot]], [[425146f32e96|classifies every change it finds by impact level]], and [[6f8284df92a2|checks that changed requirements brought their implementors' content along with them]]. It owns none of those three jobs: it parses the flags, wires the components that do own them into one pass, and turns what comes back into one report and one exit code.

## Responsibilities

- Resolve the spec directory from the root command's `--spec-dir`, and parse its own flags: snapshot path (optional, defaults to `<spec-dir>/.snapshot.json`), output format
- Build the current tree with TreeBuilder, and ask [[b2fcd9457a28|SnapshotStore]] for the previous one
- Hand both trees to [[cb262b280963|DiffEngine]] and take back the list of changed leaves
- Read the module identity hash → module name map off the tree TreeBuilder just built; without it the report would name each module by its identity hash instead
- Hand that list and that map to [[f1a672216ce9|ImpactClassifier]] and take back the same list with a level on each entry (`impl_only`, `contract`, `arch_impl`, `structural`)
- Hand the classified list to [[de3309dfbd3c|CompletenessChecker]] and collect the errors it reports
- Run the removal-name sweep over the same classified list, and append what it finds to that error list, its disclosures to a separate notes list
- Emit changes, errors and any notes together as one report, and let the errors decide the exit code

The fourth impact level `contract` appears on `data_flow` and `api` leaf changes. DiffCommand passes classified changes through unchanged; downstream consumers (impact module) are responsible for acting on the level. Anything asserting impact strings must accept the full enum.

## Interface

```
spex diff [--snapshot path] [--json]
```

The spec directory comes from the root command's persistent `--spec-dir`, not from a positional argument. `--snapshot` overrides where the previous snapshot is read from; left off, it is `<spec-dir>/.snapshot.json`.

The task journal at `<spec-dir>/.history.jsonl` is read and never written, and its absence is not an error — `spex diff` runs in trees that have never been ingested. It is the second identity-hash-to-name source for the removal checks: when a whole module is retired, the sweep first tries to recover the name from the spec corpus itself, and the name a journal event still carries is what answers when that recovery comes up empty. A malformed journal degrades this to `unverifiable` notes rather than failing the run — a deliberately gentler contract than the retired bead-map's hard error on a corrupt file, because the journal can strengthen detection but never block a gate. The corpus sweep skips dot-prefixed files, which is load-bearing here: the journal's own `removed` events carry exactly the names the sweep hunts, and must never count as survivors. The retired `--map` flag is gone with the file it pointed at. `--json` selects the machine-readable report; without it the same content is printed as text.

## Output

JSON output includes both changes and errors. The `path` and `related` fields carry identity hashes (the same values used as merkle keys and journal node keys):

```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "requirement 'Match changed nodes to beads' description changed but component NodeMatcher content leaf unchanged",
      "path": "7c5e2fa1b3d8",
      "related": ["a1b2c3d4e5f6"]
    }
  ],
  "summary": { ... }
}
```

Two gates fill that array, and what either finds is an error, not a warning.
Their findings live under the top-level `errors` array (never `warnings`) and
the per-entry `type` says which gate raised it: `incomplete_change` from
CompletenessChecker, and `surviving_name` from the removal-name sweep, raised
when a node the diff reports as removed is still named somewhere in the spec
corpus. Downstream pipeline steps (`spex impact`, `spex
emit`) treat a non-empty `errors` array as a halt signal — the pipeline does
not advance until errors clear.

A run may also carry a `notes` array alongside those three keys. Notes are
disclosures rather than violations — a removal that could not be checked, or
hits discarded because a live node covers them — and the key is left out
entirely when there are none, so a clean run emits exactly `changes`, `errors`
and `summary`. Notes never gate anything: `errors` is the only array the exit
code reads.

Text output prints changes first, then errors (if any) under an `error(s):`
heading with each line prefixed `error:`, then notes (if any) under a
`note(s):` heading with each line prefixed `note:`. Both the text and the JSON
representations call them errors, with no terminology drift between
formats.

## Exit codes

- `0` — diff completed (with or without changes), no errors found.
- `1` — IO/parse failure (missing project.json, corrupted snapshot,
  malformed JSON).
- `2` — diff completed but the `errors` array is non-empty. The full diff
  (changes + errors) is still emitted on stdout so the caller can read it,
  but the non-zero exit signals "do not pipe this into `spex impact`."

A run with a non-empty `errors` array MUST exit non-zero. The bare-output
"changes found" case still exits 0 — only errors gate the exit code, not
the presence of changes.
