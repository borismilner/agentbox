// Package change parses unified diffs into a line-numbered model of a change.
//
// A walkthrough spec carries the branch diff as its change manifest, and
// everything diff-shaped is DERIVED from it here: which lines of a cited
// range were added, where deletions sat and what they said, and (later)
// which hunks no step covers. The spec itself never restates any of it -
// that is FR61's "nothing may hold a second copy of a citation" made
// structural. The parser mirrors the diff card's frontend parser
// (Card.svelte parseDiff): hunk line counts are consumed so a body line
// starting with "---" or "+++" is never misread as a file header.
package change

import (
	"regexp"
	"strings"
)

// Line is one body line of a hunk. Exactly one of Old/New is zero for an
// added or removed line; context lines carry both.
type Line struct {
	Kind byte   // '+', '-' or ' '
	Old  int    // old-file line number, 0 for '+'
	New  int    // new-file line number, 0 for '-'
	Text string // without the leading marker
}

// Hunk is one @@ block with its declared ranges.
type Hunk struct {
	OldFrom, OldN int
	NewFrom, NewN int
	Lines         []Line
}

// File is one file's change. Path is the new-side path with the a/ b/
// prefixes stripped, repo-relative.
type File struct {
	Path    string
	OldPath string // set when renamed
	Badge   string // "", "new", "deleted", "renamed"
	Hunks   []Hunk
}

// Set is a parsed diff.
type Set struct {
	Files []File
}

// Deletion is a run of removed lines located by where it sits in the NEW
// file: immediately after new-file line After (0 = before the first line),
// numbered from old-file line Old.
type Deletion struct {
	After int
	Old   int
	Lines []string
}

