# Renderer Tests

## Setup

All renderer scenarios operate on a pre-built spec graph (output of `ReadSpec`). Every node in it is keyed by its declared 12-char identity hash, and the fixture graph contains:

- **Project**: name "test-project", description "A test spec", 2 functional requirements (`112233445566` "Parse input", `665544332211` "Build output") and 1 non-functional requirement (`778899aabbcc` "Performance")
- **Module alpha** (`111111111111`): 2 requirements (`a1a1a1a1a1a1` "Parse", `a2a2a2a2a2a2` "Build"), 2 components (Parser `c1c1c1c1c1c1`, Builder `c2c2c2c2c2c2`), 1 data_flow (`f1f1f1f1f1f1`)
  - Parser: `implements: [a1a1a1a1a1a1]`, `uses: []`, content: `"# Parser\n\nParses input into AST."`
  - Builder: `implements: [a2a2a2a2a2a2]`, `uses: [c1c1c1c1c1c1]`, content: `"# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first."`
  - Data_flow `f1f1f1f1f1f1`: `uses: [c1c1c1c1c1c1, c2c2c2c2c2c2]`, content: `"# Build Pipeline\n\nParse then build."`
- **Module beta** (`222222222222`, `requires_module: [111111111111]`): 1 requirement (`b1b1b1b1b1b1` "Consume"), 1 component (Consumer `c3c3c3c3c3c3`)
  - Consumer: `implements: [b1b1b1b1b1b1]`, `uses: []`, content: `"# Consumer\n\nConsumes built output."`

None of those IDs is the identity hash the node's own identity string would produce. That is deliberate: it is what lets a scenario tell a renderer that copies declared IDs from one that recomputes them.

A second **surface fixture** serves the api and slim scenarios. It is one module `gamma` whose every ID *is* the identity hash of its identity string, holding 1 project requirement ("Expose a surface"), 1 module requirement ("Serve requests"), 1 component (Server), 1 data_flow ("Request path"), 1 test_section ("Server tests", describing Server) and 2 apis — `spex serve` (group `cli`, description "Start the server.", `provided_by` Server) and `GET /v1/specs/{id}` (group `http`, no description, no `provided_by`).

Each renderer writes to an in-memory buffer so output can be inspected as a string.

## Scenarios

### MarkdownRenderer

#### M1: Document structure and ordering

**Given** the fixture SpecGraph.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** the output contains these sections in order:
1. `# test-project` (project heading)
2. Project description text
3. `## Requirements` with subsections `### Functional` (FR1, FR2) and `### Non-functional` (NFR3 — project non-functional numbering continues past the functional count)
4. `## Module: alpha` with module description
5. Alpha's requirements section
6. `### Architecture` with component subsections for Parser and Builder
7. `### Data Flows` with data_flow subsections
8. `## Module: beta` following the same structure

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

**Given** project.json declares alpha (`111111111111`) before beta (`222222222222`).

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** `## Module: alpha` appears before `## Module: beta` in the output.

#### M5: Output is pure markdown with no front matter

**Given** any valid SpecGraph.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:**
- Output does not start with `---` (no YAML front matter)
- Output does not contain HTML tags
- Output starts with `# ` (the project heading)

#### M6: Module requirement numbering is module-scoped

**Given** module alpha's two functional requirements, `a1a1a1a1a1a1` "Parse" and `a2a2a2a2a2a2` "Build".

**When** `RenderMarkdown(spec, &buf)` is called.

**Then** they are listed as `FR1: Parse` and `FR2: Build` — numbered within the module, never prefixed with the requirement's identity hash (no `FRaabbccddeeff`).

#### M7: APIs render one line each, between requirements and architecture

**Given** the surface fixture: module gamma declaring `spex serve` (group cli, description "Start the server.") and then `GET /v1/specs/{id}` (group http).

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:**
- A `### APIs` heading appears, carrying `` - `spex serve` (cli) — Start the server. `` and `` - `GET /v1/specs/{id}` (http) ``, in declaration order
- The headings `### Requirements`, `### APIs`, `### Architecture` and `### Data Flows` appear in that order, by byte offset
- Rendering the base fixture, whose modules declare no apis, emits no `### APIs` heading at all

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

**Given** the fixture SpecGraph with requirements, components, and data_flows.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- Requirement nodes have `shape=box`
- Component nodes have `shape=component`
- Data_flow nodes have `shape=ellipse`

#### D4: Edge types rendered with correct styles

