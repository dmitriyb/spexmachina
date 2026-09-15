package author

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// MigrateReport is what `spex migrate` prints on stdout once Migrate has
// run: whether the tree was already at the current format (in which case
// nothing else is populated and nothing was written), or the files written
// plus what changed — every requirement renamed and every undeclared array
// entry removed, by file and location — and the obligations the completeness
// rules attach to the result, the same way every other writing command's
// report does (spec/author/arch_migrator.md; spec/author/flow_authoring.md,
// "Out of the reporter").
type MigrateReport struct {
	AlreadyCurrent bool               `json:"already_current"`
	Written        []string           `json:"written,omitempty"`
	Renamed        []RenamedEntry     `json:"renamed,omitempty"`
	Removed        []RemovedEntry     `json:"removed,omitempty"`
	Obligations    []merkle.DiffError `json:"obligations,omitempty"`
}

// RenamedEntry is one requirement Migrate rewrote from the retired `title`
// key to `name`: the file that carries it and a JSON pointer to the
// requirement entry. The value moves, byte for byte; the id does not — see
// Migrate's doc comment.
type RenamedEntry struct {
	File    string `json:"file"`
	Pointer string `json:"pointer"`
}

// RemovedEntry is one entry of an array the resolved profile does not
// declare, removed by Migrate: the file and array it was found in, its own
// `name` when it carried one, and — when it named a content file via its own
// `content` field — that file's path, reported as orphaned. The file itself
// is left on disk untouched; nothing in the pipeline reads an unreferenced
// path (spec/author/arch_migrator.md, "What it rewrites").
type RemovedEntry struct {
	File    string `json:"file"`
	Array   string `json:"array"`
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
}

// Migrate is Migrator, spec/author/arch_migrator.md: given a spec directory
// — needing no initialised project, and reading nothing under .spex/ — it
// rewrites the tree from an earlier format version to the current one, in
// one pass:
//
//   - the title-to-name rename, on every requirement entry at both project
//     and module scope: the entry's `title` key is renamed to `name`,
//     keeping its value verbatim. Identity is value-derived and this moves
//     no id, so unlike NodeRenamer's `spex node rename` (a full identity
//     transaction), this is a plain key rename — no id recomputation, no
//     reference rewriting, no content move;
//   - removal of every top-level array the resolved profile does not
//     declare at that document's scope, project.json's frame-fixed `modules`
//     and `sections` arrays excepted. Each removed entry is reported by
//     file, array and name, and — when the entry named a content file via
//     its own `content` field — that file's path is reported as orphaned,
//     left on disk;
//   - `spec_version` stamped in project.json at schema.SupportedSpecVersion,
//     when it is not already there.
//
// A file is rewritten only when one of the three edits above actually
// touches it: a tree already at the current format has nothing to rewrite
// in any file, so Migrate writes nothing and reports AlreadyCurrent — the
// idempotence and no-op guarantees arch_migrator.md's "Idempotence" section
// names ("A second run over a migrated tree is that case... running it is
// the check").
//
// Three outcomes:
//
//   - A write or a no-op: (report, nil, nil). report.AlreadyCurrent is true
//     for a no-op, with nothing else populated.
//   - A refusal: (nil, refusals, nil). In practice this never happens — see
//     migrateRefusalsAndObligations below — but the shape is kept for
//     consistency with every other worker in this package.
//   - An input error: (nil, nil, err). specDir cannot be read, or the
//     profile fails to resolve.
func Migrate(specDir string) (*MigrateReport, []RefusalEntry, error) {
	profile, err := schema.ResolveProfile(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: migrate: %w", err)
	}

	before, err := loadSpecMemFS(specDir)
	if err != nil {
		return nil, nil, fmt.Errorf("author: migrate: %w", err)
	}

	docKeys, err := specDocKeys(before)
	if err != nil {
		return nil, nil, fmt.Errorf("author: migrate: %w", err)
	}

	after := cloneSpecFS(before)
	var renamed []RenamedEntry
	var removed []RemovedEntry

	for _, key := range docKeys {
		doc, err := decodeDoc(before, key)
		if err != nil {
			return nil, nil, fmt.Errorf("author: migrate: %w", err)
		}

		scope := "module"
		dir := path.Dir(key)
		if key == "project.json" {
			scope = "project"
			dir = ""
		}

		docRenamed := migrateRequirementNames(doc, key, scope, profile, &renamed)
		docPruned := pruneUndeclaredArrays(doc, key, scope, dir, profile, &removed)
		docVersioned := false
		if key == "project.json" {
			docVersioned = stampSpecVersion(doc)
		}

		if !docRenamed && !docPruned && !docVersioned {
			continue
		}

		data, err := marshalIndentNoEscape(canonicalizeDoc(doc, key, profile))
		if err != nil {
			return nil, nil, fmt.Errorf("author: migrate: marshal %s: %w", key, err)
		}
		after[key] = append(data, '\n')
	}

	written := changedPaths(before, after)
	if len(written) == 0 {
		return &MigrateReport{AlreadyCurrent: true}, nil, nil
	}

	refusals, obligations, err := migrateRefusalsAndObligations(os.DirFS(specDir), after, profile)
	if err != nil {
		return nil, nil, fmt.Errorf("author: migrate: %w", err)
	}
	if len(refusals) > 0 {
		return nil, refusals, nil
	}

	if err := writeChanges(specDir, after, written); err != nil {
		return nil, nil, fmt.Errorf("author: migrate: %w", err)
	}

	return &MigrateReport{
		Written:     written,
		Renamed:     renamed,
		Removed:     removed,
		Obligations: obligations,
	}, nil, nil
}

