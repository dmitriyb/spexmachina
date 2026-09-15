package author

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// Add is NodeEditor's half of spec/author/arch_node_editor.md, "Adding a
// node": given the spec directory and a NodeAddInput naming a type, a name,
// a module (for a module-scoped type) and one value per declared field, it
// decides everything structural the caller does not name — which file,
// which array, the derived id, and, for a content-bearing type, the
// conventional content path and its scaffolded leaf — from the resolved
// profile and the identity contract, applies the change to an in-memory
// copy, and passes it through Report before any of it reaches disk.
// input.TypeName "module" is the one addition outside the profile's own
// types (arch_node_editor.md, "Adding a node": "the one addition outside
// the profile's types, because modules are frame nodes, not declared
// ones"), handled by addModule instead of the generic path below.
//
// Three outcomes:
//
//   - A write: (report, nil, nil).
//   - A refusal: (nil, refusals, nil). Nothing is written. An undeclared
//     type is NodeEditor's own guard, carrying the profile's declared list
//     as the fix — no validator check exists for "this type does not
//     exist" before any array to check even has a name. Every other
//     refusal — a missing required field, a name the declarability
//     tokenizer would not reproduce, a duplicate id — is Report's own,
//     surfaced unchanged.
//   - An input error: (nil, nil, err). input.Module names no module in the
//     tree, a module-scoped type is named with no module given, or a field
//     value cannot be parsed as its declared kind (e.g. a non-integer value
//     for an integer field).
func Add(specDir string, input NodeAddInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}

	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}

	if input.TypeName == "module" {
		return addModule(specDir, before, profile, input)
	}

	nt, moduleRequired, ok := resolveAddNodeType(profile, input.TypeName, input.Module)
	if !ok {
		return nil, []RefusalEntry{undeclaredNodeTypeRefusal(input.TypeName, profile)}, nil
	}
	if moduleRequired {
		return nil, nil, fmt.Errorf("author: node add: --module is required for type %q", input.TypeName)
	}

	ownerFile := "project.json"
	moduleDir := ""
	idParts := []string{"project", nt.Name, input.Name}
	if nt.Scope == "module" {
		dir, found := findModulePath(before, input.Module)
		if !found {
			return nil, nil, fmt.Errorf("author: node add: no module named %q", input.Module)
		}
		moduleDir = dir
		ownerFile = path.Join(moduleDir, "module.json")
		idParts = []string{input.Module, nt.Name, input.Name}
	}
	id := schema.IdentityHash(idParts...)

	entry := map[string]any{"id": id, "name": input.Name}

	var contentPath string
	if nt.RequiresContent {
		prefix := ""
		if nt.ContentPrefix != nil {
			prefix = *nt.ContentPrefix
		}
		contentRel := prefix + snakeCase(input.Name) + ".md"
		entry["content"] = contentRel
		contentPath = path.Join(moduleDir, contentRel)
	}

	for name, raw := range input.Fields {
		f, declared := findField(nt, name)
		if !declared {
			// An undeclared field name is not this function's to refuse: it
			// goes onto the entry as given, so Report's own schema check
			// refuses it as an additional property, with the type's declared
			// fields as the fix (obligation_reporter.go's
			// disallowedFieldFix) — the same "refuses a field the type does
			// not declare" behaviour a required field's absence already
			// gets from the same checker.
			entry[name] = raw
			continue
		}
		val, err := convertFieldValue(f, raw)
		if err != nil {
			return nil, nil, fmt.Errorf("author: node add: %w", err)
		}
		entry[name] = val
	}

	after := cloneSpecFS(before)
	doc, err := decodeDoc(after, ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}
	arr, _ := doc[nt.PluralKey].([]any)
	doc[nt.PluralKey] = append(arr, entry)
	data, err := marshalIndentNoEscape(canonicalizeDoc(doc, ownerFile, profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: marshal %s: %w", ownerFile, err)
	}
	after[ownerFile] = append(data, '\n')

	if nt.RequiresContent {
		if existing, exists := before[contentPath]; exists && len(existing) > 0 {
			return nil, []RefusalEntry{nonEmptyLeafRefusal(contentPath, "spex node add")}, nil
		}
		loc := nodeLocation{
			projectScope: nt.Scope == "project",
			moduleName:   input.Module,
			moduleDir:    moduleDir,
			ownerFile:    ownerFile,
			nodeType:     nt,
			name:         input.Name,
		}
		edges := owedEdges(after, profile, loc, entry, id)
		after[contentPath] = buildSkeleton(input.Name, nt.LeafSections, edges)
	}

	refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}

	return &WriteReport{Written: written, Obligations: obligations}, nil, nil
}

