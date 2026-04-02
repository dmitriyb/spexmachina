# Change Proposal: Tool Delivery

## Context

All spex modules are implemented, all beads are closed, and the spec is fully covered. The tool works — but only for developers who clone the repo and run `go build`. There is no CI pipeline, no release process, no published binaries, and no installation documentation.

The `cmd/spex/version.go` already defines `version`, `commit`, and `date` variables designed for ldflags injection, but no build system wires these up. The `.github/` directory contains only a PR template.

More fundamentally, spex has no way to represent delivery in a project's spec. If spex is a general-purpose spec tool covering requirements through implementation through QA, then delivery is a gap — every real project has CI, releases, and distribution, but there's no place in the spec schema to declare it. Testing was added as a first-class schema concept (`test_plan` in project.json, `test_sections` in module.json) because it crosscuts every component. Delivery deserves the same first-class treatment.

### The coupled sections pattern

Three approaches were considered for how delivery fits in the spec:

1. **Project-level section only** — declares the delivery strategy, but spex's impact pipeline has no module/components to map changes to. Beads can't be created through the standard pipeline. Would require special-case logic.

2. **Module only** — has components and creates beads normally, but without a project-level schema section, delivery isn't a first-class concept. Each project would reinvent the structure.

3. **Sections array with type-based coupling** (chosen) — project.json gets a `sections` array where each entry has an `id`, `name`, and `type`. The type drives behavior: `type: "coupled"` tells spex this section requires a corresponding module. A coupled module must include a `section.schema.json` that defines validation rules for its section's content. Spex enforces the coupling and delegates content validation to the module's schema.

This is type-driven, not name-driven. Spex doesn't hardcode knowledge of "delivery" or "performance" — it knows about the `coupled` type. Adding a new project-level concern (performance, UI/UX, accessibility, anything) means adding an entry to the `sections` array with `type: "coupled"`, creating a module, and providing a `section.schema.json`. No spex binary changes. The coupling, validation, merkle tracking, impact mapping, and bead creation all work automatically from the existing pipeline.

### Module-owned schema validation

Each coupled module must provide a `section.schema.json` file in its spec directory (e.g., `spec/delivery/section.schema.json`). This is a standard JSON Schema that defines the valid structure for the module's project-level section content. During `spex validate`, for each section with `type: "coupled"`, spex:

1. Checks that a module with matching `name` exists in the modules array
2. Loads `spec/<module-path>/section.schema.json`
3. Validates the section's freeform content against that schema

This reuses spex's existing JSON Schema validation infrastructure (~20 lines of new code in the validator). The coupled module owns its own validation rules — spex core only validates the envelope (`id`, `name`, `type`).

### Follow-up proposals (deferred from this one)

Two related concerns were identified during discussion and deliberately scoped out:

1. **test_plan migration** — The existing `test_plan` section in project.json could be migrated into the `sections` array to unify the model. However, `test_plan` is lightly coupled (scenarios reference module IDs) while `test_sections` in each module are deeply interconnected with components (via `describes`). A thorough examination of the full connection graph is needed before migrating. This should be a dedicated proposal that: (a) maps all test_plan ↔ module ↔ test_sections connections, (b) determines if test_plan can become a `type: "coupled"` section with a test module, or needs a different type (e.g., `"distributed"`) reflecting its cross-module nature, (c) handles the breaking change for existing specs.

2. **Project initialization** — Currently specs live in `spec/`. A `spex init` command and `.spexmachina/` project directory convention would improve the first-use experience. This is independent of delivery (install → init → use are sequential in the user journey but separate in implementation scope) and should be its own proposal covering: directory structure, `spex init` command, project bootstrapping UX, migration path for existing specs.

Both proposals should reference this one for context on the sections/coupled-type system they build upon.

## Proposed change

### 1. Add `sections` array to project.json schema

Replace hardcoded project-level concern keys with a generic `sections` array. Each entry has a required envelope (`id`, `name`, `type`) and freeform content validated by the coupled module's schema:

```json
"sections": [
  {
    "id": 1,
    "name": "delivery",
    "type": "coupled",
    "versioning": {
      "scheme": "semver",
      "source": "git-tag"
    },
    "artifacts": [
      {
        "id": 1,
        "name": "spex",
        "type": "binary",
        "platforms": ["linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"]
      }
    ],
    "channels": [
      { "id": 1, "name": "github-releases", "type": "direct", "artifact": 1 },
      { "id": 2, "name": "homebrew", "type": "package-manager", "artifact": 1 },
      { "id": 3, "name": "aur", "type": "package-manager", "artifact": 1 },
      { "id": 4, "name": "go-install", "type": "registry", "artifact": 1 }
    ],
    "checks": [
      { "id": 1, "name": "vet", "description": "Static analysis" },
      { "id": 2, "name": "test", "description": "Unit and integration tests" },
      { "id": 3, "name": "build", "description": "Verify cross-platform compilation" }
    ]
  }
]
```

