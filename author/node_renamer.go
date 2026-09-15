package author

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// Rename is NodeRenamer, spec/author/arch_node_renamer.md: given the spec
// directory and a RenameInput, it performs the rename as one transaction —
// derive the new id, rewrite the node's own entry (name, id, and for a
// content-bearing type the new conventional content path), rewrite every
// reference field entry naming the old id in every project.json and
// module.json in the tree, repoint every typed link naming the old id in
// every content leaf, and move the content file — all applied to an
// in-memory copy and gated through Report before any of it reaches disk.
//
// Three outcomes:
//
//   - A write or a no-op: (report, nil, nil). report.Written is empty and
//     report.RetiredName is "" for a no-op (input.NewName already named the
//     node) — arch_node_renamer.md's "What it refuses": "a new name equal
//     to the old is a no-op that says so", not a refusal.
//   - A refusal: (nil, refusals, nil). Nothing is written. A refusal is
//     either NodeRenamer's own module-id guard or whatever Report's
//     validator pass finds newly wrong with the after-state — a name
//     collision (duplicate id) or a name the declarability tokenizer would
//     not reproduce, both reported through the same "id" check `spex
//     validate` would raise by hand.
//   - An input error: (nil, nil, err). input.ID names no node in the tree.
func Rename(specDir string, input RenameInput) (*WriteReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}

	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}

	loc, err := locateNode(before, profile, input.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}
	if loc.isModule {
		return nil, []RefusalEntry{moduleRenameRefusal(input.ID)}, nil
	}
	if input.NewName == loc.name {
		return &WriteReport{}, nil, nil
	}

	docKeys, err := specDocKeys(before)
	if err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}
	docs := make(map[string]map[string]any, len(docKeys))
	for _, key := range docKeys {
		doc, err := decodeDoc(before, key)
		if err != nil {
			return nil, nil, fmt.Errorf("author: rename: %w", err)
		}
		docs[key] = doc
	}

	identityParts := []string{loc.moduleName, loc.nodeType.Name, input.NewName}
	if loc.projectScope {
		identityParts = []string{"project", loc.nodeType.Name, input.NewName}
	}
	newID := schema.IdentityHash(identityParts...)

	var oldContentFull, newContentFull string
	var newContentValue *string
	if loc.nodeType.RequiresContent {
		oldContent, _ := findEntryField(docs[loc.ownerFile], loc.nodeType.PluralKey, input.ID, "content")
		oldContentFull = path.Join(loc.moduleDir, oldContent)
		prefix := ""
		if loc.nodeType.ContentPrefix != nil {
			prefix = *loc.nodeType.ContentPrefix
		}
		nv := prefix + snakeCase(input.NewName) + ".md"
		newContentValue = &nv
		newContentFull = path.Join(loc.moduleDir, nv)

		if nv != oldContent {
			if _, occupied := before[newContentFull]; occupied {
				collidingID, _ := contentPathCollision(docs[loc.ownerFile], loc.nodeType.PluralKey, input.ID, nv)
				if collidingID != newID {
					return nil, nil, fmt.Errorf("author: rename: new content path %s already exists; refusing to overwrite it", newContentFull)
				}
			}
		}
	}

	contentLeaves := declaredContentLeaves(docs, docKeys, profile)

	refFields := referenceFieldNames(profile)
	after := cloneSpecFS(before)
	for _, key := range docKeys {
		doc := docs[key]
		changed := patchReferenceFields(doc, refFields, input.ID, newID)
		if key == loc.ownerFile {
			entryChanged := mutateEntry(doc, loc.nodeType.PluralKey, input.ID, newID, input.NewName, newContentValue)
			changed = changed || entryChanged
		}
		if !changed {
			continue
		}
		data, err := marshalIndentNoEscape(canonicalizeDoc(doc, key, profile))
		if err != nil {
			return nil, nil, fmt.Errorf("author: rename: marshal %s: %w", key, err)
		}
		after[key] = append(data, '\n')
	}

	for key := range contentLeaves {
		data, ok := after[key]
		if !ok {
			continue
		}
		updated := data
		changed := false
		if newData, ok := repointLinks(updated, input.ID, newID); ok {
			updated, changed = newData, true
		}
		if newData, ok := repointDotNodeRefs(updated, input.ID, newID); ok {
			updated, changed = newData, true
		}
		if changed {
			after[key] = updated
		}
	}

	if loc.nodeType.RequiresContent {
		content, ok := after[oldContentFull]
		if !ok {
			return nil, nil, fmt.Errorf("author: rename: content file %s not found", oldContentFull)
		}
		if newContentFull != oldContentFull {
			delete(after, oldContentFull)
			after[newContentFull] = content
		}
	}

	refusals, obligations, err := Report(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	written := changedPaths(before, after)
	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: rename: %w", err)
	}
	if loc.nodeType.RequiresContent && newContentFull != oldContentFull {
		if err := os.Remove(filepath.Join(specDir, filepath.FromSlash(oldContentFull))); err != nil {
			return nil, nil, fmt.Errorf("author: rename: remove %s: %w", oldContentFull, err)
		}
	}

	return &WriteReport{
		Written:     written,
		Obligations: obligations,
		RetiredName: loc.name,
	}, nil, nil
}

