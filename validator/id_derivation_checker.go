package validator

import (
	"fmt"

	"github.com/dmitriyb/spexmachina/schema"
)

// CheckIDDerivation validates that every module-scoped node's declared id is
// the identity hash of its own (module, type, name).
//
// # Why this is a check and not an assumption
//
// Three mechanisms read an identity hash backwards, deriving meaning from the
// fact that a key equals IdentityHash(module, type, name):
//
//   - the removal-time name sweep (CheckRemovedNames) recovers the name of a
//     node that no longer exists by hashing corpus phrases against its key —
//     the name is stored nowhere else, so a hand-written id makes the removal
//     permanently unsweepable, silently;
//   - `spex hash-id` hands authors the id a node must carry;
//   - `spex map context` and the merkle tree treat the id as the node's
//     stable identity across renames of everything except the name itself.
//
// Nothing enforced the premise. A stale id — one left behind when a name was
// edited — validates clean and quietly breaks all three. This check closes
// that: an id that does not derive is reported against the node that declares
// it, with the hash it should carry.
//
// # Scope
//
// Module-scoped nodes only: requirements, components, impl_sections,
// data_flows, test_sections and apis declared in a module.json. Project-level
// requirement ids are the sole remaining exemption — 15 of the ones this
// project's own project.json carries predate the identity-hash convention
// (`Render spec` declares 6b00623735ac against a computed 060ca1db054d) — and
// correcting them would rewrite the snapshot and every bead-map record keyed
// off them. They are exempt, not "correct". Milestones and test_plan scenarios
// used to be exempt on the same grounds; both node types are gone from the
// schema, so requirements are the only project-level exemption left. Module ids
// in project.json do all derive today, but they are not checked either: the
// merkle tree keys module nodes by the declared id, so a spec whose module ids
// are synthetic is still internally consistent, and several fixtures outside
// this package rely on that.
//
// The one consequence worth naming: because a module id is unchecked, the
// module-name recovery in CheckRemovedNames can fail on a spec whose module
// ids were hand-written, which is exactly when it emits a
// NoteUnverifiableModule note instead of a silent skip.
func CheckIDDerivation(specDir string) []ValidationError {
	project, modules, errs := loadSpec(specDir, "id_derivation")
	if len(errs) > 0 {
		return errs
	}

	var result []ValidationError
	for _, mod := range project.Modules {
		modSpec, ok := modules[mod.Name]
		if !ok {
			continue
		}
		prefix := mod.Name + "/module.json:"

		for _, r := range modSpec.Requirements {
			result = append(result, derivedIDError(prefix, "requirements", mod.Name, "requirement", r.Title, r.ID)...)
		}
		for _, c := range modSpec.Components {
			result = append(result, derivedIDError(prefix, "components", mod.Name, "component", c.Name, c.ID)...)
		}
		for _, s := range modSpec.ImplSections {
			result = append(result, derivedIDError(prefix, "impl_sections", mod.Name, "impl_section", s.Name, s.ID)...)
		}
		for _, f := range modSpec.DataFlows {
			result = append(result, derivedIDError(prefix, "data_flows", mod.Name, "data_flow", f.Name, f.ID)...)
		}
		for _, ts := range modSpec.TestSections {
			result = append(result, derivedIDError(prefix, "test_sections", mod.Name, "test_section", ts.Name, ts.ID)...)
		}
		for _, a := range modSpec.APIs {
			result = append(result, derivedIDError(prefix, "apis", mod.Name, "api", a.Name, a.ID)...)
		}
	}
	return result
}

// derivedIDError reports one node whose declared id is not its identity hash.
// The message carries the hash the node must use, so the fix is a copy rather
// than a `spex hash-id` invocation.
func derivedIDError(prefix, array, module, nodeType, name, id string) []ValidationError {
	want := schema.IdentityHash(module, nodeType, name)
	if id == want {
		return nil
	}
	return []ValidationError{{
		Check:    "id_derivation",
		Severity: "error",
		Path:     fmt.Sprintf("%s/%s/%s", prefix, array, id),
		Message: fmt.Sprintf("%s %q declares id %s but its identity hash is %s; a module-scoped id must equal IdentityHash(%q, %q, %q) or the node cannot be recovered from its hash after removal",
			nodeType, name, id, want, module, nodeType, name),
	}}
}
