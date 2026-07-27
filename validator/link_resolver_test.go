package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmitriyb/spexmachina/schema"
)

// REQ-4b399b1c568f (REQ-6): Cross-reference integrity — all reference targets
// exist. A [[<hash>|<display text>]] link is a reference written in prose
// rather than in JSON, and resolves against the merkle leaf keys.

// The fixture spec is one module, "alpha", with three components. Comp1
// carries the markdown under test, Comp2 is a live link target, and Comp3
// declares no content — so it is not a merkle leaf, and so not linkable.
// Ids are derived, not invented, so a test can name a target before the
// fixture exists.
func alphaModuleID() string          { return schema.IdentityHash("module", "alpha") }
func alphaCompID(name string) string { return schema.IdentityHash("alpha", "component", name) }

// deadHash is a well-formed identity hash belonging to no node in the fixture.
const deadHash = "0123456789ab"

func newLinkFixture(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()

	writeProject(t, dir, `{
  "name": "link-test",
  "modules": [{"id": "`+alphaModuleID()+`", "name": "alpha", "path": "alpha"}]
}`)

	modDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, modDir, "module.json", `{
  "name": "alpha",
  "components": [
    {"id": "`+alphaCompID("Comp1")+`", "name": "Comp1", "content": "arch_comp1.md"},
    {"id": "`+alphaCompID("Comp2")+`", "name": "Comp2", "content": "arch_comp2.md"},
    {"id": "`+alphaCompID("Comp3")+`", "name": "Comp3"}
  ]
}`)
	writeFile(t, modDir, "arch_comp1.md", body)
	writeFile(t, modDir, "arch_comp2.md", "# Comp2\n")

	return dir
}

// wantOneLinkError asserts exactly one error, tagged as this checker's, whose
// message names the problem.
func wantOneLinkError(t *testing.T, errs []ValidationError, substr string) {
	t.Helper()
	if len(errs) != 1 {
		t.Fatalf("want 1 error containing %q, got %d: %v", substr, len(errs), errs)
	}
	if errs[0].Check != "link" {
		t.Fatalf("want check %q, got %q", "link", errs[0].Check)
	}
	if errs[0].Severity != "error" {
		t.Fatalf("want severity %q, got %q", "error", errs[0].Severity)
	}
	if !strings.Contains(errs[0].Message, substr) {
		t.Fatalf("want message containing %q, got %q", substr, errs[0].Message)
	}
}

func TestREQ6_LinkToLiveNodeResolves(t *testing.T) {
	dir := newLinkFixture(t, "Comp1 calls [["+alphaCompID("Comp2")+"|`Comp2`]] on every run.\n")

	if errs := CheckLinks(dir); len(errs) != 0 {
		t.Fatalf("want 0 errors for a link to a live node, got %d: %v", len(errs), errs)
	}
}

func TestREQ6_LinkToRemovedNodeErrors(t *testing.T) {
	dir := newLinkFixture(t, "Comp1 calls [["+deadHash+"|Gone]] which no longer exists.\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "does not resolve to any spec node")
	if !strings.HasPrefix(errs[0].Path, "alpha/arch_comp1.md:") {
		t.Fatalf("want a file:line path, got %q", errs[0].Path)
	}
}

func TestREQ6_LinkWithoutDisplayTextErrors(t *testing.T) {
	tests := []struct{ name, body string }{
		{"no pipe", "Comp1 calls [[" + alphaCompID("Comp2") + "]] today.\n"},
		{"empty display", "Comp1 calls [[" + alphaCompID("Comp2") + "|]] today.\n"},
		{"blank display", "Comp1 calls [[" + alphaCompID("Comp2") + "|   ]] today.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := CheckLinks(newLinkFixture(t, tt.body))
			wantOneLinkError(t, errs, "has no display text")
		})
	}
}

