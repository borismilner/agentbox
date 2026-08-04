// Package webui renders agentbox's surfaces in a webview (ADR-0009, superseding
// ADR-0002). It satisfies daemon.Presenter exactly as internal/ui did, so the
// daemon, the store, the queue and the protocol are untouched by the port:
// the toolkit changed, the architecture did not.
//
// Window model is unchanged too (04-platform.md): one window per prompt, born
// and destroyed with the item, never a long-lived window that unhides. What
// changed is how it gets on screen - see x11.go for the map-without-focus
// dance that keeps vision principle 3 intact under GTK4.
package webui

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/borismilner/agentbox/frontend"
	"github.com/borismilner/agentbox/internal/config"
	"github.com/borismilner/agentbox/internal/daemon"
	"github.com/borismilner/agentbox/internal/proto"
)

// Resolver is the daemon's answer-taking side, unchanged from internal/ui so
// the same *daemon.Daemon satisfies both.
type Resolver interface {
	Answer(id, label string)
	Reply(id, text string)
	AnswerForm(id string, values map[string]string)
	Dismiss(id string)
	Defer(id string)
	Undo(id string)
	Veto(id string)
	Secret(id, value string)
	RunAction(id string, index int)
	Review(id string, approved bool, c string)
	// ArtifactEvent is the human acting inside an artifact rather than on a card
	// (M10): a click or a slider, on its way to whichever agent is waiting for it.
	ArtifactEvent(ev proto.ArtifactEvent)
}

// cardView is what a card or toast window renders. It is daemon.View flattened
// for the wire, with the things only the Go side can work out folded in: the
// body rendered to HTML, the queue's identity hues, the icon the level wears,
// and whether the window closes itself.
type cardView struct {
	Item           *proto.Item `json:"item"`
	BodyHTML       string      `json:"bodyHtml"`
	Waiting        int         `json:"waiting"`
	WaitingHues    []string    `json:"waitingHues"`
	Graced         bool        `json:"graced"`
	GracedText     string      `json:"gracedText"`
	GraceUntilMS   int64       `json:"graceUntilMs"`
	DismissAtMS    int64       `json:"dismissAtMs"`
	ExpiresAtMS    int64       `json:"expiresAtMs"`
	ActionsEnabled bool        `json:"actionsEnabled"`
	Caller         string      `json:"caller"`

	// Glyph is the severity icon's name; Sticky says the strip waits to be
	// dismissed rather than counting down (warning and error notices do, which
	// is why the daemon gives them no dismiss deadline).
	Glyph  string `json:"glyph"`
	Sticky bool   `json:"sticky"`
}

type UI struct {
	app *application.App
	res Resolver
	log *slog.Logger

	mu sync.Mutex
	// prompt is the one window an item gets: a card, or a toast for a notify.
	// promptKind records which, because the two cannot be swapped in place.
	prompt     *application.WebviewWindow
	promptKind string
	appWin     *application.WebviewWindow
	// appShow is whether that window is on screen. Separate from appWin != nil
	// because the tray hides the window rather than closing it, and a hidden
	// window is still a live one holding its sessions (app.go ToggleApp).
	appShow bool
	cur     daemon.View
	theme   Theme
	x       *x11

	// src feeds the inbox and history surfaces; set after the daemon exists
	// (construction order - the daemon needs this UI as its Presenter).
	src Source

	sess   *sessions
	inbox  *inbox
	view   *viewer
	prog   *progress
	ctrl   *control
	agents *agents
	// top owns the top-centre column, so no surface places itself there (FR75).
	top   *topStack
	pan   *panel
	board *boardWin

	// boardStore feeds the review board; set after the daemon exists, like
	// src.
	boardStore BoardStore

	// assignStore feeds the Assignments surface (M12/FR82), wired the same way
	// and legitimately nil in a demo build.
	assignStore AssignmentStore

	// voice reads a screen out loud when the human asks. Nil until the daemon
	// is wired, and legitimately nil in a demo build.
	voice Voice

	// handover answers the control strip's Deny and Allow (FR74). Nil until the
	// daemon is wired, and legitimately nil in a demo build.
	handover Handover

	// roster answers the Agents surface's break-lock (FR83), wired the same way
	// and nil while the surface is still a mock over canned rows.
	roster Roster

	// OnView mirrors internal/ui so the tray badge keeps working.
	OnView      func(v daemon.View)
	OnAppChange func(open bool)

	// up says the GTK main loop is running; deferred holds the window work that
	// arrived before it did. Every window operation goes through
	// application.InvokeSync, which dereferences the platform application that
	// Run creates - before Run that pointer is nil and the process dies. The
	// daemon reaches the UI early by construction: it presents a restored
	// unresolved item from inside daemon.New, and it serves the socket a moment
	// before Run, so a `agentbox show` that spawns the daemon can land first too.
	up       bool
	deferred []deferredOp

	// cfg is the live configuration. Every window size, the reading measure, the
	// panel's geometry and the session defaults come from here rather than from a
	// constant, and SetConfig replaces it whenever the file changes - so tuning
	// agentbox is a conversation with the config file, not a restart. Read it through
	// the helpers below; they hold the lock.
	cfg config.Config

	// invoke puts a function on the UI thread. It is a field so the gate can be
	// tested without a webview; New sets it to application.InvokeSync, and a UI
	// built without one drops window work rather than reaching for a nil app.
	invoke func(func())

	ready chan struct{}
	once  sync.Once
}

