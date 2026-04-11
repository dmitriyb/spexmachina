package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// fixtureGraph builds the test SpecGraph described in test_renderers.md.
func fixtureGraph() *SpecGraph {
	return &SpecGraph{
		Project: schema.Project{
			Name:        "test-project",
			Description: "A test spec",
			Requirements: []schema.Requirement{
				{ID: 1, Type: "functional", Title: "Parse input", Description: "Accept structured input and parse it."},
				{ID: 2, Type: "functional", Title: "Build output", Description: "Build output from parsed input."},
				{ID: 3, Type: "non_functional", Title: "Performance", Description: "Complete within 2 seconds."},
			},
			Milestones: []schema.Milestone{
				{ID: 1, Title: "MVP", Groups: []int{1, 2}},
			},
		},
		Modules: []ModuleGraph{
			{
				Module: schema.Module{ID: 1, Name: "alpha", Path: "alpha", Description: "Alpha module"},
				Spec: schema.ModuleSpec{
					Name:        "alpha",
					Description: "Alpha module description",
					// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
					Requirements: []schema.ModuleRequirement{
						{ID: "aabbccddeeff", Type: "functional", Title: "Parse", PreqID: "112233445566"},
						{ID: "ffeeddccbbaa", Type: "functional", Title: "Build", PreqID: "665544332211"},
					},
					Components: []schema.Component{
						{ID: "aabbccddeeff", Name: "Parser", Description: "Parses input into AST.", Content: "arch_parser.md", Implements: []string{"aabbccddeeff"}},
						{ID: "ffeeddccbbaa", Name: "Builder", Description: "Builds output from AST.", Content: "arch_builder.md", Implements: []string{"ffeeddccbbaa"}, Uses: []string{"aabbccddeeff"}},
					},
					ImplSections: []schema.ImplSection{
						{ID: "aabbccddeeff", Name: "Parsing Implementation", Content: "impl_parsing.md", Describes: []string{"aabbccddeeff"}},
					},
					DataFlows: []schema.DataFlow{
						{ID: "aabbccddeeff", Name: "Build Pipeline", Description: "Parse then build.", Content: "flow_build_pipeline.md", Uses: []string{"aabbccddeeff", "ffeeddccbbaa"}},
					},
				},
				Content: map[string]string{
					"arch_parser.md":         "# Parser\n\nParses input into AST.\n",
					"arch_builder.md":        "# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first.\n",
					"impl_parsing.md":        "# Parsing Implementation\n\nUse recursive descent.\n",
					"flow_build_pipeline.md": "# Build Pipeline\n\nParse then build.\n",
				},
			},
			{
				Module: schema.Module{ID: 2, Name: "beta", Path: "beta", Description: "Beta module", RequiresModule: []int{1}},
				Spec: schema.ModuleSpec{
					Name:        "beta",
					Description: "Beta module description",
					// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
					Requirements: []schema.ModuleRequirement{
						{ID: "aabbccddeeff", Type: "functional", Title: "Consume", PreqID: "112233445566"},
					},
					Components: []schema.Component{
						{ID: "aabbccddeeff", Name: "Consumer", Description: "Consumes built output.", Content: "arch_consumer.md", Implements: []string{"aabbccddeeff"}},
					},
					ImplSections: []schema.ImplSection{
						{ID: "aabbccddeeff", Name: "Consumption Implementation", Content: "impl_consumption.md", Describes: []string{"aabbccddeeff"}},
					},
				},
				Content: map[string]string{
					"arch_consumer.md":     "# Consumer\n\nConsumes built output.\n",
					"impl_consumption.md": "# Consumption Implementation\n\nConsume the output.\n",
				},
			},
		},
	}
}

// ---- MarkdownRenderer tests ----

// M1: Document structure and ordering
func TestFR1_M1_MarkdownStructureAndOrdering(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	sections := []string{
		"# test-project",
		"## Requirements",
		"### Functional",
		"### Non-functional",
		"## Module: alpha",
		"### Architecture",
		"### Implementation",
		"### Data Flows",
		"## Module: beta",
	}

	lastIdx := -1
	for _, s := range sections {
		idx := strings.Index(out, s)
		if idx < 0 {
			t.Fatalf("missing section %q in output:\n%s", s, out)
		}
		if idx <= lastIdx {
			t.Fatalf("section %q should appear after previous section (idx %d <= %d)", s, idx, lastIdx)
		}
		lastIdx = idx
	}
}

