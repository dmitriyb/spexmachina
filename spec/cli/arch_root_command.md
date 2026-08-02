# RootCommand

The top-level `spex` command.

## Responsibilities

RootCommand is what [[0d7b10a39eb0|Root command and subcommand registration]] asks for: the binary's name and help text, its global flags, and the point the top-level subcommands hang from.

- Present the binary as `spex`, with a one-line summary and longer usage text.
- Offer one entry point that hands back the configured root with **no** subcommands attached. Attaching them is the binary's job, not the root's.
- Set the global persistent flags listed under Global Flags below.
- Print help and exit 0 when invoked with no arguments, or with `--help`.
- Answer an unknown subcommand with an error naming the word that was not recognised, and add a "did you mean?" suggestion when that word is a near miss for a registered subcommand. Both come from cobra's built-in rather than being written here.
- Offer the built-in `completion` subcommand for bash, zsh, fish, and powershell shell completions.
- On failure, put the error message alone on stderr and never the usage block with it, so a failing pipeline's stderr carries one readable line rather than a screen of flags.

## Subcommand Registration Pattern

Every subcommand is defined in the `cmd/spex` package — one file per command (`cmd/spex/diff.go`, `cmd/spex/emit.go`, `cmd/spex/ingest.go`, and so on), each exporting nothing and providing an unexported constructor of its own. The worker packages hold no command definitions at all.

Wiring happens in `cmd/spex/main.go`, which builds the root and attaches all twelve constructors in a single registration call, in this order: `hash-id`, `diff`, `validate`, `impact`, `map`, `register`, `log`, `template`, `version`, `render`, `emit`, `ingest`.

The root command imports no worker package; `cmd/spex` imports both the root and the workers, and is the only place the two meet. Keeping the constructors unexported in a `main` package is what makes that boundary unforgeable — nothing outside the binary can reach a subcommand constructor, so no worker package can grow a CLI dependency by accident.

## Where the surface is declared

Each of those twelve invocations is declared as an api node, and it is declared in the module that owns the subcommand's entry-point component — not here. `spex diff` belongs to merkle, `spex validate` to validator, `spex map` and its three children to map, and so on; this module declares only the two whose entry points it owns, `spex hash-id` and `spex version`. That placement is what makes the graph agree with the wiring above: the registration list is flat, but the ownership is not. Declaring all fifteen here would attach every surface to the module whose components exist precisely to hold no worker logic, and `provided_by` — module-local by design — would have no component to point at for thirteen of them.

The api names are globally unique across every module.json, so two modules cannot both claim an invocation — the check that would otherwise be impossible to state, because every other uniqueness rule in the spec is scoped to a single array in a single file.

### The bare `spex` root is not declared

There is no api node for `spex` itself, and the omission is a decision rather than an oversight.

An api node names an invocation with a contract behind it. Bare `spex` has none: with no args it prints cobra's help and exits, and every operation the binary performs is reached through one of the fifteen declared surfaces. What the root does own — the persistent `--spec-dir` flag, the "did you mean?" suggestions, the `completion` subcommand — is either a flag or a cobra built-in, and a flag is never part of an api name, so a `spex` node would carry no identity that any of the fifteen does not already carry as its first word.

The second reason is that the name would be unremovable. The removal-time name check searches the spec corpus for a removed api's declared name, longest-match-first, discarding hits a longer live name already covers. `spex` is one token and it prefixes all fifteen live names, so the subtraction clears only the mentions that are part of a longer invocation. Every other mention survives it, and the corpus is full of them: "the `spex` CLI", "the spex binary", the project's own name. Retiring such a node would report all of those, every one of them correct prose that must stay. A name the gate can never clear is worse than no node at all, because it teaches readers to override the check.

## Global Flags

| Flag | Shorthand | Type | Default | Description |
|------|-----------|------|---------|-------------|
| `--spec-dir` | `-s` | string | `spec/` | Path to the spec directory |

Global flags are defined as persistent flags on the root command, so every subcommand accepts them, every subcommand sees the same default, and a caller may write either the long form or the shorthand on either side of the subcommand word. Since the per-child `--map`/`--map-file` flags were retired with the bead-map, `--spec-dir` is the single locator for every piece of pipeline state — the spec tree, `spec/.snapshot.json`, and the task journal `spec/.history.jsonl` all resolve from it and from nothing else.

## Dependency Boundary

cobra is the only third-party CLI framework in the binary. No second command library and no hand-rolled parser stands beside it, and every capability the CLI framework owes the user is taken from a cobra built-in rather than added alongside it:

| Capability | Source |
|------------|--------|
| Argument and flag parsing | cobra, via pflag |
| Help and usage text | cobra's generated help |

cobra is confined to exactly two packages — `cli` and `cmd/spex`. Under the Subcommand Registration Pattern above, no other package imports it. What [[293b27f73924|Bounded third-party CLI surface]] fixes is both *which* third-party module may be reached for and *how far* it reaches:

- `cli/root.go` imports cobra and nothing else — no standard library, no internal package.
- cobra stops at command construction, all of which happens under `cmd/spex`. The packages that do the work — validator, merkle, impact, emit, ingest, mapping, proposal, render, schema — import no CLI framework at all; a subcommand's run function reads flags into plain Go values before calling into them, and hands back nothing a CLI framework would recognise.
- pflag and mousetrap arrive transitively through cobra and are never imported directly.

A subcommand needing something cobra does not provide uses the standard library; anything else amends the project's `Declared stack` requirement first, which enumerates the permitted modules against which `go.mod`'s direct requires are read.

## Design Rationale

cobra is the de facto standard for Go CLIs (kubectl, docker, hugo). It provides declarative subcommand registration, auto-generated help, POSIX flag parsing via pflag, and shell completions with zero custom code. This is the one sanctioned exception to the standard-library-first rule of the declared stack — reimplementing this infrastructure would add complexity without value — and the Dependency Boundary above is what keeps the exception from widening.
