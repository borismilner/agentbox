package webui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/borismilner/agentbox/internal/proto"
)

// The viewer (FR36-38): `agentbox show FILE|-` and the show_document MCP tool, a
// reading window for a whole document rather than a card's worth of body.
//
// Two things are decided here rather than in the surface. The document itself -
// reading the file, rendering it, and re-reading it when it changes on disk
// (FR37) - because a surface that could read paths would be a surface that can
// read any path. And the window's title, because "agentbox · <what>" has a rule and
// the rule should be testable. Everything about how it reads on screen (the
// measure, find-in-page, zoom, the scroll keys) belongs to the surface.

// wireDoc is one document as the surface receives it.
type wireDoc struct {
	Title string `json:"title"`
	Path  string `json:"path,omitempty"`
	HTML  string `json:"html"`
	Watch bool   `json:"watch"`
	// Artifact drops the reading measure and lets the block have the window: an
	// interactive artifact is a program, not prose, and 700px of it in the middle
	// of a wide window would be a program in a column (M10, artifact.go).
	Artifact bool `json:"artifact,omitempty"`
	// RevMS changes on every reload, so a watched document can re-render without
	// the surface having to diff HTML to know whether to keep the scroll.
	RevMS int64  `json:"revMs"`
	Error string `json:"error,omitempty"`
	Empty bool   `json:"empty"`
}

type viewer struct {
	ui *UI

	mu       sync.Mutex
	req      proto.ShowRequest
	html     string
	artifact bool // what is in html: a sandboxed artifact, or rendered prose
	errText  string
	modTime  time.Time
	rev      time.Time

	win    *application.WebviewWindow
	winGen int // bumps per window, so a watch loop retires with the window it serves
}

func newViewer(ui *UI) *viewer { return &viewer{ui: ui} }

// ShowDocument opens the reading window on req, or reloads the open one
// (daemon.Presenter). A second `agentbox show` should land in the window that is
// already there rather than pile up windows.
func (u *UI) ShowDocument(req proto.ShowRequest) {
	u.view.show(req)
}

func (v *viewer) show(req proto.ShowRequest) {
	v.load(req)
	v.ui.emit("agentbox:doc", v.snapshot())

	v.mu.Lock()
	w := v.win
	v.mu.Unlock()

	if w != nil {
		v.retitle()
		application.InvokeSync(func() {
			w.Show()
			w.Focus()
		})
		return
	}
	v.openWindow()
}

// load reads the source and renders it. A file that cannot be read becomes a
// document saying so: the reader is the only thing on screen, so failing
// silently would leave an empty window and no explanation.
func (v *viewer) load(req proto.ShowRequest) {
	content := req.Content
	errText := ""
	var mod time.Time

	if req.Path != "" {
		data, err := os.ReadFile(req.Path)
		switch {
		case err == nil:
			content = string(data)
			if fi, err := os.Stat(req.Path); err == nil {
				mod = fi.ModTime()
			}
		case content == "":
			errText = err.Error()
			content = "# Cannot open\n\n`" + req.Path + "`\n\n" + err.Error()
		}
	}

	v.mu.Lock()
	v.req = req
	v.html, v.artifact = renderShown(req, content, errText != "")
	v.errText = errText
	v.modTime = mod
	v.rev = time.Now()
	v.mu.Unlock()
}

// renderShown turns a request's content into what the surface paints. An
// artifact is run, a document is read, and a failure is always read: an
// unreadable file becomes a sentence explaining that, and a sentence is prose
// even when the request asked for a program.
func renderShown(req proto.ShowRequest, content string, failed bool) (html string, artifact bool) {
	if req.Artifact && !failed {
		return RenderArtifact(content, req.Title, req.ArtifactID), true
	}
	return RenderMarkdownIn(content, docBase(req)), false
}

// docBase is the directory this document's relative image paths mean, and the
// reader is the only surface that has one. It is the directory of the path the
// request named - resolved client-side, so it is where whoever wrote the document
// was standing - and it is the base even when the file could not be read and the
// caller's own content stood in for it, because that path is still the most
// honest answer available. Content with no path at all gets no base: there is no
// file, and the daemon's working directory is not the caller's, which leaves a
// relative path refused exactly as it was before (images.go).
func docBase(req proto.ShowRequest) string {
	if req.Path == "" {
		return ""
	}
	return filepath.Dir(req.Path)
}

