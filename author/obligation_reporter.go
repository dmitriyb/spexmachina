package author

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
	"github.com/dmitriyb/spexmachina/validator"
)

// Report is ObligationReporter, the stage every writing command in this
// module passes through on its way to disk (spec/author/arch_obligation_reporter.md).
// beforeDir is the tree as it stands on disk; afterDir is the tree as the
// worker has already left it — its own scratch copy of beforeDir with the
// caller's change applied, never the real beforeDir and never written back
// by Report itself. Report answers two questions: may this be written, and
// what does writing it oblige.
//
// Refusal is the validator's own predicate: refusalCheckers runs over both
// beforeDir and afterDir, and a finding present in the after-run and absent
// from the before-run is refused, carrying the fix computeFix derives for
// it. Nothing here restates a validator rule — the Check and Message on a
// RefusalEntry are the checker's own. A finding either run already carried
// is not the change's fault and is dropped from the refusal set entirely: it
// is what `spex validate` still finds afterwards, not what this write
// introduced.
//
// When nothing is refused, Report runs the completeness rules
// (merkle.CheckCompleteness) over the (beforeDir, afterDir) pair — hashed
// the way `spex diff` hashes a snapshot against the current tree — and
// returns the entries it attaches to the change as obligations. Report
// itself never writes; the caller writes afterDir's content to beforeDir's
// location only once it has decided, from Report's answer, to accept the
// change.
func Report(beforeDir, afterDir string, profile *schema.Profile) (refusals []RefusalEntry, obligations []merkle.DiffError, err error) {
	beforeErrs := refusalCheckers(beforeDir)
	afterErrs := refusalCheckers(afterDir)

	introduced := make(map[string]bool, len(beforeErrs))
	for _, e := range beforeErrs {
		introduced[errorKey(e)] = true
	}

	for _, e := range afterErrs {
		if introduced[errorKey(e)] {
			continue
		}
		refusals = append(refusals, RefusalEntry{
			Check:   e.Check,
			Message: e.Message,
			Path:    e.Path,
			Fix:     computeFix(e, beforeDir),
		})
	}
	if len(refusals) > 0 {
		return refusals, nil, nil
	}

	obligations, err = completenessObligations(beforeDir, afterDir, profile)
	if err != nil {
		return nil, nil, err
	}
	return nil, obligations, nil
}

// refusalCheckers is the fixed set of validator checks a write can be
// refused against: SchemaChecker for conformance of the composed documents,
// IDValidator for id derivation, uniqueness, reference integrity and name
// declarability (validator.CheckIDs and validator.CheckIDDerivation both
// report through IDValidator's "id"/"id_derivation" checks), DAGChecker for
// acyclicity, and the link check for every typed link in every leaf
// (spec/author/arch_obligation_reporter.md, "Refusal is the validator's
// predicate"). No other validator checker — content path resolution, name
// consistency, test coverage, requirement coverage, coupled sections —
// gates a write; their findings, when they are new, travel as obligations
// instead, the same way `spex diff` never refuses on them either.
func refusalCheckers(specDir string) []validator.ValidationError {
	var errs []validator.ValidationError
	errs = append(errs, validator.CheckSchema(specDir)...)
	errs = append(errs, validator.CheckIDs(specDir)...)
	errs = append(errs, validator.CheckIDDerivation(specDir)...)
	errs = append(errs, validator.CheckDAG(specDir)...)
	errs = append(errs, validator.CheckLinks(specDir)...)
	return errs
}

// errorKey identifies a validator.ValidationError for before/after
// comparison: the same check, path and message the validator would print
// for a hand edit of the same shape, so two runs agree on "the same
// finding" exactly when `spex validate` would.
func errorKey(e validator.ValidationError) string {
	return e.Check + "\x00" + e.Path + "\x00" + e.Message
}

// completenessObligations builds the merkle trees on both sides of the
// change, diffs them exactly as `spex diff` diffs a snapshot against the
// current tree (merkle.Diff(after, before)), classifies the result against
// the resolved profile, and runs merkle.CheckCompleteness over it — the
// same computation `spex diff --json` reports under `errors` once the
// change reaches disk and a diff is taken against it
// (spec/author/arch_obligation_reporter.md, "Obligations are printed, not
// discovered").
func completenessObligations(beforeDir, afterDir string, profile *schema.Profile) ([]merkle.DiffError, error) {
	beforeTree, err := merkle.BuildTree(beforeDir)
	if err != nil {
		return nil, fmt.Errorf("author: build before-state tree: %w", err)
	}
	afterTree, err := merkle.BuildTree(afterDir)
	if err != nil {
		return nil, fmt.Errorf("author: build after-state tree: %w", err)
	}

	changes := merkle.Diff(afterTree, beforeTree)
	moduleNames := merkle.ModuleNames(afterTree)
	classified := merkle.Classify(changes, moduleNames, profile)
	return merkle.CheckCompleteness(classified, afterDir, profile), nil
}

// Patterns that pull the dynamic value a fix needs out of a validator
// message that already carries it. computeFix reads these off the message
// and the before-state tree rather than a table keyed by message text: the
// table below is "which piece of this specific message", never "what to say
// for this message" (spec/author/arch_obligation_reporter.md, "Every
// refusal names its fix": "computed from the profile and the tree, never
// from a table of messages").
var (
	duplicateIDRe      = regexp.MustCompile(`^duplicate ID ([0-9a-f]+)$`)
	duplicateAPIRe     = regexp.MustCompile(`^duplicate api name "[^"]*"; api names are globally unique, declared by: (.+)$`)
	declareAsRe        = regexp.MustCompile(`declare it as "([^"]*)"$`)
	idDerivationHashRe = regexp.MustCompile(`its identity hash is ([0-9a-f]+);`)
	dagCycleRe         = regexp.MustCompile(`cycle: (.+)$`)
	missingRequiredRe  = regexp.MustCompile(`missing required propert(?:y|ies) (.+)$`)
)

// computeFix derives the command, flag or value that resolves one refusal
// entry, per the table in spec/author/arch_obligation_reporter.md's "Every
// refusal names its fix". beforeDir is read to answer "where is the node
// this collides with", never to re-derive the validator's own verdict.
func computeFix(e validator.ValidationError, beforeDir string) string {
	switch e.Check {
	case "id":
		if m := duplicateIDRe.FindStringSubmatch(e.Message); m != nil {
			id := m[1]
			if file, ok := findDeclaringFile(beforeDir, id); ok {
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
			return fmt.Sprintf("set the required field(s) %s", m[1])
		}
		return e.Message
	default:
		return e.Message
	}
}

// findDeclaringFile searches specDir's project.json and every module.json
// it declares for an entry whose "id" field equals id, returning the
// spec-relative file that declares it. It reads the tree generically (every
// array in every JSON-backed file), so a profile-declared type beyond the
// five built-in ones is still found.
func findDeclaringFile(specDir, id string) (string, bool) {
	projPath := filepath.Join(specDir, "project.json")
	projData, err := os.ReadFile(projPath)
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
		modPath := filepath.Join(specDir, mod.Path, "module.json")
		modData, err := os.ReadFile(modPath)
		if err != nil {
			continue
		}
		var modRaw map[string]json.RawMessage
		if err := json.Unmarshal(modData, &modRaw); err != nil {
			continue
		}
		for _, raw := range modRaw {
			if hasID(raw, id) {
				return filepath.ToSlash(filepath.Join(mod.Path, "module.json")), true
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
