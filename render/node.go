package render

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/dmitriyb/spexmachina/schema"
)

// Node is one profile-declared spec node, read generically off the array
// the resolved profile names for its type rather than off a fixed Go
// struct field. A renderer that walks Nodes reaches a profile-declared
// type it projects without per-type code — see flow_render_pipeline.md
// "Shape contract".
//
// Content is the relative path exactly as the node's own JSON declared
// it, unresolved — the same convention schema.Component, schema.DataFlow
// and schema.TestSection follow — so a caller looks up the inlined text
// via the owning ModuleGraph's Content map, keyed by this same string.
// Edges maps each edge kind the node's JSON carries to its identity-hash
// targets, whether the resolved profile declares the field as a single
// string (preq_id) or an array of strings (implements, uses, ...).
type Node struct {
	ID          string
	Type        string
	Name        string
	Description string
	Content     string
	Module      string
	Group       string
	Edges       map[string][]string
}

// rawTopLevelFields decodes data's top-level JSON object into its raw
// fields, left unparsed, so a profile-declared array can be found by its
// plural key without a fixed Go struct field for every possible node type.
func rawTopLevelFields(data []byte) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// nodesForScope walks the resolved profile's node types declared for one
// scope ("project" or "module"), extracting one Node per entry found under
// each type's plural key in raw. A type absent from raw contributes no
// nodes — a spec need not declare every type the profile allows — and
// nodes are returned in profile node-type declaration order, each type's
// entries in their JSON array order.
func nodesForScope(raw map[string]json.RawMessage, profile *schema.Profile, scope, moduleName string) ([]Node, error) {
	var nodes []Node
	for _, nt := range profile.NodeTypes {
		if nt.Scope != scope {
			continue
		}
		data, ok := raw[nt.PluralKey]
		if !ok {
			continue
		}
		var entries []map[string]any
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("parse %s: %w", nt.PluralKey, err)
		}
		for _, entry := range entries {
			nodes = append(nodes, nodeFromEntry(entry, nt, profile.Edges, moduleName))
		}
	}
	return nodes, nil
}

// nodeFromEntry builds one Node from a single decoded JSON object, reading
// its envelope fields opportunistically — a field the type's schema does
// not declare is simply absent from entry — and its edges from the
// resolved profile's edge declarations carried "from" this node type. Name
// falls back to "title" when absent: every built-in type's declared-name
// field is "name" but one — a requirement's is "title" (project.schema.json
// and module.schema.json declare no "name" property for it at all).
func nodeFromEntry(entry map[string]any, nt schema.NodeType, edges []schema.Edge, moduleName string) Node {
	n := Node{
		ID:          str(entry, "id"),
		Type:        nt.Name,
		Name:        str(entry, "name"),
		Description: str(entry, "description"),
		Content:     str(entry, "content"),
		Module:      moduleName,
		Group:       str(entry, "group"),
	}
	if n.Name == "" {
		n.Name = str(entry, "title")
	}

	for _, e := range edges {
		if !slices.Contains(e.From, nt.Name) {
			continue
		}
		targets := strSlice(entry, e.Kind)
		if len(targets) == 0 {
			continue
		}
		if n.Edges == nil {
			n.Edges = make(map[string][]string)
		}
		n.Edges[e.Kind] = targets
	}
	return n
}

func str(entry map[string]any, key string) string {
	v, _ := entry[key].(string)
	return v
}

// strSlice reads key from entry and normalizes it to a string slice
// regardless of whether the resolved profile declares the field as a
// single string (preq_id) or an array of strings (implements, uses,
// depends_on, ...). An empty string, or an array entry that isn't a
// non-empty string, is dropped.
func strSlice(entry map[string]any, key string) []string {
	switch v := entry[key].(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
