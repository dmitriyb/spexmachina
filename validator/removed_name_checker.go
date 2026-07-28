package validator

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dmitriyb/spexmachina/merkle"
	"github.com/dmitriyb/spexmachina/schema"
)

// maxNameWords bounds the phrase length the corpus scan considers. API names
// are the long ones — "spex render --format json --slim" is five whitespace
// tokens; component names are single identifiers.
//
// The same constant bounds what CheckIDs will accept as a declared api or
// component name (checkNameRecoverability, via declarableName below). The two
// uses share one constant on purpose: a name the sweep cannot build is a name
// the sweep can never find, so it must not be declarable in the first place.
// Moving the bound in either direction changes both halves at once, and the
// tests pin both.
const maxNameWords = 6

// corpusDirSkip is the one subdirectory of the spec tree that is outside the
// gate. Proposals are historical documents: they describe the change that
// removed the node and must go on naming it. README.md, docs/ and skills/ are
// outside the gate too, and are outside specDir already.
const corpusDirSkip = "proposals"

// Note kinds. A note is a disclosure, not a failure: it records work the sweep
// could not do, or hits it deliberately discarded, so that neither outcome is
// reached in silence.
const (
	// NoteUnverifiableModule is emitted when a removed node's module was
	// removed too and its name cannot be recovered, leaving no identity to
	// hash candidate phrases against.
	NoteUnverifiableModule = "unverifiable_module"
	// NoteSuppressedByLiveName is emitted when longest-match subtraction
	// discarded hits, and names the live node that consumed them.
	NoteSuppressedByLiveName = "suppressed_by_live_name"
)

// SurvivingName is one removed api or component whose declared name is still
// written somewhere in the spec corpus.
//
// Name is recovered, not remembered: see CheckRemovedNames.
type SurvivingName struct {
	// Key is the identity hash of the removed node.
	Key string
	// Name is the node's declared name, recovered from the corpus.
	Name string
	// NodeType is "api" or "component".
	NodeType string
	// Module is the name of the module that declared the node.
	Module string
	// Sites are "<path>:<line>" locations, relative to the spec dir,
	// sorted and deduplicated.
	Sites []string
}

// RemovedNameNote is one disclosure from the sweep. Notes never halt the
// pipeline — they are not violations — but every one of them describes a place
// where the sweep's answer is weaker than "checked and clean", which the author
// has no other way to learn.
type RemovedNameNote struct {
	// Kind is NoteUnverifiableModule or NoteSuppressedByLiveName.
	Kind string
	// Message is the human-readable disclosure.
	Message string
	// Keys are the removed identity hashes the note concerns, sorted.
	Keys []string
}

// RemovedNameReport is the outcome of one removal-time name sweep: the
// failures, and everything the sweep chose not to report as a failure.
type RemovedNameReport struct {
	Survivors []SurvivingName
	Notes     []RemovedNameNote
}

