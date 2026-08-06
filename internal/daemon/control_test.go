package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
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

// shown counts the paints, for the cases where the claim is that nothing was
// painted at all (FR95 arms recording mode on an empty desktop).
func (s *stripSpy) shown() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.states)
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

func TestControlTwoSameNamedSessionsAreNotEachOther(t *testing.T) {
	// The FR74 defect FR83's session key fixes, and it is not hypothetical: on
	// 2026-08-04 three Claude sessions shared this checkout, and any of them
	// could have written the driver's HANDS OFF line, because ownership was
	// agent-name equality and `Agent` is only the parent process name. Both
	// sessions here are called "claude" in the same project - the identical
	// triple - and differ only by key.
	c, _ := newTestControl()
	first := proto.Identity{Agent: "claude", Project: "agentbox", Key: "aaaa1111"}
	second := proto.Identity{Agent: "claude", Project: "agentbox", Key: "bbbb2222"}

	c.Request(context.Background(), first, "driving", 30*time.Millisecond)
	c.Activity(first, "checking the fullscreen marker")

	if res := c.Activity(second, "formatting your disk"); res.Granted {
		t.Errorf("a same-named second session wrote the holder's activity line: %+v", res)
	}
	if got := c.State().Activity; got != "checking the fullscreen marker" {
		t.Errorf("the strip reads %q, so the wrong session's words reached the human", got)
	}
	if res := c.Release(second); res.HeldBy == "" {
		t.Errorf("a same-named second session released the holder's run: %+v", res)
	}
	if !c.State().Live {
		t.Error("the run ended, so a second session handed back a desktop it never held")
	}

	// And the holder itself is unaffected: same key, so still the same session.
	if res := c.Activity(first, "moving the pointer"); !res.Granted {
		t.Errorf("the holder lost its own run: %+v", res)
	}
}

func TestControlAKeylessCallerStillOwnsItsRunByName(t *testing.T) {
	// A hook script and a Makefile have no key to offer - they are not sessions.
	// Tightening ownership to "key and only key" here would break every keyless
	// caller in order to fix a case they are not part of, so the fallback to
	// agent-name equality is deliberate. The sync primitives are the strict
	// ones; this check is not.
	c, _ := newTestControl()
	keyed := proto.Identity{Agent: "claude", Project: "agentbox", Key: "aaaa1111"}
	keyless := proto.Identity{Agent: "claude", Project: "agentbox"}

	c.Request(context.Background(), keyless, "driving", 30*time.Millisecond)
	if res := c.Activity(keyless, "running make deploy"); !res.Granted {
		t.Errorf("a keyless caller could not write its own activity line: %+v", res)
	}
	// One side keyless falls back to the name, which is today's behaviour.
	if res := c.Activity(keyed, "same name, has a key"); !res.Granted {
		t.Errorf("a keyed caller lost a run held by its own keyless CLI calls: %+v", res)
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

// --- FR94: the pause latch ---------------------------------------------------

func TestPauseKeepsTheRunAndResumeGivesItBack(t *testing.T) {
	// The whole point of the feature: the human takes the desktop back and the
	// run is still there afterwards. A pause that ended the run would be the
	// thing he already had (release), and worse.
	c, spy := newTestControl()
	if res := c.Request(context.Background(), agentA, "driving", 10*time.Millisecond); !res.Granted {
		t.Fatalf("request was not granted: %+v", res)
	}

	res := c.Pause("the hotkey")
	if !res.Paused {
		t.Fatalf("pause did not latch: %+v", res)
	}
	if !res.Live || res.HeldBy != agentA.Agent {
		t.Errorf("the run was lost to the pause: %+v", res)
	}
	st, ok := spy.last()
	if !ok || !st.Paused || !st.Waiting {
		t.Errorf("the strip was not told it is paused with somebody waiting: %+v", st)
	}
	if st.State != proto.ControlDriving {
		t.Errorf("the run's own state changed under the latch: %q", st.State)
	}
	if spy.hidden() != 0 {
		t.Error("the strip came off the screen for a pause; presence must not lapse")
	}

	res = c.Resume("the strip")
	if res.Paused {
		t.Errorf("resume did not clear the latch: %+v", res)
	}
	if !res.Live || res.State != proto.ControlDriving {
		t.Errorf("the run did not come back driving: %+v", res)
	}
	if st, _ := spy.last(); st.Paused {
		t.Error("the strip is still painting paused after a resume")
	}
}

func TestAPausedDesktopParksADriveUntilItIsResumed(t *testing.T) {
	// "Paused must actually stop the input, not just ask nicely." The gate is
	// what a running script asks between steps.
	c, _ := newTestControl()
	c.Pause("the hotkey")

	if blocked, _ := c.Paused(); !blocked {
		t.Fatal("the latch is not on")
	}

	done := make(chan error, 1)
	go func() { done <- c.gate(context.Background(), time.Minute) }()

	select {
	case err := <-done:
		t.Fatalf("the gate let a script through while paused: %v", err)
	case <-time.After(60 * time.Millisecond):
	}

	c.Resume("the strip")
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the gate errored on a resume: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a resume did not wake the parked script")
	}
}

func TestAParkedScriptGivesUpWithoutEndingItsRun(t *testing.T) {
	// The wait has a budget, and running it out must not read as "you lost the
	// desktop" - the run is still the agent's and the strip is still up.
	c, _ := newTestControl()
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Pause("the hotkey")

	err := c.gate(context.Background(), 20*time.Millisecond)
	if !errors.Is(err, errPaused) {
		t.Fatalf("a run-out wait reported %v, want errPaused", err)
	}
	if got := c.State(); !got.Live || got.HeldBy != agentA.Agent || !got.Paused {
		t.Errorf("the run did not survive its own wait running out: %+v", got)
	}
}

func TestAParkedScriptDiesWithItsCaller(t *testing.T) {
	// The connection is the agent. One that went away must not hold a slot behind
	// a latch that could be up for minutes.
	c, _ := newTestControl()
	c.Pause("the hotkey")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.gate(ctx, time.Minute) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("a dead caller got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a dead caller stayed parked")
	}
}

