# Renderer Tests

## Setup

All renderer scenarios operate on a pre-built `*SpecGraph` (output of `ReadSpec`). The fixture graph contains:

- **Project**: name "test-project", description "A test spec", 2 functional requirements (FR1, FR2) and 1 non-functional requirement (NFR1)
- **Module alpha** (id: 1): 2 requirements, 2 components (Parser, Builder), 1 impl_section, 1 data_flow
  - Parser: `implements: [1]`, `uses: []`, content: `"# Parser\n\nParses input into AST."`
  - Builder: `implements: [2]`, `uses: [1]`, content: `"# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first."`
  - Impl_section 1: `describes: [1]`, content: `"# Parsing Implementation\n\nUse recursive descent."`
  - Data_flow 1: `uses: [1, 2]`, content: `"# Build Pipeline\n\nParse then build."`
- **Module beta** (id: 2, `requires_module: [1]`): 1 requirement, 1 component (Consumer), 1 impl_section
  - Consumer: `implements: [1]`, `uses: []`, content: `"# Consumer\n\nConsumes built output."`

Each renderer writes to a `bytes.Buffer` so output can be inspected as a string.

## Scenarios

### MarkdownRenderer

#### M1: Document structure and ordering

**Given** the fixture SpecGraph.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** the output contains these sections in order:
1. `# test-project` (project heading)
2. Project description text
3. `## Requirements` with subsections `### Functional` (FR1, FR2) and `### Non-functional` (NFR1)
4. `## Module: alpha` with module description
5. Alpha's requirements section
6. `### Architecture` with component subsections for Parser and Builder
7. `### Implementation` with impl_section subsections
8. `### Data Flows` with data_flow subsections
9. `## Module: beta` following the same structure

Verify ordering by checking that the byte offset of each section heading is strictly increasing.

#### M2: Content inlining with heading adjustment

**Given** the Builder component has content `"# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first."`.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:**
- The Builder content appears under `### Architecture` within `## Module: alpha`
- The `# Builder` heading is adjusted to `#### Builder` (base level 4 under project > module > architecture)
- The `## Algorithm` subheading is adjusted to `##### Algorithm`
- The body text between headings is preserved verbatim

#### M3: Requirements formatting

**Given** the project has functional requirement FR1 with title "Parse input" and description "Accept structured input and parse it."

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** the output contains a line matching `FR1: Parse input` followed by or containing the description text. Functional and non-functional requirements appear in separate subsections.

#### M4: Module ordering matches project.json declaration order

**Given** project.json declares alpha (id: 1) before beta (id: 2).

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** `## Module: alpha` appears before `## Module: beta` in the output.

#### M5: Output is pure markdown with no front matter

**Given** any valid SpecGraph.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:**
- Output does not start with `---` (no YAML front matter)
- Output does not contain HTML tags
- Output starts with `# ` (the project heading)

### DOTRenderer

#### D1: Valid DOT syntax with digraph wrapper

**Given** the fixture SpecGraph.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- Output starts with `digraph spec {`
- Output ends with `}` (closing brace)
- Output contains `rankdir=LR`
- The output is syntactically valid DOT (parseable by graphviz without error)

#### D2: Module subgraphs as clusters

**Given** modules alpha and beta in the spec.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- Output contains `subgraph cluster_alpha {` with a closing `}`
- Output contains `subgraph cluster_beta {` with a closing `}`
- Each subgraph contains the nodes belonging to that module

#### D3: Node shapes match spec node types

**Given** the fixture SpecGraph with requirements, components, impl_sections, and data_flows.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- Requirement nodes have `shape=box`
- Component nodes have `shape=component`
- Impl_section nodes have `shape=note`
- Data_flow nodes have `shape=ellipse`

#### D4: Edge types rendered with correct styles

**Given** Builder `implements: [2]` and `uses: [1]` (Parser).

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- An edge exists from `alpha_comp_2` to `alpha_req_2` labeled `"implements"`
- An edge exists from `alpha_comp_2` to `alpha_comp_1` labeled `"uses"` (or with dotted style)
- The `implements` edge uses solid style
- The `uses` edge uses dotted style

#### D5: Cross-module edges rendered

**Given** beta has `requires_module: [1]` (depends on alpha).

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- An edge exists from the beta module node to the alpha module node
- This edge is labeled `"requires_module"` (or equivalent)
- The edge crosses subgraph boundaries (source in cluster_beta, target in cluster_alpha)

#### D6: Node IDs are valid DOT identifiers

**Given** any valid SpecGraph.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- All node IDs match the pattern `<module>_<type>_<id>` (e.g., `alpha_comp_1`, `beta_req_1`)
- No node ID contains spaces, hyphens, or special characters that would require quoting in DOT

#### D7: Node labels are human-readable

**Given** component Parser with name "Parser" and description "Parses input into AST."

**When** `RenderDOT(spec, &buf)` is called.

**Then:** The Parser node has a `label` attribute containing the component name "Parser" (not the raw node ID).

### JSONRenderer

#### J1: Top-level structure with nodes and edges arrays

**Given** the fixture SpecGraph.

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- Output is valid JSON
- Top-level object has exactly two keys: `"nodes"` and `"edges"`
- Both are arrays
- JSON uses 2-space indentation

#### J2: Project node present

