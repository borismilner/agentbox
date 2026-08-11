package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

// control is the desktop handover (FR74): one strip on screen for the length of a
// run, which asks for the desktop and then says what is being done with it.
//
// It exists because the announcements were events. `veto` said "I am about to
// take the mouse" once and closed; `notify` said "mouse is yours again" once.
// Between the two there was nothing on screen, so the only way to find out
// whether an agent was still driving was to touch something - which is exactly
// what breaks the run. Three drive sequences died that way in ten minutes before
// this was written.
//
// The strip's presence is the whole signal: on screen means the desktop is the
// agent's, gone means it is the human's. That is why there is no idle state and
// no "working but you may touch things" state - either would make presence
// ambiguous, and presence is the one thing this element cannot afford to blur.
//
// One run at a time, because the desktop is one resource and several agents reach
// this daemon. A second requester is refused with the holder's name rather than
// queued: see proto.ControlResult for why a queue would be worse than a refusal.
type control struct {
	log  *slog.Logger
	show func(*ControlState) // paint or move the strip; nil in tests
	hide func()              // take it off the screen

	mu  sync.Mutex
	run *controlRun

	// The pause latch (FR94). It belongs to the desktop rather than to the run,
	// which is Boris's answer at the mock and the better one: there is only ever
	// one run because there is only one desktop, so a pause that only touched the
	// current run would hand the desktop to the next agent in the queue the moment
	// that run released - while he is still typing. Latched, it parks the live run
	// AND holds off every Request, and it is legal with no run at all.
	paused   bool
	pausedAt time.Time
	// resumed is closed when the human resumes, and replaced on the next pause.
	// Waiters take a copy under the lock and select on it, so a resume wakes
	// everybody parked at once without the daemon tracking who they are.
	resumed chan struct{}
	// nag fires once, pauseNagAfter into a pause with somebody parked behind it.
	// A timer rather than a ticker because a pause is a discrete event with a
	// discrete end: there is nothing to poll for, and a clock running all day to
	// notice something that happens on a state change would be the wrong shape.
	nagTimer *time.Timer
	nagWith  func(title, body string, actions []proto.Action)

	// Recording mode (FR95): the sign demoted to four pixels, so an agent can
	// drive while the screen is being recorded without an internal tool sitting in
	// every frame. Like the latch above it belongs to the desktop and not to a run
	// - he arms it a second before he hits record, which is usually before any
	// agent has asked for anything.
	//
	// It is deliberately NOT persisted, and it expires. Both are the same answer to
	// the one failure this feature can have: a recording mode left on is a
	// hands-off sign nobody can see, and now that the demoted marker can be
	// covered, forgotten-on means no visible sign at all rather than a thin one.
	// So it dies with the daemon AND with quietFuse, and neither needs the human to
	// remember anything.
	quiet      bool
	quietAt    time.Time
	quietTimer *time.Timer
	// onQuiet hears every flip of the mode, so the card column can go quiet with
	// the sign (FR95). A callback rather than the daemon reading Quieted() under
	// its own lock: the fuse flips this from a timer inside control, so the daemon
	// has to be told anyway, and being told is the only version that does not put
	// control's lock underneath d.mu on every arriving item.
	//
	// sinkMu serializes the telling, and it is not c.mu because the sink reaches
	// into the daemon's lock and c.mu must never sit underneath that. Without it
	// the hotkey and the fuse can each release c.mu and then be scheduled in the
	// other order, leaving the daemon holding every card while the sign says loud:
	// a divergence with no symptom on screen and no way back but a restart.
	onQuiet func(quiet bool)
	sinkMu  sync.Mutex
}

// quietFuse is how long recording mode lasts before it goes loud on its own. Half
// an hour covers any take Boris has described and is short enough that a mode left
// on after one heals before the next time an agent drives. A longer recording
// re-arms with one key, which is the cheap side of the trade.
const quietFuse = 30 * time.Minute

// pauseWait is how long an agent parks on a latched desktop before it is told to
// go and do something else. It is deliberately long: a run that dies because its
// human needed the mouse is a worse outcome than a run that waits, and ten
// minutes is past any interruption Boris described when he asked for this. The
// run is not ended when it elapses - only the waiting is.
const pauseWait = 10 * time.Minute

