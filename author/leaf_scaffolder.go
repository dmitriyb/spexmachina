package author

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// Scaffold is LeafScaffolder, spec/author/arch_leaf_scaffolder.md: given the
// spec directory and a ScaffoldInput naming an existing node, it writes that
// node's content leaf skeleton — the title heading, the section headings
// the profile declares for the node's type under leaf_sections, and one
// placeholder line per declared edge the leaf owes a link for. It writes no
// JSON: the node's content path is read off its own declared "content"
// field (set by NodeEditor when the node was declared), never recomputed
// here.
//
// Three outcomes:
//
//   - A write or a no-op: (report, nil, nil). report.Written is empty for a
//     no-op — the leaf already carries exactly the skeleton Scaffold would
//     write (arch_leaf_scaffolder.md, "Refusals and idempotence": "a leaf
//     holding exactly the skeleton the scaffolder would write is reported
//     as already scaffolded and left byte-identical").
//   - A refusal: (nil, refusals, nil). Nothing is written. Every refusal
//     here is Scaffold's own guard — a non-empty leaf that does not already
//     hold the skeleton, a node whose type carries no content leaf, or a
//     node id that resolves to nothing — none of which is a shape the
//     validator's own checkers would ever raise, so unlike NodeRenamer's
//     module guard alongside Report's checkers, every LeafScaffolder
//     refusal is its own. The write itself still passes through Report so
//     the parity check on the tree runs the same way every other writing
//     command's does.
//   - An input error: (nil, nil, err). specDir cannot be read, or the
//     profile fails to resolve.
func Scaffold(specDir string, input ScaffoldInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}

	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}
	// trueBefore is the tree exactly as disk holds it, kept aside so the
	// content-check re-attachment below can tell a leaf patchMissingLeaves
	// faked present from one this write actually created.
	trueBefore := cloneSpecFS(before)
	// A content-bearing node whose leaf was deleted, or an adopter's node
	// that never had one, is exactly LeafScaffolder's own precondition
	// (arch_leaf_scaffolder.md, "Refusals and idempotence": "a leaf that is
	// absent ... is written"), but merkle.BuildTreeFS hard-fails building a
	// tree over a content-bearing entry whose declared file is missing —
	// there is no file to hash. patchMissingLeaves extends the "absent
	// means the same as zero bytes" equivalence LeafScaffolder already
	// applies to its own target leaf to every other declared leaf missing
	// from disk too, purely so Report's before/after trees build. That
	// patch also reaches Report's own CheckContentPathsFS pass, though,
	// since after is cloned from this patched before — every other
	// declared-but-missing leaf reads as present there and so never
	// resurfaces as the "content file not found" obligation
	// arch_obligation_reporter.md says a pre-existing finding still owes.
	// The call site re-attaches those findings by diffing against
	// trueBefore once Report has run.
	patchMissingLeaves(before, profile)

	loc, err := locateNode(before, profile, input.ID)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no node with id ") {
			return nil, []RefusalEntry{nodeNotFoundRefusal(before, profile, input.ID)}, nil
		}
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}
	if loc.isModule || !loc.nodeType.RequiresContent {
		return nil, []RefusalEntry{notContentBearingRefusal(input.ID, loc, profile)}, nil
	}

	ownerDoc, err := decodeDoc(before, loc.ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}
	entry := findEntryObject(ownerDoc, loc.nodeType.PluralKey, input.ID)
	if entry == nil {
		return nil, nil, fmt.Errorf("author: scaffold: %s not found in %s", input.ID, loc.ownerFile)
	}
	contentRel, _ := entry["content"].(string)
	contentPath := path.Join(loc.moduleDir, contentRel)

	edges := owedEdges(before, profile, loc, entry, input.ID)
	skeleton := buildSkeleton(loc.name, loc.nodeType.LeafSections, edges)

	existing, exists := before[contentPath]
	if exists && len(existing) > 0 {
		if string(existing) == string(skeleton) {
			return &WriteReport{}, nil, nil
		}
		return nil, []RefusalEntry{nonEmptyLeafRefusal(contentPath, "spex leaf scaffold")}, nil
	}

	after := cloneSpecFS(before)
	after[contentPath] = skeleton

	refusals, obligations, err := Report(before, after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	trueAfter := cloneSpecFS(trueBefore)
	trueAfter[contentPath] = skeleton
	obligations = append(obligations, validatorObligations(patchedContentFindings(after, trueAfter))...)

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: scaffold: %w", err)
	}

	return &WriteReport{Written: written, Obligations: obligations}, nil, nil
}

