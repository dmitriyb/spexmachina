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
// Content paths must not contain ".." or start with "/".
func CheckContentPaths(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "content")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError

	// Iterate modules in deterministic order using project.json ordering.
	for _, mod := range project.Modules {
		modSpec, ok := modules[mod.Name]
		if !ok {
			continue
		}
		result = append(result, checkModuleContent(specDir, mod.Path, mod.Name, modSpec)...)
	}

	return result
}

// contentRef pairs a content path with the node that references it.
type contentRef struct {
	content  string
	nodeName string
	nodeKind string // "component", "impl_section", "data_flow"
}

// checkModuleContent collects all content references from a module and checks each.
func checkModuleContent(specDir, modPath, modName string, mod *schema.ModuleSpec) []ValidationError {
	var refs []contentRef

	for _, c := range mod.Components {
		if c.Content != "" {
			refs = append(refs, contentRef{content: c.Content, nodeName: c.Name, nodeKind: "component"})
		}
	}
	for _, s := range mod.ImplSections {
		if s.Content != "" {
			refs = append(refs, contentRef{content: s.Content, nodeName: s.Name, nodeKind: "impl_section"})
		}
	}
	for _, d := range mod.DataFlows {
		if d.Content != "" {
			refs = append(refs, contentRef{content: d.Content, nodeName: d.Name, nodeKind: "data_flow"})
		}
	}

	// Sort for deterministic output.
	slices.SortFunc(refs, func(a, b contentRef) int {
		if a.nodeKind != b.nodeKind {
			return strings.Compare(a.nodeKind, b.nodeKind)
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
	location := fmt.Sprintf("%s/module.json:/%ss/%s/content", modName, ref.nodeKind, ref.nodeName)

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