**Schema design for the envelope** (validated by spex core):
- `id` (required, integer >= 1, unique within sections)
- `name` (required, string) — must match the coupled module's name
- `type` (required, string) — `"coupled"` enforces module existence + schema validation. Future types possible.

**Schema design for the delivery content** (validated by `spec/delivery/section.schema.json`):
- **`versioning`** (required): `scheme` (free string: "semver", "calver", "commit", etc.) and `source` (free string: "git-tag", "file", "manual", etc.). Describes intent, not tooling.
- **`artifacts`** (required, >= 1): Each has `id`, `name`, `type` (free string: "binary", "container", "library", "archive", etc.), and optional `platforms` array. A Go CLI defines binaries with platforms. A web app defines a container. A library defines a package.
- **`channels`** (required, >= 1): Each has `id`, `name`, `artifact` (reference to artifact ID). Optional `type` ("package-manager", "registry", "container-registry", "direct") enables cross-validation.
- **`checks`** (optional): Each has `id`, `name`, `description`. Optional `modules` array scopes a check to specific modules.
- **`environments`** (optional): Each has `id`, `name`, optional `auto_deploy` boolean. CLI tools skip this. Web services use it.

All `type` fields in delivery content are free strings, not enums, to support any project type without schema changes.

### 2. Add project requirements and delivery module declaration

Add three new requirements to `spec/project.json`:

**Requirement 15 — Coupled sections:**
```json
{
  "id": 15,
  "type": "functional",
  "title": "Coupled sections",
  "description": "Project.json supports a sections array with typed entries. Sections with type 'coupled' require a corresponding module and a section.schema.json in the module's spec directory. Spex validates the envelope generically and delegates content validation to the module's schema.",
  "priority": 1
}
```

**Requirement 16 — Delivery specification:**
```json
{
  "id": 16,
  "type": "functional",
  "title": "Delivery specification",
  "description": "A coupled section type for delivery: declares versioning, artifacts, distribution channels, CI checks, and optional environments. Schema validated by the delivery module's section.schema.json.",
  "depends_on": [15],
  "priority": 1
}
```

**Requirement 17 — Installable:**
```json
{
  "id": 17,
  "type": "non_functional",
  "title": "Installable",
  "description": "Pre-built binaries for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64). Installable via GitHub Releases, go install, Homebrew, and AUR.",
  "priority": 2
}
```

Add a new module declaration:

```json
{
  "id": 10,
  "name": "delivery",
  "path": "delivery",
  "description": "Delivery infrastructure: CI pipelines, release automation, and package distribution. Implements the project-level delivery section for spex itself. Provides section.schema.json for delivery content validation.",
  "requires_module": [1]
}
```

Add a new milestone:

```json
{
  "id": 7,
  "title": "Delivery",
  "description": "Coupled sections framework, delivery section schema, CI/CD pipelines, release automation, and distribution channels.",
  "groups": [10]
}
```

### 3. Schema module changes

In `spec/schema/module.json`:

- Add requirement 8: "Define sections array in project schema" (`preq_id: 15`). The project.json JSON Schema adds a `sections` array. Each entry requires `id` (integer >= 1), `name` (string), `type` (string). Additional properties allowed (freeform content validated externally by module schemas).
- Add requirement 9: "Define section.schema.json convention" (`preq_id: 15`). Document and enforce the convention that coupled modules provide `section.schema.json` in their spec directory.
- Update ProjectSchema component (component 1) `implements` to include requirements 8 and 9 (currently implements [1, 4, 6], becomes [1, 4, 6, 8, 9]).

### 4. Validator module changes

In `spec/validator/module.json`:

- Add requirement 14: "Validate sections and coupled modules" (`preq_id: 15`). Three concerns:
  - **Envelope validation**: Section IDs unique, required fields present.
  - **Coupled enforcement**: For each section with `type: "coupled"`, a module with matching `name` must exist. Bidirectional: a module claiming to be coupled must have a corresponding section.
  - **Schema delegation**: For each coupled section, load `spec/<module-path>/section.schema.json` and validate the section content against it. Error if the schema file is missing.
- Add a CoupledSectionChecker component to implement this validation generically (not delivery-specific — works for any future coupled section).

### 5. Render module changes

In `spec/render/module.json`:

The render command currently has hardcoded handling in `render/spec_reader.go` for Components, ImplSections, DataFlows, and TestSections. All three renderers (markdown, DOT, JSON) have explicit blocks for each section type. TestSections and test_plan are read but never rendered (existing gap).

- Add requirement (next available ID): "Render sections" (`preq_id: 15`). The spec reader and all three renderers (markdown, DOT, JSON) must handle the `sections` array generically — iterate over sections by name, render each section's content, and pull related content from the coupled module.
- Update SpecReader component to read the `sections` array from project.json.
- Update MarkdownRenderer, DOTRenderer, and JSONRenderer components to render sections.

This should be generic: iterate `sections`, render by name, pull coupled module content. No hardcoded "if delivery" logic.

