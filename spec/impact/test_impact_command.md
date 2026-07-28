# Impact Command Tests

Integration and acceptance tests for ImpactCommand (component 5). These tests verify the CLI entry point `spex impact`, which reads a merkle diff, wires all internal components (BeadReader, NodeMatcher, ActionClassifier, ReportGenerator), and outputs the impact report as JSON to stdout.

## Setup

Tests use a temporary directory containing:

1. **A spec tree** with `project.json` and two module directories (`validator/`, `merkle/`), each with `module.json` and content leaf files.

2. **A snapshot file** representing the previous state, so that `spex diff` can produce a meaningful diff. The snapshot is pre-computed with known hashes.

3. **A diff file** (or piped stdin) containing the merkle diff output — a JSON object with a `changes` array and an `errors` array, which is the document `spex diff --json` writes. A bare array is not a diff document and does not parse. The diff fixture's changes represent:
   - `validator/arch_schema_checker.md` modified (impact: arch_impl) — component SchemaChecker
   - `validator/arch_coupled_section_checker.md` added (impact: arch_impl) — component CoupledSectionChecker
   - `merkle/arch_hasher.md` modified (impact: arch_impl) — component Hasher
   - `merkle/arch_diff_engine.md` removed (impact: arch_impl) — component DiffEngine

4. **A mapping file** at the path `--map` names, defaulting to `.bead-map.json` beside the spec directory. It is what ties a bead to a spec node; the bead itself never carries an identity hash. The mapping store schema-validates the file before parsing it, so the fixture is a bead-map document — an object with `next_id` and `records` — and each record also carries `bead_type`, `content_file` and `spec_hash`. A bare array of records is refused with `impact: read mapping records: map: schema validation …` and exit 1. Three records:

```json
{
  "next_id": 11,
  "records": [
    {"id": 1,  "spec_node_id": "<SCHK_HASH>", "bead_id": "spex-001", "bead_type": "task", "module": "validator", "component": "SchemaChecker", "content_file": "spec/validator/arch_schema_checker.md", "spec_hash": "<SCHK_SPEC_SHA>"},
    {"id": 3,  "spec_node_id": "<HASR_HASH>", "bead_id": "spex-003", "bead_type": "task", "module": "merkle",    "component": "Hasher",        "content_file": "spec/merkle/arch_hasher.md",            "spec_hash": "<HASR_SPEC_SHA>"},
    {"id": 10, "spec_node_id": "<DIFF_HASH>", "bead_id": "spex-010", "bead_type": "task", "module": "merkle",    "component": "DiffEngine",    "content_file": "spec/merkle/arch_diff_engine.md",       "spec_hash": "<DIFF_SPEC_SHA>"}
  ]
}
```

5. **A bead file** — the tracker listing, written to disk by the caller and handed over with `--beads`. There is no mock CLI and nothing on PATH: the command starts no process, so a fixture file is the whole harness. Either the wrapped envelope or a bare array is accepted:

```json
{
  "issues": [
    {"id": "spex-001", "status": "open",   "labels": ["spex:1"]},
    {"id": "spex-003", "status": "open",   "labels": ["spex:3"]},
    {"id": "spex-010", "status": "open",   "labels": ["spex:10"]}
  ]
}
```

The `spex:<n>` label carries a mapping record id and nothing else. Each bead's `status` is joined onto the record whose own `id` is that integer, and a bead with no `spex:` label is dropped without comment. The cleanup scenario uses a variant of this file in which `spex-010` reads `"status": "closed"`.

## Scenarios

### S1: Full pipeline — diff file to JSON report on stdout

Run:
```
spex impact --diff diff.json --beads beads.json
```

Capture stdout. Parse the output as JSON. The report has exactly three top-level fields — `creates`, `obsoletes` and `summary`. There is no `closes` group and no `reviews` group; a spec change to a node that already has a bead obsoletes that bead and creates a fresh one. Assert:

