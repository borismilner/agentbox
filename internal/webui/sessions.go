package webui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/manual"
	"github.com/borismilner/agentbox/internal/session"
)

// The session surface (FR49). internal/session already does the hard part -
// it spawns `claude` headless, parses the stream-json NDJSON and hands back
// rendered turns - and it is deliberately UI-free, so all this layer does is
// turn its model into JSON the webview can paint, and throttle the pushes so
// a fast token stream cannot flood the bridge.

// wireSeg is one part of a turn: prose, thinking, or a tool call.
type wireSeg struct {
	Kind string `json:"kind"` // text | thinking | tool | result
	HTML string `json:"html,omitempty"`
	// Text is the source, carried only for a user turn: their own prompt shows as
	// what they typed, not as rendered markdown, and reconstructing that by
	// stripping tags out of the HTML leaves the entities behind (a typed
	// apostrophe came back as "&rsquo;").
	Text     string `json:"text,omitempty"`
	ToolName string `json:"toolName,omitempty"`
	// ToolInput is the plain summary (a tooltip, and what a copy would take);
	// ToolHTML is the same line highlighted when the tool's argument is code -
	// a Bash command is shell, and it was the only code in a conversation rendered
	// as flat grey text.
	ToolInput string `json:"toolInput,omitempty"`
	ToolHTML  string `json:"toolHtml,omitempty"`
	Result    string `json:"result,omitempty"`
	HasResult bool   `json:"hasResult,omitempty"`
	IsError   bool   `json:"isError,omitempty"`
}

type wireTurn struct {
	Role     string    `json:"role"`
	Segments []wireSeg `json:"segments"`
	Model    string    `json:"model,omitempty"`
	CostUSD  float64   `json:"costUsd,omitempty"`
	Err      string    `json:"err,omitempty"`
	// At is a 24-hour clock ("14:07"), and Think is how long the model worked
	// before its first word ("4s", "1m20s"). Both formatted in Go: the surface
	// prints what it is given.
	At    string `json:"at,omitempty"`
	Think string `json:"think,omitempty"`
}

// humanThink renders a thinking duration the way a person reads one. Under a
// second is not worth a line of text; over a minute wants the minutes.
func humanThink(d time.Duration) string {
	switch {
	case d < time.Second:
		return ""
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	default:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// wireSession is a row in the switcher plus, for the selected one, the whole
// conversation. Sending every conversation on every tick would be wasteful;
// sending only the selected one keeps the payload proportional to what is
// actually on screen. Ask is the exception and rides every row it belongs to,
// selected or not: it is one item, and a switcher row has to be able to say that
// this conversation is the one waiting on you (FR49, inline.go).
type wireSession struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Project  string     `json:"project"`
	Cwd      string     `json:"cwd"`
	Mode     string     `json:"mode"`
	State    string     `json:"state"`
	Hue      string     `json:"hue"`
	Turns    int        `json:"turns"`
	Selected bool       `json:"selected"`
	Model    string     `json:"model,omitempty"`
	Err      string     `json:"err,omitempty"`
	Conv     []wireTurn `json:"conv,omitempty"`
	Ask      *wireAsk   `json:"ask,omitempty"`
	// ShowCost is [session] show_cost, carried on the row so the setting applies
	// live rather than at the next window.
	ShowCost bool `json:"showCost,omitempty"`
}

type liveSession struct {
	id      string
	project string
	cwd     string
	mode    string
	drv     *session.Driver
	started time.Time

	// What the switcher chip says, in falling order of authority. Every session in
	// one directory used to be called the same thing - the directory - which is no
	// name at all once you have three of them open.
	//
	//   label: what the human called it. Renaming is the whole point of a label:
	//          you come back tomorrow and you have to find the one about the
	//          migration.
	//   auto:  what Claude called it - the first heading or sentence of its first
	//          reply, which is a better name than anything agentbox could invent,
	//          and free. Failing that, the first thing the human asked.
	//   project: the directory's name, the last resort.
	label string
	auto  string
}

// name is what the chip shows.
func (ls *liveSession) name() string {
	switch {
	case strings.TrimSpace(ls.label) != "":
		return ls.label
	case strings.TrimSpace(ls.auto) != "":
		return ls.auto
	default:
		return ls.project
	}
}

// nameFromConversation is the automatic name, worked out once and then left alone.
//
// Claude's own first words are the best name available and they cost nothing: the
// heading it opens a plan with, or its first sentence, is exactly what a person
// would have typed as a title. Asking it for a title instead would be another
// round trip and another entry in the transcript, for a worse answer. Until it has
// said anything, the first thing the human asked stands in.
func (ls *liveSession) nameFromConversation(turns []session.Turn) {
	if ls.auto != "" {
		return
	}
	var prompt string
	for _, t := range turns {
		for _, seg := range t.Segments {
			if seg.Kind != session.SegText || strings.TrimSpace(seg.Text) == "" {
				continue
			}
			if t.Role == session.RoleAssistant {
				ls.auto = titleFrom(seg.Text)
				return
			}
			if t.Role == session.RoleUser && prompt == "" {
				prompt = titleFrom(seg.Text)
			}
		}
	}
	ls.auto = prompt
}

// titleFrom reduces a piece of markdown to a chip's worth of words: the first
// line, undressed of heading marks and emphasis, cut at the first sentence and
// then at 42 characters, which is about what the switcher can show.
func titleFrom(md string) string {
	line := strings.TrimSpace(md)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimLeft(line, "#>-*` \t")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "`", "")
	line = strings.TrimSpace(line)

	// A first sentence is a name; a paragraph is not. Cut on the sentence end or
	// the dash a plan's heading usually hangs its explanation off.
	for _, cut := range []string{". ", " - ", ": ", "? ", "! "} {
		if i := strings.Index(line, cut); i > 8 {
			line = line[:i]
			break
		}
	}
	line = strings.TrimRight(line, ".:?! ")
	if r := []rune(line); len(r) > 42 {
		line = strings.TrimSpace(string(r[:42])) + "…"
	}
	return line
}

