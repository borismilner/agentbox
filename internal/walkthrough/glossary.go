package walkthrough

// Glossary matching (FR68). One implementation, used twice: the validator
// warns about an entry no prose can reach, and the board renderer marks the
// occurrences a reader can open. If these two disagreed, the warning would
// be about a different document than the one shipped.

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TermIndex resolves the spellings of a glossary against running text.
type TermIndex struct {
	names []indexedName
}

// indexedName is one spelling and the entry it belongs to. Lower is only a
// display convenience: matching folds case as it compares, so an occurrence
// keeps the exact bytes the author wrote.
type indexedName struct {
	name string
	key  string
}

// Match is one occurrence: byte offsets into the text it was found in, plus
// the glossary key to open.
type Match struct {
	From, To int
	Key      string
}

// Run is a slice of a prose segment, marked when Key is set. Splitting in Go
// rather than shipping offsets keeps the surface out of the business of
// counting characters, where Go bytes and JavaScript code units disagree.
type Run struct {
	T   string `json:"t"`
	Key string `json:"g,omitempty"`
}

// NewTermIndex indexes every spelling of every term, longest first so a
// two-word term wins over one of its words ("technical impact" before
// "impact").
func NewTermIndex(terms []Term) *TermIndex {
	ix := &TermIndex{}
	for i := range terms {
		t := &terms[i]
		key := t.Key()
		if key == "" {
			continue
		}
		for _, sp := range append([]string{t.Term}, t.Also...) {
			if sp = strings.TrimSpace(sp); sp != "" {
				ix.names = append(ix.names, indexedName{name: sp, key: key})
			}
		}
	}
	sort.SliceStable(ix.names, func(a, b int) bool {
		return len(ix.names[a].name) > len(ix.names[b].name)
	})
	return ix
}

// Find reports every occurrence in text, left to right, non-overlapping.
func (ix *TermIndex) Find(text string) []Match {
	if ix == nil || len(ix.names) == 0 || text == "" {
		return nil
	}
	var out []Match
	for i := 0; i < len(text); {
		n, ok := ix.at(text, i)
		if !ok {
			_, sz := utf8.DecodeRuneInString(text[i:])
			i += sz
			continue
		}
		out = append(out, Match{From: i, To: i + len(n.name), Key: n.key})
		i += len(n.name)
	}
	return out
}

// at reports the longest spelling matching at byte offset i, on word
// boundaries. Boundaries are only required on the sides where the spelling's
// own edge is a word character, so a term like ".go" still matches.
func (ix *TermIndex) at(text string, i int) (indexedName, bool) {
	for _, n := range ix.names {
		end := i + len(n.name)
		if end > len(text) || !strings.EqualFold(text[i:end], n.name) {
			continue
		}
		if isWordEdge(n.name, true) {
			if r, _ := utf8.DecodeLastRuneInString(text[:i]); i > 0 && isWordRune(r) {
				continue
			}
		}
		if isWordEdge(n.name, false) {
			if r, _ := utf8.DecodeRuneInString(text[end:]); end < len(text) && isWordRune(r) {
				continue
			}
		}
		return n, true
	}
	return indexedName{}, false
}

func isWordEdge(s string, leading bool) bool {
	var r rune
	if leading {
		r, _ = utf8.DecodeRuneInString(s)
	} else {
		r, _ = utf8.DecodeLastRuneInString(s)
	}
	return isWordRune(r)
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// Split cuts text into runs, marking a term only the first time the reader
// meets it - seen carries that memory across the segments of one step. Every
// occurrence marked would turn a paragraph into a field of links, which is
// the distraction the glossary exists to avoid; never marking it again would
// mean a reader who joins mid-review has no way in. Once per step is the
// compromise the board is built on.
//
// Returns nil when nothing matched, so a segment with no terms travels as it
// always did.
func (ix *TermIndex) Split(text string, seen map[string]bool) []Run {
	hits := ix.Find(text)
	if len(hits) == 0 {
		return nil
	}
	var runs []Run
	at := 0
	for _, m := range hits {
		if seen[m.Key] {
			continue
		}
		seen[m.Key] = true
		if m.From > at {
			runs = append(runs, Run{T: text[at:m.From]})
		}
		runs = append(runs, Run{T: text[m.From:m.To], Key: m.Key})
		at = m.To
	}
	if len(runs) == 0 {
		return nil
	}
	if at < len(text) {
		runs = append(runs, Run{T: text[at:]})
	}
	return runs
}
