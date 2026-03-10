# Spec Reading Tests

## Setup

All scenarios use an in-memory or temporary filesystem containing a valid spec directory structure. The minimal fixture includes:

- `spec/project.json` with at least one module declaration
- `spec/<module>/module.json` with requirements, components, impl_sections, data_flows, and test_sections
- Content leaf files (`arch_*.md`, `impl_*.md`, `flow_*.md`, `test_*.md`) referenced by module.json

The fixture spec should have at least two modules with an inter-module dependency (`requires_module`) to exercise cross-module graph construction.

**Fixture structure:**

```
spec/
  project.json          # name, description, 2+ modules, requirements, milestones
  alpha/
    module.json          # 2 requirements, 2 components, 1 impl_section, 1 data_flow
    arch_parser.md
    arch_builder.md
    impl_parsing.md
    flow_build_pipeline.md
  beta/
    module.json          # 1 requirement, 1 component (uses alpha), 1 impl_section
    arch_consumer.md
    impl_consumption.md
```

## Scenarios

### S1: Parse minimal valid spec into SpecGraph

**Given** a spec directory with a valid `project.json` referencing one module, and that module's `module.json` is valid with one component and one content leaf.

**When** `ReadSpec(specDir)` is called.

**Then:**
- Returns a non-nil `*SpecGraph` with no error
- `SpecGraph.Project.Name` matches the name in `project.json`
- `SpecGraph.Modules` has length 1
- `SpecGraph.Modules[0].Module.Name` matches the module name

### S2: Content map populated with all markdown leaves

**Given** a module with 2 components (`arch_parser.md`, `arch_builder.md`), 1 impl_section (`impl_parsing.md`), and 1 data_flow (`flow_build_pipeline.md`).

**When** `ReadSpec(specDir)` is called.

**Then:**
- `ModuleGraph.Content` has exactly 4 entries
- Each key is the relative content path (e.g., `arch_parser.md`)
- Each value is the full markdown string read from that file
- Content values are byte-identical to the original files (no trimming, no heading adjustment at read time)

### S3: Multi-module spec with cross-module dependency

**Given** `project.json` declares modules `alpha` (id: 1) and `beta` (id: 2), where `beta` has `requires_module: [1]`.

**When** `ReadSpec(specDir)` is called.

**Then:**
- `SpecGraph.Modules` has length 2
- Both modules are present with their full content maps populated
- The `requires_module` relationship is preserved in the parsed `Module` struct (beta's module struct has `RequiresModule: [1]`)
- Module ordering in `SpecGraph.Modules` matches declaration order in `project.json`

### S4: Project-level requirements and milestones preserved

**Given** `project.json` has 3 requirements (2 functional, 1 non-functional) and 1 milestone grouping modules.

**When** `ReadSpec(specDir)` is called.

**Then:**
- `SpecGraph.Project.Requirements` has length 3
- Each requirement preserves `id`, `type`, `title`, `description`, and `depends_on`
- `SpecGraph.Project.Milestones` has length 1 with correct `groups` references

### S5: All module-level edge types preserved

**Given** a module with:
- Requirements with `preq_id` and `depends_on`
- Components with `implements` and `uses`
- Impl_sections with `describes`
- Data_flows with `uses`

**When** `ReadSpec(specDir)` is called.

**Then:**
- Component `implements` arrays contain the correct requirement IDs
- Component `uses` arrays contain the correct peer component IDs
- Impl_section `describes` arrays contain the correct component IDs
- Data_flow `uses` arrays contain the correct component IDs
- Requirement `preq_id` values trace to project-level requirement IDs

### S6: Content with special characters and unicode

**Given** a content file containing markdown with code blocks, inline code with backticks, unicode characters, HTML entities, and lines exceeding 1000 characters.

**When** `ReadSpec(specDir)` is called.

**Then:**
- Content is read verbatim without corruption
- Code blocks with triple backticks are preserved intact
- Unicode characters are preserved (no encoding conversion)
- Long lines are not truncated

## Edge Cases

### E1: Missing project.json

**Given** a spec directory that exists but contains no `project.json`.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error indicating `project.json` is missing. Error message includes the expected path.

### E2: Missing module.json for declared module

**Given** `project.json` references module path `gamma`, but `spec/gamma/module.json` does not exist.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error identifying the missing module by name and path. The error message includes both the module name from `project.json` and the expected filesystem path.

### E3: Missing content leaf file

**Given** a module's `module.json` references `arch_widget.md` in a component's `content` field, but the file does not exist on disk.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error identifying the missing content file path. Since content is required for rendering, this is a hard error (not a warning).

### E4: Malformed JSON in project.json

**Given** `project.json` contains invalid JSON (e.g., trailing comma, unquoted key).

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error with the file path and JSON parse details (line/column if available, or the Go `json.Unmarshal` error message).

### E5: Malformed JSON in module.json

**Given** one module's `module.json` is valid but a second module's `module.json` contains a syntax error.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error identifying which module's JSON failed to parse. The first module should not produce partial results — the entire call fails.

### E6: Empty content file

**Given** a content leaf file exists but is 0 bytes.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns a `*SpecGraph` with no error. The `Content` map entry for that file has an empty string value. Empty content is valid (the file exists, it just has no text).

### E7: Spec directory does not exist

**Given** `specDir` points to a non-existent directory.

**When** `ReadSpec(specDir)` is called.

**Then:** Returns an error indicating the directory does not exist.

### E8: Module with no content fields

**Given** a module whose components, impl_sections, and data_flows all omit the `content` field (content is optional in the schema).

**When** `ReadSpec(specDir)` is called.

**Then:** Returns a `*SpecGraph` with no error. The `Content` map for that module is empty (length 0). No file reads are attempted for missing content fields.
