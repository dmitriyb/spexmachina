# Change Proposal: Skills Integration with Pipeline

## Context

The project proposal states: *"Skills are rewritten to call `spex` subcommands instead of doing structural work themselves."* This integration doesn't exist yet. The skills (`/spec`, `/implement`) still operate independently of the pipeline. Additionally, the pipeline has a designed review step between impact and apply — where the LLM adds intelligence by interpreting patterns and curating actions — but no skill owns this step.

## Proposed change

### 1. `/spec` skill — validate after authoring

After writing spec files (JSON + markdown), the `/spec` skill calls `spex validate` to verify the spec is structurally valid before the user commits. This replaces the skill's internal validation logic with the authoritative validator.

### 2. New `/sync` skill — pipeline review step

Owns the review step between `spex impact` and `spex apply`. This is where the LLM adds value: interpreting raw impact data, detecting patterns (renames, tweaks, rewrites), and presenting curated actions for approval.

**Workflow:**
1. Calls `spex diff --json` and `spex impact` to get raw impact report
2. Reads `.bead-map.json` (from Proposal 1: Mapping Layer) for context on existing mappings
3. Interprets patterns intelligently (e.g. "these 5 removes + 5 adds look like a module rename")
4. Presents curated actions to user for approval
5. Calls `spex apply` with approved actions
6. Commits spec changes + `.snapshot.json` + `.bead-map.json`

### 3. `/implement` skill — deterministic preflight

Replaces the current label-parsing preflight logic with `spex check <bead-id>` (from Proposal 1: Mapping Layer). This makes preflight checks deterministic and decoupled from bead label format.

## Impact expectation

- **Skill changes only** — no new Go code beyond what Proposals 1 and 2 introduce
- **Modified skills**: `/spec` (add `spex validate` call), `/implement` (use `spex check`)
- **New skill**: `/sync` (pipeline review step)
- **Dependencies**: Proposal 1 (mapping layer for `/sync` and `/implement` preflight), Proposal 2 (test format for `/spec` test generation)
