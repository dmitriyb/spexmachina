package validator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// CheckLinks resolves typed cross-node links written in content leaves.
//
// The syntax is [[<identity hash>|<display text>]]: a 12-character hex
// identity hash, a pipe, and free-form display text that is never checked.
// Name-based links are rejected — a name carries no <type> segment, so it
// cannot be turned back into an identity hash, and one module may carry the
// same name on nodes of two different types.
//
// Targets resolve against the merkle leaf keys, collected exactly as
// ingest's refresh pathway collects them. That is what makes module nodes
// unlinkable: BuildTree gives modules Type "module", and only Type "leaf"
// nodes are link targets. It also means a component whose `content` is empty
// is not a leaf and so not linkable — the same rule ingest applies.
//
// This is the only checker that reads leaf bytes; CheckContentPaths stats the
// paths but never opens them.
func CheckLinks(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "link")
	if len(errs) > 0 {
		return errs
	}

	tree, err := merkle.BuildTree(specDir)
	if err != nil {
		// Every cause of a build failure here (an unreadable content file,
		// a malformed module.json) is already reported by another checker
		// against the file that caused it. Reporting it again under "link"
		// follows loadSpec's convention: each checker owns its own copy of
		// the load failure that stopped it.
		return []ValidationError{{
			Check:    "link",
			Severity: "error",
			Path:     "project.json",
			Message:  fmt.Sprintf("cannot resolve links: %s", err),
		}}
	}

	leafKeys := map[string]bool{}
	moduleKeys := map[string]bool{}
	collectLinkTargets(tree, leafKeys, moduleKeys)

	var result []ValidationError
	for _, mod := range project.Modules {
		modSpec, ok := modules[mod.Name]
		if !ok {
			continue
		}
		result = append(result, checkModuleLinks(specDir, mod, modSpec, leafKeys, moduleKeys)...)
	}
	return result
}

// collectLinkTargets flattens the tree into the set of linkable keys (leaves)
// and the set of module keys, which exist only to give a link that names a
// module a message that says why it cannot resolve.
func collectLinkTargets(n *merkle.Node, leaves, modules map[string]bool) {
	switch n.Type {
	case "leaf":
		leaves[n.Key] = true
		return
	case "module":
		modules[n.Key] = true
	}
	for _, child := range n.Children {
		collectLinkTargets(child, leaves, modules)
	}
}

// checkModuleLinks scans every content leaf a module declares. Content refs
// are gathered in module.json order per kind so that output is deterministic.
func checkModuleLinks(specDir string, mod schema.Module, modSpec *schema.ModuleSpec, leafKeys, moduleKeys map[string]bool) []ValidationError {
	var contents []string
	for _, c := range modSpec.Components {
		if c.Content != "" {
			contents = append(contents, c.Content)
		}
	}
	for _, d := range modSpec.DataFlows {
		if d.Content != "" {
			contents = append(contents, d.Content)
		}
	}
	for _, ts := range modSpec.TestSections {
		if ts.Content != "" {
			contents = append(contents, ts.Content)
		}
	}

	var errs []ValidationError
	seen := map[string]bool{}
	for _, content := range contents {
		if seen[content] {
			continue
		}
		seen[content] = true

		rel := filepath.ToSlash(filepath.Join(mod.Path, content))
		data, err := os.ReadFile(filepath.Join(specDir, mod.Path, content))
		if err != nil {
			errs = append(errs, ValidationError{
				Check:    "link",
				Severity: "error",
				Path:     rel,
				Message:  fmt.Sprintf("read content leaf: %s", err),
			})
			continue
		}
		for _, ref := range scanLinks(string(data)) {
			if e, bad := resolveLink(ref, rel, leafKeys, moduleKeys); bad {
				errs = append(errs, e)
			}
		}
	}
	return errs
}

// resolveLink applies the link rules in the order that gives the most
// actionable message: termination, then shape, then display text, then
// resolution. One link yields at most one error.
func resolveLink(ref linkRef, path string, leafKeys, moduleKeys map[string]bool) (ValidationError, bool) {
	fail := func(format string, args ...any) (ValidationError, bool) {
		return ValidationError{
			Check:    "link",
			Severity: "error",
			Path:     fmt.Sprintf("%s:%d", path, ref.Line),
			Message:  fmt.Sprintf(format, args...),
		}, true
	}

	if ref.Kind == kindUnterminated {
		return fail("unterminated link %s; a `[[` must be closed by `]]` before the next blank line, fenced block or end of file", ref.Raw)
	}

	if ref.Kind == kindWiki {
		if !isIdentityHash(ref.Target) {
			return fail("link %s target %q is not a 12-character identity hash; links are hash-based, not name-based", ref.Raw, ref.Target)
		}
		if !ref.HasDisplay || ref.Display == "" {
			return fail("link %s has no display text; write [[%s|<display text>]]", ref.Raw, ref.Target)
		}
	}

	if leafKeys[ref.Target] {
		return ValidationError{}, false
	}
	if moduleKeys[ref.Target] {
		return fail("link target %s is a module node; module nodes are not linkable", ref.Target)
	}
	return fail("link target %s does not resolve to any spec node", ref.Target)
}