// watch re-reads a watched file when its mtime moves (FR37), so an agent
// iterating on a document turns the viewer into a live preview. Polling keeps it
// self-contained - no daemon-side file watcher, and nothing to unregister. One
// loop belongs to one window and retires with it; while the window shows an
// unwatched document the loop idles rather than exiting, because the next
// `agentbox show --watch` lands in the same window.
func (v *viewer) watch(gen int) {
	defer func() {
		if r := recover(); r != nil {
			v.ui.log.Error("panic", "component", "webui", "window", "viewer", "panic", r)
		}
	}()
	for {
		time.Sleep(500 * time.Millisecond)

		v.mu.Lock()
		retired := v.winGen != gen || v.win == nil
		path, watch, mod := v.req.Path, v.req.Watch, v.modTime
		v.mu.Unlock()
		if retired {
			return
		}
		if !watch || path == "" {
			continue
		}

		fi, err := os.Stat(path)
		if err != nil || !fi.ModTime().After(mod) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		v.mu.Lock()
		if v.winGen != gen || v.req.Path != path {
			v.mu.Unlock()
			continue
		}
		v.html, v.artifact = renderShown(v.req, string(data), false)
		v.errText = ""
		v.modTime = fi.ModTime()
		v.rev = time.Now()
		v.mu.Unlock()

		v.ui.emit("agentbox:doc", v.snapshot())
	}
}

func (v *viewer) snapshot() wireDoc {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.html == "" && v.req.Path == "" && v.req.Content == "" {
		return wireDoc{Empty: true, Title: "agentbox · viewer"}
	}
	return wireDoc{
		Title:    docTitle(v.req),
		Path:     v.req.Path,
		HTML:     v.html,
		Watch:    v.req.Watch && v.req.Path != "",
		Artifact: v.artifact,
		RevMS:    ms(v.rev),
		Error:    v.errText,
	}
}

// docTitle is what the window is called. A title beats a path, a path beats
// nothing, and a bare filename beats a long absolute path in a title bar you
// read at a glance.
func docTitle(req proto.ShowRequest) string {
	switch {
	case strings.TrimSpace(req.Title) != "":
		return "agentbox · " + strings.TrimSpace(req.Title)
	case req.Path != "":
		return "agentbox · " + filepath.Base(req.Path)
	case req.Artifact:
		return "agentbox · artifact"
	}
	return "agentbox · document"
}

// resizeToConfig follows a [window] change while the reader is open: the document
// is the same, the frame it reads in is not.
func (v *viewer) resizeToConfig() {
	v.mu.Lock()
	w := v.win
	v.mu.Unlock()
	if w == nil {
		return
	}
	vw, vh := v.ui.viewerGeom()
	v.ui.onMain("viewer.resize", func() {
		if cw, ch := w.Size(); cw != vw || ch != vh {
			w.SetSize(vw, vh)
		}
	})
}

func (v *viewer) openWindow() {
	vw, vh := v.ui.viewerGeom()
	v.ui.onMain("viewer", func() {
		w := v.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:      "agentbox-viewer",
			Title:     docTitle(v.currentReq()),
			Width:     vw,
			Height:    vh,
			MinWidth:  460,
			MinHeight: 340,
			// Frameless like the app window: the title bar carries the document
			// name, the watch state and the find box, none of which a WM
			// decoration could show.
			Frameless:        true,
			URL:              "/?surface=viewer",
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(v.ui.themeGround()),
		})

		v.mu.Lock()
		v.win = w
		v.winGen++
		gen := v.winGen
		v.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			v.mu.Lock()
			v.win = nil
			v.mu.Unlock()
		})

		go v.watch(gen)

		w.Show()
		w.Focus()
		v.ui.placeOn(w, vw, vh)
		// An artifact is a working surface, not a page: it opens with the whole
		// screen (owner's rule, 2026-07-28, from the review-board mock) and can
		// be un-maximized from there. A window that already exists keeps
		// whatever size the human gave it - only a fresh window presumes.
		if v.currentReq().Artifact {
			w.Maximise()
		}
		v.retitle()
	})
}

func (v *viewer) currentReq() proto.ShowRequest {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.req
}

// retitle keeps the window's own name in step with the document, for the task
// switcher and the tray; the surface paints its title bar from the same string.
// It goes on through X11 because Wails ignores SetTitle on a frameless window
// (x11.go setName).
func (v *viewer) retitle() {
	v.mu.Lock()
	w, x := v.win, v.ui.x
	title := docTitle(v.req)
	v.mu.Unlock()
	if w == nil {
		return
	}
	application.InvokeSync(func() {
		w.SetTitle(title)
		if x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				x.setName(xid, title)
			}
		}
	})
}