// patchMissingLeaves sets mem[path] to an empty byte slice for every
// content-bearing node's declared content path not already present in mem,
// so merkle.BuildTreeFS can hash the tree despite a leaf that was deleted
// or never written. See Scaffold's call site for why this is necessary.
func patchMissingLeaves(mem validator.MemFS, profile *schema.Profile) {
	docKeys, err := specDocKeys(mem)
	if err != nil {
		return
	}
	for _, key := range docKeys {
		doc, err := decodeDoc(mem, key)
		if err != nil {
			continue
		}
		scope := "module"
		dir := path.Dir(key)
		if key == "project.json" {
			scope = "project"
			dir = ""
		}
		for _, nt := range profile.NodeTypes {
			if nt.Scope != scope || !nt.RequiresContent {
				continue
			}
			arr, ok := doc[nt.PluralKey].([]any)
			if !ok {
				continue
			}
			for _, item := range arr {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				content, _ := obj["content"].(string)
				if content == "" {
					continue
				}
				p := path.Join(dir, content)
				if _, exists := mem[p]; !exists {
					mem[p] = []byte{}
				}
			}
		}
	}
}

// patchedContentFindings recovers the validator.CheckContentPathsFS findings
// patchMissingLeaves' fake-present entries hid from Report's own
// after-state pass: every finding CheckContentPathsFS raises against
// trueAfter (the tree carrying only this Scaffold's actual write, with
// every other declared-but-missing leaf still genuinely absent) that
// patchedAfter's own run — the one Report already ran, and whose results
// already travel in its refusals/obligations — does not also raise. A leaf
// this call itself just wrote is present in both trees and so raises
// nothing in either; only a leaf some other, unrelated node still owes is
// found here.
func patchedContentFindings(patchedAfter, trueAfter validator.MemFS) []validator.ValidationError {
	seen := make(map[string]bool)
	for _, e := range validator.CheckContentPathsFS(patchedAfter) {
		seen[errorKey(e)] = true
	}
	var out []validator.ValidationError
	for _, e := range validator.CheckContentPathsFS(trueAfter) {
		if !seen[errorKey(e)] {
			out = append(out, e)
		}
	}
	return out
}

// owedLink is one placeholder line's worth of typed link: the reference
// field that owes it and the target node's identity and display name.
type owedLink struct {
	field string
	id    string
	name  string
}

// owedEdges collects, in declaration order, every edge the node at loc owes
// a placeholder link for (arch_leaf_scaffolder.md, "What a skeleton is",
// point 3): outbound, one entry per value of every reference field loc's
// own type declares on entry; inbound, one entry per node of a
// non-content-bearing type whose own reference field targets loc's type and
// names input's id. A content-bearing source's own outbound field already
// carries the link the other way once that node is itself scaffolded, so it
// is excluded here — the default profile's own shape is what this
// generalises: a component owes its own implements/uses (outbound) plus
// every api's provided_by naming it (inbound, api being the one
// non-content-bearing type in the default vocabulary), and a data flow owes
// only its own uses (outbound; nothing non-content-bearing targets
// data_flow by default).
func owedEdges(mem validator.MemFS, profile *schema.Profile, loc nodeLocation, entry map[string]any, nodeID string) []owedLink {
	var out []owedLink

	for _, f := range loc.nodeType.Fields {
		if f.Kind != schema.FieldKindReference {
			continue
		}
		for _, targetID := range fieldValues(entry, f.Name) {
			name, _ := findNodeName(mem, targetID)
			out = append(out, owedLink{field: f.Name, id: targetID, name: name})
		}
	}

	docKeys, err := specDocKeys(mem)
	if err != nil {
		return out
	}
	for _, key := range docKeys {
		doc, err := decodeDoc(mem, key)
		if err != nil {
			continue
		}
		scope := "module"
		if key == "project.json" {
			scope = "project"
		}
		for _, t2 := range profile.NodeTypes {
			if t2.Scope != scope || t2.RequiresContent {
				continue
			}
			for _, f2 := range t2.Fields {
				if f2.Kind != schema.FieldKindReference || !slices.Contains(f2.Targets, loc.nodeType.Name) {
					continue
				}
				arr, ok := doc[t2.PluralKey].([]any)
				if !ok {
					continue
				}
				for _, item := range arr {
					obj, ok := item.(map[string]any)
					if !ok {
						continue
					}
					for _, v := range fieldValues(obj, f2.Name) {
						if v != nodeID {
							continue
						}
						srcID, _ := obj["id"].(string)
						srcName, _ := obj["name"].(string)
						out = append(out, owedLink{field: f2.Name, id: srcID, name: srcName})
					}
				}
			}
		}
	}

	return out
}

