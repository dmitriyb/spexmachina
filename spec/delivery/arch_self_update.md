# Self-Update

The install script's upgrade mode — the mechanism the cli upgrade command drives, satisfying [[d051819c3224|In-place self-update via the embedded installer]]. One script serves both surfaces: what a first-time user downloads, verifies, and runs is byte-for-byte what the binary embeds and drives against its own path. One implementation, one copy of the release-signing key, and nothing to fetch at upgrade time — the embedded script is already inside the trusted, signed binary.

## Modes

With no flags the script is first-install mode: resolve the latest release, download, verify, install into the install directory. Upgrade mode is opt-in and changes two things: the destination is the exact path of the currently-running binary (resolved by the Go side with symlinks evaluated), and replacement is staged rename, not a copy over the target. The resolve, download, and signature-verification logic is shared between the modes — upgrade adds decisions around it, never a second verifier. Fail-closed holds end to end: a failed download or a signature that does not verify exits non-zero having changed nothing on disk.

## Safe self-replace

Overwriting a running binary in place is unsafe; renaming over it is not — the running process keeps its inode, and the next invocation gets the new file. Upgrade mode stages the verified new binary in the target's own directory (same filesystem, so the final swap is one atomic rename), moves the running binary aside as a backup, and renames the new one into place. On any failure during the swap the backup is restored before exiting, so the on-disk binary is never missing or half-updated. The rollback flag restores the kept backup the same way. If the target directory is not writable, the script reports re-run-elevated and stops — upgrade mode never self-invokes sudo (first-install mode may; a silent privileged self-replace is a surprise).

## Forward-only guard

A signature proves authenticity, not freshness: a compromised origin can serve a genuine-but-older signed release as "latest". Upgrade mode orders the resolved version against the current one and splits by how the target was named. On the latest path, an older resolved release is a rollback anomaly, hard-refused and non-overridable — there is no force flag, and a stray one is rejected as an unknown option. An explicitly pinned version has no untrusted "latest" in the loop, so it installs in any direction with an as-explicitly-requested notice. A `dev` or unstamped current version cannot be ordered, so the latest path warns and proceeds; the artifact is signature-verified either way. When the caller supplies no current version at all, the script probes the target binary for one — the fallback the compiled-in stamp exists to avoid. The check flag reports this comparison and changes nothing — reporting, not gating, so it exits 0 even on the anomaly.

## Embedded equals released

[[5b33ea62c4e3|Embedded installer byte-identical to the released one]] is the security argument for skipping fetch-and-verify at upgrade time: the embedded copy is provably the same audited, signed installer a user runs by hand. The canonical script lives at the repository root and ships with each release; the embedded copy is kept identical by a build-failing test, so drift between the two cannot survive a build.

## Test-only origin hooks

So a fake-origin harness can drive the real script against a throwaway release, the script reads test-only origin variables defaulting to the production endpoints. These are not trust-sensitive: whatever origin serves the download, the archive must still verify against the embedded signing key. The trust anchor itself is deliberately not overridable — the harness bakes a throwaway key into its own copy of the script, exactly as a release bakes the real one.
