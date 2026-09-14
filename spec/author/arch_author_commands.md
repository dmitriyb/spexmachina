# AuthorCommands

The CLI entry points of the authoring surface, one command component in the way the proposal module's is: cobra reaches this component and nothing else in the module, its run functions read flags into plain values and hand them to the workers, and the exit codes are its.

## The eight surfaces

| Api | Worker |
|---|---|
| [[e4ce2bde9c43|`spex node add`]] | [[cb8ef2b70999|NodeEditor]] |
| [[d0f7520b4171|`spex node remove`]] | NodeEditor |
| [[1035b507e2d9|`spex node rename`]] | [[14b673e2a502|NodeRenamer]] |
| [[b0aa34fa5593|`spex edge add`]] | [[e26d5ac76610|EdgeEditor]] |
| [[b2f2dd8e0dfc|`spex edge remove`]] | EdgeEditor |
| [[eecc2fb02918|`spex leaf scaffold`]] | [[b1a81efbd240|LeafScaffolder]] |
| [[574e8085f3c9|`spex profile show`]] | [[62468f3bab3c|ProfileInspector]] |
| [[e205f67eb446|`spex migrate`]] | [[b9b80a158949|Migrator]] |

`spex node`, `spex edge`, `spex leaf` and `spex profile` are groupings: a bare one prints its grouping's help and exits 0, runs nothing and carries no contract, so none is declared as an api and the eight children are. That is the opposite of `spex map`, whose parent is a declared api; the difference is deliberate and local — a grouping here owns nothing a child does not, so a node for it would carry no identity a child does not already carry as its first two words. Five constructors reach the root's registration list — `node`, `edge`, `leaf`, `profile`, `migrate` — and eight api nodes are declared here, in the module that owns the entry points, which is what lets `provided_by` stay module-local.

The declared names are the invocation strings alone. Flags sit behind them, so a flag change moves no name and no hash; the api `description` is where a surface change is written down, because an api has no content file and nothing else in the spec records its surface.

Three flags are fixed by the spec: the root's persistent `--spec-dir`, honoured by every surface; `--type` on `spex node add`, taking a type the resolved profile declares or the frame value `module`; and `--force` on `spex node remove`, which performs a removal over inbound references and lists them as dangling. The rest of the vocabulary — how the name, the module, the declared field values, an edge's source, field and target, and a rename's new name are passed — is not decided here or in the proposal; it is the implementer's choice, and the authoring loop records it in each api's `description` once chosen, since that is the one field a surface is written down in.

## Exit codes and output

Three codes, in the documented shape [[91f6f338de19|Composable]] asks for and with the values `spex plan` documents for the same meanings:

- `0` — success; the report is written.
- `1` — input validation error: a missing or malformed flag, a spec directory holding no `project.json`, a malformed or out-of-range profile. Stderr names the input that failed.
- `2` — contract refusal: [[b9e7b96f6aa7|ObligationReporter]] refused the change. The error document, each entry with its `fix`, is on stdout.

There is no not-a-spex-project code here, because these commands run no pre-flight — an uninitialised project is their intended first use. A caller branches on the code, never on a message. Every non-zero run puts one error line on stderr with no usage block beside it, and a refusal puts its structured error document — the validator's entries, each with its `fix` — on stdout.

A successful writing command prints one write report on stdout: what was written, by file; the `obligations` the change incurred; and for a rename the `retired_name`. Compact when piped, pretty-printed on a terminal, exactly as the profile document `spex profile show` prints. The report is the only prose-free channel the authoring skills read, which is why every fact a skill needs — the fix, the obligation, the retired name — is a key in it rather than a sentence.

## Boundaries

Every surface honours the root's `--spec-dir` and reads and writes under it alone. None reads `.spex/`, none needs an initialised project, and none runs a subprocess. The workers see no CLI framework: cobra stops at command construction in `cmd/spex`, and this component is the last thing in the module that knows a flag by name.