// pauseNagAfter is when a forgotten pause becomes worth a card. The strip has
// already gone amber at two minutes and it is always on screen and always on
// top, so this is not for the human looking at his desktop - it is for the one
// who walked away from it, and it chimes for that reason.
//
// It fires once and never repeats. An agent parked behind a latch is not an
// emergency: it gives up on its own at pauseWait with its run intact, so a
// second and third card would be nagging about a situation that resolves itself.
const pauseNagAfter = 3 * time.Minute

// errPaused is what a parked caller is told when its wait runs out. It says the
// run is still its own, because the failure mode this whole feature exists to
// prevent is an agent concluding it has lost the desktop and starting over.
var errPaused = errors.New("the human paused the desktop and has not resumed it yet; your run is still yours, so wait again or do something else")

// controlRun is one live handover. denied is closed by the human pressing Deny,
// which is what releases the blocked request; grant is closed when the countdown
// elapses or the human allows it early.
type controlRun struct {
	id       string
	identity proto.Identity
	state    string
	reason   string
	activity string
	since    time.Time // when activity last changed, so a stuck run is visible
	deadline time.Time // when asking becomes driving

	denied  chan struct{}
	granted chan struct{}
	once    sync.Once // whichever of the two fires first wins
}

// ControlState is what the strip paints. Age is derived here rather than in the
// surface so a strip that reopens mid-run does not restart the clock.
type ControlState struct {
	ID       string         `json:"id"`
	State    string         `json:"state"`
	Reason   string         `json:"reason,omitempty"`
	Activity string         `json:"activity,omitempty"`
	Identity proto.Identity `json:"identity"`
	SinceMs  int64          `json:"since_ms"`            // ms since the activity last changed
	LeftMs   int64          `json:"left_ms,omitempty"`   // ms left on the countdown, while asking
	WindowMs int64          `json:"window_ms,omitempty"` // the countdown's full length
	// FR94. Paused overrides everything above on the strip: the run's own state
	// stays what it was, because resuming has to put it back, but while this is
	// true the desktop is the human's and the strip says so. PausedMs is derived
	// here for the same reason SinceMs is - a strip that reopens mid-pause must
	// not restart the clock. Waiting is true when a run is parked behind the
	// latch, false when the human latched an idle desktop pre-emptively.
	Paused   bool  `json:"paused,omitempty"`
	PausedMs int64 `json:"paused_ms,omitempty"`
	Waiting  bool  `json:"waiting,omitempty"`
	// FR95. Quiet is recording mode: the surface reads it and puts up the marker
	// instead of the strip, without the notification type or the keeper that make
	// the strip impossible to cover. It changes what is on screen, never what the
	// run is, which is why it sits beside Paused rather than in State.
	Quiet bool `json:"quiet,omitempty"`
}

// Handover exposes the two answers the human can give, and nothing else, so the
// surface cannot reach the rest of the daemon on the way (webui.Handover).
func (d *Daemon) Handover() *control { return d.control }

// SetControlSurface wires the strip's window to the run state. Split from the
// constructor because the UI is built after the daemon.
func (d *Daemon) SetControlSurface(show func(*ControlState), hide func()) {
	d.control.SetSurface(show, hide)
}

func newControl(log *slog.Logger) *control {
	return &control{log: log}
}

// SetSurface wires the window. Kept separate from the constructor because the UI
// is built after the daemon, and because a headless build has no strip to paint -
// the state is still correct there, which is what the tests check.
func (c *control) SetSurface(show func(*ControlState), hide func()) {
	c.mu.Lock()
	c.show, c.hide = show, hide
	c.mu.Unlock()
}

// SetQuietSink wires who hears recording mode flip. Called outside c.mu, like
// show and nagWith, because the daemon reaches back into its own lock.
func (c *control) SetQuietSink(fn func(quiet bool)) {
	c.mu.Lock()
	c.onQuiet = fn
	c.mu.Unlock()
}

// fireQuiet tells the sink what the mode is NOW, one caller at a time. Reading
// the state here rather than passing the value the caller just wrote is the
// second half of the ordering guarantee: whichever flip goes last through this
// gate reads the truth that outlived the race, so the two sides converge instead
// of latching whichever callback happened to be scheduled second.
func (c *control) fireQuiet() {
	c.sinkMu.Lock()
	defer c.sinkMu.Unlock()
	c.mu.Lock()
	quiet, sink := c.quiet, c.onQuiet
	c.mu.Unlock()
	if sink != nil {
		sink(quiet)
	}
}

