// Package walkthrough defines the declarative review spec (FR58) and its
// validation. The agent supplies this structure; agentbox owns everything that
// happens to it afterwards - rendering, annotation, submission. Validation
// errors are teaching errors: they name the step and say what to do
// instead, because the caller is a model that reads the message and
// retries (vision principle 9).
package walkthrough

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Caps. Sized so a spec (≤1 MB) plus its diff (≤2 MB) clear the 4 MB
// wire-line limit in internal/proto with room to spare.
const (
	MaxSpecBytes = 1 << 20
	MaxDiffBytes = 2 << 20
	MaxSteps     = 64
	// MaxBlocks bounds one step's body. It was 8 when a block could only be code;
	// a body that also carries figures, tables and callouts needs the room for a
	// diagram beside the two citations it explains, and 14 is still a step a
	// reader finishes.
	MaxBlocks     = 14
	MaxNotes      = 20
	MaxChecks     = 6
	MaxCmds       = 6
	MaxProseSegs  = 64
	MaxScopes     = 32
	MaxBlockLines = 400
	MaxTerms      = 48
	MaxAliases    = 6
	// The TL;DR's shape. Bottom is one sentence and Points are up to six, each one
	// a fact that stands alone. The bound is on the SHAPE, not on the total: this
	// is not the lossy version of a step, it is the same mastery laid out to be
	// glanced at, and squeezing it would defeat the purpose it was asked for.
	MaxTLDRBottom = 220
	MaxTLDRPoint  = 280
	MaxTLDRPoints = 6
	// MaxDomains is the point past which grouping stops helping. A review with
	// eight subjects in it is two reviews, and a rail with eight collapsed groups
	// is the clutter the grouping was meant to remove.
	MaxDomains = 6
	// A table is for the handful of measurements that carry an argument, not for
	// data. Past these it is a spreadsheet, and a spreadsheet belongs in a file
	// the step cites.
	MaxTableCols  = 8
	MaxTableRows  = 40
	MaxCellChars  = 240
	MaxCaption    = 300
	MaxCalloutSeg = 24
)

// Spec is the version-1 walkthrough: what is being reviewed, the change
// manifest, and the ordered steps. The diff is the ONLY carrier of
// added/removed knowledge; blocks cite ranges and agentbox derives the rest
// (FR61: nothing holds a second copy of a citation).
type Spec struct {
	Version  int    `json:"version"`
	Title    string `json:"title"`
	RepoRoot string `json:"repo_root"`
	Pinned   string `json:"pinned"`
	Base     string `json:"base,omitempty"`
	Diff     string `json:"diff,omitempty"`
	// Domains group the steps into the two or three or five subjects a change
	// actually has, so a twenty-step review reads as "four things" rather than as
	// twenty. Optional: a short walk needs no grouping and gets none. Declared
	// here rather than derived from the steps so a domain can say what it is
	// about before its first step, and so the order is the author's rather than
	// an accident of which step came first.
	Domains    []Domain `json:"domains,omitempty"`
	OutOfScope []Scope  `json:"out_of_scope,omitempty"`
	Glossary   []Term   `json:"glossary,omitempty"`
	Steps      []Step   `json:"steps"`
}

// Domain is one subject inside a review: the group its steps belong to, and one
// line saying what the reader is about to be shown. Blurb is what the board puts
// up when the domain opens, which is the moment a reader decides whether to pay
// attention, so it is worth a sentence rather than a label.
type Domain struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Blurb string `json:"blurb,omitempty"`
}

// Term is one glossary entry: a word the reader may not know, defined once
// and out of the way (FR68). A definition inlined in prose costs every
// reader the interruption while helping only the ones who needed it, so the
// board marks the first occurrence per step and opens the entry on demand.
// Short is the whole definition for most readers; Body is there for the one
// who wants the rest.
type Term struct {
	Term  string   `json:"term"`
	Short string   `json:"short"`
	Body  string   `json:"body,omitempty"`
	Also  []string `json:"also,omitempty"` // other spellings that mean this term
}

// Key is what prose resolves against and the board addresses an entry by.
func (t *Term) Key() string { return strings.ToLower(strings.TrimSpace(t.Term)) }

// Scope marks hunks deliberately not walked, so "uncovered" means
// something. Paths is a glob over file paths; Path+Lines pins one range.
type Scope struct {
	Paths  string `json:"paths,omitempty"`
	Path   string `json:"path,omitempty"`
	Lines  [2]int `json:"lines,omitempty"`
	Reason string `json:"reason"`
}

