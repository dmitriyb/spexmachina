# Change Proposal: Skills Integration with Pipeline

## Context

The project proposal states: *"Skills are rewritten to call `spex` subcommands instead of doing structural work themselves."* This integration doesn't exist yet. The skills (`/spec`, `/implement`) still operate independently of the pipeline. Additionally, the pipeline has a designed review step between impact and apply — where the LLM adds intelligence by interpreting patterns and curating actions — but no skill owns this step.

## Distribution model

All canonical SKILL.md definitions live in `skills/`. The `type` field in project.json distinguishes skill vs agent vs hybrid — the filesystem doesn't need separate directories.

**Discovery**: `.claude/skills/` contains symlinks to `../../skills/<name>` for Claude Code slash command discovery.

**Spex repo**: Symlinks are tracked in git — clone and go.

**Target projects**: `.claude/skills/` symlinks are gitignored. `spex init` scaffolds them by creating symlinks to the installed skill definitions (e.g. `~/.spex/skills/<name>`). Updates to skill definitions propagate automatically through symlinks — no copy/paste, no version drift.

```
skills/                          # ALL canonical definitions (tracked in git)
  propose/SKILL.md               # type: skill
  spec/SKILL.md                  # type: skill
  implement/SKILL.md             # type: agent
  review/SKILL.md                # type: agent
  fix/SKILL.md                   # type: agent
  (sync/SKILL.md)                # future (type: hybrid)

.claude/skills/                  # symlinks for Claude Code discovery
  propose -> ../../skills/propose
  spec -> ../../skills/spec
  implement -> ../../skills/implement
  review -> ../../skills/review
  fix -> ../../skills/fix
```

## Orchestration model

Skills and agents are the same SKILL.md files invoked as `/command` slash commands. The distinction is in how they are orchestrated:

**Interactive**: User invokes `/implement`, `/review`, `/fix` as slash commands in their Claude Code session. The signing key is available because the skill runs inline in the user's process. Creative skills (`/propose`, `/spec`) always run this way.

**Orchestrated**: Faber (the external orchestrator) runs each agent-type skill in a separate Docker container (`claude -p "/implement bd-42"`) for context isolation. The reviewer never sees the implementer's reasoning — only the artifacts (PR, code, spec). This prevents cross-contamination of LLM context between pipeline stages.

**Signing key constraint**: Skills run inline where the GPG signing key is available. Docker-based agent isolation means commits inside containers cannot be signed — faber handles signing at the orchestration layer if needed.

**Language skill injection**: Skills like `go-expert` and `zig-expert` are user-global (`~/.claude/skills/`). Future: detect the project's language from project config or CLAUDE.md and inject the appropriate language skill reference into agent containers automatically.

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

Skills are the LLM half of the architecture but have zero representation in the spec's JSON structure. Five skills exist as standalone SKILL.md files, completely outside the spec's JSON structure and merkle tracking. Two-phase approach — start lightweight, evolve to full spec integration.

**Phase 1 — Skills catalog in project.json**:
Add a `skills` array to `project.json` listing each skill with name, purpose, file path, which `spex` subcommands it calls, and which spec modules it depends on. SKILL.md files remain the authoritative definition of skill behavior. The `skills` array is a machine-readable catalog that makes skills visible to the spec structure — the validator can check paths exist, the merkle tree detects project.json changes, and other tools can discover what skills exist and what they depend on.

Skills entries for project.json:

```json
"skills": [
  {
    "name": "propose",
    "type": "skill",
    "purpose": "Guide free-form conversation into structured spec proposal",
    "path": "skills/propose",
    "uses_commands": [],
    "depends_on_modules": []
  },
  {
    "name": "spec",
    "type": "skill",
    "purpose": "Read proposal and author spec files",
    "path": "skills/spec",
    "uses_commands": ["validate"],
    "depends_on_modules": ["schema", "validator"]
  },
  {
    "name": "sync",
    "type": "hybrid",
    "purpose": "Review impact report and curate bead actions",
    "path": "skills/sync",
    "uses_commands": ["diff", "impact", "apply"],
    "depends_on_modules": ["impact", "apply", "map"]
  },
  {
    "name": "implement",
    "type": "agent",
    "purpose": "Implement a bead task — write code, tests, create PR",
    "path": "skills/implement",
    "uses_commands": ["check"],
    "depends_on_modules": ["map"]
  },
  {
    "name": "review",
    "type": "agent",
    "purpose": "Review PR for correctness and spec traceability",
    "path": "skills/review",
    "uses_commands": [],
    "depends_on_modules": []
  },
  {
    "name": "fix",
    "type": "agent",
    "purpose": "Fix review comments on a pull request",
    "path": "skills/fix",
    "uses_commands": [],
    "depends_on_modules": []
  }
]
```

