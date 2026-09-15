package author

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// Report is ObligationReporter, the stage every writing command in this
// module passes through on its way to disk (spec/author/arch_obligation_reporter.md).
// before is the tree as it stands on disk; after is the tree as the worker
// has already left it in memory — its own scratch copy of before with the
// caller's change applied, never the real before and never written back by
// Report itself. Both are fs.FS: before is typically os.DirFS(specDir), and
// after is typically a validator.MemFS a worker built without ever touching
// disk — this is the "loaded spec" the checkers this function drives take
// in place of a directory (flow_authoring.md, "Into the reporter": "the
// tree as read from disk and the tree as the worker left it in memory").
// Report answers two questions: may this be written, and what does writing
// it oblige.
//
// Refusal is the validator's own predicate: refusalCheckers runs over both
// before and after, and a finding present in the after-run and absent from
// the before-run is refused, carrying the fix computeFix derives for it.
// Nothing here restates a validator rule — the Check and Message on a
// RefusalEntry are the checker's own.
//
// When nothing is refused, Report runs the completeness rules
// (merkle.CheckCompleteness) over the (before, after) pair — hashed the way
// `spex diff` hashes a snapshot against the current tree — and returns the
// entries it attaches to the change as obligations, alongside every
// validator finding the after-state still carries that is not itself a
// refusal: an entry the before-state already carried (the change is not at
// fault for it) and any finding from a check that never gates a write in
// the first place (content, name_consistency, test_coverage,
// requirement_coverage, coupled_section) both travel here, unchanged from
// the validator's own entries (spec/author/arch_obligation_reporter.md,
// "Obligations are printed, not discovered": "the validator's own findings
// on an accepted change ... travel in the same array for the same
// reason"). Report itself never writes; the caller writes after's content
// to before's location only once it has decided, from Report's answer, to
// accept the change.
func Report(before, after fs.FS, profile *schema.Profile) (refusals []RefusalEntry, obligations []merkle.DiffError, err error) {
	beforeErrs := refusalCheckers(before)
	afterErrs := refusalCheckers(after)

	introduced := make(map[string]bool, len(beforeErrs))
	for _, e := range beforeErrs {
		for _, atom := range errorAtoms(e) {
			introduced[atom] = true
		}
	}

	for _, e := range afterErrs {
		if allIntroduced(errorAtoms(e), introduced) {
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
	obligations = append(obligations, completeness...)
	// Every refusalCheckers finding still standing on the after-state is,
	// by construction of the loop above, not new — the before-state
	// carried it too — so none of it was withheld as a refusal and all of
	// it belongs here instead.
	obligations = append(obligations, validatorObligations(afterErrs)...)
	obligations = append(obligations, validatorObligations(nonRefusalCheckers(after))...)

	return nil, obligations, nil
}

// refusalCheckers is the fixed set of validator checks a write can be
// refused against: SchemaChecker for conformance of the composed documents,
// IDValidator for id derivation, uniqueness, reference integrity and name
// declarability (validator.CheckIDsFS and validator.CheckIDDerivationFS
// both report through IDValidator's "id"/"id_derivation" checks),
// DAGChecker for acyclicity, and the link check for every typed link in
// every leaf (spec/author/arch_obligation_reporter.md, "Refusal is the
// validator's predicate"). No other validator checker gates a write; their
// findings travel as obligations instead, via nonRefusalCheckers.
func refusalCheckers(fsys fs.FS) []validator.ValidationError {
	var errs []validator.ValidationError
	errs = append(errs, validator.CheckSchemaFS(fsys)...)
	errs = append(errs, validator.CheckIDsFS(fsys)...)
	errs = append(errs, validator.CheckIDDerivationFS(fsys)...)
	errs = append(errs, validator.CheckDAGFS(fsys)...)
	errs = append(errs, validator.CheckLinksFS(fsys)...)
	return errs
}

// nonRefusalCheckers is every validator check `spex validate` runs beyond
// refusalCheckers' five: content path resolution, name consistency, test
// coverage, requirement coverage and coupled sections. None of these ever
// gates a write — the same way `spex diff` never refuses on them either —
// so every finding they produce on the after-state, new or pre-existing,
// is an obligation. CheckRequirementCoverage's notes are disclosures, not
// findings, and are dropped here.
func nonRefusalCheckers(fsys fs.FS) []validator.ValidationError {
	var errs []validator.ValidationError
	errs = append(errs, validator.CheckContentPathsFS(fsys)...)
	errs = append(errs, validator.CheckNameConsistencyFS(fsys)...)
	errs = append(errs, validator.CheckTestCoverageFS(fsys)...)
	reqErrs, _ := validator.CheckRequirementCoverageFS(fsys)
	errs = append(errs, reqErrs...)
	errs = append(errs, validator.CheckCoupledSectionsFS(fsys)...)
	return errs
}

// validatorObligations converts validator findings to merkle.DiffError, the
// obligations array's entry type: Type carries the validator's own Check
// name (e.g. "link", "requirement_coverage") so a validator-sourced
// obligation is distinguishable from a merkle.CheckCompleteness entry
// ("incomplete_change") without losing which checker raised it.
func validatorObligations(errs []validator.ValidationError) []merkle.DiffError {
	out := make([]merkle.DiffError, len(errs))
	for i, e := range errs {
		out[i] = merkle.DiffError{Type: e.Check, Message: e.Message, Path: e.Path}
	}
	return out
}

// errorKey identifies a validator.ValidationError for before/after
// comparison: the same check, path and message the validator would print
// for a hand edit of the same shape, so two runs agree on "the same
// finding" exactly when `spex validate` would.
func errorKey(e validator.ValidationError) string {
	return e.Check + "\x00" + e.Path + "\x00" + e.Message
}

// errorAtoms breaks a validator.ValidationError into the finer-grained
// claims errorKey's whole-message comparison cannot see through: jsonschema
// bundles every missing property at one location into a single "missing
// required propert(y|ies) '...'" message, so a change that supplies one of
// several missing fields narrows the message's field list without the
// change having introduced anything — and a before/after comparison keyed
// on the message text alone reads that narrowing as a brand new finding.
// For that one message shape, the atoms are one per field named
// (check+path+field), so "before had 'preq_id', 'name' missing, after has
// only 'preq_id' missing" compares as a subset, not a new finding — while a
// field that was NOT already missing before still shows up as a fresh atom,
// so a genuinely new violation at the same location is still caught. Every
// other message shape's sole atom is errorKey(e) itself, preserving
// message-exact comparison.
func errorAtoms(e validator.ValidationError) []string {
	if e.Check == "schema" {
		if m := missingRequiredRe.FindStringSubmatch(e.Message); m != nil {
			fields := requiredFieldNameRe.FindAllStringSubmatch(m[1], -1)
			if len(fields) > 0 {
				atoms := make([]string, 0, len(fields))
				for _, f := range fields {
					atoms = append(atoms, e.Check+"\x00"+e.Path+"\x00required:"+f[1])
				}
				return atoms
			}
		}
	}
	return []string{errorKey(e)}
}

// allIntroduced reports whether every atom in atoms is already present in
// introduced — i.e. e contributes nothing the before-state didn't already
// carry, so it is not a refusal.
func allIntroduced(atoms []string, introduced map[string]bool) bool {
	for _, a := range atoms {
		if !introduced[a] {
			return false
		}
	}
	return true
}

// completenessObligations builds the merkle trees on both sides of the
// change, diffs them exactly as `spex diff` diffs a snapshot against the
// current tree (merkle.Diff(after, before)), classifies the result against
// the resolved profile, and runs merkle.CheckCompleteness over it — the
// same computation `spex diff --json` reports under `errors` once the
// change reaches disk and a diff is taken against it
// (spec/author/arch_obligation_reporter.md, "Obligations are printed, not
// discovered").
func completenessObligations(before, after fs.FS, profile *schema.Profile) ([]merkle.DiffError, error) {
	beforeTree, err := merkle.BuildTreeFS(before)
	if err != nil {
		return nil, fmt.Errorf("author: build before-state tree: %w", err)
	}
	afterTree, err := merkle.BuildTreeFS(after)
	if err != nil {
		return nil, fmt.Errorf("author: build after-state tree: %w", err)
	}

	changes := merkle.Diff(afterTree, beforeTree)
	moduleNames := merkle.ModuleNames(afterTree)
	classified := merkle.Classify(changes, moduleNames, profile)
	return merkle.CheckCompletenessFS(classified, after, profile), nil
}

// Patterns that pull the dynamic value a fix needs out of a validator
// message that already carries it. computeFix reads these off the message,
// the before-state tree and the profile rather than a table keyed by
// message text: the table in spec/author/arch_obligation_reporter.md's
// "Every refusal names its fix" is "which piece of this specific message",
// never "what to say for this message".
var (
	duplicateIDRe       = regexp.MustCompile(`^duplicate ID ([0-9a-f]+)$`)
	duplicateAPIRe      = regexp.MustCompile(`^duplicate api name "[^"]*"; api names are globally unique, declared by: (.+)$`)
	declareAsRe         = regexp.MustCompile(`declare it as "([^"]*)"$`)
	idDerivationHashRe  = regexp.MustCompile(`its identity hash is ([0-9a-f]+);`)
	dagCycleRe          = regexp.MustCompile(`cycle: (.+)$`)
	missingRequiredRe   = regexp.MustCompile(`missing required propert(?:y|ies) (.+)$`)
	requiredFieldNameRe = regexp.MustCompile(`'([^']*)'`)
	additionalPropsRe   = regexp.MustCompile(`^additional propert(?:y|ies) .+ not allowed$`)
	referencesMissingRe = regexp.MustCompile(`^(\w+) references non-existent (.+) ([0-9a-f]{12})\b`)
)

// computeFix derives the command, flag or value that resolves one refusal
// entry, per the table in spec/author/arch_obligation_reporter.md's "Every
// refusal names its fix". before is read to answer "where is the node this
// collides with" or "did this reference's target exist before the
// change", never to re-derive the validator's own verdict; profile is read
// for the declared types, fields and edge targets a fix names.
func computeFix(e validator.ValidationError, before fs.FS, profile *schema.Profile) string {
	switch e.Check {
	case "id":
		if m := duplicateIDRe.FindStringSubmatch(e.Message); m != nil {
			id := m[1]
			if file, ok := findDeclaringFile(before, id); ok {
				return fmt.Sprintf("id %s is already declared at %s; choose a name that derives a different id", id, file)
			}
			return fmt.Sprintf("id %s is already declared elsewhere in the tree; choose a name that derives a different id", id)
		}
		if m := duplicateAPIRe.FindStringSubmatch(e.Message); m != nil {
			return fmt.Sprintf("already declared by %s; choose a different api name", m[1])
		}
		if m := declareAsRe.FindStringSubmatch(e.Message); m != nil {
			return fmt.Sprintf("declare it as %q", m[1])
		}
		if fix := referenceTargetFix(e, before, profile); fix != "" {
			return fix
		}
		return e.Message
	case "id_derivation":
		if m := idDerivationHashRe.FindStringSubmatch(e.Message); m != nil {
			return fmt.Sprintf("set id to %s", m[1])
		}
		return e.Message
	case "dag":
		if m := dagCycleRe.FindStringSubmatch(e.Message); m != nil {
			return fmt.Sprintf("remove one edge from the cycle %s", m[1])
		}
		return e.Message
	case "link":
		return fmt.Sprintf("point the link at an existing node's identity hash, or add the node first: %s", e.Message)
	case "schema":
		if m := missingRequiredRe.FindStringSubmatch(e.Message); m != nil {
			return missingRequiredFix(m[1], e.Path, profile)
		}
		if additionalPropsRe.MatchString(e.Message) {
			if nt, ok := nodeTypeForPath(e.Path, profile); ok {
				return disallowedFieldFix(nt)
			}
			return undeclaredTypeFix(e.Path, profile)
		}
		return e.Message
	default:
		return e.Message
	}
}

// referenceTargetFix handles the "id" check's "<field> references
// non-existent <type> <id>" message shape, which covers two of the fix
// table's rows depending on whether the target ever existed:
//
//   - it existed in the before-state and no longer does (the change itself
//     retired it, or a prior command did): "an inbound reference blocking a
//     removal" — the fix names the `spex edge remove` invocation that
//     retargets the reference, and `--force`.
//   - it never existed in either state: "a reference target that does not
//     exist" (and, for the same message shape, "a target type it may not
//     point at") — the fix names the array that was searched, by file and
//     key, and the field's permitted target types.
//
// Returns "" when the message does not match this shape at all.
func referenceTargetFix(e validator.ValidationError, before fs.FS, profile *schema.Profile) string {
	m := referencesMissingRe.FindStringSubmatch(e.Message)
	if m == nil {
		return ""
	}
	edgeKind, typeLabel, targetID := m[1], m[2], m[3]

	sourceFile := e.Path
	if idx := strings.Index(sourceFile, ":"); idx >= 0 {
		sourceFile = sourceFile[:idx]
	}

	if declFile, ok := findDeclaringFile(before, targetID); ok {
		srcID := path.Base(e.Path)
		return fmt.Sprintf(
			"%s %s existed at %s before this change; run `spex edge remove --source %s --field %s --target %s` to retarget it, or `spex node remove --force` to remove it despite the reference",
			typeLabel, targetID, declFile, srcID, edgeKind, targetID)
	}

	var permits []string
	var locations []string
	for _, edge := range profile.Edges {
		if edge.Kind != edgeKind {
			continue
		}
		permits = edge.To
		for _, target := range edge.To {
			// "module" is the frame's fixed interior-node concept, never a
			// profile-declared NodeType (schema.Profile.Validate rejects a
			// type named "module"), so the loop over profile.NodeTypes below
			// never finds it. requires_module is the one edge kind that
			// targets it, and the array it searches is fixed too:
			// project.json's own "modules" key.
			if target == "module" {
				locations = append(locations, "project.json:/modules")
				continue
			}
			for _, nt := range profile.NodeTypes {
				if nt.Name != target {
					continue
				}
				file := sourceFile
				if nt.Scope == "project" {
					file = "project.json"
				}
				locations = append(locations, fmt.Sprintf("%s:/%s", file, nt.PluralKey))
			}
		}
	}
	fix := fmt.Sprintf("no %s %s found; %s may target %s",
		typeLabel, targetID, edgeKind, strings.Join(permits, "/"))
	if len(locations) > 0 {
		fix += ", searched " + strings.Join(locations, ", ")
	}
	return fix
}

// nodeTypeForPath resolves a schema-checker path (e.g.
// "alpha/module.json:/components/2") to the profile-declared NodeType the
// violation occurred on: the file names the scope (project.json is
// project-scoped, any other file is module-scoped) and the instance
// location's first segment names the plural key. A root-level path (no
// ":", the whole document failed) has no single node type and returns
// false.
func nodeTypeForPath(errPath string, profile *schema.Profile) (schema.NodeType, bool) {
	idx := strings.Index(errPath, ":")
	if idx < 0 {
		return schema.NodeType{}, false
	}
	file := errPath[:idx]
	instance := strings.Trim(errPath[idx+1:], "/")
	segs := strings.SplitN(instance, "/", 2)
	if len(segs) == 0 || segs[0] == "" {
		return schema.NodeType{}, false
	}
	pluralKey := segs[0]
	scope := "module"
	if file == "project.json" {
		scope = "project"
	}
	for _, nt := range profile.NodeTypes {
		if nt.PluralKey == pluralKey && nt.Scope == scope {
			return nt, true
		}
	}
	return schema.NodeType{}, false
}

// missingRequiredFix is computeFix's row for a schema "missing required
// property" violation: arch_obligation_reporter.md's fix table says the
// carried fix is "the field's name and kind", so each field the message
// lists is paired with fieldKindLabel's description of it — an
// enum-constrained field like requirement's "type" names the legal values,
// not just that a value is owed.
func missingRequiredFix(fieldList, errPath string, profile *schema.Profile) string {
	nt, _ := nodeTypeForPath(errPath, profile)
	matches := requiredFieldNameRe.FindAllStringSubmatch(fieldList, -1)
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if kind := fieldKindLabel(name, nt); kind != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", name, kind))
		} else {
			parts = append(parts, name)
		}
	}
	return fmt.Sprintf("set the required field(s): %s", strings.Join(parts, ", "))
}