// migrateRequirementNames renames the `title` key to `name` on every entry
// of doc's requirement array at scope, keeping the value and touching
// nothing else on the entry — not even `id`, which the identity contract
// derives from the value, never the key it arrived under
// (arch_migrator.md, "What it rewrites"). Reports whether anything changed
// and appends one RenamedEntry per entry actually renamed, in array order.
func migrateRequirementNames(doc map[string]any, key, scope string, profile *schema.Profile, renamed *[]RenamedEntry) bool {
	pluralKey := ""
	for _, nt := range profile.NodeTypes {
		if nt.Scope == scope && nt.Name == "requirement" {
			pluralKey = nt.PluralKey
			break
		}
	}
	if pluralKey == "" {
		return false
	}
	arr, ok := doc[pluralKey].([]any)
	if !ok {
		return false
	}

	changed := false
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title, hasTitle := obj["title"]
		if !hasTitle {
			continue
		}
		obj["name"] = title
		delete(obj, "title")
		changed = true
		*renamed = append(*renamed, RenamedEntry{
			File:    key,
			Pointer: fmt.Sprintf("/%s/%d", pluralKey, i),
		})
	}
	return changed
}

// declaredArrayKeys is the set of top-level array property keys that count
// as declared for doc's scope: every plural key a profile-declared node type
// at that scope owns, plus project.json's frame-fixed `modules` and
// `sections` — neither of which is a profile node type
// (project_compose.go's projectProfileArrays: "modules is structurally
// required ... and sections is the project's generic extension mechanism,
// neither is part of the declarable node-type vocabulary"). Anything else is
// what pruneUndeclaredArrays removes.
func declaredArrayKeys(scope string, profile *schema.Profile) map[string]bool {
	keys := make(map[string]bool)
	for _, nt := range profile.NodeTypes {
		if nt.Scope == scope {
			keys[nt.PluralKey] = true
		}
	}
	if scope == "project" {
		keys["modules"] = true
		keys["sections"] = true
	}
	return keys
}

