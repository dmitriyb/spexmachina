# DiffCommand

CLI entry point for [[d487fc9c4fa5|`spex diff`]]. It [[f223179a540a|compares the current merkle tree against the stored snapshot]], [[425146f32e96|classifies every change it finds by impact level]], and [[6f8284df92a2|checks that changed requirements brought their implementors' content along with them]]. It owns none of those three jobs: it parses the flags, wires the components that do own them into one pass, and turns what comes back into one report and one exit code.

## Responsibilities

- Run the lifecycle pre-flight ([[a9aa93774cc2|ProjectResolver]]) to obtain the project context — the resolved snapshot and journal locations — refusing an uninitialised or broken project with the pre-flight's own error and exit code instead of guessing; resolve the spec directory from the root command's `--spec-dir`, and parse its own flags: snapshot path (optional, overriding the resolved location), output format
- Resolve the profile once per invocation, through the schema module's resolution (`spec/profile.json` when present, the built-in default otherwise). A malformed profile is refused before any tree is built, with the resolution's own single early error and exit code 1 — the same pre-flight-style refusal path other setup failures take. The one resolved profile serves every profile consumer in the invocation: the classifier's type-to-level rules, the completeness checker's per-type triggers, and the removal-name sweep's name-declarable set
- Build the current tree with TreeBuilder, and ask [[b2fcd9457a28|SnapshotStore]] for the previous one
- Hand both trees to [[cb262b280963|DiffEngine]] and take back the list of changed leaves
- Read the module identity hash → module name map off the tree TreeBuilder just built; without it the report would name each module by its identity hash instead
- Hand that list, that map and the resolved profile to [[f1a672216ce9|ImpactClassifier]] and take back the same list with a level on each entry (`impl_only`, `contract`, `arch_impl`, `structural` — or `unknown`, when the profile does not declare a change's node type)
- Hand the classified list to [[de3309dfbd3c|CompletenessChecker]] and collect the errors it reports
- Run the removal-name sweep over the same classified list, and append what it finds to that error list, its disclosures to a separate notes list
- Emit changes, errors and any notes together as one report, and let the errors decide the exit code

The fourth impact level `contract` appears on `data_flow` and `api` leaf changes. DiffCommand passes classified changes through unchanged; the downstream consumer (the plan module) is responsible for acting on the level. Anything asserting impact strings must accept the full enum.

## Interface

```
spex diff [--snapshot path] [--json]
```

The spec directory comes from the root command's persistent `--spec-dir`, not from a positional argument. `--snapshot` overrides where the previous snapshot is read from; left off, it is the location the pre-flight resolved. The flag does not bypass the pre-flight: it replaces the baseline file within a resolved project, and an uninitialised or broken directory is refused before the flag is consulted — a custom baseline is a comparison choice, not an exemption from being a project. There is no stat-and-fall-back path: the snapshot is always loaded, and its absence surfaced as the pre-flight's uninitialised-or-broken refusal before any tree is built — the everything-added bootstrap output belongs exclusively to a project whose snapshot is the empty tree `spex init` seeded, never to a directory that merely lacks the file.

The task journal, at its resolved location, is read and never written, and an *empty* journal is not an error — `spex diff` runs in projects that have never been ingested, and `spex init` seeds the file empty, so a never-ingested project simply contributes no names. A missing or malformed journal, by contrast, never reaches this code: the pre-flight refuses either as a broken project, naming `spex doctor`, before any tree is built — that class is ProjectResolver's contract, not this command's. The journal is the second identity-hash-to-name source for the removal checks: when a whole module is retired, the sweep first tries to recover the name from the spec corpus itself, and the name a journal event still carries is what answers when that recovery comes up empty. When both sources come up empty the note degrades to `unverifiable` rather than failing the run — the journal can strengthen detection but never block a gate. The corpus sweep skips dot-prefixed files, which is load-bearing here: the journal's own `removed` events carry exactly the names the sweep hunts, and must never count as survivors. The retired `--map` flag is gone with the file it pointed at. `--json` selects the machine-readable report; without it the same content is printed as text.

## Output

JSON output includes both changes and errors. The `path` and `related` fields carry identity hashes (the same values used as merkle keys and journal node keys):

```json
{
  "changes": [...],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "requirement 'Match changed nodes to tasks' description changed but component NodeMatcher content leaf unchanged",
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
corpus. The sweep iterates the node types the resolved profile marks
name-declarable — the same per-type role flag the validator's name-shape check
keys on — rather than a fixed pair, so a flagged profile-declared type's
removals are swept on the same terms as the built-in ones; the name
tokenization rule being a fixed point is what keeps every flagged type's names
recoverable. The default profile marks exactly components and apis, so the
swept set is unchanged. The downstream pipeline step (`spex plan`) treats a non-empty
`errors` array as a halt signal — the pipeline does
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
- not a spex project — the pre-flight's own stable exit code, distinct
  from both codes above, when the directory was never initialised.
- `2` — diff completed but the `errors` array is non-empty. The full diff
  (changes + errors) is still emitted on stdout so the caller can read it,
  but the non-zero exit signals "do not pipe this into `spex plan`."

A run with a non-empty `errors` array MUST exit non-zero. The bare-output
"changes found" case still exits 0 — only errors gate the exit code, not
the presence of changes.
