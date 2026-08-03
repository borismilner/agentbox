package webui

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The app window: agentbox as a desktop application rather than a series of
// transient cards. One window, five surfaces (session, inbox, history,
// viewer, settings), switched in the frontend - the Go side only opens it,
// raises it, and tells it which surface to land on.

// ShowApp opens or raises the main window on the given surface
// ("" = home, then "home", "session", "inbox", "history", "viewer", "library",
// "settings").
func (u *UI) ShowApp(tab string) {
	if tab == "" {
		// Home, not Session (FR81). Session is empty until you start one, so
		// opening on it meant agentbox's first screen was a blank column and a New
		// button. A caller that wants a particular surface still names it -
		// `agentbox inbox` and the tray both do.
		tab = "home"
	}

	u.mu.Lock()
	w := u.appWin
	u.mu.Unlock()

	if w != nil {
		u.onMain("app.raise", func() {
			w.Show()
			w.Focus()
		})
		u.emit("agentbox:surface", tab)
		// The window may have been HIDDEN rather than closed (tray toggle), and
		// showing it again is a state change the tray has to hear about or its
		// menu item goes on offering to show what is already on screen.
		u.appShown(true)
		return
	}

	aw, ah := u.appGeom()
	u.onMain("app", func() {
		w := u.app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name: "agentbox-app",
			// Not bare "agentbox": that title is the card's, and a driver aiming
			// "=agentbox" at a card must not hit this window when both are open
			// (Choose prefers the bigger match, which is always this one).
			Title:     "agentbox · app",
			Width:     aw,
			Height:    ah,
			MinWidth:  880,
			MinHeight: 560,
			// Frameless because the title bar is part of the design (the live
			// daemon dot lives there); the frontend marks its own drag region
			// with --wails-draggable.
			Frameless:        true,
			URL:              "/?surface=app&tab=" + tab,
			BackgroundType:   application.BackgroundTypeSolid,
			BackgroundColour: rgba(u.themeGround()),
		})
		u.mu.Lock()
		u.appWin = w
		u.mu.Unlock()

		w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			u.mu.Lock()
			u.appWin = nil
			u.mu.Unlock()
			u.appShown(false)
			// A question being answered in a conversation loses its surface when
			// this window goes; it gets a card instead (inline.go). Off this
			// goroutine because that path resizes and places a window, and this one
			// is the main loop closing one.
			go u.rerouteAsk()
		})

		w.Show()
		w.Focus()
		u.placeOn(w, aw, ah)
		// The Title option does not survive framelessness (x11.go setName);
		// write the name onto the mapped window, as every other surface does.
		if u.x != nil {
			if xid := xidOf(w.NativeWindow()); xid != 0 {
				u.x.setName(xid, "agentbox · app")
			}
		}

		u.appShown(true)
	})
}

// ToggleApp and AppOpen keep the tray's show/hide item honest.
//
// The state it toggles is VISIBILITY, not existence, and that distinction is the
// whole bug this replaces (FR79). Hide() does not close a window, so it fires no
// WindowClosing: appWin stayed non-nil, the tray was never told, and the item
// went on saying "Hide agentbox" and hiding an already hidden window. From the panel
// the tray had exactly one working action - hide - which is what Boris reported.
func (u *UI) ToggleApp() {
	u.mu.Lock()
	w := u.appWin
	shown := u.appShow
	u.mu.Unlock()

	if w != nil && shown {
		application.InvokeSync(func() { w.Hide() })
		u.appShown(false)
		return
	}
	u.ShowApp("")
}

// AppOpen is what the tray labels its item from: on screen, not merely alive.
func (u *UI) AppOpen() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.appWin != nil && u.appShow
}

// appShown records the window's visibility and tells the tray, from whichever
// path changed it - the tray item, `agentbox app`, the window's own close button.
func (u *UI) appShown(on bool) {
	u.mu.Lock()
	if u.appShow == on {
		u.mu.Unlock()
		return
	}
	u.appShow = on
	hook := u.OnAppChange
	u.mu.Unlock()
	if hook != nil {
		hook(on)
	}
}

// ShutdownApp closes the main window without ending the process, so a tray
// quit is the only thing that kills live sessions.
func (u *UI) ShutdownApp() {
	// Every live conversation to disk before anything is closed. A daemon restart
	// (a deploy, a reboot, `make restart-daemon`) is the ordinary case, and it must
	// not be how a conversation is lost: they come back in the Load list.
	if u.sess != nil {
		u.sess.SaveAll()
	}

	u.mu.Lock()
	w := u.appWin
	u.appWin = nil
	u.mu.Unlock()
	u.appShown(false)
	if w != nil {
		application.InvokeSync(func() { w.Close() })
	}
	// Idempotent with the WindowClosing hook: whichever runs first re-presents the
	// pending ask and clears it, and the other finds nothing to do.
	go u.rerouteAsk()
}

func (u *UI) themeGround() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.theme.Ground
}

// themeWarn is the amber every hands-off surface is painted in. One window opens
// on it rather than on the ground: the FR74 marker is four pixels of solid colour,
// so a ground-coloured first frame would be the whole window flashing dark.
func (u *UI) themeWarn() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.theme.Warning
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
