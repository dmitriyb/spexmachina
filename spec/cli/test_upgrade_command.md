# Upgrade Command Tests

Coverage for [[ab15d9e23c4a|UpgradeCommand]]: the thin front-end's flag translation and verdict handling. The update mechanics themselves (resolve, verify, replace, rollback) are covered by the delivery module's self-update tests; these cases assert only what the command adds on top.

## Setup

- A stub installer script capturing its argv and environment, substituted for the embedded script through the command's test seam.
- Command built with a stamped version, and once without (a `dev` build).

## Cases

- Bare `spex upgrade` invokes the installer in upgrade mode against the running binary's resolved path (symlinks evaluated), passing the stamped current version; no version pin is set in the environment.
- `--version vX.Y.Z` travels to the script via its version environment contract, not a new flag; the check and rollback flags are appended as given.
- `--check` and `--dry-run` bind the same field; on an older-than-installed latest the command reports the anomaly as a warning and exits 0.
- A forward-only refusal from the script propagates as the command's own non-zero exit.
- `--force` is rejected as an unknown flag by the command's parser; nothing is invoked.
- A `dev` build omits the current-version argument and lets the script probe the target.
- `spex upgrade` is registered on the root command and appears in top-level help.