**Given** the fixture SpecGraph with project name "test-project".

**When** `RenderJSON(spec, &buf)` is called.

**Then:** The nodes array contains an entry with `"id": "project"`, `"type": "project"`, `"name": "test-project"`.

#### J3: Synthetic node IDs follow path convention

**Given** module alpha with component Parser (id: 1) and requirement (id: 1).

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- Module node has `"id": "module:alpha"`
- Requirement node has `"id": "module:alpha:req:1"`
- Component node has `"id": "module:alpha:comp:1"`
- Impl_section node has `"id": "module:alpha:impl:1"`
- Data_flow node has `"id": "module:alpha:flow:1"`
- All IDs are globally unique across the entire nodes array

#### J4: Content inlined in component nodes

**Given** Parser component with content `"# Parser\n\nParses input into AST."`.

**When** `RenderJSON(spec, &buf)` is called.

**Then:** The Parser node has a `"content"` field containing the full markdown string, not the file path. The content is the raw markdown as read from disk.

#### J5: All edge types represented

**Given** the fixture SpecGraph with `implements`, `uses`, `describes`, `requires_module`, and `preq_id` relationships.

**When** `RenderJSON(spec, &buf)` is called.

**Then** the edges array contains entries with:
- `{"from": "module:alpha:comp:1", "to": "module:alpha:req:1", "type": "implements"}`
- `{"from": "module:alpha:comp:2", "to": "module:alpha:comp:1", "type": "uses"}`
- `{"from": "module:alpha:impl:1", "to": "module:alpha:comp:1", "type": "describes"}`
- `{"from": "module:beta", "to": "module:alpha", "type": "requires_module"}`
- Requirement nodes with `preq_id` produce edges with `"type": "preq_id"`

#### J6: Data flow uses edges

**Given** alpha's data_flow 1 has `uses: [1, 2]` (both Parser and Builder).

**When** `RenderJSON(spec, &buf)` is called.

**Then** the edges array contains:
- `{"from": "module:alpha:flow:1", "to": "module:alpha:comp:1", "type": "uses"}`
- `{"from": "module:alpha:flow:1", "to": "module:alpha:comp:2", "type": "uses"}`

#### J7: Output is self-contained and parseable by jq

**Given** any valid SpecGraph.

**When** `RenderJSON(spec, &buf)` is called and the output is piped to `jq '.nodes[] | select(.type == "component")'`.

**Then:** jq produces valid JSON output containing only component nodes. This validates that the JSON is well-formed and follows the documented structure.

#### J8: Node count matches spec contents

**Given** the fixture SpecGraph with 1 project + 2 modules + 3 project requirements + (2+1) module requirements + (2+1) components + (1+1) impl_sections + 1 data_flow = 15 total nodes.

**When** `RenderJSON(spec, &buf)` is called.

**Then:** The nodes array has exactly 15 entries. No nodes are duplicated or omitted.

## Edge Cases

### E1: Module with empty requirements array

**Given** a module with `requirements: []` (no requirements).

**When** any renderer is called.

**Then:**
- MarkdownRenderer: omits the requirements subsection for that module (or shows empty section, either is acceptable)
- DOTRenderer: no requirement nodes for that module, no `implements` edges
- JSONRenderer: no requirement nodes for that module in the nodes array

### E2: Component with no uses edges

**Given** a component with `uses: []`.

**When** DOTRenderer is called.

**Then:** No `uses` edges are emitted for that component. The component node still appears.

### E3: Content containing JSON-special characters

**Given** a content leaf file containing `"quotes"`, `\backslashes`, and `{braces}`.

**When** JSONRenderer is called.

**Then:** The `content` field in the JSON output properly escapes these characters (`\"`, `\\`, etc.). The output remains valid JSON.

### E4: Very large spec with many modules

**Given** a spec with 20 modules, each having 10 components, 5 requirements, and 3 data_flows.

**When** any renderer is called.

**Then:** Output is produced without error. No nodes or edges are dropped. For DOTRenderer, all 20 modules appear as subgraphs. For JSONRenderer, the nodes array contains all expected entries.

### E5: Content with deeply nested headings

**Given** a content leaf with headings `#`, `##`, `###`, `####` (4 levels).

**When** MarkdownRenderer is called.

**Then:** Heading adjustment adds the base offset to all levels. If base level is 4, headings become `####`, `#####`, `######`, and beyond `######` (markdown supports at most 6 `#` characters). The renderer must handle the overflow case gracefully (e.g., cap at `######` or use bold text for deeper levels).

### E6: Module name with special characters in DOT

**Given** a module named `data-pipeline` (contains a hyphen).

**When** DOTRenderer is called.

**Then:** The subgraph name and node IDs handle the hyphen correctly. Either the hyphen is replaced with an underscore in identifiers (e.g., `data_pipeline_comp_1`) or the identifier is quoted. The output remains valid DOT syntax.

### E7: Empty spec (project with one module, module has only name)

**Given** a project with one module, where the module has only `name` (all optional arrays omitted).

**When** any renderer is called.

**Then:**
- MarkdownRenderer: produces `# <project>` and `## Module: <name>` with no subsections
- DOTRenderer: produces a digraph with one subgraph containing one module node and no edges
- JSONRenderer: produces nodes array with project node and one module node, empty edges array