// CheckRemovedNames searches the spec corpus for the declared names of nodes
// the diff reports as removed, and returns one finding per node whose name
// survives somewhere, plus a note for every removal it could not fully check.
//
// Only api and component names are searched. Other node types carry generic
// noun phrases for names — "Hash computation" alone survives sixteen times in
// another module's test leaves — and searching them would report the corpus
// as broken on every removal.
//
// # Recovering the name of a node that no longer exists
//
// A removed node is gone from module.json, and the snapshot stores only
// hashes: no name is available to search for. But the name does not need to
// be remembered, because an identity hash is exactly
// IdentityHash(module, type, name). So the scan runs the other way: every
// phrase in the corpus is hashed under the removed node's (module, type) and
// compared against the removed key. A match is a proof that the phrase IS the
// removed node's declared name — the check therefore has no false positives
// beyond a 48-bit hash collision. Its failure mode is the opposite one: a
// mention whose surrounding punctuation the tokenizer does not strip is
// missed.
//
// This is also the only mechanism that can work for api nodes at all. APIs
// produce no beads, so they never appear in .bead-map.json, the one other
// place that records a hash → name pairing. It is not, however, the only
// mechanism for components: see beadMapNames.
//
// # Longest-match-first
//
// A hit is discarded when a live api or component name covers the same words
// and is at least as long. "spex map" occurs 34 times in this corpus, 29 of
// them inside "spex map get", "spex map list" or "spex map context"; without
// the subtraction, removing the bare "spex map" api would report 34 survivors
// for 5 real ones. The "at least as long" half covers the degenerate case
// where the identical name is still live in another module: the text denotes
// the live node and there is nothing to sweep. Because the live-name set is
// global rather than per-module, that second half can swallow every hit for a
// removal; whenever it discards anything a NoteSuppressedByLiveName note says
// so and names the live node responsible.
//
// # A removed node whose module was removed too
//
// Retiring a whole module is the highest-volume removal there is, and it is
// the case where the module name — the first component of every identity
// string — is gone from project.json. `spex diff` then reports the module as
// its identity hash rather than its name.
//
// Two sources can prove the name back. The corpus is tried first: phrases are
// hashed as IdentityHash("module", phrase) against the module hash. That
// source has a perverse property on its own — it works only while the module
// name still survives in prose, so the sweep's reach was inversely coupled to
// how thoroughly the removal was swept. The bead map closes it: every
// component bead record carries its module and component names against the
// spec node id, so beadMapNames can prove the module name from data the tool
// already ships, whether or not a single mention remains. Only when both
// sources come up empty is the group reported as a NoteUnverifiableModule note
// rather than skipped in silence.
//
// beadMapPath may be empty, and the file may be absent: either way the sweep
// falls back to the corpus alone.
func CheckRemovedNames(specDir, beadMapPath string, changes []merkle.ClassifiedChange) (RemovedNameReport, error) {
	var report RemovedNameReport

	targets := removedNameTargets(changes)
	if len(targets) == 0 {
		return report, nil
	}

	project, modules, errs := loadSpec(specDir, "removed_name")
	if len(errs) > 0 {
		return report, fmt.Errorf("validator: removed-name check: %s", errs[0].Message)
	}

	live := map[string][]liveNode{}
	known := map[string]bool{}
	for _, mod := range project.Modules {
		known[mod.Name] = true
		modSpec, ok := modules[mod.Name]
		if !ok {
			continue
		}
		for _, c := range modSpec.Components {
			live[c.Name] = append(live[c.Name], liveNode{module: mod.Name, nodeType: "component"})
		}
		for _, a := range modSpec.APIs {
			live[a.Name] = append(live[a.Name], liveNode{module: mod.Name, nodeType: "api"})
		}
	}

	var groups, orphans []nameTargetGroup
	for _, g := range targets {
		if known[g.module] {
			groups = append(groups, g)
		} else {
			orphans = append(orphans, g)
		}
	}

	if len(orphans) > 0 {
		recovered, err := recoverModuleNames(specDir, orphans)
		if err != nil {
			return report, err
		}
		beadMap, err := loadBeadMapNames(beadMapPath)
		if err != nil {
			return report, err
		}
		for _, g := range orphans {
			name, ok := recovered[g.module]
			if !ok {
				name, ok = beadMap.moduleName(g)
			}
			if !ok {
				report.Notes = append(report.Notes, unverifiableModuleNote(g, beadMap))
				continue
			}
			g.module = name
			groups = append(groups, g)
		}
	}

	if len(groups) > 0 {
		scan := &corpusScan{
			groups:     groups,
			live:       live,
			found:      map[string]*SurvivingName{},
			suppressed: map[string]*suppressedHits{},
		}
		err := walkCorpus(specDir, func(rel string, data []byte) {
			scan.scanFile(rel, string(data))
		})
		if err != nil {
			return report, err
		}
		report.Survivors = scan.survivors()
		report.Notes = append(report.Notes, scan.suppressionNotes()...)
	}

	slices.SortFunc(report.Notes, func(a, b RemovedNameNote) int {
		if c := strings.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	})
	return report, nil
}

// liveNode identifies one api or component that still exists, so a note can
// name the node that consumed a hit rather than only the string it shares.
type liveNode struct {
	module   string
	nodeType string
}

// nameTargetGroup is one (module, node type) pair plus the removed keys
// declared under it. Grouping matters: the corpus is hashed once per pair,
// not once per removed node.
//
// module holds the module's name once it is known. Before that it may hold the
// module's identity hash, which is what `spex diff` reports when the module was
// itself removed.
type nameTargetGroup struct {
	module   string
	nodeType string
	keys     map[string]bool
}