// Request asks for the desktop and BLOCKS until the human grants it, denies it,
// or the countdown runs out. Silence is consent, which is the shape that fits "I
// am about to do the obvious thing" - the human only has to act to stop it.
//
// ctx is the caller's connection. If it dies while we are asking, the run dies
// with it: a strip that outlives its agent would claim hands-off for a process
// that is gone, and the human would wait on nothing.
func (c *control) Request(ctx context.Context, id proto.Identity, reason string, window time.Duration) proto.ControlResult {
	if window <= 0 {
		window = 20 * time.Second
	}

	// The latch comes first, and it blocks rather than refusing (FR94). Asking a
	// paused human "may I take the desktop?" is the wrong question at the wrong
	// moment: he paused *because* he needed it. So the request parks here, with
	// nothing on his screen, and only starts its countdown once he has resumed.
	if paused, _ := c.Paused(); paused {
		c.log.Info(logging.EvControl, "component", "daemon", "control", "parked",
			"agent", id.Agent, "reason", reason)
		if err := c.gate(ctx, pauseWait); err != nil {
			// Not granted, and the result says why without pretending: Paused is
			// still true, so the caller can tell "he is using his desktop" from
			// "another agent has it" and wait again instead of giving up.
			return c.State()
		}
	}
	now := time.Now()

	c.mu.Lock()
	if c.run != nil {
		held := c.run.identity.Agent
		heldReason := c.run.reason
		c.mu.Unlock()
		c.log.Info(logging.EvControl, "component", "daemon", "control", "refused",
			"agent", id.Agent, "held_by", held)
		return proto.ControlResult{OK: true, Live: true, HeldBy: held, Reason: heldReason}
	}
	run := &controlRun{
		id:       "k" + newID()[1:13],
		identity: id,
		state:    proto.ControlAsking,
		reason:   reason,
		since:    now,
		deadline: now.Add(window),
		denied:   make(chan struct{}),
		granted:  make(chan struct{}),
	}
	c.run = run
	show := c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "asking",
		"agent", id.Agent, "reason", reason, "window_s", int(window.Seconds()))
	if show != nil {
		show(c.snapshot(run, window))
	}

	timer := time.NewTimer(window)
	defer timer.Stop()
	select {
	case <-run.denied:
		c.end(run, "denied")
		return proto.ControlResult{OK: true, Denied: true}
	case <-run.granted:
	case <-timer.C:
		run.once.Do(func() { close(run.granted) })
	case <-ctx.Done():
		// The agent went away mid-question. Nothing to hand over, and the strip
		// must not stay up asking on behalf of a process that is gone.
		c.end(run, "caller gone")
		return proto.ControlResult{OK: true}
	}

	c.mu.Lock()
	if c.run != run {
		c.mu.Unlock() // superseded while we waited
		return proto.ControlResult{OK: true}
	}
	run.state = proto.ControlDriving
	run.since = time.Now()
	show = c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "driving", "agent", id.Agent)
	if show != nil {
		show(c.snapshot(run, 0))
	}
	return proto.ControlResult{OK: true, Live: true, Granted: true, State: proto.ControlDriving}
}

// Activity updates the line the strip shows, and resets its age. Non-blocking:
// an agent narrating its work must never be held up by the narration.
//
// Only the agent holding the run may write to it, so a second agent's stray
// update cannot put words in the driver's mouth.
func (c *control) Activity(id proto.Identity, activity string) proto.ControlResult {
	c.mu.Lock()
	run := c.run
	if run == nil {
		c.mu.Unlock()
		return proto.ControlResult{OK: true}
	}
	if !run.identity.SameSession(id) {
		held := run.identity.Agent
		c.mu.Unlock()
		return proto.ControlResult{OK: true, Live: true, HeldBy: held}
	}
	if activity != "" && activity != run.activity {
		run.activity = activity
		run.since = time.Now()
	}
	show := c.show
	state, act := run.state, run.activity
	c.mu.Unlock()

	if show != nil {
		show(c.snapshot(run, 0))
	}
	return proto.ControlResult{OK: true, Live: true, Granted: true, State: state, Activity: act}
}

