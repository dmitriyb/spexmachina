# ValidateCommand

CLI entry point for [[528f91e5fb7d|`spex validate`]]. Orchestrates the checks that run against a spec directory and turns their combined result into one report and one exit status.

## Responsibilities

- Take the spec directory from the root command's persistent `--spec-dir` flag (short `-s`, default `spec/`) and resolve it to an absolute path
- Hand that path to each check and nothing else; every check loads `project.json` and the `module.json` files it needs for itself
- Run ten checks in a fixed order, appending each one's entries to a single list
- Run all ten whatever the earlier ones found — no check short-circuits the sequence, so one run produces the full report rather than one failure at a time
- Aggregate the entries through [[0f98ca780873|ErrorReporter]], which sorts by path and writes the report to stdout
- Read the exit status off the report ErrorReporter returns rather than re-inspecting the list: 0 if valid, 1 otherwise. That pairing of a machine-readable report with a branchable status is what [[608f8ca2e1b0|structured error output]] asks for

## Check Order

The order is fixed in the command and is the same on every run:

1. `schema` — [[651d5315eebf|SchemaChecker]], JSON Schema conformance of `project.json` and each `module.json`
2. `content` — [[5dcca0dab9bd|ContentResolver]], every declared `content` path resolves to a file
3. `link` — every typed cross-node link written in a content leaf resolves to a spec node
4. `id` — [[00beeeda5ddd|IDValidator]], identity-hash uniqueness, cross-reference targets, `preq_id`, `priority`
5. `id_derivation` — a module-scoped id equals the identity hash of its own module, node type and name
6. `dag` — [[c6c770a59d68|DAGChecker]], the module, requirement and component dependency graphs are acyclic
7. `name_consistency` — [[88dd4060cb44|NameConsistencyChecker]], `project.json` and `module.json` agree on a module's name
8. `test_coverage` — [[ed7a40b68995|TestCoverageChecker]], every component is described by a `test_section`
9. `requirement_coverage` — [[c7d0282b0e05|RequirementCoverageChecker]], every requirement is derived and implemented
10. `coupled_section` — [[e36112523589|CoupledSectionChecker]], the `sections` array and each coupled module's section schema

## Interface

```
spex validate
```

- The spec directory arrives through the root command's persistent `--spec-dir` flag; the subcommand declares no positional argument and no flag of its own
- Output is always structured JSON on stdout; pretty-printed when stdout is a TTY, compact otherwise

## Cost

The sequence runs in one process, and each check loads the spec for itself rather than
sharing a parsed graph with the others, so `project.json` and the `module.json` files are
re-read for each check — twice over for the `link` check, which loads the spec to find the
links and then builds the merkle tree their targets resolve against off a second reading of
the same files.
[[b42c5cdf874b|Fast validation]] bounds the run as a whole rather than any one check: a spec
of 100 modules and 1000 nodes finishes in under a second.
