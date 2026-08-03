package hand

import "testing"

// The rules that decide whether an event is about to land where the script
// aimed it. Every case here is one that would otherwise be either a wrong click
// or a script that refuses to run for no reason.

func TestJudgeChainAcceptsTheTargetItself(t *testing.T) {
	chain := []winInfo{
		{Win: 30, Name: "", Class: ""},             // an inner widget window
		{Win: 20, Name: "agentbox · review board"}, // the client window
		{Win: 10},                  // the reparenting frame
		{Win: 1, IsRootLike: true}, // root
	}
	ok, what := judgeChain(chain, 20)
	if !ok {
		t.Fatalf("the target's own child was refused: %s", what)
	}
	if what != `"agentbox · review board"` {
		t.Errorf("named it %s", what)
	}
}

func TestJudgeChainRefusesAnotherWindow(t *testing.T) {
	chain := []winInfo{
		{Win: 77, Name: "notes.txt", Class: "gedit.Gedit"},
		{Win: 1, IsRootLike: true},
	}
	ok, what := judgeChain(chain, 20)
	if ok {
		t.Fatal("a different window was accepted")
	}
	// The message has to name the thing that was actually there, or the human
	// reading the failure cannot tell what nearly happened.
	if what != `"notes.txt" (gedit.Gedit)` {
		t.Errorf("named it %s", what)
	}
}

func TestJudgeChainAcceptsAMenu(t *testing.T) {
	// A GTK menu is an override-redirect window parented to the root, so the
	// chain never reaches the window that opened it. Refusing these would break
	// every script that opens a menu and clicks an item in it.
	chain := []winInfo{
		{Win: 91, Override: true},
		{Win: 1, IsRootLike: true},
	}
	ok, what := judgeChain(chain, 20)
	if !ok {
		t.Fatalf("a menu was refused: %s", what)
	}
	if what != "a menu or popup" {
		t.Errorf("named it %s", what)
	}
}

func TestJudgeChainAcceptsAModalOfTheTarget(t *testing.T) {
	chain := []winInfo{
		{Win: 55, Name: "Save as", TransFor: 20},
		{Win: 1, IsRootLike: true},
	}
	ok, what := judgeChain(chain, 20)
	if !ok {
		t.Fatalf("the target's own dialog was refused: %s", what)
	}
	if what != `"Save as"` {
		t.Errorf("named it %s", what)
	}
}

func TestJudgeChainRefusesAnotherWindowsModal(t *testing.T) {
	chain := []winInfo{
		{Win: 55, Name: "Quit without saving?", TransFor: 99},
		{Win: 1, IsRootLike: true},
	}
	if ok, _ := judgeChain(chain, 20); ok {
		t.Fatal("a dialog belonging to another window was accepted")
	}
}

func TestJudgeChainOnNothing(t *testing.T) {
	ok, what := judgeChain(nil, 20)
	if ok {
		t.Fatal("an empty chain was accepted")
	}
	if what != "nothing" {
		t.Errorf("named it %s", what)
	}
}

func TestJudgeChainNamesAPopupByItsOwner(t *testing.T) {
	// A combo box popup inside the target: the chain passes through an
	// override-redirect window and then reaches the target itself.
	chain := []winInfo{
		{Win: 81, Override: true},
		{Win: 20, Name: "agentbox · settings"},
		{Win: 1, IsRootLike: true},
	}
	ok, what := judgeChain(chain, 20)
	if !ok {
		t.Fatalf("the target's own popup was refused: %s", what)
	}
	if what != `a menu or popup of "agentbox · settings"` {
		t.Errorf("named it %s", what)
	}
}

func TestWinInfoLabel(t *testing.T) {
	cases := []struct {
		in   winInfo
		want string
	}{
		{winInfo{Win: 5, Name: "a", Class: "b"}, `"a" (b)`},
		{winInfo{Win: 5, Name: "a"}, `"a"`},
		{winInfo{Win: 5, Class: "b"}, "b"},
		{winInfo{Win: 5}, "an unnamed window (0x5)"},
	}
	for _, c := range cases {
		if got := c.in.label(); got != c.want {
			t.Errorf("label of %+v = %s, want %s", c.in, got, c.want)
		}
	}
}
