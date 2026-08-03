package webui

import (
	"io/fs"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Bridge is the only thing the webview can call. Every method is a verb the
// daemon already understands, so the surface can express exactly what a user
// can do to an item and nothing else - no store access, no file paths, no
// arbitrary exec. The daemon keeps owning policy; this is a keyhole.
type Bridge struct {
	ui *UI
}

// Theme is how a window gets its tokens before it paints. Wails serves
// Options.Flags over a runtime call rather than injecting them into the page, so
// a surface cannot read the theme synchronously off the window - it asks here,
// and after that SetTheme pushes agentbox:theme on every config change.
func (b *Bridge) Theme() Theme {
	b.ui.mu.Lock()
	defer b.ui.mu.Unlock()
	return b.ui.theme
}

// Ready says a surface has mounted. Only the prompt surfaces gate on it: the
// first view is pushed rather than pulled, so emitting before the bundle has run
// would drop it. Every other surface pulls its own state on mount, so its Ready
// is a log line and nothing more - and must not be mistaken for the card's,
// which would let a card window miss its first item.
func (b *Bridge) Ready(surface string) {
	b.ui.log.Debug("webui.surface_ready", "component", "webui", "surface", surface)
	if surface == "card" || surface == "toast" {
		b.ui.markReady()
	}
}

func (b *Bridge) Answer(id, label string) { b.ui.res.Answer(id, label) }
func (b *Bridge) Reply(id, text string)   { b.ui.res.Reply(id, text) }
func (b *Bridge) Defer(id string)         { b.ui.res.Defer(id) }
func (b *Bridge) Dismiss(id string)       { b.ui.res.Dismiss(id) }
func (b *Bridge) Undo(id string)          { b.ui.res.Undo(id) }
func (b *Bridge) Veto(id string)          { b.ui.res.Veto(id) }
func (b *Bridge) Secret(id, value string) { b.ui.res.Secret(id, value) }

func (b *Bridge) AnswerForm(id string, values map[string]string) { b.ui.res.AnswerForm(id, values) }
func (b *Bridge) RunAction(id string, index int)                 { b.ui.res.RunAction(id, index) }
func (b *Bridge) Review(id string, approved bool, comment string) {
	b.ui.res.Review(id, approved, comment)
}

// Confirm keeps the yes/no vocabulary out of the frontend: the daemon expects
// the label, the surface should not have to know which string that is.
func (b *Bridge) Confirm(id string, yes bool) {
	if yes {
		b.ui.res.Answer(id, "yes")
		return
	}
	b.ui.res.Answer(id, "no")
}

// Fit is how a frameless card gets the right height. Guessing from the item
// (title length, option count, body lines) is always a little wrong, and on a
// window with no decoration a wrong height reads as a bug: either the body is
// clipped or there is dead space under the buttons. So the surface measures
// itself after layout and tells us.
//
// The window also has to be re-placed after a resize, because its position is
// ours (a card stays centered on both axes as it grows, a toast grows downward
// from its inset) and the WM will not do that for us.
func (b *Bridge) Fit(height int) {
	b.ui.mu.Lock()
	w, kind := b.ui.prompt, b.ui.promptKind
	b.ui.mu.Unlock()
	if w == nil || height <= 0 {
		return
	}
	// Width follows the item, not just the config: a diff card opens wider
	// (cardWidthFor) and a resize must not snap it back.
	_, cap := b.ui.cardGeom()
	width := b.ui.cardWidthFor(b.ui.currentView().Item)
	if kind == "toast" {
		width, cap, _ = b.ui.toastGeom()
	}
	if height > cap {
		height = cap
	}
	application.InvokeSync(func() {
		// Width matters too: a reused window switching between a diff card
		// and a regular one changes width at equal height, and skipping that
		// resize leaves a 470px layout in a 780px window.
		curW, cur := w.Size()
		if abs(cur-height) < 3 && curW == width {
			return
		}
		w.SetSize(width, height)
		if b.ui.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				// A toast is laid out by the column, not by itself: its new height
				// moves whatever is under it, and the strip above it must not be
				// covered (FR75). A card is centred, as it always was.
				if kind == "toast" {
					b.ui.top.put("toast", xid, width, height, false)
				} else {
					_, _, inset := b.ui.toastGeom()
					b.ui.x.place(xid, width, height, false, inset)
				}
			}
		}
	})
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Copy puts the whole item on the clipboard in a form an agent can be pasted
// (FR43).
func (b *Bridge) Copy(id string) {
	b.ui.mu.Lock()
	v := b.ui.cur
	b.ui.mu.Unlock()
	if v.Item == nil || v.Item.ID != id {
		return
	}
	text := v.Item.Title
	if v.Item.Body != "" {
		text += "\n\n" + v.Item.Body
	}
	application.InvokeSync(func() { b.ui.app.Clipboard.SetText(text) })
}