// deferredOp is one window operation waiting for the main loop. The key makes a
// repeat replace its predecessor instead of opening a second window.
type deferredOp struct {
	key string
	fn  func()
}

// cardH is the height a card opens at before the surface measures itself and
// calls Fit. It is not a knob: nobody wants to configure a value they see for one
// frame.
const cardH = 200

// The window shapes are configuration ([window] in config.toml), read through
// these so a reload lands everywhere at once.
func (u *UI) cardGeom() (w, maxH int) {
	c := u.conf()
	return c.Window.CardWidth, c.Window.CardMaxHeight
}

func (u *UI) toastGeom() (w, maxH, topInset int) {
	c := u.conf()
	return c.Window.ToastWidth, c.Window.ToastMaxHeight, c.Window.ToastTopInset
}

// toastTopInset is the gap below the top edge a top-centre surface sits at. The
// control strip shares it with the toasts: same edge, same reason to clear the
// GNOME bar, and two different insets there would look like a mistake.
func (u *UI) toastTopInset() int {
	return u.conf().Window.ToastTopInset
}

func (u *UI) appGeom() (w, h int) {
	c := u.conf()
	return c.Window.AppWidth, c.Window.AppHeight
}

func (u *UI) viewerGeom() (w, h int) {
	c := u.conf()
	return c.Window.ViewerWidth, c.Window.ViewerHeight
}

func (u *UI) progressGeom() (w, maxH int) {
	c := u.conf()
	return c.Window.ProgressWidth, c.Window.ProgressMaxHeight
}

func (u *UI) panelFracs() (float64, float64) {
	c := u.conf()
	return c.Panel.WidthFrac, c.Panel.HeightFrac
}

// New builds the application shell. It does not run it; Run blocks on the
// GTK main loop and must own the main goroutine.
func New(res Resolver, log *slog.Logger, cfg config.Config) *UI {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	u := &UI{res: res, log: log, theme: BuildTheme(cfg), ready: make(chan struct{}),
		invoke: application.InvokeSync, cfg: cfg}
	u.sess = newSessions(u)
	u.inbox = newInbox(u)
	u.view = newViewer(u)
	u.prog = newProgress(u)
	u.ctrl = newControlStrip(u)
	u.agents = newAgents(u)
	u.top = newTopStack(u)
	u.pan = newPanel(u)
	u.board = newBoard(u)

	u.app = application.New(application.Options{
		Name:        "agentbox",
		Description: "Agents ask; you answer.",
		LogLevel:    slog.LevelWarn,
		Services:    []application.Service{application.NewService(&Bridge{ui: u})},
		Assets: application.AssetOptions{
			Handler:        assetHandler(),
			DisableLogging: true,
		},
		Flags: map[string]any{"theme": u.theme},
		// agentbox is tray-resident and its windows are transient by design: a card
		// is born and destroyed with its item, so the process has to outlive them
		// all. Without this, answering the first question would close the last
		// window and take the daemon down with it.
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
			ProgramName:                   "agentbox",
		},
	})
	u.x = dialX11()
	return u
}

func assetHandler() http.Handler {
	sub, err := fsSub(frontend.Dist, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return application.BundledAssetFileServer(sub)
}

// Run drives the GTK main loop. Returns when the application quits. Anything
// the daemon asked for before this point is replayed as soon as the loop is up.
func (u *UI) Run() error {
	u.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		u.loopStarted()
	})
	return u.app.Run()
}