// fieldKindLabel names the kind of a required field by its declared name:
// id, name and content are the fixed envelope, whose kind never depends on
// the profile; every other name is looked up on nt (the type
// nodeTypeForPath resolved the violation's path to) so its declared kind —
// text, with the enum it may carry; integer, with its bounds; or reference,
// with its permitted targets — comes from the profile, never a table keyed
// by field name. Returns "" when nt is the zero value (a root-level
// violation names no single type) or the field is not declared on it.
func fieldKindLabel(name string, nt schema.NodeType) string {
	switch name {
	case "id":
		return "text, a 12-character identity hash"
	case "name":
		return "text"
	case "content":
		return "text, a relative path to the content leaf"
	}
	for _, f := range nt.Fields {
		if f.Name != name {
			continue
		}
		switch f.Kind {
		case schema.FieldKindText:
			if len(f.Enum) > 0 {
				return fmt.Sprintf("text, one of: %s", strings.Join(f.Enum, ", "))
			}
			return "text"
		case schema.FieldKindInteger:
			if f.Minimum != nil && f.Maximum != nil {
				return fmt.Sprintf("integer, %d-%d", *f.Minimum, *f.Maximum)
			}
			return "integer"
		case schema.FieldKindReference:
			return fmt.Sprintf("reference, targets: %s", strings.Join(f.Targets, ", "))
		default:
			return string(f.Kind)
		}
	}
	return ""
}