- **creates**: 3 entries, in this order — `merkle/Hasher` (reason `Spec node modified (new): merkle/Hasher`, `old_bead_id` `spex-003`), `validator/CoupledSectionChecker` (reason `New spec node: validator/CoupledSectionChecker`, no `old_bead_id`), `validator/SchemaChecker` (reason `Spec node modified (new): validator/SchemaChecker`, `old_bead_id` `spex-001`)
- **obsoletes**: 3 entries, in this order — `spex-010` for `merkle/DiffEngine` (reason `Spec node removed: merkle/DiffEngine`, `change_type` `removed`), `spex-003` for `merkle/Hasher` and `spex-001` for `validator/SchemaChecker` (both reason `Spec node modified: <module>/<node>`, `change_type` `modified`)
- **summary**: `create_count: 3, obsolete_count: 3`

Both orderings are the sort the classifier applies to the whole action list before grouping — by action type, then module, then node name, then bead id — so they are fixed, not incidental.

`spex-010` is open in this fixture, so the removed node yields an obsolete and no cleanup create. Assert exit code is 0.

### S2: Diff input from stdin (pipe)

Run:
```
cat diff.json | spex impact --beads beads.json
```

Assert the output is identical to S1. The command must accept diff input on stdin when `--diff` is not specified.

### S3: Diff input from stdin with --diff flag set to "-"

Run:
```
cat diff.json | spex impact --diff - --beads beads.json
```

Assert the output is identical to S1. The `-` convention for stdin must be supported.

### S4: No changes — empty diff produces empty report

Provide a diff document whose `changes` array is empty (`{"changes": [], "errors": []}`). Run:
```
spex impact --diff empty_diff.json --beads beads.json
```

Assert stdout contains a valid JSON report with empty arrays and zero counts. Assert exit code is 0. An empty diff is not an error condition.

### S5: --json flag (explicit JSON output)

Run:
```
spex impact --diff diff.json --beads beads.json --json
```

Assert the output is valid JSON. The `--json` flag should be the default behavior but must be accepted without error for explicitness and forward compatibility with potential non-JSON output formats.

### S6: Pipeline composition — spex diff piped into spex impact

This acceptance test validates the full pipeline integration:

```
spex diff --snapshot snapshot.json --spec-dir ./spec | spex impact --beads beads.json
```

Assert the composed pipeline produces a valid impact report. This tests that `spex diff` stdout format is exactly the format `spex impact` expects on stdin — no format adapter needed.

### S7: Exit code 0 on success, exit code 1 on error

Run `spex impact` with valid inputs. Assert exit code 0.

Run `spex impact --diff nonexistent_file.json --beads beads.json`. Assert exit code 1 and stderr contains an error message about the missing file.

### S8: `--beads` drives the cleanup gate

Run S1's command against the variant bead file in which `spex-010` reads `"status": "closed"`:

```
spex impact --diff diff.json --beads beads_closed.json
```

Assert the removed node now yields two actions rather than one: the obsolete of `spex-010`, and a create for `merkle/DiffEngine` with reason `Code cleanup: merkle/DiffEngine` carrying `old_bead_id` `spex-010`. Summary becomes `create_count: 4, obsolete_count: 3`. This is the one observable difference the live status makes, and it is the reason a caller supplies the file at all.

### S9: `--beads` omitted, and `--bead-cli` supplied, are both inert

Run `spex impact --diff diff.json` with no `--beads` flag, against the same mapping file. Assert:
- The command exits 0 and produces the S1 report — matching, classification and reporting all run without any bead file
- No cleanup create appears for `merkle/DiffEngine`, whatever the tracker says about `spex-010`: the fixture records carry no `bead_status` of their own and nothing joined one onto them, so the removed-node gate reads no closed bead. It defaults closed in the safe direction — a missing input never invents work

Then run the same command again with `--bead-cli br`, `--bead-cli bd` and `--bead-cli ./anything`. Assert the flag is accepted in every case and the output is byte-identical to the run without it. No process is started, nothing on PATH is consulted, and no `br list --json` is invoked — this binary never runs a tracker command. The flag survives only so that unmigrated pipelines still parse; it selects nothing.

### S10: Deterministic output across runs

Run `spex impact --diff diff.json --beads beads.json` five times. Capture stdout each time. Assert all five outputs are byte-for-byte identical. This validates the determinism requirement: same merkle diff + same bead state always produces the same impact report.

### S11: Report output is suitable for piping to spex emit

Run:
```
spex impact --diff diff.json --beads beads.json > report.json
```