// onMain runs fn on the UI thread, or queues it under key when the main loop is
// not up yet (see UI.up). A test UI has no application behind it and drops the
// work: the encoders and the triage rules are what tests exercise, not windows.
func (u *UI) onMain(key string, fn func()) {
	if u.invoke == nil {
		return
	}
	u.mu.Lock()
	if u.up {
		u.mu.Unlock()
		u.invoke(fn)
		return
	}
	replaced := false
	for i := range u.deferred {
		if u.deferred[i].key == key {
			u.deferred[i].fn, replaced = fn, true
			break
		}
	}
	if !replaced {
		u.deferred = append(u.deferred, deferredOp{key: key, fn: fn})
	}
	u.mu.Unlock()
	u.log.Debug("webui.deferred_until_run", "component", "webui", "op", key)
}

// loopStarted marks the loop up and plays back what arrived before it.
func (u *UI) loopStarted() {
	u.mu.Lock()
	u.up = true
	ops := u.deferred
	u.deferred = nil
	u.mu.Unlock()
	for _, op := range ops {
		u.log.Debug("webui.replayed", "component", "webui", "op", op.key)
		u.invoke(op.fn)
	}
}

// Quit stops the main loop from any goroutine.
func (u *UI) Quit() { u.app.Quit() }

// conf is the live configuration.
func (u *UI) conf() config.Config {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cfg
}

// SetConfig takes a reloaded configuration and applies all of it, now: the tokens
// go out as a CSS variable write (theme, fonts, the reading measure), and every
// open window that is sized by the config is resized in place. Only the knobs the
// daemon genuinely reads once - history retention, the log, dnd.start_in_dnd -
// need a restart, and the settings surface says so.
//
// This is the whole point of config.Watch pointing here: you edit config.toml (or
// click Save) and agentbox changes while you look at it.
func (u *UI) SetConfig(cfg config.Config) {
	t := BuildTheme(cfg)
	u.mu.Lock()
	u.theme, u.cfg = t, cfg
	u.mu.Unlock()
	u.emit("agentbox:theme", t)
	u.resizeToConfig()
}

// SetTheme is the older name for SetConfig, kept because "the config changed" and
// "re-theme" were the same call before the window sizes became knobs.
func (u *UI) SetTheme(cfg config.Config) { u.SetConfig(cfg) }

// resizeToConfig applies the new window shapes to the windows that are already
// open, so a size change is something you watch happen rather than something you
// restart for. A card is left alone deliberately: it is measured from its own
// content (Fit) and re-sizing it under an answer that is being read would move the
// buttons out from under the pointer. The next card opens at the new size.
func (u *UI) resizeToConfig() {
	u.mu.Lock()
	appWin, x := u.appWin, u.x
	u.mu.Unlock()

	if appWin != nil {
		aw, ah := u.appGeom()
		u.onMain("app.resize", func() {
			if w, h := appWin.Size(); w != aw || h != ah {
				appWin.SetSize(aw, ah)
			}
		})
	}
	// nil in a UI built by hand (a settings test builds one to exercise the
	// writer, not the windows).
	if u.view != nil {
		u.view.resizeToConfig()
	}
	if u.pan != nil {
		u.pan.resizeToConfig()
	}
	if u.board != nil {
		u.board.resizeToConfig()
	}
	_ = x
}

// Present renders the daemon's current view (daemon.Presenter). A nil Item
// clears the card.
func (u *UI) Present(v daemon.View) {
	u.mu.Lock()
	u.cur = v
	hook := u.OnView
	up := u.up
	u.mu.Unlock()
	if hook != nil {
		hook(v)
	}

	// No main loop yet: keep the view and present whatever is current once the
	// loop is up. Re-reading it there is the point - a restored item can be
	// answered or dismissed in the meantime, and then there is nothing to show.
	if !up {
		u.inbox.noteChange()
		u.onMain("present", func() { u.Present(u.currentView()) })
		return
	}

	// Every queue change comes through here, so this is where the inbox stays
	// live: the surface never polls, and a row's outcome updates the moment the
	// daemon resolves it.
	u.inbox.noteChange()

	// A question from an agent running in agentbox's own session surface is answered
	// in that conversation, not in a card over the top of it (FR49, inline.go).
	// This call also clears a panel the last item left, so it runs whatever the
	// view holds - including nothing.
	if u.sess.routeAsk(v, u.AppOpen()) {
		u.closeCard()
		return
	}

	if v.Item == nil {
		u.closeCard()
		return
	}
	u.showCard(v)
}

