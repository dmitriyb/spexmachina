package author

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// moduleLocalFields is the fixed set of reference fields whose target must
// lie in the source's own module: under the default profile, provided_by is
// the only one. Unlike the other three checks EdgeEditor runs, module
// locality is not something the resolved profile declares anywhere — it is
// the frame's own fixed rule, mirrored from validator/id_validator.go's
// checkModuleRefs ("provided_by is module-local by design") — so it is
// hardcoded here exactly as it is hardcoded there
// (spec/author/arch_edge_editor.md, "a module-local field stays
// module-local — provided_by under the default profile").
var moduleLocalFields = map[string]bool{"provided_by": true}

// AddEdge is EdgeEditor's half of spec/author/arch_edge_editor.md: given the
// spec directory and an EdgeInput naming a source node, a reference field
// and a target node, all by id, it runs the four checks the editor owns
// itself, before ever building an after-state — the target exists, the
// source type declares the field as a reference kind, the field permits the
// target's type, and a moduleLocalFields entry stays within the source's
// own module — then builds the after-state and hands it to Report for the
// fifth check, acyclicity, and the completeness obligations
// (arch_edge_editor.md, "One entry, four checks").
//
// Three outcomes:
//
//   - A write or a no-op: (report, nil, nil). report.Written is empty when
//     the field already held the entry — "adding an entry the field
//     already holds changes nothing and says so"
//     (arch_edge_editor.md, "Idempotence") — never a refusal. For a
//     cardinality-one field that already held a different target, the
//     write replaces it and report.ReplacedTarget carries the target it
//     displaced, so the retarget is visible rather than silent.
//   - A refusal: (nil, refusals, nil). Nothing is written: either one of
//     EdgeEditor's own four checks, or whatever Report's validator pass
//     finds newly wrong with the after-state (only acyclicity, by
//     construction — the other four checks already passed).
//   - An input error: (nil, nil, err). input.SourceID names no node
//     (module or profile-declared) anywhere in the tree.
func AddEdge(specDir string, input EdgeInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}
	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}

	srcLoc, err := locateNode(before, profile, input.SourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}
	srcTypeName := edgeTypeName(srcLoc)

	field, declared := edgeField(srcLoc, input.Field)
	if !declared {
		return nil, []RefusalEntry{undeclaredEdgeFieldRefusal(srcLoc, srcTypeName, input)}, nil
	}

	tgtLoc, tgtErr := locateNode(before, profile, input.TargetID)
	if tgtErr != nil {
		return nil, []RefusalEntry{targetMissingRefusal(before, profile, srcLoc, input, field)}, nil
	}
	tgtTypeName := edgeTypeName(tgtLoc)
	if !slices.Contains(field.Targets, tgtTypeName) {
		return nil, []RefusalEntry{targetTypeRefusal(srcLoc, input, field, tgtTypeName)}, nil
	}

	if moduleLocalFields[input.Field] && tgtLoc.moduleDir != srcLoc.moduleDir {
		return nil, []RefusalEntry{moduleLocalRefusal(before, profile, srcLoc, tgtLoc, input)}, nil
	}

	ownerFile := edgeOwnerFile(srcLoc)
	pluralKey := edgePluralKey(srcLoc)

	after := cloneSpecFS(before)
	doc, err := decodeDoc(after, ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}
	entry, ok := findEntryMap(doc, pluralKey, input.SourceID)
	if !ok {
		return nil, nil, fmt.Errorf("author: edge add: %s not found in %s", input.SourceID, ownerFile)
	}

	changed, replaced := setEdgeField(entry, field, input.TargetID)
	if !changed {
		return &WriteReport{}, nil, nil
	}

	data, err := marshalIndentNoEscape(canonicalizeDoc(doc, ownerFile, profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: marshal %s: %w", ownerFile, err)
	}
	after[ownerFile] = append(data, '\n')

	refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: edge add: %w", err)
	}

	return &WriteReport{Written: written, Obligations: obligations, ReplacedTarget: replaced}, nil, nil
}