// M2: Content inlining with heading adjustment
func TestFR1_M2_HeadingAdjustment(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Builder content "# Builder" under Module > Architecture should be adjusted to #### Builder
	if !strings.Contains(out, "#### Builder") {
		t.Fatalf("expected '#### Builder' heading, not found in:\n%s", out)
	}
	// "## Algorithm" should become ##### Algorithm
	if !strings.Contains(out, "##### Algorithm") {
		t.Fatalf("expected '##### Algorithm' heading, not found in:\n%s", out)
	}
	// Body text preserved
	if !strings.Contains(out, "Walk the tree depth-first.") {
		t.Fatal("body text should be preserved verbatim")
	}
}

// M3: Requirements formatting
func TestFR1_M3_RequirementsFormatting(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "FR1: Parse input") {
		t.Fatalf("expected 'FR1: Parse input' in output:\n%s", out)
	}
	if !strings.Contains(out, "NFR3: Performance") {
		t.Fatalf("expected 'NFR3: Performance' in output:\n%s", out)
	}
}

// M4: Module ordering matches declaration order
func TestFR1_M4_ModuleOrdering(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	alphaIdx := strings.Index(out, "## Module: alpha")
	betaIdx := strings.Index(out, "## Module: beta")
	if alphaIdx < 0 || betaIdx < 0 {
		t.Fatal("both module sections should be present")
	}
	if alphaIdx >= betaIdx {
		t.Fatal("alpha should appear before beta")
	}
}

// M5: Output is pure markdown with no front matter
func TestFR1_M5_PureMarkdown(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if strings.HasPrefix(out, "---") {
		t.Fatal("output should not have YAML front matter")
	}
	if strings.Contains(out, "<html") || strings.Contains(out, "<div") {
		t.Fatal("output should not contain HTML tags")
	}
	if !strings.HasPrefix(out, "# ") {
		t.Fatalf("output should start with '# ', got: %.40s", out)
	}
}

// ---- DOTRenderer tests ----

// D1: Valid DOT syntax with digraph wrapper
func TestFR2_D1_ValidDOTSyntax(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "digraph spec {") {
		t.Fatalf("should start with 'digraph spec {', got: %.40s", out)
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "}") {
		t.Fatal("should end with '}'")
	}
	if !strings.Contains(out, "rankdir=LR") {
		t.Fatal("should contain rankdir=LR")
	}
}

// D2: Module subgraphs as clusters
func TestFR2_D2_ModuleSubgraphs(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "subgraph cluster_alpha {") {
		t.Fatal("missing subgraph cluster_alpha")
	}
	if !strings.Contains(out, "subgraph cluster_beta {") {
		t.Fatal("missing subgraph cluster_beta")
	}
}

// D3: Node shapes match spec node types
func TestFR2_D3_NodeShapes(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	tests := []struct {
		nodePrefix string
		shape      string
	}{
		{"alpha_req_", "shape=box"},
		{"alpha_comp_", "shape=component"},
		{"alpha_impl_", "shape=note"},
		{"alpha_flow_", "shape=ellipse"},
	}
	for _, tt := range tests {
		// Find the line containing the node
		found := false
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, tt.nodePrefix) && strings.Contains(line, tt.shape) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("node with prefix %q should have %s", tt.nodePrefix, tt.shape)
		}
	}
}

// D4: Edge types rendered with correct styles
func TestFR2_D4_EdgeStyles(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Builder implements req 2
	if !strings.Contains(out, "alpha_comp_2") || !strings.Contains(out, "alpha_req_2") {
		t.Fatal("should have edge from Builder to req 2")
	}
	if !strings.Contains(out, "implements") {
		t.Fatal("should have implements label")
	}
	// Builder uses Parser
	if !strings.Contains(out, `"uses"`) || !strings.Contains(out, "dotted") {
		t.Fatal("uses edge should exist with dotted style")
	}
}

// D5: Cross-module edges rendered
func TestFR2_D5_CrossModuleEdges(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "requires_module") {
		t.Fatal("should have requires_module edge")
	}
}

// D6: Node IDs are valid DOT identifiers
func TestFR2_D6_ValidNodeIDs(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	for _, id := range []string{"alpha_comp_1", "alpha_comp_2", "alpha_req_1", "beta_comp_1", "beta_req_1"} {
		if !strings.Contains(out, id) {
			t.Errorf("missing node ID %q", id)
		}
	}
}

