package impact

import (
	"sort"
	"strings"
	"unicode"

	"github.com/dmitriyb/spexmachina/merkle"
)

// Match pairs a classified change with the beads that reference its spec node.
type Match struct {
	Change merkle.ClassifiedChange
	Beads  []BeadSpec
}

// Unmatched represents a changed spec node with no corresponding bead.
type Unmatched struct {
	Change merkle.ClassifiedChange
}

// Orphaned represents a bead whose referenced spec node was removed.
type Orphaned struct {
	Bead BeadSpec
}

// NodeMap maps node identifiers to their canonical spec node names.
// With spec-ID keys, the identifier is the node ID (e.g., "1" → "BeadReader").
// With legacy paths, the identifier is the filename (e.g., "arch_bead_reader.md" → "BeadReader").
type NodeMap map[string]string

// MatchNodes correlates classified changes with beads using spec metadata.
// The modules parameter maps module identifiers to their NodeMaps for ID-to-name
// resolution. Returns results sorted deterministically (NFR5).
func MatchNodes(changes []merkle.ClassifiedChange, beads []BeadSpec, modules map[string]NodeMap) ([]Match, []Unmatched, []Orphaned) {
	if modules == nil {
		modules = map[string]NodeMap{}
	}

	compIdx, implIdx, modIdx := buildBeadIndex(beads)

	matched := map[string]bool{}
	orphanCandidates := map[string]BeadSpec{}

	var matches []Match
	var unmatched []Unmatched

	for _, c := range changes {
		found := lookupBeads(c, compIdx, implIdx, modIdx, modules, beads)

		if len(found) > 0 {
			// Sort beads by ID for deterministic output.
			sort.Slice(found, func(i, j int) bool {
				return found[i].ID < found[j].ID
			})

			if c.Type == merkle.Removed {
				for _, b := range found {
					if !matched[b.ID] {
						orphanCandidates[b.ID] = b
					}
				}
			} else {
				matches = append(matches, Match{Change: c, Beads: found})
				for _, b := range found {
					matched[b.ID] = true
					delete(orphanCandidates, b.ID)
				}
			}
		} else if c.Type != merkle.Removed {
			unmatched = append(unmatched, Unmatched{Change: c})
		}
	}

	// Collect orphaned beads sorted by ID.
	var orphaned []Orphaned
	for _, b := range orphanCandidates {
		orphaned = append(orphaned, Orphaned{Bead: b})
	}
	sort.Slice(orphaned, func(i, j int) bool {
		return orphaned[i].Bead.ID < orphaned[j].Bead.ID
	})

	return matches, unmatched, orphaned
}

// indexKey builds a composite lookup key for bead indices.
func indexKey(module, name string) string {
	return module + "\x00" + name
}

// buildBeadIndex creates lookup indices for beads by their spec coordinates.
func buildBeadIndex(beads []BeadSpec) (compIdx, implIdx, modIdx map[string][]BeadSpec) {
	compIdx = map[string][]BeadSpec{}
	implIdx = map[string][]BeadSpec{}
	modIdx = map[string][]BeadSpec{}

	for _, b := range beads {
		modIdx[b.Module] = append(modIdx[b.Module], b)
		if b.Component != "" {
			k := indexKey(b.Module, b.Component)
			compIdx[k] = append(compIdx[k], b)
		}
		if b.ImplSection != "" {
			k := indexKey(b.Module, b.ImplSection)
			implIdx[k] = append(implIdx[k], b)
		}
	}

	return compIdx, implIdx, modIdx
}

// lookupBeads finds beads matching a classified change.
// Supports spec-ID keys (module/<id>/<type>/<id>) and legacy paths.
func lookupBeads(
	c merkle.ClassifiedChange,
	compIdx, implIdx, modIdx map[string][]BeadSpec,
	modules map[string]NodeMap,
	allBeads []BeadSpec,
) []BeadSpec {
	// Structural changes affect all beads in the module (or all beads for project meta).
	if c.Impact == merkle.Structural {
		if c.Module == "" {
			return copyBeads(allBeads)
		}
		return copyBeads(modIdx[c.Module])
	}

	// Parse spec-ID key: module/<module_id>/<node_type>/<node_id>
	parts := strings.Split(c.Path, "/")
	if len(parts) >= 4 && parts[0] == "module" {
		specNodeType := parts[2] // "component", "impl_section", "data_flow"
		nodeID := parts[3]
		nmKey := specNodeType + "/" + nodeID

		// Try NodeMap resolution (type/ID → name).
		if nm, ok := modules[c.Module]; ok {
			if name, ok := nm[nmKey]; ok {
				switch specNodeType {
				case "component":
					return copyBeads(compIdx[indexKey(c.Module, name)])
				case "impl_section":
					return copyBeads(implIdx[indexKey(c.Module, name)])
				case "data_flow":
					// TODO(spexmachina-3ta): data_flow bead matching not yet implemented.
					return nil
				}
			}
		}

		return nil
	}

	// Legacy fallback: filename-based matching.
	filename := lastPathSegment(c.Path)

	// Try NodeMap resolution first.
	if nm, ok := modules[c.Module]; ok {
		if name, ok := nm[filename]; ok {
			if strings.HasPrefix(filename, "arch_") {
				return copyBeads(compIdx[indexKey(c.Module, name)])
			}
			if strings.HasPrefix(filename, "impl_") {
				return copyBeads(implIdx[indexKey(c.Module, name)])
			}
			return nil
		}
	}

	// Fallback: auto-derive component name for arch_ files.
	if strings.HasPrefix(filename, "arch_") {
		slug := strings.TrimSuffix(strings.TrimPrefix(filename, "arch_"), ".md")
		name := snakeToPascal(slug)
		return copyBeads(compIdx[indexKey(c.Module, name)])
	}

	return nil
}

// lastPathSegment returns the last component of a path.
func lastPathSegment(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// copyBeads returns a shallow copy of the bead slice to avoid aliasing index internals.
func copyBeads(beads []BeadSpec) []BeadSpec {
	if len(beads) == 0 {
		return nil
	}
	out := make([]BeadSpec, len(beads))
	copy(out, beads)
	return out
}

// snakeToPascal converts a snake_case string to PascalCase.
// Example: "bead_reader" → "BeadReader".
func snakeToPascal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	upper := true
	for _, r := range s {
		if r == '_' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
