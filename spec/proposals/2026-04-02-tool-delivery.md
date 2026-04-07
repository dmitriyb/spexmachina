# Change Proposal: Tool Delivery

## Context

All spex modules are implemented, all beads are closed, and the spec is fully covered. The tool works — but only for developers who clone the repo and run `go build`. There is no CI pipeline, no release process, no published binaries, and no installation documentation.

The `cmd/spex/version.go` already defines `version`, `commit`, and `date` variables designed for ldflags injection, but no build system wires these up. The `.github/` directory contains only a PR template.

### Prerequisite: Coupled sections

This proposal depends on the **Coupled Sections** proposal (`2026-04-02-coupled-sections.md`), which adds a generic `sections` array to project.json with type-based coupling to modules. That framework must be implemented first — specifically, `schema/project.schema.json` must support the `sections` property, the validator must enforce coupled module constraints, and renderers must handle sections generically.

Once the coupled sections framework is in place, delivery becomes the first consumer: a `type: "coupled"` section in project.json with a delivery module that provides `section.schema.json` for content validation.

### Delivery section content design

The delivery section content (validated by `spec/delivery/section.schema.json`, not by spex core) has this structure:

- **`versioning`** (required): `scheme` (free string: "semver", "calver", "commit", etc.) and `source` (free string: "git-tag", "file", "manual", etc.). Describes intent, not tooling.
- **`artifacts`** (required, >= 1): Each has `id`, `name`, `type` (free string: "binary", "container", "library", "archive", etc.), and optional `platforms` array. A Go CLI defines binaries with platforms. A web app defines a container. A library defines a package.
- **`channels`** (required, >= 1): Each has `id`, `name`, `artifact` (reference to artifact ID). Optional `type` ("package-manager", "registry", "container-registry", "direct") enables cross-validation.
- **`checks`** (optional): Each has `id`, `name`, `description`. Optional `modules` array scopes a check to specific modules.
- **`environments`** (optional): Each has `id`, `name`, optional `auto_deploy` boolean. CLI tools skip this. Web services use it.

All `type` fields in delivery content are free strings, not enums, to support any project type without schema changes.

### Follow-up proposals (deferred from this one)

Two related concerns were identified during discussion and deliberately scoped out:

1. **test_plan migration** — The existing `test_plan` section in project.json could be migrated into the `sections` array to unify the model. However, `test_plan` is lightly coupled (scenarios reference module IDs) while `test_sections` in each module are deeply interconnected with components (via `describes`). A thorough examination of the full connection graph is needed before migrating. This should be a dedicated proposal that: (a) maps all test_plan <-> module <-> test_sections connections, (b) determines if test_plan can become a `type: "coupled"` section with a test module, or needs a different type (e.g., `"distributed"`) reflecting its cross-module nature, (c) handles the breaking change for existing specs.

2. **Project initialization** — Currently specs live in `spec/`. A `spex init` command and `.spexmachina/` project directory convention would improve the first-use experience. This is independent of delivery (install -> init -> use are sequential in the user journey but separate in implementation scope) and should be its own proposal covering: directory structure, `spex init` command, project bootstrapping UX, migration path for existing specs.

Both proposals should reference the coupled sections proposal for context on the sections/coupled-type system they build upon.

## Proposed change

### 1. Add project requirements and delivery module declaration

Add two new requirements to `spec/project.json`:

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
  "description": "Delivery section schema, CI/CD pipelines, release automation, and distribution channels.",
  "groups": [10]
}
```

### 2. Add delivery section to project.json

Add the `sections` array with the delivery entry to `spec/project.json`, dogfooding the coupled sections framework:

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

### 3. Delivery module (`spec/delivery/module.json`)

New module with requirements tracing to project requirements and components for each delivery concern:

**Requirements:**
- Req 1: "Delivery section schema" (`preq_id: 16`) — Define `section.schema.json` validating versioning, artifacts, channels, checks, environments.
- Req 2: "CI pipeline" (`preq_id: 17`) — Automated checks on every PR and push to main.
- Req 3: "Release pipeline" (`preq_id: 17`) — Automated cross-platform builds and GitHub Release creation on version tag.
- Req 4: "Homebrew distribution" (`preq_id: 17`) — Homebrew formula for macOS and Linux installation.
- Req 5: "AUR distribution" (`preq_id: 17`) — AUR package for Arch Linux installation.
- Req 6: "Local build tooling" (`preq_id: 17`) — Makefile with build, test, vet, install, and release dry-run targets.

**Components:**
- **DeliverySectionSchema** (implements req 1) — The `section.schema.json` file defining valid delivery content structure: versioning, artifacts, channels, checks, environments with all the design principles described above.
- **CIPipeline** (implements req 2) — GitHub Actions workflow triggered on push/PR. Runs vet, test, build.
- **ReleasePipeline** (implements req 3) — GitHub Actions workflow triggered on `v*` tags. Uses GoReleaser for cross-platform builds, ldflags injection, checksums, GitHub Releases upload.
- **HomebrewPackage** (implements req 4) — GoReleaser-generated Homebrew formula. Separate tap repo or inline config.
- **AURPackage** (implements req 5) — PKGBUILD for `spex-bin` binary package.
- **BuildTooling** (implements req 6) — Makefile with ldflags-aware build, test, vet, install, release dry-run targets.

Each component gets content leaves (arch_*.md, impl_*.md) and test_sections. Standard spex structure — merkle tracks it, impact maps it, beads get created through the normal pipeline.

### 4. Installation documentation

Update `README.md` with an Installation section covering all channels:
- GitHub Releases: direct binary download
- `go install github.com/dmitriyb/spexmachina/cmd/spex@latest`
- Homebrew: `brew install dmitriyb/tap/spex`
- AUR: `yay -S spex-bin`
- Build from source: `go build -o bin/ ./cmd/spex/`

## Impact expectation

This proposal adds one new spec module (delivery) and the delivery section content in project.json. It depends on the coupled sections proposal being implemented first.

**New beads:**
- Epic bead for delivery module (module 10)
- Feature beads for each delivery component (DeliverySectionSchema, CIPipeline, ReleasePipeline, HomebrewPackage, AURPackage, BuildTooling)
- Task beads for each component's impl and test sections

**Modified beads:** None — existing requirements and components are unchanged.

**Closed beads:** None.

**Estimated scope:** 2 sessions:
- Session 1: Delivery module spec + content leaves + section.schema.json
- Session 2: Infrastructure implementation (CI, GoReleaser, Homebrew, AUR, Makefile, README)