// Release ends the run and takes the strip off the screen, which is how the human
// learns the desktop is his again.
func (c *control) Release(id proto.Identity) proto.ControlResult {
	c.mu.Lock()
	run := c.run
	if run == nil {
		c.mu.Unlock()
		return proto.ControlResult{OK: true}
	}
	if !run.identity.SameSession(id) {
		held := run.identity.Agent
		c.mu.Unlock()
		return proto.ControlResult{OK: true, Live: true, HeldBy: held}
	}
	c.mu.Unlock()
	c.end(run, "released")
	return proto.ControlResult{OK: true}
}

// State reports without changing anything, for a surface that just opened and for
// a caller checking whether it still holds the desktop.
func (c *control) State() proto.ControlResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := proto.ControlResult{OK: true}
	if c.paused {
		res.Paused = true
		res.PausedS = int(time.Since(c.pausedAt).Seconds())
	}
	if c.quiet {
		res.Quiet = true
		res.QuietLeftS = int(max(quietFuse-time.Since(c.quietAt), 0).Seconds())
	}
	if c.run == nil {
		return res
	}
	res.Live, res.State = true, c.run.state
	res.Activity, res.HeldBy, res.Reason = c.run.activity, c.run.identity.Agent, c.run.reason
	return res
}

// Pause is the human taking the desktop back mid-run (FR94), from the strip, the
// hotkey or the CLI. It never ends the run: the agent keeps its place, its
// activity line keeps painting, and the moment Resume is called it carries on
// from the step it parked at.
//
// how is what fired it, for the log only - "the hotkey" and "the strip" fail in
// different ways and telling them apart afterwards is worth one string.
func (c *control) Pause(how string) proto.ControlResult {
	c.mu.Lock()
	if c.paused {
		c.mu.Unlock()
		return c.State() // already latched; saying so twice is not an error
	}
	c.paused = true
	c.pausedAt = time.Now()
	c.resumed = make(chan struct{})
	run, show := c.run, c.show
	agent := ""
	if run != nil {
		agent = run.identity.Agent
	}
	c.nagTimer = time.AfterFunc(pauseNagAfter, safely(c.log, "control.nag", c.nag))
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "paused",
		"how", how, "agent", agent)
	if show != nil {
		show(c.snapshot(run, 0))
	}
	return c.State()
}

// Resume hands the desktop back. Only the human can reach it - there is no MCP
// verb for this and there must not be one, or the pause is a suggestion.
//
// A resume with no run left (the agent released or timed out while parked) takes
// the strip off the screen, because presence is still the whole signal.
func (c *control) Resume(how string) proto.ControlResult {
	c.mu.Lock()
	if !c.paused {
		c.mu.Unlock()
		return c.State()
	}
	c.paused = false
	held := time.Since(c.pausedAt)
	if c.resumed != nil {
		close(c.resumed) // wakes everybody parked, at once
		c.resumed = nil
	}
	// Before anything else can observe the resume: a card that arrives a beat
	// after he has already handed the desktop back is worse than no card.
	if c.nagTimer != nil {
		c.nagTimer.Stop()
		c.nagTimer = nil
	}
	run, show, hide := c.run, c.show, c.hide
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "resumed",
		"how", how, "paused_s", int(held.Seconds()))
	switch {
	case run != nil && show != nil:
		show(c.snapshot(run, 0))
	case run == nil && hide != nil:
		hide()
	}
	return c.State()
}

