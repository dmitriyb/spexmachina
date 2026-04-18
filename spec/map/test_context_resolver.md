# Context Resolver Tests

## Setup

All scenarios use a temporary spec directory with module.json files containing components, impl_sections, test_sections, and data_flows. Mapping records reference these components by identity hash.

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
- Component `aabbccddeeff` (Parser): content "arch_parser.md"
- Component `ffeeddccbbaa` (Builder): content "arch_builder.md"
- impl_section `111111111111`: describes [`aabbccddeeff`], content "impl_parsing.md"
- impl_section `222222222222`: describes [`ffeeddccbbaa`], content "impl_building.md"
- test_section `333333333333`: describes [`aabbccddeeff`, `ffeeddccbbaa`], content "test_components.md"
- data_flow `444444444444`: uses [`aabbccddeeff`, `ffeeddccbbaa`], content "flow_pipeline.md"

**Fixture record:**
```json
{
  "id": 1,
  "spec_node_id": "aabbccddeeff",
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

**Given** the fixture record for Parser (`aabbccddeeff`) and the fixture module.

**When** `ResolveContext(specDir, record)` is called.

**Then:**
- `ContextResult.ArchFile` is `"spec/alpha/arch_parser.md"`
- `ContextResult.ImplFiles` contains `"spec/alpha/impl_parsing.md"` (impl_section `111111111111` describes [`aabbccddeeff`])
- `ContextResult.TestFiles` contains `"spec/alpha/test_components.md"` (test_section `333333333333` describes [`aabbccddeeff`, `ffeeddccbbaa`])
- `ContextResult.FlowFiles` contains `"spec/alpha/flow_pipeline.md"` (data_flow `444444444444` uses [`aabbccddeeff`, `ffeeddccbbaa`])
- `ContextResult.ModuleFile` is `"spec/alpha/module.json"`

### S2: Component referenced by multiple impl_sections

**Given** a module where Parser (`aabbccddeeff`) is described by impl_sections `111111111111` and a third impl_section `333333333333`.

**When** `ResolveContext(specDir, record)` is called.

**Then:** `ContextResult.ImplFiles` contains both impl_section content paths in module declaration order.

### S3: Component with no data_flows

**Given** a module where no data_flow's `uses` array contains the component's identity hash.

**When** `ResolveContext(specDir, record)` is called.

**Then:** `ContextResult.FlowFiles` is empty (nil or zero-length). No error — this is valid.

### S4: Resolve context for Builder (`ffeeddccbbaa`)

**Given** a record for Builder (`ffeeddccbbaa`) and the fixture module.

**When** `ResolveContext(specDir, record)` is called.

**Then:**
- `ArchFile` is the Builder's content file
- `ImplFiles` contains `impl_building.md` path (impl_section `222222222222` describes [`ffeeddccbbaa`])
- `TestFiles` contains `test_components.md` path (test_section `333333333333` describes [`aabbccddeeff`, `ffeeddccbbaa`] — shared)
- `FlowFiles` contains `flow_pipeline.md` path (data_flow `444444444444` uses [`aabbccddeeff`, `ffeeddccbbaa`])

## Edge Cases

### E1: Missing module.json

**Given** a record referencing module "deleted" but `spec/deleted/module.json` does not exist.

**When** `ResolveContext(specDir, record)` is called.

**Then:** Returns an error indicating the module.json is missing. Error includes the expected path.

### E3: Component hash not found in any section

**Given** a module where the identity hash in the record does not appear in any impl_section.describes, test_section.describes, or data_flow.uses.

**When** `ResolveContext(specDir, record)` is called.

**Then:** Returns a valid ContextResult with empty ImplFiles, TestFiles, and FlowFiles. ArchFile and ModuleFile are still populated. Not an error.