// undeclaredTypeFix is computeFix's row for a schema "additional
// properties" violation at the document root: an entry under a plural key
// the profile does not declare. The fix lists every type the profile does
// declare at that scope, plus "module" — the frame's own fixed type, never
// a profile declaration but always a legal top-level concept in
// project.json.
func undeclaredTypeFix(errPath string, profile *schema.Profile) string {
	scope := "module"
	if errPath == "project.json" {
		scope = "project"
	}
	var names []string
	for _, nt := range profile.NodeTypes {
		if nt.Scope == scope {
			names = append(names, nt.Name)
		}
	}
	names = append(names, "module")
	return fmt.Sprintf("declared types: %s", strings.Join(names, ", "))
}

// disallowedFieldFix is computeFix's row for a schema "additional
// properties" violation nested inside one entry: a field nt does not
// declare. The fix lists the fields the profile does declare on that type.
func disallowedFieldFix(nt schema.NodeType) string {
	names := []string{"id", "name", "description"}
	if nt.RequiresContent {
		names = append(names, "content")
	}
	for _, f := range nt.Fields {
		names = append(names, f.Name)
	}
	return fmt.Sprintf("%s declares fields: %s", nt.Name, strings.Join(names, ", "))
}

// findDeclaringFile searches before's project.json and every module.json it
// declares for an entry whose "id" field equals id, returning the
// spec-relative file that declares it. It reads the tree generically (every
// array in every JSON-backed file), so a profile-declared type beyond the
// five built-in ones is still found.
func findDeclaringFile(fsys fs.FS, id string) (string, bool) {
	projData, err := fs.ReadFile(fsys, "project.json")
	if err != nil {
		return "", false
	}
	var proj map[string]json.RawMessage
	if err := json.Unmarshal(projData, &proj); err != nil {
		return "", false
	}
	for _, raw := range proj {
		if hasID(raw, id) {
			return "project.json", true
		}
	}

	var modules []schema.Module
	if raw, ok := proj["modules"]; ok {
		_ = json.Unmarshal(raw, &modules)
	}
	for _, mod := range modules {
		modPath := path.Join(mod.Path, "module.json")
		modData, err := fs.ReadFile(fsys, modPath)
		if err != nil {
			continue
		}
		var modRaw map[string]json.RawMessage
		if err := json.Unmarshal(modData, &modRaw); err != nil {
			continue
		}
		for _, raw := range modRaw {
			if hasID(raw, id) {
				return modPath, true
			}
		}
	}
	return "", false
}

// hasID reports whether raw decodes as a JSON array carrying an entry whose
// "id" field equals id. raw that is not an array of objects (a module's
// "name" field, for instance) decodes to nothing and simply reports false.
func hasID(raw json.RawMessage, id string) bool {
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		return false
	}
	for _, e := range entries {
		if s, _ := e["id"].(string); s == id {
			return true
		}
	}
	return false
}