// Rename is the label a human puts on a session so it can be found tomorrow. An
// empty name gives it back to the automatic one rather than leaving it blank.
func (s *sessions) Rename(id, name string) {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	s.mu.Lock()
	ls := s.find(id)
	if ls != nil {
		ls.label = name
	}
	s.mu.Unlock()
	if ls == nil {
		return
	}
	s.save(ls) // the name is part of the conversation, not of this window
	s.ui.log.Info("webui.session_renamed", "component", "webui", "session", id, "named", name != "")
	s.touch()
}

type sessions struct {
	ui *UI

	mu       sync.Mutex
	list     []*liveSession
	selected string

	// ask is the one question routed into a conversation instead of a card
	// (FR49; the rule and the encoding are in inline.go). Zero means none.
	ask daemon.View

	// Coalesce pushes: a streaming response updates many times a second and
	// the surface only needs the latest state, never every intermediate one.
	dirty   bool
	pushing bool

	demo []wireSession // canned data for `agentbox webui-demo`; nil in the daemon
}

func newSessions(ui *UI) *sessions { return &sessions{ui: ui} }

// Start spawns a Claude session in cwd. mode is "plan" (read-only, the
// default) or "full".
//
// An empty cwd resolves to [session] dir, and an empty dir to the daemon's own
// working directory - resolved to an absolute path here rather than left as ".",
// so the switcher chip and the panel's header say where the session actually is.
// Every route in passes "" (both "+ New" buttons, the panel's own session), which
// is what makes that config knob the single place that decides.
func (s *sessions) Start(cwd, mode string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		cwd = strings.TrimSpace(s.ui.conf().Session.Dir)
	}
	if strings.TrimSpace(cwd) == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		} else {
			cwd = "."
		}
	}
	return s.spawn(spawnReq{cwd: cwd, mode: mode})
}

// spawnReq is one child to start. It became a struct when assignments (M12)
// needed a session with its own model, its own brief and no claim on the
// human's selection: eight positional arguments is a call nobody can read, and
// four of them would have been empty at every existing site.
type spawnReq struct {
	cwd  string
	mode string
	// resume is Claude's own session id to carry on from; seed is the
	// transcript to show under it.
	resume string
	seed   []session.Turn
	// id is normally empty, meaning "make one". A mode switch passes the OLD
	// id, and that is not cosmetic: the switcher is ordered by id, the child
	// carries it as AGENTBOX_SESSION_ID, and a question the previous child asked is
	// routed into this conversation by it. Keeping the id keeps the session's
	// place, its selection, and anything still waiting to be answered in it.
	id string
	// model is passed to --model; empty is whatever claude defaults to.
	model string
	// brief replaces the session briefing appended to the system prompt.
	brief string
	// label is the chip's name when the caller already knows it better than the
	// conversation will (an assignment has a name; a conversation earns one).
	label string
	// background keeps the human's selection where it is. An assignment that
	// fires while the panel is open must not move them off what they were
	// typing into.
	background bool
}