// Quiet demotes the sign for a recording (FR95): the strip comes down and the 4px
// marker takes over, mapped and then left alone so a window over the top edge
// covers it. The guarantee is deliberately weaker in this mode, and it is weaker
// weaker by request - "generally it should live on top of any and
// all surfaces; when demoted for purposes of recording or stuff like that it can be
// overlapped".
//
// Legal with no run at all, which is the common case: it gets armed a second before
// the recording starts, and the agent that asks for the desktop two minutes later
// finds the sign already demoted.
//
// Calling it again restarts the fuse rather than doing nothing. A second press is a
// human saying "still recording", and the useful reading of that is more time.
func (c *control) Quiet(how string) proto.ControlResult {
	c.mu.Lock()
	c.quiet = true
	c.quietAt = time.Now()
	if c.quietTimer != nil {
		c.quietTimer.Stop()
	}
	c.quietTimer = time.AfterFunc(quietFuse, safely(c.log, "control.fuse", func() { c.Loud("the 30 minute fuse") }))
	run, show := c.run, c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "quiet",
		"how", how, "fuse_s", int(quietFuse.Seconds()))
	// Told on every press rather than only on the transition: a second press is
	// more time and not a second demotion, so the daemon's own no-op guard is the
	// right place to notice that, and firing unconditionally is what keeps the two
	// sides converging if they ever disagree.
	c.fireQuiet()
	// Only repaint if there is something on screen to demote. With no run and no
	// latch the screen is already empty, and show(nil) there would put a strip up
	// to say the sign is quiet, which is the joke this feature is not.
	if show != nil && (run != nil || c.isPaused()) {
		show(c.snapshot(run, 0))
	}
	return c.State()
}

// Loud undoes it, from the hotkey, the CLI or the fuse. It is idempotent because
// the fuse and a keypress race by construction: he goes loud as the recording
// stops, a few seconds either side of the timer that was going to do it anyway.
func (c *control) Loud(how string) proto.ControlResult {
	c.mu.Lock()
	if !c.quiet {
		c.mu.Unlock()
		return c.State()
	}
	c.quiet = false
	was := time.Since(c.quietAt)
	if c.quietTimer != nil {
		c.quietTimer.Stop()
		c.quietTimer = nil
	}
	run, show := c.run, c.show
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "loud",
		"how", how, "quiet_s", int(was.Seconds()))
	// Before the repaint, so whatever was held is already on its way to the screen
	// by the time the strip comes back: one visible change, not two.
	c.fireQuiet()
	if show != nil && (run != nil || c.isPaused()) {
		show(c.snapshot(run, 0))
	}
	return c.State()
}

// Quieted reports recording mode and what the fuse has left, without blocking.
// The surface asks this when its window opens, the same way it asks about a run.
func (c *control) Quieted() (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.quiet {
		return false, 0
	}
	return true, max(quietFuse-time.Since(c.quietAt), 0)
}

// isPaused is Paused's boolean half without the duration, for the two places that
// only need to know whether anything is on screen.
func (c *control) isPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

// SetNag wires the card a forgotten pause raises. Separate from the surface
// because it goes through the ordinary item path - it chimes, it queues and it
// lands in history like anything else - while show/hide paint one window
// directly.
func (c *control) SetNag(fn func(title, body string, actions []proto.Action)) {
	c.mu.Lock()
	c.nagWith = fn
	c.mu.Unlock()
}

// nag is the three-minute card. It only speaks when somebody is actually parked:
// a latch he put on an idle desktop is a decision, not a thing that has been
// forgotten, and a card telling him an agent is waiting when none is would be
// the same lie the strip's warm state was caught telling.
func (c *control) nag() {
	// Everything the card needs is copied out under the lock, activity included.
	// It is the one field of a run that changes after construction - the agent
	// rewrites it through Activity while parked - so reading it from the timer's
	// goroutine afterwards would be a genuine race, unlike identity, which never
	// moves.
	c.mu.Lock()
	paused, run, with := c.paused, c.run, c.nagWith
	held := time.Since(c.pausedAt)
	c.nagTimer = nil
	agentName, activity := "", ""
	if run != nil {
		agentName, activity = run.identity.Agent, run.activity
	}
	c.mu.Unlock()
	if !paused || run == nil || with == nil {
		return
	}

	agent := nameOr(agentName, "an agent")
	c.log.Info(logging.EvControl, "component", "daemon", "control", "nagged",
		"agent", agentName, "paused_s", int(held.Seconds()))
	// One button, and it is the one that undoes this. There is deliberately no
	// "end the run" here: the agent gives up on its own at pauseWait and is told
	// its run survived, which is a gentler end than cutting it off mid-sequence,
	// and a button that fires an irreversible thing from a card he may be reading
	// in passing is the wrong shape for a state that resolves itself.
	with(
		fmt.Sprintf("%s has been parked for %s", agent, roundDur(held)),
		fmt.Sprintf("It is waiting on your desktop, part way through a run%s. "+
			"It gives up on its own in %s - the run survives, it just stops waiting. "+
			"Resume whenever you are done; nothing resumes it but you.",
			activitySuffix(activity), roundDur(pauseWait-held)),
		[]proto.Action{{Label: "Resume now", Exec: "agentbox control resume"}},
	)
}

