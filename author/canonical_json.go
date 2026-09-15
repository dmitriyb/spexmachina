package author

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// canonicalizeDoc reorders doc's top-level keys and every entry of every
// array it holds into the "profile's key order for node fields, arrays in
// declaration order" arch_node_editor.md's Formatting section requires of
// every file this package rewrites: two-space indent (json.MarshalIndent
// already gives that) plus this key order — never encoding/json's
// alphabetical map order, which is all a plain map[string]any can produce.
//
// docKey is "project.json" or "<module>/module.json" — which fixed
// top-level shape and per-array entry shape apply. A key doc does not carry
// is skipped; a key the fixed shape does not name (a profile-declared field
// or node type beyond schema's typed structs) is appended after the known
// ones, alphabetically, so nothing is ever dropped.
func canonicalizeDoc(doc map[string]any, docKey string, profile *schema.Profile) orderedFields {
	topOrder := moduleDocOrder
	arrayOrders := moduleArrayOrders
	if docKey == "project.json" {
		topOrder = projectDocOrder
		arrayOrders = projectArrayOrders
	}

	vals := make(map[string]any, len(doc))
	for k, v := range doc {
		arr, ok := v.([]any)
		if !ok {
			vals[k] = v
			continue
		}
		order, known := arrayOrders[k]
		if !known {
			order = profileFieldOrder(profile, k)
		}
		vals[k] = orderArray(arr, order)
	}

	return orderedFields{keys: orderedKeys(doc, topOrder), vals: vals}
}

// orderArray applies order to every map[string]any element of arr, leaving
// any non-object element (a string reference in implements/uses/…)
// untouched.
func orderArray(arr []any, order []string) []any {
	out := make([]any, len(arr))
	for i, item := range arr {
		if entry, ok := item.(map[string]any); ok {
			out[i] = orderedFields{keys: orderedKeys(entry, order), vals: entry}
		} else {
			out[i] = item
		}
	}
	return out
}

// orderedKeys returns m's keys in order, followed by any key order omits,
// alphabetically — the same "known fields first, extras after" shape used
// for both a document's top level and one array entry.
func orderedKeys(m map[string]any, order []string) []string {
	seen := make(map[string]bool, len(order))
	keys := make([]string, 0, len(m))
	for _, k := range order {
		if _, ok := m[k]; ok {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	extra := make([]string, 0, len(m)-len(keys))
	for k := range m {
		if !seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return append(keys, extra...)
}

// profileFieldOrder is the key order for a node type beyond schema's typed
// structs: the universal envelope (id, name, description, content for a
// content-bearing type) followed by the resolved profile's own declared
// fields for the type naming pluralKey, in declaration order — "the
// profile's key order for node fields" for exactly the case schema has no
// struct for.
func profileFieldOrder(profile *schema.Profile, pluralKey string) []string {
	if profile == nil {
		return nil
	}
	for _, nt := range profile.NodeTypes {
		if nt.PluralKey != pluralKey {
			continue
		}
		order := []string{"id", "name", "description"}
		if nt.RequiresContent {
			order = append(order, "content")
		}
		for _, f := range nt.Fields {
			order = append(order, f.Name)
		}
		return order
	}
	return nil
}

// orderedFields is a JSON object whose MarshalJSON emits keys exactly in
// keys' order — the one way to control object key order through
// encoding/json, which sorts a plain map[string]any's keys alphabetically
// regardless of insertion order. json.Indent (what MarshalIndent applies
// after Marshal) only adds whitespace; it never reorders keys, so this type
// composes with json.MarshalIndent unchanged.
type orderedFields struct {
	keys []string
	vals map[string]any
}

func (o orderedFields) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// jsonKeyOrder reads t's exported fields' `json:"..."` tags in declaration
// order — the order schema's struct definitions already fix for every
// built-in document and node-entry shape, and the same order the files
// under spec/ are hand-maintained in today.
func jsonKeyOrder(t reflect.Type) []string {
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

var (
	projectDocOrder = jsonKeyOrder(reflect.TypeOf(schema.Project{}))
	moduleDocOrder  = jsonKeyOrder(reflect.TypeOf(schema.ModuleSpec{}))

	projectArrayOrders = map[string][]string{
		"requirements": jsonKeyOrder(reflect.TypeOf(schema.Requirement{})),
		"modules":      jsonKeyOrder(reflect.TypeOf(schema.Module{})),
		"sections":     jsonKeyOrder(reflect.TypeOf(schema.Section{})),
	}
	moduleArrayOrders = map[string][]string{
		"requirements":  jsonKeyOrder(reflect.TypeOf(schema.ModuleRequirement{})),
		"components":    jsonKeyOrder(reflect.TypeOf(schema.Component{})),
		"data_flows":    jsonKeyOrder(reflect.TypeOf(schema.DataFlow{})),
		"test_sections": jsonKeyOrder(reflect.TypeOf(schema.TestSection{})),
		"apis":          jsonKeyOrder(reflect.TypeOf(schema.API{})),
	}
)
