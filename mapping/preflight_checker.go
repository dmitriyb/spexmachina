package mapping

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PreflightResult reports the readiness status for implementing a bead.
type PreflightResult struct {
	Status    string    `json:"status"`                // "ready", "blocked", "stale"
	Record    Record    `json:"record"`                // the resolved mapping record
	Blockers  []Blocker `json:"blockers,omitempty"`    // present when Status == "blocked"
	StaleHash string    `json:"stale_hash,omitempty"`  // current hash when Status == "stale"
}

// Blocker describes one dependency that prevents a bead from being ready.
type Blocker struct {
	SpecNodeID string `json:"spec_node_id"`
	BeadID     string `json:"bead_id"`
	Reason     string `json:"reason"`
}

// SpecGraph provides read access to the spec dependency structure.
// Defined where consumed (PreflightChecker), not where implemented.
type SpecGraph interface {
	ModuleByName(name string) (ModuleInfo, error)
	ModuleByID(id int) (ModuleInfo, error)
	NodeHash(specNodeID string) (string, error)
}

// ModuleInfo describes a module's identity and dependencies.
type ModuleInfo struct {
	ID             int
	Name           string
	RequiresModule []int
	Components     []ComponentInfo
}

// ComponentInfo describes a component within a module.
type ComponentInfo struct {
	ID   int
	Name string
	Uses []int
}

// Check performs a deterministic preflight check for a bead. It resolves
// the bead to its mapping record and spec node, checks dependency readiness,
// and reports status: ready, blocked, or stale.
func Check(ctx context.Context, store Store, spec SpecGraph, beadID string) (PreflightResult, error) {
	record, err := store.GetByBead(beadID)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("preflight: %w", err)
	}

	// 1. Staleness check: compare stored hash with current spec content hash.
	currentHash, err := spec.NodeHash(record.SpecNodeID)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("preflight: hash %s: %w", record.SpecNodeID, err)
	}
	if currentHash != record.SpecHash {
		return PreflightResult{
			Status:    "stale",
			Record:    record,
			StaleHash: currentHash,
		}, nil
	}

	// 2. Module-level dependency check (transitive, with cycle detection).
	mod, err := spec.ModuleByName(record.Module)
	if err != nil {
		return PreflightResult{}, fmt.Errorf("preflight: module %s: %w", record.Module, err)
	}

	var blockers []Blocker
	stack := map[int]bool{mod.ID: true}
	if err := checkModuleDeps(store, spec, mod, stack, &blockers); err != nil {
		return PreflightResult{}, fmt.Errorf("preflight: %w", err)
	}

	// 3. Component-level uses check (within the same module).
	compID, ok := parseComponentID(record.SpecNodeID)
	if ok {
		if err := checkComponentUses(store, spec, mod, compID, &blockers); err != nil {
			return PreflightResult{}, fmt.Errorf("preflight: %w", err)
		}
	}

	if len(blockers) > 0 {
		sort.Slice(blockers, func(i, j int) bool {
			return blockers[i].SpecNodeID < blockers[j].SpecNodeID
		})
		return PreflightResult{
			Status:   "blocked",
			Record:   record,
			Blockers: blockers,
		}, nil
	}

	return PreflightResult{
		Status: "ready",
		Record: record,
	}, nil
}

// checkModuleDeps walks requires_module edges transitively. For each required
// module, all components must have a mapping record with bead_status "closed".
// The stack tracks the current recursion path for cycle detection; nodes seen
// on a different branch (diamond dependencies) are allowed.
func checkModuleDeps(store Store, spec SpecGraph, mod ModuleInfo, stack map[int]bool, blockers *[]Blocker) error {
	for _, depID := range mod.RequiresModule {
		if stack[depID] {
			return fmt.Errorf("cycle detected: module %d already in dependency chain", depID)
		}
		stack[depID] = true

		depMod, err := spec.ModuleByID(depID)
		if err != nil {
			return fmt.Errorf("module %d: %w", depID, err)
		}

		// Recurse into transitive deps first.
		if err := checkModuleDeps(store, spec, depMod, stack, blockers); err != nil {
			return err
		}
		delete(stack, depID) // backtrack

		// Check all components in the dep module have closed beads.
		for _, comp := range depMod.Components {
			specNodeID := fmt.Sprintf("%s/component/%d", depMod.Name, comp.ID)
			checkAllBeadsClosed(store, specNodeID, blockers)
		}
	}
	return nil
}

// checkComponentUses checks that each component in the uses list (within the
// same module) has a closed bead.
func checkComponentUses(store Store, spec SpecGraph, mod ModuleInfo, componentID int, blockers *[]Blocker) error {
	var comp *ComponentInfo
	for i := range mod.Components {
		if mod.Components[i].ID == componentID {
			comp = &mod.Components[i]
			break
		}
	}
	if comp == nil {
		return nil
	}

	for _, usedID := range comp.Uses {
		specNodeID := fmt.Sprintf("%s/component/%d", mod.Name, usedID)
		checkAllBeadsClosed(store, specNodeID, blockers)
	}
	return nil
}

// checkAllBeadsClosed verifies that every bead mapped to a spec node is closed.
// If any bead is not closed, the dependency is not satisfied.
func checkAllBeadsClosed(store Store, specNodeID string, blockers *[]Blocker) {
	recs, err := store.GetBySpecNode(specNodeID)
	if err != nil {
		*blockers = append(*blockers, Blocker{
			SpecNodeID: specNodeID,
			Reason:     "no mapping record",
		})
		return
	}
	for _, rec := range recs {
		if rec.BeadStatus != "closed" {
			*blockers = append(*blockers, Blocker{
				SpecNodeID: specNodeID,
				BeadID:     rec.BeadID,
				Reason:     fmt.Sprintf("dependency not implemented (status: %s)", rec.BeadStatus),
			})
		}
	}
}

// parseComponentID extracts the component ID from a spec_node_id like
// "module_name/component/3". Returns (3, true) or (0, false) if not a component.
func parseComponentID(specNodeID string) (int, bool) {
	parts := strings.Split(specNodeID, "/")
	if len(parts) != 3 || parts[1] != "component" {
		return 0, false
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, false
	}
	return id, true
}