// TestREQ6_LinkToModuleNodeErrors: collectLeafHashes takes Type == "leaf" and
// modules are Type == "module", so a module hash can never resolve. The
// message says why rather than claiming the hash is unknown.
func TestREQ6_LinkToModuleNodeErrors(t *testing.T) {
	dir := newLinkFixture(t, "See [["+alphaModuleID()+"|the alpha module]].\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "module nodes are not linkable")
}

// TestREQ6_ContentlessNodeNotLinkable: a component with no content file is
// skipped by the tree builder, so ingest never sees it as a leaf and neither
// does the resolver.
func TestREQ6_ContentlessNodeNotLinkable(t *testing.T) {
	dir := newLinkFixture(t, "See [["+alphaCompID("Comp3")+"|Comp3]].\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "does not resolve to any spec node")
}

// TestREQ6_NameBasedLinkRejected: a name carries no <type> segment, so it
// cannot be turned into an identity hash, and "Identity hash algorithm" is
// already both a requirement and an impl_section in one module.
func TestREQ6_NameBasedLinkRejected(t *testing.T) {
	dir := newLinkFixture(t, "See [[Comp2|the second component]].\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "not a 12-character identity hash")
}

// TestREQ6_BareIdentityHashIgnored is the false-positive gate: the corpus
// quotes bare identity hashes as data everywhere, including hashes of nodes
// that do not exist.
func TestREQ6_BareIdentityHashIgnored(t *testing.T) {
	dir := newLinkFixture(t, "The id "+deadHash+" is written bare, and `"+deadHash+"` in a code span.\n")

	if errs := CheckLinks(dir); len(errs) != 0 {
		t.Fatalf("want 0 errors for bare hashes, got %d: %v", len(errs), errs)
	}
}

// TestREQ6_LinkInsideBashFenceIgnored: the 15 `[[` occurrences in
// spec/adapters/ are bash test brackets inside ```bash fences.
func TestREQ6_LinkInsideBashFenceIgnored(t *testing.T) {
	body := "Run the adapter:\n\n```bash\n" +
		"if [[ -n \"$RECEIPTS\" ]]; then :; fi\n" +
		"[[" + deadHash + "]]\n" +
		"[[NotAHash|text]]\n" +
		"```\n"
	dir := newLinkFixture(t, body)

	if errs := CheckLinks(dir); len(errs) != 0 {
		t.Fatalf("want 0 errors inside a bash fence, got %d: %v", len(errs), errs)
	}
}

func TestREQ6_DotFenceHashesResolve(t *testing.T) {
	live := "```dot\ndigraph G {\n  " + alphaCompID("Comp2") + ";\n}\n```\n"
	if errs := CheckLinks(newLinkFixture(t, live)); len(errs) != 0 {
		t.Fatalf("want 0 errors for a live dot node id, got %d: %v", len(errs), errs)
	}

	dead := "```dot\ndigraph G {\n  " + deadHash + ";\n}\n```\n"
	errs := CheckLinks(newLinkFixture(t, dead))
	wantOneLinkError(t, errs, "does not resolve to any spec node")
}

func TestREQ6_MultipleLinkErrorsReported(t *testing.T) {
	body := "One [[" + deadHash + "|Gone]].\n" +
		"Two [[" + alphaCompID("Comp2") + "]].\n" +
		"Three [[Comp2|name]].\n"
	errs := CheckLinks(newLinkFixture(t, body))

	if len(errs) != 3 {
		t.Fatalf("want 3 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.Check != "link" || e.Severity != "error" {
			t.Fatalf("every link error is check=link severity=error, got %+v", e)
		}
	}
}

// TestREQ6_UnreadableSpecReportsOwnError: the checker cannot resolve anything
// without a merkle tree, and says so under its own name rather than failing
// silently.
func TestREQ6_UnreadableSpecReportsOwnError(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, `{
  "name": "broken",
  "modules": [{"id": "000000000001", "name": "alpha", "path": "alpha"}]
}`)
	modDir := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, modDir, "module.json", `{
  "name": "alpha",
  "components": [{"id": "000000000002", "name": "Comp1", "content": "missing.md"}]
}`)

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "cannot resolve links")
}

func TestREQ6_SelfValidateLinks(t *testing.T) {
	errs := CheckLinks(filepath.Join("..", "spec"))
	if len(errs) > 0 {
		t.Fatalf("spex-machina's own spec should have no link errors, got %d: %v", len(errs), errs)
	}
}

// TestREQ6_WrappedLinkResolves is the reviewer probe that used to pass in
// silence: `[[<hash>|` with the display text closing on the next line. The
// corpus is hard-wrapped, so this is how a long display text breaks, and the
// target resolving to nothing must be an error rather than nothing at all.
func TestREQ6_WrappedLinkResolves(t *testing.T) {
	dir := newLinkFixture(t, "Comp1 calls [["+deadHash+"|the\nsplit display text]] on every run.\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "does not resolve to any spec node")
	if !strings.HasSuffix(errs[0].Path, "arch_comp1.md:1") {
		t.Fatalf("want the error on the opening line, got %q", errs[0].Path)
	}

	live := "Comp1 calls [[" + alphaCompID("Comp2") + "|the\nsplit display text]] on every run.\n"
	if errs := CheckLinks(newLinkFixture(t, live)); len(errs) != 0 {
		t.Fatalf("a wrapped link to a live node resolves, got %v", errs)
	}
}

// TestREQ6_UnterminatedLinkErrors: an opening with no closing resolves to
// nothing and must say so. Silence is the one unacceptable outcome for the
// checker whose job is catching dangling references.
func TestREQ6_UnterminatedLinkErrors(t *testing.T) {
	dir := newLinkFixture(t, "Comp1 calls [["+alphaCompID("Comp2")+"|Comp2 and never closes it.\n")

	errs := CheckLinks(dir)
	wantOneLinkError(t, errs, "unterminated link")
}
