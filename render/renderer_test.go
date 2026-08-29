package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// nodesFromJSON derives the profile-generic Node list a hand-built fixture
// would get from ReadSpec, by round-tripping v (a schema.Project or
// schema.ModuleSpec) through JSON and the same nodesForScope logic
// ReadSpec calls. This keeps a fixture's generic Nodes/ProjectNodes always
// consistent with its fixed schema fields without hand-duplicating every
// edge map.
func nodesFromJSON(v any, scope, moduleName string) []Node {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	raw, err := rawTopLevelFields(data)
	if err != nil {
		panic(err)
	}
	nodes, err := nodesForScope(raw, schema.DefaultProfile(), scope, moduleName)
	if err != nil {
		panic(err)
	}
	return nodes
}

// fixtureGraph builds the test SpecGraph described in test_renderers.md.
//
// Identity hashes are globally unique across node types, as they are in a real
// spec (the identity string embeds the node type), so that renderers keying on
// the bare hash cannot conflate a requirement with a component.
func fixtureGraph() *SpecGraph {
	proj := schema.Project{
		Name:        "test-project",
		Description: "A test spec",
		Requirements: []schema.Requirement{
			{ID: "112233445566", Type: "functional", Title: "Parse input", Description: "Accept structured input and parse it."},
			{ID: "665544332211", Type: "functional", Title: "Build output", Description: "Build output from parsed input."},
			{ID: "778899aabbcc", Type: "non_functional", Title: "Performance", Description: "Complete within 2 seconds."},
		},
	}
	alphaSpec := schema.ModuleSpec{
		Name:        "alpha",
		Description: "Alpha module description",
		Requirements: []schema.ModuleRequirement{
			{ID: "a1a1a1a1a1a1", Type: "functional", Title: "Parse", PreqID: "112233445566"},
			{ID: "a2a2a2a2a2a2", Type: "functional", Title: "Build", PreqID: "665544332211"},
		},
		Components: []schema.Component{
			{ID: "c1c1c1c1c1c1", Name: "Parser", Description: "Parses input into AST.", Content: "arch_parser.md", Implements: []string{"a1a1a1a1a1a1"}},
			{ID: "c2c2c2c2c2c2", Name: "Builder", Description: "Builds output from AST.", Content: "arch_builder.md", Implements: []string{"a2a2a2a2a2a2"}, Uses: []string{"c1c1c1c1c1c1"}},
		},
		DataFlows: []schema.DataFlow{
			{ID: "f1f1f1f1f1f1", Name: "Build Pipeline", Description: "Parse then build.", Content: "flow_build_pipeline.md", Uses: []string{"c1c1c1c1c1c1", "c2c2c2c2c2c2"}},
		},
	}
	betaSpec := schema.ModuleSpec{
		Name:        "beta",
		Description: "Beta module description",
		Requirements: []schema.ModuleRequirement{
			{ID: "b1b1b1b1b1b1", Type: "functional", Title: "Consume", PreqID: "112233445566"},
		},
		Components: []schema.Component{
			{ID: "c3c3c3c3c3c3", Name: "Consumer", Description: "Consumes built output.", Content: "arch_consumer.md", Implements: []string{"b1b1b1b1b1b1"}},
		},
	}

	return &SpecGraph{
		Project:      proj,
		Profile:      schema.DefaultProfile(),
		ProjectNodes: nodesFromJSON(proj, "project", ""),
		Modules: []ModuleGraph{
			{
				Module: schema.Module{ID: "111111111111", Name: "alpha", Path: "alpha", Description: "Alpha module"},
				Spec:   alphaSpec,
				Nodes:  nodesFromJSON(alphaSpec, "module", "alpha"),
				Content: map[string]string{
					"arch_parser.md":         "# Parser\n\nParses input into AST.\n",
					"arch_builder.md":        "# Builder\n\nBuilds output from AST.\n\n## Algorithm\n\nWalk the tree depth-first.\n",
					"flow_build_pipeline.md": "# Build Pipeline\n\nParse then build.\n",
				},
			},
			{
				Module: schema.Module{ID: "222222222222", Name: "beta", Path: "beta", Description: "Beta module", RequiresModule: []string{"111111111111"}},
				Spec:   betaSpec,
				Nodes:  nodesFromJSON(betaSpec, "module", "beta"),
				Content: map[string]string{
					"arch_consumer.md": "# Consumer\n\nConsumes built output.\n",
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

// M6: Module requirements use numbered prefixes (FR1, FR2, NFR1)
func TestFR1_M6_ModuleRequirementsNumbered(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Alpha has two functional module requirements: Parse, Build.
	// They should be enumerated as FR1, FR2 within the module — not "FRaabbccddeeff".
	if !strings.Contains(out, "FR1: Parse") {
		t.Errorf("expected 'FR1: Parse' for alpha module requirement, got:\n%s", out)
	}
	if !strings.Contains(out, "FR2: Build") {
		t.Errorf("expected 'FR2: Build' for alpha module requirement, got:\n%s", out)
	}
	if strings.Contains(out, "FRaabbccddeeff") || strings.Contains(out, "FRffeeddccbbaa") {
		t.Errorf("module requirement prefix should not include raw identity hash, got:\n%s", out)
	}
}

// SM1: Markdown renders sections after requirements
func TestFR1_SM1_MarkdownSections(t *testing.T) {
	proj := schema.Project{
		Name:        "delivery-test",
		Description: "Test project with sections",
		Requirements: []schema.Requirement{
			{ID: "112233445566", Type: "functional", Title: "Deploy", Description: "Ship the thing."},
		},
		Modules: []schema.Module{{ID: "delivery0001", Name: "delivery", Path: "delivery"}},
		Sections: []schema.Section{{
			ID:   "section00001",
			Name: "delivery",
			Type: "coupled",
			Raw: json.RawMessage(`{
				"id": "section00001",
				"name": "delivery",
				"type": "coupled",
				"versioning": {"scheme": "semver", "source": "git-tag"},
				"artifacts": ["app-binary", "cli"],
				"channels": ["stable", "edge"]
			}`),
		}},
	}
	deliverySpec := schema.ModuleSpec{Name: "delivery", Description: "Delivery module"}
	spec := &SpecGraph{
		Project:      proj,
		Profile:      schema.DefaultProfile(),
		ProjectNodes: nodesFromJSON(proj, "project", ""),
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "delivery0001", Name: "delivery", Path: "delivery"},
			Spec:    deliverySpec,
			Nodes:   nodesFromJSON(deliverySpec, "module", "delivery"),
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Sections heading appears after requirements and before modules.
	reqIdx := strings.Index(out, "## Requirements")
	secIdx := strings.Index(out, "## Sections")
	modIdx := strings.Index(out, "## Module: delivery")
	if reqIdx < 0 || secIdx < 0 || modIdx < 0 {
		t.Fatalf("missing expected headings, got:\n%s", out)
	}
	if !(reqIdx < secIdx && secIdx < modIdx) {
		t.Fatalf("order should be Requirements → Sections → Module, got indices %d, %d, %d\n%s",
			reqIdx, secIdx, modIdx, out)
	}

	// Section heading with type annotation.
	if !strings.Contains(out, "### delivery (coupled)") {
		t.Errorf("expected '### delivery (coupled)' heading, got:\n%s", out)
	}

	// Freeform content fields surfaced.
	for _, frag := range []string{"versioning", "semver", "artifacts", "app-binary", "channels", "stable"} {
		if !strings.Contains(out, frag) {
			t.Errorf("section content should surface %q, got:\n%s", frag, out)
		}
	}
}

// SM4: Multiple sections rendered in declaration order (markdown)
func TestFR1_SM4_MarkdownSectionOrder(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "order-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "delivery", Type: "coupled", Raw: json.RawMessage(`{"name":"delivery"}`)},
				{ID: "section00002", Name: "performance", Type: "informational", Raw: json.RawMessage(`{"name":"performance"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	delIdx := strings.Index(out, "### delivery")
	perfIdx := strings.Index(out, "### performance")
	if delIdx < 0 || perfIdx < 0 {
		t.Fatalf("expected both section headings, got:\n%s", out)
	}
	if delIdx >= perfIdx {
		t.Fatal("delivery should appear before performance (declaration order)")
	}
}

// SM5: Non-coupled section rendered without module cross-reference (markdown)
func TestFR1_SM5_MarkdownSectionNoCoupling(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "notes-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "notes", Type: "informational", Raw: json.RawMessage(`{"name":"notes","body":"free-form notes"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "### notes (informational)") {
		t.Errorf("expected '### notes (informational)' heading, got:\n%s", out)
	}
	if !strings.Contains(out, "free-form notes") {
		t.Errorf("freeform content should appear, got:\n%s", out)
	}
	// No module section should be emitted and no coupled link line.
	if strings.Contains(out, "## Module:") {
		t.Errorf("should not render a module section for informational section, got:\n%s", out)
	}
}

// SM6: Spec with no sections array omits sections heading
func TestFR1_SM6_MarkdownNoSectionsHeading(t *testing.T) {
	spec := fixtureGraph() // fixture has no Sections
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "## Sections") {
		t.Fatalf("no sections in spec — should omit '## Sections' heading, got:\n%s", out)
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
		nodeID string
		shape  string
	}{
		{`"a1a1a1a1a1a1"`, "shape=box"},
		{`"c1c1c1c1c1c1"`, "shape=component"},
		{`"f1f1f1f1f1f1"`, "shape=ellipse"},
	}
	for _, tt := range tests {
		// Find the line containing the node
		found := false
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, tt.nodeID) && strings.Contains(line, tt.shape) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("node %s should have %s, got:\n%s", tt.nodeID, tt.shape, out)
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

	// Builder (c2c2c2c2c2c2) implements the Build requirement (a2a2a2a2a2a2)
	if !strings.Contains(out, `"c2c2c2c2c2c2" -> "a2a2a2a2a2a2"`) {
		t.Fatalf("should have edge Builder -> Build requirement, got:\n%s", out)
	}
	if !strings.Contains(out, `label="implements"`) {
		t.Fatal("should have implements label")
	}
	// Builder uses Parser (c1c1c1c1c1c1) with dotted style
	if !strings.Contains(out, `"c2c2c2c2c2c2" -> "c1c1c1c1c1c1"`) {
		t.Fatalf("should have Builder -> Parser uses edge, got:\n%s", out)
	}
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

// D6: Node IDs are the bare identity hashes flow_render_pipeline.md declares
//
// "Node IDs are identity hashes" — spec/render/flow_render_pipeline.md.
// Every node declaration and every edge endpoint must therefore be a bare
// 12-char hex hash, identical to what `spex hash-id` prints, with no
// `<module>_<type>_` composite prefix. Hashes are quoted because a hash
// beginning with a digit is not a legal bare DOT identifier.
func TestFR2_D6_ValidNodeIDs(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	for _, id := range []string{
		`"c1c1c1c1c1c1"`, // Parser component
		`"c2c2c2c2c2c2"`, // Builder component
		`"a1a1a1a1a1a1"`, // alpha Parse requirement
		`"a2a2a2a2a2a2"`, // alpha Build requirement
		`"f1f1f1f1f1f1"`, // alpha data_flow
		`"c3c3c3c3c3c3"`, // Consumer component
		`"b1b1b1b1b1b1"`, // beta Consume requirement
		`"111111111111"`, // alpha module
		`"222222222222"`, // beta module
		`"112233445566"`, // project requirement
	} {
		if !strings.Contains(out, id) {
			t.Errorf("missing node ID %s in:\n%s", id, out)
		}
	}

	// No composite <module>_<type>_<hash> identifier survives.
	for _, legacy := range []string{
		"alpha_comp_", "alpha_req_", "alpha_flow_",
		"beta_comp_", "beta_req_", "preq_112233445566",
	} {
		if strings.Contains(out, legacy) {
			t.Errorf("legacy composite node ID %q still emitted:\n%s", legacy, out)
		}
	}

	// Every statement whose subject is a node — declarations and edges alike —
	// names it with a quoted bare identity hash.
	nodeDecl := regexp.MustCompile(`^"([0-9a-zA-Z_]*)" \[`)
	edgeStmt := regexp.MustCompile(`^"([0-9a-zA-Z_]*)" -> "([0-9a-zA-Z_]*)" \[`)
	bareHash := regexp.MustCompile(`^[0-9a-f]{12}$`)

	seen := 0
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `"`) {
			continue
		}
		var ids []string
		if m := edgeStmt.FindStringSubmatch(trimmed); m != nil {
			ids = m[1:]
		} else if m := nodeDecl.FindStringSubmatch(trimmed); m != nil {
			ids = m[1:]
		} else {
			t.Errorf("unrecognised quoted-subject statement: %q", trimmed)
			continue
		}
		for _, id := range ids {
			seen++
			if !bareHash.MatchString(id) {
				t.Errorf("node ID %q is not a bare 12-hex identity hash (line %q)", id, trimmed)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("no node IDs inspected; output was:\n%s", out)
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
		"project":                        true,
		"module:alpha":                   true,
		"module:beta":                    true,
		"module:alpha:req:a1a1a1a1a1a1":  true,
		"module:alpha:req:a2a2a2a2a2a2":  true,
		"module:alpha:comp:c1c1c1c1c1c1": true,
		"module:alpha:comp:c2c2c2c2c2c2": true,
		"module:alpha:flow:f1f1f1f1f1f1": true,
		"module:beta:req:b1b1b1b1b1b1":   true,
		"module:beta:comp:c3c3c3c3c3c3":  true,
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
		if n.ID == "module:alpha:comp:c1c1c1c1c1c1" {
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
		{From: "module:alpha:comp:c1c1c1c1c1c1", To: "module:alpha:req:a1a1a1a1a1a1", Type: "implements"},
		{From: "module:alpha:comp:c2c2c2c2c2c2", To: "module:alpha:comp:c1c1c1c1c1c1", Type: "uses"},
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
		{From: "module:alpha:flow:f1f1f1f1f1f1", To: "module:alpha:comp:c1c1c1c1c1c1", Type: "uses"},
		{From: "module:alpha:flow:f1f1f1f1f1f1", To: "module:alpha:comp:c2c2c2c2c2c2", Type: "uses"},
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

// SM2: DOT renders section nodes and coupling edges
func TestFR2_SM2_SectionsDOT(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name:    "delivery-test",
			Modules: []schema.Module{{ID: "delivery0001", Name: "delivery", Path: "delivery"}},
			Sections: []schema.Section{{
				ID:   "section00001",
				Name: "delivery",
				Type: "coupled",
				Raw:  json.RawMessage(`{"id":"section00001","name":"delivery","type":"coupled"}`),
			}},
		},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "delivery0001", Name: "delivery", Path: "delivery"},
			Spec:    schema.ModuleSpec{Name: "delivery"},
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Section node present with shape=tab and label "delivery"
	if !strings.Contains(out, `"section00001" `) {
		t.Fatalf("section node not declared, got:\n%s", out)
	}
	if !strings.Contains(out, "shape=tab") {
		t.Fatalf("section node should have shape=tab, got:\n%s", out)
	}
	// Coupling edge section -> delivery module, both by identity hash
	if !strings.Contains(out, `"section00001" -> "delivery0001"`) {
		t.Fatalf("missing coupling edge section -> delivery module, got:\n%s", out)
	}
	if !strings.Contains(out, `label="coupled"`) {
		t.Fatalf("coupling edge should have label coupled, got:\n%s", out)
	}

	// Section node must appear BEFORE any subgraph (project-level, not inside cluster)
	sectionIdx := strings.Index(out, `"section00001" [`)
	subgraphIdx := strings.Index(out, "subgraph cluster_delivery")
	if sectionIdx < 0 || subgraphIdx < 0 {
		t.Fatalf("expected both section decl and subgraph, got:\n%s", out)
	}
	if sectionIdx >= subgraphIdx {
		t.Fatal("section node should be emitted before module subgraph (project-level)")
	}
}

// SM4: Multiple sections rendered in declaration order (DOT)
func TestFR2_SM4_SectionOrderDOT(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "order-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "delivery", Type: "coupled", Raw: json.RawMessage(`{"name":"delivery"}`)},
				{ID: "section00002", Name: "performance", Type: "informational", Raw: json.RawMessage(`{"name":"performance"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	delIdx := strings.Index(out, `"section00001" [`)
	perfIdx := strings.Index(out, `"section00002" [`)
	if delIdx < 0 || perfIdx < 0 {
		t.Fatalf("expected both section nodes, got:\n%s", out)
	}
	if delIdx >= perfIdx {
		t.Fatal("delivery should appear before performance (declaration order)")
	}
}

// SM5: Non-coupled section rendered without module link (DOT)
func TestFR2_SM5_SectionNoCouplingDOT(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "notes-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "notes", Type: "informational", Raw: json.RawMessage(`{"name":"notes"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Section node exists
	if !strings.Contains(out, `"section00001" [`) {
		t.Fatalf("notes section node not declared, got:\n%s", out)
	}
	// No coupled edge
	if strings.Contains(out, "coupled") {
		t.Fatalf("informational section should not emit coupled edge, got:\n%s", out)
	}
}

// SM3: JSON includes section nodes and coupling edges
func TestFR3_SM3_SectionsJSON(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name:    "delivery-test",
			Modules: []schema.Module{{ID: "delivery0001", Name: "delivery", Path: "delivery"}},
			Sections: []schema.Section{{
				ID:   "section00001",
				Name: "delivery",
				Type: "coupled",
				Raw: json.RawMessage(`{
					"id": "section00001",
					"name": "delivery",
					"type": "coupled",
					"versioning": {"scheme": "semver", "source": "git-tag"},
					"artifacts": [{"id": 1, "name": "app", "type": "binary"}],
					"channels": ["stable", "edge"]
				}`),
			}},
		},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "delivery0001", Name: "delivery", Path: "delivery"},
			Spec:    schema.ModuleSpec{Name: "delivery"},
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []map[string]any `json:"nodes"`
		Edges []GraphEdge      `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	var sectionNode map[string]any
	for _, n := range result.Nodes {
		if n["id"] == "section:delivery" {
			sectionNode = n
			break
		}
	}
	if sectionNode == nil {
		t.Fatalf("section:delivery node not found in:\n%s", buf.String())
	}
	if sectionNode["type"] != "section" {
		t.Errorf("want type=section, got %v", sectionNode["type"])
	}
	if sectionNode["name"] != "delivery" {
		t.Errorf("want name=delivery, got %v", sectionNode["name"])
	}
	if sectionNode["section_type"] != "coupled" {
		t.Errorf("want section_type=coupled, got %v", sectionNode["section_type"])
	}
	if _, ok := sectionNode["versioning"]; !ok {
		t.Error("section node should include freeform 'versioning' field")
	}
	if _, ok := sectionNode["artifacts"]; !ok {
		t.Error("section node should include freeform 'artifacts' field")
	}
	if _, ok := sectionNode["channels"]; !ok {
		t.Error("section node should include freeform 'channels' field")
	}

	coupledFound := false
	for _, e := range result.Edges {
		if e.From == "section:delivery" && e.To == "module:delivery" && e.Type == "coupled" {
			coupledFound = true
			break
		}
	}
	if !coupledFound {
		t.Fatalf("missing coupled edge section:delivery -> module:delivery in:\n%s", buf.String())
	}
}

// SM4: Multiple sections rendered in declaration order (JSON)
func TestFR3_SM4_SectionOrderJSON(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "order-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "delivery", Type: "coupled", Raw: json.RawMessage(`{"name":"delivery"}`)},
				{ID: "section00002", Name: "performance", Type: "informational", Raw: json.RawMessage(`{"name":"performance"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	delIdx := strings.Index(out, `"section:delivery"`)
	perfIdx := strings.Index(out, `"section:performance"`)
	if delIdx < 0 || perfIdx < 0 {
		t.Fatalf("expected both section nodes, got:\n%s", out)
	}
	if delIdx >= perfIdx {
		t.Fatal("delivery should appear before performance (declaration order)")
	}
}

// SM5: Non-coupled section rendered without module link (JSON)
func TestFR3_SM5_SectionNoCouplingJSON(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{
			Name: "notes-test",
			Sections: []schema.Section{
				{ID: "section00001", Name: "notes", Type: "informational", Raw: json.RawMessage(`{"name":"notes"}`)},
			},
		},
		Modules: []ModuleGraph{},
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Edges []GraphEdge `json:"edges"`
	}
	json.Unmarshal(buf.Bytes(), &result)

	for _, e := range result.Edges {
		if e.Type == "coupled" {
			t.Fatalf("informational section should not emit coupled edge, got %+v", e)
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

	// 1 project + 2 modules + 3 proj reqs + (2+1) mod reqs + (2+1) comps + 1 flow = 13
	if len(result.Nodes) != 13 {
		t.Fatalf("want 13 nodes, got %d", len(result.Nodes))
	}
}

// ---- Edge cases ----

// E1: Module with empty requirements array
func TestFR1_E1_Renderer_EmptyRequirements(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{Name: "empty-req", Modules: []schema.Module{{ID: "m00000000001", Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "m00000000001", Name: "m", Path: "m"},
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
	// Requirement nodes are the only lightgreen boxes; none should exist.
	if strings.Contains(buf.String(), "fillcolor=lightgreen") {
		t.Fatalf("should have no requirement nodes in DOT, got:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), `label="implements"`) {
		t.Fatal("should have no implements edges in DOT")
	}
}

// E3: Content containing JSON-special characters
func TestFR3_E3_JSONSpecialChars(t *testing.T) {
	modSpec := schema.ModuleSpec{
		Name:       "m",
		Components: []schema.Component{{ID: "aabbccddeeff", Name: "C", Content: "arch.md"}},
	}
	spec := &SpecGraph{
		Project: schema.Project{Name: "special", Modules: []schema.Module{{ID: "m00000000001", Name: "m", Path: "m"}}},
		Profile: schema.DefaultProfile(),
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: "m00000000001", Name: "m", Path: "m"},
			Spec:   modSpec,
			Nodes:  nodesFromJSON(modSpec, "module", "m"),
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

	// Verify the escaped content actually reached the output — not just
	// that the document parses.
	if !strings.Contains(buf.String(), `\"quotes\" and \\backslashes and {braces}`) {
		t.Fatalf("expected escaped special characters in output, got:\n%s", buf.String())
	}
}

// E5: Content with deeply nested headings
func TestFR1_E5_DeeplyNestedHeadings(t *testing.T) {
	deepSpec := schema.ModuleSpec{
		Name:       "m",
		Components: []schema.Component{{ID: "aabbccddeeff", Name: "C", Content: "arch.md"}},
	}
	spec := &SpecGraph{
		Project: schema.Project{Name: "deep", Modules: []schema.Module{{ID: "m00000000001", Name: "m", Path: "m"}}},
		Profile: schema.DefaultProfile(),
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: "m00000000001", Name: "m", Path: "m"},
			Spec:   deepSpec,
			Nodes:  nodesFromJSON(deepSpec, "module", "m"),
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
	modSpec := schema.ModuleSpec{
		Name:       "data-pipeline",
		Components: []schema.Component{{ID: "aabbccddeeff", Name: "Ingest"}},
	}
	spec := &SpecGraph{
		Project: schema.Project{Name: "test", Modules: []schema.Module{{ID: "datapipe0001", Name: "data-pipeline", Path: "data-pipeline"}}},
		Profile: schema.DefaultProfile(),
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "datapipe0001", Name: "data-pipeline", Path: "data-pipeline"},
			Spec:    modSpec,
			Nodes:   nodesFromJSON(modSpec, "module", "data-pipeline"),
			Content: map[string]string{},
		}},
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Hyphens should be replaced with underscores in the cluster name.
	if !strings.Contains(out, "cluster_data_pipeline") {
		t.Fatalf("hyphen should be replaced in cluster name, got:\n%s", out)
	}
	// Node IDs are identity hashes, so a hyphenated module name never reaches
	// them; the module node itself is named by its own hash.
	if !strings.Contains(out, `"aabbccddeeff" [label="Ingest"`) {
		t.Fatalf("component node should be keyed by its identity hash, got:\n%s", out)
	}
	if !strings.Contains(out, `"datapipe0001" [label="data-pipeline"`) {
		t.Fatalf("module node should be keyed by its identity hash, got:\n%s", out)
	}
}

// E7: Empty spec (project with module that has only name)
func TestFR1_E7_EmptySpec(t *testing.T) {
	spec := &SpecGraph{
		Project: schema.Project{Name: "empty", Modules: []schema.Module{{ID: "m00000000001", Name: "m", Path: "m"}}},
		Modules: []ModuleGraph{{
			Module:  schema.Module{ID: "m00000000001", Name: "m", Path: "m"},
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

// ---- api nodes, test_section nodes and the slim JSON view ----

// surfaceGraph builds a one-module SpecGraph whose every ID is the real
// identity hash `spex hash-id` computes for that node, and which carries the
// two node types fixtureGraph deliberately omits: test_section and api.
func surfaceGraph() *SpecGraph {
	const mod = "gamma"
	modID := schema.IdentityHash("module", mod)
	projReqID := schema.IdentityHash("project", "requirement", "Expose a surface")
	reqID := schema.IdentityHash(mod, "requirement", "Serve requests")
	compID := schema.IdentityHash(mod, "component", "Server")
	flowID := schema.IdentityHash(mod, "data_flow", "Request path")
	testID := schema.IdentityHash(mod, "test_section", "Server tests")

	proj := schema.Project{
		Name:         "surface-test",
		Requirements: []schema.Requirement{{ID: projReqID, Type: "functional", Title: "Expose a surface"}},
		Modules:      []schema.Module{{ID: modID, Name: mod, Path: mod}},
	}
	modSpec := schema.ModuleSpec{
		Name: mod,
		Requirements: []schema.ModuleRequirement{
			{ID: reqID, PreqID: projReqID, Type: "functional", Title: "Serve requests"},
		},
		Components: []schema.Component{
			{ID: compID, Name: "Server", Description: "Serves requests.", Content: "arch_server.md", Implements: []string{reqID}},
		},
		DataFlows: []schema.DataFlow{
			{ID: flowID, Name: "Request path", Description: "Request in, response out.", Content: "flow_request_path.md", Uses: []string{compID}},
		},
		TestSections: []schema.TestSection{
			{ID: testID, Name: "Server tests", Content: "test_server.md", Describes: []string{compID}},
		},
		APIs: []schema.API{
			{
				ID:          schema.IdentityHash(mod, "api", "spex serve"),
				Name:        "spex serve",
				Description: "Start the server.",
				ProvidedBy:  []string{compID},
				Group:       "cli",
			},
			{
				ID:    schema.IdentityHash(mod, "api", "GET /v1/specs/{id}"),
				Name:  "GET /v1/specs/{id}",
				Group: "http",
			},
		},
	}

	return &SpecGraph{
		Project:      proj,
		Profile:      schema.DefaultProfile(),
		ProjectNodes: nodesFromJSON(proj, "project", ""),
		Modules: []ModuleGraph{{
			Module: schema.Module{ID: modID, Name: mod, Path: mod},
			Spec:   modSpec,
			Nodes:  nodesFromJSON(modSpec, "module", mod),
			Content: map[string]string{
				"arch_server.md":       "# Server\n\nServes requests.\n",
				"flow_request_path.md": "# Request path\n\nRequest in, response out.\n",
				"test_server.md":       "# Server tests\n\nDrive the server end to end.\n",
			},
		}},
	}
}

// decodeSlim renders the slim view and returns it decoded generically, so the
// assertions see the literal key set rather than a struct's projection of it.
func decodeSlim(t *testing.T, spec *SpecGraph) (map[string]any, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderJSONSlim(spec, &buf); err != nil {
		t.Fatalf("slim render error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("slim output is not valid JSON: %v\n%s", err, buf.String())
	}
	return out, buf.Bytes()
}

// assertNoKeys fails if any object anywhere below v carries a banned key.
func assertNoKeys(t *testing.T, v any, banned []string, path string) {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for k, sub := range node {
			for _, b := range banned {
				if k == b {
					t.Errorf("%s: banned key %q is present", path, b)
				}
			}
			assertNoKeys(t, sub, banned, path+"."+k)
		}
	case []any:
		for i, sub := range node {
			assertNoKeys(t, sub, banned, fmt.Sprintf("%s[%d]", path, i))
		}
	}
}

// J9: --slim emits nodes only, each exactly {id, type, name, module}
func TestFR3_J9_SlimNodesOnly(t *testing.T) {
	out, _ := decodeSlim(t, surfaceGraph())

	if _, ok := out["edges"]; ok {
		t.Error("slim output must not carry an edges array")
	}
	nodes, ok := out["nodes"].([]any)
	if !ok {
		t.Fatalf("slim output has no nodes array, got keys %v", out)
	}
	if len(nodes) == 0 {
		t.Fatal("slim output has no nodes")
	}

	allowed := map[string]bool{"id": true, "type": true, "name": true, "module": true}
	for i, n := range nodes {
		obj, ok := n.(map[string]any)
		if !ok {
			t.Fatalf("node %d is not an object: %#v", i, n)
		}
		for _, k := range []string{"id", "type", "name"} {
			if v, present := obj[k]; !present || v == "" {
				t.Errorf("node %d is missing %q: %#v", i, k, obj)
			}
		}
		for k := range obj {
			if !allowed[k] {
				t.Errorf("node %d carries unexpected key %q: %#v", i, k, obj)
			}
		}
	}
}

// J9b: --slim drops inlined content and descriptions everywhere
func TestFR3_J9_SlimDropsContentAndDescription(t *testing.T) {
	out, raw := decodeSlim(t, surfaceGraph())

	assertNoKeys(t, out, []string{"content", "description"}, "$")

	// The component and data_flow descriptions and the content leaves are the
	// bytes --slim exists to shed; none of their text may survive.
	for _, text := range []string{
		"Serves requests.",
		"Request in, response out.",
		"Accept, dispatch, reply.",
		"Start the server.",
	} {
		if bytes.Contains(raw, []byte(text)) {
			t.Errorf("slim output still carries dropped text %q:\n%s", text, raw)
		}
	}
}

// J10: --slim node IDs are the bare identity hashes hash-id computes
func TestFR3_J10_SlimBareIdentityHashes(t *testing.T) {
	spec := surfaceGraph()
	out, _ := decodeSlim(t, spec)
	nodes := out["nodes"].([]any)

	bareHash := regexp.MustCompile(`^[0-9a-f]{12}$`)
	got := map[string]string{} // id -> type
	for i, n := range nodes {
		obj := n.(map[string]any)
		id, _ := obj["id"].(string)
		if !bareHash.MatchString(id) {
			t.Errorf("node %d id %q is not a bare 12-hex identity hash", i, id)
		}
		if strings.Contains(id, ":") {
			t.Errorf("node %d id %q still carries a synthetic path prefix", i, id)
		}
		got[id] = obj["type"].(string)
	}

	// Each ID must equal what `spex hash-id` prints for that node.
	const mod = "gamma"
	want := map[string]string{
		schema.IdentityHash("module", mod):                                "module",
		schema.IdentityHash("project", "requirement", "Expose a surface"): "requirement",
		schema.IdentityHash(mod, "requirement", "Serve requests"):         "requirement",
		schema.IdentityHash(mod, "component", "Server"):                   "component",
		schema.IdentityHash(mod, "data_flow", "Request path"):             "data_flow",
		schema.IdentityHash(mod, "test_section", "Server tests"):          "test_section",
		schema.IdentityHash(mod, "api", "spex serve"):                     "api",
		schema.IdentityHash(mod, "api", "GET /v1/specs/{id}"):             "api",
	}
	for id, typ := range want {
		if got[id] != typ {
			t.Errorf("hash-id %s should be a %s node in slim output, got %q", id, typ, got[id])
		}
	}
	if len(got) != len(want) {
		t.Errorf("want %d slim nodes, got %d: %v", len(want), len(got), got)
	}
}

// J11: --slim includes every test_section and api node
func TestFR3_J11_SlimIncludesTestSectionsAndAPIs(t *testing.T) {
	out, _ := decodeSlim(t, surfaceGraph())
	nodes := out["nodes"].([]any)

	byType := map[string][]string{}
	for _, n := range nodes {
		obj := n.(map[string]any)
		typ, _ := obj["type"].(string)
		name, _ := obj["name"].(string)
		byType[typ] = append(byType[typ], name)
	}

	if want := []string{"Server tests"}; !equalStrings(byType["test_section"], want) {
		t.Errorf("slim test_section nodes = %v, want %v", byType["test_section"], want)
	}
	if want := []string{"spex serve", "GET /v1/specs/{id}"}; !equalStrings(byType["api"], want) {
		t.Errorf("slim api nodes = %v, want %v", byType["api"], want)
	}
	for _, n := range nodes {
		obj := n.(map[string]any)
		if obj["type"] == "test_section" || obj["type"] == "api" {
			if obj["module"] != "gamma" {
				t.Errorf("node %v should be scoped to module gamma", obj)
			}
		}
	}
}

// J12: --slim output is compact, not indented
func TestFR3_J12_SlimIsCompact(t *testing.T) {
	_, raw := decodeSlim(t, surfaceGraph())

	body := bytes.TrimRight(raw, "\n")
	if bytes.Contains(body, []byte("\n")) {
		t.Errorf("slim output should be a single compact line, got:\n%s", raw)
	}
	if bytes.Contains(body, []byte(`", "`)) {
		t.Errorf("slim output should not be pretty-printed, got:\n%s", raw)
	}
}

// J15: --slim emits the declared IDs verbatim, and emits project sections
//
// surfaceGraph cannot prove either half. Every ID in it *is* the
// schema.IdentityHash of its own identity string, so a renderer that recomputed
// IDs from names instead of reading the declared ones would satisfy J10 and D8
// unnoticed — and recomputation is the one failure this part exists to make
// impossible: 15 project-level requirement IDs in the live spec/project.json
// predate the current identity string and do not equal their hash, so a
// recomputing renderer would silently rewrite them. surfaceGraph also declares
// no sections, leaving the slim view's section loop unexercised.
//
// fixtureGraph's IDs are synthetic and deliberately unequal to their identity
// hashes, so it catches recomputation in the slim JSON here and, through D6 and
// E6, in the DOT renderer. A section is added locally because SM6 depends on
// fixtureGraph itself declaring none.
func TestFR3_J15_SlimEmitsDeclaredIDs(t *testing.T) {
	spec := fixtureGraph()
	spec.Project.Sections = []schema.Section{{
		ID:   "aaaabbbbcccc",
		Name: "notes",
		Type: "informational",
		Raw:  json.RawMessage(`{"id":"aaaabbbbcccc","name":"notes","type":"informational","body":"free-form notes"}`),
	}}

	_, raw := decodeSlim(t, spec)
	var got struct {
		Nodes []SlimNode `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("slim output does not decode into []SlimNode: %v\n%s", err, raw)
	}

	// Declaration order, exhaustively. Pinning the whole list also pins what
	// --slim leaves out: the project root, which RenderJSON does not emit.
	want := []SlimNode{
		{ID: "112233445566", Type: "requirement", Name: "Parse input"},
		{ID: "665544332211", Type: "requirement", Name: "Build output"},
		{ID: "778899aabbcc", Type: "requirement", Name: "Performance"},
		{ID: "aaaabbbbcccc", Type: "section", Name: "notes"},
		{ID: "111111111111", Type: "module", Name: "alpha"},
		{ID: "a1a1a1a1a1a1", Type: "requirement", Name: "Parse", Module: "alpha"},
		{ID: "a2a2a2a2a2a2", Type: "requirement", Name: "Build", Module: "alpha"},
		{ID: "c1c1c1c1c1c1", Type: "component", Name: "Parser", Module: "alpha"},
		{ID: "c2c2c2c2c2c2", Type: "component", Name: "Builder", Module: "alpha"},
		{ID: "f1f1f1f1f1f1", Type: "data_flow", Name: "Build Pipeline", Module: "alpha"},
		{ID: "222222222222", Type: "module", Name: "beta"},
		{ID: "b1b1b1b1b1b1", Type: "requirement", Name: "Consume", Module: "beta"},
		{ID: "c3c3c3c3c3c3", Type: "component", Name: "Consumer", Module: "beta"},
	}
	if len(got.Nodes) != len(want) {
		t.Fatalf("want %d slim nodes, got %d: %+v", len(want), len(got.Nodes), got.Nodes)
	}
	for i := range want {
		if got.Nodes[i] != want[i] {
			t.Errorf("slim node %d = %+v, want %+v", i, got.Nodes[i], want[i])
		}
	}

	// Guard integrity. The assertion above only detects a recomputing renderer
	// while these declared IDs differ from the hashes such a renderer would
	// produce, so pin that property rather than trusting it.
	for _, tc := range []struct {
		id       string
		identity []string
	}{
		{"112233445566", []string{"project", "requirement", "Parse input"}},
		{"111111111111", []string{"module", "alpha"}},
		{"a1a1a1a1a1a1", []string{"alpha", "requirement", "Parse"}},
		{"c1c1c1c1c1c1", []string{"alpha", "component", "Parser"}},
		{"f1f1f1f1f1f1", []string{"alpha", "data_flow", "Build Pipeline"}},
	} {
		if schema.IdentityHash(tc.identity...) == tc.id {
			t.Errorf("fixture ID %q equals IdentityHash(%v); the fixture no longer guards against ID recomputation",
				tc.id, tc.identity)
		}
	}
}

// J13: the full JSON renderer emits test_section nodes and their describes edges
func TestFR3_J13_TestSectionNodesInFullJSON(t *testing.T) {
	spec := surfaceGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	testID := schema.IdentityHash("gamma", "test_section", "Server tests")
	compID := schema.IdentityHash("gamma", "component", "Server")
	wantID := "module:gamma:test:" + testID

	var node *GraphNode
	for i := range result.Nodes {
		if result.Nodes[i].ID == wantID {
			node = &result.Nodes[i]
		}
	}
	if node == nil {
		t.Fatalf("test_section node %q missing from full JSON:\n%s", wantID, buf.String())
	}
	if node.Type != "test_section" || node.Name != "Server tests" || node.Module != "gamma" {
		t.Errorf("test_section node = %+v", *node)
	}
	if !strings.Contains(node.Content, "Drive the server end to end.") {
		t.Errorf("test_section node should inline its content leaf, got %q", node.Content)
	}

	want := GraphEdge{From: wantID, To: "module:gamma:comp:" + compID, Type: "describes"}
	if !hasEdge(result.Edges, want) {
		t.Errorf("missing test_section describes edge %+v in %+v", want, result.Edges)
	}
}

// J14: the full JSON renderer emits api nodes and their provided_by edges
func TestFR3_J14_APINodesInFullJSON(t *testing.T) {
	spec := surfaceGraph()
	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	apiID := schema.IdentityHash("gamma", "api", "spex serve")
	compID := schema.IdentityHash("gamma", "component", "Server")
	wantID := "module:gamma:api:" + apiID

	var node *GraphNode
	for i := range result.Nodes {
		if result.Nodes[i].ID == wantID {
			node = &result.Nodes[i]
		}
	}
	if node == nil {
		t.Fatalf("api node %q missing from full JSON:\n%s", wantID, buf.String())
	}
	if node.Type != "api" || node.Name != "spex serve" || node.Module != "gamma" {
		t.Errorf("api node = %+v", *node)
	}
	if node.Description != "Start the server." {
		t.Errorf("api node should carry its description, got %q", node.Description)
	}
	if node.Group != "cli" {
		t.Errorf("api node should carry its group, got %q", node.Group)
	}
	if node.Content != "" {
		t.Errorf("an api has no content leaf, got %q", node.Content)
	}

	want := GraphEdge{From: wantID, To: "module:gamma:comp:" + compID, Type: "provided_by"}
	if !hasEdge(result.Edges, want) {
		t.Errorf("missing provided_by edge %+v in %+v", want, result.Edges)
	}
}

// DT1 (JSON slice): a graph read under a profile declaring an "endpoint"
// type carries its nodes in the full JSON graph, typed "endpoint", and
// beside the built-in types in the slim node table — the renderer's own
// share of a scenario the three renderer beads split between them (see
// TestFR1_S9_ProfileDeclaredTypeFlowsGenerically for SpecReader's share).
func TestFR3_DT1_ProfileDeclaredTypeJSON(t *testing.T) {
	dir := t.TempDir()

	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
	})
	profile.Edges = append(profile.Edges, schema.Edge{
		Kind: "calls", From: []string{"endpoint"}, To: []string{"component"},
	})
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	writeFile(t, dir, "profile.json", string(profileJSON))

	writeFile(t, dir, "project.json", `{
		"name": "endpoint-project",
		"modules": [{"id": "api0000api00", "name": "api", "path": "api"}]
	}`)

	modDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, modDir, "module.json", `{
		"name": "api",
		"components": [{"id": "aabbccddeeff", "name": "Widgets", "content": "arch_widgets.md"}],
		"endpoints": [
			{"id": "112233445566", "name": "GET /v1/widgets", "content": "endpoint_get_widgets.md", "calls": ["aabbccddeeff"]},
			{"id": "665544332211", "name": "POST /v1/widgets", "content": "endpoint_post_widgets.md", "calls": ["aabbccddeeff"]}
		]
	}`)
	writeFile(t, modDir, "arch_widgets.md", "# Widgets\n")
	writeFile(t, modDir, "endpoint_get_widgets.md", "# GET /v1/widgets\n")
	writeFile(t, modDir, "endpoint_post_widgets.md", "# POST /v1/widgets\n")

	spec, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderJSON(spec, &buf); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var full struct {
		Nodes []GraphNode `json:"nodes"`
		Edges []GraphEdge `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &full); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	endpoints := map[string]GraphNode{}
	for _, n := range full.Nodes {
		if n.Type == "endpoint" {
			endpoints[n.ID] = n
		}
	}
	if len(endpoints) != 2 {
		t.Fatalf("want 2 endpoint nodes in the full JSON graph, got %d: %+v", len(endpoints), full.Nodes)
	}
	get, ok := endpoints["module:api:endpoint:112233445566"]
	if !ok {
		t.Fatalf("want endpoint node module:api:endpoint:112233445566, got %+v", endpoints)
	}
	if get.Name != "GET /v1/widgets" || get.Module != "api" {
		t.Errorf("GET endpoint node = %+v", get)
	}
	if !strings.Contains(get.Content, "GET /v1/widgets") {
		t.Errorf("GET endpoint should inline its content leaf, got %q", get.Content)
	}

	want := GraphEdge{From: "module:api:endpoint:112233445566", To: "module:api:comp:aabbccddeeff", Type: "calls"}
	if !hasEdge(full.Edges, want) {
		t.Errorf("missing calls edge %+v in %+v", want, full.Edges)
	}

	var slimBuf bytes.Buffer
	if err := RenderJSONSlim(spec, &slimBuf); err != nil {
		t.Fatalf("RenderJSONSlim: %v", err)
	}
	var slim struct {
		Nodes []SlimNode `json:"nodes"`
	}
	if err := json.Unmarshal(slimBuf.Bytes(), &slim); err != nil {
		t.Fatalf("invalid slim JSON: %v\n%s", err, slimBuf.String())
	}
	slimEndpoints := 0
	slimComponents := 0
	for _, n := range slim.Nodes {
		switch n.Type {
		case "endpoint":
			slimEndpoints++
		case "component":
			slimComponents++
		}
	}
	if slimEndpoints != 2 {
		t.Errorf("want 2 endpoint nodes in the slim table beside the built-in types, got %d: %+v", slimEndpoints, slim.Nodes)
	}
	if slimComponents != 1 {
		t.Errorf("want the built-in component node still in the slim table, got %d: %+v", slimComponents, slim.Nodes)
	}
}

// DT1 (DOT slice): a graph read under a profile declaring an "endpoint"
// type is drawn with a shape distinct from every built-in kind's, and its
// profile-declared "calls" edge reaches the picture as a labelled arrow —
// the DOT renderer's own share of the scenario TestFR3_DT1_ProfileDeclared-
// TypeJSON carries for JSON and TestFR1_S9_ProfileDeclaredTypeFlows-
// Generically carries for SpecReader.
func TestFR2_DT1_ProfileDeclaredTypeDOT(t *testing.T) {
	dir := t.TempDir()

	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
	})
	profile.Edges = append(profile.Edges, schema.Edge{
		Kind: "calls", From: []string{"endpoint"}, To: []string{"component"},
	})
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	writeFile(t, dir, "profile.json", string(profileJSON))

	writeFile(t, dir, "project.json", `{
		"name": "endpoint-project",
		"modules": [{"id": "api0000api00", "name": "api", "path": "api"}]
	}`)

	modDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, modDir, "module.json", `{
		"name": "api",
		"components": [{"id": "aabbccddeeff", "name": "Widgets", "content": "arch_widgets.md"}],
		"endpoints": [
			{"id": "112233445566", "name": "GET /v1/widgets", "content": "endpoint_get_widgets.md", "calls": ["aabbccddeeff"]},
			{"id": "665544332211", "name": "POST /v1/widgets", "content": "endpoint_post_widgets.md", "calls": ["aabbccddeeff"]}
		]
	}`)
	writeFile(t, modDir, "arch_widgets.md", "# Widgets\n")
	writeFile(t, modDir, "endpoint_get_widgets.md", "# GET /v1/widgets\n")
	writeFile(t, modDir, "endpoint_post_widgets.md", "# POST /v1/widgets\n")

	spec, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("RenderDOT: %v", err)
	}
	out := buf.String()

	for _, ep := range []struct{ id, label string }{
		{"112233445566", "GET /v1/widgets"},
		{"665544332211", "POST /v1/widgets"},
	} {
		if !strings.Contains(out, `"`+ep.id+`" [label="`+ep.label+`"`) {
			t.Fatalf("endpoint node %s not declared, got:\n%s", ep.id, out)
		}

		declLine := ""
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, `"`+ep.id+`" [`) {
				declLine = line
				break
			}
		}
		if declLine == "" {
			t.Fatalf("endpoint node %s declaration line not found, got:\n%s", ep.id, out)
		}
		for _, builtin := range []string{"shape=box", "shape=folder", "shape=component", "shape=ellipse", "shape=cds", "shape=tab"} {
			if strings.Contains(declLine, builtin) {
				t.Errorf("endpoint node %s should use a shape distinct from built-in kinds, got %q which contains %q", ep.id, declLine, builtin)
			}
		}

		if !strings.Contains(out, `"`+ep.id+`" -> "aabbccddeeff" [label="calls"];`) {
			t.Errorf("missing labelled calls edge from endpoint %s to component, got:\n%s", ep.id, out)
		}
	}
}

// D8: DOT node IDs equal the identity hashes hash-id computes
func TestFR2_D8_DOTNodeIDsMatchHashID(t *testing.T) {
	spec := surfaceGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Collect the ID of every node *declaration*. Matching the whole trimmed
	// line matters: an edge's target is also followed by " [label=", so a
	// substring search would accept a composite <module>_<type>_<hash>
	// declaration as long as some edge still mentioned the bare hash.
	declPattern := regexp.MustCompile(`^"([^"]+)" \[label=`)
	declared := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, " -> ") {
			continue
		}
		if m := declPattern.FindStringSubmatch(trimmed); m != nil {
			declared[m[1]] = true
		}
	}

	const mod = "gamma"
	for _, tc := range []struct {
		nodeType string
		parts    []string
	}{
		{"module", []string{"module", mod}},
		{"project requirement", []string{"project", "requirement", "Expose a surface"}},
		{"module requirement", []string{mod, "requirement", "Serve requests"}},
		{"component", []string{mod, "component", "Server"}},
		{"data_flow", []string{mod, "data_flow", "Request path"}},
		{"api", []string{mod, "api", "spex serve"}},
	} {
		want := schema.IdentityHash(tc.parts...)
		if !declared[want] {
			t.Errorf("%s node not declared under its hash-id %q (identity %v); declarations were %v\n%s",
				tc.nodeType, want, tc.parts, declared, out)
		}
	}
	// 1 project requirement + 1 module + 1 requirement + 1 component +
	// 1 data_flow + 2 apis.
	if len(declared) != 7 {
		t.Errorf("want 7 node declarations, got %d: %v", len(declared), declared)
	}

	// The implements edge joins two hash-id values and nothing else.
	wantEdge := fmt.Sprintf("%q -> %q",
		schema.IdentityHash(mod, "component", "Server"),
		schema.IdentityHash(mod, "requirement", "Serve requests"))
	if !strings.Contains(out, wantEdge) {
		t.Errorf("implements edge %s missing from DOT:\n%s", wantEdge, out)
	}
}

// D9: DOT emits api nodes and provided_by edges
func TestFR2_D9_APINodesInDOT(t *testing.T) {
	spec := surfaceGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	apiID := schema.IdentityHash("gamma", "api", "spex serve")
	compID := schema.IdentityHash("gamma", "component", "Server")

	wantDecl := fmt.Sprintf("%q [label=%q, shape=cds", apiID, "spex serve")
	if !strings.Contains(out, wantDecl) {
		t.Errorf("api node declaration %q missing from DOT:\n%s", wantDecl, out)
	}
	secondAPI := fmt.Sprintf("%q [label=%q", schema.IdentityHash("gamma", "api", "GET /v1/specs/{id}"), "GET /v1/specs/{id}")
	if !strings.Contains(out, secondAPI) {
		t.Errorf("second api node missing from DOT:\n%s", out)
	}
	wantEdge := fmt.Sprintf("%q -> %q [label=\"provided_by\"", apiID, compID)
	if !strings.Contains(out, wantEdge) {
		t.Errorf("provided_by edge %q missing from DOT:\n%s", wantEdge, out)
	}
}

// D10: DOT declares every node under its declared ID, never a recomputed one
//
// The DOT counterpart of J15, and needed for the same reason. D8 proves the
// node IDs equal what hash-id prints, but every ID in surfaceGraph *is* an
// IdentityHash, so D8 cannot tell a renderer that emits the declared ID from
// one that recomputes it from the name. D6's Contains checks cannot either: an
// edge endpoint still mentions the declared hash even when the declaration
// itself carries a recomputed one, so a recomputed project-requirement
// declaration slips past. fixtureGraph's synthetic IDs close the gap — J15
// carries the guard-integrity assertion that keeps them unequal to their
// identity hashes.
func TestFR2_D10_DOTDeclaresDeclaredIDs(t *testing.T) {
	spec := fixtureGraph()
	var buf bytes.Buffer
	if err := RenderDOT(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	// Only declarations, never edge endpoints — the distinction is the point.
	declPattern := regexp.MustCompile(`^"([^"]+)" \[label=`)
	declared := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, " -> ") {
			continue
		}
		if m := declPattern.FindStringSubmatch(trimmed); m != nil {
			declared[m[1]] = true
		}
	}

	want := []string{
		"112233445566", "665544332211", "778899aabbcc", // project requirements
		"111111111111",                 // module alpha
		"a1a1a1a1a1a1", "a2a2a2a2a2a2", // alpha requirements
		"c1c1c1c1c1c1", "c2c2c2c2c2c2", // alpha components
		"f1f1f1f1f1f1", // alpha data_flow
		"222222222222", // module beta
		"b1b1b1b1b1b1", // beta requirement
		"c3c3c3c3c3c3", // beta component
	}
	for _, id := range want {
		if !declared[id] {
			t.Errorf("node %q is not declared under its declared ID; declarations were %v\n%s",
				id, declared, out)
		}
	}
	if len(declared) != len(want) {
		t.Errorf("want %d node declarations, got %d: %v", len(want), len(declared), declared)
	}
}

// M7: markdown lists the module APIs with group and description
func TestFR1_M7_APIsInMarkdown(t *testing.T) {
	spec := surfaceGraph()
	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "### APIs") {
		t.Fatalf("missing '### APIs' heading:\n%s", out)
	}
	if !strings.Contains(out, "- `spex serve` (cli) — Start the server.") {
		t.Fatalf("missing spex serve api line:\n%s", out)
	}
	if !strings.Contains(out, "- `GET /v1/specs/{id}` (http)") {
		t.Fatalf("missing HTTP api line:\n%s", out)
	}

	// Declaration order is preserved.
	if strings.Index(out, "spex serve") >= strings.Index(out, "GET /v1/specs/{id}") {
		t.Error("apis should render in declaration order")
	}

	// The APIs block sits between Requirements and Architecture: a module's
	// external surface reads as the contract its requirements promise, before
	// the internals that provide it.
	lastIdx := -1
	for _, heading := range []string{
		"### Requirements",
		"### APIs",
		"### Architecture",
		"### Data Flows",
	} {
		idx := strings.Index(out, heading)
		if idx < 0 {
			t.Fatalf("missing heading %q in:\n%s", heading, out)
		}
		if idx <= lastIdx {
			t.Errorf("heading %q is out of order (byte offset %d, previous %d):\n%s",
				heading, idx, lastIdx, out)
		}
		lastIdx = idx
	}

	// A module with no apis emits no APIs heading.
	var plain bytes.Buffer
	if err := RenderMarkdown(fixtureGraph(), &plain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(plain.String(), "### APIs") {
		t.Error("a module without apis should not emit an APIs heading")
	}
}

// DT1 (Markdown slice): a graph read under a profile declaring an
// "endpoint" type gets its own per-type section, content inlined exactly as
// Architecture inlines components — the same fixture
// TestFR2_DT1_ProfileDeclaredTypeDOT and TestFR3_DT1_ProfileDeclaredTypeJSON
// carry for DOT and JSON.
func TestFR1_DT1_ProfileDeclaredTypeMarkdown(t *testing.T) {
	dir := t.TempDir()

	profile := schema.DefaultProfile()
	profile.NodeTypes = append(profile.NodeTypes, schema.NodeType{
		Name:            "endpoint",
		PluralKey:       "endpoints",
		Scope:           "module",
		RequiresContent: true,
	})
	profile.Edges = append(profile.Edges, schema.Edge{
		Kind: "calls", From: []string{"endpoint"}, To: []string{"component"},
	})
	profileJSON, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	writeFile(t, dir, "profile.json", string(profileJSON))

	writeFile(t, dir, "project.json", `{
		"name": "endpoint-project",
		"modules": [{"id": "api0000api00", "name": "api", "path": "api"}]
	}`)

	modDir := filepath.Join(dir, "api")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, modDir, "module.json", `{
		"name": "api",
		"components": [{"id": "aabbccddeeff", "name": "Widgets", "content": "arch_widgets.md"}],
		"endpoints": [
			{"id": "112233445566", "name": "GET /v1/widgets", "content": "endpoint_get_widgets.md", "calls": ["aabbccddeeff"]},
			{"id": "665544332211", "name": "POST /v1/widgets", "content": "endpoint_post_widgets.md", "calls": ["aabbccddeeff"]}
		]
	}`)
	writeFile(t, modDir, "arch_widgets.md", "# Widgets\n\nServes widget requests.\n")
	writeFile(t, modDir, "endpoint_get_widgets.md", "# GET /v1/widgets\n\nLists widgets.\n")
	writeFile(t, modDir, "endpoint_post_widgets.md", "# POST /v1/widgets\n\nCreates a widget.\n")

	spec, err := ReadSpec(dir)
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderMarkdown(spec, &buf); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "### Endpoints") {
		t.Fatalf("missing '### Endpoints' heading for the profile-declared type, got:\n%s", out)
	}
	if !strings.Contains(out, "#### GET /v1/widgets\n\nLists widgets.") {
		t.Errorf("endpoint content should be inlined with its heading adjusted, got:\n%s", out)
	}
	if !strings.Contains(out, "#### POST /v1/widgets\n\nCreates a widget.") {
		t.Errorf("endpoint content should be inlined with its heading adjusted, got:\n%s", out)
	}

	// Endpoints render alongside the built-in Architecture section, both
	// reached by the same per-type sectioning.
	archIdx := strings.Index(out, "### Architecture")
	epIdx := strings.Index(out, "### Endpoints")
	if archIdx < 0 || epIdx < 0 || archIdx >= epIdx {
		t.Fatalf("want '### Architecture' before '### Endpoints', got:\n%s", out)
	}
}

func hasEdge(edges []GraphEdge, want GraphEdge) bool {
	for _, e := range edges {
		if e == want {
			return true
		}
	}
	return false
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
