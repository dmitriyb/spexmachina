package validator

import (
	"regexp"
	"strings"
)

// linkKind distinguishes the two syntaxes the scanner recognises.
type linkKind int

const (
	// kindWiki is a `[[…]]` reference written in prose.
	kindWiki linkKind = iota
	// kindDotNode is a bare 12-hex token inside a ```dot fence, where a
	// bare identity hash is the DOT node ID `spex render --format dot`
	// emits. Bare tokens anywhere else are ignored: the corpus is full of
	// hashes quoted as data, and treating them as references would make
	// every one of them a link that must resolve.
	kindDotNode
	// kindUnterminated is a `[[` that no `]]` ever closed. It is carried
	// as a reference so the resolver can report it: an opening that
	// resolves to nothing is the exact failure this checker exists to
	// catch, and dropping it would be silence.
	kindUnterminated
)

// linkRef is one reference found while scanning a content leaf.
//
// Target and Display are the two halves of `[[<target>|<display>]]`.
// HasDisplay records whether a `|` was present at all, which is what
// separates "no display text" from "empty display text" — both are errors,
// but only the first is silent otherwise.
type linkRef struct {
	Kind       linkKind
	Line       int
	Raw        string
	Target     string
	Display    string
	HasDisplay bool
}

// identityHashPattern matches a 12-character lowercase hex identity hash.
// It carries no anchors; callers reject matches whose neighbours are word
// characters, which is what keeps a 64-hex content hash from being read as
// five consecutive identity hashes.
var identityHashPattern = regexp.MustCompile(`[0-9a-f]{12}`)