// spawn is the one place a child is started: a new session, a mode switch
// (which is a new child carrying the old conversation, because
// --permission-mode is a spawn-time flag), reopening a saved session and
// carrying out an assignment all come through here.
func (s *sessions) spawn(req spawnReq) (string, error) {
	mode := req.mode
	if mode != "full" && mode != "plan" {
		mode = "full"
	}
	id := req.id
	if id == "" {
		id = fmt.Sprintf("s%d", time.Now().UnixNano()%1e9)
	}
	brief := req.brief
	if brief == "" {
		// A session agentbox started knows it is inside agentbox: which tools it has,
		// that a question it asks here comes back inline rather than as a card,
		// and that the human will wander off the moment it starts something long.
		brief = manual.Session()
	}

	ls := &liveSession{
		id:      id,
		project: projectOf(req.cwd),
		cwd:     req.cwd,
		mode:    mode,
		label:   req.label,
		started: time.Now(),
	}
	ls.drv = session.New(session.Config{
		Bin:     s.ui.conf().Session.Binary, // empty = `claude` on PATH
		Dir:     req.cwd,
		Mode:    mode,
		Model:   req.model,
		Partial: true, // live token deltas; the surface throttles its own repaints
		Env:     []string{"AGENTBOX_SESSION_ID=" + id},
		Brief:   brief,
		Resume:  req.resume,
	}, s.ui.log, func() { s.touch() })

	if len(req.seed) > 0 {
		ls.drv.Seed(req.seed) // what was said before this child existed
	}
	if err := ls.drv.Start(); err != nil {
		return "", err
	}

	s.mu.Lock()
	s.list = append(s.list, ls)
	if !req.background {
		s.selected = id
	}
	s.mu.Unlock()

	s.touch()
	return id, nil
}

// SetMode switches a session between plan and full.
//
// It cannot be done to a running child: --permission-mode is a spawn-time flag,
// which is why the surface used to show Plan/Full as two words that did nothing.
// So the child is replaced - resumed from Claude's own session id, so it keeps its
// context, and seeded with the transcript, so the human keeps their conversation -
// and the old row goes. The new session takes the old one's place in the list
// rather than appending, or switching mode would send you to the end of your own
// switcher.
func (s *sessions) SetMode(id, mode string) (string, error) {
	s.mu.Lock()
	ls := s.find(id)
	s.mu.Unlock()
	if ls == nil {
		return "", fmt.Errorf("no session %s", id)
	}
	if ls.mode == mode {
		return id, nil
	}

	resume := ls.drv.SessionID()
	turns := ls.drv.Turns()
	cwd := ls.cwd

	// Saved first: a mode switch kills a child, and a crash between the two should
	// not be how a conversation is lost.
	s.save(ls)

	s.Close(id)
	if _, err := s.spawn(spawnReq{cwd: cwd, mode: mode, resume: resume, id: id, seed: turns}); err != nil {
		return "", err
	}
	s.ui.log.Info("webui.session_mode", "component", "webui", "session", id,
		"mode", mode, "resumed", resume != "", "turns", len(turns))
	s.touch()
	return id, nil
}

// save writes a session's conversation to the state directory, into the same file
// every time so a conversation is one file rather than a pile of snapshots. Errors
// are logged: failing to save is not a reason to refuse to close.
func (s *sessions) save(ls *liveSession) {
	turns := ls.drv.Turns()
	if len(turns) == 0 {
		return
	}
	s.mu.Lock()
	ls.nameFromConversation(turns)
	title := ls.name()
	s.mu.Unlock()

	meta := session.Meta{
		SessionID: ls.drv.SessionID(),
		Cwd:       ls.cwd,
		Mode:      ls.mode,
		Title:     title,
	}
	name := ls.started.Format("20060102-150405") + "-" + ls.id
	path, err := session.SaveInto(session.SessionsDir(), name, turns, meta)
	if err != nil {
		s.ui.log.Warn("webui.session_save_failed", "component", "webui", "err", err.Error())
		return
	}
	s.ui.log.Debug("webui.session_saved", "component", "webui", "path", path, "turns", len(turns))
}

