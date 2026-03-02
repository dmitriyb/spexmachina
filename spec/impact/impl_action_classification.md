# Action Classification Rules

## Decision Table

| Change Type | Has Matching Bead? | Action |
|-------------|-------------------|--------|
| added | no | create |
| added | yes | review (unexpected — bead existed before node) |
| modified | no | create (spec changed, no tracking bead) |
| modified | yes | review |
| removed | no | no action (nothing to close) |
| removed | yes | close |

## Impact Level Influence

The impact level from merkle classification affects the action metadata but not the action type itself:
- `impl_only` review → bead description notes implementation change
- `arch_impl` review → bead description notes architecture change
- `structural` review → bead description notes structural change

## Reason Generation

Each action includes a human-readable reason:
- create: "New spec node: {module}/{node_name}"
- close: "Spec node removed: {module}/{node_name}"
- review: "Spec node modified ({impact}): {module}/{node_name}"
