package webui

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/borismilner/agentbox/internal/daemon"
)

// Progress (FR21): a long task reports without asking anything, so it must never
// stand between the user and a question. It gets its own window - opened by the
// first report, closed when the last one finishes, mapped without taking the
// keyboard and pinned to the bottom-right corner, out of the middle of the
// screen where cards land.

const (
	progressH = 132 // opening height; the surface measures itself and calls FitProgress
)

// wireReport is one report as the surface receives it. Percent is clamped and
// the title's fallback is filled in here: a report with no title is still a
// report, and "Working" is what it is.
type wireReport struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Status        string `json:"status,omitempty"`
	Percent       int    `json:"percent"`
	Indeterminate bool   `json:"indeterminate"`
	Agent         string `json:"agent,omitempty"`
	Project       string `json:"project,omitempty"`
	Hue           string `json:"hue"`
}

type progress struct {
	ui *UI

	mu      sync.Mutex
	reports []wireReport
	win     *application.WebviewWindow
}

func newProgress(ui *UI) *progress { return &progress{ui: ui} }

// ShowProgress renders the current set of reports (daemon.Presenter). An empty
// set means every task finished, which closes the window: a progress readout
// with nothing in it is just clutter.
func (u *UI) ShowProgress(reports []daemon.ProgressState) {
	u.prog.set(reports)
}

func (p *progress) set(reports []daemon.ProgressState) {
	rows := encodeReports(reports, p.ui.themeMode() == "dark")

	p.mu.Lock()
	p.reports = rows
	w := p.win
	p.mu.Unlock()

	p.ui.emit("agentbox:progress", rows)

	if len(rows) == 0 {
		if w != nil {
			application.InvokeSync(func() { w.Close() })
		}
		return
	}
	if w == nil {
		p.openWindow(len(rows))
	}
}

func (p *progress) snapshot() []wireReport {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reports == nil {
		return []wireReport{}
	}
	return p.reports
}

// encodeReports is the whole Go-side vocabulary of the surface: the identity
// hue, the title a nameless task wears, and a percentage that cannot be outside
// the bar. The surface draws bars; it does not decide what they mean.
func encodeReports(reports []daemon.ProgressState, dark bool) []wireReport {
	out := make([]wireReport, 0, len(reports))
	for _, r := range reports {
		pct := min(max(r.Percent, 0), 100)
		title := r.Title
		if title == "" {
			title = "Working"
		}
		out = append(out, wireReport{
			ID:            r.ID,
			Title:         title,
			Status:        r.Status,
			Percent:       pct,
			Indeterminate: r.Indeterminate,
			Agent:         r.Identity.Agent,
			Project:       r.Identity.Project,
			Hue:           IdentityHue(r.Identity.Agent, r.Identity.Project, dark),
		})
	}
	return out
}

func (p *progress) openWindow(rows int) {
	pw, _ := p.ui.progressGeom()
	h := p.progressHeight(rows)
	p.ui.onMain("progress", func() {
		w := p.ui.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             "agentbox-progress",
			Title:            "agentbox · progress",
			Width:            pw,
			Height:           h,
			MinWidth:         280,
			MinHeight:        96,
			Frameless:        true,
			Hidden:           true,
			URL:              "/?surface=progress",
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(p.ui.themeGround()),
		})

		p.mu.Lock()
		p.win = w
		p.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			p.mu.Lock()
			p.win = nil
			p.mu.Unlock()
		})

		// A task starting in the background must not take the keyboard off
		// whatever the user is typing into, so the window maps quietly - but it
		// is an ordinary window otherwise: in the taskbar, closable.
		//
		// It is top-most, though. Declining focus is what put it *underneath* a
		// focused fullscreen app (x11.raise explains the Mutter layering), and a
		// bar you cannot see is worse than no bar: the showcase ran one for half a
		// minute behind a fullscreen slide deck and the film shows an empty slide
		// where the demo was.
		if p.ui.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				p.ui.x.quiet(xid)
				showNoActivate(w.NativeWindow())
				p.ui.x.above(xid)
				p.ui.x.corner(xid, pw, h)
				p.ui.x.setName(xid, "agentbox · progress")
				return
			}
		}
		w.Show()
	})
}

// progressHeight is the opening guess; the surface measures itself and calls
// FitProgress, the same arrangement the card uses.
func (p *progress) progressHeight(rows int) int {
	if rows < 1 {
		rows = 1
	}
	h := 44 + 64*rows
	if _, maxH := p.ui.progressGeom(); h > maxH {
		h = maxH
	}
	return h
}

// fit resizes the window to the surface's measured height and keeps it in its
// corner, so a second task appearing grows the window downward-from-the-corner
// rather than drifting across the screen.
func (p *progress) fit(height int) {
	p.mu.Lock()
	w := p.win
	p.mu.Unlock()
	if w == nil || height <= 0 {
		return
	}
	pw, maxH := p.ui.progressGeom()
	if height > maxH {
		height = maxH
	}
	application.InvokeSync(func() {
		_, cur := w.Size()
		if abs(cur-height) < 3 {
			return
		}
		w.SetSize(pw, height)
		if p.ui.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				p.ui.x.corner(xid, pw, height)
			}
		}
	})
}