**Given** Builder `c2c2c2c2c2c2`, which implements the alpha Build requirement `a2a2a2a2a2a2` and uses Parser `c1c1c1c1c1c1`.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- An edge exists from `c2c2c2c2c2c2` to `a2a2a2a2a2a2` labeled `"implements"`
- An edge exists from `c2c2c2c2c2c2` to `c1c1c1c1c1c1` labeled `"uses"`
- The `implements` edge uses solid style
- The `uses` edge uses dotted style

#### D5: Cross-module edges rendered

**Given** beta has `requires_module: [111111111111]` (depends on alpha).

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- The output carries the string `requires_module`, which is the label the module-dependency edge is emitted with
- The endpoints are the two declared module hashes; D6 and D10 are what pin that every emitted endpoint is a bare declared hash

#### D6: Node IDs are bare identity hashes, and are quoted

**Given** any valid SpecGraph.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- Every statement whose subject is a node — a declaration or an edge, either endpoint — names it by the node's declared 12-char hex identity hash, quoted and alone
- No composite `<module>_<type>_<id>` identifier survives anywhere in the output: none of `alpha_comp_`, `alpha_req_`, `alpha_flow_`, `beta_comp_`, `beta_req_` or `preq_112233445566` appears
- The quoting is required rather than cosmetic: an identity hash may begin with a digit, and unquoted such a token is not a legal DOT identifier
- The scan finds at least one node ID to inspect, so a renderer emitting no nodes cannot pass by vacuity

#### D7: Node labels are human-readable

**Given** component Parser with name "Parser" and description "Parses input into AST."

**When** `RenderDOT(spec, &buf)` is called.

**Then:** The Parser node has a `label` attribute containing the component name "Parser" (not the raw node ID).

#### D8: Every node is declared under the ID `spex hash-id` prints

**Given** the surface fixture, whose every node ID is the identity hash of that node's own identity string.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- The module, the project requirement, the module requirement, the component, the data_flow and both apis are each declared under exactly that hash — seven declarations, no more and no fewer
- The `implements` edge joins two such hashes and nothing else
- Declarations are matched on the whole line, not by substring: an edge's target is also followed by `[label=`, so a substring match would accept a composite declaration as long as some edge still mentioned the bare hash

#### D9: API nodes and their provided_by edges

**Given** the surface fixture's two apis, `spex serve` (provided_by the Server component) and `GET /v1/specs/{id}`.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- The `spex serve` node is declared with `shape=cds` and labeled with the api name
- The `GET /v1/specs/{id}` node is declared too, labeled with the whole invocation string
- An edge from the `spex serve` node to the Server component node is labeled `"provided_by"`

#### D10: Declared IDs are emitted, never recomputed

**Given** the base fixture, whose IDs are deliberately not the identity hashes its names would produce.

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- All 12 nodes — 3 project requirements, 2 modules, 3 module requirements, 3 components and 1 data_flow — are declared under the ID the fixture declared for them
- The declaration count is exactly 12: nothing is declared twice and nothing is missing
- Only declarations are collected, never edge endpoints; that distinction is the point of the scenario

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

**Given** module alpha with component Parser `c1c1c1c1c1c1` and requirement `a1a1a1a1a1a1`.

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- Module node has `"id": "module:alpha"`
- Requirement node has `"id": "module:alpha:req:a1a1a1a1a1a1"`
- Component node has `"id": "module:alpha:comp:c1c1c1c1c1c1"`
- Data_flow node has `"id": "module:alpha:flow:f1f1f1f1f1f1"`
- The synthetic prefix carries the module name and the abbreviated node kind; the tail is the declared identity hash, copied
- All IDs are globally unique across the entire nodes array

#### J4: Content inlined in component nodes

**Given** Parser component with content `"# Parser\n\nParses input into AST."`.

**When** `RenderJSON(spec, &buf)` is called.

**Then:** The Parser node has a `"content"` field containing the full markdown string, not the file path. The content is the raw markdown as read from disk.

#### J5: Edge types represented

**Given** the fixture SpecGraph with `implements`, `uses`, `requires_module`, and `preq_id` relationships.

**When** `RenderJSON(spec, &buf)` is called.

**Then** the edges array contains entries with:
- `{"from": "module:alpha:comp:c1c1c1c1c1c1", "to": "module:alpha:req:a1a1a1a1a1a1", "type": "implements"}`
- `{"from": "module:alpha:comp:c2c2c2c2c2c2", "to": "module:alpha:comp:c1c1c1c1c1c1", "type": "uses"}`
- `{"from": "module:beta", "to": "module:alpha", "type": "requires_module"}`
- Requirement nodes with `preq_id` produce edges with `"type": "preq_id"`
- No `describes` edge appears at all: that edge is emitted from a test_section's `describes` array, and the base fixture declares no test_sections

