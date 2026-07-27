package validator

import "testing"

// REQ-4b399b1c568f (REQ-6): Cross-reference integrity — all reference targets
// exist. The scanner decides what counts as a reference at all; the resolver
// (link_resolver_test.go) decides whether it resolves.

func TestREQ6_ScannerFindsWikiLinks(t *testing.T) {
	refs := scanLinks("See [[3f9a1c7b2e04|`spex validate`]] and [[aabbccddeeff|Comp2]].\n")
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
	if refs[0].Target != "3f9a1c7b2e04" || refs[0].Display != "`spex validate`" {
		t.Fatalf("first ref parsed wrong: %+v", refs[0])
	}
	if !refs[0].HasDisplay || refs[0].Line != 1 {
		t.Fatalf("first ref metadata wrong: %+v", refs[0])
	}
	if refs[1].Target != "aabbccddeeff" {
		t.Fatalf("second ref parsed wrong: %+v", refs[1])
	}
}

// TestREQ6_ScannerRecordsMissingDisplayText pins that "no pipe" is
// distinguishable from "empty display text" — the resolver reports both, but
// only HasDisplay tells them apart.
func TestREQ6_ScannerRecordsMissingDisplayText(t *testing.T) {
	refs := scanLinks("bare [[3f9a1c7b2e04]] link\n")
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d", len(refs))
	}
	if refs[0].HasDisplay {
		t.Fatalf("want HasDisplay=false, got %+v", refs[0])
	}
}

// TestREQ6_ScannerTracksFences is the load-bearing case: spec/adapters/ holds
// 15 bash `[[` test brackets, every one inside a ```bash fence. A scanner
// without fence tracking reports all of them.
func TestREQ6_ScannerTracksFences(t *testing.T) {
	src := "prose\n" +
		"```bash\n" +
		"if [[ -n \"$RECEIPTS\" ]]; then echo x; fi\n" +
		"[[3f9a1c7b2e04]]\n" +
		"```\n" +
		"more prose\n"
	if refs := scanLinks(src); len(refs) != 0 {
		t.Fatalf("want 0 refs inside a bash fence, got %d: %+v", len(refs), refs)
	}
}

func TestREQ6_ScannerResumesAfterFence(t *testing.T) {
	src := "```bash\n[[ -d dir ]]\n```\nafter [[3f9a1c7b2e04|text]]\n"
	refs := scanLinks(src)
	if len(refs) != 1 {
		t.Fatalf("want 1 ref after the fence closes, got %d: %+v", len(refs), refs)
	}
	if refs[0].Line != 4 {
		t.Fatalf("want line 4, got %d", refs[0].Line)
	}
}

func TestREQ6_ScannerHandlesTildeAndLongFences(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"tilde fence", "~~~bash\n[[3f9a1c7b2e04]]\n~~~\n"},
		{"long fence", "````bash\n```\n[[3f9a1c7b2e04]]\n````\n"},
		{"indented fence", "  ```bash\n  [[3f9a1c7b2e04]]\n  ```\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if refs := scanLinks(tt.src); len(refs) != 0 {
				t.Fatalf("want 0 refs, got %d: %+v", len(refs), refs)
			}
		})
	}
}

// TestREQ6_ScannerIgnoresBareHashesOutsideDotFences pins the measured
// baseline: 144 bare 12-hex tokens live in the corpus, 99 of them fake
// fixtures. Treating any of them as a reference makes day-one false
// positives certain.
func TestREQ6_ScannerIgnoresBareHashesOutsideDotFences(t *testing.T) {
	src := "The id 3f9a1c7b2e04 is quoted as data.\n" +
		"```json\n{\"id\": \"aabbccddeeff\"}\n```\n" +
		"So is `bbccddeeff00`.\n"
	if refs := scanLinks(src); len(refs) != 0 {
		t.Fatalf("want 0 refs, got %d: %+v", len(refs), refs)
	}
}

// TestREQ6_ScannerReadsBareHashesInDotFences: `spex render --format dot`
// emits bare identity hashes as node IDs, so inside a ```dot fence a bare
// token is a reference.
func TestREQ6_ScannerReadsBareHashesInDotFences(t *testing.T) {
	src := "```dot\ndigraph G {\n  3f9a1c7b2e04 -> aabbccddeeff;\n}\n```\n"
	refs := scanLinks(src)
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
	for _, r := range refs {
		if r.Kind != kindDotNode {
			t.Fatalf("want kindDotNode, got %+v", r)
		}
		if r.Line != 3 {
			t.Fatalf("want line 3, got %d", r.Line)
		}
	}
}