// RemoveEdge is EdgeEditor's other half: given the spec directory and an
// EdgeInput, it clears one entry of one reference field. Unlike AddEdge, it
// never checks that the target exists or that its type is still permitted —
// the whole point of spex edge remove is retargeting a dangling reference
// (arch_node_editor.md's N6 fix: "the spex edge remove invocation that
// would retarget it"), so a target already gone from the tree must still be
// removable. It still checks that the source type declares the field as a
// reference kind — there is no entry to search for one it does not
// declare — and still passes the after-state through Report for the
// completeness obligations (and, in principle, acyclicity, though removing
// an edge can only shrink a graph).
//
// Three outcomes, the same shapes as AddEdge: a write or a no-op
// (report.Written empty when the field never held the target — "removing
// one the field does not hold changes nothing and says so",
// arch_edge_editor.md's "Idempotence"); a refusal; an input error when
// input.SourceID names no node in the tree.
func RemoveEdge(specDir string, input EdgeInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}
	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}

	srcLoc, err := locateNode(before, profile, input.SourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}
	srcTypeName := edgeTypeName(srcLoc)

	field, declared := edgeField(srcLoc, input.Field)
	if !declared {
		return nil, []RefusalEntry{undeclaredEdgeFieldRefusal(srcLoc, srcTypeName, input)}, nil
	}

	ownerFile := edgeOwnerFile(srcLoc)
	pluralKey := edgePluralKey(srcLoc)

	after := cloneSpecFS(before)
	doc, err := decodeDoc(after, ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}
	entry, ok := findEntryMap(doc, pluralKey, input.SourceID)
	if !ok {
		return nil, nil, fmt.Errorf("author: edge remove: %s not found in %s", input.SourceID, ownerFile)
	}

	if !clearEdgeField(entry, field, input.TargetID) {
		return &WriteReport{}, nil, nil
	}

	data, err := marshalIndentNoEscape(canonicalizeDoc(doc, ownerFile, profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: marshal %s: %w", ownerFile, err)
	}
	after[ownerFile] = append(data, '\n')

	refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: edge remove: %w", err)
	}

	return &WriteReport{Written: written, Obligations: obligations}, nil, nil
}

// edgeTypeName is loc's node type name for edge-checking purposes: "module"
// for a module (never a profile-declared NodeType, but always a legal
// From/To entry per schema.Profile.Edges' own derived view), loc.nodeType's
// own declared name otherwise.
func edgeTypeName(loc nodeLocation) string {
	if loc.isModule {
		return "module"
	}
	return loc.nodeType.Name
}

// edgeOwnerFile is the file EdgeEditor writes loc's own entry into:
// project.json for a module (requires_module lives on the modules array
// entry itself), loc.ownerFile otherwise.
func edgeOwnerFile(loc nodeLocation) string {
	if loc.isModule {
		return "project.json"
	}
	return loc.ownerFile
}

// edgePluralKey is the array loc's own entry lives in: "modules" for a
// module, loc.nodeType.PluralKey otherwise.
func edgePluralKey(loc nodeLocation) string {
	if loc.isModule {
		return "modules"
	}
	return loc.nodeType.PluralKey
}

// entryPathFor is the schema-checker-style path naming loc's own entry —
// the same "<file>:/<pluralKey>/<id>" shape validator's own findings use, so
// a refusal EdgeEditor raises itself points at the same place a Report
// refusal would.
func entryPathFor(loc nodeLocation, id string) string {
	return fmt.Sprintf("%s:/%s/%s", edgeOwnerFile(loc), edgePluralKey(loc), id)
}

// edgeField resolves fieldName as a reference-kind field declared on loc's
// node type: requires_module for a module — the frame's own fixed field,
// never a profile declaration (arch_edge_editor.md, "requires_module is the
// one field that is not a profile declaration: modules are frame nodes and
// the edge belongs to the frame") — or whatever loc.nodeType's own declared
// fields carry otherwise. Returns false when fieldName is undeclared on
// loc's type, or declared there but not of reference kind.
func edgeField(loc nodeLocation, fieldName string) (schema.Field, bool) {
	if loc.isModule {
		if fieldName != "requires_module" {
			return schema.Field{}, false
		}
		return schema.Field{
			Name:        "requires_module",
			Kind:        schema.FieldKindReference,
			Targets:     []string{"module"},
			Cardinality: "many",
		}, true
	}
	f, ok := findField(loc.nodeType, fieldName)
	if !ok || f.Kind != schema.FieldKindReference {
		return schema.Field{}, false
	}
	return f, true
}

// declaredReferenceFields lists the reference-kind field names loc's node
// type declares — the fix undeclaredEdgeFieldRefusal carries, per
// arch_edge_editor.md's four-checks table: "the reference fields the
// profile declares on that type".
func declaredReferenceFields(loc nodeLocation) []string {
	if loc.isModule {
		return []string{"requires_module"}
	}
	var names []string
	for _, f := range loc.nodeType.Fields {
		if f.Kind == schema.FieldKindReference {
			names = append(names, f.Name)
		}
	}
	return names
}