// D7: Node labels are human-readable
func TestFR2_D7_NodeLabels(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"Parser"`) {
		t.Fatal("Parser node should have human-readable label")
	}
}

// ---- JSONRenderer tests ----

// J1: Top-level structure with nodes and edges arrays
func TestFR3_J1_TopLevelStructure(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := result["nodes"]; !ok {
		t.Fatal("missing 'nodes' key")
	}
	if _, ok := result["edges"]; !ok {
		t.Fatal("missing 'edges' key")
	}
	if len(result) != 2 {
		t.Fatalf("want exactly 2 top-level keys, got %d", len(result))
	}

	// Check 2-space indentation
	out := buf.String()
	if !strings.Contains(out, "  ") {
		t.Fatal("JSON should use 2-space indentation")
	}
}

// J2: Project node present
func TestFR3_J2_ProjectNode(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	found := false
	for _, n := range result.Nodes {
		if n.ID == "project" && n.Type == "project" && n.Name == "test-project" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("project node not found")
	}
}

// J3: Synthetic node IDs follow path convention
func TestFR3_J3_SyntheticNodeIDs(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	expectedIDs := map[string]bool{
		"project":            true,
		"module:alpha":       true,
		"module:beta":        true,
		"module:alpha:req:1": true,
		"module:alpha:req:2": true,
		"module:alpha:comp:1": true,
		"module:alpha:comp:2": true,
		"module:alpha:impl:1": true,
		"module:alpha:flow:1": true,
		"module:beta:req:1":   true,
		"module:beta:comp:1":  true,
		"module:beta:impl:1":  true,
	}

	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		if nodeIDs[n.ID] {
			t.Fatalf("duplicate node ID: %s", n.ID)
		}
		nodeIDs[n.ID] = true
	}

	for id := range expectedIDs {
		if !nodeIDs[id] {
			t.Errorf("missing node ID %q", id)
		}
	}
}

// J4: Content inlined in component nodes
func TestFR3_J4_ContentInlined(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	for _, n := range result.Nodes {
		if n.ID == "module:alpha:comp:1" {
			if !strings.Contains(n.Content, "Parses input into AST.") {
				t.Fatalf("Parser node should have inlined content, got: %q", n.Content)
			}
			return
		}
	}
	t.Fatal("Parser component node not found")
}

// J5: All edge types represented
func TestFR3_J5_AllEdgeTypes(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Edges []GraphEdge `json:"edges"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	expectedEdges := []GraphEdge{
		{From: "module:alpha:comp:1", To: "module:alpha:req:1", Type: "implements"},
		{From: "module:alpha:comp:2", To: "module:alpha:comp:1", Type: "uses"},
		{From: "module:alpha:impl:1", To: "module:alpha:comp:1", Type: "describes"},
		{From: "module:beta", To: "module:alpha", Type: "requires_module"},
	}

	for _, want := range expectedEdges {
		found := false
		for _, e := range result.Edges {
			if e.From == want.From && e.To == want.To && e.Type == want.Type {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing edge: %+v", want)
		}
	}

	// Check preq_id edges
	preqFound := false
	for _, e := range result.Edges {
		if e.Type == "preq_id" {
			preqFound = true
			break
		}
	}
	if !preqFound {
		t.Error("missing preq_id edge type")
	}
}

// J6: Data flow uses edges
func TestFR3_J6_DataFlowUsesEdges(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Edges []GraphEdge `json:"edges"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	expected := []GraphEdge{
		{From: "module:alpha:flow:1", To: "module:alpha:comp:1", Type: "uses"},
		{From: "module:alpha:flow:1", To: "module:alpha:comp:2", Type: "uses"},
	}
	for _, want := range expected {
		found := false
		for _, e := range result.Edges {
			if e == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing data flow edge: %+v", want)
		}
	}
}

// J8: Node count matches spec contents
func TestFR3_J8_NodeCount(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	// 1 project + 2 modules + 3 proj reqs + (2+1) mod reqs + (2+1) comps + (1+1) impls + 1 flow = 15
	if len(result.Nodes) != 15 {
		t.Fatalf("want 15 nodes, got %d", len(result.Nodes))
	}
}

// ---- Edge cases ----

// E1: Module with empty requirements array
func TestFR1_E1_Renderer_EmptyRequirements(t *testing.T) {
	// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	spec := &SpecGraph{
		Project: schema.Project{Name: "empty-req", Modules: []schema.Module{{ID: 1, Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: 1, Name: "m", Path: "m"},
			Spec:    schema.ModuleSpec{Name: "m"},
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("JSON render error: %v", err)
	}
	var result struct {
		Nodes []GraphNode `json:"nodes"`
	}
	json.Unmarshal(buf.Bytes(), &result)
	for _, n := range result.Nodes {
		if n.Type == "requirement" && n.Module == "m" {
			t.Fatal("should have no requirement nodes for module m")
		}
	}

	buf.Reset()
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("DOT render error: %v", err)
	}
	// No implements edges for empty module
	if strings.Contains(buf.String(), "m_req_") {
		t.Fatal("should have no requirement nodes in DOT")
	}
}

// E3: Content containing JSON-special characters
func TestFR3_E3_JSONSpecialChars(t *testing.T) {
	// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	spec := &SpecGraph{
		Project: schema.Project{Name: "special", Modules: []schema.Module{{ID: 1, Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: 1, Name: "m", Path: "m"},
			Spec: schema.ModuleSpec{
				Name:       "m",
				Components: []schema.Component{{ID: "aabbccddeeff", Name: "C", Content: "arch.md"}},
			},
			Content: map[string]string{
				"arch.md": `"quotes" and \backslashes and {braces}`,
			},
		}},
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output is valid JSON
	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\n%s", err, buf.String())
	}
}

// E5: Content with deeply nested headings
func TestFR1_E5_DeeplyNestedHeadings(t *testing.T) {
	// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	spec := &SpecGraph{
		Project: schema.Project{Name: "deep", Modules: []schema.Module{{ID: 1, Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: 1, Name: "m", Path: "m"},
			Spec: schema.ModuleSpec{
				Name:       "m",
				Components: []schema.Component{{ID: "aabbccddeeff", Name: "C", Content: "arch.md"}},
			},
			Content: map[string]string{
				"arch.md": "# Level 1\n\n## Level 2\n\n### Level 3\n\n#### Level 4\n",
			},
		}},
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// At base level 4 (project > module > architecture > component):
	// # -> ####, ## -> #####, ### -> ######, #### -> ###### (capped)
	if !strings.Contains(out, "#### Level 1") {
		t.Fatalf("# should become ####, output:\n%s", out)
	}
	if !strings.Contains(out, "##### Level 2") {
		t.Fatalf("## should become #####, output:\n%s", out)
	}
	if !strings.Contains(out, "###### Level 3") {
		t.Fatalf("### should become ######, output:\n%s", out)
	}
	// #### at base 4 would be 7 hashes, cap at 6
	if !strings.Contains(out, "###### Level 4") {
		t.Fatalf("#### should cap at ######, output:\n%s", out)
	}
}

// E6: Module name with special characters in DOT
func TestFR2_E6_HyphenatedModuleName(t *testing.T) {
	// TODO(bead:spexmachina-rjg): fix after spexmachina-e8t changed module IDs from int to identity hash strings
	spec := &SpecGraph{
		Project: schema.Project{Name: "test", Modules: []schema.Module{{ID: 1, Name: "data-pipeline", Path: "data-pipeline"}}},
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: 1, Name: "data-pipeline", Path: "data-pipeline"},
			Spec: schema.ModuleSpec{
				Name:       "data-pipeline",
				Components: []schema.Component{{ID: "aabbccddeeff", Name: "Ingest"}},
			},
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Hyphens should be replaced with underscores in identifiers
	if !strings.Contains(out, "cluster_data_pipeline") {
		t.Fatalf("hyphen should be replaced in cluster name, got:\n%s", out)
	}
	if !strings.Contains(out, "data_pipeline_comp_1") {
		t.Fatalf("hyphen should be replaced in node ID, got:\n%s", out)
	}
}

// E7: Empty spec (project with module that has only name)
func TestFR1_E7_EmptySpec(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{Name: "empty", Modules: []schema.Module{{ID: 1, Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: 1, Name: "m", Path: "m"},
			Spec:    schema.ModuleSpec{Name: "m"},
			Content: map[string]string{},
		}},
	}

	// Markdown
	var mdBuf bytes.Buffer
	if err := RenderMarkdown(spec, &mdBuf); err != nil {
		t.Fatalf("markdown error: %v", err)
	}
	if !strings.Contains(mdBuf.String(), "# empty") {
		t.Fatal("markdown should have project heading")
	}
	if !strings.Contains(mdBuf.String(), "## Module: m") {
		t.Fatal("markdown should have module heading")
	}

	// DOT
	var dotBuf bytes.Buffer
	if err := RenderDOT(spec, &dotBuf); err != nil {
		t.Fatalf("dot error: %v", err)
	}
	if !strings.Contains(dotBuf.String(), "digraph spec {") {
		t.Fatal("DOT should have digraph wrapper")
	}

	// JSON
	var jsonBuf bytes.Buffer
	if err := RenderJSON(spec, &jsonBuf); err != nil {
		t.Fatalf("json error: %v", err)
	}
	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	json.Unmarshal(jsonBuf.Bytes(), &result)
	if len(result.Nodes) != 2 { // project + 1 module
		t.Fatalf("want 2 nodes (project + module), got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Fatalf("want 0 edges, got %d", len(result.Edges))
	}
}
