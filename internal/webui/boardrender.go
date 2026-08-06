package webui

// boardrender turns a stored walkthrough into the board's wire model. The
// contract that shapes everything here (FR58/FR61): the surface never
// learns a path it could read - Go reads the cited files, jailed to the
// spec's repo root - and the only HTML that crosses the wire is per-line
// chroma spans. Line numbers, gutters, add/del status, note badges and
// bind regions travel as structured fields the surface lays out, which is
// what keeps the three visual channels (syntax colour, diff status,
// reading emphasis) separate by construction: one channel cannot
// impersonate another when they never share an encoding.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/borismilner/agentbox/internal/change"
	"github.com/borismilner/agentbox/internal/store"
	"github.com/borismilner/agentbox/internal/walkthrough"
)

// wireBoard is everything the board surface needs, sent whole: cross-step
// find needs every step's text client-side, and the payload is local IPC.
type wireBoard struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Repo   string `json:"repo"` // display form of the root, ~-abbreviated
	Root   string `json:"root"` // absolute, for the copy control only
	Pinned string `json:"pinned"`
	State  string `json:"state"`
	Rev    int    `json:"rev"`
	Pos    int    `json:"pos"`
	// RevMS bumps on every snapshot so the surface can tell its own echo
	// from a real change (the wireDoc pattern).
	RevMS    int64      `json:"revMs"`
	Steps    []wireStep `json:"steps"`
	Glossary []wireTerm `json:"glossary,omitempty"`

	Marks    map[string]wireMark `json:"marks"`
	Comments []wireComment       `json:"comments"`
}

type wireStep struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
	// TLDR is the step laid out for glancing rather than reading: not the lossy
	// version, the same mastery in a shape a short attention span can hold. The
	// board opens in it, so for most readers this IS the step and the full text is
	// one key away. Walkthroughs stored before it existed have none, and the
	// surface says so rather than showing an empty pane.
	TLDR   *wireTLDR        `json:"tldr,omitempty"`
	Prose  []wireProse      `json:"prose"`
	AllNew bool             `json:"allNew,omitempty"`
	Codes  []wireCode       `json:"codes,omitempty"`
	Close  []wireProse      `json:"close,omitempty"`
	Binds  map[string][]int `json:"binds,omitempty"` // name -> [block, from, to]
	Checks []wireCheck      `json:"checks,omitempty"`
	Cmds   []wireCmd        `json:"cmds,omitempty"`
}

// wireTLDR is the glance version of a step: the sentence that must survive, and
// the facts that stand on their own beneath it.
type wireTLDR struct {
	Bottom string   `json:"bottom"`
	Points []string `json:"points,omitempty"`
}

func tldrOf(t *walkthrough.TLDR) *wireTLDR {
	if t == nil {
		return nil
	}
	return &wireTLDR{Bottom: t.Bottom, Points: t.Points}
}

// wireProse is one prose segment. T always carries the whole text - find and
// read-aloud read it - and Runs is the same text pre-cut at its glossary
// marks, present only on a segment that has one. Cutting in Go keeps the
// surface out of offset arithmetic, where Go's bytes and JavaScript's code
// units disagree the moment prose stops being ASCII.
type wireProse struct {
	T    string            `json:"t,omitempty"`
	Bind string            `json:"bind,omitempty"`
	Code string            `json:"code,omitempty"`
	P    bool              `json:"p,omitempty"`
	Runs []walkthrough.Run `json:"runs,omitempty"`
}

// wireTerm is one glossary entry, addressed by Key - what a marked run
// carries and what the drawer opens at.
type wireTerm struct {
	Key   string `json:"key"`
	Term  string `json:"term"`
	Short string `json:"short"`
	Body  string `json:"body,omitempty"`
}

type wireCode struct {
	Path  string `json:"path,omitempty"`  // repo-relative display; empty for snippets
	Label string `json:"label,omitempty"` // stands in for the path header on snippets
	// Lead is the handover paragraph above this block, with its glossary
	// marks already cut the way prose segments are.
	Lead     string            `json:"lead,omitempty"`
	LeadRuns []walkthrough.Run `json:"leadRuns,omitempty"`
	Lang     string            `json:"lang,omitempty"`
	Start    int               `json:"start"`
	New      bool              `json:"new,omitempty"` // every line in the range was added
	Lines    []wireLine        `json:"lines"`
	Dels     []wireDel         `json:"dels,omitempty"`
	Notes    []wireNote        `json:"notes,omitempty"`
	Err      string            `json:"err,omitempty"` // honest render failure, never a guess
	// Pinned means these lines came from the source captured when the review
	// was written, rather than from the file as it is now. The surface does
	// not paint it; it is here because "which of the two did the reader see"
	// is the first question when a note stops matching its code.
	Pinned bool `json:"pinned,omitempty"`
}