// fsSub is here rather than in webui.go so the embed plumbing stays next to
// the only other place that touches the frontend bundle.
func fsSub(f fs.FS, dir string) (fs.FS, error) { return fs.Sub(f, dir) }

// --- session surface (FR49) -------------------------------------------------

// Sessions returns the current switcher + selected conversation. The surface
// calls this once on mount; after that the daemon pushes agentbox:sessions.
func (b *Bridge) Sessions() []wireSession { return b.ui.sess.snapshot() }

func (b *Bridge) NewSession(cwd, mode string) (string, error) { return b.ui.sess.Start(cwd, mode) }
func (b *Bridge) SelectSession(id string)                     { b.ui.sess.Select(id) }
func (b *Bridge) SendPrompt(id, prompt string) error          { return b.ui.sess.Send(id, prompt) }
func (b *Bridge) StopSession(id string)                       { b.ui.sess.Stop(id) }

// CloseSession ends a session and removes its row. The surface asks before
// calling this - a mis-click here kills a running agent - so there is no
// confirmation on this side. The conversation is saved on the way out and can be
// reopened from SavedSessions.
func (b *Bridge) CloseSession(id string) { b.ui.sess.Close(id) }

// RenameSession labels a session so a human can find it again. Empty gives the
// name back to the automatic one - Claude's own first words.
func (b *Bridge) RenameSession(id, name string) { b.ui.sess.Rename(id, name) }

// SetSessionMode switches a session between plan and full. It replaces the child
// (--permission-mode is a spawn-time flag) and returns the session id, which is
// deliberately the same one: the conversation, its place in the switcher and
// anything waiting to be answered in it all survive.
func (b *Bridge) SetSessionMode(id, mode string) (string, error) {
	return b.ui.sess.SetMode(id, mode)
}

// wirePastSession is one reopenable conversation, for the Load list.
type wirePastSession struct {
	Path    string `json:"path"`
	When    string `json:"when"`    // "Jul 25 14:07"
	Title   string `json:"title"`   // the project it was in
	Preview string `json:"preview"` // the first thing the human asked
	Turns   int    `json:"turns"`
	Mode    string `json:"mode,omitempty"`
	Resume  bool   `json:"resume"` // true when the child can carry on, not just be read
}

// SavedSessions lists conversations on disk, newest first.
func (b *Bridge) SavedSessions() []wirePastSession {
	saved := b.ui.sess.Saved()
	out := make([]wirePastSession, 0, len(saved))
	for _, s := range saved {
		out = append(out, wirePastSession{
			Path:    s.Path,
			When:    s.SavedAt.Format("Jan 2 15:04"),
			Title:   s.Meta.Title,
			Preview: s.Preview,
			Turns:   s.Turns,
			Mode:    s.Meta.Mode,
			Resume:  s.Meta.SessionID != "",
		})
	}
	return out
}

// ReopenSession puts a saved conversation back on screen and resumes its child, so
// it remembers the conversation rather than merely displaying it.
func (b *Bridge) ReopenSession(path string) (string, error) { return b.ui.sess.Reopen(path) }