// moduleRenameRefusal is the refusal NodeRenamer raises itself, ahead of
// Report, when input.ID names a module: arch_node_renamer.md's "What it
// refuses" — "a module's name is its path and its module.json name at
// once, and renaming one is a project.json edit plus a directory move made
// by hand, which the fix names." No validator check exists for this (a
// module's identity is a frame concept, not a profile declaration), so
// this is NodeRenamer's own guard rather than something Report's checkers
// would ever find.
func moduleRenameRefusal(id string) RefusalEntry {
	return RefusalEntry{
		Check:   "rename",
		Message: fmt.Sprintf("%s is a module id; a module's name is its path and its module.json name at once, so renaming one is not a single-node edit", id),
		Path:    "project.json:/modules/" + id,
		Fix:     "spex node rename does not rename modules: edit project.json's modules entry (name and path) by hand and move the module's directory to match",
	}
}

// nodeLocation is where locateNode found input.ID: which file declares it,
// what profile-declared type and scope it is, and its current name. Zero
// value with isModule true means the id names a module instead of a
// profile-declared node.
type nodeLocation struct {
	isModule     bool
	projectScope bool
	moduleName   string // "" for project scope
	moduleDir    string // "" for project scope
	ownerFile    string // "project.json", or "<moduleDir>/module.json"
	nodeType     schema.NodeType
	name         string
}

// locateNode finds id in mem: first among project.json's declared modules
// (a module id is reported distinctly, since it is never a profile-declared
// node type), then among every project-scoped node type's array in
// project.json, then among every module-scoped node type's array in each
// module's module.json — the same generic, profile-driven array search
// id_validator.go's checks use, so a profile-declared type beyond the five
// built-ins is found exactly the same way.
func locateNode(mem validator.MemFS, profile *schema.Profile, id string) (nodeLocation, error) {
	projDoc, err := decodeDoc(mem, "project.json")
	if err != nil {
		return nodeLocation{}, err
	}

	if arr, ok := projDoc["modules"].([]any); ok {
		for _, item := range arr {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if s, _ := obj["id"].(string); s == id {
				return nodeLocation{isModule: true}, nil
			}
		}
	}

	for _, nt := range profile.NodeTypes {
		if nt.Scope != "project" {
			continue
		}
		if name, ok := findEntryField(projDoc, nt.PluralKey, id, "name"); ok {
			return nodeLocation{projectScope: true, ownerFile: "project.json", nodeType: nt, name: name}, nil
		}
	}

	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err != nil {
		return nodeLocation{}, fmt.Errorf("parse project.json: %w", err)
	}
	for _, mod := range proj.Modules {
		modFile := path.Join(mod.Path, "module.json")
		modDoc, err := decodeDoc(mem, modFile)
		if err != nil {
			continue
		}
		for _, nt := range profile.NodeTypes {
			if nt.Scope != "module" {
				continue
			}
			if name, ok := findEntryField(modDoc, nt.PluralKey, id, "name"); ok {
				return nodeLocation{moduleName: mod.Name, moduleDir: mod.Path, ownerFile: modFile, nodeType: nt, name: name}, nil
			}
		}
	}

	return nodeLocation{}, fmt.Errorf("no node with id %s", id)
}