### 6. Delivery module (`spec/delivery/module.json`)

New module with requirements tracing to project requirements and components for each delivery concern:

**Requirements:**
- Req 1: "Delivery section schema" (`preq_id: 16`) — Define `section.schema.json` validating versioning, artifacts, channels, checks, environments.
- Req 2: "CI pipeline" (`preq_id: 17`) — Automated checks on every PR and push to main.
- Req 3: "Release pipeline" (`preq_id: 17`) — Automated cross-platform builds and GitHub Release creation on version tag.
- Req 4: "Homebrew distribution" (`preq_id: 17`) — Homebrew formula for macOS and Linux installation.
- Req 5: "AUR distribution" (`preq_id: 17`) — AUR package for Arch Linux installation.
- Req 6: "Local build tooling" (`preq_id: 17`) — Makefile with build, test, vet, install, and release dry-run targets.

**Components:**
- **DeliverySectionSchema** (implements req 1) — The `section.schema.json` file defining valid delivery content structure: versioning, artifacts, channels, checks, environments with all the design principles described in part 1.
- **CIPipeline** (implements req 2) — GitHub Actions workflow triggered on push/PR. Runs vet, test, build.
- **ReleasePipeline** (implements req 3) — GitHub Actions workflow triggered on `v*` tags. Uses GoReleaser for cross-platform builds, ldflags injection, checksums, GitHub Releases upload.
- **HomebrewPackage** (implements req 4) — GoReleaser-generated Homebrew formula. Separate tap repo or inline config.
- **AURPackage** (implements req 5) — PKGBUILD for `spex-bin` binary package.
- **BuildTooling** (implements req 6) — Makefile with ldflags-aware build, test, vet, install, release dry-run targets.

Each component gets content leaves (arch_*.md, impl_*.md) and test_sections. Standard spex structure — merkle tracks it, impact maps it, beads get created through the normal pipeline.

### 7. Skill changes

**`/propose` skill** (`skills/propose/SKILL.md`):

In **Step 4: Research** → "For change proposals, read" section, add after item 2:
- "All sections in `project.json` `sections` array — understand existing coupled sections and their modules."

In the **Change Proposal Template** → "Proposed change" guidance, add:
- "If the change involves a coupled section, specify: which section is affected, whether its `section.schema.json` needs updates, and whether the coupled module's components change."

**`/spec` skill** (`skills/spec/SKILL.md`):

In the **Schema Reference** section, add to project.json listing:
- `sections` — array of typed project-level sections. Each has `id`, `name`, `type`. Type `"coupled"` requires a module with matching name and a `section.schema.json` in the module's spec directory.

In the **File Layout** section, add:
- `spec/<module>/section.schema.json` — JSON Schema for coupled section content validation. Required for modules that implement a coupled section.

In the **Edge table**, add:
- `sections[].name` → `modules[].name` — coupled section references its implementing module by name match.

In **"Plan the spec graph"** step, add to the summary:
- Include any sections changes and whether coupled modules need creation or updates.

### 8. Spex's own delivery section in project.json

Add the `sections` array with the delivery entry shown in part 1 to `spec/project.json`, dogfooding the new schema.

### 9. Installation documentation

Update `README.md` with an Installation section covering all channels:
- GitHub Releases: direct binary download
- `go install github.com/dmitriyb/spexmachina/cmd/spex@latest`
- Homebrew: `brew install dmitriyb/tap/spex`
- AUR: `yay -S spex-bin`
- Build from source: `go build -o bin/ ./cmd/spex/`

## Impact expectation

This proposal touches four spec modules (schema, validator, render, new delivery), adds project-level requirements + sections array + milestone, and updates two skills.

**New beads:**
- Epic bead for delivery module (module 10)
- Feature beads for each delivery component (DeliverySectionSchema, CIPipeline, ReleasePipeline, HomebrewPackage, AURPackage, BuildTooling)
- Task beads for each component's impl and test sections
- Beads for schema module requirements 8-9 and ProjectSchema component update
- Beads for validator module requirement 14 and CoupledSectionChecker component
- Beads for render module section rendering requirement and renderer component updates

**Modified beads:** None — existing requirements and components are unchanged.

**Closed beads:** None.

**Estimated scope:** 4-5 sessions:
- Session 1: Schema changes (sections array, section.schema.json convention)
- Session 2: Validator changes (CoupledSectionChecker, schema delegation)
- Session 3: Render changes (generic section rendering) + skill updates
- Session 4: Delivery module spec + content leaves + section.schema.json
- Session 5: Infrastructure implementation (CI, GoReleaser, Homebrew, AUR, Makefile, README)

**Follow-up proposals** (to be created after this proposal is complete):
1. **test_plan migration** — examine test_plan ↔ module ↔ test_sections connection graph, determine appropriate section type, handle breaking change. References this proposal for the sections/coupled-type system.
2. **Project initialization** — `spex init` command, `.spexmachina/` directory convention, project bootstrapping UX. References this proposal for sections support in initialized projects.