// currentView is the view the daemon last presented.
func (u *UI) currentView() daemon.View {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cur
}

// showCard puts the current item on screen in the treatment it deserves. The
// open window is reused when the next item wants the same treatment - a queue of
// three questions is one window, not three - but a card cannot become a toast in
// place, so a change of treatment replaces the window.
func (u *UI) showCard(v daemon.View) {
	payload := u.encode(v)
	kind := treatment(v.Item)

	w, h := u.cardHeight2(v)
	if kind == "toast" {
		tw, _, _ := u.toastGeom()
		w, h = tw, u.toastHeight(v.Item)
	}

	// One title per surface, so a driver can say which window it means
	// (progress, panel and the viewer already carry theirs). The card keeps
	// the bare name every script and recipe targets as "=agentbox".
	title := "agentbox"
	if kind == "toast" {
		title = "agentbox · toast"
	}

	u.onMain("card", func() {
		// Reuse-or-replace is decided here, inside the closure, where the UI
		// thread serialises it with the creation it decides about. Deciding at
		// the call site read u.prompt before an earlier closure had assigned
		// it, so two near-simultaneous items could each create a window and
		// the loser fell off tracking - the card-shaped ghost that outlived
		// every item's resolution (session 25). Present runs outside the
		// daemon lock, so that race needs no restart to happen.
		u.mu.Lock()
		existing, existingKind := u.prompt, u.promptKind
		u.mu.Unlock()
		if existing != nil {
			if existingKind == kind {
				u.emit("agentbox:view", payload)
				return
			}
			u.closeCardNow()
		}

		win := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:          "agentbox-" + kind,
			Title:         title,
			Width:         w,
			Height:        h,
			Frameless:     true,
			DisableResize: true,
			AlwaysOnTop:   true,
			Hidden:        true,
			URL:           "/?surface=" + kind,
			// Transparent so the card's own rounded corners and drop shadow
			// are the window's real shape; an opaque window would square
			// them off and there is no decoration to hide the seam.
			BackgroundType:   application.BackgroundTypeTransparent,
			BackgroundColour: application.RGBA{},
			Linux:            application.LinuxWindow{WindowIsTranslucent: true},
		})
		u.mu.Lock()
		u.prompt, u.promptKind = win, kind
		u.mu.Unlock()

		// Pop above, never grab (vision principle 3). Everything about the
		// order here matters: hints go on before the map, the stacking and
		// placement client messages only work after it. A toast asks for the
		// notification type and sits at the top of the screen; a card asks for
		// dialog and takes the middle.
		if u.x != nil {
			xid := xidOf(win.NativeWindow())
			if xid == 0 {
				// Wails creates the native window lazily, so around startup
				// NativeWindow can still answer nil. Run creates it now - a
				// no-op once it exists, and Hidden in the options keeps it
				// unmapped - so the hints still go on before the first map.
				win.Run()
				xid = xidOf(win.NativeWindow())
			}
			if xid != 0 {
				u.x.prepare(xid, kind == "toast")
				showNoActivate(win.NativeWindow())
				_, _, inset := u.toastGeom()
				u.x.settle(xid, w, h, kind == "toast", inset)
				// The Title option does not survive framelessness
				// (x11.go setName); write it onto the mapped window.
				u.x.setName(xid, title)
				// A toast shares the top-centre edge with the hands-off strip, so it
				// takes a slot in the column instead of the strip's position (FR75).
				// A card is centred and wants no slot - and must give one back, since
				// this window is reused between the two treatments.
				if kind == "toast" {
					u.top.put("toast", xid, w, h, false)
				} else {
					u.top.drop("toast")
				}
				u.armCard(payload)
				return
			}
			// Still no X11 surface: a Wayland session, where Show() is the
			// designed degrade. Worth a line in the log either way - a window
			// on this path has none of the card hints, which is exactly what
			// the session-25 ghost wore.
			u.log.Warn("webui.card_unprepared", "component", "webui", "kind", kind)
		}
		win.Show()
		u.armCard(payload)
	})
}