func activitySuffix(activity string) string {
	if activity == "" {
		return ""
	}
	return " (" + activity + ")"
}

// Paused reports the latch and how long it has been on, without blocking. The
// pointer-and-keyboard path calls this between every keystroke, so it stays a
// lock and two reads.
func (c *control) Paused() (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.paused {
		return false, 0
	}
	return true, time.Since(c.pausedAt)
}

// gate blocks while the latch is on and returns when the desktop is free again.
// It is the whole of "the agent has to learn it, and wait rather than fail":
// errPaused after wait, ctx.Err() if the caller went away, nil if it may proceed.
//
// The loop re-reads the latch after each wake rather than trusting one signal,
// because the human can pause again between the resume and this goroutine being
// scheduled, and a caller that drove into that window would be driving during a
// pause - the one thing this must never do.
func (c *control) gate(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		wait = pauseWait
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		c.mu.Lock()
		paused, ch := c.paused, c.resumed
		c.mu.Unlock()
		if !paused {
			return nil
		}
		select {
		case <-ch:
		case <-deadline.C:
			return errPaused
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// holderKey is the session key driving the desktop, or empty (FR83). The roster
// asks this rather than reading the run itself, so control keeps its own lock and
// the roster never reaches inside it.
func (c *control) holderKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.run == nil {
		return ""
	}
	return c.run.identity.Key
}

// Deny is the human pressing the button on the strip. It releases the blocked
// request, which is what makes the strip the place the answer happens rather than
// a card beside it.
func (c *control) Deny(id string) {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil || (id != "" && run.id != id) {
		return
	}
	run.once.Do(func() { close(run.denied) })
}

// Allow is the human granting it early rather than waiting out the countdown.
func (c *control) Allow(id string) {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil || (id != "" && run.id != id) {
		return
	}
	run.once.Do(func() { close(run.granted) })
}

// Snapshot is what the surface asks for when its window opens mid-run.
func (c *control) Snapshot() *ControlState {
	c.mu.Lock()
	run := c.run
	c.mu.Unlock()
	if run == nil {
		return nil
	}
	return c.snapshot(run, 0)
}

// end clears the run and hides the strip, once, whoever got here first.
func (c *control) end(run *controlRun, why string) {
	c.mu.Lock()
	if c.run != run {
		c.mu.Unlock()
		return
	}
	c.run = nil
	hide, show, paused := c.hide, c.show, c.paused
	c.mu.Unlock()

	c.log.Info(logging.EvControl, "component", "daemon", "control", "ended",
		"agent", run.identity.Agent, "why", why)
	// A run ending under a live latch does not clear the screen (FR94). The
	// desktop is still held - by the human, now with nobody waiting on it - and a
	// strip that vanished here would say "agents may drive again" while they may
	// not. It flips to the no-one-waiting wording instead, and only Resume
	// takes it down.
	if paused && show != nil {
		show(c.snapshot(nil, 0))
		return
	}
	if hide != nil {
		hide()
	}
}

// snapshot reads the run under the lock the caller may or may not hold, so it
// takes its own copy of everything the surface needs. window is only passed while
// asking, where the countdown's full length is part of the paint.
func (c *control) snapshot(run *controlRun, window time.Duration) *ControlState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := &ControlState{Quiet: c.quiet}
	if c.paused {
		st.Paused = true
		st.PausedMs = time.Since(c.pausedAt).Milliseconds()
		st.Waiting = run != nil
	}
	// A latch with no run behind it is legal (FR94): the human pre-empting the
	// desktop is a state worth painting, and it is the only case where the strip
	// is on screen with nobody driving.
	if run == nil {
		return st
	}
	st.ID = run.id
	st.State = run.state
	st.Reason = run.reason
	st.Activity = run.activity
	st.Identity = run.identity
	st.SinceMs = time.Since(run.since).Milliseconds()
	if run.state == proto.ControlAsking {
		st.LeftMs = max(time.Until(run.deadline).Milliseconds(), 0)
		st.WindowMs = window.Milliseconds()
		if st.WindowMs == 0 {
			st.WindowMs = st.LeftMs
		}
	}
	return st
}