#### J6: Data flow uses edges

**Given** alpha's data_flow `f1f1f1f1f1f1` has `uses: [c1c1c1c1c1c1, c2c2c2c2c2c2]` (both Parser and Builder).

**When** `RenderJSON(spec, &buf)` is called.

**Then** the edges array contains:
- `{"from": "module:alpha:flow:f1f1f1f1f1f1", "to": "module:alpha:comp:c1c1c1c1c1c1", "type": "uses"}`
- `{"from": "module:alpha:flow:f1f1f1f1f1f1", "to": "module:alpha:comp:c2c2c2c2c2c2", "type": "uses"}`

#### J7: Output is self-contained and parseable

**Given** any valid SpecGraph.

**When** `RenderJSON(spec, &buf)` is called and the output is decoded generically.

**Then:** the output decodes without error into a top-level object of two array keys, so a downstream filter such as `jq '.nodes[] | select(.type == "component")'` can select component nodes from it. J1 carries this assertion; no scenario shells out to `jq`.

#### J8: Node count matches spec contents

**Given** the fixture SpecGraph with 1 project + 2 modules + 3 project requirements + (2+1) module requirements + (2+1) components + 1 data_flow = 13 total nodes.

**When** `RenderJSON(spec, &buf)` is called.

**Then:** The nodes array has exactly 13 entries. No nodes are duplicated or omitted.

#### J9: `--slim` emits nodes only, with four keys and nothing else

**Given** the surface fixture.

**When** `RenderJSONSlim(spec, &buf)` is called.

**Then:**
- The output is an object with a `nodes` array and no `edges` key at all
- The array is non-empty, and every node carries a non-empty `id`, `type` and `name`
- No node carries a key outside `{id, type, name, module}`
- Neither `content` nor `description` appears anywhere, and none of the text they would have carried — the component and data_flow descriptions, the api description, the inlined content leaves — survives in the raw bytes

#### J10: Slim IDs are bare identity hashes

**Given** the surface fixture, whose every declared ID is the identity hash of that node's identity string.

**When** `RenderJSONSlim(spec, &buf)` is called.

**Then:**
- Every `id` matches `^[0-9a-f]{12}$` and carries no `module:…:` synthetic prefix — no `:` at all
- Each hash maps to the node type it identifies: the module, the project requirement, the module requirement, the component, the data_flow, the test_section and both apis — eight nodes, no more and no fewer

#### J11: Test sections and apis are slim nodes

**Given** the surface fixture's test_section "Server tests" and its apis `spex serve` and `GET /v1/specs/{id}`.

**When** `RenderJSONSlim(spec, &buf)` is called.

**Then:**
- Exactly one `test_section` node, named "Server tests"
- Exactly two `api` nodes, in declaration order
- Every node of either type carries `"module": "gamma"`

#### J12: Slim output is one compact line

**Given** the surface fixture.

**When** `RenderJSONSlim(spec, &buf)` is called.

**Then** the body carries no newline once a trailing one is trimmed, and is not pretty-printed — no `", "` separator appears. It is a lookup table, not a document.

#### J13: Test section nodes and their describes edges in the full graph

**Given** the surface fixture's test_section "Server tests", which describes the Server component.

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- A node with id `module:gamma:test:<test_section hash>` exists, with `"type": "test_section"`, `"name": "Server tests"` and `"module": "gamma"`
- Its `content` carries the inlined text of the test leaf
- An edge from that node to `module:gamma:comp:<component hash>` has `"type": "describes"`

#### J14: API nodes and their provided_by edges in the full graph

**Given** the surface fixture's api `spex serve` — description "Start the server.", group "cli", `provided_by` the Server component.

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- A node with id `module:gamma:api:<api hash>` exists, with `"type": "api"`, `"name": "spex serve"` and `"module": "gamma"`
- It carries the declared description and the declared group
- It carries no `content`: an api has no content leaf to inline
- An edge from that node to `module:gamma:comp:<component hash>` has `"type": "provided_by"`

#### J15: Slim emits declared IDs, in declaration order, and omits the project root

**Given** the base fixture with one informational section `aaaabbbbcccc` "notes" added to the project.

**When** `RenderJSONSlim(spec, &buf)` is called.

