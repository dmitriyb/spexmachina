package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckContentPaths walks all content fields in module.json files and verifies
// that referenced markdown files exist relative to their module directory.
// Content paths must not contain ".." or start with "/". Which module-scoped
// node types carry a content field is the resolved profile's declaration —
// under the default profile that is components, data_flows and
// test_sections; a profile-declared content-bearing type beyond those three
// (schema.ModuleSpec has no dedicated Go field for one) is walked the same
// way, read generically off raw JSON, mirroring the pattern CheckIDDerivation
// and CheckIDs already use for profile-declared types.
func CheckContentPaths(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "content")
	if len(errs) > 0 {
		return errs
	}

	profile, perr := schema.ResolveProfile(specDir)
	if perr != nil {
		return []ValidationError{{
			Check:    "content",
			Severity: "error",
			Path:     "profile.json",
			Message:  perr.Error(),
		}}
	}
	types := contentBearingModuleTypes(profile)

	var result []ValidationError

	// Iterate modules in deterministic order using project.json ordering.
	for _, mod := range project.Modules {
		modSpec, ok := modules[mod.Name]
		if !ok {
			continue
		}
		result = append(result, checkModuleContent(specDir, mod.Path, mod.Name, modSpec, types)...)
	}

	return result
}

// contentBearingModuleTypes returns the module-scoped node types the
// resolved profile marks content-bearing (RequiresContent), in declared
// order.
func contentBearingModuleTypes(profile *schema.Profile) []schema.NodeType {
	var out []schema.NodeType
	for _, nt := range moduleScopedNodeTypes(profile) {
		if nt.RequiresContent {
			out = append(out, nt)
		}
	}
	return out
}

// contentRef pairs a content path with the node that references it.
type contentRef struct {
	content   string
	nodeName  string
	nodeType  string // e.g. "component", "data_flow", "test_section", or a profile-declared type
	pluralKey string // the array key naming it in module.json, e.g. "components"
}

// contentEntry is one node's (name, content) pair, read either from a typed
// schema.ModuleSpec field or generically off raw JSON.
type contentEntry struct {
	name    string
	content string
}

// moduleContentEntries returns the (name, content) pairs schema.ModuleSpec
// carries typed for one of the three built-in content-bearing node types.
// Any other type name — a profile-declared type, or a built-in type the
// resolved profile does not mark content-bearing — has no dedicated Go field
// and is read generically instead.
func moduleContentEntries(mod *schema.ModuleSpec, typeName string) ([]contentEntry, bool) {
	switch typeName {
	case "component":
		out := make([]contentEntry, len(mod.Components))
		for i, c := range mod.Components {
			out[i] = contentEntry{name: c.Name, content: c.Content}
		}
		return out, true
	case "data_flow":
		out := make([]contentEntry, len(mod.DataFlows))
		for i, d := range mod.DataFlows {
			out[i] = contentEntry{name: d.Name, content: d.Content}
		}
		return out, true
	case "test_section":
		out := make([]contentEntry, len(mod.TestSections))
		for i, ts := range mod.TestSections {
			out[i] = contentEntry{name: ts.Name, content: ts.Content}
		}
		return out, true
	}
	return nil, false
}

// contentEntriesFromRaw converts generically-read entries to contentEntry
// pairs using each entry's "name" and "content" fields.
func contentEntriesFromRaw(raw []rawEntry) []contentEntry {
	out := make([]contentEntry, len(raw))
	for i, e := range raw {
		out[i] = contentEntry{name: e.str("name"), content: e.str("content")}
	}
	return out
}

// checkModuleContent collects all content references from a module and checks each.
func checkModuleContent(specDir, modPath, modName string, mod *schema.ModuleSpec, types []schema.NodeType) []ValidationError {
	var refs []contentRef

	for _, nt := range types {
		entries, ok := moduleContentEntries(mod, nt.Name)
		if !ok {
			raw, err := rawModuleEntries(specDir, modPath, nt.PluralKey)
			if err != nil {
				continue
			}
			entries = contentEntriesFromRaw(raw)
		}
		for _, e := range entries {
			if e.content == "" {
				continue
			}
			refs = append(refs, contentRef{content: e.content, nodeName: e.name, nodeType: nt.Name, pluralKey: nt.PluralKey})
		}
	}

	// Sort for deterministic output.
	slices.SortFunc(refs, func(a, b contentRef) int {
		if a.nodeType != b.nodeType {
			return strings.Compare(a.nodeType, b.nodeType)
		}
		return strings.Compare(a.nodeName, b.nodeName)
	})

	var errs []ValidationError
	for _, ref := range refs {
		errs = append(errs, checkContentPath(specDir, modPath, modName, ref)...)
	}
	return errs
}

// checkContentPath validates a single content path: rejects path traversal,
// then checks file existence.
func checkContentPath(specDir, modPath, modName string, ref contentRef) []ValidationError {
	location := fmt.Sprintf("%s/module.json:/%s/%s/content", modName, ref.pluralKey, ref.nodeName)

	for _, seg := range strings.Split(filepath.ToSlash(ref.content), "/") {
		if seg == ".." {
			return []ValidationError{{
				Check:    "content",
				Severity: "error",
				Path:     location,
				Message:  fmt.Sprintf("content path contains '..': %s", ref.content),
			}}
		}
	}
	if strings.HasPrefix(ref.content, "/") {
		return []ValidationError{{
			Check:    "content",
			Severity: "error",
			Path:     location,
			Message:  fmt.Sprintf("content path is absolute: %s", ref.content),
		}}
	}

	fullPath := filepath.Join(specDir, modPath, ref.content)
	if _, err := os.Stat(fullPath); err != nil {
		msg := fmt.Sprintf("content file not found: %s", ref.content)
		if !os.IsNotExist(err) {
			msg = fmt.Sprintf("content file inaccessible: %s: %s", ref.content, err)
		}
		return []ValidationError{{
			Check:    "content",
			Severity: "error",
			Path:     location,
			Message:  msg,
		}}
	}

	return nil
}