// SaveAll writes every live conversation. The daemon calls it on shutdown, so a
// restart does not throw away what was said.
func (s *sessions) SaveAll() {
	s.mu.Lock()
	list := append([]*liveSession(nil), s.list...)
	s.mu.Unlock()
	for _, ls := range list {
		s.save(ls)
	}
}

// Saved lists the conversations on disk, newest first, for a surface that offers
// to reopen one.
func (s *sessions) Saved() []session.Saved {
	return session.List(session.SessionsDir(), 30)
}

// Reopen starts a session on a saved conversation: the transcript is shown again
// and the child is resumed from Claude's own id, so it remembers as well as
// displays. The directory and mode come from the file, because a conversation
// about one project makes no sense reopened in another.
func (s *sessions) Reopen(path string) (string, error) {
	turns, meta, err := session.Read(path)
	if err != nil {
		return "", err
	}
	cwd := meta.Cwd
	if strings.TrimSpace(cwd) == "" {
		cwd = strings.TrimSpace(s.ui.conf().Session.Dir)
	}
	mode := meta.Mode
	if mode == "" {
		mode = s.ui.conf().Session.DefaultMode
	}
	id, err := s.spawn(spawnReq{cwd: cwd, mode: mode, resume: meta.SessionID, seed: turns})
	if err != nil {
		return "", err
	}
	s.ui.log.Info("webui.session_reopened", "component", "webui", "session", id,
		"path", path, "turns", len(turns), "resumed", meta.SessionID != "")
	return id, nil
}

// EnsureOne makes sure there is a conversation to drop into. The panel is a
// console you reach with a hotkey, and a console that comes down onto "No session
// yet" with a + New button is a detour: you asked for it because you had a
// sentence in your head, and now you have to click first. So the first roll starts
// one, in [session] dir and the default mode, and every roll after that finds it
// already there.
//
// It is not started with the daemon on purpose - that would keep a `claude` child
// alive on every desktop that has agentbox installed, whether or not the panel is
// ever used. Idle costs nothing once it exists, so paying for it on first use is
// the whole cost.
//
// Errors are logged and not returned: `claude` missing is a real situation (agentbox
// is useful without it), and the surface already says so in the row it gets.
func (s *sessions) EnsureOne() {
	s.mu.Lock()
	have := len(s.list) > 0 || len(s.demo) > 0
	s.mu.Unlock()
	if have {
		return
	}
	if _, err := s.Start("", s.ui.conf().Session.DefaultMode); err != nil {
		s.ui.log.Warn("webui.panel_session_failed", "component", "webui", "err", err.Error())
	}
}

func (s *sessions) Select(id string) {
	s.mu.Lock()
	s.selected = id
	s.mu.Unlock()
	s.touch()
}

func (s *sessions) Send(id, prompt string) error {
	s.mu.Lock()
	ls := s.find(id)
	s.mu.Unlock()
	if ls == nil {
		return fmt.Errorf("no session %s", id)
	}
	if err := ls.drv.Send(prompt); err != nil {
		return err
	}
	s.touch()
	return nil
}

func (s *sessions) Stop(id string) {
	s.mu.Lock()
	ls := s.find(id)
	s.mu.Unlock()
	if ls != nil {
		ls.drv.Stop()
	}
	s.touch()
}