// BumpFontSize nudges [font] size_pt and writes it to the config file, so the
// panel can offer the one setting anybody wants while reading without opening the
// settings surface. Returns the size in effect afterwards.
func (b *Bridge) BumpFontSize(delta float64) float64 {
	const lo, hi = 9, 24
	want := b.ui.conf().Font.SizePt + delta
	if want < lo {
		want = lo
	}
	if want > hi {
		want = hi
	}
	b.ui.saveSettings(map[string]string{"font.size_pt": strconv.FormatFloat(want, 'f', -1, 64)})
	return b.ui.conf().Font.SizePt
}

// HidePanel rolls the drop-down panel back up (Esc, or its own button). Show and
// Toggle are not exposed to the webview: nothing inside a hidden panel can ask to
// be shown, and the hotkey and `agentbox panel` own that direction.
func (b *Bridge) HidePanel() { b.ui.HidePanel() }

// AskKey applies one keystroke to the question a conversation is answering in
// place (FR49). Same arrangement as Triage: the surface sends the key, Go decides
// what it means - through the same table the inbox uses - and says whether it
// meant anything. The clicks need no new verb; the panel's buttons call Answer,
// Dismiss and RunAction like every other surface.
func (b *Bridge) AskKey(id, key string) bool { return b.ui.sess.askKey(id, key) }

// Home is where the working-directory picker starts.
func (b *Bridge) Home() string { return homeDir() }

// --- inbox + history surfaces (FR10/FR34/FR35) ------------------------------

// Inbox is called once on mount; after that the daemon pushes agentbox:inbox on
// every queue change.
func (b *Bridge) Inbox() wireInbox { return b.ui.inbox.snapshot() }

// Promote summons a pending item's card (a row click, FR10).
func (b *Bridge) Promote(id string) {
	if src := b.ui.source(); src != nil {
		src.Promote(id)
	}
}

// Triage applies one keystroke to one pending item (FR34). The surface sends the
// key, not a decision: which key answers what is agentbox's vocabulary, shared with
// the card, and it lives in Go so the two cannot drift. Reports whether the key
// meant anything for that item, so the surface can ignore it silently.
func (b *Bridge) Triage(id, key string) bool { return b.ui.inbox.act(id, key) }

// CopyItem puts an inbox row on the clipboard in agent-pasteable form (FR43).
// Takes an id rather than text so the surface cannot put anything on the
// clipboard that agentbox is not already holding.
func (b *Bridge) CopyItem(id string) {
	text := b.ui.inbox.clipText(id)
	if text == "" {
		return
	}
	application.InvokeSync(func() { b.ui.app.Clipboard.SetText(text) })
}

// Stats serves the history surface for one window ("24h", "7d", "30d", "all").
func (b *Bridge) Stats(window string) wireStats { return b.ui.statsFor(window) }

// --- settings surface -------------------------------------------------------

// Settings describes every knob and its current value, read fresh from the file
// on disk. The descriptor table lives in Go so the surface renders controls
// without knowing what any of them mean.
func (b *Bridge) Settings() wireSettings { return b.ui.settings() }

// PreviewTheme resolves the tokens a set of pending values would produce,
// without writing anything, so the preview panel is the real resolver rather
// than a palette copied into JavaScript.
func (b *Bridge) PreviewTheme(values map[string]string) Theme { return b.ui.previewTheme(values) }

// SaveSettings writes the keys whose values actually changed and reports exactly
// which lines it wrote. Everything else in the file - comments, formatting,
// untouched keys, unmaterialised defaults - is left alone.
func (b *Bridge) SaveSettings(values map[string]string) wireSaved {
	return b.ui.saveSettings(values)
}

// --- rendered markdown ------------------------------------------------------