func TestTheLatchHoldsOffASecondAgentsRequest(t *testing.T) {
	// Boris's answer at the mock: pause is desktop-wide, not per-run. A request
	// that arrived while he had the desktop must wait for HIM, not be granted the
	// moment the parked run releases.
	c, _ := newTestControl()
	c.Pause("the hotkey")

	got := make(chan proto.ControlResult, 1)
	go func() {
		got <- c.Request(context.Background(), agentB, "clicking through chrome", 10*time.Millisecond)
	}()

	select {
	case res := <-got:
		t.Fatalf("a paused desktop was handed to a second agent: %+v", res)
	case <-time.After(80 * time.Millisecond):
	}

	c.Resume("the strip")
	select {
	case res := <-got:
		if !res.Granted {
			t.Errorf("the request was not granted after the resume: %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the resume did not release the waiting request")
	}
}

func TestPausingAnIdleDesktopIsLegalAndPaintsIt(t *testing.T) {
	// The pre-emptive "not now": no run, and he wants the desktop kept. It is the
	// one case where the strip is on screen with nobody driving, so Waiting is
	// what tells the surface which sentence to write.
	c, spy := newTestControl()
	res := c.Pause("the hotkey")
	if !res.Paused || res.Live {
		t.Fatalf("an idle pause reported %+v", res)
	}
	st, ok := spy.last()
	if !ok || !st.Paused {
		t.Fatalf("nothing was painted for an idle pause: %+v", st)
	}
	if st.Waiting {
		t.Error("an idle pause claims somebody is waiting on it")
	}

	c.Resume("the strip")
	if spy.hidden() == 0 {
		t.Error("resuming an idle pause left the strip on screen")
	}
}

func TestAReleaseUnderTheLatchLeavesTheLatchUp(t *testing.T) {
	// The agent gives up and releases while he is still paused. The desktop is
	// still HIS, so a strip that vanished here would say "agents may drive again"
	// at the exact moment they may not.
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Pause("the hotkey")
	before := spy.hidden()

	c.Release(agentA)
	if spy.hidden() != before {
		t.Error("a release under the latch took the strip off the screen")
	}
	st, _ := spy.last()
	if !st.Paused || st.Waiting {
		t.Errorf("the strip did not flip to nobody-waiting: %+v", st)
	}
	if got := c.State(); !got.Paused || got.Live {
		t.Errorf("state after a release under the latch is %+v", got)
	}
}

func TestPauseTwiceDoesNotRestartTheClockOrDeadlock(t *testing.T) {
	// The hotkey is a key he can lean on, and the strip's button is one click
	// away from a double click. Neither may reset how long he has been holding
	// the desktop, or the escalation on the surface never arrives.
	c, _ := newTestControl()
	c.Pause("the hotkey")
	time.Sleep(30 * time.Millisecond)
	_, first := c.Paused()
	c.Pause("the hotkey")
	_, second := c.Paused()
	if second < first {
		t.Errorf("a second pause rewound the clock: %v then %v", first, second)
	}
	// And resuming once is enough, however many times it was paused.
	c.Resume("the strip")
	if paused, _ := c.Paused(); paused {
		t.Error("one resume did not clear a doubled pause")
	}
}

func TestAForgottenPauseRaisesOneCardWithSomebodyParked(t *testing.T) {
	// FR94's escalation. The strip has already gone amber, so this is for the
	// human who walked away from the screen - which is why it goes through the
	// item path and chimes rather than being another thing on the strip.
	c, _ := newTestControl()
	type card struct {
		title, body string
		actions     []proto.Action
	}
	got := make(chan card, 4)
	c.SetNag(func(title, body string, a []proto.Action) { got <- card{title, body, a} })

	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Activity(agentA, "clicking through the settings surface")
	c.Pause("the hotkey")
	c.mu.Lock()
	c.pausedAt = time.Now().Add(-pauseNagAfter) // as if it had been held that long
	c.mu.Unlock()
	c.nag()

	select {
	case k := <-got:
		if !strings.Contains(k.title, agentA.Agent) {
			t.Errorf("the card does not name who is parked: %q", k.title)
		}
		if !strings.Contains(k.body, "clicking through the settings surface") {
			t.Errorf("the card does not say what it is parked mid-way through: %q", k.body)
		}
		if len(k.actions) != 1 || !strings.Contains(k.actions[0].Exec, "control resume") {
			t.Errorf("the card's button is %+v, want one that resumes", k.actions)
		}
	default:
		t.Fatal("a three-minute pause with an agent parked raised nothing")
	}
}

func TestAnIdlePauseIsNeverNagged(t *testing.T) {
	// He latched an idle desktop on purpose. Telling him an agent is waiting when
	// none is would be the same lie the strip's warm state was caught telling on
	// screen, and this is the version of it that also makes a noise.
	c, _ := newTestControl()
	raised := 0
	c.SetNag(func(string, string, []proto.Action) { raised++ })

	c.Pause("the hotkey")
	c.nag()
	if raised != 0 {
		t.Errorf("an idle latch raised %d cards", raised)
	}

	// And the same once an agent has released while he held the desktop: nobody
	// is waiting any more, so there is nothing left to be nagged about.
	c.Resume("the strip")
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Pause("the hotkey")
	c.Release(agentA)
	c.nag()
	if raised != 0 {
		t.Errorf("a latch with its run released raised %d cards", raised)
	}
}

func TestResumingBeforeTheNagCancelsIt(t *testing.T) {
	// The ordinary case: he takes the desktop for twenty seconds and hands it
	// back. A card arriving after that would be about nothing.
	c, _ := newTestControl()
	raised := make(chan struct{}, 4)
	c.SetNag(func(string, string, []proto.Action) { raised <- struct{}{} })

	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Pause("the hotkey")
	c.mu.Lock()
	armed := c.nagTimer != nil
	c.mu.Unlock()
	if !armed {
		t.Fatal("pausing with a run parked did not arm the card")
	}

	c.Resume("the strip")
	c.mu.Lock()
	stillArmed := c.nagTimer != nil
	c.mu.Unlock()
	if stillArmed {
		t.Error("the card is still armed after a resume")
	}
	select {
	case <-raised:
		t.Error("a card was raised for a pause that had already ended")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTheNagDoesNotRaceTheActivityLineItQuotes(t *testing.T) {
	// The card quotes what the run was doing, and `activity` is the one field of
	// a run that changes after it is built - the agent keeps rewriting it while
	// parked. Under -race this fails if nag reads it outside the lock.
	c, _ := newTestControl()
	c.SetNag(func(string, string, []proto.Action) {})
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Pause("the hotkey")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 200 {
			c.Activity(agentA, "step "+strconv.Itoa(i))
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			c.nag()
		}
	}()
	wg.Wait()
}

// --- FR95: recording mode -----------------------------------------------------

func TestQuietDemotesTheSignAndLoudPutsItBack(t *testing.T) {
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "walking the settings surface", 10*time.Millisecond)

	if st, _ := spy.last(); st.Quiet {
		t.Fatal("a run starts loud")
	}
	res := c.Quiet("the hotkey")
	if !res.Quiet {
		t.Error("quiet did not report itself")
	}
	if res.QuietLeftS <= 0 {
		t.Errorf("the fuse has no time left: %ds", res.QuietLeftS)
	}
	st, _ := spy.last()
	if !st.Quiet {
		t.Error("the surface was not told to demote the sign")
	}
	// The run is untouched, which is the whole claim: this changes what is on
	// screen and nothing about whose desktop it is.
	if st.State != proto.ControlDriving {
		t.Errorf("quiet changed the run's state to %q", st.State)
	}

	if res := c.Loud("the hotkey"); res.Quiet {
		t.Error("loud did not undo it")
	}
	if st, _ := spy.last(); st.Quiet {
		t.Error("the surface was not told to go loud")
	}
	if spy.hidden() != 0 {
		t.Error("demoting took the strip off the screen instead of repainting it")
	}
}

func TestQuietIsLegalWithNoRunAndPaintsNothing(t *testing.T) {
	// The common case: armed a second before the recording starts, which is
	// usually before any agent has asked for anything. Nothing may appear on
	// screen for it - a strip that came up to announce the sign was quiet would
	// be the joke this feature is not.
	c, spy := newTestControl()
	c.Quiet("the command line")

	if q, _ := c.Quieted(); !q {
		t.Fatal("an idle desktop cannot be armed")
	}
	if spy.shown() != 0 || spy.hidden() != 0 {
		t.Errorf("arming an empty desktop painted something: %d shows, %d hides",
			spy.shown(), spy.hidden())
	}
	// And the run that starts afterwards finds the sign already demoted.
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	if st, _ := spy.last(); !st.Quiet {
		t.Error("a run started under recording mode came up loud")
	}
}

func TestTheFuseGoesLoudOnItsOwn(t *testing.T) {
	// Not persisted AND it expires: the two independent ways a forgotten mode
	// heals. The fuse's real length is half an hour, so the timer is driven
	// directly rather than waited on.
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Quiet("the hotkey")

	c.mu.Lock()
	timer := c.quietTimer
	c.mu.Unlock()
	if timer == nil {
		t.Fatal("quiet armed no fuse, so a forgotten mode would never heal")
	}
	c.Loud("the 30 minute fuse") // what the timer's func does
	if q, _ := c.Quieted(); q {
		t.Error("the fuse did not go loud")
	}
	if st, _ := spy.last(); st.Quiet {
		t.Error("the fuse left the sign demoted on screen")
	}
}

func TestQuietAgainRestartsTheFuseRatherThanDoingNothing(t *testing.T) {
	// A second press is a human saying "still recording", and the useful reading
	// of that is more time, not a no-op.
	c, _ := newTestControl()
	c.Quiet("the hotkey")
	c.mu.Lock()
	c.quietAt = time.Now().Add(-25 * time.Minute)
	first := c.quietTimer
	c.mu.Unlock()

	if _, left := c.Quieted(); left > 6*time.Minute {
		t.Fatalf("the fuse did not age: %s left", left)
	}
	c.Quiet("the hotkey")
	if _, left := c.Quieted(); left < quietFuse-time.Minute {
		t.Errorf("a second press did not restart the fuse: %s left", left)
	}
	c.mu.Lock()
	same := first == c.quietTimer
	c.mu.Unlock()
	if same {
		t.Error("the old fuse is still the live one, so it will fire early")
	}
}

func TestQuietAndPausedAreIndependent(t *testing.T) {
	// Both are properties of the desktop and both can be true: he pauses to say
	// something to camera, mid-recording. Colour carries the pause on four
	// pixels, which is only possible if the state says both.
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	c.Quiet("the hotkey")
	c.Pause("the hotkey")

	st, _ := spy.last()
	if !st.Quiet || !st.Paused {
		t.Errorf("quiet=%v paused=%v, want both", st.Quiet, st.Paused)
	}
	if !st.Waiting {
		t.Error("the parked run was lost")
	}
	// Resuming leaves recording mode alone: the recording did not stop because
	// he took his mouse back.
	c.Resume("the hotkey")
	st, _ = spy.last()
	if st.Paused {
		t.Error("still paused after a resume")
	}
	if !st.Quiet {
		t.Error("a resume also went loud, putting the strip back in the recording")
	}
}

func TestLoudOnALoudDesktopIsNotAnEvent(t *testing.T) {
	c, spy := newTestControl()
	c.Request(context.Background(), agentA, "driving", 10*time.Millisecond)
	before := spy.shown()
	if res := c.Loud("the hotkey"); res.Quiet {
		t.Error("loud on a loud desktop reported quiet")
	}
	if spy.shown() != before {
		t.Error("loud repainted a strip that was already loud")
	}
}