// Close ends a session and takes its row off the switcher, which Stop does not:
// Stop closes the child's stdin and leaves the row (and an agent mid-turn) where
// it was. The child is killed rather than asked, because the row is going and a
// child with nowhere to write is a leak.
//
// Selection moves to the neighbour so the surface is never left pointing at a
// session that no longer exists, and a question waiting in this conversation is
// re-presented as a card - closing the conversation an agent is waiting in must
// not leave that agent waiting forever, the same rule as closing the app window
// (inline.go).
func (s *sessions) Close(id string) {
	s.mu.Lock()
	ls := s.find(id)
	s.mu.Unlock()
	if ls != nil {
		s.save(ls) // a closed conversation is reopenable, not gone
	}
	s.mu.Lock()
	ls = s.find(id)
	s.list, s.selected = dropRow(s.list, func(x *liveSession) string { return x.id }, s.selected, id)
	// The demo harness has no children and carries its selection in the canned
	// rows, so dropping the row is the whole behaviour there - enough to drive the
	// interaction without a real `claude`.
	if s.demo != nil {
		s.demo, s.selected = dropRow(s.demo, func(d wireSession) string { return d.ID }, s.selected, id)
	}
	orphaned := s.ask.Item != nil && s.ask.Item.Identity.Session == id
	s.mu.Unlock()

	if ls != nil {
		ls.drv.Kill()
	}
	s.ui.log.Info("webui.session_closed", "component", "webui", "session", id, "orphaned_ask", orphaned)
	if orphaned {
		go s.ui.rerouteAsk()
	}
	s.touch()
}

// dropRow removes the row with id and says which session the surface should
// point at afterwards: the one before it where there is one, so closing the
// middle of a list does not throw you to the far end, and nothing at all when the
// list empties. Generic because the live switcher and the demo harness hold their
// rows in different shapes and must behave identically.
func dropRow[T any](rows []T, idOf func(T) string, selected, id string) ([]T, string) {
	at := -1
	kept := make([]T, 0, len(rows))
	for i, r := range rows {
		if idOf(r) == id {
			at = i
			continue
		}
		kept = append(kept, r)
	}
	if at < 0 || selected != id {
		return kept, selected
	}
	switch {
	case len(kept) == 0:
		return kept, ""
	case at > 0:
		return kept, idOf(kept[at-1])
	default:
		return kept, idOf(kept[0])
	}
}

func (s *sessions) find(id string) *liveSession {
	for _, ls := range s.list {
		if ls.id == id {
			return ls
		}
	}
	return nil
}

// shownLocked reports whether a session id has a conversation on this surface,
// which is the question inline routing actually needs answered: a panel is only
// somewhere to put a question if there is a transcript to put it under. The demo
// list counts for exactly that reason - its conversation is on screen.
func (s *sessions) shownLocked(id string) bool {
	if id == "" {
		return false
	}
	if s.demo != nil {
		for i := range s.demo {
			if s.demo[i].ID == id {
				return true
			}
		}
		return false
	}
	return s.find(id) != nil
}

// touch marks the state dirty and schedules at most one push per frame-ish
// interval. Streaming can update hundreds of times a second; the eye cannot.
func (s *sessions) touch() {
	s.mu.Lock()
	s.dirty = true
	if s.pushing {
		s.mu.Unlock()
		return
	}
	s.pushing = true
	s.mu.Unlock()

	go func() {
		for {
			time.Sleep(60 * time.Millisecond)
			s.mu.Lock()
			if !s.dirty {
				s.pushing = false
				s.mu.Unlock()
				return
			}
			s.dirty = false
			s.mu.Unlock()
			s.ui.emit("agentbox:sessions", s.snapshot())
		}
	}()
}

