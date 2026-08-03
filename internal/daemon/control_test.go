package daemon

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

func newTestControl() (*control, *stripSpy) {
	c := newControl(slog.New(slog.NewTextHandler(io.Discard, nil)))
	spy := &stripSpy{}
	c.SetSurface(spy.show, spy.hide)
	return c, spy
}

// stripSpy stands in for the window. What matters is not how it paints but when
// it is on screen at all: presence is the whole signal this feature carries.
type stripSpy struct {
	mu     sync.Mutex
	states []ControlState
	hides  int
}

func (s *stripSpy) show(st *ControlState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states = append(s.states, *st)
}

func (s *stripSpy) hide() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hides++
}

func (s *stripSpy) last() (ControlState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.states) == 0 {
		return ControlState{}, false
	}
	return s.states[len(s.states)-1], true
}

func (s *stripSpy) hidden() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hides
}

var agentA = proto.Identity{Agent: "claude-code", Project: "agentbox"}
var agentB = proto.Identity{Agent: "chrome-driver", Project: "web"}

func TestControlSilenceGrantsAndTheStripStaysUp(t *testing.T) {
	// Silence is consent - the shape that fits "I am about to do the obvious
	// thing". The human only has to act to stop it, and the strip that asked is
	// the same strip that then reports, so nothing flickers between the two.
	c, spy := newTestControl()
	res := c.Request(context.Background(), agentA, "clicking through the board", 80*time.Millisecond)
	if !res.Granted || res.Denied {
		t.Fatalf("silence did not grant: %+v", res)
	}
	if res.State != proto.ControlDriving {
		t.Errorf("granted but state is %q", res.State)
	}
	st, ok := spy.last()
	if !ok {
		t.Fatal("nothing was ever painted")
	}
	if st.State != proto.ControlDriving {
		t.Errorf("the strip is painted %q after being granted", st.State)
	}
	if spy.hidden() != 0 {
		t.Error("the strip came off the screen between asking and driving")
	}
}

func TestControlAskingPaintsTheReasonAndTheCountdown(t *testing.T) {
	// While asking, the human needs the reason and how long he has: the reason is
	// what he is deciding about, and the countdown is the difference between "act
	// now" and "read it later".
	c, spy := newTestControl()
	done := make(chan struct{})
	go func() {
		c.Request(context.Background(), agentA, "taking the mouse for ~30s", 2*time.Second)
		close(done)
	}()

	waitFor(t, "the strip to be painted", func() bool { _, ok := spy.last(); return ok })
	st, _ := spy.last()
	if st.State != proto.ControlAsking {
		t.Errorf("first paint is %q, wanted asking", st.State)
	}
	if st.Reason != "taking the mouse for ~30s" {
		t.Errorf("reason painted as %q", st.Reason)
	}
	if st.LeftMs <= 0 || st.WindowMs <= 0 {
		t.Errorf("no countdown to paint: left=%dms window=%dms", st.LeftMs, st.WindowMs)
	}
	if st.Identity.Agent != agentA.Agent {
		t.Errorf("the strip does not say who is asking: %+v", st.Identity)
	}
	c.Deny(st.ID)
	<-done
}

func TestControlDenyReleasesTheCallerAndClearsTheScreen(t *testing.T) {
	// Deny lives on the strip rather than in a card beside it, so the answer
	// happens where the question is.
	c, spy := newTestControl()
	var res proto.ControlResult
	done := make(chan struct{})
	go func() { res = c.Request(context.Background(), agentA, "driving", 10*time.Second); close(done) }()

	waitFor(t, "the strip to ask", func() bool { _, ok := spy.last(); return ok })
	st, _ := spy.last()
	c.Deny(st.ID)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deny did not release the blocked request")
	}
	if !res.Denied || res.Granted {
		t.Errorf("denied request reported %+v", res)
	}
	if spy.hidden() == 0 {
		t.Error("the strip stayed on screen after a denial, which reads as hands off")
	}
	if c.State().Live {
		t.Error("a denied run is still live")
	}
}

func TestControlReleaseTakesTheStripOffTheScreen(t *testing.T) {
	// The rule Boris set: "once it's gone, I know the hands-off is over and I can
	// work freely". So release must hide it, not just change what it says.
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 40*time.Millisecond)
	if !c.State().Live {
		t.Fatal("granted run is not live")
	}
	c.Release(agentA)
	if c.State().Live {
		t.Error("released run is still live")
	}
	if spy.hidden() == 0 {
		t.Error("release left the strip on screen")
	}
}