// decodeDoc decodes mem's file at key as a generic JSON object, the shape
// every mutation and search helper in this file operates on so a
// profile-declared type beyond schema's typed structs is still reachable.
func decodeDoc(mem validator.MemFS, key string) (map[string]any, error) {
	data, ok := mem[key]
	if !ok {
		return nil, fmt.Errorf("missing %s", key)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", key, err)
	}
	return doc, nil
}

// contentPathCollision looks for another entry in doc's pluralKey array —
// not the node named by excludeID — whose "content" field equals
// newContent, returning that entry's id. Two different names can derive
// different ids (schema.IdentityHash is case-sensitive) yet still slug to
// the same snake-case content path, and Rename's move step would otherwise
// silently overwrite that other entry's leaf (PRRT_kwDORYErI86ipbGT). Its
// caller only calls this once it already knows the new path is occupied
// (checked directly against the before-state, which also catches an
// undeclared leaf sitting there — PRRT_kwDORYErI86ip63e, since an
// undeclared file has no array entry for this function to find); this
// function's only remaining job is telling that declared case apart from
// the id-collision case Report's own "duplicate ID" refusal already covers
// — the caller only refuses when the colliding id differs from the
// rename's own derived id.
func contentPathCollision(doc map[string]any, pluralKey, excludeID, newContent string) (string, bool) {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return "", false
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entryID, _ := obj["id"].(string)
		if entryID == excludeID {
			continue
		}
		if c, _ := obj["content"].(string); c == newContent {
			return entryID, true
		}
	}
	return "", false
}

// findEntryField searches doc's array at pluralKey for the entry whose "id"
// equals id, returning its field value. Used for "name" (locateNode) and
// "content" (the pre-rename content path, so the rename knows what to
// move).
func findEntryField(doc map[string]any, pluralKey, id, field string) (string, bool) {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return "", false
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := obj["id"].(string); s == id {
			v, _ := obj[field].(string)
			return v, true
		}
	}
	return "", false
}

// mutateEntry rewrites the one entry of doc's pluralKey array whose "id"
// equals oldID in place: the new id, the new name, and — when
// newContentValue is non-nil — the new conventional content path.
// Reports whether an entry was found and rewritten.
func mutateEntry(doc map[string]any, pluralKey, oldID, newID, newName string, newContentValue *string) bool {
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return false
	}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := obj["id"].(string); s == oldID {
			obj["id"] = newID
			obj["name"] = newName
			if newContentValue != nil {
				obj["content"] = *newContentValue
			}
			return true
		}
	}
	return false
}

// referenceFieldNames is the set of every reference field name the
// resolved profile declares anywhere — implements, uses, describes,
// provided_by, preq_id, depends_on, requires_module, and any reference
// field a profile declares beyond those — read off profile.Edges rather
// than a fixed list, so patchReferenceFields rewrites exactly the fields
// arch_node_renamer.md names: "every reference field entry ... in every
// project.json and module.json".
func referenceFieldNames(profile *schema.Profile) map[string]bool {
	set := make(map[string]bool, len(profile.Edges))
	for _, e := range profile.Edges {
		set[e.Kind] = true
	}
	return set
}

// patchReferenceFields walks every top-level array of doc and, for every
// object entry, rewrites any value of a field named in refFields that
// equals oldID to newID — a single-value reference (preq_id's
// cardinality "one") or any matching element of a many-valued reference
// (implements, uses, ...), covered by the same field-name check since the
// profile format uses the field's declared cardinality to decide which
// shape it writes, not a name convention this function needs to know.
// Reports whether anything changed, so the caller only re-marshals and
// rewrites a file its own reference fields actually touched.
func patchReferenceFields(doc map[string]any, refFields map[string]bool, oldID, newID string) bool {
	changed := false
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
			for field := range refFields {
				val, ok := obj[field]
				if !ok {
					continue
				}
				switch x := val.(type) {
				case string:
					if x == oldID {
						obj[field] = newID
						changed = true
					}
				case []any:
					for i, e := range x {
						if s, ok := e.(string); ok && s == oldID {
							x[i] = newID
							changed = true
						}
					}
				}
			}
		}
	}
	return changed
}

// linkTargetPattern matches a typed link's opening and target segment —
// `[[<oldID>` up to the `|` that starts the display text — so repointLinks
// can rewrite only the target, per validator/link_resolver.go's
// `[[<identity hash>|<display text>]]` syntax; the display text is left
// alone, exactly as arch_node_renamer.md requires ("leaving the display
// text alone — display text is prose, and the renamer does not judge
// whether it still fits").
func linkTargetPattern(oldID string) *regexp.Regexp {
	return regexp.MustCompile(`\[\[\s*` + regexp.QuoteMeta(oldID) + `\s*\|`)
}