**Then:**
- The node list is exactly, in this order: the 3 project requirements, the section, module alpha, its 2 requirements, its 2 components, its data_flow, module beta, its requirement, its component — 13 nodes, each matching id, type, name and module
- Every ID is the declared one, copied verbatim
- The project root is not among them: the full graph's `"id": "project"` node has no identity hash, so the slim view has no counterpart for it
- The fixture guards itself: for each of the five checked nodes — the first project requirement, module alpha, one alpha requirement, one alpha component and the alpha data_flow — the declared ID differs from the identity hash its identity string would produce, so a renderer that recomputed IDs would fail this scenario rather than pass it by coincidence

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

**Then:** The four headings render as `####`, `#####`, `######` and `######`. The first three take the full three-level shift; the fourth would need seven `#` characters, and markdown has only six, so it is capped at `######` rather than deepened or re-expressed as bold text. Two originally distinct levels therefore collapse onto the same rendered level, and this scenario pins that cap as the behaviour rather than as one acceptable way of handling the overflow.

### E6: Module name with special characters in DOT

**Given** a module named `data-pipeline` with identity hash `datapipe0001`, holding one component `Ingest` with identity hash `aabbccddeeff`.

**When** DOTRenderer is called.

**Then:**
- The cluster name substitutes the hyphen: `cluster_data_pipeline` appears in the output
- The hyphen reaches no node ID, because a node ID is the declared identity hash and is never derived from the module name: the module node is `"datapipe0001" [label="data-pipeline"` and the component node is `"aabbccddeeff" [label="Ingest"`
- The readable, hyphenated name survives as the cluster label and as the module node's label, never as an identifier
- The output remains valid DOT syntax

### E7: Empty spec (project with one module, module has only name)

**Given** a project with one module, where the module has only `name` (all optional arrays omitted).

**When** any renderer is called.

**Then:**
- MarkdownRenderer: produces `# <project>` and `## Module: <name>` with no subsections
- DOTRenderer: produces a digraph with one subgraph containing one module node and no edges
- JSONRenderer: produces nodes array with project node and one module node, empty edges array

## Sections Rendering

### SM1: Markdown renders sections after requirements

**Given** a spec with a `sections` array containing one coupled section named "delivery" with versioning, artifacts, and channels content. The coupled module "delivery" exists.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:**
- A `## Sections` heading appears after project requirements and before module sections
- Under it, `### delivery` heading with the section type noted (e.g., "(coupled)")
- The section's freeform content is rendered: versioning scheme, artifacts list, channels list
- Content from the coupled module (components, data_flows) is NOT duplicated here — it appears in the module's own `## Module: delivery` section

### SM2: DOT renders section nodes and coupling edges

**Given** a spec with a coupled section "delivery" (`section00001`) and a delivery module (`delivery0001`).

**When** `RenderDOT(spec, &buf)` is called.

**Then:**
- The section is declared as `"section00001"` — its declared identity hash, quoted, never a `section_<n>` composite — carrying `shape=tab` and the label "delivery"
- The coupling relationship is the edge `"section00001" -> "delivery0001"`, labelled `"coupled"`, joining two declared hashes and nothing else
- That declaration appears before the first `subgraph cluster_`, which is what places the section outside every module subgraph rather than inside one

### SM3: JSON includes section nodes and coupling edges

**Given** a spec with a coupled "delivery" section.

**When** `RenderJSON(spec, &buf)` is called.

**Then:**
- The nodes array contains an entry with `"id": "section:delivery"`, `"type": "section"`, `"name": "delivery"`
- The section node includes the freeform content fields (versioning, artifacts, channels)
- The edges array contains `{"from": "section:delivery", "to": "module:delivery", "type": "coupled"}`

### SM4: Multiple sections rendered in declaration order

**Given** a spec with two sections: "delivery" (`section00001`) and "performance" (`section00002`).

**When** any renderer is called.

**Then:** Sections appear in declaration order (delivery before performance). The ordering follows the `sections` array order in project.json.

### SM5: Non-coupled section rendered without module link

**Given** a spec with a section `{ id: "section00001", name: "notes", type: "informational" }` and no module named "notes".

**When** any renderer is called.

**Then:**
- MarkdownRenderer: renders the section heading and any content fields, no module cross-reference
- DOTRenderer: section node exists with no coupling edge
- JSONRenderer: section node exists with no coupling edge

### SM6: Spec with no sections array omits sections heading

**Given** a spec with no `sections` field in project.json.

**When** `RenderMarkdown(spec, &buf)` is called.

**Then:** No `## Sections` heading appears. The output is identical to current behavior for specs without sections.