// OpenURL hands a link to the desktop. It exists because a click on an <a> inside
// a webview navigates the window itself, and a frameless card with no back button
// that has become a web page is a card the user cannot answer. The scheme is
// checked here: this is the one place a surface can name something outside agentbox,
// so it may name a web page and nothing else - no file://, no scheme a helper
// might turn into an execution.
func (b *Bridge) OpenURL(raw string) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		b.ui.log.Warn("webui.open_url_rejected", "component", "webui", "err", err.Error())
		return
	}
	switch u.Scheme {
	case "http", "https", "mailto":
	default:
		b.ui.log.Warn("webui.open_url_rejected", "component", "webui", "scheme", u.Scheme)
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				b.ui.log.Error("panic", "component", "webui", "where", "open_url", "panic", r)
			}
		}()
		if err := exec.Command("xdg-open", u.String()).Start(); err != nil {
			b.ui.log.Warn("webui.open_url_failed", "component", "webui", "err", err.Error())
		}
	}()
}

// --- artifacts (M10) --------------------------------------------------------

// ArtifactEvent is the one way out of an artifact's sandbox. window.agentbox.emit
// posts to the surface, the surface checks the shape and hands it here, and this
// is where it becomes something an agent can be waiting on.
//
// Everything about it is bounded, because the sender is agent-authored code
// running in an opaque origin: a name that is short and plain, and a payload that
// is real JSON and small. The frontend checks the same things (artifact.svelte.js)
// and that is fine - the surface checks so it can drop a message quietly, and Go
// checks because Go is the side that must not be talked into anything.
func (b *Bridge) ArtifactEvent(artifactID, name, dataJSON string) {
	ev, ok := artifactEvent(artifactID, name, dataJSON)
	if !ok {
		b.ui.log.Warn("webui.artifact_event_rejected", "component", "webui",
			"artifact", trim(artifactID, 64), "event", trim(name, 64), "bytes", len(dataJSON))
		return
	}
	b.ui.log.Debug("webui.artifact_event", "component", "webui",
		"artifact", ev.ArtifactID, "event", ev.Name, "bytes", len(ev.Data))
	b.ui.res.ArtifactEvent(ev)
}

// CopyText backs the copy button on a code block. It is the one clipboard path
// that takes text rather than an id, and it is safe for the same reason the button
// is honest: the text is a block the daemon rendered and the user is looking at,
// copied because they clicked it.
func (b *Bridge) CopyText(text string) {
	if text == "" {
		return
	}
	application.InvokeSync(func() { b.ui.app.Clipboard.SetText(text) })
}

// --- viewer + progress (FR36-38 / FR21) -------------------------------------

// Document is the open document, rendered. Called on mount; after that the
// daemon pushes agentbox:doc, including on every reload of a watched file. The
// surface never learns a path it could read - it receives HTML that Go produced
// from a path the daemon was asked for.
func (b *Bridge) Document() wireDoc { return b.ui.view.snapshot() }

// Progress is the live report set (FR21), same arrangement: pull on mount, then
// agentbox:progress on every update.
func (b *Bridge) Progress() []wireReport { return b.ui.prog.snapshot() }

// FitProgress sizes the progress window to its content, the way Fit does for a
// card - one report is a strip, four are a panel, and a frameless window has to
// be exactly as tall as what is in it.
func (b *Bridge) FitProgress(height int) { b.ui.prog.fit(height) }

// Surface lets the frontend ask for a different surface (rail clicks stay in
// the frontend, but the tray and the CLI route through here).
func (b *Bridge) Surface(name string) { b.ui.emit("agentbox:surface", name) }

// MinimiseApp / CloseApp back the custom title bar, since a frameless window
// has no buttons of its own.
func (b *Bridge) MinimiseApp() {
	b.ui.mu.Lock()
	w := b.ui.appWin
	b.ui.mu.Unlock()
	if w != nil {
		application.InvokeSync(func() { w.Minimise() })
	}
}

func (b *Bridge) HideApp() { b.ui.ShutdownApp() }