// fieldValues reads field off obj as a list of identity-hash strings
// regardless of the reference field's declared cardinality: a "one" field
// carries a single string, a "many" field a JSON array of strings — or a
// []string when the entry has not been through JSON yet, which is the shape
// NodeEditor's convertFieldValue leaves on a node being added.
func fieldValues(obj map[string]any, field string) []string {
	v, ok := obj[field]
	if !ok {
		return nil
	}
	switch x := v.(type) {
	case string:
		if x == "" {
			return nil
		}
		return []string{x}
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// findEntryObject returns the full entry object of doc's pluralKey array
// whose "id" equals id, or nil when no such entry exists.
func findEntryObject(doc map[string]any, pluralKey, id string) map[string]any {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return nil
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := obj["id"].(string); s == id {
			return obj
		}
	}
	return nil
}

// findNodeName searches mem's project.json and every module.json it
// declares for an entry whose "id" equals id, generically over every
// top-level array (mirroring findDeclaringFile in obligation_reporter.go,
// which does the same walk for "does this id exist" rather than "what is
// this id's name"), returning its "name" field.
func findNodeName(mem validator.MemFS, id string) (string, bool) {
	if name, ok := findNameInDoc(mem, "project.json", id); ok {
		return name, true
	}
	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err == nil {
		for _, mod := range proj.Modules {
			if name, ok := findNameInDoc(mem, path.Join(mod.Path, "module.json"), id); ok {
				return name, true
			}
		}
	}
	return "", false
}

// findNameInDoc is findNodeName's single-document search.
func findNameInDoc(mem validator.MemFS, key, id string) (string, bool) {
	doc, err := decodeDoc(mem, key)
	if err != nil {
		return "", false
	}
	for _, v := range doc {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		for _, item := range arr {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if s, _ := obj["id"].(string); s == id {
				name, _ := obj["name"].(string)
				return name, true
			}
		}
	}
	return "", false
}

// buildSkeleton renders a content leaf skeleton: the title heading, one
// "##" heading per entry of sections in order (none when sections is
// empty), and one placeholder line per entry of edges — a typed link
// [[<id>|<name>]] the link checker resolves, labelled by the field that
// owes it, so the write satisfies the link obligation by construction and
// the placeholder is what an agent replaces with the sentence that
// discusses the target (arch_leaf_scaffolder.md, "What a skeleton is" and
// "The line it does not cross").
func buildSkeleton(name string, sections []string, edges []owedLink) []byte {
	var b strings.Builder
	b.WriteString("# " + name + "\n")
	for _, s := range sections {
		b.WriteString("\n## " + s + "\n")
	}
	for _, e := range edges {
		b.WriteString(fmt.Sprintf("\n%s: [[%s|%s]]\n", e.field, e.id, e.name))
	}
	return []byte(b.String())
}

// nodeNotFoundRefusal is Scaffold's own guard for a ScaffoldInput.ID that
// names nothing in the tree: the fix lists every array locateNode searched,
// by file and plural key, the same way referenceTargetFix
// (obligation_reporter.go) names "the array that was searched" for a
// dangling reference target (arch_leaf_scaffolder.md, "Refusals and
// idempotence": "a node that does not exist is refused with the array that
// was searched").
func nodeNotFoundRefusal(mem validator.MemFS, profile *schema.Profile, id string) RefusalEntry {
	searched := []string{"project.json:/modules"}
	for _, t := range profile.NodeTypes {
		if t.Scope == "project" {
			searched = append(searched, "project.json:/"+t.PluralKey)
		}
	}
	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err == nil {
		for _, mod := range proj.Modules {
			for _, t := range profile.NodeTypes {
				if t.Scope == "module" {
					searched = append(searched, path.Join(mod.Path, "module.json")+":/"+t.PluralKey)
				}
			}
		}
	}
	return RefusalEntry{
		Check:   "scaffold",
		Message: fmt.Sprintf("no node with id %s found", id),
		Path:    "project.json",
		Fix:     "searched " + strings.Join(searched, ", "),
	}
}

// notContentBearingRefusal is Scaffold's own guard for a node whose
// profile-declared type carries no content leaf (an api under the default
// profile) or that names a module (which is never a profile-declared node
// type and so never content-bearing either): the fix lists the
// content-bearing types the profile does declare
// (arch_leaf_scaffolder.md, "A node of a type the profile marks as not
// content-bearing ... has no leaf to scaffold; the refusal lists the
// content-bearing types").
func notContentBearingRefusal(id string, loc nodeLocation, profile *schema.Profile) RefusalEntry {
	var bearing []string
	for _, t := range profile.NodeTypes {
		if t.RequiresContent {
			bearing = append(bearing, t.Name)
		}
	}
	typeName := "module"
	refPath := "project.json:/modules"
	if !loc.isModule {
		typeName = loc.nodeType.Name
		refPath = loc.ownerFile + ":/" + loc.nodeType.PluralKey
	}
	return RefusalEntry{
		Check:   "scaffold",
		Message: fmt.Sprintf("%s is type %q, which carries no content leaf", id, typeName),
		Path:    refPath,
		Fix:     "content-bearing types: " + strings.Join(bearing, ", "),
	}
}

// nonEmptyLeafRefusal is LeafScaffolder's own guard for a leaf that already
// holds bytes other than exactly the skeleton it would write: prose is the
// one thing in the tree the tool must never destroy
// (arch_leaf_scaffolder.md, "Refusals and idempotence": "the refusal names
// the file and says to empty or move it"). Both `spex leaf scaffold` (via
// Scaffold) and `spex node add` (via Add, for the content-bearing node it
// declares) hit this same guard — command names the fix as the one to
// re-run once the file is out of the way.
func nonEmptyLeafRefusal(contentPath, command string) RefusalEntry {
	return RefusalEntry{
		Check:   "scaffold",
		Message: fmt.Sprintf("%s is not empty; refusing to overwrite it", contentPath),
		Path:    contentPath,
		Fix:     fmt.Sprintf("empty %s or move it aside, then re-run %s", contentPath, command),
	}
}