func (s *sessions) snapshot() []wireSession {
	s.mu.Lock()
	ask := s.ask
	if s.demo != nil {
		// Copied, not handed out: attachAsk writes into the slice it is given and
		// the canned list is shared state.
		out := make([]wireSession, len(s.demo))
		copy(out, s.demo)
		s.mu.Unlock()
		return s.attachAsk(out, ask)
	}
	list := make([]*liveSession, len(s.list))
	copy(list, s.list)
	selected := s.selected
	s.mu.Unlock()

	dark := s.ui.themeMode() == "dark"
	showCost := s.ui.conf().Session.ShowCost
	out := make([]wireSession, 0, len(list))
	for _, ls := range list {
		w := wireSession{
			ShowCost: showCost,
			ID:       ls.id,
			Title:    ls.name(),
			Project:  ls.project,
			Cwd:      ls.cwd,
			Mode:     ls.mode,
			State:    ls.drv.State().String(),
			Hue:      IdentityHue("claude-code", ls.project, dark),
			Model:    ls.drv.Model(),
			Err:      ls.drv.LastError(),
			Selected: ls.id == selected,
		}
		turns := ls.drv.Turns()
		w.Turns = len(turns)
		// The automatic name is worked out here because this is where the turns are
		// already in hand, and it sticks the first time it finds anything. Under the
		// lock, briefly: two surfaces can snapshot at once, and Rename writes the
		// label from a third goroutine.
		s.mu.Lock()
		ls.nameFromConversation(turns)
		w.Title = ls.name()
		s.mu.Unlock()
		if w.Selected {
			w.Conv = encodeTurns(turns)
		}
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return s.attachAsk(out, ask)
}

// encodeTurns renders the conversation for the surface, joining a RUN of
// consecutive assistant messages into one turn.
//
// Claude Code sends a separate assistant message per tool call, so a reply that
// ran five commands before writing a word arrived as six turns - and the surface
// drew its identity pill, its clock and its whole header on each one. Six of the
// same green pill down the left edge for one answer. They are one turn as far as
// anybody reading is concerned: the tool rows, then the prose, under one heading.
// The clock is the moment the reply started; the thinking time comes from whichever
// message finally produced text; the cost is the run's.
func encodeTurns(turns []session.Turn) []wireTurn {
	out := make([]wireTurn, 0, len(turns))
	for _, t := range turns {
		if t.Role == session.RoleAssistant && len(out) > 0 &&
			out[len(out)-1].Role == string(session.RoleAssistant) {
			prev := &out[len(out)-1]
			prev.Segments = append(prev.Segments, encodeSegments(t)...)
			prev.CostUSD += t.CostUSD
			if prev.Model == "" {
				prev.Model = t.Model
			}
			if prev.Think == "" {
				prev.Think = humanThink(t.Think)
			}
			continue
		}
		wt := wireTurn{Role: string(t.Role), Model: t.Model, CostUSD: t.CostUSD, Err: t.Err}
		// The clock is formatted here, 24 hour, because Go knows the daemon's
		// timezone and a webview's locale guessing does not need to be involved.
		if !t.At.IsZero() {
			wt.At = t.At.Format("15:04")
		}
		wt.Think = humanThink(t.Think)
		wt.Segments = encodeSegments(t)
		out = append(out, wt)
	}
	return out
}

func encodeSegments(t session.Turn) []wireSeg {
	out := make([]wireSeg, 0, len(t.Segments))
	for _, seg := range t.Segments {
		ws := wireSeg{
			ToolName:  seg.ToolName,
			ToolInput: seg.ToolInput,
			Result:    trim(seg.Result, 4000),
			HasResult: seg.HasResult,
			IsError:   seg.IsError,
		}
		switch seg.Kind {
		case session.SegText:
			ws.Kind, ws.HTML = "text", RenderMarkdown(seg.Text)
			if t.Role == session.RoleUser {
				ws.Text = seg.Text // rendered for nobody; shown as typed
			}
		case session.SegThinking:
			ws.Kind, ws.HTML = "thinking", RenderMarkdown(seg.Text)
		case session.SegToolUse:
			ws.Kind = "tool"
			if lang := toolLang(seg.ToolName); lang != "" {
				ws.ToolHTML = HighlightInline(seg.ToolInput, lang)
			}
		default:
			ws.Kind = "result"
		}
		out = append(out, ws)
	}
	return out
}

// toolLang is the lexer a tool's one-line argument should be read with, or "" for
// a tool whose argument is not code (a path, a search string, a description).
// Keyed on the tool names Claude Code actually uses, and deliberately short: a
// wrong lexer colours a path like a keyword, which is worse than not colouring it.
func toolLang(tool string) string {
	switch tool {
	case "Bash", "BashOutput", "Shell", "KillShell":
		return "bash"
	case "Grep", "Glob":
		return "regex"
	default:
		return ""
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n… truncated"
}

func projectOf(cwd string) string {
	cwd = strings.TrimRight(cwd, "/")
	if i := strings.LastIndex(cwd, "/"); i >= 0 && i+1 < len(cwd) {
		return cwd[i+1:]
	}
	if cwd == "" || cwd == "." {
		return "workspace"
	}
	return cwd
}

// --- demo -------------------------------------------------------------------

// SetDemo installs a canned conversation so the surface can be built and
// looked at without spawning a real `claude`. Only `agentbox webui-demo` calls
// it; the daemon never does.
func (s *sessions) SetDemo(list []wireSession) {
	s.mu.Lock()
	s.demo = list
	s.mu.Unlock()
	s.touch()
}
