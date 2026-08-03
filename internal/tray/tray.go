// Package tray puts agentbox near the clock (FR14): a StatusNotifierItem with
// a small menu and a pending-count state. The tray is progressive
// enhancement; every action here is also reachable via the CLI.
package tray

//go:generate go run ../../tools/genicon icons

import (
	"embed"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/systray"
)

// Only the tray variants are embedded; the larger app-256.png (the .desktop
// icon) lives in the same dir for genicon's sake but is installed by the
// Makefile, not baked into the binary.
//
//go:embed icons/idle.png icons/attn.png icons/urgent.png
var iconFS embed.FS

// Hooks are the actions the menu triggers; all run on tray goroutines.
type Hooks struct {
	ToggleApp func()      // show the agentbox window if hidden, hide it if shown
	AppOpen   func() bool // current window state, for the initial menu label
	ToggleDnd func() bool // returns the new state
	DndState  func() bool
	Quit      func()
}

var (
	pendingCount atomic.Int64
	dndOn        atomic.Bool
	appOpen      atomic.Bool
	ready        atomic.Bool
	pendingItem  *systray.MenuItem
	dndItem      *systray.MenuItem
	showItem     *systray.MenuItem

	mutedMu sync.Mutex
	muted   []string // runtime-muted agents (FR47)
)

func icon(name string) []byte {
	b, _ := iconFS.ReadFile("icons/" + name + ".png")
	return b
}

// Run registers the tray icon and blocks; call on its own goroutine. On
// desktops without a StatusNotifier host this silently does nothing,
// which is the documented degradation (04-platform.md).
func Run(h Hooks) {
	systray.Run(func() {
		systray.SetIcon(icon("idle"))
		systray.SetTitle("agentbox")
		systray.SetTooltip("agentbox: no pending items")

		pendingItem = systray.AddMenuItem("No pending items", "")
		pendingItem.Disable()
		systray.AddSeparator()
		showItem = systray.AddMenuItem("Show AgentBox", "Open or hide the AgentBox window")
		dndItem = systray.AddMenuItemCheckbox("Do not disturb", "Queue silently; urgent breaks through", h.DndState != nil && h.DndState())
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit AgentBox", "Stop the daemon (for maintenance/updates)")

		if h.DndState != nil {
			dndOn.Store(h.DndState())
		}
		ready.Store(true)
		if h.AppOpen != nil {
			SetAppOpen(h.AppOpen())
		}
		render()

		go func() {
			for {
				select {
				case <-showItem.ClickedCh:
					if h.ToggleApp != nil {
						h.ToggleApp()
					}
				case <-dndItem.ClickedCh:
					if h.ToggleDnd != nil {
						SetDnd(h.ToggleDnd())
					}
				case <-quit.ClickedCh:
					h.Quit()
					systray.Quit()
					return
				}
			}
		}()
	}, nil)
}

// SetPending updates the icon and tooltip to reflect how many items wait.
func SetPending(n int) {
	pendingCount.Store(int64(n))
	render()
}

// SetDnd syncs the checkbox when DND flips from anywhere (CLI, config).
func SetDnd(on bool) {
	dndOn.Store(on)
	render()
}

// SetAppOpen relabels the show/hide item to match the window state, which
// changes from the tray, the CLI (`agentbox app`) or the window's own close
// button, so the label stays honest wherever the toggle came from.
func SetAppOpen(open bool) {
	appOpen.Store(open)
	if !ready.Load() || showItem == nil {
		return
	}
	if open {
		showItem.SetTitle("Hide AgentBox")
	} else {
		showItem.SetTitle("Show AgentBox")
	}
}

// SetMuted reflects the runtime-muted agents (FR47) in the tooltip, so the
// tray shows when an agent is being silenced. The list is sorted by the
// daemon.
func SetMuted(agents []string) {
	mutedMu.Lock()
	muted = append(muted[:0:0], agents...)
	mutedMu.Unlock()
	render()
}

// mutedSuffix is the " · N muted (...)" tail appended to the tooltip.
func mutedSuffix() string {
	mutedMu.Lock()
	defer mutedMu.Unlock()
	if len(muted) == 0 {
		return ""
	}
	return fmt.Sprintf(" · %d muted (%s)", len(muted), strings.Join(muted, ", "))
}

func render() {
	if !ready.Load() {
		return
	}
	n := pendingCount.Load()
	dnd := dndOn.Load()
	suffix := mutedSuffix()
	if dnd {
		dndItem.Check()
	} else {
		dndItem.Uncheck()
	}
	switch {
	case n > 0:
		systray.SetIcon(icon("attn"))
		systray.SetTooltip(fmt.Sprintf("agentbox: %d pending%s", n, suffix))
		pendingItem.SetTitle(fmt.Sprintf("%d pending", n))
	case dnd:
		systray.SetIcon(icon("idle"))
		systray.SetTooltip("agentbox: do not disturb" + suffix)
		pendingItem.SetTitle("Do not disturb")
	default:
		systray.SetIcon(icon("idle"))
		systray.SetTooltip("agentbox: no pending items" + suffix)
		pendingItem.SetTitle("No pending items")
	}
}
