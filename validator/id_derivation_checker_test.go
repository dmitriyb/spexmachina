package validator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// REQ-5: ID uniqueness. Derivation is the premise underneath it: an identity
// hash is only an identity if it is the hash of the thing it identifies.
// CheckRemovedNames reads that premise backwards to recover the name of a node
// that no longer exists, and nothing else in the spec records that name — so a
// hand-written or stale id does not merely look odd, it makes a removal
// permanently unsweepable without saying so.

func TestREQ5_DerivedIDsValidate(t *testing.T) {
	errs := CheckIDDerivation(filepath.Join("testdata", "id_derivation_valid"))
	if len(errs) != 0 {
		t.Fatalf("want no errors for derived ids, got %v", errs)
	}
}

// TestREQ5_StaleIDReported: the id a node would carry if its name had never
// been edited. The message hands over the hash to paste rather than a command
// to run.
func TestREQ5_StaleIDReported(t *testing.T) {
	errs := CheckIDDerivation(filepath.Join("testdata", "id_derivation_stale"))
	if len(errs) != 1 {
		t.Fatalf("want exactly 1 error, got %d: %v", len(errs), errs)
	}
	e := errs[0]
	if e.Check != "id_derivation" || e.Severity != "error" {
		t.Fatalf("want check=id_derivation severity=error, got %+v", e)
	}
	want := schema.IdentityHash("core", "component", "Parser")
	if !strings.Contains(e.Message, "declares id 0000000000c9") || !strings.Contains(e.Message, want) {
		t.Fatalf("message must carry both the declared and the derived hash, got %q", e.Message)
	}
	if e.Path != "core/module.json:/components/0000000000c9" {
		t.Fatalf("want the declaring entry in the path, got %q", e.Path)
	}
}

// TestREQ5_EveryModuleScopedNodeTypeChecked pins the scope the docstring
// argues for. id_derivation_stale differs from id_derivation_valid in one
// component id and the live corpus derives everywhere, so five of the six loops
// had no subject in any test: each could be deleted in silence, quietly
// exempting a whole node type from the premise that CheckRemovedNames,
// `spex hash-id` and the merkle tree all read backwards. This fixture makes
// every module-scoped array stale at once, so a missing loop is a missing
// error.
func TestREQ5_EveryModuleScopedNodeTypeChecked(t *testing.T) {
	errs := CheckIDDerivation(filepath.Join("testdata", "id_derivation_stale_all"))

	want := map[string]string{
		"requirement":  "core/module.json:/requirements/0000000000a1",
		"component":    "core/module.json:/components/0000000000a2",
		"impl_section": "core/module.json:/impl_sections/0000000000a3",
		"data_flow":    "core/module.json:/data_flows/0000000000a4",
		"test_section": "core/module.json:/test_sections/0000000000a5",
		"api":          "core/module.json:/apis/0000000000a6",
	}
	if len(errs) != len(want) {
		t.Fatalf("want one error per module-scoped node type (%d), got %d: %v", len(want), len(errs), errs)
	}
	for nodeType, path := range want {
		found := false
		for _, e := range errs {
			if e.Path == path && strings.HasPrefix(e.Message, nodeType+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s ids are not checked for derivation: no error at %s in %v", nodeType, path, errs)
		}
	}
	for _, e := range errs {
		if e.Check != "id_derivation" || e.Severity != "error" {
			t.Fatalf("want check=id_derivation severity=error, got %+v", e)
		}
	}
}

// TestREQ5_ProjectLevelIDsExempt: this project's own project.json carries 15
// requirement ids that predate the identity-hash convention. They are
// load-bearing keys in the snapshot and in every bead-map record, so they are
// exempt rather than wrong; the fixture's project requirement id derives from
// nothing and must still validate. Requirements are the only project-level
// node type left to exempt — milestones and test_plan scenarios, which were
// exempt for the same reason, no longer exist.
func TestREQ5_ProjectLevelIDsExempt(t *testing.T) {
	errs := CheckIDDerivation(filepath.Join("testdata", "id_derivation_valid"))
	for _, e := range errs {
		if strings.HasPrefix(e.Path, "project.json") {
			t.Fatalf("project-level ids are exempt, got %+v", e)
		}
	}
}

// TestREQ5_SelfValidateIDDerivation is the claim that makes the removal sweep
// sound on this corpus: all 237 module-scoped nodes derive, so every one of
// their names can be recovered from its hash.
func TestREQ5_SelfValidateIDDerivation(t *testing.T) {
	errs := CheckIDDerivation(filepath.Join("..", "spec"))
	if len(errs) != 0 {
		t.Fatalf("spex-machina's own module-scoped ids must all derive, got %d: %v", len(errs), errs)
	}
}