// repointLinks rewrites every typed link in data naming oldID to name
// newID instead, reporting whether anything changed.
func repointLinks(data []byte, oldID, newID string) ([]byte, bool) {
	pattern := linkTargetPattern(oldID)
	if !pattern.Match(data) {
		return data, false
	}
	return pattern.ReplaceAll(data, []byte("[["+newID+"|")), true
}

// repointDotNodeRefs rewrites every bare identity-hash token naming oldID to
// newID instead, wherever it appears inside a ```dot fence — the second link
// form validator/markdown_scanner.go's kindDotNode resolves, alongside the
// `[[<id>|<display>]]` form repointLinks handles. Every flow_*.md leaf names
// its participants this way (spex render --format dot's own node-ID
// convention), so a rename of a data-flow participant needs this sweep too,
// or CheckLinksFS refuses the after-state with a stale "link target ... does
// not resolve" error. Reports whether anything changed.
func repointDotNodeRefs(data []byte, oldID, newID string) ([]byte, bool) {
	lines := strings.Split(string(data), "\n")
	changed := false

	inFence := false
	var fenceChar byte
	var fenceLen int
	var fenceInfo string
	for i, line := range lines {
		if ch, n, info, ok := fenceLineMarker(line); ok {
			if !inFence {
				inFence, fenceChar, fenceLen, fenceInfo = true, ch, n, info
			} else if ch == fenceChar && n >= fenceLen && info == "" {
				inFence, fenceChar, fenceLen, fenceInfo = false, 0, 0, ""
			}
			continue
		}
		if inFence && fenceInfo == "dot" {
			if newLine, ok := repointBareHashLine(line, oldID, newID); ok {
				lines[i] = newLine
				changed = true
			}
		}
	}

	if !changed {
		return data, false
	}
	return []byte(strings.Join(lines, "\n")), true
}

// fenceLineMarker mirrors validator/markdown_scanner.go's unexported
// fenceMarker: it reports whether line opens or closes a fenced code block,
// returning the fence character, its run length and the info string, so
// repointDotNodeRefs tracks ```dot fences the same way CheckLinksFS's
// scanner does. Up to three leading spaces are allowed, matching CommonMark.
func fenceLineMarker(line string) (ch byte, runLen int, info string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, 0, "", false
	}
	if len(trimmed) < 3 {
		return 0, 0, "", false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0, "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	rest := strings.TrimSpace(trimmed[n:])
	if c == '`' && strings.Contains(rest, "`") {
		return 0, 0, "", false
	}
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		rest = rest[:idx]
	}
	return c, n, rest, true
}

// repointBareHashLine replaces every standalone occurrence of oldID in line
// with newID — "standalone" meaning its neighbours are not word characters,
// the same rule validator/markdown_scanner.go's scanBareHashes applies to
// keep a 64-hex content hash from being read as five consecutive identity
// hashes. oldID and newID are always 12 lowercase-hex characters (schema.
// IdentityHash's output), so a literal search is exact — no regex needed.
func repointBareHashLine(line, oldID, newID string) (string, bool) {
	changed := false
	var b strings.Builder
	i := 0
	for {
		rel := strings.Index(line[i:], oldID)
		if rel < 0 {
			b.WriteString(line[i:])
			break
		}
		start := i + rel
		end := start + len(oldID)
		before := start == 0 || !isWordByte(line[start-1])
		after := end == len(line) || !isWordByte(line[end])
		if before && after {
			b.WriteString(line[i:start])
			b.WriteString(newID)
			changed = true
			i = end
		} else {
			b.WriteString(line[i : start+1])
			i = start + 1
		}
	}
	return b.String(), changed
}

// isWordByte mirrors validator/markdown_scanner.go's unexported isWordByte:
// the neighbour-character rule scanBareHashes and repointBareHashLine both
// use to tell a standalone identity hash from a substring of a longer token.
func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}