// addModule is Add's path for input.TypeName == "module": it appends an
// entry to project.json's modules array — name, path equal to the name, the
// derived module id — and writes a module.json skeleton declaring the name
// and nothing else, so the module is visible to every gate from its first
// moment (arch_node_editor.md, "Adding a node": "a directory project.json
// does not name is invisible to spex validate, spex diff and spex render
// alike, and creating the two together is what closes that hole").
func addModule(specDir string, before validator.MemFS, profile *schema.Profile, input NodeAddInput) (*WriteReport, []RefusalEntry, error) {
	id := schema.IdentityHash("module", input.Name)

	after := cloneSpecFS(before)
	proj, err := decodeDoc(after, "project.json")
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}
	modules, _ := proj["modules"].([]any)
	proj["modules"] = append(modules, map[string]any{
		"id":   id,
		"name": input.Name,
		"path": input.Name,
	})
	data, err := marshalIndentNoEscape(canonicalizeDoc(proj, "project.json", profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: marshal project.json: %w", err)
	}
	after["project.json"] = append(data, '\n')

	modPath := path.Join(input.Name, "module.json")
	if existing, exists := before[modPath]; exists && len(existing) > 0 {
		return nil, []RefusalEntry{moduleSkeletonExistsRefusal(modPath)}, nil
	}
	modSkeleton, err := marshalIndentNoEscape(canonicalizeDoc(map[string]any{"name": input.Name}, modPath, profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: marshal %s: %w", modPath, err)
	}
	after[modPath] = append(modSkeleton, '\n')

	refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: node add: %w", err)
	}

	return &WriteReport{Written: written, Obligations: obligations}, nil, nil
}

// resolveAddNodeType finds typeName among the resolved profile's declared
// node types and decides which of its (up to two) scope declarations Add
// should use: module-scoped when module is named and a module-scoped
// declaration exists, project-scoped otherwise — "a type declared at both
// scopes lands by whether a module was named" (arch_node_editor.md,
// "Adding a node"). Returns (zero, false, false) when typeName is
// undeclared at either scope; (zero, true, true) when only a module-scoped
// declaration exists and no module was named, so the caller can report the
// missing --module as an input error rather than a refusal — no profile
// list would fix it, only the flag would.
func resolveAddNodeType(profile *schema.Profile, typeName, module string) (schema.NodeType, bool, bool) {
	var projectNT, moduleNT *schema.NodeType
	for i := range profile.NodeTypes {
		nt := profile.NodeTypes[i]
		if nt.Name != typeName {
			continue
		}
		switch nt.Scope {
		case "project":
			t := nt
			projectNT = &t
		case "module":
			t := nt
			moduleNT = &t
		}
	}
	if projectNT == nil && moduleNT == nil {
		return schema.NodeType{}, false, false
	}
	if moduleNT != nil && module != "" {
		return *moduleNT, false, true
	}
	if projectNT != nil {
		return *projectNT, false, true
	}
	return schema.NodeType{}, true, true
}

// undeclaredNodeTypeRefusal is Add's own guard for a type the resolved
// profile does not declare at any scope: the fix lists every type the
// profile does declare, plus "module" — the frame's own fixed type, never a
// profile declaration but always legal (arch_node_editor.md, "A type the
// profile does not declare is refused with the declared list as the fix").
func undeclaredNodeTypeRefusal(typeName string, profile *schema.Profile) RefusalEntry {
	return RefusalEntry{
		Check:   "node",
		Message: fmt.Sprintf("%q is not a type the resolved profile declares", typeName),
		Path:    "profile.json",
		Fix:     "declared types: " + strings.Join(declaredTypeNames(profile), ", "),
	}
}

