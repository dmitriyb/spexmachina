# Context Resolver Tests

## Setup

All scenarios use a temporary spec directory with module.json files containing components, impl_sections, test_sections, and data_flows. Mapping records reference these components.

**Fixture structure:**

```
spec/
  alpha/
    module.json       # 2 components, 2 impl_sections, 1 test_section, 1 data_flow
    arch_parser.md
    arch_builder.md
    impl_parsing.md
    impl_building.md
    test_components.md
    flow_pipeline.md
```

**Fixture module.json** (alpha):
- Component 1 (Parser): content "arch_parser.md"
- Component 2 (Builder): content "arch_builder.md"
- impl_section 1: describes [1], content "impl_parsing.md"
- impl_section 2: describes [2], content "impl_building.md"
- test_section 1: describes [1, 2], content "test_components.md"
- data_flow 1: uses [1, 2], content "flow_pipeline.md"

**Fixture record:**
```json
{
  "id": 1,
  "spec_node_id": "alpha/component/1",
  "bead_id": "abc-123",
  "bead_type": "feature",
  "module": "alpha",
  "component": "Parser",
  "content_file": "spec/alpha/arch_parser.md",
  "spec_hash": "deadbeef"
}
```

## Scenarios

### S1: Resolve context for component with full coverage

**Given** the fixture record for component 1 (Parser) and the fixture module.

**When** `ResolveContext(specDir, record)` is called.

**Then:**
- `ContextResult.ArchFile` is `"spec/alpha/arch_parser.md"`
- `ContextResult.ImplFiles` contains `"spec/alpha/impl_parsing.md"` (impl_section 1 describes [1])
- `ContextResult.TestFiles` contains `"spec/alpha/test_components.md"` (test_section 1 describes [1, 2])
- `ContextResult.FlowFiles` contains `"spec/alpha/flow_pipeline.md"` (data_flow 1 uses [1, 2])
- `ContextResult.ModuleFile` is `"spec/alpha/module.json"`

### S2: Component referenced by multiple impl_sections

**Given** a module where component 1 is described by impl_sections 1 and 3.

**When** `ResolveContext(specDir, record)` is called.

**Then:** `ContextResult.ImplFiles` contains both impl_section content paths in module declaration order.

### S3: Component with no data_flows

**Given** a module where no data_flow's `uses` array contains the component ID.

**When** `ResolveContext(specDir, record)` is called.

**Then:** `ContextResult.FlowFiles` is empty (nil or zero-length). No error — this is valid.

### S4: Resolve context for component 2 (Builder)

**Given** a record for component 2 and the fixture module.

**When** `ResolveContext(specDir, record)` is called.

**Then:**
- `ArchFile` is the Builder's content file
- `ImplFiles` contains `impl_building.md` path (impl_section 2 describes [2])
- `TestFiles` contains `test_components.md` path (test_section 1 describes [1, 2] — shared)
- `FlowFiles` contains `flow_pipeline.md` path (data_flow 1 uses [1, 2])

## Edge Cases

### E1: Missing module.json

**Given** a record referencing module "deleted" but `spec/deleted/module.json` does not exist.

**When** `ResolveContext(specDir, record)` is called.

**Then:** Returns an error indicating the module.json is missing. Error includes the expected path.

### E2: Invalid spec_node_id format

**Given** a record with `spec_node_id: "malformed"` (no slashes).

**When** `ResolveContext(specDir, record)` is called.

**Then:** Returns an error indicating the spec_node_id format is invalid.

### E3: Component ID not found in any section

**Given** a module where the component ID in the record does not appear in any impl_section.describes, test_section.describes, or data_flow.uses.

**When** `ResolveContext(specDir, record)` is called.

**Then:** Returns a valid ContextResult with empty ImplFiles, TestFiles, and FlowFiles. ArchFile and ModuleFile are still populated. Not an error.

### E4: spec_node_id module segment differs from record.Module

**Given** a record with `spec_node_id: "beta/component/1"` but `module: "alpha"`.

**When** `ResolveContext(specDir, record)` is called.

**Then:** Either returns an error (inconsistency detected) or uses `record.Module` as the authoritative source. The implementation should document which takes precedence.