// armCard waits for the surface to say it is listening, then sends the view.
// Emitting before the bundle has run would drop the first card of a session.
func (u *UI) armCard(payload cardView) {
	go func() {
		select {
		case <-u.ready:
		case <-time.After(2 * time.Second):
			u.log.Warn("webui.card_ready_timeout", "component", "webui")
		}
		u.emit("agentbox:view", payload)
	}()
}

// closeCard retires the prompt window. The close travels through the same
// onMain key as showCard, so a close ordered after a show cannot overtake the
// queued show and leave a window on screen for an item already resolved.
func (u *UI) closeCard() {
	u.onMain("card", u.closeCardNow)
}

// closeCardNow does the closing; UI thread only.
func (u *UI) closeCardNow() {
	u.mu.Lock()
	w := u.prompt
	u.prompt, u.promptKind = nil, ""
	u.ready = make(chan struct{})
	u.once = sync.Once{}
	u.mu.Unlock()
	if w == nil {
		return
	}
	// Whatever it was, it is not in the column now: a slot nobody occupies holds a
	// gap open under the hands-off strip (FR75).
	u.top.drop("toast")
	w.Close()
}

// emit pushes an event to every open window. It tolerates a UI with no
// application behind it, which is what a test has: the encoders and the triage
// rules are the parts worth testing, and neither needs a webview.
func (u *UI) emit(name string, data any) {
	if u.app == nil {
		return
	}
	u.app.Event.Emit(name, data)
}

// themeMode reports dark or light for code paths that need to pick a hue.
func (u *UI) themeMode() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.theme.Mode
}

// markReady is called by the Bridge when a surface finishes mounting.
func (u *UI) markReady() {
	u.mu.Lock()
	ch, once := u.ready, &u.once
	u.mu.Unlock()
	once.Do(func() { close(ch) })
}

func (u *UI) encode(v daemon.View) cardView {
	dark := u.theme.Mode == "dark"
	hues := make([]string, 0, len(v.WaitingFrom))
	for _, id := range v.WaitingFrom {
		hues = append(hues, IdentityHue(id.Agent, id.Project, dark))
	}
	body := ""
	glyph := ""
	if v.Item != nil {
		body = RenderMarkdown(v.Item.Body)
		glyph = severityGlyph(v.Item.EffectiveLevel())
	}
	return cardView{
		Item:           v.Item,
		BodyHTML:       body,
		Waiting:        v.Waiting,
		WaitingHues:    hues,
		Graced:         v.Graced,
		GracedText:     v.GracedText,
		GraceUntilMS:   ms(v.GraceUntil),
		DismissAtMS:    ms(v.DismissAt),
		ExpiresAtMS:    ms(v.ExpiresAt),
		ActionsEnabled: v.ActionsEnabled,
		Caller:         callerName(v.Caller),
		Glyph:          glyph,
		// No deadline means nothing is going to take this off the screen: a
		// warning or error notice waits to be read (03-ui-ux.md), and the strip
		// has to say so instead of showing a countdown that never ticks.
		Sticky: v.DismissAt.IsZero() && isToast(v.Item),
	}
}

// cardHeight keeps the window close to its content. A frameless window cannot
// be sloppy about this the way a decorated one can: empty space below a
// two-line question reads as a bug.
func (u *UI) cardHeight(v daemon.View) int {
	if v.Item == nil {
		return cardH
	}
	h := 150
	h += 20 * countLines(v.Item.Body, 52)
	switch v.Item.Kind {
	case proto.KindChoice:
		h += 44 * rowsFor(len(v.Item.Options))
	case proto.KindConfirm:
		h += 44
	case proto.KindText, proto.KindSecret:
		h += 90
		if v.Item.Multiline {
			h += 60
		}
	case proto.KindForm:
		h += 40*len(v.Item.Fields) + 44
	case proto.KindVeto:
		h += 70
	case proto.KindDiff:
		// The diff pane, its file header, the note field and the two buttons.
		// The pane scrolls, so counting lines past what fits buys nothing: the
		// ceiling below still applies. Past one file the pane opens taller,
		// to match the surface's raised max-height when the file rail is up.
		lines := 20
		if countDiffFiles(v.Item.Diff) > 1 {
			lines = 28
		}
		h += 122 + 16*min(countDiffLines(v.Item.Diff), lines)
	}
	if _, maxH := u.cardGeom(); h > maxH {
		h = maxH
	}
	return h
}