// moduleSkeletonExistsRefusal is addModule's own guard for a modPath that
// already holds a non-empty module.json: a directory a project.json entry
// is about to name for the first time may already carry a hand-authored,
// not-yet-registered module.json (arch_node_editor.md, "a directory
// project.json does not name is invisible" — the adopter case), and
// overwriting it with the bare `{"name": ...}` skeleton would destroy every
// declaration already in it, the same destruction LeafScaffolder's own
// non-overwrite guard refuses for a content leaf (nonEmptyLeafRefusal).
func moduleSkeletonExistsRefusal(modPath string) RefusalEntry {
	return RefusalEntry{
		Check:   "node",
		Message: fmt.Sprintf("%s is not empty; refusing to overwrite it", modPath),
		Path:    modPath,
		Fix:     fmt.Sprintf("empty %s or move it aside, then re-run spex node add --type module", modPath),
	}
}

// declaredTypeNames lists the resolved profile's declared node type names,
// each once even when a name is declared for both scopes (requirement),
// in declaration order, plus the fixed "module" type last — the same set
// hash-id's own validTypeList (cmd/spex/hashid.go) reports for an unknown
// --type.
func declaredTypeNames(profile *schema.Profile) []string {
	seen := make(map[string]bool, len(profile.NodeTypes)+1)
	names := make([]string, 0, len(profile.NodeTypes)+1)
	for _, nt := range profile.NodeTypes {
		if seen[nt.Name] {
			continue
		}
		seen[nt.Name] = true
		names = append(names, nt.Name)
	}
	return append(names, "module")
}

// findField looks up name among nt's declared fields.
func findField(nt schema.NodeType, name string) (schema.Field, bool) {
	for _, f := range nt.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return schema.Field{}, false
}

// findModulePath searches mem's project.json for a module named name,
// returning its declared path.
func findModulePath(mem validator.MemFS, name string) (string, bool) {
	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err != nil {
		return "", false
	}
	for _, m := range proj.Modules {
		if m.Name == name {
			return m.Path, true
		}
	}
	return "", false
}

// convertFieldValue turns raw's flag-string value into the shape f's kind
// composes into JSON: a parsed integer for an integer field, a bare string
// for a cardinality-"one" reference or a plain text field, and a
// comma-separated list of identity hashes for a cardinality-"many"
// reference (empty segments dropped, each trimmed of surrounding
// whitespace).
func convertFieldValue(f schema.Field, raw string) (any, error) {
	switch f.Kind {
	case schema.FieldKindInteger:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("field %q: invalid integer %q", f.Name, raw)
		}
		return n, nil
	case schema.FieldKindReference:
		if f.Cardinality == "one" {
			return raw, nil
		}
		var vals []string
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				vals = append(vals, part)
			}
		}
		return vals, nil
	default:
		return raw, nil
	}
}

// Remove is NodeEditor's half of spec/author/arch_node_editor.md, "Removing
// a node": given the spec directory and a NodeRemoveInput naming a node's
// id, it deletes the node's entry and its content file, then sweeps the
// tree for what still names it — every reference field entry carrying the
// id and every typed link naming it — exactly the findings Report's own
// checkers raise against the after-state, since the before-state carried no
// dangling reference and now does. Without --force any such finding refuses
// the removal, the validator's own message and fix unchanged (the same
// "spex edge remove ... or spex node remove --force" phrasing
// computeFix's referenceTargetFix already computes for a target that
// existed before the change). With --force the removal proceeds regardless,
// and the same findings are reported instead of refused, as obligations —
// "spex validate will hold the tree red until they do it"
// (arch_node_editor.md, "Removing a node").
//
// A module id is refused, forced or not: modules are frame nodes, and
// retiring one is a project.json edit, a directory removal and a
// requires_module sweep by hand, not a single-node edit.
//
// A successful removal of a name-declarable node type (component or api
// under the default profile) sets the report's RetiredName, so the
// vocabulary sweep spex diff runs afterward starts from the command's own
// output rather than from memory (arch_node_editor.md, "Removing a node").
//
// Three outcomes: a write ((*WriteReport, nil, nil)); a refusal
// ((nil, []RefusalEntry, nil), nothing written); an input error
// ((nil, nil, err)) when input.ID names no node in the tree.
func Remove(specDir string, input NodeRemoveInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}

	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}

	loc, err := locateNode(before, profile, input.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}
	if loc.isModule {
		return nil, []RefusalEntry{moduleRemoveRefusal(input.ID)}, nil
	}

	ownerDoc, err := decodeDoc(before, loc.ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}

	after := cloneSpecFS(before)
	afterDoc, err := decodeDoc(after, loc.ownerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}
	removeEntry(afterDoc, loc.nodeType.PluralKey, input.ID)
	data, err := marshalIndentNoEscape(canonicalizeDoc(afterDoc, loc.ownerFile, profile))
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: marshal %s: %w", loc.ownerFile, err)
	}
	after[loc.ownerFile] = append(data, '\n')

	var contentPath string
	if loc.nodeType.RequiresContent {
		contentRel, _ := findEntryField(ownerDoc, loc.nodeType.PluralKey, input.ID, "content")
		contentPath = path.Join(loc.moduleDir, contentRel)
		delete(after, contentPath)
	}

	if !input.Force {
		refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
		if err != nil {
			return nil, nil, fmt.Errorf("author: node remove: %w", err)
		}
		if len(refusals) > 0 {
			return nil, refusals, nil
		}
		return finishRemove(specDir, before, after, contentPath, loc, obligations)
	}

	introduced := introducedFindings(os.DirFS(specDir), after)
	completeness, err := completenessObligations(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}
	var obligations []merkle.DiffError
	obligations = append(obligations, completeness...)
	obligations = append(obligations, validatorObligations(nonRefusalCheckers(after))...)
	obligations = append(obligations, validatorObligations(introduced)...)

	return finishRemove(specDir, before, after, contentPath, loc, obligations)
}