// snakeCase is the "snake-case slug of the name" NodeEditor's content-path
// convention names (arch_node_renamer.md, "rewrites the node's entry in
// place: ... for a content-bearing type the new conventional content
// path"): lowercase letters and digits kept, every run of anything else
// collapsed to one underscore, leading/trailing underscores trimmed.
func snakeCase(name string) string {
	var b strings.Builder
	lastUnderscore := true
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
		} else if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.TrimRight(b.String(), "_")
}

// specDocKeys lists every project.json/module.json path in the tree mem
// describes: "project.json" plus "<path>/module.json" for every module
// project.json declares. This is the fixed set of files
// patchReferenceFields must sweep, per arch_node_renamer.md's "every
// project.json and module.json in the tree."
func specDocKeys(mem validator.MemFS) ([]string, error) {
	var proj schema.Project
	if err := json.Unmarshal(mem["project.json"], &proj); err != nil {
		return nil, fmt.Errorf("parse project.json: %w", err)
	}
	keys := []string{"project.json"}
	for _, mod := range proj.Modules {
		keys = append(keys, path.Join(mod.Path, "module.json"))
	}
	return keys, nil
}

// declaredContentLeaves is the set of "every content leaf" arch_node_renamer.md
// step 4 scopes the link-repoint sweep to: every path a content-bearing node
// type's "content" field names, across every doc in docs, joined with that
// doc's owning directory ("" for project.json, the module's directory for a
// module.json). This is the same declared-only set
// validator/content_resolver.go's CheckContentPathsFS walks (content-bearing
// types read off the resolved profile, not a fixed three-type list, so a
// profile-declared type beyond components/data_flows/test_sections is swept
// too) — deliberately narrower than every .md file under specDir, since
// spec/proposals/ holds historical documents that name a retired id on
// purpose (validator/removed_name_checker.go's corpusDirSkip) and an
// undeclared stray .md is not a leaf CheckLinksFS or anything else in the
// pipeline reads (PRRT_kwDORYErI86ip63h). Called once, before any doc in
// docs is mutated, so a caller repointing links in after still finds these
// paths at their pre-rename location.
func declaredContentLeaves(docs map[string]map[string]any, docKeys []string, profile *schema.Profile) map[string]bool {
	leaves := make(map[string]bool)
	for _, key := range docKeys {
		scope := "module"
		dir := path.Dir(key)
		if key == "project.json" {
			scope = "project"
			dir = ""
		}
		doc := docs[key]
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
				if c, _ := obj["content"].(string); c != "" {
					leaves[path.Join(dir, c)] = true
				}
			}
		}
	}
	return leaves
}

// loadSpecMemFS reads every file under specDir into a validator.MemFS — the
// starting point Rename mutates its in-memory after-state from, so Report
// sees the same tree the real before-state (os.DirFS(specDir)) carries.
func loadSpecMemFS(specDir string) (validator.MemFS, error) {
	mem := validator.MemFS{}
	err := filepath.WalkDir(specDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(specDir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		mem[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", specDir, err)
	}
	return mem, nil
}

// cloneSpecFS makes a fresh top-level copy of m: same keys, independent byte
// slices, so mutating the copy (the after-state) never perturbs m (the
// before-state Report also reads, indirectly, via os.DirFS(specDir)).
func cloneSpecFS(m validator.MemFS) validator.MemFS {
	out := make(validator.MemFS, len(m))
	for k, v := range m {
		data := make([]byte, len(v))
		copy(data, v)
		out[k] = data
	}
	return out
}

// changedPaths returns, sorted, every key in after whose bytes differ from
// before (including a key added by after and absent from before — the new
// content path lands here). A key before carries that after no longer has
// (the old content path, deleted by Rename once it copies the bytes to the
// new path) is not reported: it was moved, not written, and Rename removes
// it from disk separately.
func changedPaths(before, after validator.MemFS) []string {
	var out []string
	for k, v := range after {
		if bv, ok := before[k]; !ok || !bytes.Equal(bv, v) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// writeChanges writes after's bytes for every path in written to disk under
// specDir — the one place in this file that touches disk, run only once
// Report has accepted the change.
func writeChanges(specDir string, after validator.MemFS, written []string) error {
	for _, p := range written {
		full := filepath.Join(specDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			return fmt.Errorf("mkdir for %s: %w", p, err)
		}
		if err := os.WriteFile(full, after[p], 0644); err != nil {
			return fmt.Errorf("write %s: %w", p, err)
		}
	}
	return nil
}
