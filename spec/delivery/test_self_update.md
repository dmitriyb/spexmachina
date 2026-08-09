# Self-Update Tests

Acceptance coverage for [[a1e437df5c01|SelfUpdate]]: the embedded installer's upgrade mode, driven end to end by a fake-origin harness under the ordinary Go test gate — regression coverage, not a one-time proof. The harness runs the real script (a copy with only the signing public key swapped for a throwaway test key, origins redirected through the script's test-only origin variables) against a local server shaped like the release endpoints.

## Setup

- A local HTTP server serving a fabricated newer release: archive, SSHSIG signature by the throwaway key, checksums.
- An installed older binary at a writable target path, stamped with a comparable version.

## Cases

- Golden upgrade: resolve latest, download, verify, staged atomic replace; the previous binary remains as the backup file next to the target, and the target now reports the new version.
- Rollback restores the backup with an atomic rename and exits.
- A tampered archive (signature does not verify) fails closed: non-zero exit, target and backup untouched, no staged residue left in the target directory.
- Dry-run and already-up-to-date change nothing on disk.
- Forward-only guard: a resolved latest older than the installed version is hard-refused, non-overridably — a force flag is rejected as an unknown option and nothing moves.
- An explicitly pinned older version installs in any direction, printing the as-explicitly-requested notice.
- A `dev`/unstamped current version cannot be ordered: the latest path warns and proceeds (the artifact is still signature-verified).
- An unwritable target directory produces the re-run-elevated message and stops; the script never self-invokes sudo in upgrade mode.
- Embed identity: a build-failing test asserts the embedded script is byte-identical to the released copy at the repository root; introducing a one-byte divergence fails the build.