// Step is one station of the walk. Only kind "code" counts toward the
// total - ground, none and check ride along (the finiteness promise).
// Close is the paragraph after the last block: the takeaway, which belongs
// under the code it is about rather than three blocks above it (FR69).
type Step struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // ground | code | none | check
	Title   string `json:"title"`
	Purpose string `json:"purpose,omitempty"`
	// Domain is the id of the group this step belongs to. Required once the spec
	// declares any domain at all: a review with three grouped steps and one loose
	// one reads as a bug in the board, not as a deliberate ungrouped step.
	Domain string `json:"domain,omitempty"`
	// TLDR is the step restructured for a reader with a very short attention span.
	// It is NOT the lossy version: it has to leave that reader with mastery of what
	// matters most here, which is why it is a shape rather than a string. Prose
	// asks to be read from the start; this asks to be glanced at, and a free-text
	// field would come back as the paragraph it exists to replace.
	//
	// Required on the steps that carry substance, because the board opens in it.
	TLDR  *TLDR `json:"tldr,omitempty"`
	Prose []Seg `json:"prose"`
	// Body is the step's ordered blocks: code citations, snippets, figures,
	// tables and callouts, laid out in the order they are written. Code is the
	// older name for the same array, from when a block could only be code, and it
	// is still read because every stored walkthrough uses it. Write body.
	Body   []Block         `json:"body,omitempty"`
	Code   []Block         `json:"code,omitempty"`
	Close  []Seg           `json:"close,omitempty"`
	Binds  map[string]Bind `json:"binds,omitempty"`
	Checks []Check         `json:"checks,omitempty"`
	Cmds   []Cmd           `json:"cmds,omitempty"`
}

// Blocks is the step's body in reading order. One field or the other holds it,
// never both (validation refuses that), so the read side never has to merge.
func (st *Step) Blocks() []Block {
	if len(st.Body) > 0 {
		return st.Body
	}
	return st.Code
}

// TLDR is one step laid out for glancing. Bottom is the single sentence that has
// to survive if nothing else does; Points are the load-bearing facts, each one
// standing on its own so they can be read in any order and stopped at any time.
//
// The caps are per point rather than over the whole, which is the difference
// between "be brief" and "be structured": six sharp points are the goal, one
// paragraph chopped into six pieces is not, and a point that runs past its cap is
// a paragraph wearing a bullet.
type TLDR struct {
	Bottom string   `json:"bottom"`
	Points []string `json:"points,omitempty"`
}

// Seg is one prose segment: plain text, a bound phrase (text that lights a
// code region), or an inline code chip. Exactly one form per segment.
//
// P starts a new paragraph AT this segment. It is a modifier, not a form, so
// it rides along with t or code rather than replacing them. Segments are
// inline by necessity - a bound phrase sits mid-sentence - so a paragraph
// cannot be a segment of its own, and without this flag every step rendered
// as one wall with sentences fused across the seam (FR63).
type Seg struct {
	T    string `json:"t,omitempty"`
	Bind string `json:"bind,omitempty"`
	Code string `json:"code,omitempty"`
	P    bool   `json:"p,omitempty"`
}

// Block is one code display: a citation into the repo (the normal case;
// content, diff status and deletions are read and derived at render time),
// or an inline snippet for content that lives in no file. Exactly one.
// Lead is the sentence or two directly above this block, saying what is
// about to be shown and why it comes now. Without it a step with two blocks
// renders as two walls of code with all its text stacked above the first
// (FR69), and the reader crosses the seam with nothing to hold.
type Block struct {
	Path    string   `json:"path,omitempty"`
	Lines   [2]int   `json:"lines,omitempty"`
	Snippet *Snippet `json:"snippet,omitempty"`
	Label   string   `json:"label,omitempty"`
	Lead    string   `json:"lead,omitempty"`
	Notes   []Note   `json:"notes,omitempty"`
	// The three blocks that are not code (FR101). A walkthrough used to be able
	// to describe a picture and nothing else, so the arguments that needed one -
	// a request path, a set of measurements, a warning that must not be read as
	// prose - were written somewhere else entirely and lost the board with them.
	// These are content, not presentation: each renders through the board's own
	// tokens, so the human keeps one theme across everything they open.
	Figure  *Figure  `json:"figure,omitempty"`
	Table   *Table   `json:"table,omitempty"`
	Callout *Callout `json:"callout,omitempty"`
}

// Figure is a drawing or an image. SVG is markup the author writes and agentbox
// re-composes (see svg.go); Src is an image, either repo-relative or a data: URI,
// read and re-encoded through the same budget every other image on the surface
// passes. Exactly one of the two.
//
// Wide lets a figure use the annotation margin as well as the reading column,
// which is what a wide flow diagram needs and what a portrait screenshot must
// not have.
type Figure struct {
	SVG     string `json:"svg,omitempty"`
	Src     string `json:"src,omitempty"`
	Alt     string `json:"alt,omitempty"`
	Caption string `json:"caption,omitempty"`
	Wide    bool   `json:"wide,omitempty"`
}

// Table is the shape prose cannot hold: a header row, body rows, and the column
// alignment that makes numbers comparable down a column. Cells are text - no
// markup, no nested blocks - because a table in a review is measurements and
// short labels, and anything longer is a paragraph pretending.
type Table struct {
	Head    []string   `json:"head"`
	Rows    [][]string `json:"rows"`
	Align   []string   `json:"align,omitempty"` // per column: left, right or center
	Caption string     `json:"caption,omitempty"`
}

