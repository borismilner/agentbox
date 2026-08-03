package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// Keeping the cited source, so a review outlives the tree it was written
// against.
//
// A walkthrough used to store the citation and nothing else, and the board read
// the file off disk on every render. That is true only while the working tree
// holds still. A checkout, a rename or a delete turns a step into an error; an
// edit that leaves the file long enough is worse, because the board then shows
// whatever now sits at those line numbers under the original prose and margin
// notes, and says nothing.
//
// So creation captures what it cites, from the working tree the authoring agent
// was looking at. Reviews stored before this existed are repaired from git
// instead: the blob at the pinned commit is the same text, as long as the clone
// is still there. Both write the same rows; source says which happened.

// captureFromWorktree reads every cited range out of the repository as it is
// now. It returns what it got and a line per citation it could not read - a
// miss is not fatal, because a review of work that is not on disk yet is still
// worth having, and the board falls back to reading the file exactly as it did
// before.
func captureFromWorktree(root string, cites []walkthrough.Citation) ([]store.Excerpt, []string) {
	return capture(cites, store.ExcerptWorktree, func(path string) ([]string, error) {
		return readWorktreeLines(root, path)
	})
}

// captureFromGit reads every cited range out of the pinned commit. This is the
// repair path: it is the only place the pinned SHA has ever been used for
// anything, and it works precisely as long as the clone still has the object.
func captureFromGit(root, sha string, cites []walkthrough.Citation) ([]store.Excerpt, []string) {
	if sha == "" {
		return nil, []string{"the walkthrough records no pinned commit, so there is nothing to read from git"}
	}
	files := map[string][]string{}
	return capture(cites, store.ExcerptGit, func(path string) ([]string, error) {
		if l, ok := files[path]; ok {
			return l, nil
		}
		l, err := gitBlobLines(root, sha, path)
		if err != nil {
			return nil, err
		}
		files[path] = l
		return l, nil
	})
}

// capture is the half both sources share: slice each citation out of whatever
// the reader hands back, and say which ones did not come out.
func capture(cites []walkthrough.Citation, source string, read func(string) ([]string, error)) ([]store.Excerpt, []string) {
	var out []store.Excerpt
	var missed []string
	cached := map[string][]string{}
	errs := map[string]error{}
	for _, c := range cites {
		lines, ok := cached[c.Path]
		if !ok {
			if err, bad := errs[c.Path]; bad {
				missed = append(missed, fmt.Sprintf("%s: %v", c.Path, err))
				continue
			}
			var err error
			lines, err = read(c.Path)
			if err != nil {
				errs[c.Path] = err
				missed = append(missed, fmt.Sprintf("%s: %v", c.Path, err))
				continue
			}
			cached[c.Path] = lines
		}
		if c.To > len(lines) {
			missed = append(missed, fmt.Sprintf("%s: lines %d-%d cited but the file has %d",
				c.Path, c.From, c.To, len(lines)))
			continue
		}
		out = append(out, store.Excerpt{
			Path:     c.Path,
			FromLine: c.From,
			ToLine:   c.To,
			Text:     strings.Join(lines[c.From-1:c.To], "\n"),
			Source:   source,
		})
	}
	return out, missed
}

// readWorktreeLines reads one file under the repository root. The jail is the
// same one the renderer has: the spec validator already refuses absolute paths
// and "..", and this does not rely on it.
func readWorktreeLines(root, rel string) ([]string, error) {
	clean := filepath.Clean(root)
	p := filepath.Join(clean, rel)
	if p != clean && !strings.HasPrefix(p, clean+string(os.PathSeparator)) {
		return nil, fmt.Errorf("%s escapes the repository root", rel)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return splitLines(string(raw)), nil
}

// gitBlobLines reads a file as it was at one commit. `git cat-file` rather than
// `git show`, because show applies pathspec magic and pagers; cat-file answers
// with the bytes and nothing else.
func gitBlobLines(root, sha, rel string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", root, "cat-file", "blob", sha+":"+rel)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git: %s", firstLine(msg))
	}
	return splitLines(string(out)), nil
}

// repairOne fills the gaps in one walkthrough's captured source from git, and
// leaves anything already captured alone: a range taken from the working tree
// at creation is what the authoring agent read, and git's copy of the same
// commit is at best equal to it.
func (d *Daemon) repairOne(w *store.Walkthrough) (proto.WalkthroughRepairRow, error) {
	row := proto.WalkthroughRepairRow{ID: w.ID, Title: w.Title}
	spec, _, err := walkthrough.Parse([]byte(w.Spec))
	if err != nil {
		return row, fmt.Errorf("cannot read the stored spec: %w", err)
	}
	cites := spec.Citations()
	if len(cites) == 0 {
		return row, nil
	}
	have, err := d.st.ExcerptsFor(w.ID)
	if err != nil {
		return row, err
	}
	held := map[walkthrough.Citation]bool{}
	for _, e := range have {
		held[walkthrough.Citation{Path: e.Path, From: e.FromLine, To: e.ToLine}] = true
	}
	var want []walkthrough.Citation
	for _, c := range cites {
		if held[c] {
			row.AlreadyOK++
			continue
		}
		want = append(want, c)
	}
	if len(want) == 0 {
		return row, nil
	}

	got, missed := captureFromGit(w.RepoRoot, w.PinnedSHA, want)
	row.Recovered, row.Missing, row.Notes = len(got), len(want)-len(got), missed
	if len(got) == 0 {
		return row, nil
	}
	// SaveExcerpts replaces the set, so the recovered rows go in beside what
	// was already there rather than instead of it.
	if err := d.st.SaveExcerpts(w.ID, append(have, got...)); err != nil {
		return row, err
	}
	return row, nil
}

func splitLines(s string) []string {
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func firstLine(s string) string {
	before, _, _ := strings.Cut(s, "\n")
	return before
}
