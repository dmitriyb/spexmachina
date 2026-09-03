# Command Dispatch

How user input flows from argv through cobra to the correct subcommand handler.

## Flow

Dispatch is one pass with no branching until a subcommand has been chosen. Taking `spex hash-id --module plan --type component --name NodeMatcher` as the worked example:

1. The binary receives the argument list exactly as the operating system supplied it — the program name, then `hash-id`, then the six flag words.
2. [[b6758cdfabc4|RootCommand]] parses the persistent flags it owns and matches the first non-flag word against its registered children, selecting [[a338e50fec70|HashIDCommand]].
3. The selected subcommand parses its own local flags, reads whichever persistent flags it needs from the root, and runs.
4. Its payload goes to stdout, its diagnostics to stderr, and its outcome becomes the process exit code.

[[bbdb70e6f9f7|VersionCommand]] is the degenerate case: it declares no local flags, so step 3 is one write to stdout and nothing else.

## Key Behaviors

1. **No args**: `spex` with no arguments prints help text listing the top-level subcommands. A subcommand's own children are not in that list — `spex map`'s three and `spex completion`'s four appear under their parent's help instead.
2. **Unknown subcommand**: `spex versio` puts `unknown command "versio" for "spex"` on stderr and follows it with cobra's "Did you mean this?" line naming `version`. The suggestion is offered only for a near miss — `spex foobar` gets the error line and nothing else.
3. **Global flags**: `--spec-dir` is a persistent flag on the root, inherited by every subcommand. A subcommand reads it by name; it never re-declares it, so there is one default and one spelling of it in the binary.
4. **Subcommand help**: `spex validate --help` prints validate-specific usage and flags.
5. **Error handling**: a failing subcommand reports the failure upwards rather than exiting itself; the root turns that report into a message on stderr and a non-zero process exit.

## Completions

cobra auto-generates a `completion` subcommand. Users run:

```sh
spex completion bash > /etc/bash_completion.d/spex
spex completion zsh > "${fpath[1]}/_spex"
spex completion fish > ~/.config/fish/completions/spex.fish
spex completion powershell > spex.ps1
```

No custom code is needed — cobra generates completions from the registered command tree.

## Data Shapes

### argv → RootCommand

The input is the raw argument list. Parsing splits it into four things, and nothing downstream sees the raw list again:

| After parsing | What it carries |
|---|---|
| Persistent flags | the root's own flags — `--spec-dir` today — wherever on the line they appeared |
| Subcommand path | the words that selected a command: one word for a top-level subcommand, two when a subcommand's own child is selected, as with `spex map get` |
| Local flags | the selected subcommand's own flags |
| Positional arguments | whatever is left over |

### RootCommand → subcommand

The subcommand is handed its parsed local flags, the root's persistent flags and the leftover positional arguments. No spec state and no shared object crosses with them: the subcommand opens the spec directory itself if it needs one.

The process's standard input crosses too, and for two subcommands it is where the payload arrives rather than an incidental channel. `spex plan` reads the merkle diff there whenever `--diff` names no file, and `spex log` reads the task JSON there always — it declares no flag to read it from a file, and the root's own help line for it says so.

### Subcommand → stderr / stdout (exit contract)

- stdout: that subcommand's payload and nothing else — JSON, text, DOT, whichever it documents.
- stderr: error messages, plus progress output where a subcommand emits any.
- exit code: 0 when the subcommand succeeds. A subcommand may document exit codes of its own; where it documents none, failure is 1.

Any new persistent flag on RootCommand must be documented here. Any new
subcommand must be added the one way subcommands are added: its own
`cmd/spex/<name>.go` file, attached in the single registration call in
`cmd/spex/main.go`. There is no registrar type and no registration side
effect — that call is the whole registry, and it is exhaustive by
construction. A command constructed anywhere else is unreachable from the
binary; a command attached outside that call splits the list the help output
and shell completions are generated from.