// wireLine is one rendered line: its number, its chroma spans, and whether
// this change added it. HTML is the one {@html} field on the board
// (frontend/policy_test.go names it).
type wireLine struct {
	N    int    `json:"n"`
	HTML string `json:"html"`
	Add  bool   `json:"add,omitempty"`
}

// wireDel is a removed run: shown after new-file line After, its lines
// numbered from the OLD file.
type wireDel struct {
	After int        `json:"after"`
	Lines []wireLine `json:"lines"`
}

type wireNote struct {
	Num  int    `json:"num"`
	From int    `json:"from"`
	To   int    `json:"to"`
	Side string `json:"side,omitempty"`
	Text string `json:"text"`
}

type wireCheck struct {
	Q string `json:"q"`
	A string `json:"a"`
}

type wireCmd struct {
	Cmd      string `json:"cmd"`
	Expect   string `json:"expect,omitempty"`
	Recorded string `json:"recorded,omitempty"`
}

type wireMark struct {
	Verdict  string `json:"verdict"`
	Note     string `json:"note,omitempty"`
	Revealed []int  `json:"revealed,omitempty"`
	Stale    bool   `json:"stale,omitempty"`
}

type wireComment struct {
	ID     string `json:"id"`
	StepID string `json:"stepId"`
	Path   string `json:"path,omitempty"`
	Side   string `json:"side,omitempty"`
	From   int    `json:"from,omitempty"`
	To     int    `json:"to,omitempty"`
	Exact  string `json:"exact,omitempty"`
	Body   string `json:"body"`
	AtMS   int64  `json:"atMs"`
}

// boardFmt emits bare token spans, one call per line; classes only, so the
// --k-code-* tokens colour the board exactly like every other agentbox surface.
var boardFmt = chromahtml.New(
	chromahtml.WithClasses(true),
	chromahtml.TabWidth(4),
	chromahtml.PreventSurroundingPre(true),
)

// renderSteps turns a spec (stored JSON) plus its diff manifest into wire
// steps. Render never fails whole: a block that cannot be read carries an
// honest Err and the walk goes on. renderMiss reports each such block for
// the log (path may be empty for snippet-shaped failures).
func renderSteps(specJSON, diff, root string, pinned []store.Excerpt, renderMiss func(step, path, reason string)) ([]wireStep, []wireTerm, error) {
	var spec walkthrough.Spec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, nil, fmt.Errorf("stored spec does not parse: %w", err)
	}
	manifest := change.Parse(diff)
	files := newFileCache(root, pinned)
	if renderMiss == nil {
		renderMiss = func(string, string, string) {}
	}
	terms := walkthrough.NewTermIndex(spec.Glossary)

	steps := make([]wireStep, 0, len(spec.Steps))
	for i := range spec.Steps {
		st := &spec.Steps[i]
		ws := wireStep{
			ID:      st.ID,
			Kind:    st.Kind,
			Title:   st.Title,
			Purpose: st.Purpose,
			TLDR:    tldrOf(st.TLDR),
		}
		// One memory per step: a term is marked the first time this step
		// says it, and stays plain text afterwards. The three text channels
		// are walked in the order the reader meets them - prose, each
		// block's lead, then the close - so "first" means first on the page.
		seen := map[string]bool{}
		markSegs := func(segs []walkthrough.Seg) []wireProse {
			out := make([]wireProse, 0, len(segs))
			for _, seg := range segs {
				wp := wireProse{T: seg.T, Bind: seg.Bind, Code: seg.Code, P: seg.P}
				// A bound phrase is already a control that lights code; a
				// second control inside it would be a button in a button,
				// and two different meanings under one underline.
				if seg.Bind == "" && seg.Code == "" {
					wp.Runs = terms.Split(seg.T, seen)
				}
				out = append(out, wp)
			}
			return out
		}
		ws.Prose = markSegs(st.Prose)
		for _, c := range st.Checks {
			ws.Checks = append(ws.Checks, wireCheck(c))
		}
		for _, c := range st.Cmds {
			ws.Cmds = append(ws.Cmds, wireCmd(c))
		}
		if len(st.Binds) > 0 {
			ws.Binds = make(map[string][]int, len(st.Binds))
			for name, b := range st.Binds {
				ws.Binds[name] = []int{b.Block, b.Lines[0], b.Lines[1]}
			}
		}
		noteNum := 0
		allNew := len(st.Code) > 0
		for bi := range st.Code {
			wc := renderBlock(&st.Code[bi], manifest, files, &noteNum)
			if wc.Err != "" {
				renderMiss(st.ID, st.Code[bi].Path, wc.Err)
			}
			if !wc.New {
				allNew = false
			}
			wc.LeadRuns = terms.Split(wc.Lead, seen)
			ws.Codes = append(ws.Codes, wc)
		}
		ws.AllNew = allNew
		ws.Close = markSegs(st.Close)
		steps = append(steps, ws)
	}
	glossary := make([]wireTerm, 0, len(spec.Glossary))
	for i := range spec.Glossary {
		t := &spec.Glossary[i]
		glossary = append(glossary, wireTerm{Key: t.Key(), Term: t.Term, Short: t.Short, Body: t.Body})
	}
	return steps, glossary, nil
}