// cardHeight2 is the card's whole opening rectangle: the item's width and the
// height estimated from the item.
func (u *UI) cardHeight2(v daemon.View) (int, int) {
	return u.cardWidthFor(v.Item), u.cardHeight(v)
}

// cardWidthFor is the configured width for everything except a diff. Code is
// read at code width, and past one file the pane also shares the row with the
// file rail; a configured width larger than either still wins.
func (u *UI) cardWidthFor(it *proto.Item) int {
	w, _ := u.cardGeom()
	if it == nil || it.Kind != proto.KindDiff {
		return w
	}
	want := 560
	if countDiffFiles(it.Diff) > 1 {
		want = 780
	}
	return max(w, want)
}

// countDiffFiles counts a patch's file sections the way the card's parser
// does: "diff --git" headers when they exist, bare "+++" headers for plain
// unified output, one file for anything nonempty that has neither.
func countDiffFiles(diff string) int {
	git, plus := 0, 0
	for line := range strings.Lines(diff) {
		if strings.HasPrefix(line, "diff --git ") {
			git++
		} else if strings.HasPrefix(line, "+++ ") {
			plus++
		}
	}
	if git == 0 {
		git = plus
	}
	if git == 0 && strings.TrimSpace(diff) != "" {
		git = 1
	}
	return git
}

func rowsFor(n int) int {
	if n <= 3 {
		return 1
	}
	return (n + 2) / 3
}

// countDiffLines counts a patch's lines. It does not wrap the way countLines
// does: the diff pane is monospaced and scrolls sideways rather than folding, so
// a long line is one line on screen.
func countDiffLines(diff string) int {
	if strings.TrimSpace(diff) == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(diff, "\n"), "\n") + 1
}

func countLines(s string, per int) int {
	if s == "" {
		return 0
	}
	n := 1
	col := 0
	for _, r := range s {
		if r == '\n' {
			n++
			col = 0
			continue
		}
		col++
		if col >= per {
			n++
			col = 0
		}
	}
	if n > 12 {
		n = 12
	}
	return n
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func callerName(c daemon.CallerState) string {
	switch c {
	case daemon.CallerLive:
		return "live"
	case daemon.CallerGone:
		return "gone"
	case daemon.CallerAwaiting:
		return "awaiting"
	}
	return "none"
}

func rgba(hex string) application.RGBA {
	var r, g, b uint8
	if len(hex) == 7 && hex[0] == '#' {
		var v uint64
		for i := 1; i < 7; i++ {
			c := hex[i]
			var d uint64
			switch {
			case c >= '0' && c <= '9':
				d = uint64(c - '0')
			case c >= 'a' && c <= 'f':
				d = uint64(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = uint64(c-'A') + 10
			}
			v = v<<4 | d
		}
		r, g, b = uint8(v>>16), uint8(v>>8), uint8(v)
	}
	return application.RGBA{Red: r, Green: g, Blue: b, Alpha: 255}
}

// placeOn centres a top-level window on the monitor the person is at.
//
// Cards and toasts get this for free: they are resized to fit their content and
// bridge.resize places them in the same breath. The viewer, the artifact and the
// app window open at a configured size and have no resize step to hang it on, so
// they ask once, straight after the map. Without it Mutter chooses, and on two
// monitors what Mutter chooses is the primary - which is how a showcase being
// recorded on the wide screen opened its document and its artifact on the portrait
// one, off camera. The panel resolves its own monitor before its first frame
// (panel.resolve) and progress pins itself to a corner (x11.corner).
func (u *UI) placeOn(w *application.WebviewWindow, width, height int) {
	if u.x == nil || w == nil {
		return
	}
	if xid := xidOf(w.NativeWindow()); xid != 0 {
		u.x.place(xid, width, height, false, 0)
	}
}

// Summon raises and focuses the card deliberately (FR15). This is the one
// path that is allowed to take the keyboard, because the user asked.
func (u *UI) Summon() {
	u.mu.Lock()
	w, x := u.prompt, u.x
	u.mu.Unlock()
	if w == nil || x == nil {
		return
	}
	application.InvokeSync(func() {
		if xid := xidOf(w.NativeWindow()); xid != 0 {
			x.activate(xid)
		}
	})
}

var _ daemon.Presenter = (*UI)(nil)
var _ = json.Marshal