func TestControlRefusesASecondAgentRatherThanQueueing(t *testing.T) {
	// The desktop is one resource and several agents reach this daemon. A queue
	// would grant the mouse minutes after it was asked for, when the agent has
	// moved on - and two agents each believing they hold the pointer is the exact
	// failure this feature exists to prevent.
	c, _ := newTestControl()
	c.Request(context.Background(), agentA, "driving the board", 30*time.Millisecond)

	before := time.Now()
	res := c.Request(context.Background(), agentB, "driving debug chrome", 10*time.Second)
	if took := time.Since(before); took > time.Second {
		t.Errorf("the second agent was queued for %v instead of refused", took)
	}
	if res.Granted {
		t.Fatal("two agents were both granted the desktop")
	}
	if res.HeldBy != agentA.Agent {
		t.Errorf("refusal does not name the holder: %+v", res)
	}
	if res.Reason != "driving the board" {
		t.Errorf("refusal does not say what the holder is doing: %q", res.Reason)
	}
	if res.Denied {
		t.Error("being refused for a busy desktop must not read as the human saying no")
	}
}

func TestControlOnlyTheHolderCanWriteTheActivityLine(t *testing.T) {
	// Otherwise a second agent could put words in the driver's mouth, and the human
	// would be reading a line that does not describe what is moving his pointer.
	c, _ := newTestControl()
	c.Request(context.Background(), agentA, "driving", 30*time.Millisecond)
	c.Activity(agentA, "clicking the takeaway")

	res := c.Activity(agentB, "formatting your disk")
	if res.HeldBy != agentA.Agent || res.Granted {
		t.Errorf("a stranger's activity update was accepted: %+v", res)
	}
	if got := c.State().Activity; got != "clicking the takeaway" {
		t.Errorf("the activity line reads %q", got)
	}

	// Same for release: a stranger must not be able to hand the desktop back.
	if res := c.Release(agentB); res.HeldBy != agentA.Agent {
		t.Errorf("a stranger released somebody else's run: %+v", res)
	}
	if !c.State().Live {
		t.Error("a stranger's release ended the run")
	}
}

func TestControlActivityResetsItsAgeSoStuckIsVisible(t *testing.T) {
	// The age is the whole of "nothing is stuck": Boris asked to know "every moment
	// where we are, that nothing is stuck and so on". A line that never changes has
	// to look old.
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 30*time.Millisecond)
	time.Sleep(60 * time.Millisecond)

	c.Activity(agentA, "reading the takeaway aloud")
	st, _ := spy.last()
	if st.Activity != "reading the takeaway aloud" {
		t.Fatalf("activity painted as %q", st.Activity)
	}
	if st.SinceMs > 40 {
		t.Errorf("a fresh line is already %dms old", st.SinceMs)
	}

	time.Sleep(60 * time.Millisecond)
	// Re-sending the SAME line must not look like progress: it is not.
	c.Activity(agentA, "reading the takeaway aloud")
	if st, _ = spy.last(); st.SinceMs < 40 {
		t.Errorf("repeating an unchanged line reset its age to %dms, hiding a stall", st.SinceMs)
	}
}

func TestControlRunDiesWithItsCaller(t *testing.T) {
	// A strip that outlives its agent claims hands-off for a process that is gone,
	// and the human waits on nothing. The connection dying is the run dying.
	c, spy := newTestControl()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var res proto.ControlResult
	go func() { res = c.Request(ctx, agentA, "driving", 10*time.Second); close(done) }()

	waitFor(t, "the strip to ask", func() bool { _, ok := spy.last(); return ok })
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a dead caller left the request blocked")
	}
	if res.Granted {
		t.Error("a caller that went away was granted the desktop")
	}
	if c.State().Live {
		t.Error("the run outlived its caller")
	}
	if spy.hidden() == 0 {
		t.Error("the strip stayed up for an agent that is gone")
	}
}

func TestControlWorksWithNoSurfaceAtAll(t *testing.T) {
	// A headless build has no strip to paint, and the state must still be right -
	// which is also what makes every test above possible.
	c := newControl(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if res := c.Request(context.Background(), agentA, "driving", 20*time.Millisecond); !res.Granted {
		t.Fatalf("headless request was not granted: %+v", res)
	}
	c.Activity(agentA, "working")
	if got := c.State(); !got.Live || got.Activity != "working" {
		t.Errorf("headless state is %+v", got)
	}
	c.Release(agentA)
	if c.State().Live {
		t.Error("headless release left the run live")
	}
}