// findEntryMap searches doc's array at pluralKey for the entry whose "id"
// equals id, returning the entry's own map — a live reference into doc, so
// mutating it (setEdgeField, clearEdgeField) mutates doc in place without a
// second write-back step.
func findEntryMap(doc map[string]any, pluralKey, id string) (map[string]any, bool) {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return nil, false
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := obj["id"].(string); s == id {
			return obj, true
		}
	}
	return nil, false
}

// setEdgeField adds targetID to entry's value for field, per field's
// declared cardinality (arch_edge_editor.md, "Idempotence"): cardinality
// "one" sets the field when it is empty, replaces it when it already holds
// a different target — reporting the displaced value as replaced, so the
// retarget is visible rather than silent — and reports no change when it
// already holds targetID; cardinality "many" appends when targetID is
// absent and reports no change when it is already present.
func setEdgeField(entry map[string]any, field schema.Field, targetID string) (changed bool, replaced string) {
	if field.Cardinality == "one" {
		cur, _ := entry[field.Name].(string)
		if cur == targetID {
			return false, ""
		}
		entry[field.Name] = targetID
		return true, cur
	}
	arr, _ := entry[field.Name].([]any)
	if slices.ContainsFunc(arr, func(v any) bool { s, ok := v.(string); return ok && s == targetID }) {
		return false, ""
	}
	entry[field.Name] = append(arr, targetID)
	return true, ""
}

// clearEdgeField removes targetID from entry's value for field, per field's
// declared cardinality, reporting whether anything changed. A cardinality-
// "one" field is cleared by deleting the key entirely rather than setting
// it to "", so a subsequent schema check sees an absent field, not an empty
// string one. clearEdgeField itself does not distinguish a required field
// from an optional one: RemoveEdge always builds the after-state and hands
// it to Report, so clearing a required cardinality-one field (e.g.
// preq_id) surfaces as a refusal there — the validator's own "schema"
// entry (missing required property) and "id" entry (the node missing its
// preq_id), both newly introduced by the after-state — exactly per
// arch_edge_editor.md's "Idempotence": "for a required field that is a
// refusal carrying the validator's schema and id entries ... never through
// a cleared state it cannot reach." No separate check is needed here.
func clearEdgeField(entry map[string]any, field schema.Field, targetID string) bool {
	if field.Cardinality == "one" {
		cur, _ := entry[field.Name].(string)
		if cur != targetID {
			return false
		}
		delete(entry, field.Name)
		return true
	}
	arr, ok := entry[field.Name].([]any)
	if !ok {
		return false
	}
	out := make([]any, 0, len(arr))
	removed := false
	for _, v := range arr {
		if s, ok := v.(string); ok && s == targetID {
			removed = true
			continue
		}
		out = append(out, v)
	}
	if !removed {
		return false
	}
	if len(out) == 0 {
		delete(entry, field.Name)
	} else {
		entry[field.Name] = out
	}
	return true
}

// undeclaredEdgeFieldRefusal is EdgeEditor's own check that the source's
// type declares fieldName as a reference-kind field (arch_edge_editor.md's
// four-checks table, "the source type declares the field, as a reference
// kind"). The fix lists every reference field the profile does declare on
// that type.
func undeclaredEdgeFieldRefusal(loc nodeLocation, srcTypeName string, input EdgeInput) RefusalEntry {
	fields := declaredReferenceFields(loc)
	return RefusalEntry{
		Check:   "edge",
		Message: fmt.Sprintf("%q does not declare %q as a reference field", srcTypeName, input.Field),
		Path:    entryPathFor(loc, input.SourceID),
		Fix:     fmt.Sprintf("%s declares reference fields: %s", srcTypeName, strings.Join(fields, ", ")),
	}
}

// targetMissingRefusal is EdgeEditor's own check that the target exists
// anywhere in the tree (arch_edge_editor.md's four-checks table, "the
// target exists"). The fix names, by file and key, every array a target of
// one of field's permitted types could have been declared in.
func targetMissingRefusal(before validator.MemFS, profile *schema.Profile, loc nodeLocation, input EdgeInput, field schema.Field) RefusalEntry {
	locations := targetSearchLocations(before, profile, field)
	return RefusalEntry{
		Check:   "edge",
		Message: fmt.Sprintf("%s references non-existent %s %s", input.Field, strings.Join(field.Targets, "/"), input.TargetID),
		Path:    entryPathFor(loc, input.SourceID),
		Fix:     fmt.Sprintf("no node %s found; searched %s", input.TargetID, strings.Join(locations, ", ")),
	}
}

