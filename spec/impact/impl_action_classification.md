# Action Classification Rules

## State Transition Table

| Change Type | Has Matching Bead? | Action |
|-------------|-------------------|--------|
| added | no | create |
| added | yes (unexpected) | obsolete old + create new |
| modified | no | create (spec changed, no tracking bead) |
| modified | yes | obsolete old + create new |
| removed | no | no action (nothing to obsolete) |
| removed | yes, open/in_progress | obsolete |
| removed | yes, closed | obsolete + create (cleanup) |

The "review" action from the previous model is eliminated entirely. Modified nodes always trigger obsolete+create — no in-place metadata patching.

## OldBeadID Propagation

When a modified or unexpectedly-matched-added node generates both an obsolete and a create action, the create action carries `OldBeadID` set to the obsoleted bead's ID. This enables `BeadCreator` to set `--deps blocks:<old-bead-id>` for lineage tracking.

## Reason Generation

Each action includes a human-readable reason:
- create (new): `"New spec node: {module}/{node_name}"`
- create (modified): `"Spec node modified (new): {module}/{node_name}"`
- obsolete (modified): `"Spec node modified: {module}/{node_name}"`
- obsolete (removed): `"Spec node removed: {module}/{node_name}"`
- create (cleanup): `"Code cleanup: {module}/{node_name}"`

## Cleanup Classification

When a spec node is removed and its bead is closed, the code has already shipped to main. This means there is code in the repository that no longer corresponds to any spec node — it needs to be deleted. The classifier generates an additional "create" action for a cleanup bead.

When the bead is open or in_progress, no code has shipped to main, so only the obsolete action is needed — there is nothing to clean up.