// Callout is one thing that must not be read as part of the flow: the trap, the
// consequence, the thing that will be forgotten. Tone picks which of the board's
// semantic colours it wears, and the four are the only four - a callout that
// needs a fifth meaning is prose.
type Callout struct {
	Tone  string `json:"tone"` // note | good | warn | danger
	Title string `json:"title,omitempty"`
	Prose []Seg  `json:"prose"`
}

// Snippet is the no-file fallback. Because there is no manifest to derive
// from, it keeps the inline added/del vocabulary the mock used.
type Snippet struct {
	Lang  string       `json:"lang,omitempty"`
	Text  string       `json:"text"`
	Added []int        `json:"added,omitempty"`
	Del   []SnippetDel `json:"del,omitempty"`
}

// SnippetDel is a removed run inside a snippet: shown after snippet line
// After, numbered from old-file line Old.
type SnippetDel struct {
	After int      `json:"after"`
	Old   int      `json:"old"`
	Lines []string `json:"lines"`
}

// Note is a numbered annotation: the agent's why, anchored to a line range,
// popped or shown in the margin right where the code is.
type Note struct {
	At   [2]int `json:"at"`
	Side string `json:"side,omitempty"` // "" | new | old
	Text string `json:"text"`
}

// Bind names a code region so prose can point at it. Block indexes the
// step's code array.
type Bind struct {
	Block int    `json:"block,omitempty"`
	Lines [2]int `json:"lines"`
}

// Check is a comprehension question with a hidden answer.
type Check struct {
	Q string `json:"q"`
	A string `json:"a"`
}

// Cmd is something to run, with the expected result and when that
// expectation was last true.
type Cmd struct {
	Cmd      string `json:"cmd"`
	Expect   string `json:"expect,omitempty"`
	Recorded string `json:"recorded,omitempty"`
}

var (
	stepIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	pinnedRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	// Literal line numbers in prose go stale silently (FR61 counted
	// thirteen); prose points at code through binds. The patterns aim at
	// citations - file.go:12, "line 42" - not clock times ("14:02") or
	// ratios ("13.8:1"), hence the letter required after the dot.
	citationRe = regexp.MustCompile(`\.[a-zA-Z]\w{0,7}:\d|\blines? \d`)
	// A figure's src is a repo path or a data: URI. Anything with another scheme,
	// or a protocol-relative //host, is the network.
	reFigureScheme = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9+.\-]*:|//)`)
)

