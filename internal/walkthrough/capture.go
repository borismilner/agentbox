package walkthrough

import "sort"

// What a spec points at, as a list rather than a walk. Creating a walkthrough
// and repairing one both need the same question answered - which file, which
// lines - and neither should have to know how steps and blocks are nested to
// ask it.

// Citation is one file-backed range a spec cites. Snippet blocks carry their
// own text and are not citations: there is no file behind them to go stale.
type Citation struct {
	Path string
	From int
	To   int
}

// Citations lists every file-backed range the spec cites, in a stable order and
// without duplicates. Two blocks that cite the same lines are captured once;
// two that overlap are kept apart, because each is what its own block asks for
// and the saving from merging them is not worth the arithmetic.
func (s *Spec) Citations() []Citation {
	seen := map[Citation]bool{}
	var out []Citation
	for i := range s.Steps {
		blocks := s.Steps[i].Blocks()
		for j := range blocks {
			b := &blocks[j]
			if b.Path == "" || b.Lines[0] <= 0 || b.Lines[1] < b.Lines[0] {
				continue
			}
			c := Citation{Path: b.Path, From: b.Lines[0], To: b.Lines[1]}
			if seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Path != out[b].Path {
			return out[a].Path < out[b].Path
		}
		return out[a].From < out[b].From
	})
	return out
}

// Paths is the set of files a spec cites, in first-cited order - what a caller
// needs when it reads whole files rather than ranges.
func (s *Spec) Paths() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range s.Citations() {
		if !seen[c.Path] {
			seen[c.Path] = true
			out = append(out, c.Path)
		}
	}
	return out
}