// targetTypeRefusal is EdgeEditor's own check that field permits the
// target's actual type (arch_edge_editor.md's four-checks table, "the field
// permits the target's type"). The fix names the target types field
// permits.
func targetTypeRefusal(loc nodeLocation, input EdgeInput, field schema.Field, actualType string) RefusalEntry {
	return RefusalEntry{
		Check:   "edge",
		Message: fmt.Sprintf("%s does not permit target type %q (%s is a %s)", input.Field, actualType, input.TargetID, actualType),
		Path:    entryPathFor(loc, input.SourceID),
		Fix:     fmt.Sprintf("%s may target: %s", input.Field, strings.Join(field.Targets, ", ")),
	}
}

// moduleLocalRefusal is EdgeEditor's own check for a moduleLocalFields entry
// whose target lies outside the source's own module (arch_edge_editor.md's
// four-checks table, "a module-local field stays module-local"). Check and
// Message mirror validator/id_validator.go's own checkModuleRefs finding
// exactly — the same rule, checked before the write instead of after it —
// so the parity oracle test_node_editing.md's N12 asks for holds: "the
// error is the validator's provided_by is module-local message". The fix
// names the components of the source's own module, the permissible
// targets.
func moduleLocalRefusal(before validator.MemFS, profile *schema.Profile, srcLoc, tgtLoc nodeLocation, input EdgeInput) RefusalEntry {
	field, _ := edgeField(srcLoc, input.Field)
	permitted := moduleLocalPermittedTargets(before, profile, srcLoc.moduleDir, field)
	return RefusalEntry{
		Check:   "id",
		Message: fmt.Sprintf("%s references non-existent %s %s (%s is module-local)", input.Field, tgtLoc.nodeType.Name, input.TargetID, input.Field),
		Path:    entryPathFor(srcLoc, input.SourceID),
		Fix:     fmt.Sprintf("%s is module-local; %s's own components: %s", input.Field, srcLoc.moduleName, strings.Join(permitted, ", ")),
	}
}

// targetSearchLocations lists, by file and key, every array a target of one
// of field's permitted types would be declared in: project.json's own array
// for a project-scoped type, every module's own array for a module-scoped
// type, and project.json's "modules" array for "module" itself —
// targetMissingRefusal's fix, naming where EdgeEditor looked.
func targetSearchLocations(before validator.MemFS, profile *schema.Profile, field schema.Field) []string {
	var locations []string
	for _, target := range field.Targets {
		if target == "module" {
			locations = append(locations, "project.json:/modules")
			continue
		}
		for _, nt := range profile.NodeTypes {
			if nt.Name != target {
				continue
			}
			if nt.Scope == "project" {
				locations = append(locations, fmt.Sprintf("project.json:/%s", nt.PluralKey))
				continue
			}
			for _, dir := range moduleDirs(before) {
				locations = append(locations, fmt.Sprintf("%s:/%s", path.Join(dir, "module.json"), nt.PluralKey))
			}
		}
	}
	return locations
}

// moduleDirs lists every module's own directory path, read from
// project.json's modules array.
func moduleDirs(mem validator.MemFS) []string {
	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err != nil {
		return nil
	}
	dirs := make([]string, 0, len(proj.Modules))
	for _, m := range proj.Modules {
		dirs = append(dirs, m.Path)
	}
	return dirs
}

// moduleLocalPermittedTargets lists the names of moduleDir's own entries of
// field's permitted types — moduleLocalRefusal's fix, "the components of
// the source's own module" (arch_edge_editor.md's four-checks table).
func moduleLocalPermittedTargets(before validator.MemFS, profile *schema.Profile, moduleDir string, field schema.Field) []string {
	doc, err := decodeDoc(before, path.Join(moduleDir, "module.json"))
	if err != nil {
		return nil
	}
	var names []string
	for _, target := range field.Targets {
		for _, nt := range profile.NodeTypes {
			if nt.Name != target || nt.Scope != "module" {
				continue
			}
			arr, _ := doc[nt.PluralKey].([]any)
			for _, item := range arr {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if n, _ := obj["name"].(string); n != "" {
					names = append(names, n)
				}
			}
		}
	}
	return names
}
