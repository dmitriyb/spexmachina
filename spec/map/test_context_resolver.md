# Context Resolver Tests

## Setup

All scenarios use a temporary spec directory with module.json files containing components,
test_sections, and data_flows, plus a `spec/.history.jsonl` journal for the removed-node cases.
Resolution is keyed by identity hash or task id — there are no mapping records.

**Fixture structure:**

```
spec/
  .history.jsonl
  alpha/
    module.json       # 2 components, 1 test_section, 1 data_flow
    arch_parser.md
    arch_builder.md
    test_components.md
    flow_pipeline.md
```

**Fixture module.json** (alpha):
- Component `aabbccddeeff` (Parser): content "arch_parser.md"
- Component `ffeeddccbbaa` (Builder): content "arch_builder.md"
- test_section `333333333333`: describes [`aabbccddeeff`, `ffeeddccbbaa`], content "test_components.md"
- data_flow `444444444444`: uses [`aabbccddeeff`, `ffeeddccbbaa`], content "flow_pipeline.md"

**Fixture journal:** an `added` event for each component with a `task_created` receipt
(Parser → task `abc-123`), and for the removed-node cases a node `999999999999` (Widget,
component, module alpha) with `added`, `task_created` (task `abc-777`), then `removed`
(proposal `2026-08-01-task-journal`, git_head `cafe1234`).

## Scenarios

### S1: Resolve context for a live component by identity hash

**Given** the fixture spec and journal.

**When** `ResolveContext(specDir, "aabbccddeeff")` is called.

**Then:**
- `ContextResult.ArchFile` is `"spec/alpha/arch_parser.md"` — derived from the component's declared `content`, not from any stored path
- `ContextResult.TestFiles` contains `"spec/alpha/test_components.md"` (test_section `333333333333` describes [`aabbccddeeff`, `ffeeddccbbaa`])
- `ContextResult.FlowFiles` contains `"spec/alpha/flow_pipeline.md"` (data_flow `444444444444` uses [`aabbccddeeff`, `ffeeddccbbaa`])
- `ContextResult.ModuleFile` is `"spec/alpha/module.json"`

### S2: Resolve by task id reaches the same node

**Given** the fixture journal pairing Parser with task `abc-123`.

**When** `ResolveContext(specDir, "abc-123")` is called.

**Then:** the result is identical to S1 — the task id resolves through the journal fold to the identity hash, then the spec derivation runs unchanged.

### S3: Component referenced by multiple test_sections

**Given** a module where Parser is described by test_section `333333333333` and by a second test_section `555555555555`, content "test_parser.md".

**When** `ResolveContext(specDir, "aabbccddeeff")` is called.

**Then:** `ContextResult.TestFiles` contains both test_section content paths in module declaration order.

### S4: Component with no data_flows

**Given** a module where no data_flow's `uses` array contains the component's identity hash.

**When** `ResolveContext(specDir, "aabbccddeeff")` is called.

**Then:** `ContextResult.FlowFiles` is empty (nil or zero-length). No error — this is valid.

### S5: Removed node resolves from the journal

**Given** the fixture journal's removed node `999999999999`.

**When** `ResolveContext(specDir, "999999999999")` is called.

**Then:** the result carries no spec file paths and instead reports: name `Widget`, node_type `component`, module `alpha`, the removing proposal `2026-08-01-task-journal`, the last task `abc-777`, and the `git_head` refs bracketing its final change — everything needed to run `git show`/`git diff` for the retired leaf. Not an error.

## Edge Cases

### E1: Missing module.json

**Given** a live journal entry whose module directory has no `module.json` on disk.

**When** `ResolveContext` is called with that node's hash.

**Then:** returns an error naming the expected `module.json` path.

### E2: Key known to neither spec nor journal

**Given** a hash that appears in no module.json and no journal event.

**When** `ResolveContext(specDir, "deadbeefdead")` is called.

**Then:** returns a not-found error naming the key. The error distinguishes "unknown everywhere" from S5's removed-but-remembered case.

### E3: Component hash not found in any section

**Given** a live component whose identity hash appears in no test_section.describes and no data_flow.uses.

**When** `ResolveContext` is called.

**Then:** returns a valid ContextResult with empty TestFiles and FlowFiles. ArchFile and ModuleFile are still populated. Not an error.