func renderBlock(b *walkthrough.Block, manifest change.Set, files *fileCache, noteNum *int) wireCode {
	// Lines is never null on the wire: an errored block still has a shape
	// the surface can lay out.
	wc := wireCode{Lines: []wireLine{}, Lead: b.Lead}
	for _, nt := range b.Notes {
		*noteNum++
		wc.Notes = append(wc.Notes, wireNote{
			Num: *noteNum, From: nt.At[0], To: nt.At[1], Side: nt.Side, Text: nt.Text,
		})
	}
	if b.Path == "" {
		renderSnippet(b, &wc)
		return wc
	}

	wc.Path = b.Path
	wc.Start = b.Lines[0]
	from, to := b.Lines[0], b.Lines[1]
	excerpt, source, err := files.excerpt(b.Path, from, to)
	if err != nil {
		wc.Err = err.Error()
		return wc
	}
	wc.Pinned = source != ""
	lexer := lexerFor(b.Path, "", excerpt)
	wc.Lang = lexer.Config().Name

	f := manifest.File(b.Path)
	added := make(map[int]bool)
	for _, n := range f.AddedIn(from, to) {
		added[n] = true
	}
	html := highlightLines(excerpt, lexer)
	wc.New = len(html) > 0 && len(added) == len(html)
	for i, h := range html {
		wc.Lines = append(wc.Lines, wireLine{N: from + i, HTML: h, Add: added[from+i]})
	}
	for _, d := range f.DeletionsIn(from, to) {
		wd := wireDel{After: d.After}
		for j, h := range highlightLines(strings.Join(d.Lines, "\n"), lexer) {
			wd.Lines = append(wd.Lines, wireLine{N: d.Old + j, HTML: h})
		}
		wc.Dels = append(wc.Dels, wd)
	}
	return wc
}

func renderSnippet(b *walkthrough.Block, wc *wireCode) {
	sn := b.Snippet
	wc.Label = b.Label
	wc.Start = 1
	lexer := lexerFor("", sn.Lang, sn.Text)
	wc.Lang = lexer.Config().Name
	added := make(map[int]bool, len(sn.Added))
	for _, n := range sn.Added {
		added[n] = true
	}
	html := highlightLines(sn.Text, lexer)
	wc.New = len(html) > 0 && len(added) == len(html)
	for i, h := range html {
		wc.Lines = append(wc.Lines, wireLine{N: 1 + i, HTML: h, Add: added[1+i]})
	}
	for _, d := range sn.Del {
		wd := wireDel{After: d.After}
		for j, h := range highlightLines(strings.Join(d.Lines, "\n"), lexer) {
			wd.Lines = append(wd.Lines, wireLine{N: d.Old + j, HTML: h})
		}
		wc.Dels = append(wc.Dels, wd)
	}
}

// fileCache answers "what was on those lines". It prefers the source captured
// when the review was written and falls back to reading the working tree, which
// is what every walkthrough stored before capture existed still relies on.
//
// The captured copy wins whenever there is one, and that is the whole point: a
// review says what it was written against, not what the file happens to say
// today. Reading the tree instead would be right only until the next edit, and
// wrong silently after it.
type fileCache struct {
	root   string
	cache  map[string][]string
	pinned map[string]store.Excerpt // "path:from-to"
}

func newFileCache(root string, pinned []store.Excerpt) *fileCache {
	fc := &fileCache{
		root:   filepath.Clean(root),
		cache:  map[string][]string{},
		pinned: make(map[string]store.Excerpt, len(pinned)),
	}
	for _, e := range pinned {
		fc.pinned[excerptKey(e.Path, e.FromLine, e.ToLine)] = e
	}
	return fc
}

