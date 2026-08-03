package webui

import "testing"

// FR79. The tray's item is labelled from AppOpen and acts through ToggleApp, and
// the pair used to disagree about what "open" meant: ToggleApp hid the window,
// Hide fires no WindowClosing, so appWin stayed non-nil and nothing was told.
// From the panel that is a tray with one working action - hide - which is what
// Boris reported.
//
// Neither call can be exercised here (both want a real window on the main GTK
// thread), so what is pinned is the state machine underneath them: the flag the
// label reads, and that every change of it reaches the tray exactly once.
func TestAppShownReportsEveryChangeOnce(t *testing.T) {
	var got []bool
	u := &UI{}
	u.OnAppChange = func(open bool) { got = append(got, open) }

	if u.AppOpen() {
		t.Fatal("a UI with no window reports the app as open")
	}

	u.appShown(true)
	u.appShown(true) // a second show of a shown window is not a change
	u.appShown(false)
	u.appShown(false)
	u.appShown(true)

	want := []bool{true, false, true}
	if len(got) != len(want) {
		t.Fatalf("tray heard %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tray heard %v, want %v", got, want)
		}
	}
}

// A hidden window is still a live one - it holds its sessions and its scroll
// position - so existence alone cannot be what the label reads. This is the
// assertion that fails if AppOpen ever goes back to `appWin != nil`.
func TestAppOpenIsVisibilityNotExistence(t *testing.T) {
	u := &UI{}
	u.appWin = nil
	u.appShow = true
	if u.AppOpen() {
		t.Fatal("no window, but the app reports as open")
	}

	// The case that mattered: a window that exists and is hidden.
	u.appShow = false
	if u.AppOpen() {
		t.Fatal("a hidden window reports as open, so the tray offers to hide it again")
	}
}
