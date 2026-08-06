package walkthrough

import (
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/borismilner/agentbox/internal/change"
)

// Coverage is the arithmetic behind the standard's rule 49 - "include one
// traversal that covers every changed line" - which until now was a rule nobody
// checked. The spec holds both halves at validation time: the change manifest
// (the diff) and every range the steps cite. Comparing them is the only way
// "complete" stops being a claim the author makes about their own work.
//
// FR61's principle applies to the answer as much as to the citations: silence
// must never read as covered, so Uncovered is always present and Computed says
// whether anything was actually compared.
type Coverage struct {
	Computed   bool
	Hunks      int // hunks the review is accountable for (deleted files excluded)
	Covered    int
	OutOfScope int // hunks in a file the spec excluded, with a reason
	Uncovered  []CoverageHunk
	// Deleted names files the diff removes. They are excluded from Hunks rather
	// than counted against the author: there is no new-side file left to cite, so
	// counting them as uncovered would only teach the habit of writing an
	// out_of_scope entry for every deletion.
	Deleted []string
}

// CoverageHunk names one hunk of the change by its new-file span.
type CoverageHunk struct {
	Path string
	From int
	To   int
	Kind string // add | del | mixed
}

// Cover compares the change manifest against what the steps cite. cites and
// scopes come from the same spec as diff, which is what makes this an
// observation rather than a second opinion.
func Cover(diff string, cites []Citation, scopes []Scope) Coverage {
	out := Coverage{Uncovered: []CoverageHunk{}}
	if strings.TrimSpace(diff) == "" {
		// No manifest, nothing compared. Reported uncomputed rather than clean:
		// a walkthrough with no diff is exactly the case where "0 uncovered"
		// would be the most misleading answer available.
		return out
	}
	out.Computed = true

	byPath := map[string][]Citation{}
	for _, c := range cites {
		p := normPath(c.Path)
		byPath[p] = append(byPath[p], c)
	}

	for _, f := range change.Parse(diff).Files {
		p := normPath(f.Path)
		if f.Badge == "deleted" {
			out.Deleted = append(out.Deleted, p)
			continue
		}
		for _, h := range f.Hunks {
			from, to := span(h)
			ref := CoverageHunk{Path: p, From: from, To: to, Kind: kindOf(h)}
			out.Hunks++
			switch {
			case excluded(p, from, to, scopes):
				out.OutOfScope++
			case cited(byPath[p], from, to):
				out.Covered++
			default:
				out.Uncovered = append(out.Uncovered, ref)
			}
		}
	}
	return out
}

// span is the hunk's new-file range. A hunk that only deletes has no new-side
// lines at all (`@@ -10,4 +9,0 @@`), and an empty range can never be overlapped
// by anything: it collapses to the seam the deletion left, so a citation of
// either side of it counts.
func span(h change.Hunk) (int, int) {
	if h.NewN <= 0 {
		at := max(h.NewFrom, 1)
		return at, at
	}
	return h.NewFrom, h.NewFrom + h.NewN - 1
}

func kindOf(h change.Hunk) string {
	add, del := false, false
	for _, l := range h.Lines {
		switch l.Kind {
		case '+':
			add = true
		case '-':
			del = true
		}
	}
	switch {
	case add && del:
		return "mixed"
	case del:
		return "del"
	default:
		return "add"
	}
}

// cited reports whether any citation for this file overlaps the hunk. Overlap
// rather than containment on purpose: a step that cites the twenty lines around
// a three-line change has stood on it, and demanding exact spans would make the
// arithmetic disagree with every honest review.
func cited(cites []Citation, from, to int) bool {
	for _, c := range cites {
		if c.From <= to && c.To >= from {
			return true
		}
	}
	return false
}

// excluded matches a hunk against the spec's stated exclusions. A Scope is
// either a glob over paths or one file (optionally narrowed to a line range),
// and both carry a reason - which is the whole difference between an exclusion
// and a hole.
func excluded(p string, from, to int, scopes []Scope) bool {
	for _, s := range scopes {
		switch {
		case s.Paths != "":
			if globMatch(normPath(s.Paths), p) {
				return true
			}
		case s.Path != "":
			if normPath(s.Path) != p {
				continue
			}
			if s.Lines[0] == 0 && s.Lines[1] == 0 {
				return true
			}
			if s.Lines[0] <= to && s.Lines[1] >= from {
				return true
			}
		}
	}
	return false
}

// globMatch is path.Match plus `**`, which path.Match does not have and which is
// the form every scope in the wild is written in (`docs/**`, `**/*_test.go`).
// A pattern with no `**` is left to path.Match, so the ordinary cases behave
// exactly as they read; the rest is translated to one anchored regexp, because
// walking literal segments by hand gets the mixed case (`**/*_test.go`) wrong -
// the segment after `**` is itself a glob.
func globMatch(pattern, p string) bool {
	if !strings.Contains(pattern, "**") {
		ok, err := path.Match(pattern, p)
		return err == nil && ok
	}
	re, err := globRe(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(p)
}

// globCache keeps the translated patterns: a spec is validated on every create
// and amend, and the same handful of scopes come round each time.
var (
	globMu    sync.Mutex
	globCache = map[string]*regexp.Regexp{}
)

func globRe(pattern string) (*regexp.Regexp, error) {
	globMu.Lock()
	defer globMu.Unlock()
	if re, ok := globCache[pattern]; ok {
		return re, nil
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			// Zero OR more directories, so `**/x.go` matches a top-level x.go as
			// well as a nested one. A `**/` that only ever meant "at least one
			// directory" is the classic surprise in this syntax.
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case pattern[i] == '*':
			b.WriteString("[^/]*")
		case pattern[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	globCache[pattern] = re
	return re, nil
}

// normPath makes a diff path and a citation path comparable: both are meant to
// be repo-relative, and the ways they arrive at that differ by a leading `./`
// or a stray separator often enough to be worth one line here rather than a bug
// report about a review that "covers nothing".
func normPath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return strings.TrimPrefix(p, "/")
}