// Parse decodes and validates a raw spec. It returns the spec, non-blocking
// warnings, or a teaching error. Unknown fields are rejected - a typo'd
// field name silently dropped would be a lie the author never sees.
func Parse(raw []byte) (*Spec, []string, error) {
	if len(raw) > MaxSpecBytes+MaxDiffBytes {
		return nil, nil, fmt.Errorf("spec is %d bytes; the cap is %d for the spec plus %d for its diff", len(raw), MaxSpecBytes, MaxDiffBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return nil, nil, teachDecodeError(err)
	}
	warnings, err := s.validate()
	if err != nil {
		return nil, nil, err
	}
	return &s, warnings, nil
}

// Load decodes a spec that has already been through Parse - the stored copy -
// without validating it again. The read path needs the citations and the scopes,
// not a second opinion on a spec that was accepted when it was written: a rule
// added later would otherwise make an old walkthrough unreadable, which is the
// one thing the library is for.
func Load(raw []byte) (*Spec, error) {
	var s Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// teachDecodeError upgrades the JSON errors a model is most likely to hit
// into instructions.
func teachDecodeError(err error) error {
	msg := err.Error()
	for _, f := range []string{`"new"`, `"added"`, `"del"`} {
		if strings.Contains(msg, "unknown field") && strings.Contains(msg, f) {
			return fmt.Errorf("%s: a file-backed block never states diff status - agentbox derives added and removed lines from the spec's diff; put %s inside a snippet block only", msg, f)
		}
	}
	return fmt.Errorf("spec does not parse: %s", msg)
}

func (s *Spec) validate() ([]string, error) {
	var warnings []string
	if s.Version != 1 {
		return nil, fmt.Errorf("version must be 1, got %d", s.Version)
	}
	if s.Title == "" || len(s.Title) > 200 {
		return nil, fmt.Errorf("title is required, up to 200 characters")
	}
	if !filepath.IsAbs(s.RepoRoot) {
		return nil, fmt.Errorf("repo_root must be an absolute path, got %q", s.RepoRoot)
	}
	if !pinnedRe.MatchString(s.Pinned) {
		return nil, fmt.Errorf("pinned must be the commit SHA the citations are true against (7-64 hex), got %q", s.Pinned)
	}
	if len(s.Diff) > MaxDiffBytes {
		return nil, fmt.Errorf("diff is %d bytes; the cap is %d", len(s.Diff), MaxDiffBytes)
	}
	if len(s.OutOfScope) > MaxScopes {
		return nil, fmt.Errorf("out_of_scope has %d entries; the cap is %d", len(s.OutOfScope), MaxScopes)
	}
	for i, sc := range s.OutOfScope {
		if sc.Reason == "" {
			return nil, fmt.Errorf("out_of_scope[%d]: reason is required - an unexplained exclusion is a hole, not a scope", i)
		}
		if (sc.Paths == "") == (sc.Path == "") {
			return nil, fmt.Errorf("out_of_scope[%d]: exactly one of paths (a glob) or path (one file) is required", i)
		}
	}
	if err := s.validateGlossary(); err != nil {
		return nil, err
	}
	if err := s.validateDomains(); err != nil {
		return nil, err
	}
	if len(s.Steps) == 0 || len(s.Steps) > MaxSteps {
		return nil, fmt.Errorf("steps must hold 1-%d entries, got %d", MaxSteps, len(s.Steps))
	}
	seen := make(map[string]bool, len(s.Steps))
	hasCode := false
	for i := range s.Steps {
		st := &s.Steps[i]
		if err := st.validate(); err != nil {
			return nil, err
		}
		if seen[st.ID] {
			return nil, fmt.Errorf("step id %q appears twice; ids are the identity annotations attach to and must be unique", st.ID)
		}
		seen[st.ID] = true
		if st.Kind == "code" {
			hasCode = true
		}
	}
	if s.Diff != "" && hasCode && s.Steps[len(s.Steps)-1].Kind != "check" {
		warnings = append(warnings, "the last step is not a check: finishing should be an observation, not a feeling - end with the commands that close the review (FR61)")
	}
	warnings = append(warnings, s.glossaryWarnings()...)
	return warnings, nil
}

// validateDomains keeps the grouping total. Domains are optional, but a spec
// that declares them and then leaves a step out of them renders as a review with
// a hole in its rail, which reads as a defect in the board rather than as a
// choice - so the rule is all or nothing.
func (s *Spec) validateDomains() error {
	if len(s.Domains) == 0 {
		for i := range s.Steps {
			if s.Steps[i].Domain != "" {
				return fmt.Errorf("step %q names domain %q, but the spec declares no domains - add a domains list with that id, or drop the field",
					s.Steps[i].ID, s.Steps[i].Domain)
			}
		}
		return nil
	}
	if len(s.Domains) > MaxDomains {
		return fmt.Errorf("%d domains; the cap is %d - past that the grouping is the clutter it was meant to remove, and a review with that many subjects is more than one review",
			len(s.Domains), MaxDomains)
	}
	known := make(map[string]bool, len(s.Domains))
	used := make(map[string]bool, len(s.Domains))
	for i := range s.Domains {
		d := &s.Domains[i]
		if !stepIDRe.MatchString(d.ID) {
			return fmt.Errorf("domain id %q must match ^[a-z0-9][a-z0-9_-]{0,31}$", d.ID)
		}
		if known[d.ID] {
			return fmt.Errorf("domain id %q appears twice", d.ID)
		}
		known[d.ID] = true
		if strings.TrimSpace(d.Title) == "" || len(d.Title) > 80 {
			return fmt.Errorf("domain %q needs a title, up to 80 characters", d.ID)
		}
		if len(d.Blurb) > 300 {
			return fmt.Errorf("domain %q: blurb is over 300 characters - it is the line the reader gets when the domain opens, not its summary", d.ID)
		}
	}
	for i := range s.Steps {
		st := &s.Steps[i]
		if st.Domain == "" {
			return fmt.Errorf("step %q has no domain, but this spec groups its steps - every step needs one, or the rail shows a review with a hole in it. Domains declared: %s",
				st.ID, strings.Join(domainIDs(s.Domains), ", "))
		}
		if !known[st.Domain] {
			return fmt.Errorf("step %q names domain %q, which is not declared. Domains declared: %s",
				st.ID, st.Domain, strings.Join(domainIDs(s.Domains), ", "))
		}
		used[st.Domain] = true
	}
	for _, d := range s.Domains {
		if !used[d.ID] {
			return fmt.Errorf("domain %q has no steps in it - an empty group is a heading the reader opens onto nothing", d.ID)
		}
	}
	// Contiguity is the whole point: the board shows one domain at a time, so a
	// domain the step order leaves and comes back to would open twice and finish
	// neither time.
	seen := map[string]bool{}
	last := ""
	for i := range s.Steps {
		d := s.Steps[i].Domain
		if d == last {
			continue
		}
		if seen[d] {
			return fmt.Errorf("step %q returns to domain %q after leaving it - the board walks one domain at a time, so a domain's steps must be consecutive. Reorder the steps, or split it into two domains",
				s.Steps[i].ID, d)
		}
		seen[d] = true
		last = d
	}
	return nil
}

func domainIDs(ds []Domain) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func (s *Spec) validateGlossary() error {
	if len(s.Glossary) > MaxTerms {
		return fmt.Errorf("glossary has %d terms; the cap is %d - define what a reader of this change cannot look up in one guess, not every noun", len(s.Glossary), MaxTerms)
	}
	claimed := make(map[string]string, len(s.Glossary)) // spelling -> owning term
	for i := range s.Glossary {
		t := &s.Glossary[i]
		if t.Key() == "" || len(t.Term) > 60 {
			return fmt.Errorf("glossary[%d]: term is required, up to 60 characters", i)
		}
		if t.Short == "" || len(t.Short) > 240 {
			return fmt.Errorf("glossary[%d] (%q): short is required, up to 240 characters - one sentence a reader can take in without leaving the step", i, t.Term)
		}
		if len(t.Body) > 4000 {
			return fmt.Errorf("glossary[%d] (%q): body is over 4000 characters", i, t.Term)
		}
		if len(t.Also) > MaxAliases {
			return fmt.Errorf("glossary[%d] (%q): %d aliases; the cap is %d", i, t.Term, len(t.Also), MaxAliases)
		}
		for _, sp := range append([]string{t.Term}, t.Also...) {
			key := strings.ToLower(strings.TrimSpace(sp))
			if key == "" || len(sp) > 60 {
				return fmt.Errorf("glossary[%d] (%q): every alias must be 1-60 characters", i, t.Term)
			}
			if owner, dup := claimed[key]; dup {
				return fmt.Errorf("glossary[%d]: %q is already claimed by %q; one spelling cannot resolve to two entries", i, sp, owner)
			}
			claimed[key] = t.Term
		}
	}
	return nil
}

// glossaryWarnings reports entries no prose will ever mark. An unreachable
// definition is not harmless: it is effort the reader never sees, and it
// usually means the term is spelled differently in the text than in the
// entry (add the spelling to also).
func (s *Spec) glossaryWarnings() []string {
	if len(s.Glossary) == 0 {
		return nil
	}
	idx := NewTermIndex(s.Glossary)
	hit := make(map[string]bool, len(s.Glossary))
	mark := func(text string) {
		for _, m := range idx.Find(text) {
			hit[m.Key] = true
		}
	}
	// Every channel the renderer marks, and only those: a term that appears
	// solely in a note or an inline code chip is unreachable, and saying so
	// is the point of this warning.
	for i := range s.Steps {
		st := &s.Steps[i]
		for _, segs := range [][]Seg{st.Prose, st.Close} {
			for _, seg := range segs {
				if seg.Bind == "" {
					mark(seg.T)
				}
			}
		}
		blocks := st.Blocks()
		for j := range blocks {
			mark(blocks[j].Lead)
			if c := blocks[j].Callout; c != nil {
				for _, seg := range c.Prose {
					if seg.Bind == "" {
						mark(seg.T)
					}
				}
			}
		}
	}
	var out []string
	for i := range s.Glossary {
		t := &s.Glossary[i]
		if !hit[t.Key()] {
			out = append(out, fmt.Sprintf("glossary term %q never appears in any prose, so no reader can reach it - spell it in the text the way the entry spells it, or list that spelling in also", t.Term))
		}
	}
	return out
}

func (st *Step) validate() error {
	if !stepIDRe.MatchString(st.ID) {
		return fmt.Errorf("step id %q must match ^[a-z0-9][a-z0-9_-]{0,31}$", st.ID)
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("step %q: %s", st.ID, fmt.Sprintf(format, args...))
	}
	switch st.Kind {
	case "ground", "code", "none", "check":
	default:
		return fail("kind %q is not one of ground, code, none, check", st.Kind)
	}
	if st.Title == "" || len(st.Title) > 120 {
		return fail("title is required, up to 120 characters")
	}
	if (st.Kind == "code" || st.Kind == "check") && st.Purpose == "" {
		return fail("purpose is required on %s steps - which requirement does this serve, decided where?", st.Kind)
	}
	if len(st.Purpose) > 500 {
		return fail("purpose is over 500 characters")
	}
	// Required on the steps that carry the substance, because the board opens in
	// TL;DR: a step without one shows the reader nothing until they switch, which
	// is the opposite of what the mode is for. Ground and none may skip it - they
	// are already short - but may have one.
	if err := st.validateTLDR(fail); err != nil {
		return err
	}
	if len(st.Prose) == 0 || len(st.Prose) > MaxProseSegs {
		return fail("prose must hold 1-%d segments", MaxProseSegs)
	}
	if len(st.Body) > 0 && len(st.Code) > 0 {
		return fail("the step has both body and code; they are the same array under two names - put every block in body, in the order it should be read")
	}
	blocks := st.Blocks()
	if len(blocks) > MaxBlocks {
		return fail("%d blocks in the body; the cap is %d", len(blocks), MaxBlocks)
	}
	// Code is what the review's promise counts, so a code step has to show some.
	// A figure or a table on such a step is welcome beside it and is not a
	// substitute: a step that only draws a picture of code is a ground step.
	cited := 0
	for i := range blocks {
		if blocks[i].isCode() {
			cited++
		}
	}
	if st.Kind == "code" && cited == 0 {
		return fail("a code step needs at least one block citing code - a path with lines, or a snippet. A step whose body is only figures, tables or callouts is a ground step")
	}
	if st.Kind != "code" && cited > 0 {
		return fail("code citations belong on code steps only, not %q; figures, tables and callouts may sit on any step", st.Kind)
	}
	checkSegs := func(field string, segs []Seg) error {
		for i, seg := range segs {
			hasT, hasCode := seg.T != "", seg.Code != ""
			if hasT == hasCode {
				return fail("%s[%d] must carry exactly one of t or code", field, i)
			}
			if seg.Bind != "" && !hasT {
				return fail("%s[%d] binds %q but has no text to show", field, i, seg.Bind)
			}
			if len(seg.T) > 4000 || len(seg.Code) > 4000 {
				return fail("%s[%d] is over 4000 characters", field, i)
			}
			if seg.Bind != "" {
				if _, ok := st.Binds[seg.Bind]; !ok {
					return fail("%s[%d] binds %q but binds has no such entry", field, i, seg.Bind)
				}
			}
			if seg.T != "" && citationRe.MatchString(seg.T) {
				return fail("%s[%d] contains a literal line reference (%q); line numbers in prose go stale silently - bind the phrase to the code region instead", field, i, citationRe.FindString(seg.T))
			}
		}
		return nil
	}
	if err := checkSegs("prose", st.Prose); err != nil {
		return err
	}
	if len(st.Close) > MaxProseSegs {
		return fail("close must hold at most %d segments", MaxProseSegs)
	}
	if len(st.Close) > 0 && len(blocks) == 0 {
		return fail("close is the paragraph under the last block; a step with an empty body has nothing to close - put it in prose")
	}
	if err := checkSegs("close", st.Close); err != nil {
		return err
	}
	field := "body"
	if len(st.Body) == 0 {
		field = "code"
	}
	for i := range blocks {
		if err := blocks[i].validate(); err != nil {
			return fail("%s[%d]: %s", field, i, err)
		}
		if c := blocks[i].Callout; c != nil {
			if err := checkSegs(fmt.Sprintf("%s[%d].callout.prose", field, i), c.Prose); err != nil {
				return err
			}
		}
	}
	for name, b := range st.Binds {
		if b.Block < 0 || b.Block >= len(blocks) {
			return fail("bind %q names block %d; the step has %d", name, b.Block, len(blocks))
		}
		if !blocks[b.Block].isCode() {
			return fail("bind %q names block %d, which is a %s; a bound phrase lights a region of code, so it can only point at a citation or a snippet", name, b.Block, blocks[b.Block].kindName())
		}
		lo, hi := blocks[b.Block].lineBounds()
		if b.Lines[0] < lo || b.Lines[1] > hi || b.Lines[0] > b.Lines[1] {
			return fail("bind %q lines [%d,%d] fall outside block %d's range [%d,%d]", name, b.Lines[0], b.Lines[1], b.Block, lo, hi)
		}
	}
	if len(st.Checks) > MaxChecks {
		return fail("%d checks; the cap is %d", len(st.Checks), MaxChecks)
	}
	for i, c := range st.Checks {
		if c.Q == "" || c.A == "" || len(c.Q) > 2000 || len(c.A) > 2000 {
			return fail("checks[%d] needs a q and an a, each up to 2000 characters", i)
		}
	}
	if len(st.Cmds) > MaxCmds {
		return fail("%d cmds; the cap is %d", len(st.Cmds), MaxCmds)
	}
	for i, c := range st.Cmds {
		if c.Cmd == "" {
			return fail("cmds[%d] has no command", i)
		}
	}
	return nil
}

// validateTLDR teaches the shape rather than just refusing it: the caller is a
// model that reads the message and retries (vision principle 9), and "tldr is
// required" without saying what a good one is produces a summary of the summary.
func (st *Step) validateTLDR(fail func(string, ...any) error) error {
	needs := st.Kind == "code" || st.Kind == "check"
	if st.TLDR == nil {
		if needs {
			return fail("tldr is required on %s steps. It is NOT the shortened version - it is this step restructured for a reader with a very short attention span who must still come away with mastery of what matters most. Give it as {\"bottom\": the one sentence that has to survive, \"points\": [up to %d facts, each standing on its own]}. The board opens in it, so for most readers this IS the step", st.Kind, MaxTLDRPoints)
		}
		return nil
	}
	bottom := strings.TrimSpace(st.TLDR.Bottom)
	if bottom == "" {
		return fail("tldr.bottom is required - the one sentence that has to survive if the reader reads nothing else on this step")
	}
	if len(bottom) > MaxTLDRBottom {
		return fail("tldr.bottom is %d characters; the cap is %d. It is one sentence; everything that does not fit is a point", len(bottom), MaxTLDRBottom)
	}
	if len(st.TLDR.Points) > MaxTLDRPoints {
		return fail("tldr has %d points; the cap is %d - past that it is the step again in a different shape, and the reader it was written for has already stopped", len(st.TLDR.Points), MaxTLDRPoints)
	}
	for i, p := range st.TLDR.Points {
		if strings.TrimSpace(p) == "" {
			return fail("tldr.points[%d] is empty", i)
		}
		if len(p) > MaxTLDRPoint {
			return fail("tldr.points[%d] is %d characters; the cap is %d. A point that runs past it is a paragraph wearing a bullet - split it, or move it to the prose", i, len(p), MaxTLDRPoint)
		}
	}
	if needs && len(st.TLDR.Points) == 0 {
		return fail("tldr on a %s step needs at least one point beside its bottom line: a step with substance in it has more than one thing worth mastering, and if it truly does not, it is a ground step", st.Kind)
	}
	return nil
}

// isCode says whether this block cites or carries code, which is what binds,
// notes and the coverage arithmetic all address.
func (b *Block) isCode() bool { return b.Path != "" || b.Snippet != nil }

// kindName names the form for an error message.
func (b *Block) kindName() string {
	switch {
	case b.Path != "":
		return "citation"
	case b.Snippet != nil:
		return "snippet"
	case b.Figure != nil:
		return "figure"
	case b.Table != nil:
		return "table"
	case b.Callout != nil:
		return "callout"
	}
	return "empty block"
}

func (b *Block) validate() error {
	forms := 0
	for _, present := range []bool{b.Path != "", b.Snippet != nil, b.Figure != nil, b.Table != nil, b.Callout != nil} {
		if present {
			forms++
		}
	}
	if forms != 1 {
		return fmt.Errorf("a block is exactly one of path (a citation agentbox reads at render time), snippet (code that lives in no file), figure (a drawing or an image), table or callout; this one is %d of them", forms)
	}
	if !b.isCode() {
		if len(b.Notes) > 0 {
			return fmt.Errorf("a %s takes no notes - notes are margin annotations anchored to code lines. Put the words in its caption, its title, or the lead above it", b.kindName())
		}
		if b.Lines != [2]int{} {
			return fmt.Errorf("lines belong to a citation, not to a %s", b.kindName())
		}
		if err := b.validateLead(); err != nil {
			return err
		}
		switch {
		case b.Figure != nil:
			return b.Figure.validate()
		case b.Table != nil:
			return b.Table.validate()
		default:
			return b.Callout.validate()
		}
	}
	file := b.Path != ""
	if len(b.Notes) > MaxNotes {
		return fmt.Errorf("%d notes; the cap is %d", len(b.Notes), MaxNotes)
	}
	if err := b.validateLead(); err != nil {
		return err
	}
	if file {
		if filepath.IsAbs(b.Path) || strings.Contains(b.Path, "..") {
			return fmt.Errorf("path %q must be repo-relative with no ..; the repo_root is the jail", b.Path)
		}
		if b.Lines[0] < 1 || b.Lines[1] < b.Lines[0] {
			return fmt.Errorf("lines [%d,%d] must be a 1-based inclusive range", b.Lines[0], b.Lines[1])
		}
		if n := b.Lines[1] - b.Lines[0] + 1; n > MaxBlockLines {
			return fmt.Errorf("range spans %d lines; the cap is %d - cite the region worth reading, not the file", n, MaxBlockLines)
		}
	} else {
		if b.Lines != [2]int{} {
			return fmt.Errorf("lines belongs to citations; a snippet is addressed by its own line count")
		}
		n := strings.Count(b.Snippet.Text, "\n") + 1
		if b.Snippet.Text == "" || n > MaxBlockLines {
			return fmt.Errorf("snippet text must hold 1-%d lines", MaxBlockLines)
		}
		for _, a := range b.Snippet.Added {
			if a < 1 || a > n {
				return fmt.Errorf("snippet added line %d is outside 1-%d", a, n)
			}
		}
		for i, d := range b.Snippet.Del {
			if d.After < 0 || d.After > n || len(d.Lines) == 0 {
				return fmt.Errorf("snippet del[%d] must sit after a line in 0-%d and carry lines", i, n)
			}
		}
	}
	lo, hi := b.lineBounds()
	for i, nt := range b.Notes {
		if nt.Text == "" || len(nt.Text) > 4000 {
			return fmt.Errorf("notes[%d] needs text up to 4000 characters", i)
		}
		switch nt.Side {
		case "", "new":
			if nt.At[0] < lo || nt.At[1] > hi || nt.At[0] > nt.At[1] {
				return fmt.Errorf("notes[%d] at [%d,%d] falls outside the block's range [%d,%d]", i, nt.At[0], nt.At[1], lo, hi)
			}
		case "old":
			if nt.At[0] < 1 || nt.At[0] > nt.At[1] {
				return fmt.Errorf("notes[%d] old-side range [%d,%d] must be 1-based and ordered", i, nt.At[0], nt.At[1])
			}
		default:
			return fmt.Errorf("notes[%d] side %q is not one of new, old", i, nt.Side)
		}
	}
	return nil
}

// lineBounds is the addressable new-side range of a block: the citation's
// range for a file, 1..N for a snippet.
func (b *Block) lineBounds() (int, int) {
	if b.Path != "" {
		return b.Lines[0], b.Lines[1]
	}
	return 1, strings.Count(b.Snippet.Text, "\n") + 1
}

func (b *Block) validateLead() error {
	if len(b.Lead) > 600 {
		return fmt.Errorf("lead is over 600 characters - it is the sentence or two that hands the reader into this block, not the explanation itself; that goes in notes")
	}
	if citationRe.MatchString(b.Lead) {
		return fmt.Errorf("lead contains a literal line reference (%q); line numbers go stale silently - say what the block is, and let the notes point at lines", citationRe.FindString(b.Lead))
	}
	return nil
}

func (f *Figure) validate() error {
	svg, src := strings.TrimSpace(f.SVG) != "", strings.TrimSpace(f.Src) != ""
	if svg == src {
		return fmt.Errorf("a figure is exactly one of svg (markup agentbox re-composes, for a drawing) or src (an image: a repo-relative path, or a data: URI)")
	}
	if len(f.Caption) > MaxCaption {
		return fmt.Errorf("caption is over %d characters - it says what the picture shows, and the argument goes in the prose around it", MaxCaption)
	}
	if len(f.Alt) > 300 {
		return fmt.Errorf("alt is over 300 characters")
	}
	if svg {
		if _, err := SafeSVG(f.SVG); err != nil {
			return fmt.Errorf("svg: %w", err)
		}
		return nil
	}
	return validateFigureSrc(f.Src)
}

// validateFigureSrc refuses at authoring time what the renderer would refuse at
// render time, so the author is told now rather than shipping a figure that
// arrives on the board as a grey box. The repo root is the jail: the board reads
// the file, the surface never learns a path (FR58).
func validateFigureSrc(src string) error {
	src = strings.TrimSpace(src)
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		if !strings.Contains(strings.ToLower(src[:min(64, len(src))]), "base64") {
			return fmt.Errorf("a data: URI must be base64 - percent-encoded pixels are not something to write by hand")
		}
		return nil
	}
	if reFigureScheme.MatchString(src) {
		return fmt.Errorf("src %q is a URL; the board loads nothing over the network. Cite the image by its repo-relative path, or inline it as a base64 data: URI", src)
	}
	if filepath.IsAbs(src) || strings.Contains(src, "..") {
		return fmt.Errorf("src %q must be repo-relative with no ..; the repo_root is the jail", src)
	}
	if src == "" {
		return fmt.Errorf("src is empty")
	}
	return nil
}

func (t *Table) validate() error {
	if len(t.Head) == 0 {
		return fmt.Errorf("a table needs a head: the column labels, which is what makes its rows readable in any order")
	}
	if len(t.Head) > MaxTableCols {
		return fmt.Errorf("%d columns; the cap is %d - past that the table is wider than the reading column and the reader scrolls sideways to compare", len(t.Head), MaxTableCols)
	}
	if len(t.Rows) == 0 {
		return fmt.Errorf("a table needs rows")
	}
	if len(t.Rows) > MaxTableRows {
		return fmt.Errorf("%d rows; the cap is %d - a table this long is data, and data belongs in a file the step cites", len(t.Rows), MaxTableRows)
	}
	if len(t.Align) > 0 && len(t.Align) != len(t.Head) {
		return fmt.Errorf("align has %d entries for %d columns; give one per column or leave it out", len(t.Align), len(t.Head))
	}
	for i, a := range t.Align {
		switch a {
		case "", "left", "right", "center":
		default:
			return fmt.Errorf("align[%d] is %q; it is left, right or center", i, a)
		}
	}
	if len(t.Caption) > MaxCaption {
		return fmt.Errorf("caption is over %d characters", MaxCaption)
	}
	for _, h := range t.Head {
		if len(h) > MaxCellChars {
			return fmt.Errorf("a head cell is over %d characters", MaxCellChars)
		}
	}
	for i, row := range t.Rows {
		if len(row) != len(t.Head) {
			return fmt.Errorf("row %d has %d cells for %d columns; a ragged table renders as one with a hole in it - pass an empty string for a cell with nothing in it", i, len(row), len(t.Head))
		}
		for j, c := range row {
			if len(c) > MaxCellChars {
				return fmt.Errorf("row %d column %d is %d characters; the cap is %d. A cell that long is a paragraph, and it belongs in the prose or a note", i, j, len(c), MaxCellChars)
			}
		}
	}
	return nil
}

func (c *Callout) validate() error {
	switch c.Tone {
	case "note", "good", "warn", "danger":
	default:
		return fmt.Errorf("tone %q is not one of note, good, warn, danger - the four are the board's semantic colours, and a meaning that needs a fifth is prose", c.Tone)
	}
	if len(c.Title) > 120 {
		return fmt.Errorf("title is over 120 characters - it is the one line the reader takes away from the callout")
	}
	if len(c.Prose) == 0 || len(c.Prose) > MaxCalloutSeg {
		return fmt.Errorf("callout prose must hold 1-%d segments; a callout longer than that is the step, not an aside in it", MaxCalloutSeg)
	}
	return nil
}

// StepHash is the canonical fingerprint of a spec step, recorded on the
// human's marks so an amendment that changes a step under them is
// detectable (the mark goes stale, never silently wrong).
func StepHash(st *Step) string {
	b, err := json.Marshal(st)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// CountedSteps counts the stations that make up the review's promise:
// "n of SIX understood" counts code steps only.
func (s *Spec) CountedSteps() int {
	n := 0
	for i := range s.Steps {
		if s.Steps[i].Kind == "code" {
			n++
		}
	}
	return n
}

// Step finds a step by id, or nil.
func (s *Spec) Step(id string) *Step {
	for i := range s.Steps {
		if s.Steps[i].ID == id {
			return &s.Steps[i]
		}
	}
	return nil
}
