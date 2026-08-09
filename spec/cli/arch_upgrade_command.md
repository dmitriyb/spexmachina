# Upgrade Command

The thin front-end for self-update, satisfying [[cb1775f0a576|In-place upgrade command]] and providing the [[766b86a25cc2|`spex upgrade`]] surface. It translates its flags into the embedded installer's upgrade-mode invocation and reimplements none of the resolve, download, verify, or replace logic — the mechanism is the delivery module's self-update component; this command decides what to ask of it.

Like every subcommand, it is registered on [[b6758cdfabc4|RootCommand]] and appears in top-level help.

## Flag surface

| Flag | Behavior |
|---|---|
| (none) | resolve the latest signed release and move to it, forward-only |
| `--version vX.Y.Z` | install that exact release, in any direction — the deliberate-older path |
| `--check` / `--dry-run` | report the comparison and change nothing; both bind the same field; exits 0 even when latest is older than installed (reported as an anomaly warning) |
| `--rollback` | restore the previous binary from its kept backup |

There is no `--force`: the forward-only refusal on the latest path is not overridable, and the deliberate-older path is `--version`. An unknown flag is rejected by the parser before anything is invoked.

## Translation contract

The command always passes upgrade mode and the target — the running binary's own path, resolved with symlinks evaluated — plus the current version sourced from the compiled-in version stamp, so the forward-only guard never depends on re-executing the binary about to be replaced (a `dev` build omits it and lets the script probe the target). Check and rollback travel as script flags as given; a pinned `--version` travels through the script's existing version environment contract rather than a new flag — unset is precisely what selects the script's forward-only latest path. The script's exit status is the command's exit status: a refusal propagates, a report exits 0.