Schema change: `project.schema.json` gets a `skills` array definition with properties: `name`, `type`, `purpose`, `path`, `uses_commands`, `depends_on_modules`. The `type` field accepts `"skill"`, `"agent"`, or `"hybrid"`.

Validator: New SkillPathChecker verifies each skill's `path` exists and contains a SKILL.md file.

### Three-tier architecture

The `type` field reflects a three-tier architecture for LLM-assisted development:

| Tier | Type | Mode | Examples | When |
|------|------|------|----------|------|
| **Skills** | `skill` | Interactive, user-in-the-loop | `/propose`, `/spec` | User drives creative work, LLM assists with decisions |
| **Agents** | `agent` | Autonomous, fire-and-forget | `/implement`, `/review`, `/fix` | LLM executes mechanical work, user reviews results |
| **Orchestrator** | (external) | Deterministic pipeline runner | Faber | Zero LLM tokens — pure DAG traversal, container management, status tracking |

**Skills** are conversational — the user talks through ideas, the LLM asks clarifying questions, they surface trade-offs together. Cannot be autonomous because creative decisions require human judgment.

**Agents** are autonomous — given structured input (bead ID, PR number), they execute a well-defined workflow (read spec, write code, run tests, create PR) without needing back-and-forth. Benefits: can run in background, can parallelize (implement two beads at once), don't pollute main conversation context.

**Hybrid** (`sync`) is a middle ground — the analysis phase (diff → impact → read mapping) is mechanical and runs autonomously, but action curation ("this looks like a rename, collapse into one task?") needs user approval. Design: agent performs analysis and presents curated results → user approves → agent calls apply.

The orchestrator (faber) sits outside the LLM tier entirely. It is a deterministic Go binary that does graph traversal and container management. LLM tokens only burn inside agent containers doing creative work. Everything structural (triage, scheduling, layer progression, pause/resume) is computation, not generation.

### Input/output contracts for agents

Agent-type entries (`implement`, `review`, `fix`) and the hybrid (`sync`) need input/output contracts in their SKILL.md files to enable autonomous execution:

- **Input contract**: What structured input the agent expects (e.g., bead ID for implement, PR number for review)
- **Output contract**: What the agent produces (e.g., PR URL, review comments, commit hash)
- **Autonomy level**: Fully autonomous vs needs approval at specific checkpoints (sync's hybrid model)

This metadata is informal in Phase 1 (documented in SKILL.md prose). In Phase 2, input/output contracts become formalized fields in `skill.schema.json`.

**Merkle impact**: SKILL.md files are outside `spec/` so they're not in the merkle tree currently. Recommendation: keep outside for now (Phase 1 is a catalog, not full integration). Changes to SKILL.md files don't trigger merkle diffs — only changes to the `skills` array in project.json do. In Phase 2, skills get their own spec directories under `spec/skills/` and full merkle tracking.

**Phase 2 — Skills as first-class spec concept**:
After implementing `/sync` and running the full pipeline (propose → spec → validate → diff → impact → sync → apply → implement), design a dedicated `skill.schema.json` that captures skill-specific structure: defined inputs/outputs, workflow steps, subcommand dependencies, triggers, preconditions. Each skill gets its own directory in the spec (e.g., `spec/skills/sync/`) with `skill.json` + markdown leaves (`workflow_*.md`, `input_*.md`), fully tracked by the merkle tree. This gives skills the same level of structured specification that modules have — changes to skills are detected by diff, impact can map skill changes to beads, and the validator can check skill contracts. Deferred because we don't yet know what the right schema fields are — implementing the pipeline integration will teach us what skills actually need.

**Evolution trigger**: After implementing `/sync` and running the full pipeline, we'll know what skill-specific schema fields matter. A follow-up proposal then designs `skill.schema.json` and migrates from the project.json catalog to the full spec concept.

## Impact expectation

- **Modified**: `schema/project.schema.json` (add `skills` array with `type` field)
- **Modified**: `spec/project.json` (add skills entries with skill/agent/hybrid types)
- **Modified**: `skills/spec/SKILL.md` (call `spex validate`)
- **Modified**: `skills/implement/SKILL.md` (convert to agent, use `spex check`, add input/output contracts)
- **Modified**: `skills/review/SKILL.md` (convert to agent, add input/output contracts)
- **Modified**: `skills/fix/SKILL.md` (convert to agent, add input/output contracts)
- **New**: `skills/sync/SKILL.md` (hybrid: agent analysis + user approval)
- **New**: Validator SkillPathChecker (optional, lightweight)
- **Dependencies**: Proposal 1 (mapping layer for `/sync` and `/implement` preflight), Proposal 2 (test format for `/spec` test generation)