// TestREQ6_ScannerRejectsHashSubstrings: a 64-hex content hash is not five
// identity hashes.
func TestREQ6_ScannerRejectsHashSubstrings(t *testing.T) {
	src := "```dot\n  label=\"1aa4d3311c1fc3d05db7d26cc185242f0c3bc789973bf86c997110d9f433f928\"\n```\n"
	if refs := scanLinks(src); len(refs) != 0 {
		t.Fatalf("want 0 refs from a 64-hex string, got %d: %+v", len(refs), refs)
	}
}

func TestREQ6_IsIdentityHash(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"3f9a1c7b2e04", true},
		{"000000000001", true},
		{"3F9A1C7B2E04", false},
		{"3f9a1c7b2e0", false},
		{"3f9a1c7b2e045", false},
		{"Comp2", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isIdentityHash(tt.in); got != tt.want {
			t.Fatalf("isIdentityHash(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// TestREQ6_ScannerSpansLineBreaks: this corpus is hard-wrapped at ~95
// columns, so a long display text breaks across two lines exactly like this.
// A line-local scanner drops the link without resolving or reporting it,
// which is a false negative in the check whose whole job is catching dangling
// references. The convention matches tokenizeCorpus: inside a construct a
// newline is whitespace, not a terminator.
func TestREQ6_ScannerSpansLineBreaks(t *testing.T) {
	refs := scanLinks("The resolver is described in [[3f9a1c7b2e04|the\nlink resolver section]] above.\n")
	if len(refs) != 1 {
		t.Fatalf("want 1 ref across the wrap, got %d: %+v", len(refs), refs)
	}
	if refs[0].Kind != kindWiki || refs[0].Target != "3f9a1c7b2e04" {
		t.Fatalf("wrapped link parsed wrong: %+v", refs[0])
	}
	if refs[0].Display != "the link resolver section" {
		t.Fatalf("want the wrap folded to a single space, got %q", refs[0].Display)
	}
	if refs[0].Line != 1 {
		t.Fatalf("want the opening line, got %d", refs[0].Line)
	}
}

// TestREQ6_ScannerReportsUnterminatedLink: a `[[` that no `]]` closes used to
// be dropped in silence. Markdown ends every inline construct at a blank
// line, a fenced block or end of input, so those are where the scanner gives
// up — and gives up loudly.
//
// The three boundaries are pinned independently, which is why no fixture ends
// in a newline: a trailing "\n" makes strings.Split yield an empty final
// element, so the blank-line branch closes every one of them and the fence and
// end-of-input branches never act on their own. wantRaw is what separates
// them further — a boundary that fails to close folds the text that follows
// into the buffer, which changes the reported Raw even when the ref count does
// not.
func TestREQ6_ScannerReportsUnterminatedLink(t *testing.T) {
	tests := []struct{ name, src, wantRaw string }{
		{
			name:    "end of input",
			src:     "See [[3f9a1c7b2e04|dangling",
			wantRaw: "[[3f9a1c7b2e04|dangling",
		},
		{
			name:    "blank line",
			src:     "See [[3f9a1c7b2e04|dangling\n\nNext paragraph.",
			wantRaw: "[[3f9a1c7b2e04|dangling",
		},
		{
			name:    "fence",
			src:     "See [[3f9a1c7b2e04|dangling\n```bash\necho hi\n```\nTrailing prose.",
			wantRaw: "[[3f9a1c7b2e04|dangling",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := scanLinks(tt.src)
			if len(refs) != 1 {
				t.Fatalf("want 1 ref, got %d: %+v", len(refs), refs)
			}
			if refs[0].Kind != kindUnterminated {
				t.Fatalf("want kindUnterminated, got %+v", refs[0])
			}
			if refs[0].Line != 1 {
				t.Fatalf("want the opening line, got %d", refs[0].Line)
			}
			if refs[0].Raw != tt.wantRaw {
				t.Fatalf("the construct must end at its boundary: want Raw %q, got %q", tt.wantRaw, refs[0].Raw)
			}
		})
	}
}

// TestREQ6_ScannerReportsBothFaultsOnOneLine is the case a forgotten `]]` used
// to hide entirely. An unterminated opening on one line, a well-formed link to
// a target that resolves to nothing on the next: searching the continuation
// line for `]]` without first checking for a `[[` pairs the pending opening
// with the second link's closer, and the whole line disappears — the missing
// `]]` is never reported and the dangling target is never resolved, so
// `spex validate` exits 0 on two hard errors. There is no nesting here, so a
// second `[[` proves the first was never closed.
func TestREQ6_ScannerReportsBothFaultsOnOneLine(t *testing.T) {
	src := "See [[3f9a1c7b2e04|the validator\n" +
		"and [[aabbccddeeff|the other thing]] too.\n"
	refs := scanLinks(src)
	if len(refs) != 2 {
		t.Fatalf("want both faults reported, got %d: %+v", len(refs), refs)
	}
	if refs[0].Kind != kindUnterminated || refs[0].Line != 1 {
		t.Fatalf("first ref must be the unterminated opening on line 1, got %+v", refs[0])
	}
	if refs[0].Raw != "[[3f9a1c7b2e04|the validator" {
		t.Fatalf("the unterminated opening must not swallow the next line, got %q", refs[0].Raw)
	}
	if refs[1].Kind != kindWiki || refs[1].Target != "aabbccddeeff" || refs[1].Line != 2 {
		t.Fatalf("second ref must be the well-formed link on line 2, got %+v", refs[1])
	}
	if refs[1].Display != "the other thing" {
		t.Fatalf("second ref display parsed wrong: %+v", refs[1])
	}
}

// TestREQ6_ScannerReportsUnterminatedOpeningTwice: the same rule applied twice
// on one line. Two forgotten closers must both be reported, not collapsed.
func TestREQ6_ScannerReportsUnterminatedOpeningTwice(t *testing.T) {
	refs := scanLinks("First [[3f9a1c7b2e04|one\nsecond [[aabbccddeeff|two\n")
	if len(refs) != 2 {
		t.Fatalf("want 2 unterminated refs, got %d: %+v", len(refs), refs)
	}
	for i, want := range []int{1, 2} {
		if refs[i].Kind != kindUnterminated || refs[i].Line != want {
			t.Fatalf("ref %d must be unterminated on line %d, got %+v", i, want, refs[i])
		}
	}
}

// TestREQ6_ScannerFoldsEveryWrapToOneSpace: a display text long enough to wrap
// twice folds through the carry-forward branch, which the two-line case never
// reaches — there the whole fold happens on the closing line. Dropping the
// separator there silently concatenates the last word of one line onto the
// first word of the next.
func TestREQ6_ScannerFoldsEveryWrapToOneSpace(t *testing.T) {
	src := "See [[3f9a1c7b2e04|the link\nresolver section described\nabove]] for details.\n"
	refs := scanLinks(src)
	if len(refs) != 1 {
		t.Fatalf("want 1 ref across two wraps, got %d: %+v", len(refs), refs)
	}
	if refs[0].Display != "the link resolver section described above" {
		t.Fatalf("every wrap must fold to a single space, got %q", refs[0].Display)
	}
}

// TestREQ6_ScannerResumesAfterCompletedWrap: the pending opening must be
// cleared once it closes, or every later `[[` on the same line is swallowed.
func TestREQ6_ScannerResumesAfterCompletedWrap(t *testing.T) {
	refs := scanLinks("Open [[3f9a1c7b2e04|first\nhalf]] then [[aabbccddeeff|second]] here.\n")
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %d: %+v", len(refs), refs)
	}
	if refs[0].Target != "3f9a1c7b2e04" || refs[1].Target != "aabbccddeeff" {
		t.Fatalf("refs parsed wrong: %+v", refs)
	}
	if refs[1].Line != 2 {
		t.Fatalf("want the second ref on line 2, got %d", refs[1].Line)
	}
}

// TestREQ6_ScannerRejectsHashWithWordCharBefore is the left-hand half of the
// word-boundary rule in scanBareHashes. The right half is pinned by
// TestREQ6_ScannerRejectsHashSubstrings, whose 64-hex fixture is preceded by a
// quote and so never exercises the left one.
func TestREQ6_ScannerRejectsHashWithWordCharBefore(t *testing.T) {
	if refs := scanLinks("```dot\n  x3f9a1c7b2e04;\n```\n"); len(refs) != 0 {
		t.Fatalf("a 12-hex tail of a longer identifier is not a node id, got %+v", refs)
	}
}

// TestREQ6_ScannerHonoursFenceIndentLimit: CommonMark allows a fence up to
// three leading spaces. At four the line is an indented code block, not a
// fence, so the text that follows is still prose the scanner must read.
func TestREQ6_ScannerHonoursFenceIndentLimit(t *testing.T) {
	refs := scanLinks("    ```dot\n[[notahash|x]]\n    ```\n")
	if len(refs) != 1 {
		t.Fatalf("want 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Kind != kindWiki || refs[0].Target != "notahash" {
		t.Fatalf("want the prose link, got %+v", refs[0])
	}
}