// pruneUndeclaredArrays deletes every top-level array property of doc whose
// key declaredArrayKeys does not name, reporting each removed entry — its
// array, its own `name` when it has one, and the content file its own
// `content` field names (joined against dir, so a module-scoped orphan is
// reported by its path from specDir, matching what CheckContentPathsFS would
// have searched) — as a RemovedEntry. The content file itself is never
// touched; this function only ever mutates doc. Reports whether anything
// changed. Iterates the undeclared keys in sorted order, so a tree with more
// than one undeclared array reports and removes them deterministically.
func pruneUndeclaredArrays(doc map[string]any, key, scope, dir string, profile *schema.Profile, removed *[]RemovedEntry) bool {
	declared := declaredArrayKeys(scope, profile)

	var undeclared []string
	for k, v := range doc {
		if declared[k] {
			continue
		}
		if _, ok := v.([]any); !ok {
			continue
		}
		undeclared = append(undeclared, k)
	}
	sort.Strings(undeclared)

	changed := false
	for _, k := range undeclared {
		arr := doc[k].([]any)
		for _, item := range arr {
			entry := RemovedEntry{File: key, Array: k}
			if obj, ok := item.(map[string]any); ok {
				if name, _ := obj["name"].(string); name != "" {
					entry.Name = name
				}
				if content, _ := obj["content"].(string); content != "" {
					entry.Content = path.Join(dir, content)
				}
			}
			*removed = append(*removed, entry)
		}
		delete(doc, k)
		changed = true
	}
	return changed
}

// stampSpecVersion sets doc's `spec_version` field to
// schema.SupportedSpecVersion, reporting whether that changed anything — a
// document already carrying the current version, whether as an int or as
// the float64 a generic JSON decode produces, is left alone so a fully
// current file is never rewritten (arch_migrator.md, "Idempotence").
func stampSpecVersion(doc map[string]any) bool {
	current := float64(schema.SupportedSpecVersion)
	if v, ok := doc["spec_version"]; ok {
		if f, ok := v.(float64); ok && f == current {
			return false
		}
	}
	doc["spec_version"] = schema.SupportedSpecVersion
	return true
}

// migrateRefusalsAndObligations mirrors Report's refusal/obligation split
// (obligation_reporter.go) — the same refusalCheckers/nonRefusalCheckers/
// completenessObligations Report itself drives — but classifies "did the
// change introduce this" by (Check, Path) rather than by Report's exact
// message-text match. Migrate's two edits only ever narrow or clear a
// location's schema violations, never widen them: the title-to-name rename
// can turn a combined "missing required properties 'preq_id', 'name'"
// finding at one location into a narrower "missing required property
// 'preq_id'" at the same location without the input having changed in any
// way this command is responsible for, and Report's own message-equality
// test would misread that narrowing as new. Migrator is the one writer in
// this package where matching on location alone is still correct — no other
// worker's edit is guaranteed to only ever fix or leave alone a location's
// conformance — which is what arch_migrator.md's "Boundaries" promises: "The
// rename and the removals introduce none [refusals], so the input's own
// defects ... reach the report as obligations rather than as a refusal, and
// the migration lands."
func migrateRefusalsAndObligations(before, after fs.FS, profile *schema.Profile) ([]RefusalEntry, []merkle.DiffError, error) {
	beforeErrs := refusalCheckers(before)
	afterErrs := refusalCheckers(after)

	priorLocations := make(map[string]bool, len(beforeErrs))
	for _, e := range beforeErrs {
		priorLocations[e.Check+"\x00"+e.Path] = true
	}

	var refusals []RefusalEntry
	for _, e := range afterErrs {
		if priorLocations[e.Check+"\x00"+e.Path] {
			continue
		}
		refusals = append(refusals, RefusalEntry{
			Check:   e.Check,
			Message: e.Message,
			Path:    e.Path,
			Fix:     computeFix(e, before, profile),
		})
	}
	if len(refusals) > 0 {
		return refusals, nil, nil
	}

	completeness, err := completenessObligations(before, after, profile)
	if err != nil {
		return nil, nil, err
	}
	var obligations []merkle.DiffError
	obligations = append(obligations, completeness...)
	obligations = append(obligations, validatorObligations(afterErrs)...)
	obligations = append(obligations, validatorObligations(nonRefusalCheckers(after))...)
	return nil, obligations, nil
}