Then verify `report.json` can be parsed as an `ImpactReport` struct and passed to `spex emit`. Specifically:
- The file contains only the JSON report (no log lines, no progress output mixed in)
- Diagnostic or error messages go to stderr, not stdout
- The JSON is terminated with a newline

### S12: Large diff with many changes

Generate a diff with 500 changed nodes across 20 modules. Provide a bead fixture with 300 beads. Run `spex impact` and assert:
- The command completes in under 5 seconds
- The report is valid JSON
- Summary counts match the expected values based on the generated data
- Exit code is 0

## Edge Cases

### E1: `--beads` names a file that does not exist

Run `spex impact --diff diff.json --beads nonexistent_file.json`. Assert:
- Exit code is 1
- stderr contains `"impact: read beads:"` error context, wrapping the underlying file error
- stdout is empty (no partial report)

An unreadable bead file is fatal, unlike an omitted one: a caller who asked for live status and did not get it is not silently given a report computed without it.

### E2: Bead file parses but names no spec-managed bead

Provide a `--beads` file that is a valid JSON array in which no object carries a `spex:` label. Assert:
- Exit code is 0 — an empty result is not an error
- Every bead in the file is dropped, no mapping record gains a status, and the report matches the S9 run with `--beads` omitted

Provide a second file in which one object has no `id` at all. Assert exit code 1 and an error naming the offending index — a bead with no id is malformed input, not an unmanaged bead to skip.

### E3: Bead file holds malformed JSON

Provide a `--beads` file containing `{"broken":`. Assert:
- Exit code is 1
- The error carries the `impact: read beads:` context and references JSON parsing
- stdout is empty

### E4: Diff file contains malformed JSON

Provide a diff file containing `[{"path": "foo"` (truncated). Assert:
- Exit code is 1
- Error message references diff parsing

### E5: Diff file with zero-length content

Provide a diff file that is completely empty (0 bytes, not `[]`). Assert:
- Exit code is 1 (empty file is not valid JSON)
- Error message indicates the diff input could not be parsed

### E6: Concurrent access safety

If `spex impact` is run twice simultaneously against the same spec directory, mapping file and bead file, both invocations must complete without corrupting each other's output. This is naturally satisfied because `spex impact` is read-only over all three — it writes nothing but its report on stdout, and it starts no process that could write anything else. Assert both processes exit 0 and produce identical reports.

### E7: Spec directory with no modules

Provide a diff referencing modules that do not exist in the spec tree. Assert that unmatched changes are reported (creates for added nodes) and no panics occur due to missing module.json files.

### E8: Bead file carries two beads claiming the same record

Provide a `--beads` file holding two beads with different ids but the same `spex:3` label, one open and one closed. Assert the command exits 0 rather than reporting a conflict, and that record 3 ends up with the status of the bead that appeared **last** in the file — the join keeps one status per record id and later entries overwrite earlier ones. Order is preserved from the input, so the outcome is deterministic, not arbitrary.

Also provide a bead carrying two `spex:` labels, `spex:3` and `spex:99`. Assert the first one that reads as a non-negative integer wins and the rest are ignored, and that a bead whose only `spex:` label has a non-numeric suffix is dropped from the result silently — exactly as a bead carrying no `spex:` label is.

### E9: Diff input contains errors — impact refuses to proceed

Provide a diff JSON with a non-empty `errors` array. The `path` and `related` fields are identity hashes — for a requirement-description complaint the `path` is the changed requirement's own hash, with the implementing component's hash in `related`. The `meta/<module-hash>` form belongs to the module-envelope complaint instead:

```json
{
  "changes": [
    {"path": "a1b2c3d4e5f6", "type": "modified", "impact": "arch_impl", "module": "impact", "node_type": "component"}
  ],
  "errors": [
    {
      "type": "incomplete_change",
      "message": "Requirement 'Match changed nodes to beads' (impact) description changed but implementing component NodeMatcher content leaf unchanged",
      "path": "0011223344aa",
      "related": ["a1b2c3d4e5f6"]
    }
  ]
}
```

Assert:
- Exit code is 1
- stderr contains the error message from the errors array
- stdout is empty (no report generated)
- NodeMatcher, ActionClassifier, and ReportGenerator are never invoked

### E10: Diff input contains empty errors array — impact proceeds normally

Provide a diff JSON with `"errors": []`. Assert impact processes normally — empty errors array is not a rejection condition.
