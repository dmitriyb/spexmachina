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

### 4. Skills section in spec

Skills are the LLM half of the architecture but have zero representation in the spec's JSON structure. Two-phase approach:

**Phase 1 — Skills catalog in project.json**:
Add a `skills` array to `project.json` listing each skill with name, purpose, file path, which `spex` subcommands it calls, and which spec modules it depends on. SKILL.md files remain authoritative. The catalog makes skills visible to the spec structure — the validator can check paths exist, the merkle tree detects project.json changes, and tools can discover what skills exist.

Schema change: `project.schema.json` gets a `skills` array with properties: `name`, `purpose`, `path`, `uses_commands`, `depends_on_modules`.

Validator: New SkillPathChecker verifies each skill's `path` exists and contains a SKILL.md file.

**Phase 2 — Skills as first-class spec concept**:
After implementing `/sync` and running the full pipeline, design a dedicated `skill.schema.json` that captures skill-specific structure: inputs/outputs, workflow steps, subcommand dependencies, triggers, preconditions. Each skill gets its own spec directory (e.g., `spec/skills/sync/`) with `skill.json` + markdown leaves, fully tracked by the merkle tree. Deferred because we don't yet know what the right schema fields are.

## Impact expectation

- **Modified**: `schema/project.schema.json` (add `skills` array)
- **Modified**: `spec/project.json` (add skills entries)
- **Modified skills**: `/spec` (add `spex validate` call, test section generation), `/implement` (use `spex check`)
- **New skill**: `/sync` (pipeline review step)
- **New validator**: SkillPathChecker (optional, lightweight)
- **Dependencies**: Proposal 1 (mapping layer for `/sync` and `/implement` preflight), Proposal 2 (test format for `/spec` test generation)