var hunkRe = regexp.MustCompile(`^@@+ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// strip removes the a/ or b/ prefix and a plain-diff "\tTIMESTAMP" suffix.
func strip(p string) string {
	p = strings.TrimPrefix(strings.TrimPrefix(p, "a/"), "b/")
	if i := strings.IndexByte(p, '\t'); i >= 0 {
		p = p[:i]
	}
	return p
}

// maxLine bounds what a hunk header may claim. The digits in `@@ -1,2 +3,4 @@`
// are unbounded and the text is agent-authored: twenty of them overflow int and
// come back NEGATIVE, and that negative then flows into every span, every slice
// and every loop built from the geometry. No file has two billion lines, so
// clamping is the honest answer - found by FuzzParse, which is the only way this
// was ever going to be found.
const maxLine = 1 << 31

// isBody reports whether a line can be part of a hunk's body at all. A unified
// diff gives every body line a marker: ' ', '+', '-', or the `\` of "no newline
// at end of file". An empty line is a context line whose trailing space was
// stripped somewhere between the generator and here, which happens often enough
// that treating it as structure would break ordinary patches.
func isBody(t string) bool {
	if t == "" {
		return true
	}
	switch t[0] {
	case ' ', '+', '-', '\\':
		return true
	}
	return false
}

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s { // the caller's regexp guarantees digits
		n = n*10 + int(c-'0')
		if n > maxLine {
			return maxLine
		}
	}
	return n
}

// Parse reads a unified diff (git or plain). Content it cannot account for
// line-by-line is dropped rather than guessed at: a malformed hunk yields no
// numbers, and no number beats a wrong one.
func Parse(raw string) Set {
	var s Set
	var cur *File
	var h *Hunk
	remOld, remNew := 0, 0
	oldLn, newLn := 0, 0
	open := func(name string) {
		s.Files = append(s.Files, File{Path: name})
		cur = &s.Files[len(s.Files)-1]
		h = nil
	}
	closeHunk := func() {
		if h != nil && cur != nil {
			cur.Hunks = append(cur.Hunks, *h)
			h = nil
		}
	}
	for t := range strings.SplitSeq(strings.TrimSuffix(raw, "\n"), "\n") {
		if (remOld > 0 || remNew > 0) && !isBody(t) {
			// The header claimed more body than it has. Consuming on trust here is
			// what lets one lying hunk swallow every file after it - and for the
			// coverage arithmetic that is worse than a garbled render, because the
			// hunks it ate are hunks nobody is told are uncovered. The header is a
			// claim; a line that cannot be a body line is the evidence against it.
			remOld, remNew = 0, 0
			closeHunk()
		}
		if remOld > 0 || remNew > 0 {
			switch {
			case strings.HasPrefix(t, "+"):
				remNew--
				newLn++
				h.Lines = append(h.Lines, Line{Kind: '+', New: newLn, Text: t[1:]})
			case strings.HasPrefix(t, "-"):
				remOld--
				oldLn++
				h.Lines = append(h.Lines, Line{Kind: '-', Old: oldLn, Text: t[1:]})
			case strings.HasPrefix(t, `\`):
				// "\ No newline at end of file" - no line on either side.
			default:
				remOld--
				remNew--
				oldLn++
				newLn++
				text := t
				if strings.HasPrefix(t, " ") {
					text = t[1:]
				}
				h.Lines = append(h.Lines, Line{Kind: ' ', Old: oldLn, New: newLn, Text: text})
			}
			if remOld <= 0 && remNew <= 0 {
				closeHunk()
			}
			continue
		}
		if m := hunkRe.FindStringSubmatch(t); m != nil {
			closeHunk()
			if cur == nil {
				open("")
			}
			h = &Hunk{
				OldFrom: atoi(m[1], 1), OldN: atoi(m[2], 1),
				NewFrom: atoi(m[3], 1), NewN: atoi(m[4], 1),
			}
			remOld, remNew = h.OldN, h.NewN
			oldLn, newLn = h.OldFrom-1, h.NewFrom-1
			continue
		}
		switch {
		case strings.HasPrefix(t, "diff --git "):
			closeHunk()
			name := ""
			if m := regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`).FindStringSubmatch(t); m != nil {
				name = m[2]
			}
			open(name)
		case strings.HasPrefix(t, "--- "):
			// Opens a file in plain `diff -u` output; inside a git section
			// (no hunks yet) it only refines the name.
			if cur == nil || len(cur.Hunks) > 0 || h != nil {
				closeHunk()
				open("")
			}
			if cur.Path == "" && t != "--- /dev/null" {
				cur.Path = strip(t[4:])
			}
		case strings.HasPrefix(t, "+++ "):
			if cur == nil {
				open("")
			}
			if t == "+++ /dev/null" {
				cur.Badge = "deleted"
			} else {
				cur.Path = strip(t[4:])
			}
		case strings.HasPrefix(t, "new file mode"):
			if cur != nil {
				cur.Badge = "new"
			}
		case strings.HasPrefix(t, "deleted file mode"):
			if cur != nil {
				cur.Badge = "deleted"
			}
		case strings.HasPrefix(t, "rename from "):
			if cur != nil {
				cur.OldPath = t[len("rename from "):]
			}
		case strings.HasPrefix(t, "rename to "):
			if cur != nil {
				cur.Badge = "renamed"
				cur.Path = t[len("rename to "):]
			}
		}
	}
	closeHunk()
	// Drop header-only artifacts of malformed input: a file with no path and
	// no hunks says nothing.
	kept := s.Files[:0]
	for _, f := range s.Files {
		if f.Path != "" || len(f.Hunks) > 0 {
			kept = append(kept, f)
		}
	}
	s.Files = kept
	return s
}

// File finds the change for a new-side path, or nil when the diff does not
// touch it.
func (s *Set) File(path string) *File {
	for i := range s.Files {
		if s.Files[i].Path == path {
			return &s.Files[i]
		}
	}
	return nil
}

// AddedIn lists the new-file lines within [from,to] that this change added.
func (f *File) AddedIn(from, to int) []int {
	if f == nil {
		return nil
	}
	var out []int
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Kind == '+' && l.New >= from && l.New <= to {
				out = append(out, l.New)
			}
		}
	}
	return out
}

// DeletionsIn lists the removed-line runs whose seam falls within [from,to]
// of the NEW file (After may be from-1: a deletion sitting just above the
// range's first line belongs to the range's story).
func (f *File) DeletionsIn(from, to int) []Deletion {
	if f == nil {
		return nil
	}
	var out []Deletion
	for _, h := range f.Hunks {
		after := h.NewFrom - 1
		var run *Deletion
		flush := func() {
			if run != nil && run.After >= from-1 && run.After <= to {
				out = append(out, *run)
			}
			run = nil
		}
		for _, l := range h.Lines {
			switch l.Kind {
			case '-':
				if run == nil {
					run = &Deletion{After: after, Old: l.Old}
				}
				run.Lines = append(run.Lines, l.Text)
			default:
				flush()
				after = l.New
			}
		}
		flush()
	}
	return out
}