// sortedKeys returns the group's removed keys in a stable order.
func (g nameTargetGroup) sortedKeys() []string {
	keys := make([]string, 0, len(g.keys))
	for k := range g.keys {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// removedNameTargets selects the removed api and component changes and groups
// them by (module name, node type).
func removedNameTargets(changes []merkle.ClassifiedChange) []nameTargetGroup {
	index := map[string]*nameTargetGroup{}
	var order []string
	for _, c := range changes {
		if c.Type != merkle.Removed {
			continue
		}
		if c.NodeType != "api" && c.NodeType != "component" {
			continue
		}
		if c.Module == "" {
			continue
		}
		k := c.Module + "\x00" + c.NodeType
		g, ok := index[k]
		if !ok {
			g = &nameTargetGroup{module: c.Module, nodeType: c.NodeType, keys: map[string]bool{}}
			index[k] = g
			order = append(order, k)
		}
		g.keys[c.Key] = true
	}

	groups := make([]nameTargetGroup, 0, len(order))
	for _, k := range order {
		groups = append(groups, *index[k])
	}
	return groups
}

// beadMapNames is the hash → name table .bead-map.json already ships, indexed
// the two ways the sweep can use it.
//
// The bead map was dismissed as a source because apis produce no beads. That
// is true of apis and false of components, which are the only node type the
// corpus declares today: a component record carries its module name, its
// component name and the spec node id those two hash into. So a module whose
// name is nowhere in the remaining prose is still recoverable — from data the
// tool wrote itself, at the time the node existed.
//
// Nothing here is trusted on its word. Both lookups are proofs of the same
// kind the corpus scan uses: a name is accepted only when it hashes to the
// key being looked up, so a stale or hand-edited record simply fails to match.
type beadMapNames struct {
	// modules maps IdentityHash("module", name) to name.
	modules map[string]string
	// nodes maps a record's spec_node_id to the record.
	nodes map[string]schema.BeadMapRecord
}

// loadBeadMapNames indexes the bead map at path. An empty path or an absent
// file yields a nil index, which every method treats as "no names known" —
// `spex diff` must work in a tree that has never been ingested. A file that
// exists and cannot be read or parsed is an error: it is the same corruption
// every other command that touches the map reports.
func loadBeadMapNames(path string) (*beadMapNames, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validator: removed-name check: read %s: %w", path, err)
	}
	var bm schema.BeadMap
	if err := json.Unmarshal(data, &bm); err != nil {
		return nil, fmt.Errorf("validator: removed-name check: parse %s: %w", path, err)
	}

	idx := &beadMapNames{
		modules: make(map[string]string, len(bm.Records)),
		nodes:   make(map[string]schema.BeadMapRecord, len(bm.Records)),
	}
	for _, r := range bm.Records {
		if r.Module != "" {
			idx.modules[schema.IdentityHash("module", r.Module)] = r.Module
		}
		if r.SpecNodeID != "" {
			if _, seen := idx.nodes[r.SpecNodeID]; !seen {
				idx.nodes[r.SpecNodeID] = r
			}
		}
	}
	return idx, nil
}

// moduleName proves the name of an orphan group's module from the bead map.
//
// Two routes, both hash-verified. The first reads the module hash directly:
// a module id is IdentityHash("module", name), so a record naming that module
// reproduces it. The second is for a module whose declared id was hand-written
// and so derives from nothing (CheckIDDerivation exempts module ids): a record
// for one of the removed keys reproduces that key from its own (module,
// nodeType, component) triple, which proves the module name just as well.
func (b *beadMapNames) moduleName(g nameTargetGroup) (string, bool) {
	if b == nil {
		return "", false
	}
	if name, ok := b.modules[g.module]; ok {
		return name, true
	}
	for _, key := range g.sortedKeys() {
		rec, ok := b.nodes[key]
		if !ok || rec.Module == "" {
			continue
		}
		if schema.IdentityHash(rec.Module, g.nodeType, rec.Component) == key {
			return rec.Module, true
		}
	}
	return "", false
}

// describeKey renders one removed key for a note, adding the name the bead map
// records for it when there is one. The record cannot be trusted here — if it
// hashed back to the key, moduleName would have proved the module name from it
// and this note would not exist — so it is labelled as the unverified lead it
// is. It is still the difference between a hash a reader can act on and one
// they cannot.
func (b *beadMapNames) describeKey(key string) string {
	if b == nil {
		return key
	}
	rec, ok := b.nodes[key]
	if !ok || rec.Component == "" {
		return key
	}
	return fmt.Sprintf("%s (bead map records %q in module %q, unverified: it does not hash back to the key)",
		key, rec.Component, rec.Module)
}

// unverifiableModuleNote reports a group the sweep cannot check at all: the
// module that declared these nodes is gone, its name is nowhere in the corpus,
// and the bead map cannot prove it either, so no candidate phrase can be
// hashed into their identity hashes.
//
// This note does not gate, and that is a deliberate choice rather than the
// original reasoning carried forward. "I did not check" is worse than
// "suppressed_by_live_name", which is a correct answer — but after the bead
// map closes the recoverable case, what is left is a removal the author has no
// action that clears: the only way to make the module name recoverable from
// the corpus is to write it back into the prose, which is the exact opposite
// of sweeping it, and the only way to make it recoverable from the bead map is
// to have ingested the node before it was removed. Gating on it would be a
// halt with no remedy. So it stays loud and non-blocking, and names everything
// it can name.
func unverifiableModuleNote(g nameTargetGroup, beadMap *beadMapNames) RemovedNameNote {
	keys := g.sortedKeys()
	described := make([]string, len(keys))
	for i, k := range keys {
		described[i] = beadMap.describeKey(k)
	}
	return RemovedNameNote{
		Kind: NoteUnverifiableModule,
		Message: fmt.Sprintf(
			"%d removed %s node(s) under module %s could not be checked: the module was removed too and its name is recoverable neither from the corpus nor from the bead map, so their identity hashes cannot be reproduced; sweep any mention of them by hand (keys: %s)",
			len(keys), g.nodeType, g.module, strings.Join(described, ", ")),
		Keys: keys,
	}
}

// recoverModuleNames hashes corpus phrases as IdentityHash("module", phrase)
// against the module hashes of the given groups, returning the names it
// proves. This works because a module's declared id is itself an identity
// hash of its name; a hand-written module id is unrecoverable, and its group
// becomes a NoteUnverifiableModule note.
func recoverModuleNames(specDir string, orphans []nameTargetGroup) (map[string]string, error) {
	wanted := map[string]bool{}
	for _, g := range orphans {
		wanted[g.module] = true
	}

	names := make(map[string]string, len(wanted))
	err := walkCorpus(specDir, func(rel string, data []byte) {
		if len(names) == len(wanted) {
			return
		}
		tokens := tokenizeCorpus(string(data))
		for i := range tokens {
			phrase := tokens[i].text
			for n := 1; n <= maxNameWords && i+n <= len(tokens); n++ {
				if n > 1 {
					phrase += " " + tokens[i+n-1].text
				}
				h := schema.IdentityHash("module", phrase)
				if wanted[h] {
					if _, seen := names[h]; !seen {
						names[h] = phrase
					}
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// walkCorpus visits every markdown and JSON file under specDir that the gate
// covers. Dot-prefixed entries (.snapshot.json) are generated state, not
// prose, and the proposals directory is out of scope by design.
func walkCorpus(specDir string, visit func(rel string, data []byte)) error {
	return filepath.WalkDir(specDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("validator: removed-name check: walk %s: %w", path, err)
		}
		rel, relErr := filepath.Rel(specDir, path)
		if relErr != nil {
			return fmt.Errorf("validator: removed-name check: %w", relErr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || rel == corpusDirSkip {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if ext := filepath.Ext(name); ext != ".md" && ext != ".json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("validator: removed-name check: read %s: %w", rel, readErr)
		}
		visit(rel, data)
		return nil
	})
}

// corpusHit is one phrase occurrence whose identity hash matched a removed
// key, held as a token span so the live-name subtraction can be applied.
type corpusHit struct {
	start int
	words int
	line  int
	name  string
	group *nameTargetGroup
	key   string
}

// liveSpan is one occurrence of a live api or component name, held as a token
// span plus the name itself so a suppression note can identify it.
type liveSpan struct {
	words int
	name  string
}

// suppressedHits accumulates, per removed key, the hits longest-match
// subtraction discarded and the live names that did the discarding.
type suppressedHits struct {
	key      string
	name     string
	nodeType string
	module   string
	count    int
	coverers map[string]bool
}

// corpusScan carries the state of one sweep across every corpus file.
type corpusScan struct {
	groups     []nameTargetGroup
	live       map[string][]liveNode
	found      map[string]*SurvivingName
	suppressed map[string]*suppressedHits
}

// scanFile hashes every 1..maxNameWords phrase in one file against each
// group's removed keys, then keeps the hits no live name covers.
func (s *corpusScan) scanFile(rel, src string) {
	tokens := tokenizeCorpus(src)
	if len(tokens) == 0 {
		return
	}

	liveSpans := map[int][]liveSpan{}
	var hits []corpusHit

	for i := range tokens {
		phrase := tokens[i].text
		for n := 1; n <= maxNameWords && i+n <= len(tokens); n++ {
			if n > 1 {
				phrase += " " + tokens[i+n-1].text
			}
			if len(s.live[phrase]) > 0 {
				liveSpans[i] = append(liveSpans[i], liveSpan{words: n, name: phrase})
			}
			for gi := range s.groups {
				g := &s.groups[gi]
				key := schema.IdentityHash(g.module, g.nodeType, phrase)
				if g.keys[key] {
					hits = append(hits, corpusHit{
						start: i, words: n, line: tokens[i].line,
						name: phrase, group: g, key: key,
					})
				}
			}
		}
	}

	for _, h := range hits {
		if cover, covered := coveringLiveName(h, liveSpans); covered {
			s.recordSuppressed(h, cover)
			continue
		}
		sn, ok := s.found[h.key]
		if !ok {
			sn = &SurvivingName{Key: h.key, Name: h.name, NodeType: h.group.nodeType, Module: h.group.module}
			s.found[h.key] = sn
		}
		sn.Sites = append(sn.Sites, fmt.Sprintf("%s:%d", rel, h.line))
	}
}

// recordSuppressed remembers a discarded hit and the live name that covered it.
func (s *corpusScan) recordSuppressed(h corpusHit, cover string) {
	sup, ok := s.suppressed[h.key]
	if !ok {
		sup = &suppressedHits{
			key: h.key, name: h.name, nodeType: h.group.nodeType,
			module: h.group.module, coverers: map[string]bool{},
		}
		s.suppressed[h.key] = sup
	}
	sup.count++
	for _, owner := range s.live[cover] {
		sup.coverers[fmt.Sprintf("%s %q in module %s", owner.nodeType, cover, owner.module)] = true
	}
}

// survivors returns the findings with their sites sorted and deduplicated.
func (s *corpusScan) survivors() []SurvivingName {
	out := make([]SurvivingName, 0, len(s.found))
	for _, sn := range s.found {
		slices.Sort(sn.Sites)
		sn.Sites = slices.Compact(sn.Sites)
		out = append(out, *sn)
	}
	slices.SortFunc(out, func(a, b SurvivingName) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// suppressionNotes turns the discarded hits into disclosures. Call after
// survivors, which is what fixes the reported site counts.
func (s *corpusScan) suppressionNotes() []RemovedNameNote {
	keys := make([]string, 0, len(s.suppressed))
	for k := range s.suppressed {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	notes := make([]RemovedNameNote, 0, len(keys))
	for _, k := range keys {
		sup := s.suppressed[k]
		coverers := make([]string, 0, len(sup.coverers))
		for c := range sup.coverers {
			coverers = append(coverers, c)
		}
		slices.Sort(coverers)

		reported := 0
		if sn, ok := s.found[k]; ok {
			reported = len(sn.Sites)
		}
		notes = append(notes, RemovedNameNote{
			Kind: NoteSuppressedByLiveName,
			Message: fmt.Sprintf(
				"removed %s %q (%s): %d mention(s) not reported because a live node of the same or longer name covers them (%s); %d site(s) reported",
				sup.nodeType, sup.name, sup.key, sup.count, strings.Join(coverers, ", "), reported),
			Keys: []string{sup.key},
		})
	}
	return notes
}

// coveringLiveName reports the live api or component name occupying a span
// that contains the hit and is at least as long, if there is one.
func coveringLiveName(h corpusHit, liveSpans map[int][]liveSpan) (string, bool) {
	first := h.start - maxNameWords + 1
	if first < 0 {
		first = 0
	}
	for j := first; j <= h.start; j++ {
		for _, sp := range liveSpans[j] {
			if sp.words >= h.words && j+sp.words >= h.start+h.words {
				return sp.name, true
			}
		}
	}
	return "", false
}

// corpusToken is one whitespace-delimited word, stripped of the markdown and
// JSON punctuation that wraps prose, plus the line it started on.
type corpusToken struct {
	text string
	line int
}

// tokenTrimCutset is the punctuation stripped from both ends of every token.
// It deliberately excludes '/', '-', '{', '}', '<', '>' and '|': an api name
// is written the way callers type it, and "GET /v1/specs/{id}" and
// "--format" must survive tokenization intact.
const tokenTrimCutset = "`*_\"'()[],;:!?“”‘’"

// tokenizeCorpus splits source text into normalized tokens. Splitting spans
// line breaks so a name that a paragraph wraps across two lines is still one
// phrase; the recorded line is where the phrase starts. scanLinks takes the
// same position on a hard wrap — a newline inside a construct is whitespace,
// not a terminator.
func tokenizeCorpus(src string) []corpusToken {
	tokens := make([]corpusToken, 0, len(src)/8+1)
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		if c == '\n' {
			line++
			i++
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f' {
			i++
			continue
		}
		j := i
		for j < len(src) && !isSpaceByte(src[j]) {
			j++
		}
		if t := normalizeToken(src[i:j]); t != "" {
			tokens = append(tokens, corpusToken{text: t, line: line})
		}
		i = j
	}
	return tokens
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// nameTokens returns what corpus tokenization makes of a declared name. It
// runs the name through the very tokenizer the sweep runs over prose, so
// there is no second implementation to fall out of step with the first.
func nameTokens(name string) []string {
	toks := tokenizeCorpus(name)
	out := make([]string, len(toks))
	for i, t := range toks {
		out[i] = t.text
	}
	return out
}

// declarableName reports whether an api or component name may be declared, and
// returns the phrase corpus tokenization reduces it to.
//
// The rule is one sentence: a name is declarable iff tokenizing it the way the
// corpus is tokenized reproduces it exactly, in at least one and at most
// maxNameWords tokens. It is not a style rule — it is the sweep's reachability
// condition stated directly. Every phrase corpusScan.scanFile ever hashes is a
// join of corpusTokens with single spaces, so a name that is not itself such a
// join is a name no candidate phrase can ever equal: the node is unsweepable
// from the moment it is declared, and nothing says so.
//
// This is not a marginal shape. schema.go defines an api name as the exact
// surface string callers write, and `spex validate [--json]` is how an
// optional argument is written — tokenTrimCutset strips the brackets, so the
// declared name and the phrase the sweep builds differ and the removal can
// never be checked. `Validator (core)` is the same failure for the standard
// way of disambiguating a component name.
//
// Two properties follow from the same fact, and both used to be asserted
// rather than true:
//
//   - declarable and sweepable are one set. normalizeToken is idempotent and
//     tokens never contain whitespace, so every phrase the sweep builds is
//     itself a fixed point of tokenization — that is, declarable. The
//     converse is this predicate. Neither half can drift, because both are
//     tokenizeCorpus.
//   - two distinct declarable names cannot be confused. A declarable name IS
//     its own tokenization, so name → phrase is the identity on declarable
//     names and therefore injective. `spex validate [--json]` and
//     `spex validate --json` used to collapse to the same phrase and be
//     mutually mis-attributable; the first is now not declarable at all.
func declarableName(name string) (phrase string, ok bool) {
	toks := nameTokens(name)
	phrase = strings.Join(toks, " ")
	return phrase, len(toks) > 0 && len(toks) <= maxNameWords && phrase == name
}

// normalizeToken strips wrapping punctuation until the token stops changing,
// so `"OrphanDetector",` and **OrphanDetector** both reduce to the name.
// Trailing periods go too — a sentence-ending dot is not part of a name,
// while an interior one ("schema.IdentityHash") is — and so does a possessive
// suffix, which is how prose most often puts a component name in the middle
// of a sentence.
func normalizeToken(s string) string {
	for {
		t := strings.TrimRight(strings.Trim(s, tokenTrimCutset), ".")
		for _, poss := range []string{"'s", "’s"} {
			if len(t) > len(poss) {
				t = strings.TrimSuffix(t, poss)
			}
		}
		if t == s {
			return t
		}
		s = t
	}
}
