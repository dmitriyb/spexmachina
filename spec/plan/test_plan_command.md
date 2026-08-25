# Plan command tests

End-to-end tests for the `spex plan` CLI subcommand — full process invocation against fixture inputs. This single section joins the command-level coverage of the two retired seam commands: everything from diff intake to changeset output is now asserted against one invocation.

## Setup

Tests use a temporary directory containing:

1. **A spec tree** with `project.json` and two module directories (`validator/`, `merkle/`), each with `module.json` and content leaf files.

2. **A diff file** (or piped stdin) containing the merkle diff output — a JSON object with a `changes` array and an `errors` array, which is the document `spex diff --json` writes. A bare array is not a diff document and does not parse.

3. **A journal** at the fixture project's resolved location, seeding one `added` event plus `task_created` receipt per tracked node, and a `registered` event for the fixture proposal. It is the sole source of what ties a task to a spec node — the bead file below contributes status only. A malformed line is refused with `plan: read journal: map: journal line <n>: …` and exit 1.

4. **A bead file** — the tracker listing, written to disk by the caller and handed over with `--beads`. There is no mock CLI and nothing on PATH: the command starts no process, so a fixture file is the whole harness. Either the wrapped envelope (`{"issues": [...]}`) or a bare array is accepted. Variants: all-open, one-closed (drives the cleanup gate), one-in_progress (drives the refusal).

5. **An absorb file** for the absorption scenarios: a JSON list of `{node, reason}` entries.

## Scenarios

### S1: Full pipeline — diff file to changeset on stdout

```
spex plan --proposal <ref> --git-head deadbeef --diff diff.json --beads beads.json
```

Capture stdout. Parse the output as JSON. Assert `"version": 3`, `git_head` as given, the proposal ref, and an `ops` array whose creates precede closes, epic first — the canonical shape the builder tests pin in detail. Exit code 0.

### S2: Diff input from stdin (pipe), and `--diff -`

`cat diff.json | spex plan --proposal <ref> --git-head <sha> --beads beads.json`, and the same with `--diff -`. Assert output identical to S1 in both forms.

### S3: Pipeline composition — spex diff piped into spex plan

```
spex diff --snapshot snapshot.json --spec-dir ./spec | spex plan --proposal <ref> --git-head <sha> --beads beads.json
```

Assert the composed pipeline produces a valid changeset. This tests that `spex diff` stdout format is exactly what `spex plan` expects on stdin — no format adapter, and no intermediate document between the two.

### S4: Empty diff produces an epic-only or empty changeset

A diff with `{"changes": [], "errors": []}` exits 0. The changeset carries the proposal epic op when the fold pairs none, otherwise an empty op list. An empty diff is not an error condition.

### S5: Diff input containing errors refuses to proceed

A diff JSON with a non-empty `errors` array carrying **two** entries (an `incomplete_change` and a `surviving_name`, paths and related as identity hashes). Assert exit 1, stderr carries *every* error message and not merely the first, stdout is empty, and no matching, classification or composition ran. Two entries are what makes the assertion mean anything: a single-entry fixture is satisfied by a command that prints one and stops. This is the pipeline's one gate on the diff — there is no second command downstream to re-check it.

### S6: `--beads` drives the cleanup gate

Run S1 against the one-closed variant: the removed fixture node now yields the obsolete plus a create with reason `Code cleanup: <module>/<node>`. With `--beads` omitted, the same run exits 0, produces no cleanup create, and every modified-with-open-pairing node takes the obsolete+create path rather than a retarget — a pairing with no joined status is not known-open, and the gate defaults in the direction that never moves a task silently.

### S7: A claimed task refuses the run — exit 2

Run S1 against the one-in_progress variant, with that pairing's node modified in the diff. Assert:
- Exit code 2.
- stderr names every claimed task whose node changed (fixture includes two to prove the list is complete).
- stdout is empty; no changeset is written even with `--out`.

### S8: `--absorb` marks a node out of the op stream

Run S1 with an absorb file marking one modified node:
- The changeset's `ops` carry nothing for that node; the top-level `absorbed` array carries its entry (node, before/after hashes, reason).
- Marking a node the diff reports `added` or `removed`, or one absent from the diff, exits 2 naming the node; stdout stays empty.
- A marked node with an open pairing is absorbed without a retarget op.
- A marked node with an `in_progress` pairing is absorbed too — exit 0, no refusal: the mark withholds the change before classification, so the claimed-task refusal never sees it. Run the same fixture without the mark and assert exit 2, proving the mark is what makes the difference.