// finishRemove writes after's changed files to disk, removes a
// content-bearing node's leaf, and builds the resulting WriteReport —
// Remove's shared tail for both the refusal-gated and the forced path.
func finishRemove(specDir string, before, after validator.MemFS, contentPath string, loc nodeLocation, obligations []merkle.DiffError) (*WriteReport, []RefusalEntry, error) {
	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: node remove: %w", err)
	}
	if contentPath != "" {
		if err := os.Remove(filepath.Join(specDir, filepath.FromSlash(contentPath))); err != nil && !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("author: node remove: remove %s: %w", contentPath, err)
		}
	}

	report := &WriteReport{Written: written, Obligations: obligations}
	if loc.nodeType.NameDeclarable {
		report.RetiredName = loc.name
	}
	return report, nil, nil
}

// removeEntry deletes the entry of doc's pluralKey array whose "id" equals
// id, reporting whether one was found and removed.
func removeEntry(doc map[string]any, pluralKey, id string) bool {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return false
	}
	out := make([]any, 0, len(arr))
	removed := false
	for _, item := range arr {
		if obj, ok := item.(map[string]any); ok {
			if s, _ := obj["id"].(string); s == id {
				removed = true
				continue
			}
		}
		out = append(out, item)
	}
	doc[pluralKey] = out
	return removed
}

// moduleRemoveRefusal is Remove's own guard for input.ID naming a module,
// refused forced or not: a module's identity is a frame concept — the
// project.json entry, the directory, and every requires_module edge naming
// it — never a single node NodeEditor's generic removal path could safely
// retire, so it names the three hand edits instead
// (arch_node_editor.md, "Removing a node": "A module id is refused, forced
// or not").
func moduleRemoveRefusal(id string) RefusalEntry {
	return RefusalEntry{
		Check:   "node",
		Message: fmt.Sprintf("%s is a module id; removing a module is not a single-node edit", id),
		Path:    "project.json:/modules/" + id,
		Fix:     "spex node remove does not remove modules: delete the project.json modules entry, remove the module's directory, and remove every requires_module edge naming it, by hand",
	}
}

// introducedFindings mirrors Report's own refusal-detection logic (Report,
// in obligation_reporter.go) without blocking on it: every refusalCheckers
// finding present in after and absent from before, via the same atom-based
// comparison Report itself applies (errorAtoms/allIntroduced), so a finding
// the before-state already carried never shows up here as newly dangling.
// Remove's forced path needs exactly this list — refused findings reported
// instead of blocked — while its non-forced path goes through Report itself
// and is blocked by the very same list.
func introducedFindings(before, after fs.FS) []validator.ValidationError {
	beforeErrs := refusalCheckers(before)
	afterErrs := refusalCheckers(after)

	introduced := make(map[string]bool, len(beforeErrs))
	for _, e := range beforeErrs {
		for _, atom := range errorAtoms(e) {
			introduced[atom] = true
		}
	}

	var out []validator.ValidationError
	for _, e := range afterErrs {
		if !allIntroduced(errorAtoms(e), introduced) {
			out = append(out, e)
		}
	}
	return out
}