func excerptKey(path string, from, to int) string {
	return fmt.Sprintf("%s:%d-%d", path, from, to)
}

// excerpt returns the cited lines and where they came from. An empty source
// means the working tree, which is the answer that can be stale.
func (fc *fileCache) excerpt(path string, from, to int) (text, source string, err error) {
	if e, ok := fc.pinned[excerptKey(path, from, to)]; ok {
		return e.Text, e.Source, nil
	}
	lines, err := fc.lines(path)
	if err != nil {
		return "", "", err
	}
	if to > len(lines) {
		return "", "", fmt.Errorf("lines %d-%d cited, but %s has %d lines - the file has moved since the review was written",
			from, to, path, len(lines))
	}
	return strings.Join(lines[from-1:to], "\n"), "", nil
}

func (fc *fileCache) lines(rel string) ([]string, error) {
	if l, ok := fc.cache[rel]; ok {
		return l, nil
	}
	// The spec validator already refuses absolute paths and "..", but the
	// jail does not rely on that: the joined path must stay under the root.
	p := filepath.Join(fc.root, rel)
	if p != fc.root && !strings.HasPrefix(p, fc.root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("%s escapes the repository root", rel)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s at the pinned path: %v", rel, err)
	}
	l := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	fc.cache[rel] = l
	return l, nil
}

// lexerFor resolves a lexer by filename, declared language, or content, in
// that order - never nil.
func lexerFor(path, lang, src string) chroma.Lexer {
	var lexer chroma.Lexer
	if path != "" {
		lexer = lexers.Match(filepath.Base(path))
	}
	if lexer == nil && lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Analyse(src)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	return lexer
}

// highlightLines colours src and returns one HTML fragment per line:
// tokenised once so multi-line constructs (strings, block comments) keep
// their state, then split so the surface owns the line layout.
func highlightLines(src string, lexer chroma.Lexer) []string {
	iter, err := chroma.Coalesce(lexer).Tokenise(nil, src)
	if err != nil {
		return escapeLines(src)
	}
	lineTokens := chroma.SplitTokensIntoLines(iter.Tokens())
	out := make([]string, 0, len(lineTokens))
	for _, lt := range lineTokens {
		// Each split line carries its trailing newline inside the last
		// token; the surface renders rows, so the newline goes.
		if n := len(lt); n > 0 {
			lt[n-1].Value = strings.TrimSuffix(lt[n-1].Value, "\n")
		}
		var b strings.Builder
		if err := boardFmt.Format(&b, styles(), chroma.Literator(lt...)); err != nil {
			return escapeLines(src)
		}
		out = append(out, b.String())
	}
	// A trailing newline in src yields a phantom empty line after the
	// split; the excerpt join never adds one, but a snippet author might.
	if n := len(out); n > 0 && out[n-1] == "" && strings.HasSuffix(src, "\n") {
		out = out[:n-1]
	}
	return out
}

func escapeLines(src string) []string {
	raw := strings.Split(src, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = template(l)
	}
	return out
}

// displayRepo abbreviates the root for the header the way a shell would.
func displayRepo(root string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(root, home) {
		return "~" + strings.TrimPrefix(root, home)
	}
	return root
}

// boardSnapshot assembles the full wire model from stored state.
func boardSnapshot(w store.Walkthrough, marks []store.Mark, comments []store.Comment, revMS int64,
	renderMiss func(step, path, reason string)) (*wireBoard, error) {
	steps, glossary, err := renderSteps(w.Spec, w.Diff, w.RepoRoot, w.Excerpts, renderMiss)
	if err != nil {
		return nil, err
	}
	wb := &wireBoard{
		ID: w.ID, Title: w.Title, Repo: displayRepo(w.RepoRoot), Root: w.RepoRoot,
		Pinned: w.PinnedSHA, State: w.State, Rev: w.SpecRev, Pos: w.Pos, RevMS: revMS,
		Steps: steps, Glossary: glossary, Marks: map[string]wireMark{}, Comments: []wireComment{},
	}
	for _, m := range marks {
		wb.Marks[m.StepID] = wireMark{Verdict: m.Verdict, Note: m.Note, Revealed: m.Revealed, Stale: m.Stale}
	}
	for _, c := range comments {
		wb.Comments = append(wb.Comments, wireComment{
			ID: c.ID, StepID: c.StepID, Path: c.Path, Side: c.Side,
			From: c.FromLine, To: c.ToLine, Exact: c.Exact, Body: c.Body,
			AtMS: c.CreatedAt.UnixMilli(),
		})
	}
	return wb, nil
}