### S9: Missing required flags are errors

Without `--proposal`, and separately without `--git-head` (or with a malformed SHA), exit is non-zero and stderr names the flag. No partial output is written.

### S10: `--out` writes atomically

`--out changeset.json` writes the file and leaves stdout empty. The write is temp-file + rename: kill the process mid-write (or simulate the failure) and assert the target path holds either the previous run's changeset or nothing — never a splice.

### S11: Deterministic output across runs

Run S1 five times. All five stdout captures are byte-for-byte identical: same diff + same beads + same journal + same `--git-head` always produce the same changeset.

### S12: Exit codes

- Valid inputs → 0.
- Unreadable or malformed inputs (missing diff file, malformed JSON, unreadable `--beads`) → 1, stderr naming the input.
- Contract refusals (claimed task's node changed, invalid absorb entry, unresolvable dep, dep cycle) → 2, stderr naming the spec_node_ids or tasks implicated.
- No project state at the resolved location (no snapshot ever seeded) → the not-a-spex-project code, stderr naming `spex init`; a present but unloadable snapshot, or a malformed journal line, is a broken project → the same code, stderr naming `spex doctor` — never `1`, the pre-flight refuses before the fold reads anything.

### S13: The diff document itself is malformed or empty

The `malformed JSON` case S12 lists, aimed at the diff specifically — the one input that arrives on stdin and so has no filename to blame:

- A diff file holding `{"changes": [` → exit 1, stderr names the diff as the input that failed, stdout empty.
- A diff file holding a bare array rather than a document object (`[{…}]`) → exit 1: a bare array is not a diff document, per Setup.
- A **zero-length** diff file, and separately zero bytes piped on stdin → exit 1. Empty input is not an empty diff: `{"changes": [], "errors": []}` is the empty diff, exits 0, and S4 covers it. Nothing is written in any of these cases, `--out` included, per the Exit Codes contract that failure modes never write a partial changeset.

### S14: A removed node's tombstone participates in nothing

The journal's fold carries an entry for every removed node. Neither projection the run builds may include it:

- **Re-added under the same name.** Seed the journal with a node's `added` + `task_created`, then its `removed` + `task_closed`; the diff now reports that same identity hash as `added` again (a re-add carries the same hash, since the hash is a function of module, kind and name). Assert the changeset carries exactly one plain create for it — no close against the already-closed task, and no `old_bead_id`/`blocks` lineage dep to it. Matching never saw the tombstone, so the re-add is new work and nothing else.
- **A dead epic never parents.** Seed a `registered` event and its epic `task_created`, then a `removed` event whose entry would collide with the epic's key in the lookup. Assert the run still resolves its epic normally — the epic create is emitted (or the live epic task parents the ops, whichever the fold state calls for) and no op is parented at a closed task id. This is the PR #217 regression: a removed tombstone reaching the Resolver lookup makes every op in the run a child of a dead task, and the run still exits 0, so only an assertion on the parent refs catches it.

## Edge Cases

### E1: `--beads` names a file that does not exist

Exit 1, stderr carries `plan: read beads:` wrapping the underlying file error, stdout empty. An unreadable bead file is fatal, unlike an omitted one: a caller who asked for live status and did not get it is not silently given a changeset computed without it.

### E2: Bead file parses but names no bead the fold knows

Exit 0 — an empty join is not an error; the output matches the run with `--beads` omitted. A bead object with no `id` at all exits 1 naming the offending index.

### E3: Malformed absorb file

`{"broken":` exits 1 with a parse error naming the file; an absorb entry whose `node` is not a 12-hex string exits 2 naming the entry.

### E4: Concurrent invocations are safe

Two simultaneous runs over the same spec directory, journal and inputs both exit 0 with identical output — the command is read-only over everything but its own stdout/`--out`.

### E5: Large diff

500 changed nodes across 20 modules, 300 beads: completes in under 5 seconds, valid JSON, correct op counts, exit 0. The fixture's modules must declare `requires_module` edges and its components `uses` edges — a spec graph with none reduces dep collection to a walk over nothing, and the run would be timed doing the one thing the scenario exists to time. No leaf promises a complexity bound, so this is a smoke test on a realistic shape rather than a performance contract; it fails only when something has become pathological.