// isIdentityHash reports whether s is exactly a 12-character lowercase hex
// string — the shape every spec node ID has.
func isIdentityHash(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// scanLinks walks markdown source line by line and returns every reference
// worth resolving.
//
// Fenced code blocks are tracked and their contents skipped: the 15 `[[`
// occurrences in spec/adapters/ are bash test brackets inside ```bash fences,
// and a scanner without fence tracking would report every one of them. The
// single exception is a ```dot fence, where bare identity hashes are node IDs
// and therefore real references.
//
// Inline code spans are deliberately NOT tracked. A link may contain one in
// its display text — [[3f9a1c7b2e04|`spex validate`]] — so backtick state
// cannot be used to decide whether a `[[` is live. Prose that needs to show
// the link syntax literally belongs in a fence.
//
// A link may span a line break. This corpus is hard-wrapped at ~95 columns, so
// that is exactly how a long display text breaks, and a line-local scanner
// would drop such a link without resolving or reporting it. The convention is
// the one tokenizeCorpus already applies to names: inside a construct a
// newline is whitespace, not a terminator. The construct still ends where
// markdown ends every inline construct — at a blank line, at a fenced block,
// or at end of input — and a `[[` still open at that point is returned as
// kindUnterminated rather than dropped.
func scanLinks(src string) []linkRef {
	var refs []linkRef

	var fenceChar byte
	var fenceLen int
	var fenceInfo string
	inFence := false

	var pending *pendingLink
	closePending := func() {
		if pending != nil {
			refs = append(refs, pending.unterminated())
			pending = nil
		}
	}

	for i, line := range strings.Split(src, "\n") {
		lineNo := i + 1

		if ch, n, info, ok := fenceMarker(line); ok {
			closePending()
			if !inFence {
				inFence, fenceChar, fenceLen, fenceInfo = true, ch, n, info
			} else if ch == fenceChar && n >= fenceLen && info == "" {
				inFence, fenceChar, fenceLen, fenceInfo = false, 0, 0, ""
			}
			continue
		}

		if inFence {
			if fenceInfo == "dot" {
				refs = append(refs, scanBareHashes(line, lineNo)...)
			}
			continue
		}

		if strings.TrimSpace(line) == "" {
			closePending()
			continue
		}

		var found []linkRef
		found, pending = scanWikiLinks(line, lineNo, pending)
		refs = append(refs, found...)
	}
	closePending()

	return refs
}

// pendingLink is a `[[` opened on an earlier line whose `]]` has not been seen
// yet. buf holds the text collected since the opening, with each hard wrap
// folded to a single space; line is where the opening was written, which is
// where any error about it is reported.
type pendingLink struct {
	line int
	buf  string
}

// unterminated renders the pending opening as a reference the resolver will
// reject. Raw is truncated because the buffer may hold a whole paragraph.
func (p *pendingLink) unterminated() linkRef {
	return linkRef{
		Kind: kindUnterminated,
		Line: p.line,
		Raw:  "[[" + truncateForMessage(p.buf, 48),
	}
}

// truncateForMessage shortens s to at most n runes, marking the cut.
func truncateForMessage(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// fenceMarker reports whether line opens or closes a fenced code block,
// returning the fence character, its run length and the info string. Up to
// three leading spaces are allowed, matching CommonMark.
func fenceMarker(line string) (ch byte, runLen int, info string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, 0, "", false
	}
	if len(trimmed) < 3 {
		return 0, 0, "", false
	}
	c := trimmed[0]
	if c != '`' && c != '~' {
		return 0, 0, "", false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	rest := strings.TrimSpace(trimmed[n:])
	// A backtick fence's info string may not contain a backtick.
	if c == '`' && strings.Contains(rest, "`") {
		return 0, 0, "", false
	}
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		rest = rest[:idx]
	}
	return c, n, rest, true
}

// scanWikiLinks extracts every `[[…]]` completed on one line and returns the
// opening left dangling at its end, if any. Nesting is not supported: the
// first `]]` after a `[[` closes it. pending, when non-nil, is an opening
// carried in from an earlier line; this line's text is folded onto it.
//
// A `[[` that appears before the pending `]]` ends the pending construct.
// Because there is no nesting, a second opening is proof the first was never
// closed, and the `]]` further along the line belongs to the second — pairing
// it with the first would report neither fault: the forgotten `]]` would go
// unmentioned and the following link's target would never be resolved. So the
// pending opening is emitted as kindUnterminated and the line is rescanned
// from the new opening, which reports both.
func scanWikiLinks(line string, lineNo int, pending *pendingLink) ([]linkRef, *pendingLink) {
	var refs []linkRef
	idx := 0

	if pending != nil {
		closeAt := strings.Index(line, "]]")
		openAt := strings.Index(line, "[[")
		switch {
		case openAt >= 0 && (closeAt < 0 || openAt < closeAt):
			refs = append(refs, pending.unterminated())
			idx = openAt
		case closeAt < 0:
			pending.buf += " " + strings.TrimSpace(line)
			return nil, pending
		default:
			inner := pending.buf + " " + strings.TrimLeft(line[:closeAt], " \t")
			refs = append(refs, wikiRef(pending.line, inner))
			idx = closeAt + 2
		}
	}

	for {
		rel := strings.Index(line[idx:], "[[")
		if rel < 0 {
			return refs, nil
		}
		start := idx + rel
		rel = strings.Index(line[start+2:], "]]")
		if rel < 0 {
			return refs, &pendingLink{line: lineNo, buf: line[start+2:]}
		}
		end := start + 2 + rel
		refs = append(refs, wikiRef(lineNo, line[start+2:end]))
		idx = end + 2
	}
}

// wikiRef splits the inside of a `[[…]]` into its target and display halves.
func wikiRef(lineNo int, inner string) linkRef {
	target, display, hasDisplay := strings.Cut(inner, "|")
	return linkRef{
		Kind:       kindWiki,
		Line:       lineNo,
		Raw:        "[[" + inner + "]]",
		Target:     strings.TrimSpace(target),
		Display:    strings.TrimSpace(display),
		HasDisplay: hasDisplay,
	}
}

// scanBareHashes extracts standalone 12-hex tokens from one line of a ```dot
// fence. A token whose neighbour is a word character belongs to a longer
// string (a 64-hex content hash, an identifier) and is not a reference.
func scanBareHashes(line string, lineNo int) []linkRef {
	var refs []linkRef
	for _, loc := range identityHashPattern.FindAllStringIndex(line, -1) {
		if loc[0] > 0 && isWordByte(line[loc[0]-1]) {
			continue
		}
		if loc[1] < len(line) && isWordByte(line[loc[1]]) {
			continue
		}
		token := line[loc[0]:loc[1]]
		refs = append(refs, linkRef{
			Kind:   kindDotNode,
			Line:   lineNo,
			Raw:    token,
			Target: token,
		})
	}
	return refs
}

func isWordByte(c byte) bool {
	return c == '_' ||
		(c >= '0' && c <= '9') ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z')
}
